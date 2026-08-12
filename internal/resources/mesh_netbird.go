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

// MeshProviderNetBird is the provider name for NetBird.
const MeshProviderNetBird = "netbird"

const (
	// DefaultNetBirdImage is the default image for the NetBird sidecar.
	DefaultNetBirdImage = "docker.io/netbirdio/netbird"

	// DefaultNetBirdSetupKeySecretKey is the default key in the setup key Secret.
	DefaultNetBirdSetupKeySecretKey = "setupkey"

	// NetBirdStateDir is where the NetBird client keeps its peer identity.
	// It lives on an emptyDir under /tmp so the root filesystem stays read-only.
	NetBirdStateDir = "/tmp/netbird"

	// NetBirdManagementPort is the default port of a self-hosted NetBird
	// management server, used for NetworkPolicy egress.
	NetBirdManagementPort = 33073

	// NetBirdSignalPort is the default port of the NetBird signal server.
	NetBirdSignalPort = 10000

	// NetBirdWireGuardPort is the NetBird WireGuard data plane port.
	NetBirdWireGuardPort = 51820
)

// netbirdProvider is the MeshProvider implementation for NetBird (#560).
//
// It deliberately mirrors the Tailscale sidecar's security posture: the client
// runs in netstack (userspace) mode, so the container needs neither NET_ADMIN
// nor /dev/net/tun and can keep the Restricted PSS defaults the operator applies
// everywhere else -- all capabilities dropped, read-only root filesystem,
// non-root. A kernel-mode NetBird peer would need elevated capabilities, which
// is a different security decision than this operator makes by default.
type netbirdProvider struct{}

var _ MeshProvider = netbirdProvider{}

func (netbirdProvider) Name() string { return MeshProviderNetBird }

// GetNetBirdImage returns the full NetBird sidecar image reference.
func GetNetBirdImage(instance *openclawv1alpha1.OpenClawInstance) string {
	spec := instance.Spec.NetBird
	repo := DefaultNetBirdImage
	tag := DefaultImageTag
	digest := ""
	if spec != nil {
		if spec.Image.Repository != "" {
			repo = spec.Image.Repository
		}
		if spec.Image.Tag != "" {
			tag = spec.Image.Tag
		}
		digest = spec.Image.Digest
	}
	if digest != "" {
		return repo + "@" + digest
	}
	return repo + ":" + tag
}

// NetBirdHostname returns the peer name for this instance.
func NetBirdHostname(instance *openclawv1alpha1.OpenClawInstance) string {
	if instance.Spec.NetBird != nil && instance.Spec.NetBird.Hostname != "" {
		return instance.Spec.NetBird.Hostname
	}
	return instance.Name
}

func (netbirdProvider) SidecarContainers(instance *openclawv1alpha1.OpenClawInstance) []corev1.Container {
	spec := instance.Spec.NetBird

	env := []corev1.EnvVar{
		// Userspace networking: no NET_ADMIN, no /dev/net/tun, so the sidecar
		// keeps the same Restricted PSS posture as every other container here.
		{Name: "NB_USE_NETSTACK_MODE", Value: "true"},
		{Name: "NB_STATE_DIR", Value: NetBirdStateDir},
		{Name: "NB_CONFIG", Value: NetBirdStateDir + "/config.json"},
		{Name: "NB_HOSTNAME", Value: NetBirdHostname(instance)},
		// The client is non-interactive in a pod; without this it would block
		// waiting for a browser-based login instead of using the setup key.
		{Name: "NB_FOREGROUND_MODE", Value: "true"},
	}

	if spec != nil && spec.ManagementURL != "" {
		env = append(env, corev1.EnvVar{Name: "NB_MANAGEMENT_URL", Value: spec.ManagementURL})
	}

	if spec != nil && spec.SetupKeySecretRef != nil {
		key := spec.SetupKeySecretKey
		if key == "" {
			key = DefaultNetBirdSetupKeySecretKey
		}
		env = append(env, corev1.EnvVar{
			Name: "NB_SETUP_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: *spec.SetupKeySecretRef,
					Key:                  key,
				},
			},
		})
	}

	return []corev1.Container{{
		Name:            "netbird",
		Image:           GetNetBirdImage(instance),
		ImagePullPolicy: corev1.PullIfNotPresent,
		Env:             env,
		VolumeMounts: []corev1.VolumeMount{
			{
				// Peer identity and config. An emptyDir means the peer
				// re-enrolls on restart, which a reusable setup key handles.
				Name:      "netbird-state",
				MountPath: NetBirdStateDir,
			},
		},
		Resources: buildNetBirdResourceRequirements(instance),
		SecurityContext: &corev1.SecurityContext{
			AllowPrivilegeEscalation: Ptr(false),
			ReadOnlyRootFilesystem:   Ptr(true),
			RunAsNonRoot:             Ptr(podRunAsNonRoot(instance)),
			Capabilities: &corev1.Capabilities{
				Drop: []corev1.Capability{"ALL"},
			},
			SeccompProfile: &corev1.SeccompProfile{
				Type: corev1.SeccompProfileTypeRuntimeDefault,
			},
		},
		TerminationMessagePath:   corev1.TerminationMessagePathDefault,
		TerminationMessagePolicy: corev1.TerminationMessageReadFile,
	}}
}

