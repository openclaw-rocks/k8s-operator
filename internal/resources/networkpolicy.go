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
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// BuildNetworkPolicy creates a NetworkPolicy for the OpenClawInstance
// This implements a default-deny with selective allowlist approach
func BuildNetworkPolicy(instance *openclawv1alpha1.OpenClawInstance) *networkingv1.NetworkPolicy {
	labels := Labels(instance)
	selectorLabels := SelectorLabels(instance)

	np := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      NetworkPolicyName(instance),
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: selectorLabels,
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: buildIngressRules(instance),
			Egress:  buildEgressRules(instance),
		},
	}

	return np
}

// networkPolicyIngressPorts returns the application ports to allow in
// NetworkPolicy ingress rules. When custom service ports are configured, those
// are used instead of the defaults.
//
// The metrics port is deliberately not included: it is rendered in its own rule
// by buildMetricsIngressRules so that application allowlists don't implicitly
// grant access to the unauthenticated /metrics endpoint (#578).
func networkPolicyIngressPorts(instance *openclawv1alpha1.OpenClawInstance) []networkingv1.NetworkPolicyPort {
	if len(instance.Spec.Networking.Service.Ports) > 0 {
		ports := make([]networkingv1.NetworkPolicyPort, 0, len(instance.Spec.Networking.Service.Ports))
		for _, p := range instance.Spec.Networking.Service.Ports {
			protocol := p.Protocol
			if protocol == "" {
				protocol = corev1.ProtocolTCP
			}
			port := p.Port
			if p.TargetPort != nil {
				port = *p.TargetPort
			}
			ports = append(ports, networkingv1.NetworkPolicyPort{
				Protocol: Ptr(protocol),
				Port:     Ptr(intstr.FromInt32(port)),
			})
		}
		return ports
	}

	// Use proxy ports when the gateway proxy sidecar is enabled (default),
	// otherwise use the direct gateway/canvas ports.
	gwPort := int32(GatewayProxyPort)
	canvasPort := int32(CanvasProxyPort)
	if !IsGatewayProxyEnabled(instance) {
		gwPort = int32(GatewayPort)
		canvasPort = int32(CanvasPort)
	}

	ports := []networkingv1.NetworkPolicyPort{
		{
			Protocol: Ptr(corev1.ProtocolTCP),
			Port:     Ptr(intstr.FromInt32(gwPort)),
		},
		{
			Protocol: Ptr(corev1.ProtocolTCP),
			Port:     Ptr(intstr.FromInt32(canvasPort)),
		},
	}

	if instance.Spec.WebTerminal.Enabled {
		ports = append(ports, networkingv1.NetworkPolicyPort{
			Protocol: Ptr(corev1.ProtocolTCP),
			Port:     Ptr(intstr.FromInt32(int32(WebTerminalPort))),
		})
	}

	if instance.Spec.Chromium.Enabled {
		ports = append(ports, networkingv1.NetworkPolicyPort{
			Protocol: Ptr(corev1.ProtocolTCP),
			Port:     Ptr(intstr.FromInt32(int32(ChromiumPort))),
		})
	}

	return ports
}

// namespacePeer builds a peer matching a namespace by name, optionally narrowed
// to specific pods within it.
func namespacePeer(namespace string, podSelector *metav1.LabelSelector) networkingv1.NetworkPolicyPeer {
	return networkingv1.NetworkPolicyPeer{
		NamespaceSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"kubernetes.io/metadata.name": namespace,
			},
		},
		PodSelector: podSelector,
	}
}

// buildIngressRules creates the ingress rules for the NetworkPolicy
func buildIngressRules(instance *openclawv1alpha1.OpenClawInstance) []networkingv1.NetworkPolicyIngressRule {
	rules := []networkingv1.NetworkPolicyIngressRule{}
	npPorts := networkPolicyIngressPorts(instance)

	// Allow from same namespace by default
	rules = append(rules, networkingv1.NetworkPolicyIngressRule{
		From:  []networkingv1.NetworkPolicyPeer{namespacePeer(instance.Namespace, nil)},
		Ports: npPorts,
	})

	// Allow from specified namespaces
	for _, ns := range instance.Spec.Security.NetworkPolicy.AllowedIngressNamespaces {
		rules = append(rules, networkingv1.NetworkPolicyIngressRule{
			From:  []networkingv1.NetworkPolicyPeer{namespacePeer(ns, nil)},
			Ports: npPorts,
		})
	}

	// Allow from specified CIDRs
	for _, cidr := range instance.Spec.Security.NetworkPolicy.AllowedIngressCIDRs {
		rules = append(rules, networkingv1.NetworkPolicyIngressRule{
			From: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{
						CIDR: cidr,
					},
				},
			},
			Ports: npPorts,
		})
	}

	rules = append(rules, buildMetricsIngressRules(instance)...)

	return rules
}

