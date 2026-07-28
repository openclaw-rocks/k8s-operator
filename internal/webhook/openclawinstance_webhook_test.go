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

package webhook

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// ptr returns a pointer to the given value.
func ptr[T any](v T) *T {
	return &v
}

// newTestInstance returns a well-configured OpenClawInstance that passes
// validation with zero warnings and zero errors. Individual tests mutate
// this baseline to trigger specific validation paths.
func newTestInstance() *openclawv1alpha1.OpenClawInstance {
	return &openclawv1alpha1.OpenClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
		Spec: openclawv1alpha1.OpenClawInstanceSpec{
			EnvFrom: []corev1.EnvFromSource{
				{SecretRef: &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: "test-secret"},
				}},
			},
			Resources: openclawv1alpha1.ResourcesSpec{
				Limits: openclawv1alpha1.ResourceList{
					CPU:    "2",
					Memory: "4Gi",
				},
			},
		},
	}
}

// containsWarning returns true if any warning message contains the substring.
func containsWarning(warnings []string, substring string) bool {
	for _, w := range warnings {
		if strings.Contains(w, substring) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// ValidateCreate tests
// ---------------------------------------------------------------------------

func TestValidateCreate_ValidInstance(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error for a valid instance, got: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected 0 warnings for a valid instance, got %d: %v", len(warnings), warnings)
	}
}

func TestValidateCreate_BlocksRootUser(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Security.PodSecurityContext = &openclawv1alpha1.PodSecurityContextSpec{
		RunAsUser: ptr(int64(0)),
	}

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error when RunAsUser=0, got nil")
	}
	if !strings.Contains(err.Error(), "root") && !strings.Contains(err.Error(), "UID 0") {
		t.Fatalf("error message should mention root or UID 0, got: %v", err)
	}
	// When an error is returned, warnings should be nil.
	if warnings != nil {
		t.Fatalf("expected nil warnings on hard error, got: %v", warnings)
	}
}

func TestValidateCreate_AllowsNonRootUser(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Security.PodSecurityContext = &openclawv1alpha1.PodSecurityContextSpec{
		RunAsUser: ptr(int64(1000)),
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error for RunAsUser=1000, got: %v", err)
	}
}

func TestValidateCreate_WarnsRunAsNonRootFalse(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Security.PodSecurityContext = &openclawv1alpha1.PodSecurityContextSpec{
		RunAsNonRoot: ptr(false),
	}

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "runAsNonRoot") {
		t.Fatalf("expected warning about runAsNonRoot, got: %v", warnings)
	}
}

func TestValidateCreate_NoWarnRunAsNonRootTrue(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Security.PodSecurityContext = &openclawv1alpha1.PodSecurityContextSpec{
		RunAsNonRoot: ptr(true),
	}

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "runAsNonRoot") {
		t.Fatalf("expected no runAsNonRoot warning when set to true, got: %v", warnings)
	}
}

func TestValidateCreate_WarnsNetworkPolicyDisabled(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Security.NetworkPolicy.Enabled = ptr(false)

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "NetworkPolicy") {
		t.Fatalf("expected warning about NetworkPolicy, got: %v", warnings)
	}
}

func TestValidateCreate_NoWarnNetworkPolicyEnabled(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Security.NetworkPolicy.Enabled = ptr(true)

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "NetworkPolicy") {
		t.Fatalf("expected no NetworkPolicy warning when enabled, got: %v", warnings)
	}
}

func TestValidateCreate_WarnsIngressWithoutTLS(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Networking.Ingress.Enabled = true
	// TLS is empty by default.

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "TLS") {
		t.Fatalf("expected warning about TLS, got: %v", warnings)
	}
}

func TestValidateCreate_NoWarnIngressWithTLS(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Networking.Ingress.Enabled = true
	instance.Spec.Networking.Ingress.TLS = []openclawv1alpha1.IngressTLS{
		{Hosts: []string{"example.com"}, SecretName: "tls-secret"},
	}

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "TLS") {
		t.Fatalf("expected no TLS warning when TLS is configured, got: %v", warnings)
	}
}

func TestValidateCreate_NoWarnIngressDisabled(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	// Ingress.Enabled defaults to false. Even without TLS, no warning.

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "TLS") {
		t.Fatalf("expected no TLS warning when Ingress is disabled, got: %v", warnings)
	}
}

func TestValidateCreate_WarnsIngressForceHTTPSDisabled(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Networking.Ingress.Enabled = true
	instance.Spec.Networking.Ingress.TLS = []openclawv1alpha1.IngressTLS{
		{Hosts: []string{"example.com"}, SecretName: "tls-secret"},
	}
	instance.Spec.Networking.Ingress.Security.ForceHTTPS = ptr(false)

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "forceHTTPS") {
		t.Fatalf("expected warning about forceHTTPS, got: %v", warnings)
	}
}

func TestValidateCreate_NoWarnForceHTTPSWhenIngressDisabled(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	// Ingress disabled; forceHTTPS=false should NOT trigger a warning.
	instance.Spec.Networking.Ingress.Security.ForceHTTPS = ptr(false)

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "forceHTTPS") {
		t.Fatalf("expected no forceHTTPS warning when Ingress is disabled, got: %v", warnings)
	}
}

func TestValidateCreate_WarnsChromiumWithoutDigest(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Chromium.Image.Digest = "" // no digest

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "Chromium") || !containsWarning(warnings, "digest") {
		t.Fatalf("expected warning about Chromium digest pinning, got: %v", warnings)
	}
}

func TestValidateCreate_NoWarnChromiumWithDigest(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Chromium.Image.Digest = "sha256:abc123"

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "Chromium") {
		t.Fatalf("expected no Chromium warning when digest is set, got: %v", warnings)
	}
}

func TestValidateCreate_NoWarnChromiumDisabled(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	// Chromium.Enabled defaults to false.

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "Chromium") {
		t.Fatalf("expected no Chromium warning when disabled, got: %v", warnings)
	}
}

