/*
Copyright 2026 Paperclip Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	otelprom "go.opentelemetry.io/contrib/bridges/prometheus"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	otelmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
	"github.com/paperclipinc/openclaw-operator/internal/controller"
	"github.com/paperclipinc/openclaw-operator/internal/registry"
	"github.com/paperclipinc/openclaw-operator/internal/skillpacks"
)

// version is set at build time via ldflags (see .goreleaser.yaml).
var version = "dev"

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(openclawv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var otlpEndpoint string
	var otlpInsecure bool
	var watchNamespacesFlag string
	var tlsOpts []func(*tls.Config)

	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true, "If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false, "If set, HTTP/2 will be enabled for the metrics and webhook servers.")
	flag.StringVar(&otlpEndpoint, "otlp-endpoint", "", "OTLP gRPC endpoint for metrics export (e.g. collector.observability.svc:4317). Also respects OTEL_EXPORTER_OTLP_ENDPOINT env var.")
	flag.BoolVar(&otlpInsecure, "otlp-insecure", true, "If set, OTLP exporter connects without TLS.")
	flag.StringVar(&watchNamespacesFlag, "watch-namespaces", "", "Comma-separated list of namespaces to watch. If empty, the operator watches all namespaces (cluster-scoped). Set this when running with namespaced RBAC.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being affected by the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	webhookServer := webhook.NewServer(webhook.Options{
		TLSOpts: tlsOpts,
	})

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'.
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	mgrOpts := ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "openclaw-operator.openclaw.rocks",
	}

	operatorNamespace := os.Getenv("POD_NAMESPACE")
	if operatorNamespace == "" {
		operatorNamespace = "openclaw-operator-system"
	}

	watchNamespaces := parseWatchNamespaces(watchNamespacesFlag)
	if len(watchNamespaces) > 0 {
		mgrOpts.Cache = buildCacheOptions(watchNamespaces, operatorNamespace)
		setupLog.Info("restricting watch to namespaces", "namespaces", watchNamespaces, "operatorNamespace", operatorNamespace)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), mgrOpts)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	versionResolver := registry.NewResolver(5 * time.Minute)
	skillPackResolver := skillpacks.NewResolver(5*time.Minute, os.Getenv("GITHUB_TOKEN"))

	if err = (&controller.OpenClawInstanceReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		Recorder:          mgr.GetEventRecorderFor("openclawinstance-controller"),
		OperatorNamespace: operatorNamespace,
		VersionResolver:   versionResolver,
		SkillPackResolver: skillPackResolver,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OpenClawInstance")
		os.Exit(1)
	}

	if err = (&controller.OpenClawSelfConfigReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("openclawselfconfig-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "OpenClawSelfConfig")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	// Readiness check waits for informer caches to sync before reporting ready.
	// This prevents the Deployment from becoming Available before the controllers
	// are actually processing events (which caused flaky E2E tests).
	var cacheSynced atomic.Bool
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		if mgr.GetCache().WaitForCacheSync(ctx) {
			cacheSynced.Store(true)
			setupLog.Info("informer caches synced, readiness check will pass")
		} else {
			setupLog.Error(fmt.Errorf("cache sync timed out"), "informer caches failed to sync")
		}
	}()
	if err := mgr.AddReadyzCheck("readyz", func(_ *http.Request) error {
		if !cacheSynced.Load() {
			return fmt.Errorf("informer caches not yet synced")
		}
		return nil
	}); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	// Set up OTLP metrics export if configured. The flag takes precedence
	// over the environment variable. OTLP export is supplementary - if setup
	// fails, the operator continues without it rather than crash-looping.
	var shutdownOTLP func(context.Context) error
	if otlpEndpoint == "" {
		otlpEndpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	}
	if otlpEndpoint != "" {
		var otlpErr error
		shutdownOTLP, otlpErr = setupOTLPMetrics(otlpEndpoint, otlpInsecure)
		if otlpErr != nil {
			setupLog.Error(otlpErr, "unable to set up OTLP metrics exporter, continuing without OTLP")
		} else {
			setupLog.Info("OTLP metrics exporter configured", "endpoint", otlpEndpoint, "insecure", otlpInsecure)
		}
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
	}

	// Flush remaining OTLP metrics before exit
	if shutdownOTLP != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if shutdownErr := shutdownOTLP(shutdownCtx); shutdownErr != nil {
			setupLog.Error(shutdownErr, "error shutting down OTLP metrics exporter")
		}
	}
}

// parseWatchNamespaces splits the --watch-namespaces flag value into a
// deduplicated list of namespace names, dropping empty entries.
func parseWatchNamespaces(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	seen := make(map[string]struct{})
	out := make([]string, 0)
	for _, ns := range strings.Split(raw, ",") {
		ns = strings.TrimSpace(ns)
		if ns == "" {
			continue
		}
		if _, ok := seen[ns]; ok {
			continue
		}
		seen[ns] = struct{}{}
		out = append(out, ns)
	}
	return out
}

// buildCacheOptions restricts the manager cache to the watched namespaces.
//
// The operator also reads its own backup credentials Secret from its release
// namespace. That namespace used to be added to DefaultNamespaces, which
// started an informer there for every watched type -- Deployments,
// StatefulSets, Services and the rest. The chart only renders Roles in the
// watched namespaces, so those informers failed with Forbidden and the manager
// never started (#586). Only the Secret informer is widened instead, which is
// all the credential read actually needs.
func buildCacheOptions(watchNamespaces []string, operatorNamespace string) cache.Options {
	defaultNamespaces := make(map[string]cache.Config, len(watchNamespaces))
	secretNamespaces := make(map[string]cache.Config, len(watchNamespaces)+1)
	for _, ns := range watchNamespaces {
		defaultNamespaces[ns] = cache.Config{}
		secretNamespaces[ns] = cache.Config{}
	}
	secretNamespaces[operatorNamespace] = cache.Config{}

	return cache.Options{
		DefaultNamespaces: defaultNamespaces,
		ByObject: map[client.Object]cache.ByObject{
			&corev1.Secret{}: {Namespaces: secretNamespaces},
		},
	}
}

// setupOTLPMetrics configures an OTLP gRPC metrics exporter that bridges all
// Prometheus metrics registered with controller-runtime's default registry.
// This includes both built-in controller-runtime metrics (workqueue, client,
// informer) and custom openclaw_* metrics. Returns a shutdown function that
// flushes remaining metrics during graceful termination.
func setupOTLPMetrics(endpoint string, insecure bool) (func(context.Context) error, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := []otlpmetricgrpc.Option{
		otlpmetricgrpc.WithEndpoint(endpoint),
	}
	if insecure {
		opts = append(opts, otlpmetricgrpc.WithInsecure())
	}

	exporter, err := otlpmetricgrpc.New(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("creating OTLP metrics exporter: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("openclaw-operator"),
			semconv.ServiceVersion(version),
		),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("creating OTel resource: %w", err)
	}

	// Bridge controller-runtime's Prometheus registry to OTel. All metrics
	// registered with metrics.Registry are automatically exported via OTLP
	// alongside the existing Prometheus scrape endpoint.
	bridge := otelprom.NewMetricProducer(
		otelprom.WithGatherer(metrics.Registry),
	)

	provider := otelmetric.NewMeterProvider(
		otelmetric.WithReader(
			otelmetric.NewPeriodicReader(exporter,
				otelmetric.WithInterval(30*time.Second),
				otelmetric.WithProducer(bridge),
			),
		),
		otelmetric.WithResource(res),
	)

	return provider.Shutdown, nil
}