// buildMetricsIngressRules creates the ingress rules covering the metrics port.
//
// Metrics get their own rule so the unauthenticated /metrics endpoint can be
// restricted independently of application traffic (#578). Without a
// metricsIngress block the instance's own namespace is allowed, preserving the
// behavior from before the field existed.
func buildMetricsIngressRules(instance *openclawv1alpha1.OpenClawInstance) []networkingv1.NetworkPolicyIngressRule {
	if !IsMetricsEnabled(instance) {
		return nil
	}

	metricsPorts := []networkingv1.NetworkPolicyPort{
		{
			Protocol: Ptr(corev1.ProtocolTCP),
			Port:     Ptr(intstr.FromInt32(MetricsPort(instance))),
		},
	}

	cfg := instance.Spec.Networking.MetricsIngress
	mode := openclawv1alpha1.MetricsIngressFromSameNamespace
	if cfg != nil && cfg.From != "" {
		mode = cfg.From
	}

	switch mode {
	case openclawv1alpha1.MetricsIngressFromNone:
		return nil

	case openclawv1alpha1.MetricsIngressFromAllowedPeers:
		peers := []networkingv1.NetworkPolicyPeer{}
		if cfg != nil {
			for _, ns := range cfg.AllowedNamespaces {
				peers = append(peers, namespacePeer(ns, cfg.PodSelector))
			}
			// The pod selector narrows namespace peers only -- a CIDR peer has
			// no pod identity to match against.
			for _, cidr := range cfg.AllowedCIDRs {
				peers = append(peers, networkingv1.NetworkPolicyPeer{
					IPBlock: &networkingv1.IPBlock{CIDR: cidr},
				})
			}
		}
		if len(peers) == 0 {
			// AllowedPeers with nothing listed means nobody may scrape. Emitting
			// a rule with an empty From would allow everything, so emit none.
			return nil
		}
		rules := make([]networkingv1.NetworkPolicyIngressRule, 0, len(peers))
		for _, peer := range peers {
			rules = append(rules, networkingv1.NetworkPolicyIngressRule{
				From:  []networkingv1.NetworkPolicyPeer{peer},
				Ports: metricsPorts,
			})
		}
		return rules

	default: // SameNamespace
		return []networkingv1.NetworkPolicyIngressRule{
			{
				From:  []networkingv1.NetworkPolicyPeer{namespacePeer(instance.Namespace, nil)},
				Ports: metricsPorts,
			},
		}
	}
}

// buildEgressRules creates the egress rules for the NetworkPolicy
func buildEgressRules(instance *openclawv1alpha1.OpenClawInstance) []networkingv1.NetworkPolicyEgressRule {
	rules := []networkingv1.NetworkPolicyEgressRule{}

	// Allow DNS if enabled (default: true)
	allowDNS := instance.Spec.Security.NetworkPolicy.AllowDNS == nil || *instance.Spec.Security.NetworkPolicy.AllowDNS
	if allowDNS {
		rules = append(rules, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: Ptr(corev1.ProtocolUDP),
					Port:     Ptr(intstr.FromInt(53)),
				},
				{
					Protocol: Ptr(corev1.ProtocolTCP),
					Port:     Ptr(intstr.FromInt(53)),
				},
			},
		})
	}

	// Allow HTTPS egress for AI APIs (port 443)
	// This is essential for OpenClaw to communicate with AI providers
	rules = append(rules, networkingv1.NetworkPolicyEgressRule{
		To: []networkingv1.NetworkPolicyPeer{},
		Ports: []networkingv1.NetworkPolicyPort{
			{
				Protocol: Ptr(corev1.ProtocolTCP),
				Port:     Ptr(intstr.FromInt(443)),
			},
		},
	})

	// Allow K8s API server egress when self-configure is enabled, or when the
	// mesh provider's sidecar manages its state via the API (Tailscale's
	// containerboot does). Port 6443 covers clusters where the API server
	// listens on a non-standard port (e.g., K3s DNATs 443 -> 6443 before
	// NetworkPolicy evaluation). Emitted once here rather than also by the
	// provider, so an instance needing it for both reasons gets one rule.
	if instance.Spec.SelfConfigure.Enabled || MeshNeedsServiceAccountToken(instance) {
		rules = append(rules, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: Ptr(corev1.ProtocolTCP),
					Port:     Ptr(intstr.FromInt(6443)),
				},
			},
		})
	}

	// Mesh provider control and data plane egress (#560). Each provider owns
	// its own coordination server, STUN and WireGuard ports.
	rules = append(rules, MeshEgressRules(instance)...)

	// Allow egress to the Chromium sidecar. The main container reaches Chrome
	// via a headless Service that resolves to the pod's own IP. Cilium
	// short-circuits self-traffic and doesn't require this rule, but it's
	// correct to include for portability (e.g. Calico).
	if instance.Spec.Chromium.Enabled {
		rules = append(rules, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					PodSelector: &metav1.LabelSelector{
						MatchLabels: SelectorLabels(instance),
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: Ptr(corev1.ProtocolTCP),
					Port:     Ptr(intstr.FromInt32(int32(ChromiumPort))),
				},
			},
		})
	}

	// Allow additional egress CIDRs if specified
	for _, cidr := range instance.Spec.Security.NetworkPolicy.AllowedEgressCIDRs {
		rules = append(rules, networkingv1.NetworkPolicyEgressRule{
			To: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{
						CIDR: cidr,
					},
				},
			},
		})
	}

	// Append user-defined additional egress rules
	rules = append(rules, instance.Spec.Security.NetworkPolicy.AdditionalEgress...)

	return rules
}