// buildNetBirdResourceRequirements creates resource requirements for the NetBird
// sidecar, using the same defaults as the Tailscale sidecar.
func buildNetBirdResourceRequirements(instance *openclawv1alpha1.OpenClawInstance) corev1.ResourceRequirements {
	req := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{},
		Limits:   corev1.ResourceList{},
	}
	var res ResourcesSpecView
	if instance.Spec.NetBird != nil {
		res = ResourcesSpecView{
			RequestsCPU:    instance.Spec.NetBird.Resources.Requests.CPU,
			RequestsMemory: instance.Spec.NetBird.Resources.Requests.Memory,
			LimitsCPU:      instance.Spec.NetBird.Resources.Limits.CPU,
			LimitsMemory:   instance.Spec.NetBird.Resources.Limits.Memory,
		}
	}
	req.Requests[corev1.ResourceCPU] = ParseQuantity(res.RequestsCPU, "50m")
	req.Requests[corev1.ResourceMemory] = ParseQuantity(res.RequestsMemory, "64Mi")
	req.Limits[corev1.ResourceCPU] = ParseQuantity(res.LimitsCPU, "200m")
	req.Limits[corev1.ResourceMemory] = ParseQuantity(res.LimitsMemory, "256Mi")
	return req
}

// ResourcesSpecView flattens a ResourcesSpec so a nil provider spec can be
// handled without repeating nil checks per field.
type ResourcesSpecView struct {
	RequestsCPU    string
	RequestsMemory string
	LimitsCPU      string
	LimitsMemory   string
}

// InitContainers returns nothing: NetBird has no CLI the agent needs staged,
// unlike Tailscale's "tailscale whois" for SSO.
func (netbirdProvider) InitContainers(_ *openclawv1alpha1.OpenClawInstance) []corev1.Container {
	return nil
}

func (netbirdProvider) PodVolumes(_ *openclawv1alpha1.OpenClawInstance) []corev1.Volume {
	return []corev1.Volume{{
		Name:         "netbird-state",
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}}
}

// MainContainerMounts returns nothing: the agent does not talk to the NetBird
// client directly, it just reaches the mesh through the pod's network namespace.
func (netbirdProvider) MainContainerMounts(_ *openclawv1alpha1.OpenClawInstance) []corev1.VolumeMount {
	return nil
}

func (netbirdProvider) MainContainerEnv(_ *openclawv1alpha1.OpenClawInstance) []corev1.EnvVar {
	return nil
}

func (netbirdProvider) BinPathPrefixes(_ *openclawv1alpha1.OpenClawInstance) []string {
	return nil
}

func (netbirdProvider) EgressRules(instance *openclawv1alpha1.OpenClawInstance) []networkingv1.NetworkPolicyEgressRule {
	// HTTPS (443) to the management and signal servers is already allowed by the
	// baseline egress rules, so only the non-standard control-plane ports and
	// the WireGuard data plane need adding.
	ports := []networkingv1.NetworkPolicyPort{
		{Protocol: Ptr(corev1.ProtocolTCP), Port: Ptr(intstr.FromInt(NetBirdManagementPort))},
		{Protocol: Ptr(corev1.ProtocolTCP), Port: Ptr(intstr.FromInt(NetBirdSignalPort))},
		{Protocol: Ptr(corev1.ProtocolUDP), Port: Ptr(intstr.FromInt(NetBirdWireGuardPort))},
		// STUN/TURN for NAT traversal, same port as Tailscale uses.
		{Protocol: Ptr(corev1.ProtocolUDP), Port: Ptr(intstr.FromInt(3478))},
	}
	return []networkingv1.NetworkPolicyEgressRule{{
		To:    []networkingv1.NetworkPolicyPeer{},
		Ports: ports,
	}}
}

func (netbirdProvider) CredentialSecretName(instance *openclawv1alpha1.OpenClawInstance) string {
	if instance.Spec.NetBird == nil || instance.Spec.NetBird.SetupKeySecretRef == nil {
		return ""
	}
	return instance.Spec.NetBird.SetupKeySecretRef.Name
}

// StateSecretName returns "": NetBird keeps peer state on a volume, so the
// operator neither creates a Secret for it nor grants API access to one.
func (netbirdProvider) StateSecretName(_ *openclawv1alpha1.OpenClawInstance) string {
	return ""
}

// NeedsServiceAccountToken returns false: nothing in the NetBird sidecar talks
// to the Kubernetes API, so no token is mounted.
func (netbirdProvider) NeedsServiceAccountToken(_ *openclawv1alpha1.OpenClawInstance) bool {
	return false
}

// ConfigMapData returns nothing: NetBird has no declarative serve config
// equivalent to Tailscale's TS_SERVE_CONFIG.
func (netbirdProvider) ConfigMapData(_ *openclawv1alpha1.OpenClawInstance) map[string]string {
	return nil
}

// EnrichConfig returns the config unchanged. NetBird has no SSO identity header
// equivalent to "tailscale whois", so there is no gateway auth mode to inject.
func (netbirdProvider) EnrichConfig(configJSON []byte, _ *openclawv1alpha1.OpenClawInstance) ([]byte, error) {
	return configJSON, nil
}