func TestValidateCreate_WarnsNoEnvVars(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.EnvFrom = nil
	instance.Spec.Env = nil

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "No AI provider API keys") {
		t.Fatalf("expected warning about missing provider keys, got: %v", warnings)
	}
}

func TestValidateCreate_NoWarnWithEnvFrom(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	// newTestInstance already has EnvFrom configured.

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "No AI provider API keys") {
		t.Fatalf("expected no provider warning when envFrom is set, got: %v", warnings)
	}
}

func TestValidateCreate_NoWarnWithEnv(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.EnvFrom = nil
	instance.Spec.Env = []corev1.EnvVar{
		{Name: "ANTHROPIC_API_KEY", Value: "sk-test"},
	}

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "No AI provider API keys") {
		t.Fatalf("expected no provider warning when known env key is set, got: %v", warnings)
	}
}

func TestValidateCreate_WarnsPrivilegeEscalation(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Security.ContainerSecurityContext = &openclawv1alpha1.ContainerSecurityContextSpec{
		AllowPrivilegeEscalation: ptr(true),
	}

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "allowPrivilegeEscalation") {
		t.Fatalf("expected warning about allowPrivilegeEscalation, got: %v", warnings)
	}
}

func TestValidateCreate_NoWarnPrivilegeEscalationFalse(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Security.ContainerSecurityContext = &openclawv1alpha1.ContainerSecurityContextSpec{
		AllowPrivilegeEscalation: ptr(false),
	}

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "allowPrivilegeEscalation") {
		t.Fatalf("expected no privilege escalation warning when false, got: %v", warnings)
	}
}

func TestValidateCreate_RejectsNoResourceLimits(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Resources.Limits = openclawv1alpha1.ResourceList{} // empty

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for missing resource limits, got nil")
	}
	if !strings.Contains(err.Error(), "resource limits are required") {
		t.Fatalf("expected resource limits error, got: %v", err)
	}
}

func TestValidateCreate_RejectsPartialResourceLimits_MissingCPU(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Resources.Limits = openclawv1alpha1.ResourceList{
		Memory: "4Gi",
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for missing CPU limit, got nil")
	}
	if !strings.Contains(err.Error(), "resource limits are required") {
		t.Fatalf("expected resource limits error, got: %v", err)
	}
}

func TestValidateCreate_RejectsPartialResourceLimits_MissingMemory(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Resources.Limits = openclawv1alpha1.ResourceList{
		CPU: "2",
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for missing memory limit, got nil")
	}
	if !strings.Contains(err.Error(), "resource limits are required") {
		t.Fatalf("expected resource limits error, got: %v", err)
	}
}

func TestValidateCreate_WarnsLatestImageTag(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Image.Tag = "latest"

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "latest") {
		t.Fatalf("expected warning about latest tag, got: %v", warnings)
	}
}

func TestValidateCreate_NoWarnLatestTagWithDigest(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Image.Tag = "latest"
	instance.Spec.Image.Digest = "sha256:abc123"

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "latest") {
		t.Fatalf("expected no latest warning when digest is set, got: %v", warnings)
	}
}

func TestValidateCreate_NoWarnSpecificImageTag(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Image.Tag = "v1.2.3"

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "latest") {
		t.Fatalf("expected no latest warning for specific tag, got: %v", warnings)
	}
}

func TestValidateCreate_MultipleWarnings(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()

	// Trigger multiple warnings at once:
	// 1. runAsNonRoot=false
	instance.Spec.Security.PodSecurityContext = &openclawv1alpha1.PodSecurityContextSpec{
		RunAsNonRoot: ptr(false),
	}
	// 2. NetworkPolicy disabled
	instance.Spec.Security.NetworkPolicy.Enabled = ptr(false)
	// 3. Ingress without TLS
	instance.Spec.Networking.Ingress.Enabled = true
	// 4. forceHTTPS disabled
	instance.Spec.Networking.Ingress.Security.ForceHTTPS = ptr(false)
	// 5. Chromium without digest
	instance.Spec.Chromium.Enabled = true
	// 6. No env vars
	instance.Spec.EnvFrom = nil
	instance.Spec.Env = nil
	// 7. AllowPrivilegeEscalation=true
	instance.Spec.Security.ContainerSecurityContext = &openclawv1alpha1.ContainerSecurityContextSpec{
		AllowPrivilegeEscalation: ptr(true),
	}
	// 8. Resource limits set (required, not a warning)
	instance.Spec.Resources.Limits = openclawv1alpha1.ResourceList{
		CPU:    "2",
		Memory: "4Gi",
	}
	// 9. Latest image tag
	instance.Spec.Image.Tag = "latest"
	instance.Spec.Image.Digest = ""

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error (only warnings), got: %v", err)
	}

	expectedCount := 8
	if len(warnings) != expectedCount {
		t.Fatalf("expected %d warnings, got %d: %v", expectedCount, len(warnings), warnings)
	}

	// Verify each expected warning substring is present.
	expectedSubstrings := []string{
		"runAsNonRoot",
		"NetworkPolicy",
		"TLS",
		"forceHTTPS",
		"Chromium",
		"No AI provider API keys",
		"allowPrivilegeEscalation",
		"latest",
	}
	for _, sub := range expectedSubstrings {
		if !containsWarning(warnings, sub) {
			t.Errorf("expected a warning containing %q, but none found in: %v", sub, warnings)
		}
	}
}

// ---------------------------------------------------------------------------
// ValidateUpdate tests
// ---------------------------------------------------------------------------

func TestValidateUpdate_ImmutableStorageClass(t *testing.T) {
	v := &OpenClawInstanceValidator{}

	oldInstance := newTestInstance()
	oldInstance.Spec.Storage.Persistence.StorageClass = ptr("standard")

	newInstance := newTestInstance()
	newInstance.Spec.Storage.Persistence.StorageClass = ptr("premium-ssd")

	warnings, err := v.ValidateUpdate(context.Background(), oldInstance, newInstance)
	if err == nil {
		t.Fatal("expected error when changing storage class, got nil")
	}
	if !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("error should mention immutability, got: %v", err)
	}
	if warnings != nil {
		t.Fatalf("expected nil warnings on hard error, got: %v", warnings)
	}
}

