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
	"k8s.io/apimachinery/pkg/util/intstr"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// MeshProviderTailscale is the provider name for Tailscale.
const MeshProviderTailscale = "tailscale"

// tailscaleProvider is the MeshProvider implementation for Tailscale.
//
// It delegates to the existing build* helpers rather than reimplementing them,
// so introducing the abstraction cannot change the rendered pod for anyone
// already running Tailscale -- the existing test suite is the proof.
type tailscaleProvider struct{}

var _ MeshProvider = tailscaleProvider{}

func (tailscaleProvider) Name() string { return MeshProviderTailscale }

func (tailscaleProvider) SidecarContainers(instance *openclawv1alpha1.OpenClawInstance) []corev1.Container {
	return []corev1.Container{buildTailscaleContainer(instance)}
}

func (tailscaleProvider) InitContainers(instance *openclawv1alpha1.OpenClawInstance) []corev1.Container {
	// Stages the tailscale CLI so the agent can call "tailscale whois".
	return []corev1.Container{buildTailscaleBinInitContainer(instance)}
}

func (tailscaleProvider) PodVolumes(_ *openclawv1alpha1.OpenClawInstance) []corev1.Volume {
	// State lives under /tmp, so no separate state volume is needed.
	return []corev1.Volume{
		{
			Name:         "tailscale-socket",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
		{
			Name:         "tailscale-bin",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
		{
			Name:         "tailscale-tmp",
			VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
	}
}

func (tailscaleProvider) MainContainerMounts(_ *openclawv1alpha1.OpenClawInstance) []corev1.VolumeMount {
	// Socket for "tailscale whois", bin for the CLI binary.
	return []corev1.VolumeMount{
		{Name: "tailscale-socket", MountPath: TailscaleSocketDir, ReadOnly: true},
		{Name: "tailscale-bin", MountPath: TailscaleBinPath, ReadOnly: true},
	}
}

func (tailscaleProvider) MainContainerEnv(_ *openclawv1alpha1.OpenClawInstance) []corev1.EnvVar {
	// The main container talks to the sidecar's tailscaled over this socket.
	return []corev1.EnvVar{{Name: "TS_SOCKET", Value: TailscaleSocketPath}}
}

func (tailscaleProvider) BinPathPrefixes(_ *openclawv1alpha1.OpenClawInstance) []string {
	return []string{TailscaleBinPath}
}

// EgressRules returns only the mesh-specific egress. Kubernetes API egress
// (6443) is emitted once by buildEgressRules for any instance that needs it --
// self-configure or a provider whose sidecar talks to the API -- so contributing
// it here too would render the rule twice for a Tailscale + selfConfigure
// instance.
func (tailscaleProvider) EgressRules(_ *openclawv1alpha1.OpenClawInstance) []networkingv1.NetworkPolicyEgressRule {
	return []networkingv1.NetworkPolicyEgressRule{
		{
			// STUN for NAT traversal and the WireGuard data plane.
			To: []networkingv1.NetworkPolicyPeer{},
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: Ptr(corev1.ProtocolUDP), Port: Ptr(intstr.FromInt(3478))},
				{Protocol: Ptr(corev1.ProtocolUDP), Port: Ptr(intstr.FromInt(41641))},
			},
		},
	}
}

func (tailscaleProvider) CredentialSecretName(instance *openclawv1alpha1.OpenClawInstance) string {
	if instance.Spec.Tailscale.AuthKeySecretRef == nil {
		return ""
	}
	return instance.Spec.Tailscale.AuthKeySecretRef.Name
}

func (tailscaleProvider) StateSecretName(instance *openclawv1alpha1.OpenClawInstance) string {
	// Node identity and TLS certs are persisted to a Secret so state survives
	// restarts (otherwise hostnames increment and certs are re-issued).
	return TailscaleStateSecretName(instance)
}

func (tailscaleProvider) NeedsServiceAccountToken(_ *openclawv1alpha1.OpenClawInstance) bool {
	// containerboot reads and writes its state Secret via the API server.
	return true
}

func (tailscaleProvider) ConfigMapData(instance *openclawv1alpha1.OpenClawInstance) map[string]string {
	// The sidecar reads this via TS_SERVE_CONFIG.
	return map[string]string{
		TailscaleServeConfigKey: BuildTailscaleServeConfig(instance),
	}
}

func (tailscaleProvider) EnrichConfig(configJSON []byte, instance *openclawv1alpha1.OpenClawInstance) ([]byte, error) {
	return enrichConfigWithTailscale(configJSON, instance)
}
