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

package resources

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// BuildService creates a Service for the OpenClawInstance
func BuildService(instance *openclawv1alpha1.OpenClawInstance) *corev1.Service {
	labels := Labels(instance)
	selectorLabels := SelectorLabels(instance)

	serviceType := instance.Spec.Networking.Service.Type
	if serviceType == "" {
		serviceType = corev1.ServiceTypeClusterIP
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ServiceName(instance),
			Namespace:   instance.Namespace,
			Labels:      labels,
			Annotations: instance.Spec.Networking.Service.Annotations,
		},
		Spec: corev1.ServiceSpec{
			Type:            serviceType,
			Selector:        selectorLabels,
			SessionAffinity: corev1.ServiceAffinityNone,
			Ports:           buildServicePorts(instance),
		},
	}

	return service
}

// buildServicePorts returns custom ports if specified, otherwise default ports.
//
// The operator-managed metrics port is appended in both branches. It used to be
// added only to the default ports, so an instance that set custom Service ports
// and enabled observability.metrics.serviceMonitor got a ServiceMonitor whose
// `port: metrics` endpoint had no matching Service port, and Prometheus silently
// scraped nothing.
func buildServicePorts(instance *openclawv1alpha1.OpenClawInstance) []corev1.ServicePort {
	if len(instance.Spec.Networking.Service.Ports) > 0 {
		ports := make([]corev1.ServicePort, 0, len(instance.Spec.Networking.Service.Ports)+1)
		for _, p := range instance.Spec.Networking.Service.Ports {
			protocol := p.Protocol
			if protocol == "" {
				protocol = corev1.ProtocolTCP
			}
			tp := intstr.FromInt32(p.Port)
			if p.TargetPort != nil {
				tp = intstr.FromInt32(*p.TargetPort)
			}
			ports = append(ports, corev1.ServicePort{
				Name:       p.Name,
				Port:       p.Port,
				TargetPort: tp,
				Protocol:   protocol,
			})
		}
		return appendMetricsPort(instance, ports)
	}

	// When the gateway proxy is enabled, route through the proxy ports.
	// When disabled, target the gateway and canvas ports directly.
	gwTarget := int32(GatewayProxyPort)
	canvasTarget := int32(CanvasProxyPort)
	if !IsGatewayProxyEnabled(instance) {
		gwTarget = int32(GatewayPort)
		canvasTarget = int32(CanvasPort)
	}

	ports := []corev1.ServicePort{
		{
			Name:       "gateway",
			Port:       int32(GatewayPort),
			TargetPort: intstr.FromInt32(gwTarget),
			Protocol:   corev1.ProtocolTCP,
		},
		{
			Name:       "canvas",
			Port:       int32(CanvasPort),
			TargetPort: intstr.FromInt32(canvasTarget),
			Protocol:   corev1.ProtocolTCP,
		},
	}

	if instance.Spec.Chromium.Enabled {
		ports = append(ports, corev1.ServicePort{
			Name:       "chromium",
			Port:       int32(ChromiumPort),
			TargetPort: intstr.FromInt32(int32(ChromiumPort)),
			Protocol:   corev1.ProtocolTCP,
		})
	}

	if instance.Spec.WebTerminal.Enabled {
		ports = append(ports, corev1.ServicePort{
			Name:       "web-terminal",
			Port:       int32(WebTerminalPort),
			TargetPort: intstr.FromInt32(int32(WebTerminalPort)),
			Protocol:   corev1.ProtocolTCP,
		})
	}

	return appendMetricsPort(instance, ports)
}

// appendMetricsPort adds the operator-managed "metrics" port that
// BuildServiceMonitor's endpoint resolves against.
//
// A user-supplied port takes precedence: if the instance already declares a port
// named "metrics", or one occupying the managed metrics port number, the managed
// port is skipped. Appending it anyway would produce a Service with duplicate
// port names or numbers, which the API server rejects outright — turning a
// silently-unscraped Service into a Service that will not admit at all.
func appendMetricsPort(instance *openclawv1alpha1.OpenClawInstance, ports []corev1.ServicePort) []corev1.ServicePort {
	if !IsMetricsEnabled(instance) {
		return ports
	}

	metricsPort := MetricsPort(instance)
	for _, p := range ports {
		if p.Name == MetricsPortName || p.Port == metricsPort {
			return ports
		}
	}

	return append(ports, corev1.ServicePort{
		Name:       MetricsPortName,
		Port:       metricsPort,
		TargetPort: intstr.FromInt32(metricsPort),
		Protocol:   corev1.ProtocolTCP,
	})
}

// BuildChromiumCDPService creates a headless Service for the Chromium CDP
// endpoint with publishNotReadyAddresses=true. This ensures the CDP URL
// resolves even before the pod is fully Ready, which is critical because the
// main container (OpenClaw) checks CDP connectivity during startup - before
// its own readiness probe has passed. Without this, the main ClusterIP Service
// has no endpoints and the CDP health check fails permanently.
//
// Traffic is routed directly to Chrome on ChromiumPort (9222).
func BuildChromiumCDPService(instance *openclawv1alpha1.OpenClawInstance) *corev1.Service {
	labels := Labels(instance)
	selectorLabels := SelectorLabels(instance)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ChromiumCDPServiceName(instance),
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:                     corev1.ServiceTypeClusterIP,
			ClusterIP:                corev1.ClusterIPNone, // headless
			Selector:                 selectorLabels,
			PublishNotReadyAddresses: true,
			Ports: []corev1.ServicePort{
				{
					Name:       "cdp",
					Port:       int32(ChromiumPort),
					TargetPort: intstr.FromInt32(int32(ChromiumPort)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}
