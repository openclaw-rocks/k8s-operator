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
	"bytes"
	"testing"

	corev1 "k8s.io/api/core/v1"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// Mesh provider abstraction (#560).

func newNetBirdInstance(name string) *openclawv1alpha1.OpenClawInstance {
	instance := newTestInstance(name)
	instance.Spec.NetBird = &openclawv1alpha1.NetBirdSpec{
		Enabled: true,
		SetupKeySecretRef: &corev1.LocalObjectReference{
			Name: "netbird-setup-key",
		},
	}
	return instance
}

func TestActiveMeshProvider_NoneByDefault(t *testing.T) {
	instance := newTestInstance("mesh-none")
	if p := ActiveMeshProvider(instance); p != nil {
		t.Errorf("expected no mesh provider by default, got %q", p.Name())
	}
	if IsMeshEnabled(instance) {
		t.Error("IsMeshEnabled should be false by default")
	}
}

func TestActiveMeshProvider_Tailscale(t *testing.T) {
	instance := newTestInstance("mesh-ts")
	instance.Spec.Tailscale.Enabled = true

	p := ActiveMeshProvider(instance)
	if p == nil || p.Name() != MeshProviderTailscale {
		t.Fatalf("expected the tailscale provider, got %v", p)
	}
}

func TestActiveMeshProvider_NetBird(t *testing.T) {
	instance := newNetBirdInstance("mesh-nb")

	p := ActiveMeshProvider(instance)
	if p == nil || p.Name() != MeshProviderNetBird {
		t.Fatalf("expected the netbird provider, got %v", p)
	}
}

// A disabled NetBird block must not activate the provider.
func TestActiveMeshProvider_NetBirdDisabled(t *testing.T) {
	instance := newTestInstance("mesh-nb-off")
	instance.Spec.NetBird = &openclawv1alpha1.NetBirdSpec{Enabled: false}

	if p := ActiveMeshProvider(instance); p != nil {
		t.Errorf("expected no provider when netbird.enabled is false, got %q", p.Name())
	}
}

func TestEnabledMeshProviders_DetectsConflict(t *testing.T) {
	instance := newNetBirdInstance("mesh-both")
	instance.Spec.Tailscale.Enabled = true

	if got := EnabledMeshProviders(instance); len(got) != 2 {
		t.Errorf("expected both providers reported as enabled, got %d", len(got))
	}
}

// Tailscale keeps needing the ServiceAccount token (containerboot uses the K8s
// API); NetBird keeps state on a volume and must not get one.
func TestMeshNeedsServiceAccountToken(t *testing.T) {
	ts := newTestInstance("mesh-token-ts")
	ts.Spec.Tailscale.Enabled = true
	if !MeshNeedsServiceAccountToken(ts) {
		t.Error("tailscale needs the ServiceAccount token for its state Secret")
	}

	nb := newNetBirdInstance("mesh-token-nb")
	if MeshNeedsServiceAccountToken(nb) {
		t.Error("netbird stores state on a volume and must not mount a token")
	}

	none := newTestInstance("mesh-token-none")
	if MeshNeedsServiceAccountToken(none) {
		t.Error("no provider means no token")
	}
}

func TestMeshStateSecretName(t *testing.T) {
	ts := newTestInstance("mesh-state-ts")
	ts.Spec.Tailscale.Enabled = true
	if got := MeshStateSecretName(ts); got != TailscaleStateSecretName(ts) {
		t.Errorf("tailscale state secret = %q, want %q", got, TailscaleStateSecretName(ts))
	}

	nb := newNetBirdInstance("mesh-state-nb")
	if got := MeshStateSecretName(nb); got != "" {
		t.Errorf("netbird should need no state Secret, got %q", got)
	}
}

func TestMeshCredentialSecretName(t *testing.T) {
	ts := newTestInstance("mesh-cred-ts")
	ts.Spec.Tailscale.Enabled = true
	ts.Spec.Tailscale.AuthKeySecretRef = &corev1.LocalObjectReference{Name: "ts-authkey"}
	if got := MeshCredentialSecretName(ts); got != "ts-authkey" {
		t.Errorf("tailscale credential = %q, want ts-authkey", got)
	}

	nb := newNetBirdInstance("mesh-cred-nb")
	if got := MeshCredentialSecretName(nb); got != "netbird-setup-key" {
		t.Errorf("netbird credential = %q, want netbird-setup-key", got)
	}

	// No credential configured is reported as absent, not as a stale name.
	bare := newTestInstance("mesh-cred-bare")
	bare.Spec.Tailscale.Enabled = true
	if got := MeshCredentialSecretName(bare); got != "" {
		t.Errorf("expected no credential, got %q", got)
	}
}

// NetBird provider specifics.