func TestValidateUpdate_AllowsSameStorageClass(t *testing.T) {
	v := &OpenClawInstanceValidator{}

	oldInstance := newTestInstance()
	oldInstance.Spec.Storage.Persistence.StorageClass = ptr("standard")

	newInstance := newTestInstance()
	newInstance.Spec.Storage.Persistence.StorageClass = ptr("standard")

	warnings, err := v.ValidateUpdate(context.Background(), oldInstance, newInstance)
	if err != nil {
		t.Fatalf("expected no error when storage class is unchanged, got: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected 0 warnings, got %d: %v", len(warnings), warnings)
	}
}

func TestValidateUpdate_AllowsStorageClassWhenOldIsNil(t *testing.T) {
	v := &OpenClawInstanceValidator{}

	oldInstance := newTestInstance()
	// StorageClass is nil by default.

	newInstance := newTestInstance()
	newInstance.Spec.Storage.Persistence.StorageClass = ptr("standard")

	_, err := v.ValidateUpdate(context.Background(), oldInstance, newInstance)
	if err != nil {
		t.Fatalf("expected no error when old storage class is nil, got: %v", err)
	}
}

func TestValidateUpdate_AllowsStorageClassWhenNewIsNil(t *testing.T) {
	v := &OpenClawInstanceValidator{}

	oldInstance := newTestInstance()
	oldInstance.Spec.Storage.Persistence.StorageClass = ptr("standard")

	newInstance := newTestInstance()
	// StorageClass is nil by default.

	_, err := v.ValidateUpdate(context.Background(), oldInstance, newInstance)
	if err != nil {
		t.Fatalf("expected no error when new storage class is nil, got: %v", err)
	}
}

func TestValidateUpdate_AllowsOtherChanges(t *testing.T) {
	v := &OpenClawInstanceValidator{}

	oldInstance := newTestInstance()
	oldInstance.Spec.Image.Tag = "v1.0.0"

	newInstance := newTestInstance()
	newInstance.Spec.Image.Tag = "v2.0.0"

	warnings, err := v.ValidateUpdate(context.Background(), oldInstance, newInstance)
	if err != nil {
		t.Fatalf("expected no error when changing image tag, got: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected 0 warnings, got %d: %v", len(warnings), warnings)
	}
}

func TestValidateUpdate_RunsValidationAfterImmutabilityCheck(t *testing.T) {
	v := &OpenClawInstanceValidator{}

	oldInstance := newTestInstance()
	newInstance := newTestInstance()
	// Trigger a validation warning (root user would be blocked).
	newInstance.Spec.Security.PodSecurityContext = &openclawv1alpha1.PodSecurityContextSpec{
		RunAsUser: ptr(int64(0)),
	}

	_, err := v.ValidateUpdate(context.Background(), oldInstance, newInstance)
	if err == nil {
		t.Fatal("expected error for RunAsUser=0 in update, got nil")
	}
	if !strings.Contains(err.Error(), "root") && !strings.Contains(err.Error(), "UID 0") {
		t.Fatalf("error should mention root/UID 0, got: %v", err)
	}
}

func TestValidateUpdate_ReturnsWarningsFromValidation(t *testing.T) {
	v := &OpenClawInstanceValidator{}

	oldInstance := newTestInstance()
	newInstance := newTestInstance()
	// Remove env vars to trigger a warning during update validation.
	newInstance.Spec.EnvFrom = nil
	newInstance.Spec.Env = nil

	warnings, err := v.ValidateUpdate(context.Background(), oldInstance, newInstance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "No AI provider API keys") {
		t.Fatalf("expected warning about provider keys from update validation, got: %v", warnings)
	}
}

// ---------------------------------------------------------------------------
// ValidateDelete tests
// ---------------------------------------------------------------------------

func TestValidateDelete_AlwaysAllowed(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()

	warnings, err := v.ValidateDelete(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error on delete, got: %v", err)
	}
	if warnings != nil {
		t.Fatalf("expected nil warnings on delete, got: %v", warnings)
	}
}

func TestValidateDelete_AllowsEvenWithInvalidSpec(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	// Instance that would fail create/update validation.
	instance := newTestInstance()
	instance.Spec.Security.PodSecurityContext = &openclawv1alpha1.PodSecurityContextSpec{
		RunAsUser: ptr(int64(0)),
	}

	warnings, err := v.ValidateDelete(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error on delete even with invalid spec, got: %v", err)
	}
	if warnings != nil {
		t.Fatalf("expected nil warnings on delete, got: %v", warnings)
	}
}

// ---------------------------------------------------------------------------
// Edge case tests
// ---------------------------------------------------------------------------

func TestValidateCreate_NilPodSecurityContext(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Security.PodSecurityContext = nil

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error with nil PodSecurityContext, got: %v", err)
	}
}

func TestValidateCreate_NilContainerSecurityContext(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Security.ContainerSecurityContext = nil

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error with nil ContainerSecurityContext, got: %v", err)
	}
}

func TestValidateCreate_NilNetworkPolicyEnabled(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Security.NetworkPolicy.Enabled = nil

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error with nil NetworkPolicy.Enabled, got: %v", err)
	}
	if containsWarning(warnings, "NetworkPolicy") {
		t.Fatalf("expected no NetworkPolicy warning when Enabled is nil, got: %v", warnings)
	}
}

func TestValidateCreate_NilForceHTTPS(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Networking.Ingress.Enabled = true
	instance.Spec.Networking.Ingress.TLS = []openclawv1alpha1.IngressTLS{
		{Hosts: []string{"example.com"}, SecretName: "tls-secret"},
	}
	instance.Spec.Networking.Ingress.Security.ForceHTTPS = nil

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "forceHTTPS") {
		t.Fatalf("expected no forceHTTPS warning when nil, got: %v", warnings)
	}
}

func TestValidateCreate_NilRunAsUser(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Security.PodSecurityContext = &openclawv1alpha1.PodSecurityContextSpec{
		RunAsUser: nil,
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error with nil RunAsUser, got: %v", err)
	}
}

