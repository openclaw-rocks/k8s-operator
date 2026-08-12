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

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// MeshProvider abstracts a WireGuard-based overlay network integration.
//
// Tailscale was originally wired directly into the StatefulSet, NetworkPolicy,
// RBAC, Secret and config-enrichment paths. Adding a second provider the same
// way would have doubled that surface and forced every future networking change
// to be made and tested twice, so the integration points are collected behind
// this interface instead (#560). Tailscale is its first implementation; NetBird
// is the second.
//
// Every method takes the instance rather than closing over it, so a provider is
// a stateless value and the whole set can live in a package-level table.
// Methods return nil/empty when the provider needs nothing at that point.
type MeshProvider interface {
	// Name identifies the provider in status, conditions and container names.
	Name() string

	// SidecarContainers returns the containers that join the pod to the mesh.
	SidecarContainers(instance *openclawv1alpha1.OpenClawInstance) []corev1.Container

	// InitContainers returns init containers the provider needs, e.g. staging a
	// CLI binary onto a shared volume.
	InitContainers(instance *openclawv1alpha1.OpenClawInstance) []corev1.Container

	// PodVolumes returns the volumes backing the provider's mounts.
	PodVolumes(instance *openclawv1alpha1.OpenClawInstance) []corev1.Volume

	// MainContainerMounts returns mounts to add to the OpenClaw container, e.g.
	// a control socket the agent talks to.
	MainContainerMounts(instance *openclawv1alpha1.OpenClawInstance) []corev1.VolumeMount

	// MainContainerEnv returns env vars to add to the OpenClaw container.
	MainContainerEnv(instance *openclawv1alpha1.OpenClawInstance) []corev1.EnvVar

	// BinPathPrefixes returns PATH prefixes for provider CLIs staged into the
	// main container.
	BinPathPrefixes(instance *openclawv1alpha1.OpenClawInstance) []string

	// EgressRules returns the NetworkPolicy egress the mesh control and data
	// planes require (coordination server, STUN, WireGuard).
	EgressRules(instance *openclawv1alpha1.OpenClawInstance) []networkingv1.NetworkPolicyEgressRule

	// CredentialSecretName returns the Secret holding the join credential
	// (auth key or setup key), or "" when none is configured. The controller
	// watches it so a rotation rolls the pod, and grants the instance read
	// access to it.
	CredentialSecretName(instance *openclawv1alpha1.OpenClawInstance) string

	// StateSecretName returns the Secret the provider persists node identity in,
	// or "" when it keeps state elsewhere. A non-empty name makes the operator
	// create the Secret, grant the instance write access, and report it in
	// status.
	StateSecretName(instance *openclawv1alpha1.OpenClawInstance) string

	// NeedsServiceAccountToken reports whether a sidecar talks to the
	// Kubernetes API, which requires mounting the ServiceAccount token.
	NeedsServiceAccountToken(instance *openclawv1alpha1.OpenClawInstance) bool

	// ConfigMapData returns extra keys for the instance ConfigMap, e.g. a
	// declarative serve configuration the sidecar reads.
	ConfigMapData(instance *openclawv1alpha1.OpenClawInstance) map[string]string

	// EnrichConfig injects provider settings into the OpenClaw config JSON.
	// It must return the input unchanged when there is nothing to add.
	EnrichConfig(configJSON []byte, instance *openclawv1alpha1.OpenClawInstance) ([]byte, error)
}

// meshProviders is the full set of known providers, checked in order. Adding a
// third provider means appending one entry and implementing the interface --
// no changes to the StatefulSet, NetworkPolicy, RBAC or config paths.
var meshProviders = []MeshProvider{
	tailscaleProvider{},
	netbirdProvider{},
}

// meshProviderEnabled reports whether a provider is switched on for an instance.
// Kept next to the provider table so enablement stays a single lookup.
func meshProviderEnabled(p MeshProvider, instance *openclawv1alpha1.OpenClawInstance) bool {
	switch p.Name() {
	case MeshProviderTailscale:
		return instance.Spec.Tailscale.Enabled
	case MeshProviderNetBird:
		return instance.Spec.NetBird != nil && instance.Spec.NetBird.Enabled
	default:
		return false
	}
}

// ActiveMeshProvider returns the enabled mesh provider, or nil when none is.
//
// Only one provider may be active: two overlay networks in one pod would race
// for the same egress rules and the agent's routing. The webhook rejects the
// combination, and this returns the first enabled provider as a deterministic
// fallback if one ever slips through.
func ActiveMeshProvider(instance *openclawv1alpha1.OpenClawInstance) MeshProvider {
	for _, p := range meshProviders {
		if meshProviderEnabled(p, instance) {
			return p
		}
	}
	return nil
}

// EnabledMeshProviders returns every enabled provider. Used by validation to
// reject more than one at a time.
func EnabledMeshProviders(instance *openclawv1alpha1.OpenClawInstance) []MeshProvider {
	var out []MeshProvider
	for _, p := range meshProviders {
		if meshProviderEnabled(p, instance) {
			out = append(out, p)
		}
	}
	return out
}

// IsMeshEnabled reports whether any overlay network integration is active.
func IsMeshEnabled(instance *openclawv1alpha1.OpenClawInstance) bool {
	return ActiveMeshProvider(instance) != nil
}

// meshNeedsServiceAccountToken reports whether the active provider's sidecar
// talks to the Kubernetes API and therefore needs the ServiceAccount token
// mounted. Providers that keep state on a volume do not.
func meshNeedsServiceAccountToken(instance *openclawv1alpha1.OpenClawInstance) bool {
	mesh := ActiveMeshProvider(instance)
	return mesh != nil && mesh.NeedsServiceAccountToken(instance)
}

// MeshNeedsServiceAccountToken is the exported form used by the RBAC builder.
func MeshNeedsServiceAccountToken(instance *openclawv1alpha1.OpenClawInstance) bool {
	return meshNeedsServiceAccountToken(instance)
}

// MeshStateSecretName returns the Secret the active provider persists node
// identity in, or "" when it needs none.
func MeshStateSecretName(instance *openclawv1alpha1.OpenClawInstance) string {
	mesh := ActiveMeshProvider(instance)
	if mesh == nil {
		return ""
	}
	return mesh.StateSecretName(instance)
}

// MeshCredentialSecretName returns the Secret holding the active provider's join
// credential, or "" when none is configured.
func MeshCredentialSecretName(instance *openclawv1alpha1.OpenClawInstance) string {
	mesh := ActiveMeshProvider(instance)
	if mesh == nil {
		return ""
	}
	return mesh.CredentialSecretName(instance)
}

// MeshEgressRules returns the active provider's NetworkPolicy egress rules.
func MeshEgressRules(instance *openclawv1alpha1.OpenClawInstance) []networkingv1.NetworkPolicyEgressRule {
	mesh := ActiveMeshProvider(instance)
	if mesh == nil {
		return nil
	}
	return mesh.EgressRules(instance)
}

// MeshProviderNames returns the names of all known providers, for error messages.
func MeshProviderNames() []string {
	names := make([]string, 0, len(meshProviders))
	for _, p := range meshProviders {
		names = append(names, p.Name())
	}
	return names
}