func TestNetBirdSidecar_UsesUserspaceMode(t *testing.T) {
	instance := newNetBirdInstance("nb-userspace")
	containers := netbirdProvider{}.SidecarContainers(instance)
	if len(containers) != 1 {
		t.Fatalf("expected 1 sidecar, got %d", len(containers))
	}
	c := containers[0]

	if c.Name != "netbird" {
		t.Errorf("container name = %q, want netbird", c.Name)
	}

	env := envMap(c.Env)
	// Netstack mode is what keeps the sidecar inside the Restricted PSS
	// defaults -- kernel mode would need NET_ADMIN and /dev/net/tun.
	if env["NB_USE_NETSTACK_MODE"] != "true" {
		t.Errorf("NB_USE_NETSTACK_MODE = %q, want true", env["NB_USE_NETSTACK_MODE"])
	}
	if env["NB_HOSTNAME"] != instance.Name {
		t.Errorf("NB_HOSTNAME = %q, want %q", env["NB_HOSTNAME"], instance.Name)
	}

	// Same hardened posture as every other container the operator builds.
	if c.SecurityContext == nil {
		t.Fatal("expected a security context")
	}
	if c.SecurityContext.Capabilities == nil || len(c.SecurityContext.Capabilities.Drop) != 1 ||
		c.SecurityContext.Capabilities.Drop[0] != "ALL" {
		t.Errorf("expected all capabilities dropped, got %v", c.SecurityContext.Capabilities)
	}
	if c.SecurityContext.ReadOnlyRootFilesystem == nil || !*c.SecurityContext.ReadOnlyRootFilesystem {
		t.Error("expected a read-only root filesystem")
	}
	if c.SecurityContext.AllowPrivilegeEscalation == nil || *c.SecurityContext.AllowPrivilegeEscalation {
		t.Error("expected privilege escalation to be disallowed")
	}
}

func TestNetBirdSidecar_InjectsSetupKeyFromSecret(t *testing.T) {
	instance := newNetBirdInstance("nb-setupkey")
	c := netbirdProvider{}.SidecarContainers(instance)[0]

	for _, e := range c.Env {
		if e.Name != "NB_SETUP_KEY" {
			continue
		}
		if e.ValueFrom == nil || e.ValueFrom.SecretKeyRef == nil {
			t.Fatal("NB_SETUP_KEY should come from a Secret reference")
		}
		if e.ValueFrom.SecretKeyRef.Name != "netbird-setup-key" {
			t.Errorf("secret name = %q, want netbird-setup-key", e.ValueFrom.SecretKeyRef.Name)
		}
		if e.ValueFrom.SecretKeyRef.Key != DefaultNetBirdSetupKeySecretKey {
			t.Errorf("secret key = %q, want %q", e.ValueFrom.SecretKeyRef.Key, DefaultNetBirdSetupKeySecretKey)
		}
		// The key must never be inlined as a literal value.
		if e.Value != "" {
			t.Errorf("NB_SETUP_KEY must not carry a literal value, got %q", e.Value)
		}
		return
	}
	t.Error("NB_SETUP_KEY not found in the sidecar env")
}

func TestNetBirdSidecar_CustomSetupKeyKey(t *testing.T) {
	instance := newNetBirdInstance("nb-custom-key")
	instance.Spec.NetBird.SetupKeySecretKey = "my-key"
	c := netbirdProvider{}.SidecarContainers(instance)[0]

	for _, e := range c.Env {
		if e.Name == "NB_SETUP_KEY" && e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
			if e.ValueFrom.SecretKeyRef.Key != "my-key" {
				t.Errorf("secret key = %q, want my-key", e.ValueFrom.SecretKeyRef.Key)
			}
			return
		}
	}
	t.Error("NB_SETUP_KEY not found")
}

// A self-hosted control plane is the reason to pick NetBird over Tailscale, so
// the management URL has to reach the sidecar.
func TestNetBirdSidecar_SelfHostedManagementURL(t *testing.T) {
	instance := newNetBirdInstance("nb-selfhosted")
	instance.Spec.NetBird.ManagementURL = "https://netbird.example.com:33073"
	c := netbirdProvider{}.SidecarContainers(instance)[0]

	if got := envMap(c.Env)["NB_MANAGEMENT_URL"]; got != "https://netbird.example.com:33073" {
		t.Errorf("NB_MANAGEMENT_URL = %q, want the configured URL", got)
	}
}

func TestNetBirdSidecar_OmitsManagementURLWhenUnset(t *testing.T) {
	instance := newNetBirdInstance("nb-hosted")
	c := netbirdProvider{}.SidecarContainers(instance)[0]

	if _, ok := envMap(c.Env)["NB_MANAGEMENT_URL"]; ok {
		t.Error("NB_MANAGEMENT_URL should be omitted so the client uses its own default")
	}
}

func TestNetBirdSidecar_CustomHostname(t *testing.T) {
	instance := newNetBirdInstance("nb-hostname")
	instance.Spec.NetBird.Hostname = "custom-peer"

	if got := NetBirdHostname(instance); got != "custom-peer" {
		t.Errorf("hostname = %q, want custom-peer", got)
	}
}