func TestValidateCreate_NilAllowPrivilegeEscalation(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Security.ContainerSecurityContext = &openclawv1alpha1.ContainerSecurityContextSpec{
		AllowPrivilegeEscalation: nil,
	}

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "allowPrivilegeEscalation") {
		t.Fatalf("expected no privilege escalation warning when nil, got: %v", warnings)
	}
}

// ---------------------------------------------------------------------------
// Workspace validation tests
// ---------------------------------------------------------------------------

func TestValidateCreate_ValidWorkspace(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{
			"SOUL.md":      "personality content",
			"AGENTS.md":    "agents config",
			"HEARTBEAT.md": "heartbeat config",
			"USER.md":      "user config",
		},
		InitialDirectories: []string{"memory", "tools/scripts"},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error for valid workspace, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Resource quantity validation tests
// ---------------------------------------------------------------------------

func TestValidateResourceQuantities(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*openclawv1alpha1.OpenClawInstance)
		wantErr bool
		errSub  string
	}{
		{
			name:    "Valid default instance",
			mutate:  func(i *openclawv1alpha1.OpenClawInstance) {},
			wantErr: false,
		},
		{
			name: "Valid custom quantities",
			mutate: func(i *openclawv1alpha1.OpenClawInstance) {
				i.Spec.Storage.Persistence.Size = "10Gi"
				i.Spec.Resources.Requests.CPU = "500m"
				i.Spec.Resources.Requests.Memory = "1Gi"
				i.Spec.Resources.Limits.CPU = "1"
				i.Spec.Resources.Limits.Memory = "2Gi"
				i.Spec.Chromium.Resources.Requests.CPU = "100m"
				i.Spec.Tailscale.Resources.Limits.Memory = "128Mi"
				i.Spec.Ollama.Resources.Requests.CPU = "2"
				i.Spec.Ollama.Storage.SizeLimit = "50Gi"
				i.Spec.WebTerminal.Resources.Requests.CPU = "100m"
				i.Spec.WebTerminal.Resources.Limits.Memory = "256Mi"
				i.Spec.Chromium.Persistence.Size = "5Gi"
			},
			wantErr: false,
		},
		{
			name: "Invalid Chromium persistence size",
			mutate: func(i *openclawv1alpha1.OpenClawInstance) {
				i.Spec.Chromium.Persistence.Size = "invalid"
			},
			wantErr: true,
			errSub:  "spec.chromium.persistence.size",
		},
		{
			name: "Invalid Ollama storage size limit",
			mutate: func(i *openclawv1alpha1.OpenClawInstance) {
				i.Spec.Ollama.Storage.SizeLimit = "invalid"
			},
			wantErr: true,
			errSub:  "spec.ollama.storage.sizeLimit",
		},
		{
			name: "Invalid Web terminal CPU request",
			mutate: func(i *openclawv1alpha1.OpenClawInstance) {
				i.Spec.WebTerminal.Resources.Requests.CPU = "abc"
			},
			wantErr: true,
			errSub:  "spec.webTerminal.resources.requests.cpu",
		},
		{
			name: "Invalid storage size",
			mutate: func(i *openclawv1alpha1.OpenClawInstance) {
				i.Spec.Storage.Persistence.Size = "invalid"
			},
			wantErr: true,
			errSub:  "spec.storage.persistence.size",
		},
		{
			name: "Invalid main resources CPU request",
			mutate: func(i *openclawv1alpha1.OpenClawInstance) {
				i.Spec.Resources.Requests.CPU = "abc"
			},
			wantErr: true,
			errSub:  "spec.resources.requests.cpu",
		},
		{
			name: "Invalid main resources Memory request",
			mutate: func(i *openclawv1alpha1.OpenClawInstance) {
				i.Spec.Resources.Requests.Memory = "100 invalid"
			},
			wantErr: true,
			errSub:  "spec.resources.requests.memory",
		},
		{
			name: "Invalid main resources CPU limit",
			mutate: func(i *openclawv1alpha1.OpenClawInstance) {
				i.Spec.Resources.Limits.CPU = "0.5.0"
			},
			wantErr: true,
			errSub:  "spec.resources.limits.cpu",
		},
		{
			name: "Invalid main resources Memory limit",
			mutate: func(i *openclawv1alpha1.OpenClawInstance) {
				i.Spec.Resources.Limits.Memory = "1024mB"
			},
			wantErr: true,
			errSub:  "spec.resources.limits.memory",
		},
		{
			name: "Invalid Chromium CPU request",
			mutate: func(i *openclawv1alpha1.OpenClawInstance) {
				i.Spec.Chromium.Resources.Requests.CPU = "1000x"
			},
			wantErr: true,
			errSub:  "spec.chromium.resources.requests.cpu",
		},
		{
			name: "Invalid Chromium Memory limit",
			mutate: func(i *openclawv1alpha1.OpenClawInstance) {
				i.Spec.Chromium.Resources.Limits.Memory = "512 gigabytes"
			},
			wantErr: true,
			errSub:  "spec.chromium.resources.limits.memory",
		},
		{
			name: "Invalid Tailscale CPU limit",
			mutate: func(i *openclawv1alpha1.OpenClawInstance) {
				i.Spec.Tailscale.Resources.Limits.CPU = "1.5i"
			},
			wantErr: true,
			errSub:  "spec.tailscale.resources.limits.cpu",
		},
		{
			name: "Invalid Ollama Memory request",
			mutate: func(i *openclawv1alpha1.OpenClawInstance) {
				i.Spec.Ollama.Resources.Requests.Memory = "8Giii"
			},
			wantErr: true,
			errSub:  "spec.ollama.resources.requests.memory",
		},
		{
			name: "Empty values are allowed",
			mutate: func(i *openclawv1alpha1.OpenClawInstance) {
				i.Spec.Storage.Persistence.Size = ""
				i.Spec.Resources.Requests.CPU = ""
				i.Spec.Resources.Limits.Memory = ""
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := newTestInstance()
			tt.mutate(instance)
			err := validateResourceQuantities(instance)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateResourceQuantities() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
				t.Errorf("validateResourceQuantities() error = %v, want error containing %q", err, tt.errSub)
			}
		})
	}
}

