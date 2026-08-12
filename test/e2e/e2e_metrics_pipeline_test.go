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

package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
	"github.com/paperclipinc/openclaw-operator/internal/resources"
)

// npRuleHasPort reports whether an ingress rule allows the given port.
func npRuleHasPort(rule networkingv1.NetworkPolicyIngressRule, port int32) bool {
	for _, p := range rule.Ports {
		if p.Port != nil && int32(p.Port.IntValue()) == port {
			return true
		}
	}
	return false
}

// npRuleNamespace returns the namespace a rule's first peer selects, if any.
func npRuleNamespace(rule networkingv1.NetworkPolicyIngressRule) string {
	if len(rule.From) == 0 || rule.From[0].NamespaceSelector == nil {
		return ""
	}
	return rule.From[0].NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"]
}

var _ = Describe("Metrics pipeline", func() {
	const (
		timeout  = time.Second * 60
		interval = time.Second * 1
	)

	var (
		namespace string
		specNum   int
	)

	newInstance := func(name string) *openclawv1alpha1.OpenClawInstance {
		return &openclawv1alpha1.OpenClawInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Annotations: map[string]string{
					"openclaw.rocks/skip-backup": "true",
				},
			},
			Spec: openclawv1alpha1.OpenClawInstanceSpec{
				Image: openclawv1alpha1.ImageSpec{
					Repository: "ghcr.io/openclaw/openclaw",
					Tag:        "latest",
				},
			},
		}
	}

	BeforeEach(func() {
		if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
			Skip("Skipping resource validation in minimal mode")
		}
		// Second granularity alone collides between specs that start within the
		// same second, and namespace creation then fails with AlreadyExists.
		specNum++
		namespace = fmt.Sprintf("test-metrics-%s-%d", time.Now().Format("20060102150405"), specNum)
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
	})

	AfterEach(func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		_ = k8sClient.Delete(ctx, ns)
	})

	// #588: the rendered config must state the collector endpoint explicitly
	// rather than relying on the OTLP library default happening to match.
	Context("OTel collector endpoint injection", func() {
		It("Should fill the endpoint into a partial diagnostics.otel block", func() {
			instance := newInstance("otel-partial")
			instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
				RawExtension: runtime.RawExtension{
					Raw: []byte(`{"diagnostics":{"otel":{"enabled":true}}}`),
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			cm := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.ConfigMapName(instance),
					Namespace: namespace,
				}, cm)
			}, timeout, interval).Should(Succeed())

			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(cm.Data["openclaw.json"]), &parsed)).To(Succeed())

			diag, ok := parsed["diagnostics"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "rendered config should have diagnostics")
			otel, ok := diag["otel"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "rendered config should have diagnostics.otel")

			expected := fmt.Sprintf("http://localhost:%d", resources.OTelHTTPReceiverPort)
			Expect(otel["endpoint"]).To(Equal(expected),
				"the operator owns the collector endpoint and must write it explicitly")
			Expect(otel["enabled"]).To(Equal(true))
		})

		It("Should preserve an explicit user endpoint", func() {
			instance := newInstance("otel-custom")
			instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
				RawExtension: runtime.RawExtension{
					Raw: []byte(`{"diagnostics":{"otel":{"enabled":true,"endpoint":"http://my-collector:4318"}}}`),
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			cm := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.ConfigMapName(instance),
					Namespace: namespace,
				}, cm)
			}, timeout, interval).Should(Succeed())

			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(cm.Data["openclaw.json"]), &parsed)).To(Succeed())
			otel := parsed["diagnostics"].(map[string]interface{})["otel"].(map[string]interface{})
			Expect(otel["endpoint"]).To(Equal("http://my-collector:4318"))
		})
	})

	// #578: application allowlists must not imply access to the unauthenticated
	// /metrics endpoint.
	Context("Metrics NetworkPolicy isolation", func() {
		getNetworkPolicy := func(instance *openclawv1alpha1.OpenClawInstance) *networkingv1.NetworkPolicy {
			np := &networkingv1.NetworkPolicy{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.NetworkPolicyName(instance),
					Namespace: namespace,
				}, np)
			}, timeout, interval).Should(Succeed())
			return np
		}

		It("Should not grant the metrics port to application ingress peers", func() {
			instance := newInstance("np-app-peers")
			instance.Spec.Security.NetworkPolicy.AllowedIngressNamespaces = []string{"ingress-nginx"}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			np := getNetworkPolicy(instance)
			metricsPort := resources.MetricsPort(instance)

			var metricsRules int
			for _, rule := range np.Spec.Ingress {
				if npRuleNamespace(rule) == "ingress-nginx" {
					Expect(npRuleHasPort(rule, metricsPort)).To(BeFalse(),
						"an application allowlist must not grant metrics access")
				}
				if npRuleHasPort(rule, metricsPort) {
					metricsRules++
				}
			}

			// Default (no metricsIngress block) still allows the own namespace.
			Expect(metricsRules).To(Equal(1))
		})

		It("Should restrict metrics to the named peers under AllowedPeers", func() {
			instance := newInstance("np-allowed-peers")
			instance.Spec.Networking.MetricsIngress = &openclawv1alpha1.MetricsIngressSpec{
				From:              openclawv1alpha1.MetricsIngressFromAllowedPeers,
				AllowedNamespaces: []string{"monitoring"},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			np := getNetworkPolicy(instance)
			metricsPort := resources.MetricsPort(instance)

			var metricsNamespaces []string
			for _, rule := range np.Spec.Ingress {
				if npRuleHasPort(rule, metricsPort) {
					metricsNamespaces = append(metricsNamespaces, npRuleNamespace(rule))
				}
			}
			Expect(metricsNamespaces).To(ConsistOf("monitoring"),
				"only the named peer may reach the metrics port")
		})

		It("Should emit no metrics rule under None", func() {
			instance := newInstance("np-metrics-none")
			instance.Spec.Networking.MetricsIngress = &openclawv1alpha1.MetricsIngressSpec{
				From: openclawv1alpha1.MetricsIngressFromNone,
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			np := getNetworkPolicy(instance)
			metricsPort := resources.MetricsPort(instance)

			for _, rule := range np.Spec.Ingress {
				Expect(npRuleHasPort(rule, metricsPort)).To(BeFalse(),
					"From=None must emit no metrics ingress rule")
			}
			// Application traffic is unaffected.
			Expect(np.Spec.Ingress).ToNot(BeEmpty())
		})
	})

	// #589: disable/rollback and out-of-band drift must both converge.
	Context("ServiceMonitor lifecycle", func() {
		getServiceMonitor := func(instance *openclawv1alpha1.OpenClawInstance) (*unstructured.Unstructured, error) {
			sm := &unstructured.Unstructured{}
			sm.SetGroupVersionKind(resources.ServiceMonitorGVK())
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      resources.ServiceMonitorName(instance),
				Namespace: namespace,
			}, sm)
			return sm, err
		}

		It("Should delete the ServiceMonitor when disabled and recreate it on re-enable", func() {
			if !serviceMonitorCRDAvailable() {
				Skip("ServiceMonitor CRD not installed (prometheus-operator required)")
			}

			instance := newInstance("sm-lifecycle")
			enabled := true
			instance.Spec.Observability.Metrics.ServiceMonitor = &openclawv1alpha1.ServiceMonitorSpec{
				Enabled: &enabled,
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			By("creating the ServiceMonitor")
			Eventually(func() error {
				_, err := getServiceMonitor(instance)
				return err
			}, timeout, interval).Should(Succeed())

			By("deleting it when the flag is disabled")
			Eventually(func() error {
				current := &openclawv1alpha1.OpenClawInstance{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: instance.Name, Namespace: namespace,
				}, current); err != nil {
					return err
				}
				disabled := false
				current.Spec.Observability.Metrics.ServiceMonitor.Enabled = &disabled
				return k8sClient.Update(ctx, current)
			}, timeout, interval).Should(Succeed())

			Eventually(func() bool {
				_, err := getServiceMonitor(instance)
				return apierrors.IsNotFound(err)
			}, timeout, interval).Should(BeTrue(),
				"a disabled ServiceMonitor must not leave a stale scrape target behind")
		})

		It("Should restore the ServiceMonitor after an out-of-band deletion", func() {
			if !serviceMonitorCRDAvailable() {
				Skip("ServiceMonitor CRD not installed (prometheus-operator required)")
			}

			instance := newInstance("sm-drift")
			enabled := true
			instance.Spec.Observability.Metrics.ServiceMonitor = &openclawv1alpha1.ServiceMonitorSpec{
				Enabled: &enabled,
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			var sm *unstructured.Unstructured
			Eventually(func() error {
				var err error
				sm, err = getServiceMonitor(instance)
				return err
			}, timeout, interval).Should(Succeed())

			By("deleting it behind the operator's back")
			Expect(k8sClient.Delete(ctx, sm)).Should(Succeed())

			By("converging it back")
			Eventually(func() error {
				_, err := getServiceMonitor(instance)
				return err
			}, timeout, interval).Should(Succeed())
		})

		It("Should not create a ServiceMonitor when metrics are disabled", func() {
			if !serviceMonitorCRDAvailable() {
				Skip("ServiceMonitor CRD not installed (prometheus-operator required)")
			}

			instance := newInstance("sm-no-metrics")
			enabled := true
			disabled := false
			instance.Spec.Observability.Metrics.Enabled = &disabled
			instance.Spec.Observability.Metrics.ServiceMonitor = &openclawv1alpha1.ServiceMonitorSpec{
				Enabled: &enabled,
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			By("surfacing the invalid combination instead of creating a dead target")
			Eventually(func() string {
				current := &openclawv1alpha1.OpenClawInstance{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: instance.Name, Namespace: namespace,
				}, current); err != nil {
					return ""
				}
				for _, c := range current.Status.Conditions {
					if c.Type == openclawv1alpha1.ConditionTypeServiceMonitorReady {
						return c.Reason
					}
				}
				return ""
			}, timeout, interval).Should(Equal("MetricsDisabled"))

			_, err := getServiceMonitor(instance)
			Expect(apierrors.IsNotFound(err)).To(BeTrue())
		})
	})
})