func TestGetNetBirdImage(t *testing.T) {
	instance := newNetBirdInstance("nb-image")
	if got := GetNetBirdImage(instance); got != DefaultNetBirdImage+":latest" {
		t.Errorf("default image = %q, want %s:latest", got, DefaultNetBirdImage)
	}

	instance.Spec.NetBird.Image.Repository = "registry.example.com/netbird"
	instance.Spec.NetBird.Image.Tag = "0.30.0"
	if got := GetNetBirdImage(instance); got != "registry.example.com/netbird:0.30.0" {
		t.Errorf("image = %q, want registry.example.com/netbird:0.30.0", got)
	}

	// A digest pins the image and wins over the tag.
	instance.Spec.NetBird.Image.Digest = "sha256:abc123"
	if got := GetNetBirdImage(instance); got != "registry.example.com/netbird@sha256:abc123" {
		t.Errorf("image = %q, want the digest form", got)
	}
}

func TestNetBird_NoInitContainersOrMainMounts(t *testing.T) {
	instance := newNetBirdInstance("nb-minimal")
	p := netbirdProvider{}

	if got := p.InitContainers(instance); len(got) != 0 {
		t.Errorf("netbird needs no init container, got %d", len(got))
	}
	if got := p.MainContainerMounts(instance); len(got) != 0 {
		t.Errorf("netbird needs no main container mounts, got %d", len(got))
	}
	if got := p.MainContainerEnv(instance); len(got) != 0 {
		t.Errorf("netbird needs no main container env, got %d", len(got))
	}
	if got := p.BinPathPrefixes(instance); len(got) != 0 {
		t.Errorf("netbird stages no CLI, got %v", got)
	}
	if got := p.ConfigMapData(instance); len(got) != 0 {
		t.Errorf("netbird has no serve config, got %v", got)
	}
}

// EnrichConfig must be a no-op, not a mangler: NetBird has no SSO identity
// header equivalent to "tailscale whois".
func TestNetBird_EnrichConfigIsNoOp(t *testing.T) {
	instance := newNetBirdInstance("nb-config")
	in := []byte(`{"gateway":{"auth":{"mode":"token"}}}`)

	out, err := netbirdProvider{}.EnrichConfig(in, instance)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, in) {
		t.Errorf("config should be unchanged, got %s", out)
	}
}

// StatefulSet integration.

func TestBuildStatefulSet_NetBirdSidecar(t *testing.T) {
	instance := newNetBirdInstance("sts-netbird")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var found bool
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "netbird" {
			found = true
		}
		if c.Name == "tailscale" {
			t.Error("the tailscale sidecar must not appear when netbird is the provider")
		}
	}
	if !found {
		t.Error("netbird sidecar not found in the pod spec")
	}

	var hasStateVolume bool
	for _, v := range sts.Spec.Template.Spec.Volumes {
		if v.Name == "netbird-state" {
			hasStateVolume = true
		}
	}
	if !hasStateVolume {
		t.Error("netbird-state volume not found")
	}

	// No K8s API access is needed, so no token is mounted.
	if sts.Spec.Template.Spec.AutomountServiceAccountToken == nil ||
		*sts.Spec.Template.Spec.AutomountServiceAccountToken {
		t.Error("netbird must not cause the ServiceAccount token to be mounted")
	}
}

func TestBuildNetworkPolicy_NetBirdEgress(t *testing.T) {
	instance := newNetBirdInstance("np-netbird")
	np := BuildNetworkPolicy(instance)

	want := []int{NetBirdManagementPort, NetBirdSignalPort, NetBirdWireGuardPort, 3478}
	for _, port := range want {
		var found bool
		for _, rule := range np.Spec.Egress {
			for _, p := range rule.Ports {
				if p.Port != nil && p.Port.IntValue() == port {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("NetworkPolicy should allow egress to port %d", port)
		}
	}
}

// Tailscale's egress must survive the refactor unchanged.
func TestBuildNetworkPolicy_TailscaleEgressUnchanged(t *testing.T) {
	instance := newTestInstance("np-ts-egress")
	instance.Spec.Tailscale.Enabled = true
	np := BuildNetworkPolicy(instance)

	for _, port := range []int{6443, 3478, 41641} {
		var found bool
		for _, rule := range np.Spec.Egress {
			for _, p := range rule.Ports {
				if p.Port != nil && p.Port.IntValue() == port {
					found = true
				}
			}
		}
		if !found {
			t.Errorf("tailscale egress should still allow port %d", port)
		}
	}
}

func TestBuildRole_NetBirdNeedsNoStateSecretRule(t *testing.T) {
	instance := newNetBirdInstance("role-netbird")
	role := BuildRole(instance)

	for _, rule := range role.Rules {
		for _, name := range rule.ResourceNames {
			if name == TailscaleStateSecretName(instance) {
				t.Error("netbird must not be granted access to a tailscale state Secret")
			}
		}
	}
}

// envMap flattens a container's env for lookups, ignoring valueFrom entries.
func envMap(env []corev1.EnvVar) map[string]string {
	out := make(map[string]string, len(env))
	for _, e := range env {
		if e.ValueFrom == nil {
			out[e.Name] = e.Value
		}
	}
	return out
}