func TestValidateCreate_WorkspaceNil(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = nil

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error with nil workspace, got: %v", err)
	}
}

func TestValidateCreate_WorkspaceFileNestedPath(t *testing.T) {
	// Nested paths in initialFiles must be accepted; the operator encodes '/'
	// as '--' for ConfigMap storage and decodes it back in the init script (#482).
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{
			"agents/AGENT.md":         "# Agent",
			"skills/redmine/SKILL.md": "# Skill",
		},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected nested initialFiles paths to be accepted, got: %v", err)
	}
}

func TestValidateCreate_WorkspaceFileNestedRejected(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	cases := []struct {
		name string
		path string
	}{
		{"absolute", "/etc/passwd"},
		{"trailing slash", "agents/"},
		{"empty segment", "agents//AGENT.md"},
		{"dotdot segment", "agents/../escape.md"},
		{"hidden middle segment", "agents/.secret/AGENT.md"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			instance := newTestInstance()
			instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
				InitialFiles: map[string]string{tc.path: "content"},
			}
			if _, err := v.ValidateCreate(context.Background(), instance); err == nil {
				t.Fatalf("expected error for path %q", tc.path)
			}
		})
	}
}

func TestValidateCreate_WorkspaceFileBackslash(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{"file\\name.md": "content"},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for filename with '\\'")
	}
}

func TestValidateCreate_WorkspaceFileDotDot(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{"..bad": "content"},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for filename with '..'")
	}
}

func TestValidateCreate_WorkspaceFileDotPrefix(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{".hidden": "content"},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for filename starting with '.'")
	}
}

func TestValidateCreate_WorkspaceFileReservedName(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{"openclaw.json": "content"},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for reserved filename 'openclaw.json'")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("error should mention reserved, got: %v", err)
	}
}

func TestValidateCreate_WorkspaceDirDotDot(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialDirectories: []string{"../escape"},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for directory with '..'")
	}
}

func TestValidateCreate_WorkspaceDirBackslash(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialDirectories: []string{"dir\\sub"},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for directory with '\\'")
	}
}

func TestValidateCreate_WorkspaceDirAbsolutePath(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialDirectories: []string{"/etc/shadow"},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for absolute path directory")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("error should mention absolute, got: %v", err)
	}
}

func TestValidateCreate_WorkspaceNestedDirAllowed(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialDirectories: []string{"tools/scripts", "memory"},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error for nested directories, got: %v", err)
	}
}

func TestValidateCreate_WorkspaceConfigMapRefValid(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		ConfigMapRef: &openclawv1alpha1.ConfigMapNameSelector{
			Name: "my-workspace-cm",
		},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error for valid configMapRef, got: %v", err)
	}
}

func TestValidateCreate_WorkspaceConfigMapRefEmptyName(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		ConfigMapRef: &openclawv1alpha1.ConfigMapNameSelector{
			Name: "",
		},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for empty configMapRef.name")
	}
	if !strings.Contains(err.Error(), "configMapRef.name must not be empty") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// ---------------------------------------------------------------------------
// CA bundle validation tests
// ---------------------------------------------------------------------------

func TestValidateCreate_CABundle_BothSources(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Security.CABundle = &openclawv1alpha1.CABundleSpec{
		ConfigMapName: "my-cm",
		SecretName:    "my-secret",
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error when both configMapName and secretName are set")
	}
	if !strings.Contains(err.Error(), "only one") {
		t.Fatalf("error should mention 'only one', got: %v", err)
	}
}

func TestValidateCreate_CABundle_NoSource(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Security.CABundle = &openclawv1alpha1.CABundleSpec{}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error when neither source is set")
	}
	if !strings.Contains(err.Error(), "must be set") {
		t.Fatalf("error should mention 'must be set', got: %v", err)
	}
}

func TestValidateCreate_CABundle_ConfigMapOnly(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Security.CABundle = &openclawv1alpha1.CABundleSpec{
		ConfigMapName: "my-ca",
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error with configMapName only, got: %v", err)
	}
}

func TestValidateCreate_CABundle_SecretOnly(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Security.CABundle = &openclawv1alpha1.CABundleSpec{
		SecretName: "my-ca-secret",
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error with secretName only, got: %v", err)
	}
}

func TestValidateCreate_CABundle_Nil(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	// CABundle is nil by default

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error with nil CABundle, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Provider warnings tests
// ---------------------------------------------------------------------------

func TestValidateCreate_ProviderWarning_EnvFromSet(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	// newTestInstance has EnvFrom set — should skip provider check

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "No AI provider") {
		t.Fatalf("expected no provider warning when envFrom is set, got: %v", warnings)
	}
}

func TestValidateCreate_ProviderWarning_KnownKeyInEnv(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.EnvFrom = nil
	instance.Spec.Env = []corev1.EnvVar{
		{Name: "ANTHROPIC_API_KEY", Value: "sk-test"},
	}

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "No AI provider") {
		t.Fatalf("expected no provider warning when ANTHROPIC_API_KEY is set, got: %v", warnings)
	}
}

func TestValidateCreate_ProviderWarning_OnlyUnrelatedEnv(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.EnvFrom = nil
	instance.Spec.Env = []corev1.EnvVar{
		{Name: "HOME", Value: "/foo"},
	}

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "No AI provider") {
		t.Fatalf("expected provider warning when only unrelated env vars are set, got: %v", warnings)
	}
}

func TestValidateCreate_ProviderWarning_SecretKeyRef(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.EnvFrom = nil
	instance.Spec.Env = []corev1.EnvVar{
		{
			Name: "MY_KEY",
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "my-secret"},
					Key:                  "api-key",
				},
			},
		},
	}

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "No AI provider") {
		t.Fatalf("expected no provider warning when valueFrom secretKeyRef is set, got: %v", warnings)
	}
}

func TestValidateCreate_ProviderWarning_EmptyEnvAndEnvFrom(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.EnvFrom = nil
	instance.Spec.Env = nil

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "No AI provider") {
		t.Fatalf("expected provider warning with empty env and envFrom, got: %v", warnings)
	}
}

// ---------------------------------------------------------------------------
// Config schema validation tests
// ---------------------------------------------------------------------------

func TestValidateCreate_ConfigSchema_Nil(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	// No raw config

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "Unknown config key") {
		t.Fatalf("expected no config schema warning with nil raw, got: %v", warnings)
	}
}

func TestValidateCreate_ConfigSchema_ValidKeys(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: k8sruntime.RawExtension{
			Raw: []byte(`{"mcpServers":{},"settings":{},"apiKeys":{}}`),
		},
	}

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "Unknown config key") {
		t.Fatalf("expected no config schema warning for valid keys, got: %v", warnings)
	}
}

func TestValidateCreate_ConfigSchema_UnknownKey(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: k8sruntime.RawExtension{
			Raw: []byte(`{"mcpServers":{},"foobar":"baz"}`),
		},
	}

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "Unknown config key") || !containsWarning(warnings, "foobar") {
		t.Fatalf("expected warning about unknown key 'foobar', got: %v", warnings)
	}
}

// ---------------------------------------------------------------------------
// Custom init container validation tests
// ---------------------------------------------------------------------------

func TestValidateCreate_InitContainers_Valid(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.InitContainers = []corev1.Container{
		{Name: "my-init", Image: "busybox:1.37"},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error for valid init container name, got: %v", err)
	}
}

func TestValidateCreate_InitContainers_ReservedName_InitConfig(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.InitContainers = []corev1.Container{
		{Name: "init-config", Image: "busybox:1.37"},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for reserved init container name 'init-config'")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("error should mention reserved, got: %v", err)
	}
}

func TestValidateCreate_InitContainers_ReservedName_InitSkills(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.InitContainers = []corev1.Container{
		{Name: "init-skills", Image: "busybox:1.37"},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for reserved init container name 'init-skills'")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("error should mention reserved, got: %v", err)
	}
}

func TestValidateCreate_InitContainers_ReservedName_InitPnpm(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.InitContainers = []corev1.Container{
		{Name: "init-pnpm", Image: "busybox:1.37"},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for reserved init container name 'init-pnpm'")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("error should mention reserved, got: %v", err)
	}
}

func TestValidateCreate_InitContainers_ReservedName_InitPython(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.InitContainers = []corev1.Container{
		{Name: "init-python", Image: "busybox:1.37"},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for reserved init container name 'init-python'")
	}
	if !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("error should mention reserved, got: %v", err)
	}
}

func TestValidateCreate_InitContainers_EmptyName(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.InitContainers = []corev1.Container{
		{Name: "", Image: "busybox:1.37"},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for empty init container name")
	}
	if !strings.Contains(err.Error(), "empty") {
		t.Fatalf("error should mention empty, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// JSON5 config validation tests
// ---------------------------------------------------------------------------

func TestValidateCreate_JSON5_WithRaw(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Config.Format = "json5"
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: k8sruntime.RawExtension{Raw: []byte(`{}`)},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for json5 with raw config")
	}
	if !strings.Contains(err.Error(), "json5") || !strings.Contains(err.Error(), "configMapRef") {
		t.Fatalf("error should mention json5 and configMapRef, got: %v", err)
	}
}

func TestValidateCreate_JSON5_WithMerge(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Config.Format = "json5"
	instance.Spec.Config.MergeMode = "merge"
	instance.Spec.Config.ConfigMapRef = &openclawv1alpha1.ConfigMapKeySelector{
		Name: "my-config",
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for json5 with merge mode")
	}
	if !strings.Contains(err.Error(), "json5") || !strings.Contains(err.Error(), "merge") {
		t.Fatalf("error should mention json5 and merge, got: %v", err)
	}
}

func TestValidateSkillName_NpmPrefix(t *testing.T) {
	// Valid npm-prefixed skill should pass
	if err := validateSkillName("npm:@openclaw/matrix"); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateSkillName_NpmPrefixUnscoped(t *testing.T) {
	if err := validateSkillName("npm:some-package"); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateSkillName_NpmPrefixEmpty(t *testing.T) {
	err := validateSkillName("npm:")
	if err == nil {
		t.Fatal("expected error for bare npm: prefix")
	}
	if !strings.Contains(err.Error(), "requires a package name") {
		t.Fatalf("error should mention package name, got: %v", err)
	}
}

func TestValidateCreate_NpmPrefixedSkillAccepted(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Skills = []string{"npm:@openclaw/matrix", "@anthropic/mcp-server-fetch"}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateCreate_BareColonSkillRejected(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Skills = []string{"foo:bar"}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for colon in skill name without known prefix")
	}
	if !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("error should mention invalid character, got: %v", err)
	}
}

func TestValidateCreate_JSON5_WithConfigMapRef(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Config.Format = "json5"
	instance.Spec.Config.ConfigMapRef = &openclawv1alpha1.ConfigMapKeySelector{
		Name: "my-config",
		Key:  "config.json5",
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error for json5 with configMapRef, got: %v", err)
	}
}

func TestValidateCreate_JSON5_SkipsConfigSchemaValidation(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Config.Format = "json5"
	instance.Spec.Config.ConfigMapRef = &openclawv1alpha1.ConfigMapKeySelector{
		Name: "my-config",
	}
	// No raw config — schema validation should be skipped entirely for json5

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "Unknown config key") {
		t.Fatalf("should not warn about config keys when format is json5, got: %v", warnings)
	}
}

// ---------------------------------------------------------------------------
// forcePaths validation tests
// ---------------------------------------------------------------------------

func TestValidateCreate_ForcePaths_ValidWithMerge(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Config.MergeMode = "merge"
	instance.Spec.Config.ForcePaths = []string{
		"gateway",
		"models.providers",
		"agents.defaults.sandbox",
		"agents.defaults.model.primary",
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error for valid forcePaths under mergeMode=merge, got: %v", err)
	}
}

func TestValidateCreate_ForcePaths_EmptyAllowed(t *testing.T) {
	// An empty / unset forcePaths must not trip the mergeMode check --
	// otherwise the field becomes effectively mandatory under overwrite,
	// which would be a breaking change for every existing CR.
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Config.MergeMode = "overwrite"
	instance.Spec.Config.ForcePaths = nil

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error for empty forcePaths under overwrite mode, got: %v", err)
	}

	instance.Spec.Config.ForcePaths = []string{}
	if _, err := v.ValidateCreate(context.Background(), instance); err != nil {
		t.Fatalf("expected no error for zero-length forcePaths slice under overwrite mode, got: %v", err)
	}
}

func TestValidateCreate_ForcePaths_RequiresMerge(t *testing.T) {
	// Under overwrite mode the whole file is rebuilt every restart, so
	// forcePaths is meaningless. Reject rather than silently ignore -- the
	// user almost certainly didn't mean to set it.
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Config.MergeMode = "overwrite"
	instance.Spec.Config.ForcePaths = []string{"gateway"}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for forcePaths under mergeMode=overwrite")
	}
	if !strings.Contains(err.Error(), "forcePaths") || !strings.Contains(err.Error(), "merge") {
		t.Fatalf("error should mention forcePaths and merge, got: %v", err)
	}
}

func TestValidateCreate_ForcePaths_RequiresMergeMode_DefaultedOverwrite(t *testing.T) {
	// The defaulter normally fills MergeMode="overwrite", but ValidateCreate
	// is exercised directly in unit tests so the field may be empty. Treat
	// an empty mergeMode as "not merge" -- the defaulter's contract is that
	// empty means overwrite.
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Config.MergeMode = ""
	instance.Spec.Config.ForcePaths = []string{"gateway"}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for forcePaths with empty mergeMode (treated as overwrite)")
	}
	if !strings.Contains(err.Error(), "forcePaths") {
		t.Fatalf("error should mention forcePaths, got: %v", err)
	}
}

func TestValidateCreate_ForcePaths_InvalidEntries(t *testing.T) {
	// Table of malformed paths. Every entry must be rejected with an error
	// that mentions both the field path ("forcePaths") and a hint about the
	// specific violation.
	cases := []struct {
		name   string
		path   string
		errSub string
	}{
		{"empty string", "", "empty"},
		{"leading dot", ".gateway", "start or end"},
		{"trailing dot", "gateway.", "start or end"},
		{"double dot", "models..providers", "empty segments"},
		{"triple dot", "a...b", "empty segments"},
		{"contains space", "gateway providers", "invalid character"}, // space is not in [a-zA-Z0-9._-]
		{"contains slash", "gateway/providers", "invalid character"},
		{"contains backslash", `gateway\providers`, "invalid character"},
		{"contains bracket", "gateway[0]", "invalid character"},
		{"contains quote", `gateway."x"`, "invalid character"},
		{"contains dollar", "gateway.$x", "invalid character"},
	}

	v := &OpenClawInstanceValidator{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			instance := newTestInstance()
			instance.Spec.Config.MergeMode = "merge"
			instance.Spec.Config.ForcePaths = []string{tc.path}

			_, err := v.ValidateCreate(context.Background(), instance)
			if err == nil {
				t.Fatalf("expected error for path %q, got nil", tc.path)
			}
			if !strings.Contains(err.Error(), "forcePaths") {
				t.Errorf("error should mention forcePaths, got: %v", err)
			}
			if tc.errSub != "" && !strings.Contains(err.Error(), tc.errSub) {
				t.Errorf("error for path %q should contain %q, got: %v", tc.path, tc.errSub, err)
			}
		})
	}
}

func TestValidateCreate_ForcePaths_ValidEntries(t *testing.T) {
	// Table of paths that must be accepted under mergeMode=merge.
	cases := []string{
		"gateway",
		"a",
		"a.b",
		"a.b.c.d.e.f.g.h.i.j", // arbitrary depth
		"models.providers",
		"models.providers.openai-compat", // dash
		"agents.defaults.model_primary",  // underscore
		"a1.b2.c3",                       // digits
		"UPPER.case",                     // uppercase
		"Mixed-Case_With.dots",
	}

	v := &OpenClawInstanceValidator{}
	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			instance := newTestInstance()
			instance.Spec.Config.MergeMode = "merge"
			instance.Spec.Config.ForcePaths = []string{p}

			_, err := v.ValidateCreate(context.Background(), instance)
			if err != nil {
				t.Fatalf("expected no error for path %q, got: %v", p, err)
			}
		})
	}
}

func TestValidateCreate_ForcePaths_ReportsBadEntryIndex(t *testing.T) {
	// When multiple paths are supplied, the error message should make it
	// obvious which one is bad. The first invalid entry wins.
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Config.MergeMode = "merge"
	instance.Spec.Config.ForcePaths = []string{
		"gateway",          // ok
		"models.providers", // ok
		"bad..path",        // bad -- index 2
		"agents",           // never reached
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for entry with empty segments")
	}
	if !strings.Contains(err.Error(), "[2]") {
		t.Errorf("error should identify the bad index [2], got: %v", err)
	}
	if !strings.Contains(err.Error(), "bad..path") {
		t.Errorf("error should quote the offending path, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Web terminal validation tests
// ---------------------------------------------------------------------------

func TestValidateCreate_WarnsWebTerminalWithoutDigest(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.WebTerminal.Enabled = true
	instance.Spec.WebTerminal.Image.Digest = "" // no digest

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if !containsWarning(warnings, "Web terminal") || !containsWarning(warnings, "digest") {
		t.Fatalf("expected warning about web terminal digest pinning, got: %v", warnings)
	}
}

func TestValidateCreate_NoWarnWebTerminalWithDigest(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.WebTerminal.Enabled = true
	instance.Spec.WebTerminal.Image.Digest = "sha256:abc123"

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "Web terminal") {
		t.Fatalf("expected no web terminal warning when digest is set, got: %v", warnings)
	}
}

func TestValidateCreate_NoWarnWebTerminalDisabled(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	// WebTerminal.Enabled defaults to false.

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if containsWarning(warnings, "Web terminal") {
		t.Fatalf("expected no web terminal warning when disabled, got: %v", warnings)
	}
}

// ---------------------------------------------------------------------------
// Additional workspaces validation tests
// ---------------------------------------------------------------------------

func TestValidateCreate_AdditionalWorkspaceValid(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{
				Name: "work",
				ConfigMapRef: &openclawv1alpha1.ConfigMapNameSelector{
					Name: "work-files",
				},
				InitialFiles: map[string]string{
					"SOUL.md": "work soul",
				},
				InitialDirectories: []string{"tools"},
			},
			{
				Name: "research",
				InitialFiles: map[string]string{
					"AGENT.md": "research agent",
				},
			},
		},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error for valid additional workspaces, got: %v", err)
	}
}

func TestValidateCreate_AdditionalWorkspaceDuplicateName(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{Name: "work"},
			{Name: "work"},
		},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for duplicate additional workspace names")
	}
	if !strings.Contains(err.Error(), "duplicated") {
		t.Errorf("expected duplicate error, got: %v", err)
	}
}

func TestValidateCreate_AdditionalWorkspaceEmptyName(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{Name: ""},
		},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for empty additional workspace name")
	}
	if !strings.Contains(err.Error(), "must not be empty") {
		t.Errorf("expected empty name error, got: %v", err)
	}
}

func TestValidateCreate_AdditionalWorkspaceConsecutiveHyphens(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{Name: "my--agent"},
		},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for consecutive hyphens in additional workspace name")
	}
	if !strings.Contains(err.Error(), "no consecutive hyphens") {
		t.Errorf("expected consecutive hyphens error, got: %v", err)
	}
}

func TestValidateCreate_AdditionalWorkspaceInvalidFilename(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{
				Name: "work",
				InitialFiles: map[string]string{
					"../evil.md": "path traversal",
				},
			},
		},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for invalid filename in additional workspace")
	}
	if !strings.Contains(err.Error(), "initialFiles") {
		t.Errorf("expected initialFiles error, got: %v", err)
	}
}

func TestValidateCreate_AdditionalWorkspaceConfigMapRefEmptyName(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{
				Name: "work",
				ConfigMapRef: &openclawv1alpha1.ConfigMapNameSelector{
					Name: "",
				},
			},
		},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for empty configMapRef.name in additional workspace")
	}
	if !strings.Contains(err.Error(), "configMapRef.name must not be empty") {
		t.Errorf("expected configMapRef error, got: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Suspended + HPA mutual exclusivity tests
// ---------------------------------------------------------------------------

func TestValidateCreate_SuspendedWithHPA(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Suspended = true
	instance.Spec.Availability.AutoScaling = &openclawv1alpha1.AutoScalingSpec{
		Enabled: ptr(true),
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error when suspended and HPA are both enabled")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should mention mutual exclusivity, got: %s", err.Error())
	}
}

func TestValidateCreate_SuspendedWithoutHPA(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Suspended = true

	warnings, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Errorf("suspended without HPA should be valid, got: %v", err)
	}
	_ = warnings
}

func TestValidateUpdate_SuspendedWithHPA(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	oldInstance := newTestInstance()
	newInstance := newTestInstance()
	newInstance.Spec.Suspended = true
	newInstance.Spec.Availability.AutoScaling = &openclawv1alpha1.AutoScalingSpec{
		Enabled: ptr(true),
	}

	_, err := v.ValidateUpdate(context.Background(), oldInstance, newInstance)
	if err == nil {
		t.Fatal("expected error when suspended and HPA are both enabled on update")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error should mention mutual exclusivity, got: %s", err.Error())
	}
}

func TestValidateSkillName_PackPrefix(t *testing.T) {
	if err := validateSkillName("pack:acme/skills/example@v1.2.0"); err != nil {
		t.Fatalf("expected no error for pack: reference, got: %v", err)
	}
}

func TestValidateSkillName_PackPrefixEmpty(t *testing.T) {
	err := validateSkillName("pack:")
	if err == nil {
		t.Fatal("expected error for bare pack: prefix")
	}
	if !strings.Contains(err.Error(), "requires an owner/repo/path") {
		t.Fatalf("error should mention the expected reference format, got: %v", err)
	}
}

func TestValidateCreate_PackSkillAccepted(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Skills = []string{"pack:acme/skills/example@main"}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error for pack: skill, got: %v", err)
	}
}

func TestValidateCreate_AdditionalWorkspaceSkillsValid(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{
				Name:   "work",
				Skills: []string{"pack:acme/skills/example@v1", "npm:@acme/cli", "@acme/browser-use"},
			},
			{
				// The same skill in a different workspace is allowed:
				// uniqueness is enforced per workspace only (#568).
				Name:   "research",
				Skills: []string{"pack:acme/skills/example@v1"},
			},
		},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err != nil {
		t.Fatalf("expected no error for valid workspace skills, got: %v", err)
	}
}

func TestValidateCreate_AdditionalWorkspaceSkillsDuplicate(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{
				Name:   "work",
				Skills: []string{"@acme/browser-use", "@acme/browser-use"},
			},
		},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for duplicate skill within one workspace")
	}
	if !strings.Contains(err.Error(), "duplicated within the workspace") {
		t.Errorf("expected per-workspace duplicate error, got: %v", err)
	}
}

func TestValidateCreate_AdditionalWorkspaceSkillsInvalidName(t *testing.T) {
	v := &OpenClawInstanceValidator{}
	instance := newTestInstance()
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{
				Name:   "work",
				Skills: []string{"foo:bar"},
			},
		},
	}

	_, err := v.ValidateCreate(context.Background(), instance)
	if err == nil {
		t.Fatal("expected error for invalid workspace skill name")
	}
	if !strings.Contains(err.Error(), "invalid character") {
		t.Errorf("expected invalid character error, got: %v", err)
	}
}
