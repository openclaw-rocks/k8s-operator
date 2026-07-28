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
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// newTestInstance creates a minimal OpenClawInstance for testing.
func newTestInstance(name string) *openclawv1alpha1.OpenClawInstance {
	return &openclawv1alpha1.OpenClawInstance{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "test-ns",
		},
		Spec: openclawv1alpha1.OpenClawInstanceSpec{},
	}
}

// ---------------------------------------------------------------------------
// common.go tests
// ---------------------------------------------------------------------------

func TestLabels(t *testing.T) {
	instance := newTestInstance("my-instance")
	labels := Labels(instance)

	expected := map[string]string{
		"app.kubernetes.io/name":       "openclaw",
		"app.kubernetes.io/instance":   "my-instance",
		"app.kubernetes.io/managed-by": "openclaw-operator",
	}

	if len(labels) != len(expected) {
		t.Fatalf("expected %d labels, got %d", len(expected), len(labels))
	}
	for k, v := range expected {
		if labels[k] != v {
			t.Errorf("label %q: expected %q, got %q", k, v, labels[k])
		}
	}
}

func TestSelectorLabels(t *testing.T) {
	instance := newTestInstance("my-instance")
	labels := SelectorLabels(instance)

	expected := map[string]string{
		"app.kubernetes.io/name":     "openclaw",
		"app.kubernetes.io/instance": "my-instance",
	}

	if len(labels) != len(expected) {
		t.Fatalf("expected %d selector labels, got %d", len(expected), len(labels))
	}
	for k, v := range expected {
		if labels[k] != v {
			t.Errorf("selector label %q: expected %q, got %q", k, v, labels[k])
		}
	}
}

func TestSelectorLabels_SubsetOfLabels(t *testing.T) {
	instance := newTestInstance("test")
	allLabels := Labels(instance)
	selectorLabels := SelectorLabels(instance)

	for k, v := range selectorLabels {
		if allLabels[k] != v {
			t.Errorf("selector label %q=%q is not present in full labels", k, v)
		}
	}

	if len(selectorLabels) >= len(allLabels) {
		t.Error("selector labels should be a strict subset of full labels")
	}
}

func TestGetImage(t *testing.T) {
	tests := []struct {
		name     string
		image    openclawv1alpha1.ImageSpec
		expected string
	}{
		{
			name:     "defaults",
			image:    openclawv1alpha1.ImageSpec{},
			expected: "ghcr.io/openclaw/openclaw:latest",
		},
		{
			name: "custom repo and tag",
			image: openclawv1alpha1.ImageSpec{
				Repository: "my-registry.io/openclaw",
				Tag:        "v1.2.3",
			},
			expected: "my-registry.io/openclaw:v1.2.3",
		},
		{
			name: "digest takes precedence over tag",
			image: openclawv1alpha1.ImageSpec{
				Repository: "my-registry.io/openclaw",
				Tag:        "v1.2.3",
				Digest:     "sha256:abc123",
			},
			expected: "my-registry.io/openclaw@sha256:abc123",
		},
		{
			name: "digest with default repo",
			image: openclawv1alpha1.ImageSpec{
				Digest: "sha256:def456",
			},
			expected: "ghcr.io/openclaw/openclaw@sha256:def456",
		},
		{
			name: "custom repo with default tag",
			image: openclawv1alpha1.ImageSpec{
				Repository: "custom.io/img",
			},
			expected: "custom.io/img:latest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := newTestInstance("test")
			instance.Spec.Image = tt.image

			got := GetImage(instance)
			if got != tt.expected {
				t.Errorf("GetImage() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestNameHelpers(t *testing.T) {
	instance := newTestInstance("foo")

	tests := []struct {
		name     string
		fn       func(*openclawv1alpha1.OpenClawInstance) string
		expected string
	}{
		{"StatefulSetName", StatefulSetName, "foo"},
		{"DeploymentName", DeploymentName, "foo"},
		{"ServiceName", ServiceName, "foo"},
		{"RoleName", RoleName, "foo"},
		{"RoleBindingName", RoleBindingName, "foo"},
		{"ConfigMapName", ConfigMapName, "foo-config"},
		{"PVCName", PVCName, "foo-data"},
		{"NetworkPolicyName", NetworkPolicyName, "foo"},
		{"PDBName", PDBName, "foo"},
		{"IngressName", IngressName, "foo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.fn(instance)
			if got != tt.expected {
				t.Errorf("%s() = %q, want %q", tt.name, got, tt.expected)
			}
		})
	}
}

func TestServiceAccountName_Default(t *testing.T) {
	instance := newTestInstance("my-inst")
	if got := ServiceAccountName(instance); got != "my-inst" {
		t.Errorf("ServiceAccountName() = %q, want %q", got, "my-inst")
	}
}

func TestServiceAccountName_Custom(t *testing.T) {
	instance := newTestInstance("my-inst")
	instance.Spec.Security.RBAC.ServiceAccountName = "custom-sa"
	if got := ServiceAccountName(instance); got != "custom-sa" {
		t.Errorf("ServiceAccountName() = %q, want %q", got, "custom-sa")
	}
}

func TestPtr(t *testing.T) {
	intVal := Ptr(int32(42))
	if *intVal != 42 {
		t.Errorf("Ptr(42) = %d, want 42", *intVal)
	}

	boolVal := Ptr(true)
	if !*boolVal {
		t.Error("Ptr(true) should be true")
	}

	strVal := Ptr("hello")
	if *strVal != "hello" {
		t.Errorf("Ptr(hello) = %q, want %q", *strVal, "hello")
	}
}

// ---------------------------------------------------------------------------
// deployment.go tests
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_Defaults(t *testing.T) {
	instance := newTestInstance("test-deploy")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// ObjectMeta
	if sts.Name != "test-deploy" {
		t.Errorf("statefulset name = %q, want %q", sts.Name, "test-deploy")
	}
	if sts.Namespace != "test-ns" {
		t.Errorf("statefulset namespace = %q, want %q", sts.Namespace, "test-ns")
	}

	// Labels present
	if sts.Labels["app.kubernetes.io/name"] != "openclaw" {
		t.Error("statefulset missing app.kubernetes.io/name label")
	}

	// Replicas
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 1 {
		t.Errorf("replicas = %v, want 1", sts.Spec.Replicas)
	}

	// StatefulSet-specific fields
	if sts.Spec.ServiceName != "test-deploy" {
		t.Errorf("serviceName = %q, want %q", sts.Spec.ServiceName, "test-deploy")
	}
	if sts.Spec.PodManagementPolicy != appsv1.ParallelPodManagement {
		t.Errorf("podManagementPolicy = %v, want Parallel", sts.Spec.PodManagementPolicy)
	}
	if sts.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
		t.Errorf("updateStrategy = %v, want RollingUpdate", sts.Spec.UpdateStrategy.Type)
	}
	if sts.Spec.PersistentVolumeClaimRetentionPolicy == nil {
		t.Fatal("persistentVolumeClaimRetentionPolicy should be set (prevents spec drift)")
	}
	if sts.Spec.PersistentVolumeClaimRetentionPolicy.WhenDeleted != appsv1.RetainPersistentVolumeClaimRetentionPolicyType {
		t.Errorf("pvcRetentionPolicy.whenDeleted = %v, want Retain", sts.Spec.PersistentVolumeClaimRetentionPolicy.WhenDeleted)
	}
	if sts.Spec.PersistentVolumeClaimRetentionPolicy.WhenScaled != appsv1.RetainPersistentVolumeClaimRetentionPolicyType {
		t.Errorf("pvcRetentionPolicy.whenScaled = %v, want Retain", sts.Spec.PersistentVolumeClaimRetentionPolicy.WhenScaled)
	}

	// Selector labels
	sel := sts.Spec.Selector.MatchLabels
	if sel["app.kubernetes.io/name"] != "openclaw" || sel["app.kubernetes.io/instance"] != "test-deploy" {
		t.Error("selector labels do not match expected values")
	}

	// Config hash annotation
	ann := sts.Spec.Template.Annotations
	if _, ok := ann["openclaw.rocks/config-hash"]; !ok {
		t.Error("config-hash annotation missing from pod template")
	}

	// Pod security context
	psc := sts.Spec.Template.Spec.SecurityContext
	if psc == nil {
		t.Fatal("pod security context is nil")
	}
	if psc.RunAsNonRoot == nil || !*psc.RunAsNonRoot {
		t.Error("pod security context: runAsNonRoot should be true")
	}
	if psc.RunAsUser == nil || *psc.RunAsUser != 1000 {
		t.Errorf("pod security context: runAsUser = %v, want 1000", psc.RunAsUser)
	}
	if psc.RunAsGroup == nil || *psc.RunAsGroup != 1000 {
		t.Errorf("pod security context: runAsGroup = %v, want 1000", psc.RunAsGroup)
	}
	if psc.FSGroup == nil || *psc.FSGroup != 1000 {
		t.Errorf("pod security context: fsGroup = %v, want 1000", psc.FSGroup)
	}
	if psc.SeccompProfile == nil || psc.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Error("pod security context: seccomp profile should be RuntimeDefault")
	}

	// Containers (main + gateway-proxy + otel-collector; metrics enabled by default)
	containers := sts.Spec.Template.Spec.Containers
	if len(containers) != 3 {
		t.Fatalf("expected 3 containers (main + gateway-proxy + otel-collector), got %d", len(containers))
	}

	main := containers[0]
	if main.Name != "openclaw" {
		t.Errorf("main container name = %q, want %q", main.Name, "openclaw")
	}
	if main.Image != "ghcr.io/openclaw/openclaw:latest" {
		t.Errorf("main container image = %q, want default image", main.Image)
	}
	if main.ImagePullPolicy != corev1.PullIfNotPresent {
		t.Errorf("image pull policy = %v, want IfNotPresent", main.ImagePullPolicy)
	}

	// Container security context
	csc := main.SecurityContext
	if csc == nil {
		t.Fatal("container security context is nil")
	}
	if csc.AllowPrivilegeEscalation == nil || *csc.AllowPrivilegeEscalation {
		t.Error("container security context: allowPrivilegeEscalation should be false")
	}
	if csc.RunAsNonRoot == nil || !*csc.RunAsNonRoot {
		t.Error("container security context: runAsNonRoot should be true")
	}
	if csc.Capabilities == nil || len(csc.Capabilities.Drop) != 1 || csc.Capabilities.Drop[0] != "ALL" {
		t.Error("container security context: capabilities should drop ALL")
	}
	if csc.SeccompProfile == nil || csc.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Error("container security context: seccomp profile should be RuntimeDefault")
	}

	// HOME env var must be set to match the mount path
	if len(main.Env) < 1 || main.Env[0].Name != "HOME" || main.Env[0].Value != "/home/openclaw" {
		t.Error("HOME env var should be set to /home/openclaw")
	}

	// Ports (gateway, canvas - metrics port is on the OTel collector sidecar)
	if len(main.Ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(main.Ports))
	}
	assertContainerPort(t, main.Ports, "gateway", GatewayPort)
	assertContainerPort(t, main.Ports, "canvas", CanvasPort)

	// Default resources
	cpuReq := main.Resources.Requests[corev1.ResourceCPU]
	if cpuReq.String() != "500m" {
		t.Errorf("cpu request = %v, want 500m", cpuReq.String())
	}
	memReq := main.Resources.Requests[corev1.ResourceMemory]
	if memReq.Cmp(resource.MustParse("1Gi")) != 0 {
		t.Errorf("memory request = %v, want 1Gi", memReq.String())
	}
	cpuLim := main.Resources.Limits[corev1.ResourceCPU]
	if cpuLim.String() != "2" {
		t.Errorf("cpu limit = %v, want 2 (2000m)", cpuLim.String())
	}
	memLim := main.Resources.Limits[corev1.ResourceMemory]
	if memLim.Cmp(resource.MustParse("4Gi")) != 0 {
		t.Errorf("memory limit = %v, want 4Gi", memLim.String())
	}

	// Probes
	if main.LivenessProbe == nil {
		t.Error("liveness probe should not be nil by default")
	}
	if main.ReadinessProbe == nil {
		t.Error("readiness probe should not be nil by default")
	}
	if main.StartupProbe == nil {
		t.Error("startup probe should not be nil by default")
	}

	// Liveness probe defaults (HTTPGet via proxy sidecar)
	if main.LivenessProbe.HTTPGet == nil {
		t.Fatal("liveness probe should use HTTPGet handler")
	}
	if main.LivenessProbe.HTTPGet.Path != "/healthz" {
		t.Errorf("liveness probe path = %q, want %q", main.LivenessProbe.HTTPGet.Path, "/healthz")
	}
	if main.LivenessProbe.HTTPGet.Port.IntValue() != int(GatewayProxyPort) {
		t.Errorf("liveness probe port = %d, want %d", main.LivenessProbe.HTTPGet.Port.IntValue(), GatewayProxyPort)
	}
	if main.LivenessProbe.InitialDelaySeconds != 30 {
		t.Errorf("liveness probe initialDelaySeconds = %d, want 30", main.LivenessProbe.InitialDelaySeconds)
	}
	if main.LivenessProbe.PeriodSeconds != 10 {
		t.Errorf("liveness probe periodSeconds = %d, want 10", main.LivenessProbe.PeriodSeconds)
	}

	// Readiness probe defaults
	if main.ReadinessProbe.InitialDelaySeconds != 5 {
		t.Errorf("readiness probe initialDelaySeconds = %d, want 5", main.ReadinessProbe.InitialDelaySeconds)
	}

	// Startup probe defaults
	if main.StartupProbe.InitialDelaySeconds != 5 {
		t.Errorf("startup probe initialDelaySeconds = %d, want 5", main.StartupProbe.InitialDelaySeconds)
	}
	if main.StartupProbe.FailureThreshold != 60 {
		t.Errorf("startup probe failureThreshold = %d, want 60", main.StartupProbe.FailureThreshold)
	}

	// Data volume mount
	assertVolumeMount(t, main.VolumeMounts, "data", "/home/openclaw/.openclaw")

	// Volumes - default persistence is enabled, so data volume should be PVC
	volumes := sts.Spec.Template.Spec.Volumes
	dataVol := findVolume(volumes, "data")
	if dataVol == nil {
		t.Fatal("data volume not found")
	}
	if dataVol.PersistentVolumeClaim == nil {
		t.Error("data volume should use PVC by default")
	}
	if dataVol.PersistentVolumeClaim.ClaimName != "test-deploy-data" {
		t.Errorf("PVC claim name = %q, want %q", dataVol.PersistentVolumeClaim.ClaimName, "test-deploy-data")
	}
}

func TestBuildStatefulSet_WithChromium(t *testing.T) {
	instance := newTestInstance("chromium-test")
	instance.Spec.Chromium.Enabled = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	containers := sts.Spec.Template.Spec.Containers
	initContainers := sts.Spec.Template.Spec.InitContainers

	if len(containers) != 3 {
		t.Fatalf("expected 3 containers (main + gateway-proxy + otel-collector), got %d", len(containers))
	}

	// Find chromium in init containers (native sidecar)
	var chromium *corev1.Container
	for i := range initContainers {
		if initContainers[i].Name == "chromium" {
			chromium = &initContainers[i]
			break
		}
	}
	if chromium == nil {
		t.Fatal("chromium init container not found")
	}

	// Native sidecar: RestartPolicy must be Always
	if chromium.RestartPolicy == nil || *chromium.RestartPolicy != corev1.ContainerRestartPolicyAlways {
		t.Errorf("chromium RestartPolicy = %v, want Always (native sidecar)", chromium.RestartPolicy)
	}

	// Main container should have OPENCLAW_CHROMIUM_CDP using localhost (sidecar shares pod network)
	mainContainer := containers[0]
	foundChromiumCDP := false
	for _, env := range mainContainer.Env {
		if env.Name == "OPENCLAW_CHROMIUM_CDP" {
			foundChromiumCDP = true
			expected := fmt.Sprintf("http://127.0.0.1:%d", ChromiumPort)
			if env.Value != expected {
				t.Errorf("OPENCLAW_CHROMIUM_CDP = %q, want %q", env.Value, expected)
			}
		}
	}
	if !foundChromiumCDP {
		t.Error("main container should have OPENCLAW_CHROMIUM_CDP env var when chromium is enabled")
	}

	// Chrome runs directly - no PORT/HOST env vars (those were browserless-specific)
	// HOME=/tmp is set so fontconfig and other tools find a writable home directory.
	foundHome := false
	for _, env := range chromium.Env {
		if env.Name == "PORT" {
			t.Error("chromium container should not have PORT env var (no longer using browserless)")
		}
		if env.Name == "HOST" {
			t.Error("chromium container should not have HOST env var (no longer using browserless)")
		}
		if env.Name == "HOME" && env.Value == "/tmp" {
			foundHome = true
		}
	}
	if !foundHome {
		t.Error("chromium container should have HOME=/tmp env var")
	}

	// Chromium image defaults
	expectedImage := DefaultChromiumImage + ":" + DefaultChromiumTag
	if chromium.Image != expectedImage {
		t.Errorf("chromium image = %q, want %q", chromium.Image, expectedImage)
	}

	// Command must be the entrypoint wrapper that fixes unquoted $@ in
	// upstream run.sh, preventing word-splitting of args with spaces (#396).
	if len(chromium.Command) != len(ChromiumEntrypointCommand) {
		t.Errorf("chromium Command = %v, want ChromiumEntrypointCommand", chromium.Command)
	}

	// Container args should include launch flags
	if len(chromium.Args) == 0 {
		t.Fatal("chromium container should have Args with Chrome launch flags")
	}
	argsStr := strings.Join(chromium.Args, " ")
	for _, required := range []string{"--no-sandbox", "--disable-gpu", "--no-first-run", "--disable-oom-score-adj"} {
		if !strings.Contains(argsStr, required) {
			t.Errorf("chromium Args missing %q", required)
		}
	}

	// Chromium port
	if len(chromium.Ports) != 1 {
		t.Fatalf("chromium container should have 1 port, got %d", len(chromium.Ports))
	}
	if chromium.Ports[0].ContainerPort != ChromiumPort {
		t.Errorf("chromium port = %d, want %d", chromium.Ports[0].ContainerPort, ChromiumPort)
	}
	if chromium.Ports[0].Name != "cdp" {
		t.Errorf("chromium port name = %q, want %q", chromium.Ports[0].Name, "cdp")
	}

	// Chromium security context
	csc := chromium.SecurityContext
	if csc == nil {
		t.Fatal("chromium security context is nil")
	}
	if csc.ReadOnlyRootFilesystem == nil || *csc.ReadOnlyRootFilesystem {
		t.Error("chromium: readOnlyRootFilesystem should be false (Chromium needs writable dirs)")
	}
	if csc.RunAsUser == nil || *csc.RunAsUser != 65534 {
		t.Errorf("chromium: runAsUser = %v, want 65534 (nobody)", csc.RunAsUser)
	}

	// Chromium resource defaults
	cpuReq := chromium.Resources.Requests[corev1.ResourceCPU]
	if cpuReq.String() != "250m" {
		t.Errorf("chromium cpu request = %v, want 250m", cpuReq.String())
	}
	memReq := chromium.Resources.Requests[corev1.ResourceMemory]
	if memReq.Cmp(resource.MustParse("512Mi")) != 0 {
		t.Errorf("chromium memory request = %v, want 512Mi", memReq.String())
	}

	// Chromium volume mounts
	assertVolumeMount(t, chromium.VolumeMounts, "chromium-tmp", "/tmp")
	assertVolumeMount(t, chromium.VolumeMounts, "chromium-shm", "/dev/shm")
	assertVolumeMount(t, chromium.VolumeMounts, "chromium-data", "/chromium-data")

	// Volumes - check chromium-specific volumes exist
	volumes := sts.Spec.Template.Spec.Volumes
	tmpVol := findVolume(volumes, "chromium-tmp")
	if tmpVol == nil {
		t.Fatal("chromium-tmp volume not found")
	}
	if tmpVol.EmptyDir == nil {
		t.Error("chromium-tmp should be emptyDir")
	}

	shmVol := findVolume(volumes, "chromium-shm")
	if shmVol == nil {
		t.Fatal("chromium-shm volume not found")
	}
	if shmVol.EmptyDir == nil {
		t.Fatal("chromium-shm should be emptyDir")
	}
	if shmVol.EmptyDir.Medium != corev1.StorageMediumMemory {
		t.Errorf("chromium-shm medium = %v, want Memory", shmVol.EmptyDir.Medium)
	}
	expectedShmSize := resource.NewQuantity(1024*1024*1024, resource.BinarySI) // 1Gi
	if shmVol.EmptyDir.SizeLimit == nil {
		t.Fatal("chromium-shm sizeLimit is nil")
	}
	if shmVol.EmptyDir.SizeLimit.Cmp(*expectedShmSize) != 0 {
		t.Errorf("chromium-shm sizeLimit = %v, want 1Gi", shmVol.EmptyDir.SizeLimit.String())
	}

	// chromium-data should be emptyDir when persistence is not enabled
	dataVol := findVolume(volumes, "chromium-data")
	if dataVol == nil {
		t.Fatal("chromium-data volume not found")
	}
	if dataVol.EmptyDir == nil {
		t.Error("chromium-data should be emptyDir when persistence is disabled")
	}

	// DEFAULT_LAUNCH_ARGS should not exist anymore
	for _, env := range chromium.Env {
		if env.Name == "DEFAULT_LAUNCH_ARGS" {
			t.Error("DEFAULT_LAUNCH_ARGS env var should not be set on chromium container")
		}
	}
}

func TestBuildStatefulSet_ChromiumExtraArgs(t *testing.T) {
	instance := newTestInstance("chromium-extraargs")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Chromium.ExtraArgs = []string{
		"--window-size=1920,1080",
		"--user-agent=CustomAgent/1.0",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var chromium *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "chromium" {
			chromium = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if chromium == nil {
		t.Fatal("chromium init container not found")
	}

	// ExtraArgs should be merged into container Args along with defaults
	argsStr := strings.Join(chromium.Args, " ")
	for _, extra := range instance.Spec.Chromium.ExtraArgs {
		if !strings.Contains(argsStr, extra) {
			t.Errorf("chromium Args missing user ExtraArg %q", extra)
		}
	}
	// Default args should also be present
	if !strings.Contains(argsStr, "--no-sandbox") {
		t.Error("chromium Args missing --no-sandbox")
	}
}

func TestBuildStatefulSet_ChromiumExtraEnv(t *testing.T) {
	instance := newTestInstance("chromium-extraenv")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Chromium.ExtraEnv = []corev1.EnvVar{
		{Name: "DEFAULT_STEALTH", Value: "true"},
		{Name: "CUSTOM_VAR", Value: "hello"},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var chromium *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "chromium" {
			chromium = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if chromium == nil {
		t.Fatal("chromium init container not found")
	}

	// Extra env vars should be appended
	foundStealth := false
	foundCustom := false
	for _, env := range chromium.Env {
		if env.Name == "DEFAULT_STEALTH" && env.Value == "true" {
			foundStealth = true
		}
		if env.Name == "CUSTOM_VAR" && env.Value == "hello" {
			foundCustom = true
		}
	}
	if !foundStealth {
		t.Error("chromium container should have DEFAULT_STEALTH env var from ExtraEnv")
	}
	if !foundCustom {
		t.Error("chromium container should have CUSTOM_VAR env var from ExtraEnv")
	}
}

func TestBuildStatefulSet_ChromiumNoExtraArgs(t *testing.T) {
	instance := newTestInstance("chromium-no-args")
	instance.Spec.Chromium.Enabled = true
	// No ExtraArgs - should still have default launch args

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var chromium *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "chromium" {
			chromium = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if chromium == nil {
		t.Fatal("chromium init container not found")
	}

	// Should have default args even without ExtraArgs
	if len(chromium.Args) == 0 {
		t.Fatal("chromium container should have default Args")
	}
	argsStr := strings.Join(chromium.Args, " ")
	if !strings.Contains(argsStr, "--no-sandbox") {
		t.Error("chromium Args missing --no-sandbox")
	}
}

func TestBuildStatefulSet_CustomResources(t *testing.T) {
	instance := newTestInstance("res-test")
	instance.Spec.Resources = openclawv1alpha1.ResourcesSpec{
		Requests: openclawv1alpha1.ResourceList{
			CPU:    "1",
			Memory: "2Gi",
		},
		Limits: openclawv1alpha1.ResourceList{
			CPU:    "4",
			Memory: "8Gi",
		},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	cpuReq := main.Resources.Requests[corev1.ResourceCPU]
	if cpuReq.String() != "1" {
		t.Errorf("cpu request = %v, want 1", cpuReq.String())
	}
	memReq := main.Resources.Requests[corev1.ResourceMemory]
	if memReq.Cmp(resource.MustParse("2Gi")) != 0 {
		t.Errorf("memory request = %v, want 2Gi", memReq.String())
	}
	cpuLim := main.Resources.Limits[corev1.ResourceCPU]
	if cpuLim.String() != "4" {
		t.Errorf("cpu limit = %v, want 4", cpuLim.String())
	}
	memLim := main.Resources.Limits[corev1.ResourceMemory]
	if memLim.Cmp(resource.MustParse("8Gi")) != 0 {
		t.Errorf("memory limit = %v, want 8Gi", memLim.String())
	}
}

func TestBuildStatefulSet_ImageDigest(t *testing.T) {
	instance := newTestInstance("digest-test")
	instance.Spec.Image = openclawv1alpha1.ImageSpec{
		Repository: "my-registry.io/openclaw",
		Tag:        "v1.0.0",
		Digest:     "sha256:abcdef1234567890",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	expected := "my-registry.io/openclaw@sha256:abcdef1234567890"
	if main.Image != expected {
		t.Errorf("image = %q, want %q", main.Image, expected)
	}
}

func TestBuildStatefulSet_ProbesDisabled(t *testing.T) {
	instance := newTestInstance("probes-disabled")
	instance.Spec.Probes = &openclawv1alpha1.ProbesSpec{
		Liveness: &openclawv1alpha1.ProbeSpec{
			Enabled: Ptr(false),
		},
		Readiness: &openclawv1alpha1.ProbeSpec{
			Enabled: Ptr(false),
		},
		Startup: &openclawv1alpha1.ProbeSpec{
			Enabled: Ptr(false),
		},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	if main.LivenessProbe != nil {
		t.Error("liveness probe should be nil when disabled")
	}
	if main.ReadinessProbe != nil {
		t.Error("readiness probe should be nil when disabled")
	}
	if main.StartupProbe != nil {
		t.Error("startup probe should be nil when disabled")
	}
}

func TestBuildStatefulSet_CustomProbeValues(t *testing.T) {
	instance := newTestInstance("probes-custom")
	instance.Spec.Probes = &openclawv1alpha1.ProbesSpec{
		Liveness: &openclawv1alpha1.ProbeSpec{
			InitialDelaySeconds: Ptr(int32(60)),
			PeriodSeconds:       Ptr(int32(20)),
			TimeoutSeconds:      Ptr(int32(10)),
			FailureThreshold:    Ptr(int32(5)),
		},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	probe := sts.Spec.Template.Spec.Containers[0].LivenessProbe

	if probe == nil {
		t.Fatal("liveness probe should not be nil")
	}
	if probe.InitialDelaySeconds != 60 {
		t.Errorf("liveness initialDelaySeconds = %d, want 60", probe.InitialDelaySeconds)
	}
	if probe.PeriodSeconds != 20 {
		t.Errorf("liveness periodSeconds = %d, want 20", probe.PeriodSeconds)
	}
	if probe.TimeoutSeconds != 10 {
		t.Errorf("liveness timeoutSeconds = %d, want 10", probe.TimeoutSeconds)
	}
	if probe.FailureThreshold != 5 {
		t.Errorf("liveness failureThreshold = %d, want 5", probe.FailureThreshold)
	}
}

func TestBuildStatefulSet_PersistenceDisabled(t *testing.T) {
	instance := newTestInstance("no-pvc")
	instance.Spec.Storage.Persistence.Enabled = Ptr(false)

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	volumes := sts.Spec.Template.Spec.Volumes

	dataVol := findVolume(volumes, "data")
	if dataVol == nil {
		t.Fatal("data volume not found")
	}
	if dataVol.EmptyDir == nil {
		t.Error("data volume should be emptyDir when persistence is disabled")
	}
	if dataVol.PersistentVolumeClaim != nil {
		t.Error("data volume should not use PVC when persistence is disabled")
	}
}

func TestBuildStatefulSet_ExistingClaim(t *testing.T) {
	instance := newTestInstance("existing-pvc")
	instance.Spec.Storage.Persistence.ExistingClaim = "my-existing-pvc"

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	volumes := sts.Spec.Template.Spec.Volumes

	dataVol := findVolume(volumes, "data")
	if dataVol == nil {
		t.Fatal("data volume not found")
	}
	if dataVol.PersistentVolumeClaim == nil {
		t.Fatal("data volume should use PVC")
	}
	if dataVol.PersistentVolumeClaim.ClaimName != "my-existing-pvc" {
		t.Errorf("PVC claim name = %q, want %q", dataVol.PersistentVolumeClaim.ClaimName, "my-existing-pvc")
	}
}

func TestBuildStatefulSet_ConfigVolume_RawConfig(t *testing.T) {
	instance := newTestInstance("raw-cfg")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"key":"value"}`),
		},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	// Main container should have config volume mounted read-only at /operator-config
	// for the postStart lifecycle hook to restore config on container restarts.
	assertVolumeMount(t, main.VolumeMounts, "config", "/operator-config")
	for _, vm := range main.VolumeMounts {
		if vm.Name == "config" && !vm.ReadOnly {
			t.Error("config volume mount in main container should be read-only")
		}
	}

	// Init container should copy config from ConfigMap to data volume
	initContainers := sts.Spec.Template.Spec.InitContainers
	if len(initContainers) != 4 {
		t.Fatalf("expected 4 init containers (init-config + init-uv + init-pip + init-plugin-runtime-deps), got %d", len(initContainers))
	}
	initC := initContainers[0]
	if initC.Name != "init-config" {
		t.Errorf("init container name = %q, want %q", initC.Name, "init-config")
	}
	assertVolumeMount(t, initC.VolumeMounts, "data", "/data")
	assertVolumeMount(t, initC.VolumeMounts, "config", "/config")

	// Init container must set SeccompProfile to prevent spec drift
	if initC.SecurityContext == nil {
		t.Fatal("init-config security context is nil")
	}
	if initC.SecurityContext.SeccompProfile == nil || initC.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Error("init-config container: seccomp profile should be RuntimeDefault (prevents spec drift)")
	}

	// Should have config volume pointing to managed configmap
	volumes := sts.Spec.Template.Spec.Volumes
	cfgVol := findVolume(volumes, "config")
	if cfgVol == nil {
		t.Fatal("config volume not found")
	}
	if cfgVol.ConfigMap == nil {
		t.Fatal("config volume should use ConfigMap")
	}
	if cfgVol.ConfigMap.Name != "raw-cfg-config" {
		t.Errorf("config volume configmap name = %q, want %q", cfgVol.ConfigMap.Name, "raw-cfg-config")
	}
}

func TestBuildStatefulSet_ConfigVolume_ConfigMapRef(t *testing.T) {
	instance := newTestInstance("ref-cfg")
	instance.Spec.Config.ConfigMapRef = &openclawv1alpha1.ConfigMapKeySelector{
		Name: "external-config",
		Key:  "my-config.json",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Init container should copy openclaw.json (operator-managed key) from ConfigMap to data volume.
	// The controller reads the external CM and writes enriched content into the
	// operator-managed CM under "openclaw.json", so the init container always uses that key.
	initContainers := sts.Spec.Template.Spec.InitContainers
	if len(initContainers) != 4 {
		t.Fatalf("expected 4 init containers (init-config + init-uv + init-pip + init-plugin-runtime-deps), got %d", len(initContainers))
	}
	initC := initContainers[0]
	assertVolumeMount(t, initC.VolumeMounts, "data", "/data")
	assertVolumeMount(t, initC.VolumeMounts, "config", "/config")

	// Verify the command starts with copying openclaw.json (not the custom key)
	expectedPrefix := "cp /config/'openclaw.json' /data/openclaw.json"
	if len(initC.Command) != 3 || !strings.HasPrefix(initC.Command[2], expectedPrefix) {
		t.Errorf("init container command should start with %q, got %v", expectedPrefix, initC.Command)
	}

	// Volume should reference the operator-managed ConfigMap (not the external one)
	volumes := sts.Spec.Template.Spec.Volumes
	cfgVol := findVolume(volumes, "config")
	if cfgVol == nil {
		t.Fatal("config volume not found")
	}
	if cfgVol.ConfigMap.Name != "ref-cfg-config" {
		t.Errorf("config volume configmap name = %q, want %q", cfgVol.ConfigMap.Name, "ref-cfg-config")
	}
}

func TestBuildStatefulSet_ConfigMapRef_DefaultKey(t *testing.T) {
	instance := newTestInstance("ref-default-key")
	instance.Spec.Config.ConfigMapRef = &openclawv1alpha1.ConfigMapKeySelector{
		Name: "external-config",
		// Key not set - operator-managed CM always uses "openclaw.json"
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Init container should use "openclaw.json" (operator-managed key)
	initContainers := sts.Spec.Template.Spec.InitContainers
	if len(initContainers) != 4 {
		t.Fatalf("expected 4 init containers (init-config + init-uv + init-pip + init-plugin-runtime-deps), got %d", len(initContainers))
	}
	expectedPrefix := "cp /config/'openclaw.json' /data/openclaw.json"
	if !strings.HasPrefix(initContainers[0].Command[2], expectedPrefix) {
		t.Errorf("init container command should start with %q, got %q", expectedPrefix, initContainers[0].Command[2])
	}

	// Volume should reference the operator-managed ConfigMap
	cfgVol := findVolume(sts.Spec.Template.Spec.Volumes, "config")
	if cfgVol == nil {
		t.Fatal("config volume not found")
	}
	if cfgVol.ConfigMap.Name != "ref-default-key-config" {
		t.Errorf("config volume configmap name = %q, want %q", cfgVol.ConfigMap.Name, "ref-default-key-config")
	}
}

func TestBuildStatefulSet_VanillaDeployment_HasInitContainer(t *testing.T) {
	instance := newTestInstance("no-config")
	// No config set at all — vanilla deployment

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Vanilla deployments get init-config + init-uv + init-pip + init-plugin-runtime-deps
	if len(sts.Spec.Template.Spec.InitContainers) != 4 {
		t.Fatalf("expected 4 init containers for vanilla deployment, got %d", len(sts.Spec.Template.Spec.InitContainers))
	}
	if sts.Spec.Template.Spec.InitContainers[0].Name != "init-config" {
		t.Errorf("init container name = %q, want %q", sts.Spec.Template.Spec.InitContainers[0].Name, "init-config")
	}

	// Verify config volume is present
	configVol := findVolume(sts.Spec.Template.Spec.Volumes, "config")
	if configVol == nil {
		t.Fatal("config volume not found for vanilla deployment")
	}
	if configVol.ConfigMap == nil {
		t.Fatal("config volume should use ConfigMap")
	}
	if configVol.ConfigMap.Name != "no-config-config" {
		t.Errorf("config volume ConfigMap name = %q, want %q", configVol.ConfigMap.Name, "no-config-config")
	}
}

// ---------------------------------------------------------------------------
// PostStart lifecycle hook tests (config restore on container restart)
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_PostStart_OverwriteMode(t *testing.T) {
	instance := newTestInstance("poststart-overwrite")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	if main.Lifecycle == nil || main.Lifecycle.PostStart == nil {
		t.Fatal("expected postStart lifecycle hook on main container")
	}

	cmd := main.Lifecycle.PostStart.Exec.Command
	if len(cmd) != 3 || cmd[0] != "sh" || cmd[1] != "-c" {
		t.Fatalf("expected sh -c command, got %v", cmd)
	}
	expected := "cp /operator-config/openclaw.json /home/openclaw/.openclaw/openclaw.json"
	if cmd[2] != expected {
		t.Errorf("postStart command = %q, want %q", cmd[2], expected)
	}
}

func TestBuildStatefulSet_PostStart_MergeMode(t *testing.T) {
	instance := newTestInstance("poststart-merge")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}
	instance.Spec.Config.MergeMode = ConfigMergeModeMerge

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	if main.Lifecycle == nil || main.Lifecycle.PostStart == nil {
		t.Fatal("expected postStart lifecycle hook for merge mode")
	}

	cmd := main.Lifecycle.PostStart.Exec.Command
	if len(cmd) != 3 || cmd[0] != "sh" || cmd[1] != "-c" {
		t.Fatalf("expected sh -c command, got %v", cmd)
	}

	// Merge mode should use a Node.js deep-merge script
	if !strings.Contains(cmd[2], "node -e") {
		t.Errorf("merge mode postStart should use node, got %q", cmd[2])
	}
	if !strings.Contains(cmd[2], "/operator-config/openclaw.json") {
		t.Errorf("merge mode postStart should reference operator-config path, got %q", cmd[2])
	}
	// Regression #162: node -e argument must be single-quoted so that
	// !Array.isArray is not interpreted as bash history expansion.
	if !strings.Contains(cmd[2], "node -e '") {
		t.Errorf("merge mode postStart must single-quote the node -e argument to avoid bash history expansion (#162), got %q", cmd[2])
	}
}

func TestBuildStatefulSet_PostStart_JSON5Mode_NoHook(t *testing.T) {
	instance := newTestInstance("poststart-json5")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}
	instance.Spec.Config.Format = ConfigFormatJSON5

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	// JSON5 mode can't use postStart (needs npx, too slow)
	if main.Lifecycle != nil && main.Lifecycle.PostStart != nil {
		t.Error("JSON5 mode should not have a postStart hook (npx too slow)")
	}
}

func TestBuildStatefulSet_PostStart_ConfigMapRef(t *testing.T) {
	instance := newTestInstance("poststart-ref")
	instance.Spec.Config.ConfigMapRef = &openclawv1alpha1.ConfigMapKeySelector{
		Name: "external-config",
		Key:  "my-config.json",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	if main.Lifecycle == nil || main.Lifecycle.PostStart == nil {
		t.Fatal("expected postStart lifecycle hook with configMapRef")
	}

	// PostStart should use openclaw.json (operator-managed key), not the
	// external CM's custom key, because the operator-managed CM always
	// stores enriched config under "openclaw.json".
	cmd := main.Lifecycle.PostStart.Exec.Command[2]
	expected := "cp /operator-config/openclaw.json /home/openclaw/.openclaw/openclaw.json"
	if cmd != expected {
		t.Errorf("postStart command = %q, want %q", cmd, expected)
	}
}

func TestBuildStatefulSet_PostStart_VanillaDeployment(t *testing.T) {
	instance := newTestInstance("poststart-vanilla")
	// No config set at all - vanilla deployment still gets postStart
	// because operator always creates a ConfigMap with gateway.bind=lan

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	if main.Lifecycle == nil || main.Lifecycle.PostStart == nil {
		t.Fatal("vanilla deployment should still have postStart hook")
	}

	cmd := main.Lifecycle.PostStart.Exec.Command[2]
	if !strings.Contains(cmd, "cp /operator-config/openclaw.json") {
		t.Errorf("vanilla postStart should copy config, got %q", cmd)
	}
}

func TestBuildStatefulSet_ServiceAccountName(t *testing.T) {
	instance := newTestInstance("sa-test")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	if sts.Spec.Template.Spec.ServiceAccountName != "sa-test" {
		t.Errorf("serviceAccountName = %q, want %q", sts.Spec.Template.Spec.ServiceAccountName, "sa-test")
	}
}

func TestBuildStatefulSet_AutomountServiceAccountTokenDisabled(t *testing.T) {
	instance := newTestInstance("automount-test")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	token := sts.Spec.Template.Spec.AutomountServiceAccountToken
	if token == nil || *token != false {
		t.Errorf("AutomountServiceAccountToken = %v, want false", token)
	}
}

func TestBuildStatefulSet_ImagePullSecrets(t *testing.T) {
	instance := newTestInstance("pull-secrets")
	instance.Spec.Image.PullSecrets = []corev1.LocalObjectReference{
		{Name: "my-secret"},
		{Name: "other-secret"},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	secrets := sts.Spec.Template.Spec.ImagePullSecrets
	if len(secrets) != 2 {
		t.Fatalf("expected 2 pull secrets, got %d", len(secrets))
	}
	if secrets[0].Name != "my-secret" {
		t.Errorf("first pull secret = %q, want %q", secrets[0].Name, "my-secret")
	}
	if secrets[1].Name != "other-secret" {
		t.Errorf("second pull secret = %q, want %q", secrets[1].Name, "other-secret")
	}
}

func TestBuildStatefulSet_ChromiumCustomImage(t *testing.T) {
	instance := newTestInstance("chromium-custom")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Chromium.Image = openclawv1alpha1.ChromiumImageSpec{
		Repository: "my-registry.io/chromium",
		Tag:        "v120",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	var chromium *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "chromium" {
			chromium = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if chromium == nil {
		t.Fatal("chromium init container not found")
	}
	if chromium.Image != "my-registry.io/chromium:v120" {
		t.Errorf("chromium image = %q, want %q", chromium.Image, "my-registry.io/chromium:v120")
	}
}

func TestBuildStatefulSet_ChromiumDigest(t *testing.T) {
	instance := newTestInstance("chromium-digest")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Chromium.Image = openclawv1alpha1.ChromiumImageSpec{
		Repository: "my-registry.io/chromium",
		Tag:        "v120",
		Digest:     "sha256:chromiumhash",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	var chromium *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "chromium" {
			chromium = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if chromium == nil {
		t.Fatal("chromium init container not found")
	}
	expected := "my-registry.io/chromium@sha256:chromiumhash"
	if chromium.Image != expected {
		t.Errorf("chromium image = %q, want %q", chromium.Image, expected)
	}
}

func TestBuildStatefulSet_NodeSelectorAndTolerations(t *testing.T) {
	instance := newTestInstance("scheduling")
	instance.Spec.Availability.NodeSelector = map[string]string{
		"node-type": "gpu",
	}
	instance.Spec.Availability.Tolerations = []corev1.Toleration{
		{
			Key:      "gpu",
			Operator: corev1.TolerationOpEqual,
			Value:    "true",
			Effect:   corev1.TaintEffectNoSchedule,
		},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	podSpec := sts.Spec.Template.Spec

	if podSpec.NodeSelector["node-type"] != "gpu" {
		t.Error("nodeSelector not applied")
	}
	if len(podSpec.Tolerations) != 1 || podSpec.Tolerations[0].Key != "gpu" {
		t.Error("tolerations not applied")
	}
}

func TestBuildStatefulSet_TopologySpreadConstraints(t *testing.T) {
	instance := newTestInstance("tsc-test")
	instance.Spec.Availability.TopologySpreadConstraints = []corev1.TopologySpreadConstraint{
		{
			MaxSkew:           1,
			TopologyKey:       "topology.kubernetes.io/zone",
			WhenUnsatisfiable: corev1.DoNotSchedule,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/instance": "tsc-test",
				},
			},
		},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	podSpec := sts.Spec.Template.Spec

	if len(podSpec.TopologySpreadConstraints) != 1 {
		t.Fatalf("expected 1 topology spread constraint, got %d", len(podSpec.TopologySpreadConstraints))
	}
	tsc := podSpec.TopologySpreadConstraints[0]
	if tsc.TopologyKey != "topology.kubernetes.io/zone" {
		t.Errorf("topologyKey = %q, want %q", tsc.TopologyKey, "topology.kubernetes.io/zone")
	}
	if tsc.MaxSkew != 1 {
		t.Errorf("maxSkew = %d, want 1", tsc.MaxSkew)
	}
	if tsc.WhenUnsatisfiable != corev1.DoNotSchedule {
		t.Errorf("whenUnsatisfiable = %v, want DoNotSchedule", tsc.WhenUnsatisfiable)
	}
}

func TestBuildStatefulSet_TopologySpreadConstraints_Empty(t *testing.T) {
	instance := newTestInstance("tsc-empty")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	podSpec := sts.Spec.Template.Spec

	if podSpec.TopologySpreadConstraints != nil {
		t.Errorf("expected nil topology spread constraints, got %v", podSpec.TopologySpreadConstraints)
	}
}

func TestBuildStatefulSet_RuntimeClassName(t *testing.T) {
	instance := newTestInstance("rtc-test")
	instance.Spec.Availability.RuntimeClassName = Ptr("kata-fc")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	podSpec := sts.Spec.Template.Spec

	if podSpec.RuntimeClassName == nil {
		t.Fatal("expected RuntimeClassName to be set")
	}
	if *podSpec.RuntimeClassName != "kata-fc" {
		t.Errorf("RuntimeClassName = %q, want %q", *podSpec.RuntimeClassName, "kata-fc")
	}
}

func TestBuildStatefulSet_RuntimeClassName_Unset(t *testing.T) {
	instance := newTestInstance("rtc-unset")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	podSpec := sts.Spec.Template.Spec

	if podSpec.RuntimeClassName != nil {
		t.Errorf("expected nil RuntimeClassName, got %q", *podSpec.RuntimeClassName)
	}
}

func TestBuildStatefulSet_ShareProcessNamespace_DefaultsTrue(t *testing.T) {
	instance := newTestInstance("spn-default")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	podSpec := sts.Spec.Template.Spec

	if podSpec.ShareProcessNamespace == nil {
		t.Fatal("expected ShareProcessNamespace to be set when field unset")
	}
	if !*podSpec.ShareProcessNamespace {
		t.Error("ShareProcessNamespace should default to true")
	}
}

func TestBuildStatefulSet_ShareProcessNamespace_ExplicitTrue(t *testing.T) {
	instance := newTestInstance("spn-true")
	instance.Spec.ShareProcessNamespace = Ptr(true)

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	podSpec := sts.Spec.Template.Spec

	if podSpec.ShareProcessNamespace == nil || !*podSpec.ShareProcessNamespace {
		t.Errorf("ShareProcessNamespace = %v, want true", podSpec.ShareProcessNamespace)
	}
}

func TestBuildStatefulSet_ShareProcessNamespace_ExplicitFalse(t *testing.T) {
	instance := newTestInstance("spn-false")
	instance.Spec.ShareProcessNamespace = Ptr(false)

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	podSpec := sts.Spec.Template.Spec

	if podSpec.ShareProcessNamespace == nil {
		t.Fatal("expected ShareProcessNamespace to be set")
	}
	if *podSpec.ShareProcessNamespace {
		t.Error("ShareProcessNamespace should be false when explicitly disabled")
	}
}

func TestBuildStatefulSet_PodAnnotations_UserAnnotationsPresent(t *testing.T) {
	instance := newTestInstance("pod-ann-test")
	instance.Spec.PodAnnotations = map[string]string{
		"cluster-autoscaler.kubernetes.io/safe-to-evict": "false",
		"custom.io/label": "value",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	ann := sts.Spec.Template.Annotations

	if ann["cluster-autoscaler.kubernetes.io/safe-to-evict"] != "false" {
		t.Errorf("expected safe-to-evict=false, got %q", ann["cluster-autoscaler.kubernetes.io/safe-to-evict"])
	}
	if ann["custom.io/label"] != "value" {
		t.Errorf("expected custom.io/label=value, got %q", ann["custom.io/label"])
	}
}

func TestBuildStatefulSet_PodAnnotations_OperatorKeyWins(t *testing.T) {
	instance := newTestInstance("pod-ann-conflict")
	instance.Spec.PodAnnotations = map[string]string{
		"openclaw.rocks/config-hash": "user-supplied-value",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	ann := sts.Spec.Template.Annotations

	if ann["openclaw.rocks/config-hash"] == "user-supplied-value" {
		t.Error("operator-managed config-hash should not be overridable by user podAnnotations")
	}
	if ann["openclaw.rocks/config-hash"] == "" {
		t.Error("config-hash annotation should still be present")
	}
}

func TestBuildStatefulSet_PodAnnotations_NilStillHasConfigHash(t *testing.T) {
	instance := newTestInstance("pod-ann-nil")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	ann := sts.Spec.Template.Annotations

	if _, ok := ann["openclaw.rocks/config-hash"]; !ok {
		t.Error("config-hash annotation must always be present even when podAnnotations is nil")
	}
}

func TestBuildStatefulSet_EnvAndEnvFrom(t *testing.T) {
	instance := newTestInstance("env-test")
	instance.Spec.Env = []corev1.EnvVar{
		{Name: "MY_VAR", Value: "my-value"},
	}
	instance.Spec.EnvFrom = []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: "api-keys"},
			},
		},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	names := envNames(main.Env)
	expectedPrefix := []string{"HOME", "OPENCLAW_DISABLE_BONJOUR", "OPENCLAW_GATEWAY_HANDSHAKE_TIMEOUT_MS", "NPM_CONFIG_PREFIX", "NPM_CONFIG_CACHE", "PIP_USER"}
	for i, want := range expectedPrefix {
		if i >= len(names) || names[i] != want {
			t.Fatalf("env vars should start with %v, got %v", expectedPrefix, names)
		}
	}
	// User-defined vars come after operator-injected vars
	if names[len(names)-1] != "MY_VAR" {
		t.Errorf("user-defined MY_VAR should be last, got %v", names)
	}
	if len(main.EnvFrom) != 1 || main.EnvFrom[0].SecretRef.Name != "api-keys" {
		t.Error("envFrom not passed through")
	}
}

// ---------------------------------------------------------------------------
// service.go tests
// ---------------------------------------------------------------------------

func TestBuildService_Default(t *testing.T) {
	instance := newTestInstance("svc-test")
	svc := BuildService(instance)

	if svc.Name != "svc-test" {
		t.Errorf("service name = %q, want %q", svc.Name, "svc-test")
	}
	if svc.Namespace != "test-ns" {
		t.Errorf("service namespace = %q, want %q", svc.Namespace, "test-ns")
	}
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Errorf("service type = %v, want ClusterIP", svc.Spec.Type)
	}

	// Labels
	if svc.Labels["app.kubernetes.io/name"] != "openclaw" {
		t.Error("service missing app label")
	}

	// Selector
	sel := svc.Spec.Selector
	if sel["app.kubernetes.io/name"] != "openclaw" || sel["app.kubernetes.io/instance"] != "svc-test" {
		t.Error("service selector does not match expected values")
	}

	// Ports - should have gateway, canvas, and metrics (metrics enabled by default)
	if len(svc.Spec.Ports) != 3 {
		t.Fatalf("expected 3 ports, got %d", len(svc.Spec.Ports))
	}

	assertServicePortWithTarget(t, svc.Spec.Ports, "gateway", int32(GatewayPort), int32(GatewayProxyPort))
	assertServicePortWithTarget(t, svc.Spec.Ports, "canvas", int32(CanvasPort), int32(CanvasProxyPort))
	assertServicePort(t, svc.Spec.Ports, "metrics", DefaultMetricsPort)
}

func TestBuildService_WithChromium(t *testing.T) {
	instance := newTestInstance("svc-chromium")
	instance.Spec.Chromium.Enabled = true

	svc := BuildService(instance)

	if len(svc.Spec.Ports) != 4 {
		t.Fatalf("expected 4 ports with chromium, got %d", len(svc.Spec.Ports))
	}

	assertServicePortWithTarget(t, svc.Spec.Ports, "gateway", int32(GatewayPort), int32(GatewayProxyPort))
	assertServicePortWithTarget(t, svc.Spec.Ports, "canvas", int32(CanvasPort), int32(CanvasProxyPort))
	assertServicePort(t, svc.Spec.Ports, "chromium", int32(ChromiumPort))
	assertServicePort(t, svc.Spec.Ports, "metrics", DefaultMetricsPort)
}

func TestBuildService_LoadBalancer(t *testing.T) {
	instance := newTestInstance("svc-lb")
	instance.Spec.Networking.Service.Type = corev1.ServiceTypeLoadBalancer

	svc := BuildService(instance)

	if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
		t.Errorf("service type = %v, want LoadBalancer", svc.Spec.Type)
	}
}

func TestBuildService_NodePort(t *testing.T) {
	instance := newTestInstance("svc-np")
	instance.Spec.Networking.Service.Type = corev1.ServiceTypeNodePort

	svc := BuildService(instance)

	if svc.Spec.Type != corev1.ServiceTypeNodePort {
		t.Errorf("service type = %v, want NodePort", svc.Spec.Type)
	}
}

func TestBuildService_CustomAnnotations(t *testing.T) {
	instance := newTestInstance("svc-ann")
	instance.Spec.Networking.Service.Annotations = map[string]string{
		"service.beta.kubernetes.io/aws-load-balancer-type": "nlb",
	}

	svc := BuildService(instance)

	if svc.Annotations["service.beta.kubernetes.io/aws-load-balancer-type"] != "nlb" {
		t.Error("service annotations not applied")
	}
}

func TestBuildService_CustomPorts(t *testing.T) {
	instance := newTestInstance("svc-custom-ports")
	instance.Spec.Networking.Service.Ports = []openclawv1alpha1.ServicePortSpec{
		{
			Name: "http",
			Port: 3978,
		},
	}

	svc := BuildService(instance)

	if len(svc.Spec.Ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(svc.Spec.Ports))
	}
	assertServicePort(t, svc.Spec.Ports, "http", 3978)
}

func TestBuildService_CustomPortsWithTargetPort(t *testing.T) {
	instance := newTestInstance("svc-custom-tp")
	instance.Spec.Networking.Service.Ports = []openclawv1alpha1.ServicePortSpec{
		{
			Name:       "http",
			Port:       80,
			TargetPort: Ptr(int32(3978)),
		},
	}

	svc := BuildService(instance)

	if len(svc.Spec.Ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(svc.Spec.Ports))
	}
	if svc.Spec.Ports[0].Port != 80 {
		t.Errorf("port = %d, want 80", svc.Spec.Ports[0].Port)
	}
	if svc.Spec.Ports[0].TargetPort.IntValue() != 3978 {
		t.Errorf("targetPort = %d, want 3978", svc.Spec.Ports[0].TargetPort.IntValue())
	}
}

func TestBuildService_CustomPortsMultiple(t *testing.T) {
	instance := newTestInstance("svc-multi-ports")
	instance.Spec.Networking.Service.Ports = []openclawv1alpha1.ServicePortSpec{
		{
			Name: "http",
			Port: 3978,
		},
		{
			Name:       "grpc",
			Port:       50051,
			TargetPort: Ptr(int32(50051)),
			Protocol:   corev1.ProtocolTCP,
		},
	}

	svc := BuildService(instance)

	if len(svc.Spec.Ports) != 2 {
		t.Fatalf("expected 2 ports, got %d", len(svc.Spec.Ports))
	}
	assertServicePort(t, svc.Spec.Ports, "http", 3978)
	assertServicePort(t, svc.Spec.Ports, "grpc", 50051)
}

func TestBuildService_CustomPortsOverrideDefaults(t *testing.T) {
	instance := newTestInstance("svc-override")
	instance.Spec.Networking.Service.Ports = []openclawv1alpha1.ServicePortSpec{
		{
			Name: "http",
			Port: 8080,
		},
	}
	instance.Spec.Chromium.Enabled = true

	svc := BuildService(instance)

	if len(svc.Spec.Ports) != 1 {
		t.Fatalf("custom ports should replace defaults including chromium; got %d ports", len(svc.Spec.Ports))
	}
	assertServicePort(t, svc.Spec.Ports, "http", 8080)
}

func TestBuildService_CustomPortsDefaultProtocol(t *testing.T) {
	instance := newTestInstance("svc-proto")
	instance.Spec.Networking.Service.Ports = []openclawv1alpha1.ServicePortSpec{
		{
			Name: "http",
			Port: 8080,
		},
	}

	svc := BuildService(instance)

	if svc.Spec.Ports[0].Protocol != corev1.ProtocolTCP {
		t.Errorf("protocol = %v, want TCP", svc.Spec.Ports[0].Protocol)
	}
}

func TestBuildService_CustomPortsTargetPortDefaultsToPort(t *testing.T) {
	instance := newTestInstance("svc-tp-default")
	instance.Spec.Networking.Service.Ports = []openclawv1alpha1.ServicePortSpec{
		{
			Name: "http",
			Port: 3978,
		},
	}

	svc := BuildService(instance)

	if svc.Spec.Ports[0].TargetPort.IntValue() != 3978 {
		t.Errorf("targetPort = %d, want 3978 (same as port)", svc.Spec.Ports[0].TargetPort.IntValue())
	}
}

// ---------------------------------------------------------------------------
// networkpolicy.go tests
// ---------------------------------------------------------------------------

func TestBuildNetworkPolicy_Default(t *testing.T) {
	instance := newTestInstance("np-test")
	np := BuildNetworkPolicy(instance)

	if np.Name != "np-test" {
		t.Errorf("network policy name = %q, want %q", np.Name, "np-test")
	}
	if np.Namespace != "test-ns" {
		t.Errorf("network policy namespace = %q, want %q", np.Namespace, "test-ns")
	}

	// Labels
	if np.Labels["app.kubernetes.io/name"] != "openclaw" {
		t.Error("network policy missing app label")
	}

	// Pod selector
	sel := np.Spec.PodSelector.MatchLabels
	if sel["app.kubernetes.io/name"] != "openclaw" || sel["app.kubernetes.io/instance"] != "np-test" {
		t.Error("pod selector does not match expected values")
	}

	// Policy types
	if len(np.Spec.PolicyTypes) != 2 {
		t.Fatalf("expected 2 policy types, got %d", len(np.Spec.PolicyTypes))
	}

	// Ingress rules - by default, allow from same namespace
	if len(np.Spec.Ingress) < 1 {
		t.Fatal("expected at least 1 ingress rule")
	}
	firstIngress := np.Spec.Ingress[0]
	if len(firstIngress.From) != 1 {
		t.Fatalf("expected 1 peer in first ingress rule, got %d", len(firstIngress.From))
	}
	nsSel := firstIngress.From[0].NamespaceSelector
	if nsSel == nil {
		t.Fatal("first ingress rule should have namespace selector")
	}
	if nsSel.MatchLabels["kubernetes.io/metadata.name"] != "test-ns" {
		t.Errorf("ingress namespace selector = %v, want test-ns", nsSel.MatchLabels)
	}

	// Ingress ports - gateway proxy, canvas proxy, and metrics (enabled by default)
	if len(firstIngress.Ports) != 3 {
		t.Fatalf("expected 3 ingress ports, got %d", len(firstIngress.Ports))
	}
	assertNPPort(t, firstIngress.Ports, GatewayProxyPort)
	assertNPPort(t, firstIngress.Ports, CanvasProxyPort)
	assertNPPort(t, firstIngress.Ports, int(DefaultMetricsPort))

	// Egress rules - DNS (UDP+TCP 53) and HTTPS (443)
	if len(np.Spec.Egress) < 2 {
		t.Fatalf("expected at least 2 egress rules (DNS + HTTPS), got %d", len(np.Spec.Egress))
	}

	// First egress: DNS
	dnsRule := np.Spec.Egress[0]
	if len(dnsRule.Ports) != 2 {
		t.Fatalf("DNS egress rule should have 2 ports (UDP+TCP), got %d", len(dnsRule.Ports))
	}
	foundUDP53 := false
	foundTCP53 := false
	for _, p := range dnsRule.Ports {
		if p.Port != nil && p.Port.IntValue() == 53 {
			if p.Protocol != nil && *p.Protocol == corev1.ProtocolUDP {
				foundUDP53 = true
			}
			if p.Protocol != nil && *p.Protocol == corev1.ProtocolTCP {
				foundTCP53 = true
			}
		}
	}
	if !foundUDP53 {
		t.Error("DNS egress rule missing UDP port 53")
	}
	if !foundTCP53 {
		t.Error("DNS egress rule missing TCP port 53")
	}

	// Second egress: HTTPS
	httpsRule := np.Spec.Egress[1]
	if len(httpsRule.Ports) != 1 {
		t.Fatalf("HTTPS egress rule should have 1 port, got %d", len(httpsRule.Ports))
	}
	if httpsRule.Ports[0].Port == nil || httpsRule.Ports[0].Port.IntValue() != 443 {
		t.Error("HTTPS egress rule should allow port 443")
	}
}

func TestBuildNetworkPolicy_CustomServicePorts(t *testing.T) {
	instance := newTestInstance("np-custom-ports")
	instance.Spec.Networking.Service.Ports = []openclawv1alpha1.ServicePortSpec{
		{Name: "http", Port: 3978},
		{Name: "grpc", Port: 50051, Protocol: corev1.ProtocolTCP},
	}

	np := BuildNetworkPolicy(instance)

	// Same-namespace ingress rule should use custom ports + metrics (enabled by default)
	firstIngress := np.Spec.Ingress[0]
	if len(firstIngress.Ports) != 3 {
		t.Fatalf("expected 3 ingress ports for custom service ports + metrics, got %d", len(firstIngress.Ports))
	}
	assertNPPort(t, firstIngress.Ports, 3978)
	assertNPPort(t, firstIngress.Ports, 50051)
	assertNPPort(t, firstIngress.Ports, int(DefaultMetricsPort))
}

func TestBuildNetworkPolicy_CustomServicePortsWithTargetPort(t *testing.T) {
	instance := newTestInstance("np-custom-tp")
	instance.Spec.Networking.Service.Ports = []openclawv1alpha1.ServicePortSpec{
		{Name: "http", Port: 80, TargetPort: Ptr(int32(3978))},
	}

	np := BuildNetworkPolicy(instance)

	firstIngress := np.Spec.Ingress[0]
	if len(firstIngress.Ports) != 2 {
		t.Fatalf("expected 2 ingress ports (custom + metrics), got %d", len(firstIngress.Ports))
	}
	// NetworkPolicy should use targetPort (container port), not service port
	assertNPPort(t, firstIngress.Ports, 3978)
	assertNPPort(t, firstIngress.Ports, int(DefaultMetricsPort))
}

func TestBuildNetworkPolicy_CustomPortsApplyToAllRules(t *testing.T) {
	instance := newTestInstance("np-custom-all")
	instance.Spec.Networking.Service.Ports = []openclawv1alpha1.ServicePortSpec{
		{Name: "http", Port: 8080},
	}
	instance.Spec.Security.NetworkPolicy.AllowedIngressNamespaces = []string{"monitoring"}
	instance.Spec.Security.NetworkPolicy.AllowedIngressCIDRs = []string{"10.0.0.0/8"}

	np := BuildNetworkPolicy(instance)

	// 3 rules: same-ns, monitoring ns, CIDR
	if len(np.Spec.Ingress) != 3 {
		t.Fatalf("expected 3 ingress rules, got %d", len(np.Spec.Ingress))
	}
	for i, rule := range np.Spec.Ingress {
		if len(rule.Ports) != 2 {
			t.Fatalf("rule %d: expected 2 ports (custom + metrics), got %d", i, len(rule.Ports))
		}
		assertNPPort(t, rule.Ports, 8080)
		assertNPPort(t, rule.Ports, int(DefaultMetricsPort))
	}
}

func TestBuildNetworkPolicy_CustomCIDRs(t *testing.T) {
	instance := newTestInstance("np-cidrs")
	instance.Spec.Security.NetworkPolicy.AllowedIngressCIDRs = []string{
		"10.0.0.0/8",
		"192.168.1.0/24",
	}
	instance.Spec.Security.NetworkPolicy.AllowedEgressCIDRs = []string{
		"172.16.0.0/12",
	}

	np := BuildNetworkPolicy(instance)

	// Should have 3 ingress rules: same-ns + 2 CIDRs
	if len(np.Spec.Ingress) != 3 {
		t.Fatalf("expected 3 ingress rules, got %d", len(np.Spec.Ingress))
	}

	// Verify CIDR ingress rules
	cidrRule1 := np.Spec.Ingress[1]
	if cidrRule1.From[0].IPBlock == nil {
		t.Fatal("second ingress rule should have IPBlock")
	}
	if cidrRule1.From[0].IPBlock.CIDR != "10.0.0.0/8" {
		t.Errorf("first CIDR = %q, want %q", cidrRule1.From[0].IPBlock.CIDR, "10.0.0.0/8")
	}

	cidrRule2 := np.Spec.Ingress[2]
	if cidrRule2.From[0].IPBlock.CIDR != "192.168.1.0/24" {
		t.Errorf("second CIDR = %q, want %q", cidrRule2.From[0].IPBlock.CIDR, "192.168.1.0/24")
	}

	// Egress should include the CIDR rule (DNS + HTTPS + 1 custom)
	if len(np.Spec.Egress) != 3 {
		t.Fatalf("expected 3 egress rules, got %d", len(np.Spec.Egress))
	}
	egressCIDR := np.Spec.Egress[2]
	if len(egressCIDR.To) != 1 || egressCIDR.To[0].IPBlock == nil {
		t.Fatal("third egress rule should have IPBlock")
	}
	if egressCIDR.To[0].IPBlock.CIDR != "172.16.0.0/12" {
		t.Errorf("egress CIDR = %q, want %q", egressCIDR.To[0].IPBlock.CIDR, "172.16.0.0/12")
	}
}

func TestBuildNetworkPolicy_DNSDisabled(t *testing.T) {
	instance := newTestInstance("np-no-dns")
	instance.Spec.Security.NetworkPolicy.AllowDNS = Ptr(false)

	np := BuildNetworkPolicy(instance)

	// Without DNS, only HTTPS egress rule
	if len(np.Spec.Egress) != 1 {
		t.Fatalf("expected 1 egress rule (HTTPS only), got %d", len(np.Spec.Egress))
	}

	// The single rule should be HTTPS (443)
	httpsRule := np.Spec.Egress[0]
	if len(httpsRule.Ports) != 1 || httpsRule.Ports[0].Port.IntValue() != 443 {
		t.Error("single egress rule should be HTTPS port 443")
	}
}

func TestBuildNetworkPolicy_AllowedNamespaces(t *testing.T) {
	instance := newTestInstance("np-ns")
	instance.Spec.Security.NetworkPolicy.AllowedIngressNamespaces = []string{
		"ingress-nginx",
		"monitoring",
	}

	np := BuildNetworkPolicy(instance)

	// Should have 3 ingress rules: same-ns + 2 allowed namespaces
	if len(np.Spec.Ingress) != 3 {
		t.Fatalf("expected 3 ingress rules, got %d", len(np.Spec.Ingress))
	}

	nsRule1 := np.Spec.Ingress[1]
	if nsRule1.From[0].NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] != "ingress-nginx" {
		t.Error("second ingress rule should allow ingress-nginx namespace")
	}
	nsRule2 := np.Spec.Ingress[2]
	if nsRule2.From[0].NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] != "monitoring" {
		t.Error("third ingress rule should allow monitoring namespace")
	}
}

func TestBuildNetworkPolicy_AdditionalEgress(t *testing.T) {
	instance := newTestInstance("np-extra-egress")
	instance.Spec.Security.NetworkPolicy.AdditionalEgress = []networkingv1.NetworkPolicyEgressRule{
		{
			To: []networkingv1.NetworkPolicyPeer{
				{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							"kubernetes.io/metadata.name": "bifrost",
						},
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: Ptr(corev1.ProtocolTCP),
					Port:     Ptr(intstr.FromInt(8080)),
				},
			},
		},
		{
			To: []networkingv1.NetworkPolicyPeer{
				{
					IPBlock: &networkingv1.IPBlock{
						CIDR: "10.96.0.0/16",
					},
				},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: Ptr(corev1.ProtocolTCP),
					Port:     Ptr(intstr.FromInt(9090)),
				},
			},
		},
	}

	np := BuildNetworkPolicy(instance)

	// Default rules: DNS (index 0) + HTTPS (index 1) = 2, plus 2 additional = 4
	if len(np.Spec.Egress) != 4 {
		t.Fatalf("expected 4 egress rules (DNS + HTTPS + 2 additional), got %d", len(np.Spec.Egress))
	}

	// First two rules should be the defaults (DNS + HTTPS)
	dnsRule := np.Spec.Egress[0]
	if len(dnsRule.Ports) != 2 || dnsRule.Ports[0].Port.IntValue() != 53 {
		t.Error("first egress rule should be DNS (port 53)")
	}
	httpsRule := np.Spec.Egress[1]
	if len(httpsRule.Ports) != 1 || httpsRule.Ports[0].Port.IntValue() != 443 {
		t.Error("second egress rule should be HTTPS (port 443)")
	}

	// Third rule: bifrost namespace on port 8080
	bifrostRule := np.Spec.Egress[2]
	if len(bifrostRule.To) != 1 || bifrostRule.To[0].NamespaceSelector == nil {
		t.Fatal("third egress rule should have a namespace selector")
	}
	if bifrostRule.To[0].NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] != "bifrost" {
		t.Error("third egress rule should target bifrost namespace")
	}
	if len(bifrostRule.Ports) != 1 || bifrostRule.Ports[0].Port.IntValue() != 8080 {
		t.Error("third egress rule should allow port 8080")
	}

	// Fourth rule: CIDR 10.96.0.0/16 on port 9090
	cidrRule := np.Spec.Egress[3]
	if len(cidrRule.To) != 1 || cidrRule.To[0].IPBlock == nil {
		t.Fatal("fourth egress rule should have an IPBlock")
	}
	if cidrRule.To[0].IPBlock.CIDR != "10.96.0.0/16" {
		t.Errorf("fourth egress CIDR = %q, want %q", cidrRule.To[0].IPBlock.CIDR, "10.96.0.0/16")
	}
	if len(cidrRule.Ports) != 1 || cidrRule.Ports[0].Port.IntValue() != 9090 {
		t.Error("fourth egress rule should allow port 9090")
	}
}

// ---------------------------------------------------------------------------
// rbac.go tests
// ---------------------------------------------------------------------------

func TestBuildServiceAccount(t *testing.T) {
	instance := newTestInstance("sa-test")
	sa := BuildServiceAccount(instance)

	if sa.Name != "sa-test" {
		t.Errorf("service account name = %q, want %q", sa.Name, "sa-test")
	}
	if sa.Namespace != "test-ns" {
		t.Errorf("service account namespace = %q, want %q", sa.Namespace, "test-ns")
	}
	if sa.Labels["app.kubernetes.io/name"] != "openclaw" {
		t.Error("service account missing app label")
	}
	if sa.Labels["app.kubernetes.io/instance"] != "sa-test" {
		t.Error("service account missing instance label")
	}
	if sa.Labels["app.kubernetes.io/managed-by"] != "openclaw-operator" {
		t.Error("service account missing managed-by label")
	}
}

func TestBuildServiceAccount_AutomountDisabled(t *testing.T) {
	instance := newTestInstance("sa-automount")
	sa := BuildServiceAccount(instance)
	if sa.AutomountServiceAccountToken == nil || *sa.AutomountServiceAccountToken != false {
		t.Errorf("AutomountServiceAccountToken = %v, want false", sa.AutomountServiceAccountToken)
	}
}

func TestBuildServiceAccount_CustomName(t *testing.T) {
	instance := newTestInstance("sa-custom")
	instance.Spec.Security.RBAC.ServiceAccountName = "my-custom-sa"

	sa := BuildServiceAccount(instance)

	if sa.Name != "my-custom-sa" {
		t.Errorf("service account name = %q, want %q", sa.Name, "my-custom-sa")
	}
}

func TestBuildRole_Default(t *testing.T) {
	instance := newTestInstance("role-test")
	role := BuildRole(instance)

	if role.Name != "role-test" {
		t.Errorf("role name = %q, want %q", role.Name, "role-test")
	}
	if role.Namespace != "test-ns" {
		t.Errorf("role namespace = %q, want %q", role.Namespace, "test-ns")
	}

	// Should have exactly 1 rule (configmap read)
	if len(role.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(role.Rules))
	}

	rule := role.Rules[0]
	if len(rule.APIGroups) != 1 || rule.APIGroups[0] != "" {
		t.Error("base rule should have core API group")
	}
	if len(rule.Resources) != 1 || rule.Resources[0] != "configmaps" {
		t.Errorf("base rule resources = %v, want [configmaps]", rule.Resources)
	}
	if len(rule.ResourceNames) != 1 || rule.ResourceNames[0] != "role-test-config" {
		t.Errorf("base rule resourceNames = %v, want [role-test-config]", rule.ResourceNames)
	}
	if len(rule.Verbs) != 2 {
		t.Fatalf("expected 2 verbs, got %d", len(rule.Verbs))
	}
	expectedVerbs := map[string]bool{"get": true, "watch": true}
	for _, v := range rule.Verbs {
		if !expectedVerbs[v] {
			t.Errorf("unexpected verb %q", v)
		}
	}
}

func TestBuildRole_AdditionalRules(t *testing.T) {
	instance := newTestInstance("role-extra")
	instance.Spec.Security.RBAC.AdditionalRules = []openclawv1alpha1.RBACRule{
		{
			APIGroups: []string{""},
			Resources: []string{"secrets"},
			Verbs:     []string{"get", "list"},
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{"deployments"},
			Verbs:     []string{"get"},
		},
	}

	role := BuildRole(instance)

	// 1 base rule + 2 additional rules
	if len(role.Rules) != 3 {
		t.Fatalf("expected 3 rules, got %d", len(role.Rules))
	}

	// Verify additional rules
	secondRule := role.Rules[1]
	if secondRule.Resources[0] != "secrets" {
		t.Errorf("second rule resources = %v, want [secrets]", secondRule.Resources)
	}
	thirdRule := role.Rules[2]
	if thirdRule.APIGroups[0] != "apps" || thirdRule.Resources[0] != "deployments" {
		t.Error("third rule does not match expected values")
	}
}

func TestBuildRoleBinding(t *testing.T) {
	instance := newTestInstance("rb-test")
	rb := BuildRoleBinding(instance)

	if rb.Name != "rb-test" {
		t.Errorf("role binding name = %q, want %q", rb.Name, "rb-test")
	}
	if rb.Namespace != "test-ns" {
		t.Errorf("role binding namespace = %q, want %q", rb.Namespace, "test-ns")
	}

	// RoleRef
	if rb.RoleRef.Kind != "Role" {
		t.Errorf("roleRef kind = %q, want %q", rb.RoleRef.Kind, "Role")
	}
	if rb.RoleRef.Name != "rb-test" {
		t.Errorf("roleRef name = %q, want %q", rb.RoleRef.Name, "rb-test")
	}
	if rb.RoleRef.APIGroup != "rbac.authorization.k8s.io" {
		t.Errorf("roleRef apiGroup = %q, want %q", rb.RoleRef.APIGroup, "rbac.authorization.k8s.io")
	}

	// Subjects
	if len(rb.Subjects) != 1 {
		t.Fatalf("expected 1 subject, got %d", len(rb.Subjects))
	}
	subj := rb.Subjects[0]
	if subj.Kind != "ServiceAccount" {
		t.Errorf("subject kind = %q, want ServiceAccount", subj.Kind)
	}
	if subj.Name != "rb-test" {
		t.Errorf("subject name = %q, want %q", subj.Name, "rb-test")
	}
	if subj.Namespace != "test-ns" {
		t.Errorf("subject namespace = %q, want %q", subj.Namespace, "test-ns")
	}
}

func TestBuildRoleBinding_CustomServiceAccount(t *testing.T) {
	instance := newTestInstance("rb-custom-sa")
	instance.Spec.Security.RBAC.ServiceAccountName = "my-sa"

	rb := BuildRoleBinding(instance)

	// Subject should use the custom SA name
	if rb.Subjects[0].Name != "my-sa" {
		t.Errorf("subject name = %q, want %q", rb.Subjects[0].Name, "my-sa")
	}
	// RoleRef should still use instance name
	if rb.RoleRef.Name != "rb-custom-sa" {
		t.Errorf("roleRef name = %q, want %q", rb.RoleRef.Name, "rb-custom-sa")
	}
}

// ---------------------------------------------------------------------------
// configmap.go tests
// ---------------------------------------------------------------------------

func TestBuildConfigMap_Default(t *testing.T) {
	instance := newTestInstance("cm-test")
	cm := BuildConfigMap(instance, "", nil)

	if cm.Name != "cm-test-config" {
		t.Errorf("configmap name = %q, want %q", cm.Name, "cm-test-config")
	}
	if cm.Namespace != "test-ns" {
		t.Errorf("configmap namespace = %q, want %q", cm.Namespace, "test-ns")
	}
	if cm.Labels["app.kubernetes.io/name"] != "openclaw" {
		t.Error("configmap missing app label")
	}

	// Default config should have gateway.bind=lan injected
	content, ok := cm.Data["openclaw.json"]
	if !ok {
		t.Fatal("configmap missing openclaw.json key")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse default config: %v", err)
	}
	gw, ok := parsed["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway key in default config")
	}
	if gw["bind"] != "loopback" {
		t.Errorf("gateway.bind = %v, want %q", gw["bind"], "loopback")
	}
}

func TestBuildConfigMap_RawConfig(t *testing.T) {
	instance := newTestInstance("cm-raw")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"mcpServers":{"test":{"url":"http://localhost:3000"}}}`),
		},
	}

	cm := BuildConfigMap(instance, "", nil)

	content, ok := cm.Data["openclaw.json"]
	if !ok {
		t.Fatal("configmap missing openclaw.json key")
	}

	// The builder pretty-prints JSON, so check it contains the expected keys
	if content == "{}" {
		t.Error("config content should not be empty with raw config")
	}

	// Verify the content is valid JSON and contains expected data
	if content == "" {
		t.Error("config content should not be empty")
	}
}

func TestBuildConfigMap_InvalidJSON_RawPreserved(t *testing.T) {
	instance := newTestInstance("cm-invalid")
	// If raw JSON is technically valid but the builder tries to pretty-print,
	// verify it handles valid JSON correctly and gateway.bind is injected
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"key":"value"}`),
		},
	}

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}
	if parsed["key"] != "value" {
		t.Errorf("key = %v, want %q", parsed["key"], "value")
	}
	gw, ok := parsed["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway key after enrichment")
	}
	if gw["bind"] != "loopback" {
		t.Errorf("gateway.bind = %v, want %q", gw["bind"], "loopback")
	}
}

// TestBuildConfigMap_InjectsGatewayMode verifies the operator sets
// gateway.mode=local on a default instance. Upstream openclaw >= v2026.5.18
// refuses to start without this field.
func TestBuildConfigMap_InjectsGatewayMode(t *testing.T) {
	instance := newTestInstance("cm-gateway-mode")

	cm := BuildConfigMap(instance, "tok", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}
	gw, ok := parsed["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway key after enrichment")
	}
	if gw["mode"] != "local" {
		t.Errorf("gateway.mode = %v, want %q", gw["mode"], "local")
	}
}

// TestBuildConfigMap_PreservesUserGatewayMode verifies the operator does not
// overwrite a gateway.mode that the user has explicitly set.
func TestBuildConfigMap_PreservesUserGatewayMode(t *testing.T) {
	instance := newTestInstance("cm-gateway-mode-user")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"gateway":{"mode":"remote"}}`),
		},
	}

	cm := BuildConfigMap(instance, "tok", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}
	gw, ok := parsed["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway key after enrichment")
	}
	if gw["mode"] != "remote" {
		t.Errorf("gateway.mode = %v, want %q (user override should win)", gw["mode"], "remote")
	}
}

// ---------------------------------------------------------------------------
// BuildConfigMapFromBytes tests
// ---------------------------------------------------------------------------

func TestBuildConfigMapFromBytes_EnrichesExternalConfig(t *testing.T) {
	instance := newTestInstance("from-bytes")
	externalConfig := []byte(`{"mcpServers":{"test":{"url":"http://localhost"}}}`)

	cm := BuildConfigMapFromBytes(instance, externalConfig, "my-gateway-token", nil)

	content := cm.Data["openclaw.json"]
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	// Verify user config is preserved
	if _, ok := parsed["mcpServers"]; !ok {
		t.Error("mcpServers should be preserved from external config")
	}

	// Verify gateway auth was injected
	gw, ok := parsed["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway key after enrichment")
	}
	auth, ok := gw["auth"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway.auth key after enrichment")
	}
	if auth["mode"] != "token" {
		t.Errorf("gateway.auth.mode = %v, want %q", auth["mode"], "token")
	}
	if auth["token"] != "my-gateway-token" {
		t.Errorf("gateway.auth.token = %v, want %q", auth["token"], "my-gateway-token")
	}

	// Verify gateway.bind was injected
	if gw["bind"] != "loopback" {
		t.Errorf("gateway.bind = %v, want %q", gw["bind"], "loopback")
	}

	// Verify device auth was injected
	controlUI, ok := gw["controlUi"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway.controlUi key after enrichment")
	}
	if controlUI["dangerouslyDisableDeviceAuth"] != true {
		t.Errorf("gateway.controlUi.dangerouslyDisableDeviceAuth = %v, want true", controlUI["dangerouslyDisableDeviceAuth"])
	}
}

func TestBuildConfigMapFromBytes_PreservesTrustedProxyMode(t *testing.T) {
	instance := newTestInstance("trusted-proxy")
	externalConfig := []byte(`{"gateway":{"auth":{"mode":"trusted-proxy"}},"mcpServers":{"test":{"url":"http://localhost"}}}`)

	cm := BuildConfigMapFromBytes(instance, externalConfig, "my-gateway-token", nil)

	content := cm.Data["openclaw.json"]
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	// User config should be preserved
	if _, ok := parsed["mcpServers"]; !ok {
		t.Error("mcpServers should be preserved from external config")
	}

	// Gateway auth mode should be preserved, token must NOT be injected
	// (trusted-proxy mode is mutually exclusive with token auth)
	gw, ok := parsed["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway key after enrichment")
	}
	auth, ok := gw["auth"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway.auth key after enrichment")
	}
	if auth["mode"] != "trusted-proxy" {
		t.Errorf("gateway.auth.mode = %v, want %q (user's mode should be preserved)", auth["mode"], "trusted-proxy")
	}
	if _, hasToken := auth["token"]; hasToken {
		t.Errorf("gateway.auth.token should not be set in trusted-proxy mode, got %v", auth["token"])
	}
}

func TestBuildConfigMapFromBytes_PreservesUserConfig(t *testing.T) {
	instance := newTestInstance("from-bytes-preserve")
	externalConfig := []byte(`{
		"mcpServers": {"fetch": {"url": "http://localhost:3000"}},
		"globalShortcut": "Ctrl+Space",
		"selectedProvider": "anthropic"
	}`)

	cm := BuildConfigMapFromBytes(instance, externalConfig, "tok", nil)

	content := cm.Data["openclaw.json"]
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if parsed["globalShortcut"] != "Ctrl+Space" {
		t.Errorf("globalShortcut = %v, want %q", parsed["globalShortcut"], "Ctrl+Space")
	}
	if parsed["selectedProvider"] != "anthropic" {
		t.Errorf("selectedProvider = %v, want %q", parsed["selectedProvider"], "anthropic")
	}
	if _, ok := parsed["mcpServers"]; !ok {
		t.Error("mcpServers should be preserved from external config")
	}
}

func TestBuildConfigMapFromBytes_EmptyBytes(t *testing.T) {
	instance := newTestInstance("from-bytes-empty")

	cm := BuildConfigMapFromBytes(instance, nil, "tok", nil)

	content := cm.Data["openclaw.json"]
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	// Should still have gateway enrichment
	gw, ok := parsed["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway key even with empty base config")
	}
	if gw["bind"] != "loopback" {
		t.Errorf("gateway.bind = %v, want %q", gw["bind"], "loopback")
	}
}

func TestBuildConfigMapFromBytes_JSON5Passthrough(t *testing.T) {
	instance := newTestInstance("from-bytes-json5")
	// JSON5 content can't be parsed as JSON, so enrichment returns it unchanged
	json5Content := []byte(`{mcpServers: {test: {url: "http://localhost"}}}`)

	cm := BuildConfigMapFromBytes(instance, json5Content, "tok", nil)

	// JSON5 content should pass through unchanged (enrichment can't parse it)
	content := cm.Data["openclaw.json"]
	if content != string(json5Content) {
		t.Errorf("JSON5 content should pass through unchanged, got %q", content)
	}
}

// ---------------------------------------------------------------------------
// OTel metrics config injection tests (#356, #373)
// The operator injects diagnostics.otel (NOT diagnostics.metrics) and adds
// an OTel Collector sidecar that exposes a Prometheus scrape endpoint.
// ---------------------------------------------------------------------------

func TestBuildConfigMap_NoDiagnosticsMetricsInjected(t *testing.T) {
	instance := newTestInstance("cm-no-metrics-inject")
	cm := BuildConfigMap(instance, "", nil)

	content := cm.Data["openclaw.json"]
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	diag, ok := parsed["diagnostics"].(map[string]interface{})
	if ok {
		if _, hasMetrics := diag["metrics"]; hasMetrics {
			t.Error("diagnostics.metrics must not be injected - OpenClaw rejects this key")
		}
	}
}

func TestEnrichConfigWithOTelMetrics(t *testing.T) {
	input := []byte(`{}`)
	out, err := enrichConfigWithOTelMetrics(input)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	diag, ok := cfg["diagnostics"].(map[string]interface{})
	if !ok {
		t.Fatal("expected diagnostics key")
	}
	otel, ok := diag["otel"].(map[string]interface{})
	if !ok {
		t.Fatal("expected diagnostics.otel key")
	}
	if otel["metrics"] != true {
		t.Errorf("diagnostics.otel.metrics = %v, want true", otel["metrics"])
	}
	expectedEndpoint := fmt.Sprintf("http://localhost:%d", OTelHTTPReceiverPort)
	if otel["endpoint"] != expectedEndpoint {
		t.Errorf("diagnostics.otel.endpoint = %v, want %s", otel["endpoint"], expectedEndpoint)
	}
}

func TestEnrichConfigWithOTelMetrics_PreservesUserOverride(t *testing.T) {
	input := []byte(`{"diagnostics":{"otel":{"metrics":true,"endpoint":"http://my-collector:4318"}}}`)
	out, err := enrichConfigWithOTelMetrics(input)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	diag := cfg["diagnostics"].(map[string]interface{})
	otel := diag["otel"].(map[string]interface{})
	if otel["endpoint"] != "http://my-collector:4318" {
		t.Errorf("user-set endpoint should be preserved, got %v", otel["endpoint"])
	}
}

func TestEnrichConfigWithOTelMetrics_DisabledNoInjection(t *testing.T) {
	instance := newTestInstance("cm-metrics-disabled")
	disabled := false
	instance.Spec.Observability.Metrics.Enabled = &disabled
	cm := BuildConfigMap(instance, "", nil)

	content := cm.Data["openclaw.json"]
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	if diag, ok := parsed["diagnostics"].(map[string]interface{}); ok {
		if _, hasOTel := diag["otel"]; hasOTel {
			t.Error("diagnostics.otel should not be injected when metrics.enabled=false")
		}
	}
}

func TestBuildConfigMap_OTelCollectorConfig(t *testing.T) {
	instance := newTestInstance("cm-otel-config")
	cm := BuildConfigMap(instance, "", nil)

	config, ok := cm.Data[OTelCollectorConfigKey]
	if !ok {
		t.Fatal("ConfigMap should have OTel Collector config key")
	}
	if !strings.Contains(config, "otlp:") {
		t.Error("OTel config should contain OTLP receiver")
	}
	if !strings.Contains(config, "prometheus:") {
		t.Error("OTel Collector config should contain prometheus exporter")
	}
	if !strings.Contains(config, "metric_expiration: 8760h") {
		t.Error("OTel config should contain Prometheus exporter")
	}
	if !strings.Contains(config, fmt.Sprintf("0.0.0.0:%d", DefaultMetricsPort)) {
		t.Errorf("OTel config should contain Prometheus exporter on port %d", DefaultMetricsPort)
	}
}

func TestBuildConfigMap_MetricsDisabled_NoOTelConfig(t *testing.T) {
	instance := newTestInstance("cm-no-otel-config")
	disabled := false
	instance.Spec.Observability.Metrics.Enabled = &disabled
	cm := BuildConfigMap(instance, "", nil)

	if _, ok := cm.Data[OTelCollectorConfigKey]; ok {
		t.Error("OTel Collector config should not be present when metrics disabled")
	}
}

func TestBuildStatefulSet_OTelCollectorContainer(t *testing.T) {
	instance := newTestInstance("sts-otel-collector")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var found bool
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name != "otel-collector" {
			continue
		}
		found = true
		// Verify metrics port is on the collector
		assertContainerPort(t, c.Ports, "metrics", DefaultMetricsPort)
		// Verify config volume mount (directory mount, no SubPath)
		var hasConfigMount bool
		for _, vm := range c.VolumeMounts {
			if vm.Name == "otel-collector-config" && vm.MountPath == "/etc/otel-collector" {
				hasConfigMount = true
			}
		}
		if !hasConfigMount {
			t.Error("otel-collector should mount OTel Collector config")
		}
		// Verify security context
		if !*c.SecurityContext.ReadOnlyRootFilesystem {
			t.Error("otel-collector should have read-only rootfs")
		}
		if !*c.SecurityContext.RunAsNonRoot {
			t.Error("otel-collector should run as non-root")
		}
	}
	if !found {
		t.Error("StatefulSet should have otel-collector container when metrics enabled")
	}
}

func TestBuildStatefulSet_MetricsDisabled_NoOTelCollector(t *testing.T) {
	instance := newTestInstance("sts-no-otel-collector")
	disabled := false
	instance.Spec.Observability.Metrics.Enabled = &disabled
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "otel-collector" {
			t.Error("otel-collector should not be present when metrics disabled")
		}
	}
}

func TestBuildStatefulSet_MainContainerNoMetricsPort(t *testing.T) {
	instance := newTestInstance("sts-main-no-metrics")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	main := sts.Spec.Template.Spec.Containers[0]
	for _, p := range main.Ports {
		if p.Name == "metrics" {
			t.Error("main container should not have metrics port (it belongs on otel-collector)")
		}
	}
}

// ---------------------------------------------------------------------------
// enrichConfigWithGatewayBind tests
// ---------------------------------------------------------------------------

func TestEnrichConfigWithGatewayBind(t *testing.T) {
	input := []byte(`{}`)
	instance := newTestInstance("bind-test")
	out, err := enrichConfigWithGatewayBind(input, instance)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	gw, ok := cfg["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway key")
	}
	if gw["bind"] != "loopback" {
		t.Errorf("gateway.bind = %v, want %q", gw["bind"], "loopback")
	}
}

func TestEnrichConfigWithGatewayBind_PreservesUserBind(t *testing.T) {
	input := []byte(`{"gateway":{"bind":"0.0.0.0"}}`)
	instance := newTestInstance("bind-user-override")
	out, err := enrichConfigWithGatewayBind(input, instance)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	gw := cfg["gateway"].(map[string]interface{})
	if gw["bind"] != "0.0.0.0" {
		t.Errorf("gateway.bind = %v, want %q (user override)", gw["bind"], "0.0.0.0")
	}
}

func TestEnrichConfigWithGatewayBind_PreservesOtherFields(t *testing.T) {
	input := []byte(`{"gateway":{"auth":{"mode":"token","token":"secret"}}}`)
	instance := newTestInstance("bind-other-fields")
	out, err := enrichConfigWithGatewayBind(input, instance)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	gw := cfg["gateway"].(map[string]interface{})
	if gw["bind"] != "loopback" {
		t.Errorf("gateway.bind = %v, want %q", gw["bind"], "loopback")
	}
	auth, ok := gw["auth"].(map[string]interface{})
	if !ok {
		t.Fatal("gateway.auth should be preserved")
	}
	if auth["token"] != "secret" {
		t.Errorf("gateway.auth.token = %v, want %q", auth["token"], "secret")
	}
}

func TestEnrichConfigWithGatewayBind_InvalidJSON(t *testing.T) {
	input := []byte(`not valid json`)
	instance := newTestInstance("bind-invalid-json")
	out, err := enrichConfigWithGatewayBind(input, instance)
	if err != nil {
		t.Fatal("should not error on invalid JSON")
	}

	if !bytes.Equal(out, input) {
		t.Errorf("invalid JSON should be returned unchanged")
	}
}

// ---------------------------------------------------------------------------
// enrichConfigWithDeviceAuth tests
// ---------------------------------------------------------------------------

func TestEnrichConfigWithDeviceAuth(t *testing.T) {
	input := []byte(`{}`)
	out, err := enrichConfigWithDeviceAuth(input)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	gw, ok := cfg["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway key")
	}
	controlUI, ok := gw["controlUi"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway.controlUi key")
	}
	if controlUI["dangerouslyDisableDeviceAuth"] != true {
		t.Errorf("gateway.controlUi.dangerouslyDisableDeviceAuth = %v, want true", controlUI["dangerouslyDisableDeviceAuth"])
	}
}

func TestEnrichConfigWithDeviceAuth_PreservesUserOverride(t *testing.T) {
	input := []byte(`{"gateway":{"controlUi":{"dangerouslyDisableDeviceAuth":false}}}`)
	out, err := enrichConfigWithDeviceAuth(input)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	gw := cfg["gateway"].(map[string]interface{})
	controlUI := gw["controlUi"].(map[string]interface{})
	if controlUI["dangerouslyDisableDeviceAuth"] != false {
		t.Errorf("gateway.controlUi.dangerouslyDisableDeviceAuth = %v, want false (user override)", controlUI["dangerouslyDisableDeviceAuth"])
	}
}

func TestEnrichConfigWithDeviceAuth_PreservesOtherFields(t *testing.T) {
	input := []byte(`{"gateway":{"auth":{"mode":"token","token":"secret"}}}`)
	out, err := enrichConfigWithDeviceAuth(input)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	gw := cfg["gateway"].(map[string]interface{})
	controlUI, ok := gw["controlUi"].(map[string]interface{})
	if !ok {
		t.Fatal("gateway.controlUi should be created")
	}
	if controlUI["dangerouslyDisableDeviceAuth"] != true {
		t.Errorf("gateway.controlUi.dangerouslyDisableDeviceAuth = %v, want true", controlUI["dangerouslyDisableDeviceAuth"])
	}
	auth, ok := gw["auth"].(map[string]interface{})
	if !ok {
		t.Fatal("gateway.auth should be preserved")
	}
	if auth["token"] != "secret" {
		t.Errorf("gateway.auth.token = %v, want %q", auth["token"], "secret")
	}
}

func TestEnrichConfigWithDeviceAuth_InvalidJSON(t *testing.T) {
	input := []byte(`not valid json`)
	out, err := enrichConfigWithDeviceAuth(input)
	if err != nil {
		t.Fatal("should not error on invalid JSON")
	}

	if !bytes.Equal(out, input) {
		t.Errorf("invalid JSON should be returned unchanged")
	}
}

// ---------------------------------------------------------------------------
// Handshake timeout env var tests
// ---------------------------------------------------------------------------

func TestHandshakeTimeoutEnvVar(t *testing.T) {
	instance := newTestInstance("handshake-test")
	env := buildMainEnv(instance, "test-token-secret")
	want := fmt.Sprintf("%d", DefaultHandshakeTimeoutMs)

	var found bool
	for _, e := range env {
		if e.Name == "OPENCLAW_GATEWAY_HANDSHAKE_TIMEOUT_MS" {
			found = true
			if e.Value != want {
				t.Errorf("OPENCLAW_GATEWAY_HANDSHAKE_TIMEOUT_MS = %q, want %q", e.Value, want)
			}
			break
		}
	}
	if !found {
		t.Error("OPENCLAW_GATEWAY_HANDSHAKE_TIMEOUT_MS env var not found")
	}
}

func TestHandshakeTimeoutEnvVar_UserOverrideWins(t *testing.T) {
	instance := newTestInstance("handshake-override")
	instance.Spec.Env = []corev1.EnvVar{
		{Name: "OPENCLAW_GATEWAY_HANDSHAKE_TIMEOUT_MS", Value: "5000"},
	}
	sts := BuildStatefulSet(instance, "test-token-secret", nil, nil, nil)
	mainContainer := sts.Spec.Template.Spec.Containers[0]

	var count int
	var lastValue string
	for _, e := range mainContainer.Env {
		if e.Name == "OPENCLAW_GATEWAY_HANDSHAKE_TIMEOUT_MS" {
			count++
			lastValue = e.Value
		}
	}
	// User env vars are appended after operator defaults; K8s uses the
	// last value when duplicates exist, so the user's value wins.
	if lastValue != "5000" {
		t.Errorf("last OPENCLAW_GATEWAY_HANDSHAKE_TIMEOUT_MS = %q, want 5000 (user override)", lastValue)
	}
	if count < 1 {
		t.Error("OPENCLAW_GATEWAY_HANDSHAKE_TIMEOUT_MS env var not found")
	}
}

// ---------------------------------------------------------------------------
// enrichConfigWithTrustedProxies tests
// ---------------------------------------------------------------------------

func TestEnrichConfigWithTrustedProxies(t *testing.T) {
	input := []byte(`{}`)
	out, err := enrichConfigWithTrustedProxies(input)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	gw, ok := cfg["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway key")
	}
	proxies, ok := gw["trustedProxies"].([]interface{})
	if !ok {
		t.Fatal("expected gateway.trustedProxies array")
	}
	if len(proxies) != 1 || proxies[0] != "127.0.0.0/8" {
		t.Errorf("gateway.trustedProxies = %v, want [127.0.0.0/8]", proxies)
	}
}

func TestEnrichConfigWithTrustedProxies_MergesWithUserEntries(t *testing.T) {
	input := []byte(`{"gateway":{"trustedProxies":["10.0.0.0/8"]}}`)
	out, err := enrichConfigWithTrustedProxies(input)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	gw := cfg["gateway"].(map[string]interface{})
	proxies := gw["trustedProxies"].([]interface{})
	if len(proxies) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(proxies), proxies)
	}
	if proxies[0] != "10.0.0.0/8" {
		t.Errorf("proxies[0] = %v, want 10.0.0.0/8", proxies[0])
	}
	if proxies[1] != "127.0.0.0/8" {
		t.Errorf("proxies[1] = %v, want 127.0.0.0/8", proxies[1])
	}
}

func TestEnrichConfigWithTrustedProxies_SkipsIfAlreadyPresent(t *testing.T) {
	input := []byte(`{"gateway":{"trustedProxies":["127.0.0.0/8","10.0.0.0/8"]}}`)
	out, err := enrichConfigWithTrustedProxies(input)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	gw := cfg["gateway"].(map[string]interface{})
	proxies := gw["trustedProxies"].([]interface{})
	if len(proxies) != 2 {
		t.Errorf("expected 2 entries (no duplicate), got %d: %v", len(proxies), proxies)
	}
}

func TestEnrichConfigWithTrustedProxies_PreservesOtherFields(t *testing.T) {
	input := []byte(`{"gateway":{"bind":"loopback","auth":{"mode":"token"}},"mcpServers":{}}`)
	out, err := enrichConfigWithTrustedProxies(input)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	gw := cfg["gateway"].(map[string]interface{})
	if gw["bind"] != "loopback" {
		t.Errorf("gateway.bind should be preserved, got %v", gw["bind"])
	}
	if _, ok := gw["auth"].(map[string]interface{}); !ok {
		t.Error("gateway.auth should be preserved")
	}
	if _, ok := cfg["mcpServers"]; !ok {
		t.Error("mcpServers should be preserved")
	}
}

func TestEnrichConfigWithTrustedProxies_InvalidJSON(t *testing.T) {
	input := []byte(`not valid json`)
	out, err := enrichConfigWithTrustedProxies(input)
	if err != nil {
		t.Fatal("should not error on invalid JSON")
	}

	if !bytes.Equal(out, input) {
		t.Errorf("invalid JSON should be returned unchanged")
	}
}

func TestBuildConfigMap_TrustedProxiesInjected(t *testing.T) {
	instance := newTestInstance("cm-proxies-inject")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{}`),
		},
	}

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	gw, ok := parsed["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway key")
	}
	proxies, ok := gw["trustedProxies"].([]interface{})
	if !ok {
		t.Fatal("expected gateway.trustedProxies")
	}
	if len(proxies) != 1 || proxies[0] != "127.0.0.0/8" {
		t.Errorf("gateway.trustedProxies = %v, want [127.0.0.0/8]", proxies)
	}
}

func TestBuildConfigMap_TrustedProxiesMergesWithUserConfig(t *testing.T) {
	instance := newTestInstance("cm-proxies-merge")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"gateway":{"trustedProxies":["10.0.0.0/8"]}}`),
		},
	}

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	gw := parsed["gateway"].(map[string]interface{})
	proxies := gw["trustedProxies"].([]interface{})
	if len(proxies) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(proxies), proxies)
	}
	// User entry preserved, loopback appended
	if proxies[0] != "10.0.0.0/8" || proxies[1] != "127.0.0.0/8" {
		t.Errorf("gateway.trustedProxies = %v, want [10.0.0.0/8 127.0.0.0/8]", proxies)
	}
}

func TestBuildConfigMap_RawConfig_GatewayBindInjected(t *testing.T) {
	instance := newTestInstance("cm-bind-inject")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"mcpServers":{"test":{"url":"http://localhost"}}}`),
		},
	}

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	gw, ok := parsed["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway key injected into raw config")
	}
	if gw["bind"] != "loopback" {
		t.Errorf("gateway.bind = %v, want %q", gw["bind"], "loopback")
	}
	// Original data preserved
	if _, ok := parsed["mcpServers"]; !ok {
		t.Error("mcpServers should be preserved from raw config")
	}
}

func TestBuildConfigMap_RawConfig_UserBindPreserved(t *testing.T) {
	instance := newTestInstance("cm-bind-preserve")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"gateway":{"bind":"0.0.0.0"}}`),
		},
	}

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	gw := parsed["gateway"].(map[string]interface{})
	if gw["bind"] != "0.0.0.0" {
		t.Errorf("gateway.bind = %v, want %q (user override should win)", gw["bind"], "0.0.0.0")
	}
}

// ---------------------------------------------------------------------------
// enrichConfigWithControlUIOrigins tests
// ---------------------------------------------------------------------------

func TestEnrichConfigWithControlUIOrigins_InjectsFromIngress(t *testing.T) {
	input := []byte(`{}`)
	instance := newTestInstance("origins-ingress")
	instance.Spec.Networking.Ingress.Hosts = []openclawv1alpha1.IngressHost{
		{Host: "openclaw.example.com"},
	}
	instance.Spec.Networking.Ingress.TLS = []openclawv1alpha1.IngressTLS{
		{Hosts: []string{"openclaw.example.com"}, SecretName: "tls-secret"},
	}

	out, err := enrichConfigWithControlUIOrigins(input, instance)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	gw := cfg["gateway"].(map[string]interface{})
	controlUI := gw["controlUi"].(map[string]interface{})
	origins, ok := controlUI["allowedOrigins"].([]interface{})
	if !ok {
		t.Fatal("expected allowedOrigins array")
	}

	originStrs := make([]string, len(origins))
	for i, o := range origins {
		originStrs[i] = o.(string)
	}

	expected := []string{
		"http://127.0.0.1:18789",
		"http://localhost:18789",
		"https://openclaw.example.com",
	}
	if len(originStrs) != len(expected) {
		t.Fatalf("expected %d origins, got %d: %v", len(expected), len(originStrs), originStrs)
	}
	for i, exp := range expected {
		if originStrs[i] != exp {
			t.Errorf("origin[%d] = %q, want %q", i, originStrs[i], exp)
		}
	}
}

func TestEnrichConfigWithControlUIOrigins_PreservesUserOrigins(t *testing.T) {
	input := []byte(`{"gateway":{"controlUi":{"allowedOrigins":["https://my-proxy.example.com"]}}}`)
	instance := newTestInstance("origins-user-override")
	instance.Spec.Networking.Ingress.Hosts = []openclawv1alpha1.IngressHost{
		{Host: "openclaw.example.com"},
	}

	out, err := enrichConfigWithControlUIOrigins(input, instance)
	if err != nil {
		t.Fatal(err)
	}

	// Should return unchanged because user already set allowedOrigins
	if !bytes.Equal(out, input) {
		t.Error("user-set allowedOrigins should not be overridden")
	}
}

func TestEnrichConfigWithControlUIOrigins_NoIngress(t *testing.T) {
	input := []byte(`{}`)
	instance := newTestInstance("origins-no-ingress")

	out, err := enrichConfigWithControlUIOrigins(input, instance)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	gw := cfg["gateway"].(map[string]interface{})
	controlUI := gw["controlUi"].(map[string]interface{})
	origins := controlUI["allowedOrigins"].([]interface{})

	// Should only have localhost origins
	if len(origins) != 2 {
		t.Fatalf("expected 2 origins (localhost only), got %d: %v", len(origins), origins)
	}
}

func TestEnrichConfigWithControlUIOrigins_CRDExplicitOrigins(t *testing.T) {
	input := []byte(`{}`)
	instance := newTestInstance("origins-crd")
	instance.Spec.Gateway.ControlUIOrigins = []string{
		"https://custom-proxy.example.com",
		"http://internal.corp:8080",
	}

	out, err := enrichConfigWithControlUIOrigins(input, instance)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	gw := cfg["gateway"].(map[string]interface{})
	controlUI := gw["controlUi"].(map[string]interface{})
	origins := controlUI["allowedOrigins"].([]interface{})

	expected := []string{
		"http://127.0.0.1:18789",
		"http://internal.corp:8080",
		"http://localhost:18789",
		"https://custom-proxy.example.com",
	}
	if len(origins) != len(expected) {
		t.Fatalf("expected %d origins, got %d: %v", len(expected), len(origins), origins)
	}
	for i, exp := range expected {
		if origins[i].(string) != exp {
			t.Errorf("origin[%d] = %q, want %q", i, origins[i], exp)
		}
	}
}

func TestEnrichConfigWithControlUIOrigins_Deduplicates(t *testing.T) {
	input := []byte(`{}`)
	instance := newTestInstance("origins-dedup")
	instance.Spec.Networking.Ingress.Hosts = []openclawv1alpha1.IngressHost{
		{Host: "openclaw.example.com"},
	}
	instance.Spec.Networking.Ingress.TLS = []openclawv1alpha1.IngressTLS{
		{Hosts: []string{"openclaw.example.com"}, SecretName: "tls-secret"},
	}
	// Add an explicit origin that duplicates the ingress-derived one
	instance.Spec.Gateway.ControlUIOrigins = []string{
		"https://openclaw.example.com",
		"http://localhost:18789",
	}

	out, err := enrichConfigWithControlUIOrigins(input, instance)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	gw := cfg["gateway"].(map[string]interface{})
	controlUI := gw["controlUi"].(map[string]interface{})
	origins := controlUI["allowedOrigins"].([]interface{})

	// Should have exactly 3: localhost, 127.0.0.1, and the ingress host (no duplicates)
	if len(origins) != 3 {
		t.Fatalf("expected 3 deduplicated origins, got %d: %v", len(origins), origins)
	}
}

func TestEnrichConfigWithControlUIOrigins_InvalidJSON(t *testing.T) {
	input := []byte(`not valid json`)
	instance := newTestInstance("origins-invalid-json")

	out, err := enrichConfigWithControlUIOrigins(input, instance)
	if err != nil {
		t.Fatal("should not error on invalid JSON")
	}

	if !bytes.Equal(out, input) {
		t.Error("invalid JSON should be returned unchanged")
	}
}

func TestEnrichConfigWithControlUIOrigins_PreservesOtherFields(t *testing.T) {
	input := []byte(`{"gateway":{"auth":{"mode":"token","token":"secret"},"bind":"loopback"}}`)
	instance := newTestInstance("origins-preserves-fields")

	out, err := enrichConfigWithControlUIOrigins(input, instance)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	gw := cfg["gateway"].(map[string]interface{})

	// Verify existing fields are preserved
	auth := gw["auth"].(map[string]interface{})
	if auth["token"] != "secret" {
		t.Errorf("gateway.auth.token = %v, want %q", auth["token"], "secret")
	}
	if gw["bind"] != "loopback" {
		t.Errorf("gateway.bind = %v, want %q", gw["bind"], "loopback")
	}

	// Verify origins were still injected
	controlUI := gw["controlUi"].(map[string]interface{})
	if _, ok := controlUI["allowedOrigins"]; !ok {
		t.Error("allowedOrigins should be injected")
	}
}

func TestEnrichConfigWithControlUIOrigins_HttpWithoutTLS(t *testing.T) {
	input := []byte(`{}`)
	instance := newTestInstance("origins-http")
	instance.Spec.Networking.Ingress.Hosts = []openclawv1alpha1.IngressHost{
		{Host: "openclaw.example.com"},
	}
	// No TLS config - should use http:// scheme

	out, err := enrichConfigWithControlUIOrigins(input, instance)
	if err != nil {
		t.Fatal(err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}

	gw := cfg["gateway"].(map[string]interface{})
	controlUI := gw["controlUi"].(map[string]interface{})
	origins := controlUI["allowedOrigins"].([]interface{})

	// Find the ingress-derived origin
	found := false
	for _, o := range origins {
		if o.(string) == "http://openclaw.example.com" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected http://openclaw.example.com in origins, got %v", origins)
	}
}

func TestBuildConfigMap_ControlUIOriginsInjected(t *testing.T) {
	instance := newTestInstance("cm-origins-inject")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"mcpServers":{"test":{"url":"http://localhost"}}}`),
		},
	}
	instance.Spec.Networking.Ingress.Hosts = []openclawv1alpha1.IngressHost{
		{Host: "openclaw.example.com"},
	}
	instance.Spec.Networking.Ingress.TLS = []openclawv1alpha1.IngressTLS{
		{Hosts: []string{"openclaw.example.com"}, SecretName: "tls-secret"},
	}

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config: %v", err)
	}

	// User config should be preserved
	if _, ok := parsed["mcpServers"]; !ok {
		t.Error("mcpServers should be preserved from raw config")
	}

	// Origins should be injected
	gw := parsed["gateway"].(map[string]interface{})
	controlUI := gw["controlUi"].(map[string]interface{})
	origins, ok := controlUI["allowedOrigins"].([]interface{})
	if !ok {
		t.Fatal("expected allowedOrigins array")
	}

	// Should contain localhost and ingress-derived origins
	originStrs := make([]string, len(origins))
	for i, o := range origins {
		originStrs[i] = o.(string)
	}

	hasLocalhost := false
	hasIngress := false
	for _, o := range originStrs {
		if o == "http://localhost:18789" {
			hasLocalhost = true
		}
		if o == "https://openclaw.example.com" {
			hasIngress = true
		}
	}
	if !hasLocalhost {
		t.Errorf("expected http://localhost:18789 in origins, got %v", originStrs)
	}
	if !hasIngress {
		t.Errorf("expected https://openclaw.example.com in origins, got %v", originStrs)
	}
}

// ---------------------------------------------------------------------------
// pvc.go tests
// ---------------------------------------------------------------------------

func TestBuildPVC_Default(t *testing.T) {
	instance := newTestInstance("pvc-test")
	pvc := BuildPVC(instance)

	if pvc.Name != "pvc-test-data" {
		t.Errorf("pvc name = %q, want %q", pvc.Name, "pvc-test-data")
	}
	if pvc.Namespace != "test-ns" {
		t.Errorf("pvc namespace = %q, want %q", pvc.Namespace, "test-ns")
	}
	if pvc.Labels["app.kubernetes.io/name"] != "openclaw" {
		t.Error("pvc missing app label")
	}

	// Backup annotation
	if pvc.Annotations["openclaw.rocks/backup-enabled"] != "true" {
		t.Error("pvc missing backup-enabled annotation")
	}

	// Default size - 10Gi
	storageReq := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if storageReq.Cmp(resource.MustParse("10Gi")) != 0 {
		t.Errorf("storage size = %v, want 10Gi", storageReq.String())
	}

	// Default access mode - ReadWriteOnce
	if len(pvc.Spec.AccessModes) != 1 {
		t.Fatalf("expected 1 access mode, got %d", len(pvc.Spec.AccessModes))
	}
	if pvc.Spec.AccessModes[0] != corev1.ReadWriteOnce {
		t.Errorf("access mode = %v, want ReadWriteOnce", pvc.Spec.AccessModes[0])
	}

	// No storage class by default
	if pvc.Spec.StorageClassName != nil {
		t.Errorf("storageClassName should be nil by default, got %v", *pvc.Spec.StorageClassName)
	}
}

func TestBuildPVC_CustomSize(t *testing.T) {
	instance := newTestInstance("pvc-custom")
	instance.Spec.Storage.Persistence.Size = "50Gi"
	scName := "fast-ssd"
	instance.Spec.Storage.Persistence.StorageClass = &scName

	pvc := BuildPVC(instance)

	storageReq := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if storageReq.Cmp(resource.MustParse("50Gi")) != 0 {
		t.Errorf("storage size = %v, want 50Gi", storageReq.String())
	}

	if pvc.Spec.StorageClassName == nil {
		t.Fatal("storageClassName should not be nil")
	}
	if *pvc.Spec.StorageClassName != "fast-ssd" {
		t.Errorf("storageClassName = %q, want %q", *pvc.Spec.StorageClassName, "fast-ssd")
	}
}

func TestBuildPVC_CustomAccessModes(t *testing.T) {
	instance := newTestInstance("pvc-modes")
	instance.Spec.Storage.Persistence.AccessModes = []corev1.PersistentVolumeAccessMode{
		corev1.ReadWriteMany,
	}

	pvc := BuildPVC(instance)

	if len(pvc.Spec.AccessModes) != 1 {
		t.Fatalf("expected 1 access mode, got %d", len(pvc.Spec.AccessModes))
	}
	if pvc.Spec.AccessModes[0] != corev1.ReadWriteMany {
		t.Errorf("access mode = %v, want ReadWriteMany", pvc.Spec.AccessModes[0])
	}
}

// ---------------------------------------------------------------------------
// Chromium PVC builder tests
// ---------------------------------------------------------------------------

func TestBuildChromiumPVC_Default(t *testing.T) {
	instance := newTestInstance("chromium-pvc-test")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Chromium.Persistence.Enabled = true

	pvc := BuildChromiumPVC(instance)

	if pvc.Name != "chromium-pvc-test-chromium-data" {
		t.Errorf("pvc name = %q, want %q", pvc.Name, "chromium-pvc-test-chromium-data")
	}
	if pvc.Namespace != "test-ns" {
		t.Errorf("pvc namespace = %q, want %q", pvc.Namespace, "test-ns")
	}

	// Default size - 1Gi
	storageReq := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if storageReq.Cmp(resource.MustParse("1Gi")) != 0 {
		t.Errorf("storage size = %v, want 1Gi", storageReq.String())
	}

	// Default access mode - ReadWriteOnce
	if len(pvc.Spec.AccessModes) != 1 {
		t.Fatalf("expected 1 access mode, got %d", len(pvc.Spec.AccessModes))
	}
	if pvc.Spec.AccessModes[0] != corev1.ReadWriteOnce {
		t.Errorf("access mode = %v, want ReadWriteOnce", pvc.Spec.AccessModes[0])
	}

	// No storage class by default
	if pvc.Spec.StorageClassName != nil {
		t.Errorf("storageClassName should be nil by default, got %v", *pvc.Spec.StorageClassName)
	}

	// Should NOT have backup annotation (chromium PVC is not backed up)
	if _, ok := pvc.Annotations["openclaw.rocks/backup-enabled"]; ok {
		t.Error("chromium PVC should not have backup-enabled annotation")
	}
}

func TestBuildChromiumPVC_CustomSize(t *testing.T) {
	instance := newTestInstance("chromium-pvc-custom")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Chromium.Persistence.Enabled = true
	instance.Spec.Chromium.Persistence.Size = "5Gi"
	scName := "fast-ssd"
	instance.Spec.Chromium.Persistence.StorageClass = &scName

	pvc := BuildChromiumPVC(instance)

	storageReq := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if storageReq.Cmp(resource.MustParse("5Gi")) != 0 {
		t.Errorf("storage size = %v, want 5Gi", storageReq.String())
	}

	if pvc.Spec.StorageClassName == nil {
		t.Fatal("storageClassName should not be nil")
	}
	if *pvc.Spec.StorageClassName != "fast-ssd" {
		t.Errorf("storageClassName = %q, want %q", *pvc.Spec.StorageClassName, "fast-ssd")
	}
}

// ---------------------------------------------------------------------------
// pdb.go tests
// ---------------------------------------------------------------------------

func TestBuildPDB_Default(t *testing.T) {
	instance := newTestInstance("pdb-test")
	pdb := BuildPDB(instance)

	if pdb.Name != "pdb-test" {
		t.Errorf("pdb name = %q, want %q", pdb.Name, "pdb-test")
	}
	if pdb.Namespace != "test-ns" {
		t.Errorf("pdb namespace = %q, want %q", pdb.Namespace, "test-ns")
	}
	if pdb.Labels["app.kubernetes.io/name"] != "openclaw" {
		t.Error("pdb missing app label")
	}

	// Selector
	sel := pdb.Spec.Selector.MatchLabels
	if sel["app.kubernetes.io/name"] != "openclaw" || sel["app.kubernetes.io/instance"] != "pdb-test" {
		t.Error("pdb selector does not match expected values")
	}

	// Default maxUnavailable = 1
	if pdb.Spec.MaxUnavailable == nil {
		t.Fatal("maxUnavailable should not be nil")
	}
	if pdb.Spec.MaxUnavailable.Type != intstr.Int {
		t.Error("maxUnavailable should be int type")
	}
	if pdb.Spec.MaxUnavailable.IntVal != 1 {
		t.Errorf("maxUnavailable = %d, want 1", pdb.Spec.MaxUnavailable.IntVal)
	}
}

func TestBuildPDB_Custom(t *testing.T) {
	instance := newTestInstance("pdb-custom")
	instance.Spec.Availability.PodDisruptionBudget = &openclawv1alpha1.PodDisruptionBudgetSpec{
		MaxUnavailable: Ptr(int32(0)),
	}

	pdb := BuildPDB(instance)

	if pdb.Spec.MaxUnavailable.IntVal != 0 {
		t.Errorf("maxUnavailable = %d, want 0", pdb.Spec.MaxUnavailable.IntVal)
	}
}

func TestBuildPDB_CustomValue(t *testing.T) {
	instance := newTestInstance("pdb-val")
	instance.Spec.Availability.PodDisruptionBudget = &openclawv1alpha1.PodDisruptionBudgetSpec{
		MaxUnavailable: Ptr(int32(2)),
	}

	pdb := BuildPDB(instance)

	if pdb.Spec.MaxUnavailable.IntVal != 2 {
		t.Errorf("maxUnavailable = %d, want 2", pdb.Spec.MaxUnavailable.IntVal)
	}
}

// ---------------------------------------------------------------------------
// ingress.go tests
// ---------------------------------------------------------------------------

func TestBuildIngress_Basic(t *testing.T) {
	instance := newTestInstance("ing-test")
	className := "nginx"
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled:   true,
		ClassName: &className,
		Hosts: []openclawv1alpha1.IngressHost{
			{
				Host: "openclaw.example.com",
			},
		},
		TLS: []openclawv1alpha1.IngressTLS{
			{
				Hosts:      []string{"openclaw.example.com"},
				SecretName: "openclaw-tls",
			},
		},
	}

	ing := BuildIngress(instance)

	// ObjectMeta
	if ing.Name != "ing-test" {
		t.Errorf("ingress name = %q, want %q", ing.Name, "ing-test")
	}
	if ing.Namespace != "test-ns" {
		t.Errorf("ingress namespace = %q, want %q", ing.Namespace, "test-ns")
	}
	if ing.Labels["app.kubernetes.io/name"] != "openclaw" {
		t.Error("ingress missing app label")
	}

	// IngressClassName
	if ing.Spec.IngressClassName == nil || *ing.Spec.IngressClassName != "nginx" {
		t.Error("ingress className should be nginx")
	}

	// Rules
	if len(ing.Spec.Rules) != 1 {
		t.Fatalf("expected 1 ingress rule, got %d", len(ing.Spec.Rules))
	}
	rule := ing.Spec.Rules[0]
	if rule.Host != "openclaw.example.com" {
		t.Errorf("ingress host = %q, want %q", rule.Host, "openclaw.example.com")
	}
	if rule.HTTP == nil || len(rule.HTTP.Paths) != 1 {
		t.Fatal("expected 1 path in ingress rule")
	}
	path := rule.HTTP.Paths[0]
	if path.Path != "/" {
		t.Errorf("ingress path = %q, want %q", path.Path, "/")
	}
	if path.Backend.Service == nil {
		t.Fatal("ingress backend service is nil")
	}
	if path.Backend.Service.Name != "ing-test" {
		t.Errorf("ingress backend service name = %q, want %q", path.Backend.Service.Name, "ing-test")
	}
	if path.Backend.Service.Port.Number != int32(GatewayPort) {
		t.Errorf("ingress backend port = %d, want %d", path.Backend.Service.Port.Number, GatewayPort)
	}

	// TLS
	if len(ing.Spec.TLS) != 1 {
		t.Fatalf("expected 1 TLS config, got %d", len(ing.Spec.TLS))
	}
	tls := ing.Spec.TLS[0]
	if tls.SecretName != "openclaw-tls" {
		t.Errorf("TLS secretName = %q, want %q", tls.SecretName, "openclaw-tls")
	}
	if len(tls.Hosts) != 1 || tls.Hosts[0] != "openclaw.example.com" {
		t.Errorf("TLS hosts = %v, want [openclaw.example.com]", tls.Hosts)
	}
}

func TestBuildIngress_DefaultAnnotations(t *testing.T) {
	instance := newTestInstance("ing-ann")
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled: true,
		Hosts: []openclawv1alpha1.IngressHost{
			{Host: "test.example.com"},
		},
	}

	ing := BuildIngress(instance)
	ann := ing.Annotations

	// No className = unknown provider = no provider-specific annotations
	if _, ok := ann["nginx.ingress.kubernetes.io/ssl-redirect"]; ok {
		t.Error("nginx annotations should not be emitted for unknown provider")
	}
	if _, ok := ann["traefik.ingress.kubernetes.io/router.entrypoints"]; ok {
		t.Error("traefik annotations should not be emitted for unknown provider")
	}
	if _, ok := ann["traefik.ingress.kubernetes.io/router.middlewares"]; ok {
		t.Error("router.middlewares annotation should never be emitted")
	}
	if _, ok := ann["nginx.ingress.kubernetes.io/proxy-read-timeout"]; ok {
		t.Error("nginx WebSocket annotations should not be emitted for unknown provider")
	}
}

func TestBuildIngress_SecurityDisabled(t *testing.T) {
	instance := newTestInstance("ing-nosec")
	className := "nginx"
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled:   true,
		ClassName: &className,
		Hosts: []openclawv1alpha1.IngressHost{
			{Host: "test.example.com"},
		},
		Security: openclawv1alpha1.IngressSecuritySpec{
			ForceHTTPS: Ptr(false),
			EnableHSTS: Ptr(false),
		},
	}

	ing := BuildIngress(instance)
	ann := ing.Annotations

	if _, ok := ann["nginx.ingress.kubernetes.io/ssl-redirect"]; ok {
		t.Error("ssl-redirect should not be set when ForceHTTPS is false")
	}
	if _, ok := ann["nginx.ingress.kubernetes.io/force-ssl-redirect"]; ok {
		t.Error("force-ssl-redirect should not be set when ForceHTTPS is false")
	}
	if _, ok := ann["nginx.ingress.kubernetes.io/configuration-snippet"]; ok {
		t.Error("HSTS configuration snippet should not be set when EnableHSTS is false")
	}
	if _, ok := ann["traefik.ingress.kubernetes.io/router.entrypoints"]; ok {
		t.Error("traefik annotations should not be emitted for nginx provider")
	}

	// WebSocket annotations should still be present for nginx provider
	if ann["nginx.ingress.kubernetes.io/proxy-read-timeout"] != "3600" {
		t.Error("proxy-read-timeout should still be 3600")
	}
}

func TestBuildIngress_RateLimiting(t *testing.T) {
	instance := newTestInstance("ing-rl")
	rps := int32(20)
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled:   true,
		ClassName: Ptr("nginx"),
		Hosts: []openclawv1alpha1.IngressHost{
			{Host: "test.example.com"},
		},
		Security: openclawv1alpha1.IngressSecuritySpec{
			RateLimiting: &openclawv1alpha1.RateLimitingSpec{
				RequestsPerSecond: &rps,
			},
		},
	}

	ing := BuildIngress(instance)
	ann := ing.Annotations

	if ann["nginx.ingress.kubernetes.io/limit-rps"] != "20" {
		t.Errorf("limit-rps = %q, want %q", ann["nginx.ingress.kubernetes.io/limit-rps"], "20")
	}
}

func TestBuildIngress_RateLimitingDefault(t *testing.T) {
	instance := newTestInstance("ing-rl-default")
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled:   true,
		ClassName: Ptr("nginx"),
		Hosts: []openclawv1alpha1.IngressHost{
			{Host: "test.example.com"},
		},
		Security: openclawv1alpha1.IngressSecuritySpec{
			RateLimiting: &openclawv1alpha1.RateLimitingSpec{
				// Enabled defaults to true, RPS defaults to 10
			},
		},
	}

	ing := BuildIngress(instance)
	ann := ing.Annotations

	if ann["nginx.ingress.kubernetes.io/limit-rps"] != "10" {
		t.Errorf("limit-rps = %q, want %q", ann["nginx.ingress.kubernetes.io/limit-rps"], "10")
	}
}

func TestBuildIngress_RateLimitingDisabled(t *testing.T) {
	instance := newTestInstance("ing-rl-off")
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled:   true,
		ClassName: Ptr("nginx"),
		Hosts: []openclawv1alpha1.IngressHost{
			{Host: "test.example.com"},
		},
		Security: openclawv1alpha1.IngressSecuritySpec{
			RateLimiting: &openclawv1alpha1.RateLimitingSpec{
				Enabled: Ptr(false),
			},
		},
	}

	ing := BuildIngress(instance)
	ann := ing.Annotations

	if _, ok := ann["nginx.ingress.kubernetes.io/limit-rps"]; ok {
		t.Error("limit-rps should not be set when rate limiting is disabled")
	}
}

func TestBuildIngress_CustomAnnotations(t *testing.T) {
	instance := newTestInstance("ing-custom-ann")
	className := "nginx"
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled:   true,
		ClassName: &className,
		Annotations: map[string]string{
			"custom-key": "custom-value",
		},
		Hosts: []openclawv1alpha1.IngressHost{
			{Host: "test.example.com"},
		},
	}

	ing := BuildIngress(instance)

	if ing.Annotations["custom-key"] != "custom-value" {
		t.Error("custom annotation not preserved")
	}
	// Provider annotations should coexist with custom annotations
	if ing.Annotations["nginx.ingress.kubernetes.io/proxy-http-version"] != "1.1" {
		t.Error("nginx annotations should coexist with custom annotations")
	}
}

func TestBuildIngress_MultipleHosts(t *testing.T) {
	instance := newTestInstance("ing-multi")
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled: true,
		Hosts: []openclawv1alpha1.IngressHost{
			{Host: "a.example.com"},
			{Host: "b.example.com"},
		},
		TLS: []openclawv1alpha1.IngressTLS{
			{
				Hosts:      []string{"a.example.com", "b.example.com"},
				SecretName: "multi-tls",
			},
		},
	}

	ing := BuildIngress(instance)

	if len(ing.Spec.Rules) != 2 {
		t.Fatalf("expected 2 ingress rules, got %d", len(ing.Spec.Rules))
	}
	if ing.Spec.Rules[0].Host != "a.example.com" {
		t.Errorf("first host = %q, want %q", ing.Spec.Rules[0].Host, "a.example.com")
	}
	if ing.Spec.Rules[1].Host != "b.example.com" {
		t.Errorf("second host = %q, want %q", ing.Spec.Rules[1].Host, "b.example.com")
	}
	if len(ing.Spec.TLS) != 1 {
		t.Fatalf("expected 1 TLS entry, got %d", len(ing.Spec.TLS))
	}
	if len(ing.Spec.TLS[0].Hosts) != 2 {
		t.Errorf("TLS hosts count = %d, want 2", len(ing.Spec.TLS[0].Hosts))
	}
}

func TestBuildIngress_CustomPaths(t *testing.T) {
	instance := newTestInstance("ing-paths")
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled: true,
		Hosts: []openclawv1alpha1.IngressHost{
			{
				Host: "test.example.com",
				Paths: []openclawv1alpha1.IngressPath{
					{Path: "/api", PathType: "Prefix"},
					{Path: "/health", PathType: "Exact"},
				},
			},
		},
	}

	ing := BuildIngress(instance)

	rule := ing.Spec.Rules[0]
	if len(rule.HTTP.Paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(rule.HTTP.Paths))
	}
	if rule.HTTP.Paths[0].Path != "/api" {
		t.Errorf("first path = %q, want %q", rule.HTTP.Paths[0].Path, "/api")
	}
	if rule.HTTP.Paths[1].Path != "/health" {
		t.Errorf("second path = %q, want %q", rule.HTTP.Paths[1].Path, "/health")
	}

	// Verify path types
	if rule.HTTP.Paths[0].PathType == nil {
		t.Fatal("first path pathType is nil")
	}
	// "Prefix" maps to PathTypePrefix
	// "Exact" maps to PathTypeExact

	if rule.HTTP.Paths[1].PathType == nil {
		t.Fatal("second path pathType is nil")
	}
}

func TestBuildIngress_NoHosts(t *testing.T) {
	instance := newTestInstance("ing-no-hosts")
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled: true,
		// No hosts
	}

	ing := BuildIngress(instance)

	if len(ing.Spec.Rules) != 0 {
		t.Errorf("expected 0 rules with no hosts, got %d", len(ing.Spec.Rules))
	}
}

func TestBuildIngress_CustomBackendPort(t *testing.T) {
	instance := newTestInstance("ing-port")
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled: true,
		Hosts: []openclawv1alpha1.IngressHost{
			{
				Host: "aibot.example.com",
				Paths: []openclawv1alpha1.IngressPath{
					{Path: "/api/messages", PathType: "Prefix", Port: Ptr(int32(3978))},
				},
			},
		},
	}

	ing := BuildIngress(instance)

	rule := ing.Spec.Rules[0]
	if len(rule.HTTP.Paths) != 1 {
		t.Fatalf("expected 1 path, got %d", len(rule.HTTP.Paths))
	}
	backend := rule.HTTP.Paths[0].Backend.Service
	if backend.Port.Number != 3978 {
		t.Errorf("backend port = %d, want 3978", backend.Port.Number)
	}
}

func TestBuildIngress_DefaultBackendPort(t *testing.T) {
	instance := newTestInstance("ing-default-port")
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled: true,
		Hosts: []openclawv1alpha1.IngressHost{
			{
				Host: "test.example.com",
				Paths: []openclawv1alpha1.IngressPath{
					{Path: "/", PathType: "Prefix"},
				},
			},
		},
	}

	ing := BuildIngress(instance)

	backend := ing.Spec.Rules[0].HTTP.Paths[0].Backend.Service
	if backend.Port.Number != int32(GatewayPort) {
		t.Errorf("backend port = %d, want %d (GatewayPort)", backend.Port.Number, GatewayPort)
	}
}

func TestBuildIngress_MixedPorts(t *testing.T) {
	instance := newTestInstance("ing-mixed-ports")
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled: true,
		Hosts: []openclawv1alpha1.IngressHost{
			{
				Host: "app.example.com",
				Paths: []openclawv1alpha1.IngressPath{
					{Path: "/api", PathType: "Prefix", Port: Ptr(int32(3978))},
					{Path: "/ws", PathType: "Prefix"},
				},
			},
		},
	}

	ing := BuildIngress(instance)

	paths := ing.Spec.Rules[0].HTTP.Paths
	if len(paths) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(paths))
	}
	if paths[0].Backend.Service.Port.Number != 3978 {
		t.Errorf("first path backend port = %d, want 3978", paths[0].Backend.Service.Port.Number)
	}
	if paths[1].Backend.Service.Port.Number != int32(GatewayPort) {
		t.Errorf("second path backend port = %d, want %d (GatewayPort)", paths[1].Backend.Service.Port.Number, GatewayPort)
	}
}

// ---------------------------------------------------------------------------
// Provider detection tests
// ---------------------------------------------------------------------------

func TestDetectIngressProvider(t *testing.T) {
	tests := []struct {
		name     string
		class    *string
		expected IngressProvider
	}{
		{"nil className", nil, IngressProviderUnknown},
		{"nginx", Ptr("nginx"), IngressProviderNginx},
		{"NGINX uppercase", Ptr("NGINX"), IngressProviderNginx},
		{"nginx-internal", Ptr("nginx-internal"), IngressProviderNginx},
		{"traefik", Ptr("traefik"), IngressProviderTraefik},
		{"Traefik mixed case", Ptr("Traefik"), IngressProviderTraefik},
		{"traefik-external", Ptr("traefik-external"), IngressProviderTraefik},
		{"haproxy", Ptr("haproxy"), IngressProviderUnknown},
		{"empty string", Ptr(""), IngressProviderUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectIngressProvider(tt.class)
			if got != tt.expected {
				t.Errorf("DetectIngressProvider(%v) = %q, want %q", tt.class, got, tt.expected)
			}
		})
	}
}

func TestBuildIngress_NginxProvider(t *testing.T) {
	instance := newTestInstance("ing-nginx")
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled:   true,
		ClassName: Ptr("nginx"),
		Hosts: []openclawv1alpha1.IngressHost{
			{Host: "test.example.com"},
		},
	}

	ing := BuildIngress(instance)
	ann := ing.Annotations

	// nginx annotations should be present
	if ann["nginx.ingress.kubernetes.io/ssl-redirect"] != "true" {
		t.Error("nginx ssl-redirect should be present")
	}
	if ann["nginx.ingress.kubernetes.io/force-ssl-redirect"] != "true" {
		t.Error("nginx force-ssl-redirect should be present")
	}
	if _, ok := ann["nginx.ingress.kubernetes.io/configuration-snippet"]; !ok {
		t.Error("nginx HSTS snippet should be present")
	}
	if ann["nginx.ingress.kubernetes.io/proxy-read-timeout"] != "3600" {
		t.Error("nginx proxy-read-timeout should be present")
	}

	// traefik annotations should NOT be present
	if _, ok := ann["traefik.ingress.kubernetes.io/router.entrypoints"]; ok {
		t.Error("traefik annotation should not be emitted for nginx provider")
	}
	if _, ok := ann["traefik.ingress.kubernetes.io/router.middlewares"]; ok {
		t.Error("traefik middleware annotation should never be emitted")
	}
}

func TestBuildIngress_TraefikProvider(t *testing.T) {
	instance := newTestInstance("ing-traefik")
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled:   true,
		ClassName: Ptr("traefik"),
		Hosts: []openclawv1alpha1.IngressHost{
			{Host: "test.example.com"},
		},
	}

	ing := BuildIngress(instance)
	ann := ing.Annotations

	// traefik annotation should be present
	if ann["traefik.ingress.kubernetes.io/router.entrypoints"] != "websecure" {
		t.Errorf("traefik router.entrypoints = %q, want %q", ann["traefik.ingress.kubernetes.io/router.entrypoints"], "websecure")
	}

	// nginx annotations should NOT be present
	if _, ok := ann["nginx.ingress.kubernetes.io/ssl-redirect"]; ok {
		t.Error("nginx ssl-redirect should not be emitted for traefik provider")
	}
	if _, ok := ann["nginx.ingress.kubernetes.io/force-ssl-redirect"]; ok {
		t.Error("nginx force-ssl-redirect should not be emitted for traefik provider")
	}
	if _, ok := ann["nginx.ingress.kubernetes.io/configuration-snippet"]; ok {
		t.Error("nginx HSTS snippet should not be emitted for traefik provider")
	}
	if _, ok := ann["nginx.ingress.kubernetes.io/proxy-read-timeout"]; ok {
		t.Error("nginx proxy-read-timeout should not be emitted for traefik provider")
	}
	if _, ok := ann["nginx.ingress.kubernetes.io/proxy-send-timeout"]; ok {
		t.Error("nginx proxy-send-timeout should not be emitted for traefik provider")
	}
	if _, ok := ann["nginx.ingress.kubernetes.io/proxy-http-version"]; ok {
		t.Error("nginx proxy-http-version should not be emitted for traefik provider")
	}
	if _, ok := ann["nginx.ingress.kubernetes.io/upstream-hash-by"]; ok {
		t.Error("nginx upstream-hash-by should not be emitted for traefik provider")
	}

	// Old middleware annotation must never appear
	if _, ok := ann["traefik.ingress.kubernetes.io/router.middlewares"]; ok {
		t.Error("traefik middleware annotation should never be emitted")
	}
}

func TestBuildIngress_UnknownProvider(t *testing.T) {
	instance := newTestInstance("ing-haproxy")
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled:   true,
		ClassName: Ptr("haproxy"),
		Hosts: []openclawv1alpha1.IngressHost{
			{Host: "test.example.com"},
		},
	}

	ing := BuildIngress(instance)
	ann := ing.Annotations

	// No provider-specific annotations for unknown provider
	if _, ok := ann["nginx.ingress.kubernetes.io/ssl-redirect"]; ok {
		t.Error("nginx annotations should not be emitted for unknown provider")
	}
	if _, ok := ann["traefik.ingress.kubernetes.io/router.entrypoints"]; ok {
		t.Error("traefik annotations should not be emitted for unknown provider")
	}
	if _, ok := ann["nginx.ingress.kubernetes.io/proxy-read-timeout"]; ok {
		t.Error("nginx WebSocket annotations should not be emitted for unknown provider")
	}
}

func TestBuildIngress_TraefikNoRateLimiting(t *testing.T) {
	instance := newTestInstance("ing-traefik-rl")
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled:   true,
		ClassName: Ptr("traefik"),
		Hosts: []openclawv1alpha1.IngressHost{
			{Host: "test.example.com"},
		},
		Security: openclawv1alpha1.IngressSecuritySpec{
			RateLimiting: &openclawv1alpha1.RateLimitingSpec{
				// Enabled defaults to true
			},
		},
	}

	ing := BuildIngress(instance)
	ann := ing.Annotations

	// Rate limiting annotation should NOT be emitted for traefik (requires Middleware CRD)
	if _, ok := ann["nginx.ingress.kubernetes.io/limit-rps"]; ok {
		t.Error("rate limiting annotation should not be emitted for traefik provider")
	}
}

// ---------------------------------------------------------------------------
// Cross-cutting / integration-style tests
// ---------------------------------------------------------------------------

func TestAllBuilders_ConsistentLabels(t *testing.T) {
	instance := newTestInstance("label-check")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{}`),
		},
	}
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled: true,
		Hosts:   []openclawv1alpha1.IngressHost{{Host: "test.example.com"}},
	}

	expectedLabels := Labels(instance)

	resources := []struct {
		name   string
		labels map[string]string
	}{
		{"Deployment", BuildStatefulSet(instance, "", nil, nil, nil).Labels},
		{"Service", BuildService(instance).Labels},
		{"NetworkPolicy", BuildNetworkPolicy(instance).Labels},
		{"ServiceAccount", BuildServiceAccount(instance).Labels},
		{"Role", BuildRole(instance).Labels},
		{"RoleBinding", BuildRoleBinding(instance).Labels},
		{"ConfigMap", BuildConfigMap(instance, "", nil).Labels},
		{"PVC", BuildPVC(instance).Labels},
		{"PDB", BuildPDB(instance).Labels},
		{"Ingress", BuildIngress(instance).Labels},
	}

	for _, r := range resources {
		t.Run(r.name, func(t *testing.T) {
			for k, v := range expectedLabels {
				if r.labels[k] != v {
					t.Errorf("%s: label %q = %q, want %q", r.name, k, r.labels[k], v)
				}
			}
		})
	}
}

func TestAllBuilders_ConsistentNamespace(t *testing.T) {
	instance := newTestInstance("ns-check")
	instance.Namespace = "production"
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{}`),
		},
	}
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled: true,
		Hosts:   []openclawv1alpha1.IngressHost{{Host: "test.example.com"}},
	}

	resources := []struct {
		name      string
		namespace string
	}{
		{"Deployment", BuildStatefulSet(instance, "", nil, nil, nil).Namespace},
		{"Service", BuildService(instance).Namespace},
		{"NetworkPolicy", BuildNetworkPolicy(instance).Namespace},
		{"ServiceAccount", BuildServiceAccount(instance).Namespace},
		{"Role", BuildRole(instance).Namespace},
		{"RoleBinding", BuildRoleBinding(instance).Namespace},
		{"ConfigMap", BuildConfigMap(instance, "", nil).Namespace},
		{"PVC", BuildPVC(instance).Namespace},
		{"PDB", BuildPDB(instance).Namespace},
		{"Ingress", BuildIngress(instance).Namespace},
	}

	for _, r := range resources {
		t.Run(r.name, func(t *testing.T) {
			if r.namespace != "production" {
				t.Errorf("%s: namespace = %q, want %q", r.name, r.namespace, "production")
			}
		})
	}
}

func TestBuildStatefulSet_ChromiumCustomResources(t *testing.T) {
	instance := newTestInstance("chromium-res")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Chromium.Resources = openclawv1alpha1.ResourcesSpec{
		Requests: openclawv1alpha1.ResourceList{
			CPU:    "500m",
			Memory: "1Gi",
		},
		Limits: openclawv1alpha1.ResourceList{
			CPU:    "2",
			Memory: "4Gi",
		},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	var chromium *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "chromium" {
			chromium = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if chromium == nil {
		t.Fatal("chromium init container not found")
	}

	cpuReq := chromium.Resources.Requests[corev1.ResourceCPU]
	if cpuReq.String() != "500m" {
		t.Errorf("chromium cpu request = %v, want 500m", cpuReq.String())
	}
	memReq := chromium.Resources.Requests[corev1.ResourceMemory]
	if memReq.Cmp(resource.MustParse("1Gi")) != 0 {
		t.Errorf("chromium memory request = %v, want 1Gi", memReq.String())
	}
	cpuLim := chromium.Resources.Limits[corev1.ResourceCPU]
	if cpuLim.String() != "2" {
		t.Errorf("chromium cpu limit = %v, want 2", cpuLim.String())
	}
	memLim := chromium.Resources.Limits[corev1.ResourceMemory]
	if memLim.Cmp(resource.MustParse("4Gi")) != 0 {
		t.Errorf("chromium memory limit = %v, want 4Gi", memLim.String())
	}
}

func TestBuildStatefulSet_CustomPodSecurityContext(t *testing.T) {
	instance := newTestInstance("custom-psc")
	instance.Spec.Security.PodSecurityContext = &openclawv1alpha1.PodSecurityContextSpec{
		RunAsUser:  Ptr(int64(2000)),
		RunAsGroup: Ptr(int64(3000)),
		FSGroup:    Ptr(int64(4000)),
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	psc := sts.Spec.Template.Spec.SecurityContext

	if *psc.RunAsUser != 2000 {
		t.Errorf("runAsUser = %d, want 2000", *psc.RunAsUser)
	}
	if *psc.RunAsGroup != 3000 {
		t.Errorf("runAsGroup = %d, want 3000", *psc.RunAsGroup)
	}
	if *psc.FSGroup != 4000 {
		t.Errorf("fsGroup = %d, want 4000", *psc.FSGroup)
	}
	// runAsNonRoot should still be true (default)
	if !*psc.RunAsNonRoot {
		t.Error("runAsNonRoot should still be true")
	}
}

func TestBuildStatefulSet_CustomPullPolicy(t *testing.T) {
	instance := newTestInstance("pull-policy")
	instance.Spec.Image.PullPolicy = corev1.PullAlways

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	if main.ImagePullPolicy != corev1.PullAlways {
		t.Errorf("pullPolicy = %v, want Always", main.ImagePullPolicy)
	}
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

func assertContainerPort(t *testing.T, ports []corev1.ContainerPort, name string, expectedPort int32) {
	t.Helper()
	for _, p := range ports {
		if p.Name == name {
			if p.ContainerPort != expectedPort {
				t.Errorf("port %q = %d, want %d", name, p.ContainerPort, expectedPort)
			}
			if p.Protocol != corev1.ProtocolTCP {
				t.Errorf("port %q protocol = %v, want TCP", name, p.Protocol)
			}
			return
		}
	}
	t.Errorf("port %q not found", name)
}

func assertServicePort(t *testing.T, ports []corev1.ServicePort, name string, expectedPort int32) {
	t.Helper()
	for _, p := range ports {
		if p.Name == name {
			if p.Port != expectedPort {
				t.Errorf("service port %q = %d, want %d", name, p.Port, expectedPort)
			}
			if p.TargetPort.IntValue() != int(expectedPort) {
				t.Errorf("service target port %q = %d, want %d", name, p.TargetPort.IntValue(), expectedPort)
			}
			if p.Protocol != corev1.ProtocolTCP {
				t.Errorf("service port %q protocol = %v, want TCP", name, p.Protocol)
			}
			return
		}
	}
	t.Errorf("service port %q not found", name)
}

func assertServicePortWithTarget(t *testing.T, ports []corev1.ServicePort, name string, expectedPort, expectedTargetPort int32) {
	t.Helper()
	for _, p := range ports {
		if p.Name == name {
			if p.Port != expectedPort {
				t.Errorf("service port %q = %d, want %d", name, p.Port, expectedPort)
			}
			if p.TargetPort.IntValue() != int(expectedTargetPort) {
				t.Errorf("service target port %q = %d, want %d", name, p.TargetPort.IntValue(), expectedTargetPort)
			}
			if p.Protocol != corev1.ProtocolTCP {
				t.Errorf("service port %q protocol = %v, want TCP", name, p.Protocol)
			}
			return
		}
	}
	t.Errorf("service port %q not found", name)
}

func assertNPPort(t *testing.T, ports []networkingv1.NetworkPolicyPort, expectedPort int) {
	t.Helper()
	for _, p := range ports {
		if p.Port != nil && p.Port.IntValue() == expectedPort {
			return
		}
	}
	t.Errorf("network policy port %d not found", expectedPort)
}

func assertVolumeMount(t *testing.T, mounts []corev1.VolumeMount, name, expectedPath string) {
	t.Helper()
	for _, m := range mounts {
		if m.Name == name {
			if m.MountPath != expectedPath {
				t.Errorf("volume mount %q path = %q, want %q", name, m.MountPath, expectedPath)
			}
			return
		}
	}
	t.Errorf("volume mount %q not found", name)
}

func envNames(envs []corev1.EnvVar) []string {
	names := make([]string, len(envs))
	for i, e := range envs {
		names[i] = e.Name
	}
	return names
}

func findVolume(volumes []corev1.Volume, name string) *corev1.Volume {
	for i := range volumes {
		if volumes[i].Name == name {
			return &volumes[i]
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Kubernetes default field tests (regression for issue #28 — reconcile loop)
// ---------------------------------------------------------------------------

// TestBuildStatefulSet_KubernetesDefaults verifies that the StatefulSet builder
// explicitly sets all fields that Kubernetes would default on the server side.
// If any of these are missing, controllerutil.CreateOrUpdate sees a diff on
// every reconcile, causing an endless update loop.
func TestBuildStatefulSet_KubernetesDefaults(t *testing.T) {
	instance := newTestInstance("k8s-defaults")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// StatefulSetSpec defaults
	if sts.Spec.RevisionHistoryLimit == nil || *sts.Spec.RevisionHistoryLimit != 10 {
		t.Errorf("RevisionHistoryLimit = %v, want 10", sts.Spec.RevisionHistoryLimit)
	}
	if sts.Spec.ServiceName != "k8s-defaults" {
		t.Errorf("ServiceName = %q, want %q", sts.Spec.ServiceName, "k8s-defaults")
	}
	if sts.Spec.PodManagementPolicy != appsv1.ParallelPodManagement {
		t.Errorf("PodManagementPolicy = %v, want Parallel", sts.Spec.PodManagementPolicy)
	}
	if sts.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
		t.Errorf("UpdateStrategy = %v, want RollingUpdate", sts.Spec.UpdateStrategy.Type)
	}

	// PodSpec defaults
	podSpec := sts.Spec.Template.Spec
	if podSpec.RestartPolicy != corev1.RestartPolicyAlways {
		t.Errorf("RestartPolicy = %v, want Always", podSpec.RestartPolicy)
	}
	if podSpec.DNSPolicy != corev1.DNSClusterFirst {
		t.Errorf("DNSPolicy = %v, want ClusterFirst", podSpec.DNSPolicy)
	}
	if podSpec.SchedulerName != corev1.DefaultSchedulerName {
		t.Errorf("SchedulerName = %v, want %v", podSpec.SchedulerName, corev1.DefaultSchedulerName)
	}
	if podSpec.TerminationGracePeriodSeconds == nil || *podSpec.TerminationGracePeriodSeconds != 30 {
		t.Errorf("TerminationGracePeriodSeconds = %v, want 30", podSpec.TerminationGracePeriodSeconds)
	}

	// Container defaults
	main := sts.Spec.Template.Spec.Containers[0]
	if main.TerminationMessagePath != corev1.TerminationMessagePathDefault {
		t.Errorf("TerminationMessagePath = %q, want %q", main.TerminationMessagePath, corev1.TerminationMessagePathDefault)
	}
	if main.TerminationMessagePolicy != corev1.TerminationMessageReadFile {
		t.Errorf("TerminationMessagePolicy = %v, want File", main.TerminationMessagePolicy)
	}

	// Probe successThreshold defaults
	if main.LivenessProbe.SuccessThreshold != 1 {
		t.Errorf("LivenessProbe.SuccessThreshold = %d, want 1", main.LivenessProbe.SuccessThreshold)
	}
	if main.ReadinessProbe.SuccessThreshold != 1 {
		t.Errorf("ReadinessProbe.SuccessThreshold = %d, want 1", main.ReadinessProbe.SuccessThreshold)
	}
	if main.StartupProbe.SuccessThreshold != 1 {
		t.Errorf("StartupProbe.SuccessThreshold = %d, want 1", main.StartupProbe.SuccessThreshold)
	}
}

// TestBuildStatefulSet_InitContainerDefaults verifies init containers include
// Kubernetes default fields to avoid reconcile-loop drift.
func TestBuildStatefulSet_InitContainerDefaults(t *testing.T) {
	instance := newTestInstance("init-defaults")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	if len(sts.Spec.Template.Spec.InitContainers) == 0 {
		t.Fatal("expected init container when raw config is set")
	}

	init := sts.Spec.Template.Spec.InitContainers[0]
	if init.TerminationMessagePath != corev1.TerminationMessagePathDefault {
		t.Errorf("init container TerminationMessagePath = %q, want %q", init.TerminationMessagePath, corev1.TerminationMessagePathDefault)
	}
	if init.TerminationMessagePolicy != corev1.TerminationMessageReadFile {
		t.Errorf("init container TerminationMessagePolicy = %v, want File", init.TerminationMessagePolicy)
	}
	if init.ImagePullPolicy != corev1.PullIfNotPresent {
		t.Errorf("init container ImagePullPolicy = %v, want IfNotPresent", init.ImagePullPolicy)
	}
}

// TestBuildStatefulSet_ChromiumContainerDefaults verifies the chromium native sidecar
// includes Kubernetes default fields.
func TestBuildStatefulSet_ChromiumContainerDefaults(t *testing.T) {
	instance := newTestInstance("chromium-defaults")
	instance.Spec.Chromium.Enabled = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var chromium *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "chromium" {
			chromium = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if chromium == nil {
		t.Fatal("chromium init container not found")
	}

	if chromium.TerminationMessagePath != corev1.TerminationMessagePathDefault {
		t.Errorf("chromium TerminationMessagePath = %q, want %q", chromium.TerminationMessagePath, corev1.TerminationMessagePathDefault)
	}
	if chromium.TerminationMessagePolicy != corev1.TerminationMessageReadFile {
		t.Errorf("chromium TerminationMessagePolicy = %v, want File", chromium.TerminationMessagePolicy)
	}
}

func TestBuildStatefulSet_ChromiumPersistenceEnabled(t *testing.T) {
	instance := newTestInstance("chromium-persist")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Chromium.Persistence.Enabled = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// chromium-data volume should be a PVC
	dataVol := findVolume(sts.Spec.Template.Spec.Volumes, "chromium-data")
	if dataVol == nil {
		t.Fatal("chromium-data volume not found")
	}
	if dataVol.PersistentVolumeClaim == nil {
		t.Fatal("chromium-data should be a PVC when persistence is enabled")
	}
	if dataVol.PersistentVolumeClaim.ClaimName != "chromium-persist-chromium-data" {
		t.Errorf("PVC claim name = %q, want %q", dataVol.PersistentVolumeClaim.ClaimName, "chromium-persist-chromium-data")
	}

	// --user-data-dir should be in Args when persistence is enabled
	var chromium *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "chromium" {
			chromium = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if chromium == nil {
		t.Fatal("chromium init container not found")
	}

	argsStr := strings.Join(chromium.Args, " ")
	if !strings.Contains(argsStr, "--user-data-dir=/chromium-data") {
		t.Error("persistence should add --user-data-dir=/chromium-data to Args")
	}

	// Volume mount should exist
	assertVolumeMount(t, chromium.VolumeMounts, "chromium-data", "/chromium-data")
}

func TestBuildStatefulSet_ChromiumPersistenceExistingClaim(t *testing.T) {
	instance := newTestInstance("chromium-existing")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Chromium.Persistence.Enabled = true
	instance.Spec.Chromium.Persistence.ExistingClaim = "my-chromium-pvc"

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	dataVol := findVolume(sts.Spec.Template.Spec.Volumes, "chromium-data")
	if dataVol == nil {
		t.Fatal("chromium-data volume not found")
	}
	if dataVol.PersistentVolumeClaim == nil {
		t.Fatal("chromium-data should be a PVC when using existing claim")
	}
	if dataVol.PersistentVolumeClaim.ClaimName != "my-chromium-pvc" {
		t.Errorf("PVC claim name = %q, want %q", dataVol.PersistentVolumeClaim.ClaimName, "my-chromium-pvc")
	}
}

func TestBuildStatefulSet_ChromiumPersistenceDisabled(t *testing.T) {
	instance := newTestInstance("chromium-no-persist")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Chromium.Persistence.Enabled = false

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	dataVol := findVolume(sts.Spec.Template.Spec.Volumes, "chromium-data")
	if dataVol == nil {
		t.Fatal("chromium-data volume not found")
	}
	if dataVol.EmptyDir == nil {
		t.Error("chromium-data should be emptyDir when persistence is disabled")
	}
	if dataVol.PersistentVolumeClaim != nil {
		t.Error("chromium-data should not be a PVC when persistence is disabled")
	}
}

// TestBuildStatefulSet_ConfigMapDefaultMode verifies the ConfigMap volume
// explicitly sets DefaultMode to match the Kubernetes default (0644).
func TestBuildStatefulSet_ConfigMapDefaultMode(t *testing.T) {
	instance := newTestInstance("cm-default-mode")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	configVol := findVolume(sts.Spec.Template.Spec.Volumes, "config")
	if configVol == nil {
		t.Fatal("config volume not found")
	}
	if configVol.ConfigMap == nil {
		t.Fatal("config volume should use ConfigMap")
	}
	if configVol.ConfigMap.DefaultMode == nil || *configVol.ConfigMap.DefaultMode != 0o644 {
		t.Errorf("ConfigMap DefaultMode = %v, want 0o644", configVol.ConfigMap.DefaultMode)
	}
}

// TestBuildService_KubernetesDefaults verifies Service builder includes
// Kubernetes default fields.
func TestBuildService_KubernetesDefaults(t *testing.T) {
	instance := newTestInstance("svc-defaults")
	svc := BuildService(instance)

	if svc.Spec.SessionAffinity != corev1.ServiceAffinityNone {
		t.Errorf("SessionAffinity = %v, want None", svc.Spec.SessionAffinity)
	}
}

// TestBuildStatefulSet_Idempotent verifies calling BuildStatefulSet twice with
// the same input produces identical specs (no random maps, no pointer aliasing
// issues). This is essential for CreateOrUpdate comparisons to work.
func TestBuildStatefulSet_Idempotent(t *testing.T) {
	instance := newTestInstance("idempotent")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{"key":"val"}`)},
	}
	instance.Spec.Chromium.Enabled = true

	dep1 := BuildStatefulSet(instance, "", nil, nil, nil)
	dep2 := BuildStatefulSet(instance, "", nil, nil, nil)

	b1, _ := json.Marshal(dep1.Spec)
	b2, _ := json.Marshal(dep2.Spec)

	if !bytes.Equal(b1, b2) {
		t.Error("BuildStatefulSet is not idempotent — two calls with the same input produce different specs")
	}
}

// ---------------------------------------------------------------------------
// workspace_configmap.go tests
// ---------------------------------------------------------------------------

func TestBuildWorkspaceConfigMap_Nil(t *testing.T) {
	instance := newTestInstance("ws-nil")
	instance.Spec.Workspace = nil

	cm := BuildWorkspaceConfigMap(instance, nil, nil, nil)
	// Operator files are always injected
	if cm == nil {
		t.Fatal("expected non-nil ConfigMap (operator files are always injected)")
	}
	for _, f := range []string{"ENVIRONMENT.md", "BOOTSTRAP.md"} {
		if _, ok := cm.Data[f]; !ok {
			t.Errorf("expected %s in workspace ConfigMap", f)
		}
	}
}

func TestBuildWorkspaceConfigMap_EmptyFiles(t *testing.T) {
	instance := newTestInstance("ws-empty")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialDirectories: []string{"memory"},
	}

	cm := BuildWorkspaceConfigMap(instance, nil, nil, nil)
	// Operator files are always injected
	if cm == nil {
		t.Fatal("expected non-nil ConfigMap (operator files are always injected)")
	}
	for _, f := range []string{"ENVIRONMENT.md", "BOOTSTRAP.md"} {
		if _, ok := cm.Data[f]; !ok {
			t.Errorf("expected %s in workspace ConfigMap", f)
		}
	}
}

func TestBuildWorkspaceConfigMap_WithFiles(t *testing.T) {
	instance := newTestInstance("ws-files")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{
			"SOUL.md":   "# Personality\nBe helpful.",
			"AGENTS.md": "# Agents config",
		},
	}

	cm := BuildWorkspaceConfigMap(instance, nil, nil, nil)
	if cm == nil {
		t.Fatal("expected non-nil ConfigMap when files are set")
	}
	if cm.Name != "ws-files-workspace" {
		t.Errorf("ConfigMap name = %q, want %q", cm.Name, "ws-files-workspace")
	}
	if cm.Namespace != "test-ns" {
		t.Errorf("ConfigMap namespace = %q, want %q", cm.Namespace, "test-ns")
	}
	// 2 user files + ENVIRONMENT.md + BOOTSTRAP.md = 4
	if len(cm.Data) != 4 {
		t.Fatalf("expected 4 data entries (2 user + ENVIRONMENT.md + BOOTSTRAP.md), got %d", len(cm.Data))
	}
	if cm.Data["SOUL.md"] != "# Personality\nBe helpful." {
		t.Errorf("SOUL.md content mismatch")
	}
	if cm.Data["AGENTS.md"] != "# Agents config" {
		t.Errorf("AGENTS.md content mismatch")
	}
	for _, f := range []string{"ENVIRONMENT.md", "BOOTSTRAP.md"} {
		if _, ok := cm.Data[f]; !ok {
			t.Errorf("expected %s in workspace ConfigMap", f)
		}
	}
}

// TestBuildWorkspaceConfigMap_NestedInitialFiles verifies that user-defined
// initialFiles paths containing '/' are stored under ConfigMap-safe keys
// (slashes encoded as '--'), since Kubernetes rejects '/' in data keys (#482).
func TestBuildWorkspaceConfigMap_NestedInitialFiles(t *testing.T) {
	instance := newTestInstance("ws-nested")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{
			"agents/AGENT.md":         "# Agent",
			"skills/redmine/SKILL.md": "# Skill",
			"SOUL.md":                 "# Soul",
		},
	}

	cm := BuildWorkspaceConfigMap(instance, nil, nil, nil)
	if cm == nil {
		t.Fatal("expected non-nil ConfigMap")
	}
	if got, want := cm.Data["agents--AGENT.md"], "# Agent"; got != want {
		t.Errorf("encoded key agents--AGENT.md: got %q, want %q", got, want)
	}
	if got, want := cm.Data["skills--redmine--SKILL.md"], "# Skill"; got != want {
		t.Errorf("encoded key skills--redmine--SKILL.md: got %q, want %q", got, want)
	}
	// Flat keys must remain unchanged.
	if got, want := cm.Data["SOUL.md"], "# Soul"; got != want {
		t.Errorf("flat key SOUL.md: got %q, want %q", got, want)
	}
	// Raw nested keys must not appear (Kubernetes would reject them).
	for k := range cm.Data {
		if strings.Contains(k, "/") {
			t.Errorf("ConfigMap data key %q contains '/' which is invalid", k)
		}
	}
}

// TestBuildWorkspaceConfigMap_AdditionalWorkspaceNestedFiles verifies that
// nested initialFiles inside additionalWorkspaces are also encoded (#482).
func TestBuildWorkspaceConfigMap_AdditionalWorkspaceNestedFiles(t *testing.T) {
	instance := newTestInstance("ws-aw-nested")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{
				Name: "research",
				InitialFiles: map[string]string{
					"docs/PLAN.md": "# Plan",
				},
			},
		},
	}

	cm := BuildWorkspaceConfigMap(instance, nil, nil, nil)
	if cm == nil {
		t.Fatal("expected non-nil ConfigMap")
	}
	want := AdditionalWorkspaceCMKey("research", "docs--PLAN.md")
	if got := cm.Data[want]; got != "# Plan" {
		t.Errorf("key %q: got %q, want %q", want, got, "# Plan")
	}
	for k := range cm.Data {
		if strings.Contains(k, "/") {
			t.Errorf("ConfigMap data key %q contains '/'", k)
		}
	}
}

// TestBuildWorkspaceConfigMap_BootstrapDisabled verifies that setting
// spec.workspace.bootstrap.enabled=false suppresses the operator-injected
// BOOTSTRAP.md. OpenClaw deletes the file after applying it, so re-seeding
// on every pod start forces the agent to re-run onboarding repeatedly (#463).
func TestBuildWorkspaceConfigMap_BootstrapDisabled(t *testing.T) {
	falseVal := false
	instance := newTestInstance("ws-no-bootstrap")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		Bootstrap: openclawv1alpha1.BootstrapSpec{
			Enabled: &falseVal,
		},
	}

	cm := BuildWorkspaceConfigMap(instance, nil, nil, nil)
	if cm == nil {
		t.Fatal("expected non-nil ConfigMap (ENVIRONMENT.md is still injected)")
	}
	if _, ok := cm.Data["BOOTSTRAP.md"]; ok {
		t.Error("BOOTSTRAP.md should not be injected when spec.workspace.bootstrap.enabled=false")
	}
	if _, ok := cm.Data["ENVIRONMENT.md"]; !ok {
		t.Error("ENVIRONMENT.md should still be injected when bootstrap is disabled")
	}
}

// TestBuildWorkspaceConfigMap_BootstrapEnabledExplicitly verifies that an
// explicit enabled=true produces the same behavior as the default (backward
// compatibility guarantee for anyone who sets the field explicitly).
func TestBuildWorkspaceConfigMap_BootstrapEnabledExplicitly(t *testing.T) {
	trueVal := true
	instance := newTestInstance("ws-bootstrap-explicit")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		Bootstrap: openclawv1alpha1.BootstrapSpec{
			Enabled: &trueVal,
		},
	}

	cm := BuildWorkspaceConfigMap(instance, nil, nil, nil)
	if cm == nil {
		t.Fatal("expected non-nil ConfigMap")
	}
	if _, ok := cm.Data["BOOTSTRAP.md"]; !ok {
		t.Error("BOOTSTRAP.md should be injected when spec.workspace.bootstrap.enabled=true")
	}
}

func TestBuildWorkspaceConfigMap_WithExternalFiles(t *testing.T) {
	instance := newTestInstance("ws-ext")
	externalFiles := map[string]string{
		"AGENT.md": "# External agent file",
		"SOUL.md":  "# External soul",
	}

	cm := BuildWorkspaceConfigMap(instance, externalFiles, nil, nil)
	if cm == nil {
		t.Fatal("expected non-nil ConfigMap")
	}
	// 2 external + ENVIRONMENT.md + BOOTSTRAP.md = 4
	if len(cm.Data) != 4 {
		t.Fatalf("expected 4 data entries, got %d", len(cm.Data))
	}
	if cm.Data["AGENT.md"] != "# External agent file" {
		t.Errorf("AGENT.md content mismatch: got %q", cm.Data["AGENT.md"])
	}
	if cm.Data["SOUL.md"] != "# External soul" {
		t.Errorf("SOUL.md content mismatch: got %q", cm.Data["SOUL.md"])
	}
}

func TestBuildWorkspaceConfigMap_MergePriority(t *testing.T) {
	instance := newTestInstance("ws-merge")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{
			"SOUL.md":  "# Inline soul wins",
			"EXTRA.md": "# Only inline",
		},
	}
	externalFiles := map[string]string{
		"SOUL.md":   "# External soul loses",
		"REMOTE.md": "# Only external",
	}
	skillPacks := &ResolvedSkillPacks{
		Files: map[string]string{
			"SOUL.md":  "# Skill pack soul loses",
			"SKILL.md": "# Only skill pack",
		},
	}

	cm := BuildWorkspaceConfigMap(instance, externalFiles, nil, skillPacks)
	if cm == nil {
		t.Fatal("expected non-nil ConfigMap")
	}

	// Inline should override external and skill pack
	if cm.Data["SOUL.md"] != "# Inline soul wins" {
		t.Errorf("SOUL.md should be inline value, got %q", cm.Data["SOUL.md"])
	}
	// External-only file should be present
	if cm.Data["REMOTE.md"] != "# Only external" {
		t.Errorf("REMOTE.md should be from external, got %q", cm.Data["REMOTE.md"])
	}
	// Skill pack-only file should be present
	if cm.Data["SKILL.md"] != "# Only skill pack" {
		t.Errorf("SKILL.md should be from skill pack, got %q", cm.Data["SKILL.md"])
	}
	// Inline-only file should be present
	if cm.Data["EXTRA.md"] != "# Only inline" {
		t.Errorf("EXTRA.md should be from inline, got %q", cm.Data["EXTRA.md"])
	}
	// Operator files always present and override everything
	if cm.Data["ENVIRONMENT.md"] != EnvironmentSkillContent {
		t.Error("ENVIRONMENT.md should be operator-injected content")
	}
}

func TestBuildWorkspaceConfigMap_ExternalOnly(t *testing.T) {
	instance := newTestInstance("ws-ext-only")
	// No workspace spec set, but external files provided
	externalFiles := map[string]string{
		"AGENT.md": "# External agent",
	}

	cm := BuildWorkspaceConfigMap(instance, externalFiles, nil, nil)
	if cm == nil {
		t.Fatal("expected non-nil ConfigMap")
	}
	// 1 external + ENVIRONMENT.md + BOOTSTRAP.md = 3
	if len(cm.Data) != 3 {
		t.Fatalf("expected 3 data entries, got %d", len(cm.Data))
	}
	if cm.Data["AGENT.md"] != "# External agent" {
		t.Errorf("AGENT.md content mismatch")
	}
}

func TestBuildInitScript_WithExternalFiles(t *testing.T) {
	instance := newTestInstance("init-ext")
	externalFiles := map[string]string{
		"AGENT.md": "# External agent",
		"SOUL.md":  "# External soul",
	}

	script := BuildInitScript(instance, externalFiles, nil, nil)
	// Should have copy commands for external files
	if !strings.Contains(script, "AGENT.md") {
		t.Error("expected init script to reference AGENT.md from external files")
	}
	if !strings.Contains(script, "SOUL.md") {
		t.Error("expected init script to reference SOUL.md from external files")
	}
}

func TestConfigHash_ChangesWithExternalWorkspaceFiles(t *testing.T) {
	instance := newTestInstance("hash-ext")

	sts1 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash1 := sts1.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	externalFiles := map[string]string{
		"AGENT.md": "# External agent content",
	}
	sts2 := BuildStatefulSet(instance, "", nil, externalFiles, nil)
	hash2 := sts2.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	if hash1 == hash2 {
		t.Error("config hash must change when external workspace files are added (workspace-init only copies them at pod start)")
	}

	// Content-only change: the init script embeds file names but not contents,
	// so the hash is the only thing that can trigger the rollout.
	externalFiles["AGENT.md"] = "# Updated external agent content"
	sts3 := BuildStatefulSet(instance, "", nil, externalFiles, nil)
	hash3 := sts3.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	if hash2 == hash3 {
		t.Error("config hash must change when external workspace file content changes")
	}
}

func TestConfigHash_ChangesWithAdditionalExternalFiles(t *testing.T) {
	instance := newTestInstance("hash-add-ext")

	sts1 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash1 := sts1.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	additionalFiles := map[string]map[string]string{
		"secondary": {"NOTES.md": "# Secondary workspace notes"},
	}
	sts2 := BuildStatefulSet(instance, "", nil, nil, additionalFiles)
	hash2 := sts2.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	if hash1 == hash2 {
		t.Error("config hash must change when additional-workspace external files change")
	}
}

func TestConfigHash_ChangesWithControlUIOrigins(t *testing.T) {
	instance := newTestInstance("hash-cui")

	sts1 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash1 := sts1.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	instance.Spec.Gateway.ControlUIOrigins = []string{"https://claw.example.com"}

	sts2 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash2 := sts2.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	if hash1 == hash2 {
		t.Error("config hash must change when spec.gateway.controlUiOrigins changes (it re-renders gateway.controlUi.allowedOrigins)")
	}
}

func TestConfigHash_ChangesWithIngressHosts(t *testing.T) {
	instance := newTestInstance("hash-ingress")

	sts1 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash1 := sts1.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	instance.Spec.Networking.Ingress.Hosts = []openclawv1alpha1.IngressHost{
		{Host: "claw.example.com"},
	}

	sts2 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash2 := sts2.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	if hash1 == hash2 {
		t.Error("config hash must change when ingress hosts change (they feed derived control UI origins)")
	}
}

func TestConfigHash_StableWithUnrelatedSpecChange(t *testing.T) {
	instance := newTestInstance("hash-unrelated")

	sts1 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash1 := sts1.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	instance.Spec.Resources = openclawv1alpha1.ResourcesSpec{
		Limits: openclawv1alpha1.ResourceList{Memory: "2Gi"},
	}

	sts2 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash2 := sts2.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	if hash1 != hash2 {
		t.Error("config hash must not change for spec changes that do not affect rendered config (resources roll the pod via the template itself)")
	}
}

func TestWorkspaceConfigMapName(t *testing.T) {
	instance := newTestInstance("foo")
	if got := WorkspaceConfigMapName(instance); got != "foo-workspace" {
		t.Errorf("WorkspaceConfigMapName() = %q, want %q", got, "foo-workspace")
	}
}

// ---------------------------------------------------------------------------
// BuildInitScript tests
// ---------------------------------------------------------------------------

func TestShellQuote(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "'simple'"},
		{"it's", "'it'\\''s'"},
		{"no quotes", "'no quotes'"},
		{"a'b'c", "'a'\\''b'\\''c'"},
	}
	for _, tt := range tests {
		got := shellQuote(tt.input)
		if got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// operatorSeedLines is the init script suffix that seeds operator-injected
// workspace files (always present). In the default Replace skill pack policy
// it is followed by the no-pack sync block, which cleans up files seeded by a
// previously declared pack revision (#564).
var operatorSeedLines = "mkdir -p /data/workspace\n[ -f /data/workspace/'BOOTSTRAP.md' ] || cp /workspace-init/'BOOTSTRAP.md' /data/workspace/'BOOTSTRAP.md'\n[ -f /data/workspace/'ENVIRONMENT.md' ] || cp /workspace-init/'ENVIRONMENT.md' /data/workspace/'ENVIRONMENT.md'\n" + strings.Join(buildSkillPackSyncLines(nil), "\n")

func TestBuildInitScript_ConfigOnly(t *testing.T) {
	instance := newTestInstance("init-config-only")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}

	script := BuildInitScript(instance, nil, nil, nil)
	expected := "cp /config/'openclaw.json' /data/openclaw.json\n" + operatorSeedLines
	if script != expected {
		t.Errorf("unexpected script:\ngot:  %q\nwant: %q", script, expected)
	}
}

func TestBuildInitScript_WorkspaceOnly(t *testing.T) {
	instance := newTestInstance("init-ws-only")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{
			"SOUL.md": "content",
		},
		InitialDirectories: []string{"memory"},
	}

	script := BuildInitScript(instance, nil, nil, nil)
	expected := "cp /config/'openclaw.json' /data/openclaw.json\nmkdir -p /data/workspace/'memory'\nmkdir -p /data/workspace\n[ -f /data/workspace/'BOOTSTRAP.md' ] || cp /workspace-init/'BOOTSTRAP.md' /data/workspace/'BOOTSTRAP.md'\n[ -f /data/workspace/'ENVIRONMENT.md' ] || cp /workspace-init/'ENVIRONMENT.md' /data/workspace/'ENVIRONMENT.md'\n[ -f /data/workspace/'SOUL.md' ] || cp /workspace-init/'SOUL.md' /data/workspace/'SOUL.md'\n" + strings.Join(buildSkillPackSyncLines(nil), "\n")
	if script != expected {
		t.Errorf("unexpected script:\ngot:  %q\nwant: %q", script, expected)
	}
}

// TestBuildInitScript_BootstrapDisabled verifies that the init script omits the
// BOOTSTRAP.md seed line when spec.workspace.bootstrap.enabled=false. Without
// this, OpenClaw's post-bootstrap cleanup is undone on every pod restart (#463).
func TestBuildInitScript_BootstrapDisabled(t *testing.T) {
	falseVal := false
	instance := newTestInstance("init-no-bootstrap")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{
			"SOUL.md": "content",
		},
		Bootstrap: openclawv1alpha1.BootstrapSpec{
			Enabled: &falseVal,
		},
	}

	script := BuildInitScript(instance, nil, nil, nil)
	if strings.Contains(script, "BOOTSTRAP.md") {
		t.Errorf("init script should not reference BOOTSTRAP.md when bootstrap disabled:\n%s", script)
	}
	// ENVIRONMENT.md must still be seeded.
	if !strings.Contains(script, "ENVIRONMENT.md") {
		t.Errorf("init script should still seed ENVIRONMENT.md, got:\n%s", script)
	}
	// User workspace files must still be seeded.
	if !strings.Contains(script, "SOUL.md") {
		t.Errorf("init script should still seed user workspace files, got:\n%s", script)
	}
}

func TestBuildInitScript_Both(t *testing.T) {
	instance := newTestInstance("init-both")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{
			"SOUL.md":   "soul",
			"AGENTS.md": "agents",
		},
		InitialDirectories: []string{"memory", "tools"},
	}

	script := BuildInitScript(instance, nil, nil, nil)

	// The default Replace policy appends the no-pack sync block; strip it so
	// the exact line assertions below stay focused on seeding.
	syncSuffix := "\n" + strings.Join(buildSkillPackSyncLines(nil), "\n")
	if !strings.HasSuffix(script, syncSuffix) {
		t.Fatalf("expected script to end with the no-pack sync block, got:\n%s", script)
	}
	script = strings.TrimSuffix(script, syncSuffix)

	// Verify all expected lines are present (sorted order, operator files included)
	lines := strings.Split(script, "\n")
	if len(lines) != 8 {
		t.Fatalf("expected 8 lines, got %d:\n%s", len(lines), script)
	}
	if lines[0] != "cp /config/'openclaw.json' /data/openclaw.json" {
		t.Errorf("line 0: %q", lines[0])
	}
	if lines[1] != "mkdir -p /data/workspace/'memory'" {
		t.Errorf("line 1: %q", lines[1])
	}
	if lines[2] != "mkdir -p /data/workspace/'tools'" {
		t.Errorf("line 2: %q", lines[2])
	}
	if lines[3] != "mkdir -p /data/workspace" {
		t.Errorf("line 3: %q", lines[3])
	}
	if lines[4] != "[ -f /data/workspace/'AGENTS.md' ] || cp /workspace-init/'AGENTS.md' /data/workspace/'AGENTS.md'" {
		t.Errorf("line 4: %q", lines[4])
	}
	if lines[5] != "[ -f /data/workspace/'BOOTSTRAP.md' ] || cp /workspace-init/'BOOTSTRAP.md' /data/workspace/'BOOTSTRAP.md'" {
		t.Errorf("line 5: %q", lines[5])
	}
	if lines[6] != "[ -f /data/workspace/'ENVIRONMENT.md' ] || cp /workspace-init/'ENVIRONMENT.md' /data/workspace/'ENVIRONMENT.md'" {
		t.Errorf("line 6: %q", lines[6])
	}
	if lines[7] != "[ -f /data/workspace/'SOUL.md' ] || cp /workspace-init/'SOUL.md' /data/workspace/'SOUL.md'" {
		t.Errorf("line 7: %q", lines[7])
	}
}

func TestBuildInitScript_DirsOnly(t *testing.T) {
	instance := newTestInstance("init-dirs-only")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialDirectories: []string{"memory", "tools/scripts"},
	}

	script := BuildInitScript(instance, nil, nil, nil)
	expected := "cp /config/'openclaw.json' /data/openclaw.json\nmkdir -p /data/workspace/'memory'\nmkdir -p /data/workspace/'tools/scripts'\n" + operatorSeedLines
	if script != expected {
		t.Errorf("unexpected script:\ngot:  %q\nwant: %q", script, expected)
	}
}

func TestBuildInitScript_ShellQuotesSpecialChars(t *testing.T) {
	instance := newTestInstance("init-special")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{
			"it's a file.md": "content",
		},
	}

	script := BuildInitScript(instance, nil, nil, nil)
	expected := "cp /config/'openclaw.json' /data/openclaw.json\nmkdir -p /data/workspace\n[ -f /data/workspace/'BOOTSTRAP.md' ] || cp /workspace-init/'BOOTSTRAP.md' /data/workspace/'BOOTSTRAP.md'\n[ -f /data/workspace/'ENVIRONMENT.md' ] || cp /workspace-init/'ENVIRONMENT.md' /data/workspace/'ENVIRONMENT.md'\n[ -f /data/workspace/'it'\\''s a file.md' ] || cp /workspace-init/'it'\\''s a file.md' /data/workspace/'it'\\''s a file.md'\n" + strings.Join(buildSkillPackSyncLines(nil), "\n")
	if script != expected {
		t.Errorf("unexpected script:\ngot:  %q\nwant: %q", script, expected)
	}
}

// TestBuildInitScript_NestedInitialFiles verifies that nested initialFiles
// paths produce a `mkdir -p` of the parent and a `cp` from the encoded source
// key to the original workspace path (#482).
func TestBuildInitScript_NestedInitialFiles(t *testing.T) {
	instance := newTestInstance("init-nested")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{
			"agents/AGENT.md":         "# Agent",
			"skills/redmine/SKILL.md": "# Skill",
		},
	}

	script := BuildInitScript(instance, nil, nil, nil)
	wants := []string{
		"mkdir -p /data/workspace/'agents'",
		"mkdir -p /data/workspace/'skills/redmine'",
		"[ -f /data/workspace/'agents/AGENT.md' ] || cp /workspace-init/'agents--AGENT.md' /data/workspace/'agents/AGENT.md'",
		"[ -f /data/workspace/'skills/redmine/SKILL.md' ] || cp /workspace-init/'skills--redmine--SKILL.md' /data/workspace/'skills/redmine/SKILL.md'",
	}
	for _, w := range wants {
		if !strings.Contains(script, w) {
			t.Errorf("script missing %q\n%s", w, script)
		}
	}
}

// TestBuildInitScript_AdditionalWorkspaceNestedFiles verifies nested seeding
// works for additionalWorkspaces too (#482).
func TestBuildInitScript_AdditionalWorkspaceNestedFiles(t *testing.T) {
	instance := newTestInstance("init-aw-nested")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{
				Name: "research",
				InitialFiles: map[string]string{
					"docs/PLAN.md": "# Plan",
				},
			},
		},
	}

	script := BuildInitScript(instance, nil, nil, nil)
	expectedCMKey := AdditionalWorkspaceCMKey("research", "docs--PLAN.md")
	wants := []string{
		"mkdir -p /data/'workspace-research'/'docs'",
		"[ -f /data/'workspace-research'/'docs/PLAN.md' ] || cp /workspace-init/'" + expectedCMKey + "' /data/'workspace-research'/'docs/PLAN.md'",
	}
	for _, w := range wants {
		if !strings.Contains(script, w) {
			t.Errorf("script missing %q\n%s", w, script)
		}
	}
}

func TestBuildInitScript_FilesOnly_MkdirWorkspace(t *testing.T) {
	// Regression test: files without directories must still mkdir /data/workspace
	// so that cp doesn't fail on first run with emptyDir.
	instance := newTestInstance("init-files-only")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{
			"README.md": "hello",
		},
	}

	script := BuildInitScript(instance, nil, nil, nil)
	if !strings.Contains(script, "mkdir -p /data/workspace\n") {
		t.Errorf("script should contain mkdir -p /data/workspace, got:\n%s", script)
	}
}

func TestBuildInitScript_VanillaDeployment(t *testing.T) {
	instance := newTestInstance("init-empty")
	script := BuildInitScript(instance, nil, nil, nil)
	// Vanilla deployments get config copy + ENVIRONMENT.md seeding
	expected := "cp /config/'openclaw.json' /data/openclaw.json\n" + operatorSeedLines
	if script != expected {
		t.Errorf("unexpected script:\ngot:  %q\nwant: %q", script, expected)
	}
}

// ---------------------------------------------------------------------------
// Config hash includes workspace
// ---------------------------------------------------------------------------

func TestConfigHash_StableWithWorkspace(t *testing.T) {
	instance := newTestInstance("hash-ws")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}

	dep1 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash1 := dep1.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{"SOUL.md": "hello"},
	}

	dep2 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash2 := dep2.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	if hash1 != hash2 {
		t.Error("config hash should not change for workspace-only changes (delivered via ConfigMap volume)")
	}
}

func TestConfigHash_StableWithFileContent(t *testing.T) {
	instance := newTestInstance("hash-content")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{"SOUL.md": "v1"},
	}

	dep1 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash1 := dep1.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	instance.Spec.Workspace.InitialFiles["SOUL.md"] = "v2"

	dep2 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash2 := dep2.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	if hash1 != hash2 {
		t.Error("config hash should not change for workspace file content changes (delivered via ConfigMap volume)")
	}
}

// ---------------------------------------------------------------------------
// Workspace volume and volume mount tests
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_WorkspaceVolume(t *testing.T) {
	instance := newTestInstance("ws-vol")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{"SOUL.md": "hello"},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Verify workspace-init volume exists
	wsVol := findVolume(sts.Spec.Template.Spec.Volumes, "workspace-init")
	if wsVol == nil {
		t.Fatal("workspace-init volume not found")
	}
	if wsVol.ConfigMap == nil {
		t.Fatal("workspace-init volume should use ConfigMap")
	}
	if wsVol.ConfigMap.Name != "ws-vol-workspace" {
		t.Errorf("workspace-init ConfigMap name = %q, want %q", wsVol.ConfigMap.Name, "ws-vol-workspace")
	}

	// Verify init container has workspace-init mount
	init := sts.Spec.Template.Spec.InitContainers[0]
	assertVolumeMount(t, init.VolumeMounts, "workspace-init", "/workspace-init")
}

func TestBuildStatefulSet_AlwaysHasWorkspaceVolume(t *testing.T) {
	instance := newTestInstance("no-ws-vol")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// workspace-init volume always exists because ENVIRONMENT.md is always injected
	wsVol := findVolume(sts.Spec.Template.Spec.Volumes, "workspace-init")
	if wsVol == nil {
		t.Error("workspace-init volume should always exist (ENVIRONMENT.md is always injected)")
	}
}

func TestBuildStatefulSet_WorkspaceDirsOnly_StillHasVolume(t *testing.T) {
	instance := newTestInstance("ws-dirs-no-vol")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialDirectories: []string{"memory"},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// workspace-init volume always exists because ENVIRONMENT.md is always injected
	wsVol := findVolume(sts.Spec.Template.Spec.Volumes, "workspace-init")
	if wsVol == nil {
		t.Error("workspace-init volume should exist (ENVIRONMENT.md is always injected)")
	}

	// Init container should exist (for mkdir + file seeding)
	if len(sts.Spec.Template.Spec.InitContainers) == 0 {
		t.Fatal("expected init container for workspace directories")
	}
}

func TestBuildStatefulSet_Idempotent_WithWorkspace(t *testing.T) {
	instance := newTestInstance("idempotent-ws")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{"key":"val"}`)},
	}
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles:       map[string]string{"SOUL.md": "hello", "AGENTS.md": "agents"},
		InitialDirectories: []string{"memory", "tools"},
	}

	dep1 := BuildStatefulSet(instance, "", nil, nil, nil)
	dep2 := BuildStatefulSet(instance, "", nil, nil, nil)

	b1, _ := json.Marshal(dep1.Spec)
	b2, _ := json.Marshal(dep2.Spec)

	if !bytes.Equal(b1, b2) {
		t.Error("BuildStatefulSet with workspace is not idempotent")
	}
}

// ---------------------------------------------------------------------------
// Feature 1: ReadOnlyRootFilesystem tests
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_ReadOnlyRootFilesystem_Default(t *testing.T) {
	instance := newTestInstance("rorfs-default")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	csc := main.SecurityContext
	if csc.ReadOnlyRootFilesystem == nil || !*csc.ReadOnlyRootFilesystem {
		t.Error("readOnlyRootFilesystem should default to true")
	}
}

func TestBuildStatefulSet_ReadOnlyRootFilesystem_ExplicitFalse(t *testing.T) {
	instance := newTestInstance("rorfs-false")
	instance.Spec.Security.ContainerSecurityContext = &openclawv1alpha1.ContainerSecurityContextSpec{
		ReadOnlyRootFilesystem: Ptr(false),
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	if main.SecurityContext.ReadOnlyRootFilesystem == nil || *main.SecurityContext.ReadOnlyRootFilesystem {
		t.Error("readOnlyRootFilesystem should be false when explicitly overridden")
	}
}

func TestBuildStatefulSet_WritablePVCSubPaths(t *testing.T) {
	instance := newTestInstance("writable-subpaths")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	// Verify ~/.local, ~/.cache, and ~/.config are mounted as PVC subPaths.
	// ~/.local and ~/.cache cover pip/npm/uv package installs; ~/.config is
	// required for tools (e.g. Codex CLI, ACP runtimes) that write config to
	// ~/.config/<tool>/ on startup under readOnlyRootFilesystem: true.
	// See https://github.com/paperclipinc/openclaw-operator/issues/456.
	wantMounts := []struct {
		mountPath string
		subPath   string
	}{
		{"/home/openclaw/.local", ".local"},
		{"/home/openclaw/.cache", ".cache"},
		{"/home/openclaw/.config", ".config"},
	}
	for _, want := range wantMounts {
		found := false
		for _, m := range main.VolumeMounts {
			if m.Name == "data" && m.MountPath == want.mountPath {
				found = true
				if m.SubPath != want.subPath {
					t.Errorf("mount %s: subPath = %q, want %q", want.mountPath, m.SubPath, want.subPath)
				}
				break
			}
		}
		if !found {
			t.Errorf("expected PVC subPath mount at %s not found", want.mountPath)
		}
	}
}

// TestBuildStatefulSet_InitUvPipMountFullDataVolume verifies that init-uv and
// init-pip mount the data PVC in full (not via SubPath .local). This is
// required so that on hostPath-backed storage (where fsGroup is not applied),
// the init containers create .local, .cache, and skills subdirectories with
// the pod UID as owner. Every downstream container that mounts these paths via
// SubPath inherits the correct ownership from the pre-created directory.
// See https://github.com/paperclipinc/openclaw-operator/issues/448.
func TestBuildStatefulSet_InitUvPipMountFullDataVolume(t *testing.T) {
	instance := newTestInstance("init-uv-hostpath")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var initUv, initPip *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		c := &sts.Spec.Template.Spec.InitContainers[i]
		switch c.Name {
		case "init-uv":
			initUv = c
		case "init-pip":
			initPip = c
		}
	}
	if initUv == nil {
		t.Fatal("init-uv container not found")
	}
	if initPip == nil {
		t.Fatal("init-pip container not found")
	}

	// Both init containers must mount data in full at /data with no SubPath,
	// otherwise kubelet creates the missing SubPath dir as root:root on
	// hostPath PVCs and the non-root init container cannot write to it.
	assertDataMountFull := func(t *testing.T, c *corev1.Container) {
		t.Helper()
		var found bool
		for _, m := range c.VolumeMounts {
			if m.Name != "data" {
				continue
			}
			if m.SubPath != "" {
				t.Errorf("%s data mount must not use SubPath (got %q): SubPath dirs are created root:root by kubelet on hostPath PVCs", c.Name, m.SubPath)
			}
			if m.MountPath != "/data" {
				t.Errorf("%s data mount path = %q, want %q", c.Name, m.MountPath, "/data")
			}
			found = true
		}
		if !found {
			t.Errorf("%s: data volume mount not found", c.Name)
		}
	}
	assertDataMountFull(t, initUv)
	assertDataMountFull(t, initPip)

	// init-uv must pre-create all SubPath subdirectories so every downstream
	// container (main, init-pip, init-skills, ...) inherits UID ownership.
	script := ""
	if len(initUv.Command) >= 3 {
		script = initUv.Command[2]
	}
	for _, dir := range []string{"/data/.local/bin", "/data/.cache", "/data/skills"} {
		if !strings.Contains(script, dir) {
			t.Errorf("init-uv script should pre-create %s, got:\n%s", dir, script)
		}
	}

	// init-pip must set HOME=/data so ensurepip --user writes to /data/.local
	// (not /home/openclaw/.local, which would be an unmounted path now).
	var homeEnv string
	for _, e := range initPip.Env {
		if e.Name == "HOME" {
			homeEnv = e.Value
		}
	}
	if homeEnv != "/data" {
		t.Errorf("init-pip HOME = %q, want %q (so ~/.local resolves to /data/.local)", homeEnv, "/data")
	}
}

// TestBuildStatefulSet_PluginRuntimeDepsSymlink verifies that every StatefulSet
// includes the init-plugin-runtime-deps container, which points the bundled-plugin
// openclaw import at the current container image. Without this symlink, bundled
// plugins (e.g. discord) crash loop after an image upgrade because they resolve
// "openclaw" against the stale npm-cached package on the PVC.
// See https://github.com/paperclipinc/openclaw-operator/issues/462.
func TestBuildStatefulSet_PluginRuntimeDepsSymlink(t *testing.T) {
	instance := newTestInstance("plugin-runtime-deps")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var c *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "init-plugin-runtime-deps" {
			c = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if c == nil {
		t.Fatal("init-plugin-runtime-deps container not found")
	}

	// Must use the OpenClaw image so /app resolves correctly in the main container
	// (the symlink target is evaluated at access time in whatever container dereferences it).
	if c.Image != GetImage(instance) {
		t.Errorf("init-plugin-runtime-deps image = %q, want %q", c.Image, GetImage(instance))
	}

	// Must mount data at /data in full (not via SubPath). The symlink must land at
	// /data/plugin-runtime-deps/node_modules/openclaw so that, in the main
	// container (data mounted at /home/openclaw/.openclaw), Node's ESM resolver
	// finds ~/.openclaw/plugin-runtime-deps/node_modules/openclaw.
	assertVolumeMount(t, c.VolumeMounts, "data", "/data")
	for _, m := range c.VolumeMounts {
		if m.Name == "data" && m.SubPath != "" {
			t.Errorf("init-plugin-runtime-deps data mount must not use SubPath, got %q", m.SubPath)
		}
	}

	// Script must be idempotent (mkdir -p + ln -sfn) so re-running on every pod
	// start is safe, and must create the version-agnostic symlink.
	if len(c.Command) != 3 {
		t.Fatalf("init-plugin-runtime-deps command = %v, want [sh -c <script>]", c.Command)
	}
	script := c.Command[2]
	if !strings.Contains(script, "mkdir -p /data/plugin-runtime-deps/node_modules") {
		t.Errorf("script must create plugin-runtime-deps/node_modules, got:\n%s", script)
	}
	if !strings.Contains(script, "ln -sfn /app /data/plugin-runtime-deps/node_modules/openclaw") {
		t.Errorf("script must symlink openclaw -> /app, got:\n%s", script)
	}

	// Security context should match the other read-only init containers.
	if c.SecurityContext == nil {
		t.Fatal("init-plugin-runtime-deps security context is nil")
	}
	if c.SecurityContext.ReadOnlyRootFilesystem == nil || !*c.SecurityContext.ReadOnlyRootFilesystem {
		t.Error("init-plugin-runtime-deps should have ReadOnlyRootFilesystem=true")
	}
}

func TestBuildStatefulSet_TmpVolumeAndMount(t *testing.T) {
	instance := newTestInstance("tmp-vol")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Check /tmp volume mount on main container
	main := sts.Spec.Template.Spec.Containers[0]
	assertVolumeMount(t, main.VolumeMounts, "tmp", "/tmp")

	// Check tmp volume exists as emptyDir
	tmpVol := findVolume(sts.Spec.Template.Spec.Volumes, "tmp")
	if tmpVol == nil {
		t.Fatal("tmp volume not found")
	}
	if tmpVol.EmptyDir == nil {
		t.Error("tmp volume should be emptyDir")
	}
}

// ---------------------------------------------------------------------------
// Feature 2: Config merge mode tests
// ---------------------------------------------------------------------------

func TestBuildInitScript_OverwriteMode(t *testing.T) {
	instance := newTestInstance("init-overwrite")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}
	instance.Spec.Config.MergeMode = "overwrite"

	script := BuildInitScript(instance, nil, nil, nil)
	if !strings.Contains(script, "cp /config/") {
		t.Errorf("overwrite mode should use cp, got: %q", script)
	}
	if strings.Contains(script, "node -e") {
		t.Error("overwrite mode should not use node deep merge")
	}
}

func TestBuildInitScript_MergeMode(t *testing.T) {
	instance := newTestInstance("init-merge")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}
	instance.Spec.Config.MergeMode = ConfigMergeModeMerge

	script := BuildInitScript(instance, nil, nil, nil)
	// Merge now uses Node.js deep merge instead of jq (jq image is distroless, no shell)
	if !strings.Contains(script, "node -e") {
		t.Errorf("merge mode should use node deep merge, got: %q", script)
	}
	if !strings.Contains(script, "fs.existsSync") {
		t.Errorf("merge mode should check for existing file via fs.existsSync, got: %q", script)
	}
	if !strings.Contains(script, "/tmp/merged.json") {
		t.Errorf("merge mode should write to /tmp/merged.json atomically, got: %q", script)
	}
	// Regression: renameSync fails across mount boundaries (EXDEV) - #120
	if strings.Contains(script, "renameSync") {
		t.Errorf("merge mode must not use renameSync (fails cross-device between /tmp and /data), got: %q", script)
	}
	if !strings.Contains(script, "copyFileSync") {
		t.Errorf("merge mode should use copyFileSync to move merged config to /data, got: %q", script)
	}
	// Regression #162: node -e argument must be single-quoted so that
	// !Array.isArray is not interpreted as bash history expansion.
	if !strings.Contains(script, "node -e '") {
		t.Errorf("merge mode init script must single-quote the node -e argument to avoid bash history expansion (#162), got %q", script)
	}
}

func TestBuildStatefulSet_MergeMode_OpenClawImage(t *testing.T) {
	instance := newTestInstance("merge-oci")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}
	instance.Spec.Config.MergeMode = ConfigMergeModeMerge

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	initContainers := sts.Spec.Template.Spec.InitContainers
	if len(initContainers) == 0 {
		t.Fatal("expected init container for merge mode")
	}

	initC := initContainers[0]
	wantImage := GetImage(instance)
	if initC.Image != wantImage {
		t.Errorf("merge mode init container image = %q, want %q", initC.Image, wantImage)
	}

	// Should have init-tmp mount
	assertVolumeMount(t, initC.VolumeMounts, "init-tmp", "/tmp")

	// Should have HOME and NPM_CONFIG_CACHE env vars
	var hasHome, hasNpm bool
	for _, e := range initC.Env {
		if e.Name == "HOME" && e.Value == "/tmp" {
			hasHome = true
		}
		if e.Name == "NPM_CONFIG_CACHE" && e.Value == "/tmp/.npm" {
			hasNpm = true
		}
	}
	if !hasHome {
		t.Error("merge mode init container should have HOME=/tmp")
	}
	if !hasNpm {
		t.Error("merge mode init container should have NPM_CONFIG_CACHE=/tmp/.npm")
	}

	// Should have writable rootfs (Node.js may need it)
	if initC.SecurityContext.ReadOnlyRootFilesystem == nil || *initC.SecurityContext.ReadOnlyRootFilesystem {
		t.Error("merge mode init container should have writable rootfs")
	}
}

func TestBuildStatefulSet_OverwriteMode_BusyboxImage(t *testing.T) {
	instance := newTestInstance("overwrite-bb")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}
	instance.Spec.Config.MergeMode = "overwrite"

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	initContainers := sts.Spec.Template.Spec.InitContainers
	if len(initContainers) == 0 {
		t.Fatal("expected init container for overwrite mode")
	}

	initC := initContainers[0]
	if initC.Image != "busybox:1.37" {
		t.Errorf("overwrite mode init container image = %q, want busybox:1.37", initC.Image)
	}
}

func TestBuildStatefulSet_MergeMode_InitTmpVolume(t *testing.T) {
	instance := newTestInstance("merge-vol")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}
	instance.Spec.Config.MergeMode = ConfigMergeModeMerge

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	initTmpVol := findVolume(sts.Spec.Template.Spec.Volumes, "init-tmp")
	if initTmpVol == nil {
		t.Fatal("init-tmp volume not found in merge mode")
	}
	if initTmpVol.EmptyDir == nil {
		t.Error("init-tmp volume should be emptyDir")
	}
}

func TestBuildStatefulSet_OverwriteMode_NoInitTmpVolume(t *testing.T) {
	instance := newTestInstance("overwrite-vol")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}
	instance.Spec.Config.MergeMode = "overwrite"

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	initTmpVol := findVolume(sts.Spec.Template.Spec.Volumes, "init-tmp")
	if initTmpVol != nil {
		t.Error("init-tmp volume should not exist in overwrite mode")
	}
}

func TestBuildInitScript_MergeMode_NoConfig(t *testing.T) {
	instance := newTestInstance("merge-no-cfg")
	instance.Spec.Config.MergeMode = ConfigMergeModeMerge
	// No raw config set — but operator always creates a ConfigMap (gateway.bind)

	script := BuildInitScript(instance, nil, nil, nil)
	// Should produce a merge script since configMapKey() now always returns "openclaw.json"
	if !strings.Contains(script, "node -e") {
		t.Errorf("merge mode should produce node deep merge script, got: %q", script)
	}
}

func TestBuildInitScript_MergeMode_NoForcePaths(t *testing.T) {
	// When forcePaths is unset, the __forcepaths env must still be a valid
	// empty JSON array so the merge script's JSON.parse doesn't blow up.
	instance := newTestInstance("merge-no-fp")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}
	instance.Spec.Config.MergeMode = ConfigMergeModeMerge

	script := BuildInitScript(instance, nil, nil, nil)
	if !strings.Contains(script, `__forcepaths='[]'`) {
		t.Errorf("merge mode with no forcePaths should set __forcepaths='[]', got: %q", script)
	}
	// Delete-by-path helper must still be present so the merge script is
	// structurally identical regardless of whether forcePaths is empty.
	if !strings.Contains(script, "function dp(o,p)") {
		t.Errorf("merge script must define dp() even when forcePaths is empty, got: %q", script)
	}
}

func TestBuildInitScript_MergeMode_WithForcePaths(t *testing.T) {
	instance := newTestInstance("merge-fp")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}
	instance.Spec.Config.MergeMode = ConfigMergeModeMerge
	instance.Spec.Config.ForcePaths = []string{"gateway", "models.providers", "agents.defaults.sandbox"}

	script := BuildInitScript(instance, nil, nil, nil)
	// Each path must appear inside the JSON-encoded env var, in order.
	wantEnv := `__forcepaths='["gateway","models.providers","agents.defaults.sandbox"]'`
	if !strings.Contains(script, wantEnv) {
		t.Errorf("merge script missing forcePaths env, want it to contain %q\ngot: %s", wantEnv, script)
	}
	// The deletion loop must precede the deep merge -- otherwise a
	// force-deleted subtree could be re-merged in from the existing PVC
	// config before being overwritten, leaving stale partial state.
	dpLoop := "for(const p of fp)dp(base,p);"
	mergeCall := "dm(base,inc)"
	dpIdx := strings.Index(script, dpLoop)
	mergeIdx := strings.Index(script, mergeCall)
	if dpIdx == -1 {
		t.Fatalf("merge script missing forcePaths deletion loop, got: %s", script)
	}
	if mergeIdx == -1 {
		t.Fatalf("merge script missing deep-merge call, got: %s", script)
	}
	if dpIdx >= mergeIdx {
		t.Errorf("forcePaths deletion loop must run before deep merge (dp at %d, dm at %d)", dpIdx, mergeIdx)
	}
}

func TestBuildInitScript_OverwriteMode_IgnoresForcePaths(t *testing.T) {
	// Under overwrite mode, forcePaths is rejected by the webhook -- but if
	// it leaks through (e.g. an older webhook deployment) the script
	// generator must still produce a valid overwrite script and never embed
	// the forcePaths machinery, because there is no merge to apply them to.
	instance := newTestInstance("ow-with-fp")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}
	instance.Spec.Config.MergeMode = "overwrite"
	instance.Spec.Config.ForcePaths = []string{"gateway"}

	script := BuildInitScript(instance, nil, nil, nil)
	if !strings.Contains(script, "cp /config/") {
		t.Errorf("overwrite mode should use cp, got: %q", script)
	}
	if strings.Contains(script, "__forcepaths") {
		t.Errorf("overwrite mode must not embed __forcepaths env var, got: %q", script)
	}
}

// ---------------------------------------------------------------------------
// Feature 3: Declarative skill installation tests
// ---------------------------------------------------------------------------

func TestNormalizeClawHubSlug(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"bare slug", "mcp-server-fetch", "mcp-server-fetch"},
		{"@owner/slug preserved", "@anthropic/mcp-server-fetch", "@anthropic/mcp-server-fetch"},
		{"owner/slug gains @", "anthropic/mcp-server-fetch", "@anthropic/mcp-server-fetch"},
		{"ambiguous @owner/slug preserved", "@ivangdavila/excel-xlsx", "@ivangdavila/excel-xlsx"},
		{"@slug (no owner) trimmed to bare", "@mcp-server-fetch", "mcp-server-fetch"},
		{"nested path preserved", "@org/sub/skill-name", "@org/sub/skill-name"},
		{"npm: passthrough", "npm:@openclaw/matrix", "npm:@openclaw/matrix"},
		{"pack: passthrough", "pack:paperclipinc/skills/image-gen", "pack:paperclipinc/skills/image-gen"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeClawHubSlug(tt.input); got != tt.want {
				t.Errorf("normalizeClawHubSlug(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseSkillEntry_ClawHub(t *testing.T) {
	// Owner-qualified refs must be passed through verbatim so ClawHub can
	// disambiguate slugs shared across owners (#558).
	got := parseSkillEntry("@anthropic/mcp-server-fetch")
	want := "_install_skill '@anthropic/mcp-server-fetch'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseSkillEntry_ClawHub_AmbiguousOwnerQualified(t *testing.T) {
	// Regression for #558: ambiguous slugs must keep the @owner/ prefix.
	got := parseSkillEntry("@ivangdavila/excel-xlsx")
	want := "_install_skill '@ivangdavila/excel-xlsx'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseSkillEntry_ClawHub_OwnerQualifiedNoAt(t *testing.T) {
	// Defensive: "owner/slug" without a leading @ is normalized to "@owner/slug".
	got := parseSkillEntry("ivangdavila/excel-xlsx")
	want := "_install_skill '@ivangdavila/excel-xlsx'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseSkillEntry_ClawHub_BareSlug(t *testing.T) {
	got := parseSkillEntry("mcp-server-fetch")
	want := "_install_skill 'mcp-server-fetch'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseSkillEntry_Npm(t *testing.T) {
	got := parseSkillEntry("npm:@openclaw/matrix")
	want := "npm install -g '@openclaw/matrix'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParseSkillEntry_NpmUnscoped(t *testing.T) {
	got := parseSkillEntry("npm:some-package")
	want := "npm install -g 'some-package'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildSkillsScript_NoSkills(t *testing.T) {
	instance := newTestInstance("no-skills")
	script := BuildSkillsScript(instance)
	if script != "" {
		t.Errorf("expected empty script, got: %q", script)
	}
}

func TestBuildSkillsScript_WithSkills(t *testing.T) {
	instance := newTestInstance("with-skills")
	instance.Spec.Skills = []string{"@anthropic/mcp-server-fetch", "@github/copilot-skill"}

	script := BuildSkillsScript(instance)

	if !strings.HasPrefix(script, "set -e\n") {
		t.Error("script should start with set -e")
	}
	if !strings.Contains(script, clawHubSkillsSetup) {
		t.Error("script should contain clawhub skills setup (PVC redirect)")
	}
	if !strings.Contains(script, skillInstallWrapper) {
		t.Error("script should contain the _install_skill wrapper")
	}
	if !strings.Contains(script, "_install_skill '@anthropic/mcp-server-fetch'") {
		t.Error("script should contain _install_skill with the owner-qualified @anthropic/mcp-server-fetch preserved")
	}
	if !strings.Contains(script, "_install_skill '@github/copilot-skill'") {
		t.Error("script should contain _install_skill with the owner-qualified @github/copilot-skill preserved")
	}
}

func TestBuildSkillsScript_OwnerQualifiedAndBare(t *testing.T) {
	// #558: owner-qualified refs preserved verbatim, bare slugs stay bare, in one list.
	instance := newTestInstance("mixed-owner-bare")
	instance.Spec.Skills = []string{"@ivangdavila/excel-xlsx", "pandas"}

	script := BuildSkillsScript(instance)

	if !strings.Contains(script, "_install_skill '@ivangdavila/excel-xlsx'") {
		t.Error("script should preserve the owner-qualified @ivangdavila/excel-xlsx ref")
	}
	if !strings.Contains(script, "_install_skill 'pandas'") {
		t.Error("script should keep the bare pandas slug bare")
	}
	// The bare slug must not accidentally acquire an @ prefix.
	if strings.Contains(script, "_install_skill '@pandas'") {
		t.Error("bare slug pandas must not be rewritten to @pandas")
	}
}

func TestBuildSkillsScript_MixedPrefixes(t *testing.T) {
	instance := newTestInstance("mixed-skills")
	instance.Spec.Skills = []string{
		"npm:@openclaw/matrix",
		"@anthropic/mcp-server-fetch",
		"npm:some-tool",
	}

	script := BuildSkillsScript(instance)

	if !strings.HasPrefix(script, "set -e\n") {
		t.Error("script should start with set -e")
	}
	if !strings.Contains(script, clawHubSkillsSetup) {
		t.Error("script should contain clawhub skills setup (PVC redirect)")
	}
	if !strings.Contains(script, skillInstallWrapper) {
		t.Error("script should contain the _install_skill wrapper (has clawhub skills)")
	}
	if !strings.Contains(script, "_install_skill '@anthropic/mcp-server-fetch'") {
		t.Error("script should contain _install_skill with the owner-qualified clawhub ref preserved")
	}
	if !strings.Contains(script, "npm install -g '@openclaw/matrix'") {
		t.Error("script should contain npm install -g for @openclaw/matrix")
	}
	if !strings.Contains(script, "npm install -g 'some-tool'") {
		t.Error("script should contain npm install -g for some-tool")
	}
}

func TestBuildSkillsScript_OnlyNpmSkills_NoWrapper(t *testing.T) {
	instance := newTestInstance("npm-only")
	instance.Spec.Skills = []string{"npm:@openclaw/matrix", "npm:some-tool"}

	script := BuildSkillsScript(instance)

	if !strings.HasPrefix(script, "set -e\n") {
		t.Error("script should start with set -e")
	}
	if strings.Contains(script, "_install_skill") {
		t.Error("script should not contain _install_skill wrapper when only npm skills")
	}
	if strings.Contains(script, clawHubSkillsSetup) {
		t.Error("script should not contain clawhub skills setup when only npm skills")
	}
	if !strings.Contains(script, "npm install") {
		t.Error("script should contain npm install commands")
	}
}

func TestHasClawHubSkills(t *testing.T) {
	tests := []struct {
		name   string
		skills []string
		want   bool
	}{
		{"empty", nil, false},
		{"only npm", []string{"npm:foo", "npm:bar"}, false},
		{"only clawhub", []string{"@anthropic/mcp-server-fetch"}, true},
		{"mixed", []string{"npm:foo", "@anthropic/mcp-server-fetch"}, true},
		{"only packs", []string{"pack:owner/repo/path"}, true}, // pack: is not npm:, so true
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasClawHubSkills(tt.skills); got != tt.want {
				t.Errorf("hasClawHubSkills(%v) = %v, want %v", tt.skills, got, tt.want)
			}
		})
	}
}

func TestBuildSkillsScript_WrapperOrdering(t *testing.T) {
	instance := newTestInstance("ordering")
	instance.Spec.Skills = []string{"@anthropic/mcp-server-fetch"}

	script := BuildSkillsScript(instance)

	setEIdx := strings.Index(script, "set -e")
	setupIdx := strings.Index(script, "mkdir -p /home/openclaw/.openclaw/skills")
	wrapperIdx := strings.Index(script, "_install_skill()")
	installIdx := strings.Index(script, "_install_skill '@anthropic/mcp-server-fetch'")

	if setEIdx == -1 || setupIdx == -1 || wrapperIdx == -1 || installIdx == -1 {
		t.Fatalf("missing expected content in script:\n%s", script)
	}
	if setEIdx >= setupIdx {
		t.Error("set -e must come before the skills setup")
	}
	if setupIdx >= wrapperIdx {
		t.Error("skills setup must come before the wrapper function")
	}
	if wrapperIdx >= installIdx {
		t.Error("wrapper function must come before install commands")
	}
}

func TestBuildStatefulSet_NoSkills_NoInitSkillsContainer(t *testing.T) {
	instance := newTestInstance("no-skills-sts")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	for _, c := range sts.Spec.Template.Spec.InitContainers {
		if c.Name == "init-skills" {
			t.Error("init-skills container should not exist without skills")
		}
	}
}

func TestBuildStatefulSet_WithSkills_InitSkillsContainer(t *testing.T) {
	instance := newTestInstance("skills-sts")
	instance.Spec.Skills = []string{"@anthropic/mcp-server-fetch"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var skillsContainer *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "init-skills" {
			skillsContainer = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if skillsContainer == nil {
		t.Fatal("init-skills container not found")
	}

	// Should use same image as main container
	expectedImage := GetImage(instance)
	if skillsContainer.Image != expectedImage {
		t.Errorf("init-skills image = %q, want %q", skillsContainer.Image, expectedImage)
	}

	// Should have HOME and NPM_CONFIG_CACHE env vars
	envMap := map[string]string{}
	for _, e := range skillsContainer.Env {
		envMap[e.Name] = e.Value
	}
	if envMap["HOME"] != "/tmp" {
		t.Errorf("init-skills HOME = %q, want /tmp", envMap["HOME"])
	}
	if envMap["NPM_CONFIG_CACHE"] != "/tmp/.npm" {
		t.Errorf("init-skills NPM_CONFIG_CACHE = %q, want /tmp/.npm", envMap["NPM_CONFIG_CACHE"])
	}

	// Should have data and skills-tmp mounts
	assertVolumeMount(t, skillsContainer.VolumeMounts, "data", "/home/openclaw/.openclaw")
	assertVolumeMount(t, skillsContainer.VolumeMounts, "skills-tmp", "/tmp")

	// Security context should be restricted
	sc := skillsContainer.SecurityContext
	if sc == nil {
		t.Fatal("init-skills security context is nil")
	}
	if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		t.Error("init-skills: allowPrivilegeEscalation should be false")
	}
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Error("init-skills: runAsNonRoot should be true")
	}
	if sc.SeccompProfile == nil || sc.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Error("init-skills: seccomp profile should be RuntimeDefault")
	}

	// NPM_CONFIG_IGNORE_SCRIPTS must be set to mitigate supply chain attacks (#91)
	if envMap["NPM_CONFIG_IGNORE_SCRIPTS"] != "true" {
		t.Errorf("init-skills NPM_CONFIG_IGNORE_SCRIPTS = %q, want \"true\"", envMap["NPM_CONFIG_IGNORE_SCRIPTS"])
	}
}

func TestBuildStatefulSet_WithNpmSkill_InitSkillsScript(t *testing.T) {
	instance := newTestInstance("npm-skill-sts")
	instance.Spec.Skills = []string{"npm:@openclaw/matrix"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var skillsContainer *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "init-skills" {
			skillsContainer = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if skillsContainer == nil {
		t.Fatal("init-skills container not found")
	}

	// Command should use npm install, not clawhub
	script := skillsContainer.Command[2]
	if !strings.Contains(script, "npm install") {
		t.Errorf("expected npm install in script, got: %q", script)
	}
	if strings.Contains(script, "clawhub") {
		t.Errorf("npm: prefixed skill should not use clawhub, got: %q", script)
	}
}

func TestBuildStatefulSet_WithSkills_EnvAndEnvFromPropagated(t *testing.T) {
	instance := newTestInstance("skills-env")
	instance.Spec.Skills = []string{"@anthropic/mcp-server-fetch"}
	instance.Spec.Env = []corev1.EnvVar{
		{Name: "CLAWHUB_TOKEN", Value: "secret-token"},
	}
	instance.Spec.EnvFrom = []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: "vault-secrets"},
			},
		},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var skillsContainer *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "init-skills" {
			skillsContainer = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if skillsContainer == nil {
		t.Fatal("init-skills container not found")
	}

	// Hardcoded env vars should come first (take precedence)
	names := envNames(skillsContainer.Env)
	if len(names) < 5 {
		t.Fatalf("expected at least 5 env vars, got %d: %v", len(names), names)
	}
	if names[0] != "HOME" || names[1] != "NPM_CONFIG_CACHE" || names[2] != "NPM_CONFIG_PREFIX" || names[3] != "NPM_CONFIG_IGNORE_SCRIPTS" {
		t.Errorf("hardcoded env vars should come first, got %v", names[:4])
	}

	// User-defined env var should be appended after hardcoded ones
	if names[len(names)-1] != "CLAWHUB_TOKEN" {
		t.Errorf("user-defined CLAWHUB_TOKEN should be last, got %v", names)
	}

	// EnvFrom should be propagated
	if len(skillsContainer.EnvFrom) != 1 {
		t.Fatalf("expected 1 envFrom source, got %d", len(skillsContainer.EnvFrom))
	}
	if skillsContainer.EnvFrom[0].SecretRef.Name != "vault-secrets" {
		t.Errorf("envFrom secretRef = %q, want vault-secrets", skillsContainer.EnvFrom[0].SecretRef.Name)
	}
}

func TestBuildStatefulSet_WithSkills_SkillsTmpVolume(t *testing.T) {
	instance := newTestInstance("skills-vol")
	instance.Spec.Skills = []string{"some-skill"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	skillsTmpVol := findVolume(sts.Spec.Template.Spec.Volumes, "skills-tmp")
	if skillsTmpVol == nil {
		t.Fatal("skills-tmp volume not found")
	}
	if skillsTmpVol.EmptyDir == nil {
		t.Error("skills-tmp volume should be emptyDir")
	}
}

func TestBuildStatefulSet_NoSkills_NoSkillsTmpVolume(t *testing.T) {
	instance := newTestInstance("no-skills-vol")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	skillsTmpVol := findVolume(sts.Spec.Template.Spec.Volumes, "skills-tmp")
	if skillsTmpVol != nil {
		t.Error("skills-tmp volume should not exist without skills")
	}
}

func TestBuildStatefulSet_ClawHubSkills_MainContainerSkillsMount(t *testing.T) {
	instance := newTestInstance("clawhub-skills-mount")
	instance.Spec.Skills = []string{"@anthropic/mcp-server-fetch"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var mainContainer *corev1.Container
	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == "openclaw" {
			mainContainer = &sts.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if mainContainer == nil {
		t.Fatal("main container not found")
	}

	// ClawHub skills should cause a PVC subpath mount at /app/skills (#313)
	var found bool
	for _, m := range mainContainer.VolumeMounts {
		if m.MountPath == "/app/skills" {
			found = true
			if m.Name != "data" {
				t.Errorf("skills mount volume name = %q, want \"data\"", m.Name)
			}
			if m.SubPath != "skills" {
				t.Errorf("skills mount subpath = %q, want \"skills\"", m.SubPath)
			}
			break
		}
	}
	if !found {
		t.Error("main container should have /app/skills volume mount for ClawHub skills")
	}
}

func TestBuildStatefulSet_NpmSkills_PathIncludesLocalBin(t *testing.T) {
	instance := newTestInstance("npm-path")
	instance.Spec.Skills = []string{"npm:mcporter"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	var pathVar *corev1.EnvVar
	for i := range main.Env {
		if main.Env[i].Name == "PATH" {
			pathVar = &main.Env[i]
			break
		}
	}
	if pathVar == nil {
		t.Fatal("PATH env var should be set when npm skills are configured")
	}
	if !strings.Contains(pathVar.Value, RuntimeDepsLocalBin) {
		t.Errorf("PATH should contain %q for npm skill binaries, got %q", RuntimeDepsLocalBin, pathVar.Value)
	}
}

func TestBuildStatefulSet_ClawHubOnlySkills_NoPathOverride(t *testing.T) {
	instance := newTestInstance("clawhub-no-path")
	instance.Spec.Skills = []string{"@anthropic/mcp-server-fetch"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	for i := range main.Env {
		if main.Env[i].Name == "PATH" {
			t.Errorf("PATH should not be set for clawhub-only skills (no runtime deps), got %q", main.Env[i].Value)
			return
		}
	}
}

func TestBuildStatefulSet_MixedSkills_PathIncludesLocalBin(t *testing.T) {
	instance := newTestInstance("mixed-skills-path")
	instance.Spec.Skills = []string{"@anthropic/mcp-server-fetch", "npm:mcporter"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	var pathVar *corev1.EnvVar
	for i := range main.Env {
		if main.Env[i].Name == "PATH" {
			pathVar = &main.Env[i]
			break
		}
	}
	if pathVar == nil {
		t.Fatal("PATH env var should be set when npm skills are configured")
	}
	if !strings.Contains(pathVar.Value, RuntimeDepsLocalBin) {
		t.Errorf("PATH should contain %q, got %q", RuntimeDepsLocalBin, pathVar.Value)
	}
}

func TestBuildStatefulSet_NpmSkills_InitContainerUsesGlobalInstall(t *testing.T) {
	instance := newTestInstance("npm-global")
	instance.Spec.Skills = []string{"npm:mcporter"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var skillsContainer *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "init-skills" {
			skillsContainer = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if skillsContainer == nil {
		t.Fatal("init-skills container not found")
	}

	// Should use npm install -g (global), not local install
	script := skillsContainer.Command[2]
	if !strings.Contains(script, "npm install -g") {
		t.Errorf("expected global npm install in script, got: %q", script)
	}

	// NPM_CONFIG_PREFIX should redirect global installs to PVC .local dir
	envMap := map[string]string{}
	for _, e := range skillsContainer.Env {
		envMap[e.Name] = e.Value
	}
	if envMap["NPM_CONFIG_PREFIX"] != "/home/openclaw/.openclaw/.local" {
		t.Errorf("NPM_CONFIG_PREFIX = %q, want %q", envMap["NPM_CONFIG_PREFIX"], "/home/openclaw/.openclaw/.local")
	}
}

func TestHasNpmSkills(t *testing.T) {
	tests := []struct {
		name   string
		skills []string
		want   bool
	}{
		{"empty", nil, false},
		{"clawhub only", []string{"@anthropic/fetch"}, false},
		{"npm only", []string{"npm:mcporter"}, true},
		{"mixed", []string{"@anthropic/fetch", "npm:mcporter"}, true},
		{"pack only", []string{"pack:owner/repo/path"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasNpmSkills(tt.skills); got != tt.want {
				t.Errorf("hasNpmSkills(%v) = %v, want %v", tt.skills, got, tt.want)
			}
		})
	}
}

func TestBuildStatefulSet_NpmOnlySkills_NoMainSkillsMount(t *testing.T) {
	instance := newTestInstance("npm-only-mount")
	instance.Spec.Skills = []string{"npm:@openclaw/matrix"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var mainContainer *corev1.Container
	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == "openclaw" {
			mainContainer = &sts.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if mainContainer == nil {
		t.Fatal("main container not found")
	}

	for _, m := range mainContainer.VolumeMounts {
		if m.MountPath == "/app/skills" {
			t.Error("main container should NOT have /app/skills mount when only npm skills")
		}
	}
}

func TestConfigHash_ChangesWithSkills(t *testing.T) {
	instance := newTestInstance("hash-skills")

	dep1 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash1 := dep1.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	instance.Spec.Skills = []string{"new-skill"}

	dep2 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash2 := dep2.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	if hash1 == hash2 {
		t.Error("config hash should change when skills are added")
	}
}

func TestBuildStatefulSet_SkillsOnly_HasBothInitContainers(t *testing.T) {
	instance := newTestInstance("skills-only")
	instance.Spec.Skills = []string{"some-skill"}
	// No raw config set — but operator always creates config (gateway.bind)

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Should have init-config container (gateway.bind=lan config)
	foundConfig := false
	foundSkills := false
	for _, c := range sts.Spec.Template.Spec.InitContainers {
		if c.Name == "init-config" {
			foundConfig = true
		}
		if c.Name == "init-skills" {
			foundSkills = true
		}
	}
	if !foundConfig {
		t.Error("init-config container should exist (gateway.bind config)")
	}
	if !foundSkills {
		t.Error("init-skills container should exist with skills defined")
	}
}

// Feature: Declarative plugin installation tests (issue #372)
// ---------------------------------------------------------------------------

func TestParsePluginEntry_Basic(t *testing.T) {
	got := parsePluginEntry("@martian-engineering/lossless-claw")
	want := "openclaw plugins install --force 'clawhub:@martian-engineering/lossless-claw'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParsePluginEntry_WithNpmPrefix(t *testing.T) {
	got := parsePluginEntry("npm:@openclaw/some-plugin")
	want := "openclaw plugins install --force 'npm:@openclaw/some-plugin'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParsePluginEntry_Unscoped(t *testing.T) {
	got := parsePluginEntry("some-plugin")
	want := "openclaw plugins install --force 'clawhub:some-plugin'"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestParsePluginEntry_Versioned(t *testing.T) {
	cases := []struct {
		entry, want string
	}{
		{"brave-plugin@1.2.3", "openclaw plugins install --force 'clawhub:brave-plugin@1.2.3'"},
		{"@openclaw/brave-plugin@1.2.3", "openclaw plugins install --force 'clawhub:@openclaw/brave-plugin@1.2.3'"},
		{"npm:@openclaw/brave-plugin@1.2.3", "openclaw plugins install --force 'npm:@openclaw/brave-plugin@1.2.3'"},
	}
	for _, c := range cases {
		if got := parsePluginEntry(c.entry); got != c.want {
			t.Errorf("parsePluginEntry(%q) = %q, want %q", c.entry, got, c.want)
		}
	}
}

func TestBuildPluginsScript_NoPlugins(t *testing.T) {
	instance := newTestInstance("no-plugins")
	script := BuildPluginsScript(instance)
	if script != "" {
		t.Errorf("expected empty script, got: %q", script)
	}
}

func TestBuildPluginsScript_WithPlugins(t *testing.T) {
	instance := newTestInstance("with-plugins")
	instance.Spec.Plugins = []string{"@martian-engineering/lossless-claw", "another-plugin"}

	script := BuildPluginsScript(instance)

	if !strings.HasPrefix(script, "set -e\n") {
		t.Error("script should start with set -e")
	}
	// Issue #474: plugins must be installed under ~/.openclaw/extensions/, not node_modules
	if !strings.Contains(script, "mkdir -p /home/openclaw/.openclaw/extensions") {
		t.Error("script should ensure ~/.openclaw/extensions exists")
	}
	if !strings.Contains(script, "openclaw plugins install --force 'clawhub:@martian-engineering/lossless-claw'") {
		t.Errorf("script should install lossless-claw via openclaw CLI, got:\n%s", script)
	}
	if !strings.Contains(script, "openclaw plugins install --force 'clawhub:another-plugin'") {
		t.Errorf("script should install another-plugin via openclaw CLI, got:\n%s", script)
	}
	// Regression guard: don't emit the old broken layout
	if strings.Contains(script, "cd /home/openclaw/.openclaw && npm install") {
		t.Error("script must not install plugins into ~/.openclaw/node_modules (issue #474)")
	}
	// Regression guard #505: must not call raw `npm install` or `npm pack`,
	// which fail on workspace:* deps used by first-party plugins like matrix.
	if strings.Contains(script, "npm pack") || strings.Contains(script, "npm install") {
		t.Errorf("script must not invoke raw npm install/pack (issue #505), got:\n%s", script)
	}
}

func TestBuildPluginsScript_UsesClawhubCLI(t *testing.T) {
	instance := newTestInstance("clawhub-plugins")
	instance.Spec.Plugins = []string{"@openclaw/matrix"}

	script := BuildPluginsScript(instance)

	// The CLI handles workspace:* dependency markers correctly, whereas raw
	// `npm install` rejects them with EUNSUPPORTEDPROTOCOL (#505).
	want := "openclaw plugins install --force 'clawhub:@openclaw/matrix'"
	if !strings.Contains(script, want) {
		t.Errorf("expected %q in script, got:\n%s", want, script)
	}
}

func TestBuildPluginsScript_Deterministic(t *testing.T) {
	instance := newTestInstance("deterministic-plugins")
	instance.Spec.Plugins = []string{"z-plugin", "a-plugin"}

	script := BuildPluginsScript(instance)

	// Find the lines that invoke the install command for our plugins.
	var pluginCalls []string
	for _, line := range strings.Split(script, "\n") {
		if strings.HasPrefix(line, "openclaw plugins install ") {
			pluginCalls = append(pluginCalls, line)
		}
	}
	if len(pluginCalls) != 2 {
		t.Fatalf("expected 2 install calls, got %d: %v", len(pluginCalls), pluginCalls)
	}
	if !strings.Contains(pluginCalls[0], "'clawhub:a-plugin'") {
		t.Errorf("expected a-plugin first, got %q", pluginCalls[0])
	}
	if !strings.Contains(pluginCalls[1], "'clawhub:z-plugin'") {
		t.Errorf("expected z-plugin second, got %q", pluginCalls[1])
	}
}

func TestBuildStatefulSet_NoPlugins_NoInitPluginsContainer(t *testing.T) {
	instance := newTestInstance("no-plugins-sts")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	for _, c := range sts.Spec.Template.Spec.InitContainers {
		if c.Name == "init-plugins" {
			t.Error("init-plugins container should not exist without plugins")
		}
	}
}

func TestBuildStatefulSet_WithPlugins_InitPluginsContainer(t *testing.T) {
	instance := newTestInstance("plugins-sts")
	instance.Spec.Plugins = []string{"@martian-engineering/lossless-claw"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var pluginsContainer *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "init-plugins" {
			pluginsContainer = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if pluginsContainer == nil {
		t.Fatal("init-plugins container not found")
	}

	// Should use same image as main container
	expectedImage := GetImage(instance)
	if pluginsContainer.Image != expectedImage {
		t.Errorf("init-plugins image = %q, want %q", pluginsContainer.Image, expectedImage)
	}

	// HOME must point at the OpenClaw user dir so the `openclaw plugins
	// install` CLI finds its data directory (#505); NPM_CONFIG_PREFIX and
	// NPM_CONFIG_CACHE point at PVC subpaths so the CLI's npm work persists
	// the same way the main container expects.
	envMap := map[string]string{}
	for _, e := range pluginsContainer.Env {
		envMap[e.Name] = e.Value
	}
	if envMap["HOME"] != "/home/openclaw" {
		t.Errorf("init-plugins HOME = %q, want /home/openclaw", envMap["HOME"])
	}
	if envMap["NPM_CONFIG_PREFIX"] != "/home/openclaw/.local" {
		t.Errorf("init-plugins NPM_CONFIG_PREFIX = %q, want /home/openclaw/.local", envMap["NPM_CONFIG_PREFIX"])
	}
	if envMap["NPM_CONFIG_CACHE"] != "/home/openclaw/.cache/npm" {
		t.Errorf("init-plugins NPM_CONFIG_CACHE = %q, want /home/openclaw/.cache/npm", envMap["NPM_CONFIG_CACHE"])
	}

	// Should have data, .local and .cache subpath mounts, plus plugins-tmp.
	// `data` is mounted at three different paths, so check (name, path)
	// tuples explicitly rather than going through assertVolumeMount (which
	// stops at the first name match).
	type mp struct{ name, path string }
	have := map[mp]bool{}
	for _, m := range pluginsContainer.VolumeMounts {
		have[mp{m.Name, m.MountPath}] = true
	}
	for _, want := range []mp{
		{"data", "/home/openclaw/.openclaw"},
		{"data", "/home/openclaw/.local"},
		{"data", "/home/openclaw/.cache"},
		{"plugins-tmp", "/tmp"},
	} {
		if !have[want] {
			t.Errorf("missing volume mount %s at %s", want.name, want.path)
		}
	}

	// Security context should be restricted
	sc := pluginsContainer.SecurityContext
	if sc == nil {
		t.Fatal("init-plugins security context is nil")
	}
	if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		t.Error("init-plugins: allowPrivilegeEscalation should be false")
	}
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Error("init-plugins: runAsNonRoot should be true")
	}
	if sc.ReadOnlyRootFilesystem == nil || *sc.ReadOnlyRootFilesystem {
		t.Error("init-plugins: readOnlyRootFilesystem should be false (npm needs writable node_modules)")
	}

	// NPM_CONFIG_IGNORE_SCRIPTS must be set
	if envMap["NPM_CONFIG_IGNORE_SCRIPTS"] != "true" {
		t.Errorf("init-plugins NPM_CONFIG_IGNORE_SCRIPTS = %q, want \"true\"", envMap["NPM_CONFIG_IGNORE_SCRIPTS"])
	}

	// Script should invoke the openclaw CLI (#505)
	script := pluginsContainer.Command[2]
	if !strings.Contains(script, "openclaw plugins install --force") {
		t.Errorf("expected openclaw plugins install --force in script, got: %q", script)
	}
}

func TestBuildStatefulSet_WithPlugins_EnvAndEnvFromPropagated(t *testing.T) {
	instance := newTestInstance("plugins-env")
	instance.Spec.Plugins = []string{"some-plugin"}
	instance.Spec.Env = []corev1.EnvVar{
		{Name: "NPM_TOKEN", Value: "secret-token"},
	}
	instance.Spec.EnvFrom = []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: "vault-secrets"},
			},
		},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var pluginsContainer *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "init-plugins" {
			pluginsContainer = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if pluginsContainer == nil {
		t.Fatal("init-plugins container not found")
	}

	// Hardcoded env vars should come first (take precedence)
	names := envNames(pluginsContainer.Env)
	if len(names) < 5 {
		t.Fatalf("expected at least 5 env vars, got %d: %v", len(names), names)
	}
	wantPrefix := []string{"HOME", "NPM_CONFIG_PREFIX", "NPM_CONFIG_CACHE", "NPM_CONFIG_IGNORE_SCRIPTS"}
	for i, want := range wantPrefix {
		if names[i] != want {
			t.Errorf("hardcoded env var %d = %q, want %q (full: %v)", i, names[i], want, names[:len(wantPrefix)])
		}
	}

	// User-defined env var should be appended after hardcoded ones
	if names[len(names)-1] != "NPM_TOKEN" {
		t.Errorf("user-defined NPM_TOKEN should be last, got %v", names)
	}

	// EnvFrom should be propagated
	if len(pluginsContainer.EnvFrom) != 1 {
		t.Fatalf("expected 1 envFrom source, got %d", len(pluginsContainer.EnvFrom))
	}
	if pluginsContainer.EnvFrom[0].SecretRef.Name != "vault-secrets" {
		t.Errorf("envFrom secretRef = %q, want vault-secrets", pluginsContainer.EnvFrom[0].SecretRef.Name)
	}
}

func TestBuildStatefulSet_WithPlugins_PluginsTmpVolume(t *testing.T) {
	instance := newTestInstance("plugins-vol")
	instance.Spec.Plugins = []string{"some-plugin"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	pluginsTmpVol := findVolume(sts.Spec.Template.Spec.Volumes, "plugins-tmp")
	if pluginsTmpVol == nil {
		t.Fatal("plugins-tmp volume not found")
	}
	if pluginsTmpVol.EmptyDir == nil {
		t.Error("plugins-tmp volume should be emptyDir")
	}
}

func TestBuildStatefulSet_NoPlugins_NoPluginsTmpVolume(t *testing.T) {
	instance := newTestInstance("no-plugins-vol")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	pluginsTmpVol := findVolume(sts.Spec.Template.Spec.Volumes, "plugins-tmp")
	if pluginsTmpVol != nil {
		t.Error("plugins-tmp volume should not exist without plugins")
	}
}

func TestConfigHash_ChangesWithPlugins(t *testing.T) {
	instance := newTestInstance("hash-plugins")

	dep1 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash1 := dep1.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	instance.Spec.Plugins = []string{"some-plugin"}

	dep2 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash2 := dep2.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	if hash1 == hash2 {
		t.Error("config hash should change when plugins are added")
	}
}

func TestBuildStatefulSet_InitContainerOrdering_SkillsThenPlugins(t *testing.T) {
	instance := newTestInstance("ordering-plugins")
	instance.Spec.Skills = []string{"some-skill"}
	instance.Spec.Plugins = []string{"some-plugin"}
	instance.Spec.InitContainers = []corev1.Container{
		{Name: "user-init", Image: "busybox:1.37"},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	initContainers := sts.Spec.Template.Spec.InitContainers

	// Find indices
	skillsIdx, pluginsIdx, userIdx := -1, -1, -1
	for i, c := range initContainers {
		switch c.Name {
		case "init-skills":
			skillsIdx = i
		case "init-plugins":
			pluginsIdx = i
		case "user-init":
			userIdx = i
		}
	}

	if skillsIdx < 0 || pluginsIdx < 0 || userIdx < 0 {
		names := make([]string, len(initContainers))
		for i, c := range initContainers {
			names[i] = c.Name
		}
		t.Fatalf("expected init-skills, init-plugins, and user-init containers, got %v", names)
	}
	if skillsIdx >= pluginsIdx {
		t.Error("init-skills should come before init-plugins")
	}
	if pluginsIdx >= userIdx {
		t.Error("init-plugins should come before user-init")
	}
}

func TestBuildStatefulSet_CABundle_InitPlugins(t *testing.T) {
	instance := newTestInstance("ca-plugins")
	instance.Spec.Plugins = []string{"some-plugin"}
	instance.Spec.Security.CABundle = &openclawv1alpha1.CABundleSpec{
		ConfigMapName: "my-ca",
		Key:           "ca.crt",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Find init-plugins container
	var initPlugins *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "init-plugins" {
			initPlugins = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if initPlugins == nil {
		t.Fatal("init-plugins container not found")
	}

	// Check mount
	assertVolumeMount(t, initPlugins.VolumeMounts, "ca-bundle", "/etc/ssl/certs/custom-ca-bundle.crt")

	// Check env
	foundEnv := false
	for _, env := range initPlugins.Env {
		if env.Name == "NODE_EXTRA_CA_CERTS" {
			foundEnv = true
		}
	}
	if !foundEnv {
		t.Error("NODE_EXTRA_CA_CERTS env var not found on init-plugins container")
	}
}

// ---------------------------------------------------------------------------
// secret.go tests — gateway token Secret
// ---------------------------------------------------------------------------

func TestGatewayTokenSecretName(t *testing.T) {
	instance := newTestInstance("my-app")
	got := GatewayTokenSecretName(instance)
	if got != "my-app-gateway-token" {
		t.Errorf("GatewayTokenSecretName() = %q, want %q", got, "my-app-gateway-token")
	}
}

func TestBuildGatewayTokenSecret(t *testing.T) {
	instance := newTestInstance("my-app")
	token := "abcdef1234567890abcdef1234567890"

	secret := BuildGatewayTokenSecret(instance, token)

	if secret.Name != "my-app-gateway-token" {
		t.Errorf("secret name = %q, want %q", secret.Name, "my-app-gateway-token")
	}
	if secret.Namespace != "test-ns" {
		t.Errorf("secret namespace = %q, want %q", secret.Namespace, "test-ns")
	}
	if secret.Labels["app.kubernetes.io/name"] != "openclaw" {
		t.Error("secret missing app label")
	}
	if string(secret.Data[GatewayTokenSecretKey]) != token {
		t.Errorf("secret data[%q] = %q, want %q", GatewayTokenSecretKey, string(secret.Data[GatewayTokenSecretKey]), token)
	}
}

// ---------------------------------------------------------------------------
// configmap.go tests — gateway auth enrichment
// ---------------------------------------------------------------------------

func TestEnrichConfigWithGatewayAuth_InjectsToken(t *testing.T) {
	configJSON := []byte(`{"channels":{"slack":{"enabled":true}}}`)
	token := "my-test-token"

	result, err := enrichConfigWithGatewayAuth(configJSON, token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	gw, ok := parsed["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway key in config")
	}
	auth, ok := gw["auth"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway.auth key in config")
	}
	if auth["mode"] != "token" {
		t.Errorf("gateway.auth.mode = %v, want %q", auth["mode"], "token")
	}
	if auth["token"] != token {
		t.Errorf("gateway.auth.token = %v, want %q", auth["token"], token)
	}
}

func TestEnrichConfigWithGatewayAuth_PreservesUserToken(t *testing.T) {
	configJSON := []byte(`{"gateway":{"auth":{"mode":"token","token":"user-token-123"}}}`)
	token := "operator-generated-token"

	result, err := enrichConfigWithGatewayAuth(configJSON, token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	gw := parsed["gateway"].(map[string]interface{})
	auth := gw["auth"].(map[string]interface{})

	// User's token should be preserved, not overwritten
	if auth["token"] != "user-token-123" {
		t.Errorf("gateway.auth.token = %v, want %q (user's value should be preserved)", auth["token"], "user-token-123")
	}
}

func TestEnrichConfigWithGatewayAuth_EmptyConfig(t *testing.T) {
	configJSON := []byte(`{}`)
	token := "my-token"

	result, err := enrichConfigWithGatewayAuth(configJSON, token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(result, &parsed); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	gw := parsed["gateway"].(map[string]interface{})
	auth := gw["auth"].(map[string]interface{})
	if auth["mode"] != "token" {
		t.Errorf("gateway.auth.mode = %v, want %q", auth["mode"], "token")
	}
	if auth["token"] != "my-token" {
		t.Errorf("gateway.auth.token = %v, want %q", auth["token"], "my-token")
	}
}

func TestEnrichConfigWithGatewayAuth_InvalidJSON(t *testing.T) {
	configJSON := []byte(`not json`)
	token := "my-token"

	result, err := enrichConfigWithGatewayAuth(configJSON, token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return unchanged
	if !bytes.Equal(result, configJSON) {
		t.Errorf("expected unchanged result for invalid JSON, got %s", string(result))
	}
}

func TestEnrichConfigWithGatewayAuth_PreservesUserMode(t *testing.T) {
	configJSON := []byte(`{"gateway":{"auth":{"mode":"trusted-proxy"}}}`)
	token := "operator-generated-token"

	result, err := enrichConfigWithGatewayAuth(configJSON, token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// trusted-proxy mode is mutually exclusive with token auth, so the
	// config should be returned unchanged (no token injected).
	if !bytes.Equal(result, configJSON) {
		t.Errorf("config should be unchanged in trusted-proxy mode\ngot:  %s\nwant: %s", string(result), string(configJSON))
	}
}

func TestEnrichConfigWithGatewayAuth_PreservesUserModeAndToken(t *testing.T) {
	configJSON := []byte(`{"gateway":{"auth":{"mode":"trusted-proxy","token":"user-custom-token"}}}`)
	token := "operator-generated-token"

	result, err := enrichConfigWithGatewayAuth(configJSON, token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should be completely unchanged because user set their own token
	if !bytes.Equal(result, configJSON) {
		t.Errorf("config should be unchanged when user sets both mode and token\ngot:  %s\nwant: %s", string(result), string(configJSON))
	}
}

func TestEnrichConfigWithGatewayAuth_PreservesOtherAuthFields(t *testing.T) {
	configJSON := []byte(`{"gateway":{"auth":{"mode":"trusted-proxy","allowTailscale":true}}}`)
	token := "operator-token"

	result, err := enrichConfigWithGatewayAuth(configJSON, token)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// trusted-proxy mode is mutually exclusive with token auth, so the
	// config should be returned unchanged (no token injected, other
	// fields like allowTailscale preserved).
	if !bytes.Equal(result, configJSON) {
		t.Errorf("config should be unchanged in trusted-proxy mode\ngot:  %s\nwant: %s", string(result), string(configJSON))
	}
}

func TestIsGatewayAuthTrustedProxy(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   bool
	}{
		{"trusted-proxy mode", `{"gateway":{"auth":{"mode":"trusted-proxy"}}}`, true},
		{"token mode", `{"gateway":{"auth":{"mode":"token"}}}`, false},
		{"no mode set", `{"gateway":{"auth":{}}}`, false},
		{"no auth key", `{"gateway":{}}`, false},
		{"no gateway key", `{}`, false},
		{"empty config", ``, false},
		{"invalid JSON", `not-json`, false},
		{"trusted-proxy with extra fields", `{"gateway":{"auth":{"mode":"trusted-proxy","allowTailscale":true}}}`, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsGatewayAuthTrustedProxy([]byte(tt.config))
			if got != tt.want {
				t.Errorf("IsGatewayAuthTrustedProxy(%s) = %v, want %v", tt.config, got, tt.want)
			}
		})
	}
}

func TestBuildConfigMap_WithGatewayToken(t *testing.T) {
	instance := newTestInstance("gw-test")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"channels":{"slack":{"enabled":true}}}`),
		},
	}
	token := "abc123"

	cm := BuildConfigMap(instance, token, nil)

	configContent := cm.Data["openclaw.json"]
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(configContent), &parsed); err != nil {
		t.Fatalf("failed to parse ConfigMap data: %v", err)
	}

	gw, ok := parsed["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway key in ConfigMap config")
	}
	auth, ok := gw["auth"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway.auth key in ConfigMap config")
	}
	if auth["token"] != token {
		t.Errorf("gateway.auth.token = %v, want %q", auth["token"], token)
	}
}

func TestBuildConfigMap_WithGatewayToken_NoRawConfig(t *testing.T) {
	instance := newTestInstance("gw-noraw")
	// No raw config set
	token := "abc123"

	cm := BuildConfigMap(instance, token, nil)

	configContent := cm.Data["openclaw.json"]
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(configContent), &parsed); err != nil {
		t.Fatalf("failed to parse ConfigMap data: %v", err)
	}

	gw, ok := parsed["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway key in ConfigMap config even with no raw config")
	}
	auth, ok := gw["auth"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway.auth in ConfigMap config")
	}
	if auth["token"] != token {
		t.Errorf("gateway.auth.token = %v, want %q", auth["token"], token)
	}
	// bind=loopback should also be present
	if gw["bind"] != "loopback" {
		t.Errorf("gateway.bind = %v, want %q", gw["bind"], "loopback")
	}
}

func TestBuildConfigMap_EmptyGatewayToken(t *testing.T) {
	instance := newTestInstance("gw-empty")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"key":"value"}`),
		},
	}

	cm := BuildConfigMap(instance, "", nil)

	configContent := cm.Data["openclaw.json"]
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(configContent), &parsed); err != nil {
		t.Fatalf("failed to parse ConfigMap data: %v", err)
	}

	// gateway key IS present (gateway.bind=lan), but no auth when token is empty
	gw, ok := parsed["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("expected gateway key (bind is always injected)")
	}
	if gw["bind"] != "loopback" {
		t.Errorf("gateway.bind = %v, want %q", gw["bind"], "loopback")
	}
	if _, ok := gw["auth"]; ok {
		t.Error("gateway.auth should not be present when token is empty")
	}
}

// ---------------------------------------------------------------------------
// statefulset.go tests — gateway token env + Bonjour disable
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_DisableBonjour(t *testing.T) {
	instance := newTestInstance("bonjour-test")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	main := sts.Spec.Template.Spec.Containers[0]
	found := false
	for _, env := range main.Env {
		if env.Name == "OPENCLAW_DISABLE_BONJOUR" {
			found = true
			if env.Value != "1" {
				t.Errorf("OPENCLAW_DISABLE_BONJOUR = %q, want %q", env.Value, "1")
			}
			break
		}
	}
	if !found {
		t.Error("OPENCLAW_DISABLE_BONJOUR env var should always be present")
	}
}

func TestBuildStatefulSet_GatewayTokenEnv(t *testing.T) {
	instance := newTestInstance("gw-env-test")
	secretName := "gw-env-test-gateway-token"

	sts := BuildStatefulSet(instance, secretName, nil, nil, nil)

	main := sts.Spec.Template.Spec.Containers[0]
	var gwEnv *corev1.EnvVar
	for i := range main.Env {
		if main.Env[i].Name == "OPENCLAW_GATEWAY_TOKEN" {
			gwEnv = &main.Env[i]
			break
		}
	}

	if gwEnv == nil {
		t.Fatal("OPENCLAW_GATEWAY_TOKEN env var not found")
	}
	if gwEnv.ValueFrom == nil || gwEnv.ValueFrom.SecretKeyRef == nil {
		t.Fatal("OPENCLAW_GATEWAY_TOKEN should use SecretKeyRef")
	}
	if gwEnv.ValueFrom.SecretKeyRef.Name != secretName {
		t.Errorf("secret name = %q, want %q", gwEnv.ValueFrom.SecretKeyRef.Name, secretName)
	}
	if gwEnv.ValueFrom.SecretKeyRef.Key != GatewayTokenSecretKey {
		t.Errorf("secret key = %q, want %q", gwEnv.ValueFrom.SecretKeyRef.Key, GatewayTokenSecretKey)
	}
}

func TestBuildStatefulSet_GatewayTokenEnv_UserOverride(t *testing.T) {
	instance := newTestInstance("gw-override")
	instance.Spec.Env = []corev1.EnvVar{
		{Name: "OPENCLAW_GATEWAY_TOKEN", Value: "user-provided-token"},
	}
	secretName := "gw-override-gateway-token"

	sts := BuildStatefulSet(instance, secretName, nil, nil, nil)

	main := sts.Spec.Template.Spec.Containers[0]
	// Count occurrences of OPENCLAW_GATEWAY_TOKEN
	count := 0
	for _, env := range main.Env {
		if env.Name == "OPENCLAW_GATEWAY_TOKEN" {
			count++
			// The one present should be the user's value, not a SecretKeyRef
			if env.Value != "user-provided-token" {
				t.Errorf("OPENCLAW_GATEWAY_TOKEN value = %q, want %q (user's value)", env.Value, "user-provided-token")
			}
			if env.ValueFrom != nil {
				t.Error("user's OPENCLAW_GATEWAY_TOKEN should not use SecretKeyRef")
			}
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 OPENCLAW_GATEWAY_TOKEN env var, got %d", count)
	}
}

func TestBuildStatefulSet_ExistingSecret(t *testing.T) {
	instance := newTestInstance("existing-secret")
	instance.Spec.Gateway.ExistingSecret = "my-custom-secret"
	existingSecretName := "my-custom-secret"

	sts := BuildStatefulSet(instance, existingSecretName, nil, nil, nil)

	main := sts.Spec.Template.Spec.Containers[0]
	var gwEnv *corev1.EnvVar
	for i := range main.Env {
		if main.Env[i].Name == "OPENCLAW_GATEWAY_TOKEN" {
			gwEnv = &main.Env[i]
			break
		}
	}

	if gwEnv == nil {
		t.Fatal("OPENCLAW_GATEWAY_TOKEN env var not found")
	}
	if gwEnv.ValueFrom == nil || gwEnv.ValueFrom.SecretKeyRef == nil {
		t.Fatal("OPENCLAW_GATEWAY_TOKEN should use SecretKeyRef")
	}
	if gwEnv.ValueFrom.SecretKeyRef.Name != existingSecretName {
		t.Errorf("secret name = %q, want %q", gwEnv.ValueFrom.SecretKeyRef.Name, existingSecretName)
	}
	if gwEnv.ValueFrom.SecretKeyRef.Key != GatewayTokenSecretKey {
		t.Errorf("secret key = %q, want %q", gwEnv.ValueFrom.SecretKeyRef.Key, GatewayTokenSecretKey)
	}
}

func TestBuildStatefulSet_ExistingSecret_UserOverride(t *testing.T) {
	instance := newTestInstance("existing-secret-override")
	instance.Spec.Gateway.ExistingSecret = "my-custom-secret"
	instance.Spec.Env = []corev1.EnvVar{
		{Name: "OPENCLAW_GATEWAY_TOKEN", Value: "user-provided-token"},
	}

	sts := BuildStatefulSet(instance, "my-custom-secret", nil, nil, nil)

	main := sts.Spec.Template.Spec.Containers[0]
	count := 0
	for _, env := range main.Env {
		if env.Name == "OPENCLAW_GATEWAY_TOKEN" {
			count++
			if env.Value != "user-provided-token" {
				t.Errorf("OPENCLAW_GATEWAY_TOKEN value = %q, want %q", env.Value, "user-provided-token")
			}
			if env.ValueFrom != nil {
				t.Error("user's OPENCLAW_GATEWAY_TOKEN should not use SecretKeyRef")
			}
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 OPENCLAW_GATEWAY_TOKEN env var, got %d", count)
	}
}

func TestBuildStatefulSet_NoGatewayTokenSecretName(t *testing.T) {
	instance := newTestInstance("no-gw")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	main := sts.Spec.Template.Spec.Containers[0]
	for _, env := range main.Env {
		if env.Name == "OPENCLAW_GATEWAY_TOKEN" {
			t.Error("OPENCLAW_GATEWAY_TOKEN should not be present when no secret name is provided")
		}
	}
}

// TestBuildStatefulSet_TrustedProxy_NoGatewayTokenEnv verifies that when the
// controller detects trusted-proxy mode and passes an empty gwSecretName,
// the OPENCLAW_GATEWAY_TOKEN env var is not injected into the StatefulSet.
func TestBuildStatefulSet_TrustedProxy_NoGatewayTokenEnv(t *testing.T) {
	instance := newTestInstance("trusted-proxy-sts")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"gateway":{"auth":{"mode":"trusted-proxy"}}}`),
		},
	}

	// In trusted-proxy mode, the controller passes empty gwSecretName
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	main := sts.Spec.Template.Spec.Containers[0]
	for _, env := range main.Env {
		if env.Name == "OPENCLAW_GATEWAY_TOKEN" {
			t.Error("OPENCLAW_GATEWAY_TOKEN must not be present in trusted-proxy mode")
		}
	}
}

// ---------------------------------------------------------------------------
// Feature: fsGroupChangePolicy
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_FSGroupChangePolicy_Default(t *testing.T) {
	instance := newTestInstance("fsgcp-default")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	psc := sts.Spec.Template.Spec.SecurityContext
	if psc.FSGroupChangePolicy != nil {
		t.Errorf("FSGroupChangePolicy should be nil by default, got %v", *psc.FSGroupChangePolicy)
	}
}

func TestBuildStatefulSet_FSGroupChangePolicy_OnRootMismatch(t *testing.T) {
	instance := newTestInstance("fsgcp-onroot")
	policy := corev1.FSGroupChangeOnRootMismatch
	instance.Spec.Security.PodSecurityContext = &openclawv1alpha1.PodSecurityContextSpec{
		FSGroupChangePolicy: &policy,
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	psc := sts.Spec.Template.Spec.SecurityContext
	if psc.FSGroupChangePolicy == nil {
		t.Fatal("FSGroupChangePolicy should not be nil")
	}
	if *psc.FSGroupChangePolicy != corev1.FSGroupChangeOnRootMismatch {
		t.Errorf("FSGroupChangePolicy = %v, want OnRootMismatch", *psc.FSGroupChangePolicy)
	}
}

func TestBuildStatefulSet_FSGroupChangePolicy_Always(t *testing.T) {
	instance := newTestInstance("fsgcp-always")
	policy := corev1.FSGroupChangeAlways
	instance.Spec.Security.PodSecurityContext = &openclawv1alpha1.PodSecurityContextSpec{
		FSGroupChangePolicy: &policy,
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	psc := sts.Spec.Template.Spec.SecurityContext
	if psc.FSGroupChangePolicy == nil {
		t.Fatal("FSGroupChangePolicy should not be nil")
	}
	if *psc.FSGroupChangePolicy != corev1.FSGroupChangeAlways {
		t.Errorf("FSGroupChangePolicy = %v, want Always", *psc.FSGroupChangePolicy)
	}
}

// ---------------------------------------------------------------------------
// Feature: SA annotations
// ---------------------------------------------------------------------------

func TestBuildServiceAccount_NoAnnotations(t *testing.T) {
	instance := newTestInstance("sa-no-ann")
	sa := BuildServiceAccount(instance)
	if len(sa.Annotations) > 0 {
		t.Errorf("expected nil/empty annotations, got %v", sa.Annotations)
	}
}

func TestBuildServiceAccount_WithAnnotations(t *testing.T) {
	instance := newTestInstance("sa-ann")
	instance.Spec.Security.RBAC.ServiceAccountAnnotations = map[string]string{
		"eks.amazonaws.com/role-arn":     "arn:aws:iam::123456789:role/my-role",
		"iam.gke.io/gcp-service-account": "my-sa@my-project.iam.gserviceaccount.com",
	}

	sa := BuildServiceAccount(instance)
	if len(sa.Annotations) != 2 {
		t.Fatalf("expected 2 annotations, got %d", len(sa.Annotations))
	}
	if sa.Annotations["eks.amazonaws.com/role-arn"] != "arn:aws:iam::123456789:role/my-role" {
		t.Error("IRSA annotation not found")
	}
	if sa.Annotations["iam.gke.io/gcp-service-account"] != "my-sa@my-project.iam.gserviceaccount.com" {
		t.Error("GKE WI annotation not found")
	}
}

func TestBuildServiceAccount_AnnotationsDoNotAffectLabels(t *testing.T) {
	instance := newTestInstance("sa-ann-labels")
	instance.Spec.Security.RBAC.ServiceAccountAnnotations = map[string]string{
		"test": "value",
	}

	sa := BuildServiceAccount(instance)
	if sa.Labels["app.kubernetes.io/name"] != "openclaw" {
		t.Error("labels should still be set when annotations are present")
	}
}

// ---------------------------------------------------------------------------
// Feature: Extra volumes/mounts
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_ExtraVolumes_None(t *testing.T) {
	instance := newTestInstance("no-extras")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	for _, v := range sts.Spec.Template.Spec.Volumes {
		if v.Name == "my-extra" {
			t.Error("should not have extra volumes when none configured")
		}
	}
}

func TestBuildStatefulSet_ExtraVolumes(t *testing.T) {
	instance := newTestInstance("extras")
	instance.Spec.ExtraVolumes = []corev1.Volume{
		{
			Name: "ssh-keys",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{SecretName: "ssh-secret"},
			},
		},
	}
	instance.Spec.ExtraVolumeMounts = []corev1.VolumeMount{
		{Name: "ssh-keys", MountPath: "/home/openclaw/.ssh", ReadOnly: true},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Check volume exists
	vol := findVolume(sts.Spec.Template.Spec.Volumes, "ssh-keys")
	if vol == nil {
		t.Fatal("extra volume 'ssh-keys' not found")
	}
	if vol.Secret == nil || vol.Secret.SecretName != "ssh-secret" {
		t.Error("extra volume should reference ssh-secret")
	}

	// Check mount exists on main container
	main := sts.Spec.Template.Spec.Containers[0]
	assertVolumeMount(t, main.VolumeMounts, "ssh-keys", "/home/openclaw/.ssh")
}

func TestBuildStatefulSet_ExtraVolumes_DontInterfereWithExisting(t *testing.T) {
	instance := newTestInstance("extras-coexist")
	instance.Spec.ExtraVolumes = []corev1.Volume{
		{
			Name: "custom-vol",
			VolumeSource: corev1.VolumeSource{
				EmptyDir: &corev1.EmptyDirVolumeSource{},
			},
		},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	volumes := sts.Spec.Template.Spec.Volumes

	// Existing volumes should still be present
	if findVolume(volumes, "data") == nil {
		t.Error("data volume should still exist")
	}
	if findVolume(volumes, "tmp") == nil {
		t.Error("tmp volume should still exist")
	}
	if findVolume(volumes, "custom-vol") == nil {
		t.Error("extra volume should be appended")
	}
}

// ---------------------------------------------------------------------------
// Feature: CA bundle injection
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_CABundle_Nil(t *testing.T) {
	instance := newTestInstance("no-ca")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	if findVolume(sts.Spec.Template.Spec.Volumes, "ca-bundle") != nil {
		t.Error("ca-bundle volume should not exist when CABundle is nil")
	}
}

func TestBuildStatefulSet_CABundle_ConfigMap(t *testing.T) {
	instance := newTestInstance("ca-cm")
	instance.Spec.Security.CABundle = &openclawv1alpha1.CABundleSpec{
		ConfigMapName: "my-ca-bundle",
		Key:           "custom-ca.crt",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Volume
	vol := findVolume(sts.Spec.Template.Spec.Volumes, "ca-bundle")
	if vol == nil {
		t.Fatal("ca-bundle volume not found")
	}
	if vol.ConfigMap == nil {
		t.Fatal("ca-bundle volume should use ConfigMap")
	}
	if vol.ConfigMap.Name != "my-ca-bundle" {
		t.Errorf("ca-bundle configmap = %q, want %q", vol.ConfigMap.Name, "my-ca-bundle")
	}

	// Main container mount + env
	main := sts.Spec.Template.Spec.Containers[0]
	assertVolumeMount(t, main.VolumeMounts, "ca-bundle", "/etc/ssl/certs/custom-ca-bundle.crt")

	foundEnv := false
	for _, env := range main.Env {
		if env.Name == "NODE_EXTRA_CA_CERTS" {
			foundEnv = true
			if env.Value != "/etc/ssl/certs/custom-ca-bundle.crt" {
				t.Errorf("NODE_EXTRA_CA_CERTS = %q, want /etc/ssl/certs/custom-ca-bundle.crt", env.Value)
			}
		}
	}
	if !foundEnv {
		t.Error("NODE_EXTRA_CA_CERTS env var not found on main container")
	}
}

func TestBuildStatefulSet_CABundle_Secret(t *testing.T) {
	instance := newTestInstance("ca-secret")
	instance.Spec.Security.CABundle = &openclawv1alpha1.CABundleSpec{
		SecretName: "ca-secret",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	vol := findVolume(sts.Spec.Template.Spec.Volumes, "ca-bundle")
	if vol == nil {
		t.Fatal("ca-bundle volume not found")
	}
	if vol.Secret == nil {
		t.Fatal("ca-bundle volume should use Secret")
	}
	if vol.Secret.SecretName != "ca-secret" {
		t.Errorf("ca-bundle secret = %q, want %q", vol.Secret.SecretName, "ca-secret")
	}
}

func TestBuildStatefulSet_CABundle_DefaultKey(t *testing.T) {
	instance := newTestInstance("ca-default-key")
	instance.Spec.Security.CABundle = &openclawv1alpha1.CABundleSpec{
		ConfigMapName: "my-ca",
		// Key not set — should default to "ca-bundle.crt"
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	// Find the ca-bundle mount and check subPath
	for _, m := range main.VolumeMounts {
		if m.Name == "ca-bundle" {
			if m.SubPath != "ca-bundle.crt" {
				t.Errorf("subPath = %q, want %q", m.SubPath, "ca-bundle.crt")
			}
			return
		}
	}
	t.Error("ca-bundle volume mount not found")
}

func TestBuildStatefulSet_CABundle_WithChromium(t *testing.T) {
	instance := newTestInstance("ca-chromium")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Security.CABundle = &openclawv1alpha1.CABundleSpec{
		ConfigMapName: "my-ca",
		Key:           "ca.crt",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Find chromium in init containers (native sidecar)
	var chromium *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "chromium" {
			chromium = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if chromium == nil {
		t.Fatal("chromium init container not found")
	}

	// Check mount - Chrome picks up certs from /etc/ssl/certs/ directory
	assertVolumeMount(t, chromium.VolumeMounts, "ca-bundle", "/etc/ssl/certs/custom-ca-bundle.crt")

	// NODE_EXTRA_CA_CERTS should NOT be set (Chrome is not a Node.js app)
	for _, env := range chromium.Env {
		if env.Name == "NODE_EXTRA_CA_CERTS" {
			t.Error("NODE_EXTRA_CA_CERTS should not be set on chromium container (not Node.js)")
		}
	}
}

func TestBuildStatefulSet_CABundle_InitSkills(t *testing.T) {
	instance := newTestInstance("ca-skills")
	instance.Spec.Skills = []string{"some-skill"}
	instance.Spec.Security.CABundle = &openclawv1alpha1.CABundleSpec{
		ConfigMapName: "my-ca",
		Key:           "ca.crt",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Find init-skills container
	var initSkills *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "init-skills" {
			initSkills = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if initSkills == nil {
		t.Fatal("init-skills container not found")
	}

	// Check mount
	assertVolumeMount(t, initSkills.VolumeMounts, "ca-bundle", "/etc/ssl/certs/custom-ca-bundle.crt")

	// Check env
	foundEnv := false
	for _, env := range initSkills.Env {
		if env.Name == "NODE_EXTRA_CA_CERTS" {
			foundEnv = true
		}
	}
	if !foundEnv {
		t.Error("NODE_EXTRA_CA_CERTS env var not found on init-skills container")
	}
}

// ---------------------------------------------------------------------------
// Feature: Custom init containers
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_NoCustomInitContainers(t *testing.T) {
	instance := newTestInstance("no-custom-init")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	for _, c := range sts.Spec.Template.Spec.InitContainers {
		if c.Name == "my-init" {
			t.Error("should not have custom init containers when none configured")
		}
	}
}

func TestBuildStatefulSet_CustomInitContainers(t *testing.T) {
	instance := newTestInstance("custom-init")
	instance.Spec.InitContainers = []corev1.Container{
		{
			Name:    "my-init",
			Image:   "busybox:1.37",
			Command: []string{"echo", "hello"},
		},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Custom init container should be last
	initContainers := sts.Spec.Template.Spec.InitContainers
	if len(initContainers) == 0 {
		t.Fatal("expected at least one init container")
	}
	last := initContainers[len(initContainers)-1]
	if last.Name != "my-init" {
		t.Errorf("last init container = %q, want %q", last.Name, "my-init")
	}
	if last.Image != "busybox:1.37" {
		t.Errorf("custom init container image = %q, want %q", last.Image, "busybox:1.37")
	}
}

func TestBuildStatefulSet_CustomInitContainers_AfterOperatorManaged(t *testing.T) {
	instance := newTestInstance("custom-init-order")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}
	instance.Spec.Skills = []string{"some-skill"}
	instance.Spec.InitContainers = []corev1.Container{
		{Name: "user-init", Image: "busybox:1.37"},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	initContainers := sts.Spec.Template.Spec.InitContainers

	if len(initContainers) != 6 {
		t.Fatalf("expected 6 init containers, got %d", len(initContainers))
	}
	if initContainers[0].Name != "init-config" {
		t.Errorf("initContainers[0] = %q, want init-config", initContainers[0].Name)
	}
	if initContainers[1].Name != "init-uv" {
		t.Errorf("initContainers[1] = %q, want init-uv", initContainers[1].Name)
	}
	if initContainers[2].Name != "init-pip" {
		t.Errorf("initContainers[2] = %q, want init-pip", initContainers[2].Name)
	}
	if initContainers[3].Name != "init-plugin-runtime-deps" {
		t.Errorf("initContainers[3] = %q, want init-plugin-runtime-deps", initContainers[3].Name)
	}
	if initContainers[4].Name != "init-skills" {
		t.Errorf("initContainers[4] = %q, want init-skills", initContainers[4].Name)
	}
	if initContainers[5].Name != "user-init" {
		t.Errorf("initContainers[5] = %q, want user-init", initContainers[5].Name)
	}
}

func TestConfigHash_ChangesWithInitContainers(t *testing.T) {
	instance := newTestInstance("hash-ic")

	dep1 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash1 := dep1.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	instance.Spec.InitContainers = []corev1.Container{
		{Name: "my-init", Image: "busybox:1.37"},
	}

	dep2 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash2 := dep2.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	if hash1 == hash2 {
		t.Error("config hash should change when init containers are added")
	}
}

// ---------------------------------------------------------------------------
// Feature: JSON5 config support
// ---------------------------------------------------------------------------

func TestBuildInitScript_JSON5_Overwrite(t *testing.T) {
	instance := newTestInstance("json5-overwrite")
	instance.Spec.Config.ConfigMapRef = &openclawv1alpha1.ConfigMapKeySelector{
		Name: "my-config",
		Key:  "config.json5",
	}
	instance.Spec.Config.Format = ConfigFormatJSON5

	script := BuildInitScript(instance, nil, nil, nil)
	if !strings.Contains(script, "npx -y json5") {
		t.Errorf("JSON5 overwrite should use npx json5, got: %q", script)
	}
	if !strings.Contains(script, "/tmp/converted.json") {
		t.Errorf("JSON5 overwrite should write to /tmp/converted.json, got: %q", script)
	}
}

func TestBuildStatefulSet_JSON5_UsesOpenClawImage(t *testing.T) {
	instance := newTestInstance("json5-image")
	instance.Spec.Config.ConfigMapRef = &openclawv1alpha1.ConfigMapKeySelector{
		Name: "my-config",
		Key:  "config.json5",
	}
	instance.Spec.Config.Format = ConfigFormatJSON5

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	initContainers := sts.Spec.Template.Spec.InitContainers
	if len(initContainers) == 0 {
		t.Fatal("expected init container for JSON5 mode")
	}

	initC := initContainers[0]
	expectedImage := GetImage(instance)
	if initC.Image != expectedImage {
		t.Errorf("JSON5 init container image = %q, want %q", initC.Image, expectedImage)
	}
}

func TestBuildStatefulSet_JSON5_InitTmpVolume(t *testing.T) {
	instance := newTestInstance("json5-vol")
	instance.Spec.Config.ConfigMapRef = &openclawv1alpha1.ConfigMapKeySelector{
		Name: "my-config",
	}
	instance.Spec.Config.Format = ConfigFormatJSON5

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Should have init-tmp volume
	initTmpVol := findVolume(sts.Spec.Template.Spec.Volumes, "init-tmp")
	if initTmpVol == nil {
		t.Fatal("init-tmp volume not found in JSON5 mode")
	}

	// Should have init-tmp mount on init container
	initC := sts.Spec.Template.Spec.InitContainers[0]
	assertVolumeMount(t, initC.VolumeMounts, "init-tmp", "/tmp")
}

func TestBuildStatefulSet_JSON5_WritableRootFS(t *testing.T) {
	instance := newTestInstance("json5-writable")
	instance.Spec.Config.ConfigMapRef = &openclawv1alpha1.ConfigMapKeySelector{
		Name: "my-config",
	}
	instance.Spec.Config.Format = ConfigFormatJSON5

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	initC := sts.Spec.Template.Spec.InitContainers[0]

	if initC.SecurityContext.ReadOnlyRootFilesystem == nil || *initC.SecurityContext.ReadOnlyRootFilesystem {
		t.Error("JSON5 init container should have writable root filesystem for npx")
	}
}

func TestBuildInitScript_JSON_Overwrite_NoBusyboxRegression(t *testing.T) {
	instance := newTestInstance("json-overwrite")
	instance.Spec.Config.ConfigMapRef = &openclawv1alpha1.ConfigMapKeySelector{
		Name: "my-config",
	}
	instance.Spec.Config.Format = "json"

	script := BuildInitScript(instance, nil, nil, nil)
	if strings.Contains(script, "npx") {
		t.Errorf("JSON overwrite should not use npx, got: %q", script)
	}
	if !strings.Contains(script, "cp /config/") {
		t.Errorf("JSON overwrite should use cp, got: %q", script)
	}
}

// ---------------------------------------------------------------------------
// Feature: Runtime dependency init containers (pnpm, Python/uv)
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_RuntimeDeps_Pnpm(t *testing.T) {
	instance := newTestInstance("pnpm")
	instance.Spec.RuntimeDeps.Pnpm = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	initContainers := sts.Spec.Template.Spec.InitContainers

	// Should have init-pnpm container
	var pnpmContainer *corev1.Container
	for i := range initContainers {
		if initContainers[i].Name == "init-pnpm" {
			pnpmContainer = &initContainers[i]
			break
		}
	}
	if pnpmContainer == nil {
		t.Fatal("init-pnpm container not found")
	}

	// Should use the OpenClaw image (has Node.js + corepack)
	if pnpmContainer.Image != GetImage(instance) {
		t.Errorf("init-pnpm image = %q, want %q", pnpmContainer.Image, GetImage(instance))
	}

	// Should mount data volume
	assertVolumeMount(t, pnpmContainer.VolumeMounts, "data", "/home/openclaw/.openclaw")
	// Should mount pnpm-tmp volume
	assertVolumeMount(t, pnpmContainer.VolumeMounts, "pnpm-tmp", "/tmp")

	// Script should check for existing install (idempotent)
	script := pnpmContainer.Command[2]
	if !strings.Contains(script, "already installed") {
		t.Error("pnpm init script should check for existing install")
	}
	if !strings.Contains(script, "corepack enable pnpm") {
		t.Error("pnpm init script should use corepack")
	}

	// Security context
	if pnpmContainer.SecurityContext.ReadOnlyRootFilesystem == nil || *pnpmContainer.SecurityContext.ReadOnlyRootFilesystem {
		t.Error("init-pnpm should have writable root filesystem")
	}
	if pnpmContainer.SecurityContext.RunAsNonRoot == nil || !*pnpmContainer.SecurityContext.RunAsNonRoot {
		t.Error("init-pnpm should run as non-root")
	}

	// pnpm-tmp volume should exist
	pnpmTmpVol := findVolume(sts.Spec.Template.Spec.Volumes, "pnpm-tmp")
	if pnpmTmpVol == nil {
		t.Fatal("pnpm-tmp volume not found")
	}
	if pnpmTmpVol.EmptyDir == nil {
		t.Error("pnpm-tmp should be an emptyDir volume")
	}

	// PATH should be extended in main container
	mainContainer := sts.Spec.Template.Spec.Containers[0]
	var pathEnv *corev1.EnvVar
	for i := range mainContainer.Env {
		if mainContainer.Env[i].Name == "PATH" {
			pathEnv = &mainContainer.Env[i]
			break
		}
	}
	if pathEnv == nil {
		t.Fatal("PATH env var not found in main container")
	}
	if !strings.Contains(pathEnv.Value, RuntimeDepsLocalBin) {
		t.Errorf("PATH should contain %q, got %q", RuntimeDepsLocalBin, pathEnv.Value)
	}
}

func TestBuildStatefulSet_RuntimeDeps_Python(t *testing.T) {
	instance := newTestInstance("python")
	instance.Spec.RuntimeDeps.Python = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	initContainers := sts.Spec.Template.Spec.InitContainers

	// Should have init-python container
	var pythonContainer *corev1.Container
	for i := range initContainers {
		if initContainers[i].Name == "init-python" {
			pythonContainer = &initContainers[i]
			break
		}
	}
	if pythonContainer == nil {
		t.Fatal("init-python container not found")
	}

	// Should use the uv image
	if pythonContainer.Image != UvImage {
		t.Errorf("init-python image = %q, want %q", pythonContainer.Image, UvImage)
	}

	// Should mount data volume
	assertVolumeMount(t, pythonContainer.VolumeMounts, "data", "/home/openclaw/.openclaw")
	// Should mount python-tmp volume
	assertVolumeMount(t, pythonContainer.VolumeMounts, "python-tmp", "/tmp")

	// Script should check for existing install (idempotent)
	script := pythonContainer.Command[2]
	if !strings.Contains(script, "already installed") {
		t.Error("python init script should check for existing install")
	}
	if !strings.Contains(script, "uv python install 3.12") {
		t.Error("python init script should install Python 3.12")
	}
	if !strings.Contains(script, "cp /usr/local/bin/uv") {
		t.Error("python init script should copy uv binary")
	}

	// Security context
	if pythonContainer.SecurityContext.ReadOnlyRootFilesystem == nil || *pythonContainer.SecurityContext.ReadOnlyRootFilesystem {
		t.Error("init-python should have writable root filesystem")
	}
	if pythonContainer.SecurityContext.RunAsNonRoot == nil || !*pythonContainer.SecurityContext.RunAsNonRoot {
		t.Error("init-python should run as non-root")
	}

	// python-tmp volume should exist
	pythonTmpVol := findVolume(sts.Spec.Template.Spec.Volumes, "python-tmp")
	if pythonTmpVol == nil {
		t.Fatal("python-tmp volume not found")
	}

	// PATH should be extended in main container
	mainContainer := sts.Spec.Template.Spec.Containers[0]
	var pathEnv *corev1.EnvVar
	for i := range mainContainer.Env {
		if mainContainer.Env[i].Name == "PATH" {
			pathEnv = &mainContainer.Env[i]
			break
		}
	}
	if pathEnv == nil {
		t.Fatal("PATH env var not found in main container")
	}
}

func TestBuildStatefulSet_RuntimeDeps_Both(t *testing.T) {
	instance := newTestInstance("both-deps")
	instance.Spec.RuntimeDeps.Pnpm = true
	instance.Spec.RuntimeDeps.Python = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	initContainers := sts.Spec.Template.Spec.InitContainers

	var hasPnpm, hasPython bool
	var pnpmIdx, pythonIdx int
	for i, c := range initContainers {
		if c.Name == "init-pnpm" {
			hasPnpm = true
			pnpmIdx = i
		}
		if c.Name == "init-python" {
			hasPython = true
			pythonIdx = i
		}
	}
	if !hasPnpm {
		t.Error("init-pnpm not found")
	}
	if !hasPython {
		t.Error("init-python not found")
	}
	if hasPnpm && hasPython && pnpmIdx >= pythonIdx {
		t.Error("init-pnpm should come before init-python")
	}

	// Both tmp volumes should exist
	if findVolume(sts.Spec.Template.Spec.Volumes, "pnpm-tmp") == nil {
		t.Error("pnpm-tmp volume not found")
	}
	if findVolume(sts.Spec.Template.Spec.Volumes, "python-tmp") == nil {
		t.Error("python-tmp volume not found")
	}
}

func TestBuildStatefulSet_RuntimeDeps_None(t *testing.T) {
	instance := newTestInstance("no-deps")
	// RuntimeDeps defaults to zero value (both false)

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	initContainers := sts.Spec.Template.Spec.InitContainers

	for _, c := range initContainers {
		if c.Name == "init-pnpm" || c.Name == "init-python" {
			t.Errorf("unexpected runtime dep init container: %s", c.Name)
		}
	}

	// No runtime dep tmp volumes
	if findVolume(sts.Spec.Template.Spec.Volumes, "pnpm-tmp") != nil {
		t.Error("pnpm-tmp volume should not exist")
	}
	if findVolume(sts.Spec.Template.Spec.Volumes, "python-tmp") != nil {
		t.Error("python-tmp volume should not exist")
	}

	// No PATH override
	mainContainer := sts.Spec.Template.Spec.Containers[0]
	for _, e := range mainContainer.Env {
		if e.Name == "PATH" {
			t.Error("PATH env var should not be set when no runtime deps")
		}
	}
}

func TestBuildStatefulSet_RuntimeDeps_InitContainerOrder(t *testing.T) {
	instance := newTestInstance("order")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
	}
	instance.Spec.RuntimeDeps.Pnpm = true
	instance.Spec.RuntimeDeps.Python = true
	instance.Spec.Skills = []string{"some-skill"}
	instance.Spec.InitContainers = []corev1.Container{
		{Name: "user-init", Image: "busybox:1.37"},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	initContainers := sts.Spec.Template.Spec.InitContainers

	expected := []string{"init-config", "init-uv", "init-pip", "init-plugin-runtime-deps", "init-pnpm", "init-python", "init-skills", "user-init"}
	if len(initContainers) != len(expected) {
		t.Fatalf("expected %d init containers, got %d: %v", len(expected), len(initContainers),
			func() []string {
				names := make([]string, len(initContainers))
				for i, c := range initContainers {
					names[i] = c.Name
				}
				return names
			}())
	}
	for i, name := range expected {
		if initContainers[i].Name != name {
			t.Errorf("initContainers[%d] = %q, want %q", i, initContainers[i].Name, name)
		}
	}
}

func TestBuildStatefulSet_RuntimeDeps_Pnpm_CABundle(t *testing.T) {
	instance := newTestInstance("pnpm-ca")
	instance.Spec.RuntimeDeps.Pnpm = true
	instance.Spec.Security.CABundle = &openclawv1alpha1.CABundleSpec{
		ConfigMapName: "my-ca",
		Key:           "ca.crt",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	var pnpmContainer *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "init-pnpm" {
			pnpmContainer = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if pnpmContainer == nil {
		t.Fatal("init-pnpm container not found")
	}

	// Should have CA bundle mount
	assertVolumeMount(t, pnpmContainer.VolumeMounts, "ca-bundle", "/etc/ssl/certs/custom-ca-bundle.crt")

	// Should have NODE_EXTRA_CA_CERTS env
	var hasCAEnv bool
	for _, e := range pnpmContainer.Env {
		if e.Name == "NODE_EXTRA_CA_CERTS" {
			hasCAEnv = true
			break
		}
	}
	if !hasCAEnv {
		t.Error("init-pnpm should have NODE_EXTRA_CA_CERTS when CA bundle is configured")
	}
}

func TestConfigHash_ChangesWithRuntimeDeps(t *testing.T) {
	instance := newTestInstance("hash-rd")

	sts1 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash1 := sts1.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	instance.Spec.RuntimeDeps.Pnpm = true

	sts2 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash2 := sts2.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	if hash1 == hash2 {
		t.Error("config hash should change when runtime deps are enabled")
	}
}

// ---------------------------------------------------------------------------
// Tailscale sidecar integration tests
// ---------------------------------------------------------------------------

func TestBuildConfigMap_WithTailscale_NoTailscaleConfig(t *testing.T) {
	instance := newTestInstance("ts-serve")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.Mode = "serve"

	cm := BuildConfigMap(instance, "", nil)
	content, ok := cm.Data["openclaw.json"]
	if !ok {
		t.Fatal("ConfigMap should have openclaw.json key")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	// Sidecar handles serve/funnel - config should NOT have gateway.tailscale
	gw, ok := parsed["gateway"].(map[string]interface{})
	if ok {
		if _, hasTailscale := gw["tailscale"]; hasTailscale {
			t.Error("gateway.tailscale should NOT be set - sidecar handles serve/funnel via TS_SERVE_CONFIG")
		}
	}
}

func TestBuildConfigMap_TailscaleServeConfig(t *testing.T) {
	instance := newTestInstance("ts-serve-cfg")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.Mode = "serve"

	cm := BuildConfigMap(instance, "", nil)

	serveJSON, ok := cm.Data[TailscaleServeConfigKey]
	if !ok {
		t.Fatal("ConfigMap should have tailscale-serve.json key when Tailscale is enabled")
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(serveJSON), &cfg); err != nil {
		t.Fatalf("failed to parse tailscale serve config: %v", err)
	}

	tcp, ok := cfg["TCP"].(map[string]interface{})
	if !ok {
		t.Fatal("serve config should have TCP key")
	}
	handler, ok := tcp["443"].(map[string]interface{})
	if !ok {
		t.Fatal("serve config should have TCP.443")
	}
	if handler["HTTPS"] != true {
		t.Errorf("TCP.443.HTTPS = %v, want true", handler["HTTPS"])
	}

	// serve mode should NOT have AllowFunnel
	if _, hasFunnel := cfg["AllowFunnel"]; hasFunnel {
		t.Error("AllowFunnel should not be set in serve mode")
	}
}

func TestBuildConfigMap_TailscaleServeConfig_Funnel(t *testing.T) {
	instance := newTestInstance("ts-funnel-cfg")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.Mode = "funnel"

	cm := BuildConfigMap(instance, "", nil)

	serveJSON := cm.Data[TailscaleServeConfigKey]

	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(serveJSON), &cfg); err != nil {
		t.Fatalf("failed to parse tailscale serve config: %v", err)
	}

	// funnel mode should have AllowFunnel
	af, ok := cfg["AllowFunnel"].(map[string]interface{})
	if !ok {
		t.Fatal("AllowFunnel should be set in funnel mode")
	}
	if af["${TS_CERT_DOMAIN}:443"] != true {
		t.Error("AllowFunnel should enable ${TS_CERT_DOMAIN}:443")
	}
}

func TestBuildConfigMap_TailscaleDisabled_NoServeConfig(t *testing.T) {
	instance := newTestInstance("ts-disabled-cfg")

	cm := BuildConfigMap(instance, "", nil)

	if _, ok := cm.Data[TailscaleServeConfigKey]; ok {
		t.Error("tailscale-serve.json should not be present when Tailscale is disabled")
	}
}

func TestBuildConfigMap_TailscaleAuthSSO(t *testing.T) {
	instance := newTestInstance("ts-sso")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.AuthSSO = true

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	gw := parsed["gateway"].(map[string]interface{})
	auth, ok := gw["auth"].(map[string]interface{})
	if !ok {
		t.Fatal("gateway should have auth key when AuthSSO is enabled")
	}

	if auth["allowTailscale"] != true {
		t.Errorf("expected auth.allowTailscale=true, got %v", auth["allowTailscale"])
	}
}

func TestBuildConfigMap_TailscaleUserConfig_Preserved(t *testing.T) {
	instance := newTestInstance("ts-override")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.Mode = "serve"
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"gateway":{"tailscale":{"mode":"funnel","resetOnExit":false}}}`),
		},
	}

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	// User-set gateway.tailscale should be preserved (operator no longer injects it)
	gw := parsed["gateway"].(map[string]interface{})
	ts := gw["tailscale"].(map[string]interface{})

	if ts["mode"] != "funnel" {
		t.Errorf("user-set mode should be preserved, expected funnel, got %v", ts["mode"])
	}
	if ts["resetOnExit"] != false {
		t.Errorf("user-set resetOnExit should be preserved, expected false, got %v", ts["resetOnExit"])
	}
}

func TestBuildStatefulSet_TailscaleSidecar(t *testing.T) {
	instance := newTestInstance("ts-sidecar")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.AuthKeySecretRef = &corev1.LocalObjectReference{Name: "ts-auth"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	containers := sts.Spec.Template.Spec.Containers

	// Find the tailscale sidecar
	var tsSidecar *corev1.Container
	for i := range containers {
		if containers[i].Name == "tailscale" {
			tsSidecar = &containers[i]
			break
		}
	}
	if tsSidecar == nil {
		t.Fatal("tailscale sidecar container should be present")
	}

	// Check image
	if tsSidecar.Image != DefaultTailscaleImage+":"+DefaultImageTag {
		t.Errorf("tailscale image = %q, want %q", tsSidecar.Image, DefaultTailscaleImage+":"+DefaultImageTag)
	}

	// Check env vars on sidecar
	envMap := make(map[string]corev1.EnvVar)
	for _, e := range tsSidecar.Env {
		envMap[e.Name] = e
	}

	if envMap["TS_USERSPACE"].Value != "true" {
		t.Errorf("TS_USERSPACE = %q, want %q", envMap["TS_USERSPACE"].Value, "true")
	}
	if envMap["TS_STATE_DIR"].Value != TailscaleStatePath {
		t.Errorf("TS_STATE_DIR = %q, want %q", envMap["TS_STATE_DIR"].Value, TailscaleStatePath)
	}
	if envMap["TS_SOCKET"].Value != TailscaleSocketPath {
		t.Errorf("TS_SOCKET = %q, want %q", envMap["TS_SOCKET"].Value, TailscaleSocketPath)
	}
	if envMap["TS_HOSTNAME"].Value != "ts-sidecar" {
		t.Errorf("TS_HOSTNAME = %q, want %q", envMap["TS_HOSTNAME"].Value, "ts-sidecar")
	}
	expectedStateSecret := TailscaleStateSecretName(instance)
	if envMap["TS_KUBE_SECRET"].Value != expectedStateSecret {
		t.Errorf("TS_KUBE_SECRET = %q, want %q", envMap["TS_KUBE_SECRET"].Value, expectedStateSecret)
	}
	if _, hasKSH := envMap["KUBERNETES_SERVICE_HOST"]; hasKSH {
		t.Error("KUBERNETES_SERVICE_HOST should not be set (containerboot needs kube API access)")
	}

	tsAuthKey, ok := envMap["TS_AUTHKEY"]
	if !ok {
		t.Fatal("TS_AUTHKEY env var should be present on sidecar")
	}
	if tsAuthKey.ValueFrom == nil || tsAuthKey.ValueFrom.SecretKeyRef == nil {
		t.Fatal("TS_AUTHKEY should use SecretKeyRef")
	}
	if tsAuthKey.ValueFrom.SecretKeyRef.Name != "ts-auth" {
		t.Errorf("expected secret name ts-auth, got %s", tsAuthKey.ValueFrom.SecretKeyRef.Name)
	}
	if tsAuthKey.ValueFrom.SecretKeyRef.Key != "authkey" {
		t.Errorf("expected secret key authkey, got %s", tsAuthKey.ValueFrom.SecretKeyRef.Key)
	}

	// Verify security context
	sc := tsSidecar.SecurityContext
	if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
		t.Error("tailscale sidecar should have readOnlyRootFilesystem=true")
	}
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Error("tailscale sidecar should have runAsNonRoot=true")
	}
}

func TestBuildStatefulSet_TailscaleAuthKeyOnSidecar_NotMain(t *testing.T) {
	instance := newTestInstance("ts-env-split")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.AuthKeySecretRef = &corev1.LocalObjectReference{Name: "ts-auth"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	// TS_AUTHKEY and TS_HOSTNAME should NOT be on the main container
	for _, env := range main.Env {
		if env.Name == "TS_AUTHKEY" {
			t.Error("TS_AUTHKEY should NOT be on main container (moved to sidecar)")
		}
		if env.Name == "TS_HOSTNAME" {
			t.Error("TS_HOSTNAME should NOT be on main container (moved to sidecar)")
		}
	}

	// TS_SOCKET should be on main container
	var found bool
	for _, env := range main.Env {
		if env.Name == "TS_SOCKET" {
			found = true
			if env.Value != TailscaleSocketPath {
				t.Errorf("TS_SOCKET = %q, want %q", env.Value, TailscaleSocketPath)
			}
		}
	}
	if !found {
		t.Error("TS_SOCKET env var should be present on main container")
	}
}

func TestBuildStatefulSet_TailscaleCustomHostname(t *testing.T) {
	instance := newTestInstance("ts-custom")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.Hostname = "my-custom-host"

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Find sidecar
	var tsSidecar *corev1.Container
	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == "tailscale" {
			tsSidecar = &sts.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if tsSidecar == nil {
		t.Fatal("tailscale sidecar container should be present")
	}

	for _, env := range tsSidecar.Env {
		if env.Name == "TS_HOSTNAME" {
			if env.Value != "my-custom-host" {
				t.Errorf("expected TS_HOSTNAME=my-custom-host, got %s", env.Value)
			}
			return
		}
	}
	t.Fatal("TS_HOSTNAME env var should be present on sidecar")
}

func TestBuildStatefulSet_TailscaleCustomSecretKey(t *testing.T) {
	instance := newTestInstance("ts-customkey")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.AuthKeySecretRef = &corev1.LocalObjectReference{Name: "ts-secret"}
	instance.Spec.Tailscale.AuthKeySecretKey = "my-key"

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Find sidecar
	var tsSidecar *corev1.Container
	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == "tailscale" {
			tsSidecar = &sts.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if tsSidecar == nil {
		t.Fatal("tailscale sidecar container should be present")
	}

	for _, env := range tsSidecar.Env {
		if env.Name == "TS_AUTHKEY" {
			if env.ValueFrom.SecretKeyRef.Key != "my-key" {
				t.Errorf("expected custom secret key my-key, got %s", env.ValueFrom.SecretKeyRef.Key)
			}
			return
		}
	}
	t.Fatal("TS_AUTHKEY env var should be present on sidecar")
}

func TestBuildStatefulSet_TailscaleDisabled(t *testing.T) {
	instance := newTestInstance("ts-disabled")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	for _, env := range main.Env {
		if env.Name == "TS_AUTHKEY" || env.Name == "TS_HOSTNAME" || env.Name == "TS_SOCKET" {
			t.Errorf("unexpected Tailscale env var %s when Tailscale is disabled", env.Name)
		}
	}

	// No tailscale sidecar should exist
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "tailscale" {
			t.Error("tailscale sidecar should not be present when Tailscale is disabled")
		}
	}

	// No tailscale init container
	for _, c := range sts.Spec.Template.Spec.InitContainers {
		if c.Name == "init-tailscale-bin" {
			t.Error("init-tailscale-bin should not be present when Tailscale is disabled")
		}
	}
}

func TestBuildStatefulSet_TailscaleInitContainer(t *testing.T) {
	instance := newTestInstance("ts-init")
	instance.Spec.Tailscale.Enabled = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var initContainer *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "init-tailscale-bin" {
			initContainer = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if initContainer == nil {
		t.Fatal("init-tailscale-bin init container should be present")
	}
	if initContainer.Image != DefaultTailscaleImage+":"+DefaultImageTag {
		t.Errorf("init container image = %q, want %q", initContainer.Image, DefaultTailscaleImage+":"+DefaultImageTag)
	}
	assertVolumeMount(t, initContainer.VolumeMounts, "tailscale-bin", TailscaleBinPath)
}

func TestBuildStatefulSet_TailscaleVolumes(t *testing.T) {
	instance := newTestInstance("ts-vols")
	instance.Spec.Tailscale.Enabled = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	volumes := sts.Spec.Template.Spec.Volumes

	for _, name := range []string{"tailscale-socket", "tailscale-bin", "tailscale-tmp"} {
		v := findVolume(volumes, name)
		if v == nil {
			t.Errorf("volume %q not found", name)
			continue
		}
		if v.EmptyDir == nil {
			t.Errorf("volume %q should be emptyDir", name)
		}
	}
}

func TestBuildStatefulSet_TailscaleMainContainerMounts(t *testing.T) {
	instance := newTestInstance("ts-mounts")
	instance.Spec.Tailscale.Enabled = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	assertVolumeMount(t, main.VolumeMounts, "tailscale-socket", TailscaleSocketDir)
	assertVolumeMount(t, main.VolumeMounts, "tailscale-bin", TailscaleBinPath)

	// Verify read-only
	for _, m := range main.VolumeMounts {
		if m.Name == "tailscale-socket" && !m.ReadOnly {
			t.Error("tailscale-socket mount should be read-only on main container")
		}
		if m.Name == "tailscale-bin" && !m.ReadOnly {
			t.Error("tailscale-bin mount should be read-only on main container")
		}
	}
}

func TestBuildStatefulSet_TailscalePATH(t *testing.T) {
	instance := newTestInstance("ts-path")
	instance.Spec.Tailscale.Enabled = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	var pathVar *corev1.EnvVar
	for i := range main.Env {
		if main.Env[i].Name == "PATH" {
			pathVar = &main.Env[i]
			break
		}
	}
	if pathVar == nil {
		t.Fatal("PATH env var should be set when Tailscale is enabled")
	}
	if !strings.Contains(pathVar.Value, TailscaleBinPath) {
		t.Errorf("PATH should contain %q, got %q", TailscaleBinPath, pathVar.Value)
	}
}

func TestBuildStatefulSet_TailscalePATH_WithRuntimeDeps(t *testing.T) {
	instance := newTestInstance("ts-path-rd")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.RuntimeDeps.Pnpm = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	var pathVar *corev1.EnvVar
	for i := range main.Env {
		if main.Env[i].Name == "PATH" {
			pathVar = &main.Env[i]
			break
		}
	}
	if pathVar == nil {
		t.Fatal("PATH env var should be set")
	}
	// Both tailscale-bin and runtime deps should be in PATH
	if !strings.Contains(pathVar.Value, TailscaleBinPath) {
		t.Errorf("PATH should contain %q, got %q", TailscaleBinPath, pathVar.Value)
	}
	if !strings.Contains(pathVar.Value, RuntimeDepsLocalBin) {
		t.Errorf("PATH should contain %q, got %q", RuntimeDepsLocalBin, pathVar.Value)
	}
}

func TestBuildStatefulSet_TailscaleCustomImage(t *testing.T) {
	instance := newTestInstance("ts-custom-img")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.Image.Repository = "my-registry/tailscale"
	instance.Spec.Tailscale.Image.Tag = "v1.60.0"

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Check sidecar
	var tsSidecar *corev1.Container
	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == "tailscale" {
			tsSidecar = &sts.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if tsSidecar == nil {
		t.Fatal("tailscale sidecar should be present")
	}
	if tsSidecar.Image != "my-registry/tailscale:v1.60.0" {
		t.Errorf("sidecar image = %q, want %q", tsSidecar.Image, "my-registry/tailscale:v1.60.0")
	}

	// Check init container uses same image
	for _, c := range sts.Spec.Template.Spec.InitContainers {
		if c.Name == "init-tailscale-bin" {
			if c.Image != "my-registry/tailscale:v1.60.0" {
				t.Errorf("init container image = %q, want %q", c.Image, "my-registry/tailscale:v1.60.0")
			}
			return
		}
	}
	t.Fatal("init-tailscale-bin should be present")
}

func TestBuildStatefulSet_TailscaleImageDigest(t *testing.T) {
	instance := newTestInstance("ts-digest")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.Image.Digest = "sha256:abc123"

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var tsSidecar *corev1.Container
	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == "tailscale" {
			tsSidecar = &sts.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if tsSidecar == nil {
		t.Fatal("tailscale sidecar should be present")
	}
	expected := DefaultTailscaleImage + "@sha256:abc123"
	if tsSidecar.Image != expected {
		t.Errorf("sidecar image = %q, want %q", tsSidecar.Image, expected)
	}
}

func TestBuildNetworkPolicy_TailscaleEgress(t *testing.T) {
	instance := newTestInstance("ts-np")
	instance.Spec.Tailscale.Enabled = true

	np := BuildNetworkPolicy(instance)

	// Default egress: DNS (0), HTTPS (1), K8s API 6443 (2), Tailscale STUN+WireGuard (3)
	if len(np.Spec.Egress) < 4 {
		t.Fatalf("expected at least 4 egress rules, got %d", len(np.Spec.Egress))
	}

	// Verify K8s API port 6443 is included (tailscale needs it for state secret)
	found6443 := false
	foundSTUN := false
	foundWG := false
	for _, rule := range np.Spec.Egress {
		for _, p := range rule.Ports {
			if p.Port == nil {
				continue
			}
			switch p.Port.IntValue() {
			case 6443:
				found6443 = true
			case 3478:
				if p.Protocol != nil && *p.Protocol == corev1.ProtocolUDP {
					foundSTUN = true
				}
			case 41641:
				if p.Protocol != nil && *p.Protocol == corev1.ProtocolUDP {
					foundWG = true
				}
			}
		}
	}
	if !found6443 {
		t.Error("expected K8s API egress rule (TCP 6443) for tailscale state secret")
	}
	if !foundSTUN {
		t.Error("expected STUN egress rule (UDP 3478)")
	}
	if !foundWG {
		t.Error("expected WireGuard egress rule (UDP 41641)")
	}
}

func TestBuildNetworkPolicy_TailscaleDisabled(t *testing.T) {
	instance := newTestInstance("ts-np-off")

	np := BuildNetworkPolicy(instance)

	// Default egress: DNS (0), HTTPS (1) - no Tailscale rules
	if len(np.Spec.Egress) != 2 {
		t.Errorf("expected 2 egress rules when Tailscale is disabled, got %d", len(np.Spec.Egress))
	}

	for _, rule := range np.Spec.Egress {
		for _, p := range rule.Ports {
			if p.Protocol != nil && *p.Protocol == corev1.ProtocolUDP && p.Port != nil {
				if p.Port.IntValue() == 3478 || p.Port.IntValue() == 41641 {
					t.Errorf("unexpected Tailscale egress port %d when disabled", p.Port.IntValue())
				}
			}
		}
	}
}

func TestBuildStatefulSet_Idempotent_WithTailscale(t *testing.T) {
	instance := newTestInstance("idempotent-ts")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{"key":"val"}`)},
	}
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.Mode = "serve"
	instance.Spec.Tailscale.AuthKeySecretRef = &corev1.LocalObjectReference{Name: "ts-auth"}
	instance.Spec.Tailscale.Hostname = "my-ts-host"

	dep1 := BuildStatefulSet(instance, "", nil, nil, nil)
	dep2 := BuildStatefulSet(instance, "", nil, nil, nil)

	b1, _ := json.Marshal(dep1.Spec)
	b2, _ := json.Marshal(dep2.Spec)

	if !bytes.Equal(b1, b2) {
		t.Error("BuildStatefulSet with Tailscale is not idempotent")
	}
}

func TestConfigHash_ChangesWithTailscale(t *testing.T) {
	instance := newTestInstance("hash-ts")

	sts1 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash1 := sts1.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.Mode = "serve"

	sts2 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash2 := sts2.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	if hash1 == hash2 {
		t.Error("config hash should change when Tailscale is enabled")
	}
}

func TestBuildConfigMap_TailscaleDefaultMode_ServeConfig(t *testing.T) {
	instance := newTestInstance("ts-default-mode")
	instance.Spec.Tailscale.Enabled = true
	// Mode is empty - should default to "serve" in serve config

	cm := BuildConfigMap(instance, "", nil)
	serveJSON := cm.Data[TailscaleServeConfigKey]

	var cfg map[string]interface{}
	if err := json.Unmarshal([]byte(serveJSON), &cfg); err != nil {
		t.Fatalf("failed to parse serve config JSON: %v", err)
	}

	// Default mode is serve - should NOT have AllowFunnel
	if _, hasFunnel := cfg["AllowFunnel"]; hasFunnel {
		t.Error("AllowFunnel should not be set when mode defaults to serve")
	}
}

func TestBuildConfigMap_TailscaleNoAuthSSO(t *testing.T) {
	instance := newTestInstance("ts-no-sso")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.AuthSSO = false

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	// gateway.bind=loopback should still be set (always), but no auth.allowTailscale
	if gw, ok := parsed["gateway"].(map[string]interface{}); ok {
		if auth, ok := gw["auth"].(map[string]interface{}); ok {
			if _, hasAllowTS := auth["allowTailscale"]; hasAllowTS {
				t.Error("auth.allowTailscale should not be set when AuthSSO is disabled")
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Tailscale loopback bind + exec probe tests
// ---------------------------------------------------------------------------

func TestBuildConfigMap_TailscaleServe_SetsLoopbackBind(t *testing.T) {
	instance := newTestInstance("ts-loopback-serve")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.Mode = "serve"

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	gw := parsed["gateway"].(map[string]interface{})
	if gw["bind"] != "loopback" {
		t.Errorf("gateway.bind = %v, want %q when Tailscale serve is enabled", gw["bind"], "loopback")
	}
}

func TestBuildConfigMap_TailscaleFunnel_SetsLoopbackBind(t *testing.T) {
	instance := newTestInstance("ts-loopback-funnel")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.Mode = "funnel"

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	gw := parsed["gateway"].(map[string]interface{})
	if gw["bind"] != "loopback" {
		t.Errorf("gateway.bind = %v, want %q when Tailscale funnel is enabled", gw["bind"], "loopback")
	}
}

func TestBuildConfigMap_TailscaleDefaultMode_SetsLoopbackBind(t *testing.T) {
	instance := newTestInstance("ts-loopback-default")
	instance.Spec.Tailscale.Enabled = true
	// Mode left empty -- defaults to "serve"

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	gw := parsed["gateway"].(map[string]interface{})
	if gw["bind"] != "loopback" {
		t.Errorf("gateway.bind = %v, want %q when Tailscale is enabled with default mode", gw["bind"], "loopback")
	}
}

func TestBuildConfigMap_AlwaysSetsLoopbackBind(t *testing.T) {
	instance := newTestInstance("always-loopback")
	// Tailscale not enabled - should still set loopback (proxy handles external access)

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	gw := parsed["gateway"].(map[string]interface{})
	if gw["bind"] != "loopback" {
		t.Errorf("gateway.bind = %v, want %q (always loopback with proxy sidecar)", gw["bind"], "loopback")
	}
}

func TestBuildConfigMap_TailscaleLoopback_UserOverridePreserved(t *testing.T) {
	instance := newTestInstance("ts-user-override")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.Mode = "serve"
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"gateway":{"bind":"0.0.0.0"}}`),
		},
	}

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	gw := parsed["gateway"].(map[string]interface{})
	if gw["bind"] != "0.0.0.0" {
		t.Errorf("gateway.bind = %v, want %q (user override should be preserved)", gw["bind"], "0.0.0.0")
	}
}

func TestBuildStatefulSet_TailscaleServe_UsesHTTPProbes(t *testing.T) {
	instance := newTestInstance("ts-http-probes")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.Mode = "serve"

	sts := BuildStatefulSet(instance, "hash", nil, nil, nil)
	container := sts.Spec.Template.Spec.Containers[0]

	if container.LivenessProbe == nil {
		t.Fatal("liveness probe should not be nil")
	}
	if container.LivenessProbe.HTTPGet == nil {
		t.Fatal("liveness probe should use HTTPGet handler when Tailscale serve is enabled")
	}
	if container.LivenessProbe.Exec != nil {
		t.Error("liveness probe should not use Exec when Tailscale serve is enabled")
	}

	if container.ReadinessProbe == nil {
		t.Fatal("readiness probe should not be nil")
	}
	if container.ReadinessProbe.HTTPGet == nil {
		t.Fatal("readiness probe should use HTTPGet handler when Tailscale serve is enabled")
	}

	if container.StartupProbe == nil {
		t.Fatal("startup probe should not be nil")
	}
	if container.StartupProbe.HTTPGet == nil {
		t.Fatal("startup probe should use HTTPGet handler when Tailscale serve is enabled")
	}

	// Verify HTTP probe targets proxy port
	if container.LivenessProbe.HTTPGet.Port.IntValue() != int(GatewayProxyPort) {
		t.Errorf("liveness probe port = %d, want %d", container.LivenessProbe.HTTPGet.Port.IntValue(), GatewayProxyPort)
	}
}

func TestBuildStatefulSet_AlwaysUsesHTTPProbes(t *testing.T) {
	instance := newTestInstance("always-http-probes")
	// Tailscale not enabled - probes should still use HTTPGet via proxy

	sts := BuildStatefulSet(instance, "hash", nil, nil, nil)
	container := sts.Spec.Template.Spec.Containers[0]

	if container.LivenessProbe == nil {
		t.Fatal("liveness probe should not be nil")
	}
	if container.LivenessProbe.HTTPGet == nil {
		t.Fatal("liveness probe should always use HTTPGet handler")
	}
	if container.LivenessProbe.Exec != nil {
		t.Error("liveness probe should not use Exec")
	}

	if container.ReadinessProbe.HTTPGet == nil {
		t.Fatal("readiness probe should always use HTTPGet handler")
	}
	if container.StartupProbe.HTTPGet == nil {
		t.Fatal("startup probe should always use HTTPGet handler")
	}
}

func TestBuildStatefulSet_TailscaleFunnel_UsesHTTPProbes(t *testing.T) {
	instance := newTestInstance("ts-funnel-probes")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.Mode = "funnel"

	sts := BuildStatefulSet(instance, "hash", nil, nil, nil)
	container := sts.Spec.Template.Spec.Containers[0]

	if container.LivenessProbe.HTTPGet == nil {
		t.Fatal("liveness probe should use HTTPGet handler when Tailscale funnel is enabled")
	}
	if container.ReadinessProbe.HTTPGet == nil {
		t.Fatal("readiness probe should use HTTPGet handler when Tailscale funnel is enabled")
	}
	if container.StartupProbe.HTTPGet == nil {
		t.Fatal("startup probe should use HTTPGet handler when Tailscale funnel is enabled")
	}
}

func TestBuildStatefulSet_TailscaleStateSecretName(t *testing.T) {
	instance := newTestInstance("ts-state")
	got := TailscaleStateSecretName(instance)
	want := "ts-state-ts-state"
	if got != want {
		t.Errorf("TailscaleStateSecretName = %q, want %q", got, want)
	}
}

func TestBuildTailscaleStateSecret(t *testing.T) {
	instance := newTestInstance("ts-secret")
	secret := BuildTailscaleStateSecret(instance)

	if secret.Name != TailscaleStateSecretName(instance) {
		t.Errorf("secret name = %q, want %q", secret.Name, TailscaleStateSecretName(instance))
	}
	if secret.Namespace != instance.Namespace {
		t.Errorf("secret namespace = %q, want %q", secret.Namespace, instance.Namespace)
	}
	if secret.Data != nil {
		t.Error("state secret should have nil Data (containerboot manages content)")
	}
}

func TestBuildStatefulSet_TailscaleAutoMountToken(t *testing.T) {
	instance := newTestInstance("ts-automount")
	instance.Spec.Tailscale.Enabled = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	token := sts.Spec.Template.Spec.AutomountServiceAccountToken
	if token == nil || !*token {
		t.Error("AutomountServiceAccountToken should be true when Tailscale is enabled")
	}
}

func TestBuildServiceAccount_TailscaleAutoMountToken(t *testing.T) {
	instance := newTestInstance("ts-sa-token")
	instance.Spec.Tailscale.Enabled = true

	sa := BuildServiceAccount(instance)
	if sa.AutomountServiceAccountToken == nil || !*sa.AutomountServiceAccountToken {
		t.Error("AutomountServiceAccountToken should be true when Tailscale is enabled")
	}
}

func TestBuildRole_TailscaleStateSecretRule(t *testing.T) {
	instance := newTestInstance("ts-role")
	instance.Spec.Tailscale.Enabled = true

	role := BuildRole(instance)

	var found bool
	for _, rule := range role.Rules {
		for _, res := range rule.Resources {
			if res == "secrets" {
				for _, name := range rule.ResourceNames {
					if name == TailscaleStateSecretName(instance) {
						found = true
						// Verify verbs
						verbSet := make(map[string]bool)
						for _, v := range rule.Verbs {
							verbSet[v] = true
						}
						if !verbSet["get"] || !verbSet["update"] || !verbSet["patch"] {
							t.Errorf("expected get/update/patch verbs, got %v", rule.Verbs)
						}
					}
				}
			}
		}
	}
	if !found {
		t.Error("Role should include a rule for the Tailscale state Secret")
	}
}

func TestBuildRole_NoTailscaleRule_WhenDisabled(t *testing.T) {
	instance := newTestInstance("no-ts-role")

	role := BuildRole(instance)

	for _, rule := range role.Rules {
		for _, res := range rule.Resources {
			if res == "secrets" {
				for _, name := range rule.ResourceNames {
					if name == TailscaleStateSecretName(instance) {
						t.Error("Role should not include Tailscale state Secret rule when Tailscale is disabled")
					}
				}
			}
		}
	}
}

func TestBuildStatefulSet_ProbeEndpointPaths(t *testing.T) {
	instance := newTestInstance("probe-paths")

	sts := BuildStatefulSet(instance, "hash", nil, nil, nil)
	container := sts.Spec.Template.Spec.Containers[0]

	tests := []struct {
		name     string
		probe    *corev1.Probe
		wantPath string
		wantPort int
	}{
		{"liveness", container.LivenessProbe, "/healthz", int(GatewayProxyPort)},
		{"readiness", container.ReadinessProbe, "/readyz", int(GatewayProxyPort)},
		{"startup", container.StartupProbe, "/healthz", int(GatewayProxyPort)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.probe == nil {
				t.Fatalf("%s probe should not be nil", tt.name)
			}
			if tt.probe.HTTPGet == nil {
				t.Fatalf("%s probe should use HTTPGet handler", tt.name)
			}
			if tt.probe.HTTPGet.Path != tt.wantPath {
				t.Errorf("%s probe path = %q, want %q", tt.name, tt.probe.HTTPGet.Path, tt.wantPath)
			}
			if tt.probe.HTTPGet.Port.IntValue() != tt.wantPort {
				t.Errorf("%s probe port = %d, want %d", tt.name, tt.probe.HTTPGet.Port.IntValue(), tt.wantPort)
			}
			if tt.probe.HTTPGet.Scheme != corev1.URISchemeHTTP {
				t.Errorf("%s probe scheme = %q, want %q", tt.name, tt.probe.HTTPGet.Scheme, corev1.URISchemeHTTP)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Browser config enrichment tests (Chromium sidecar)
// ---------------------------------------------------------------------------

func TestBuildConfigMap_ChromiumBrowserConfig(t *testing.T) {
	instance := newTestInstance("cr-browser")
	instance.Spec.Chromium.Enabled = true

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	browser, ok := parsed["browser"].(map[string]interface{})
	if !ok {
		t.Fatal("expected browser key in config when chromium is enabled")
	}

	if browser["defaultProfile"] != "default" {
		t.Errorf("browser.defaultProfile = %v, want %q", browser["defaultProfile"], "default")
	}

	// attachOnly must be true so OpenClaw skips local browser binary detection
	// and goes straight to the remote CDP connection.
	attachOnly, ok := browser["attachOnly"].(bool)
	if !ok || !attachOnly {
		t.Errorf("browser.attachOnly = %v, want true", browser["attachOnly"])
	}

	// remoteCdpTimeoutMs gives the browser service time to become ready,
	// preventing permanent failure when tool registration fires first.
	timeout, ok := browser["remoteCdpTimeoutMs"].(float64)
	if !ok || timeout != 30000 {
		t.Errorf("browser.remoteCdpTimeoutMs = %v, want 30000", browser["remoteCdpTimeoutMs"])
	}

	profiles, ok := browser["profiles"].(map[string]interface{})
	if !ok {
		t.Fatal("expected browser.profiles key")
	}

	// Both "default" and "chrome" profiles must use the env var reference
	// which resolves to the Chromium sidecar's localhost address at runtime.
	expectedCDP := "${OPENCLAW_CHROMIUM_CDP}"
	for _, name := range []string{"default", "chrome"} {
		p, ok := profiles[name].(map[string]interface{})
		if !ok {
			t.Fatalf("expected browser.profiles.%s key", name)
		}
		if p["cdpUrl"] != expectedCDP {
			t.Errorf("browser.profiles.%s.cdpUrl = %v, want %q", name, p["cdpUrl"], expectedCDP)
		}
		if p["color"] != "#4285F4" {
			t.Errorf("browser.profiles.%s.color = %v, want %q", name, p["color"], "#4285F4")
		}
	}
}

func TestBuildConfigMap_ChromiumUserOverrideAttachOnly(t *testing.T) {
	instance := newTestInstance("cr-override-attachonly")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"browser":{"attachOnly":true}}`),
		},
	}

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	browser := parsed["browser"].(map[string]interface{})
	attachOnly := browser["attachOnly"].(bool)
	if attachOnly != true {
		t.Errorf("user-set attachOnly should be preserved, got %v", attachOnly)
	}
}

func TestBuildConfigMap_ChromiumDisabled_NoBrowserConfig(t *testing.T) {
	instance := newTestInstance("cr-disabled")
	// Chromium not enabled (default)

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	if _, ok := parsed["browser"]; ok {
		t.Error("browser config should not be present when chromium is disabled")
	}
}

func TestBuildConfigMap_ChromiumUserOverrideDefaultProfile(t *testing.T) {
	instance := newTestInstance("cr-override-profile")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"browser":{"defaultProfile":"chrome"}}`),
		},
	}

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	browser := parsed["browser"].(map[string]interface{})
	if browser["defaultProfile"] != "chrome" {
		t.Errorf("user-set defaultProfile should be preserved, got %v", browser["defaultProfile"])
	}
}

func TestBuildConfigMap_ChromiumUserOverrideCdpUrl(t *testing.T) {
	instance := newTestInstance("cr-override-cdp")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"browser":{"profiles":{"default":{"cdpUrl":"ws://custom:1234"}}}}`),
		},
	}

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	browser := parsed["browser"].(map[string]interface{})
	profiles := browser["profiles"].(map[string]interface{})
	defaultProfile := profiles["default"].(map[string]interface{})

	if defaultProfile["cdpUrl"] != "ws://custom:1234" {
		t.Errorf("user-set cdpUrl should be preserved, got %v", defaultProfile["cdpUrl"])
	}
}

func TestBuildConfigMap_ChromiumUserOverrideCdpPort(t *testing.T) {
	instance := newTestInstance("cr-override-port")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"browser":{"profiles":{"default":{"cdpPort":18800}}}}`),
		},
	}

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	browser := parsed["browser"].(map[string]interface{})
	profiles := browser["profiles"].(map[string]interface{})
	defaultProfile := profiles["default"].(map[string]interface{})

	// cdpUrl should NOT be injected when user set cdpPort
	if _, hasCdpURL := defaultProfile["cdpUrl"]; hasCdpURL {
		t.Error("cdpUrl should not be injected when user set cdpPort")
	}
	// cdpPort should be preserved
	if defaultProfile["cdpPort"] != float64(18800) {
		t.Errorf("user-set cdpPort should be preserved, got %v", defaultProfile["cdpPort"])
	}
}

func TestBuildConfigMap_ChromiumUserOverrideRemoteCdpTimeout(t *testing.T) {
	instance := newTestInstance("cr-override-timeout")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{
			Raw: []byte(`{"browser":{"remoteCdpTimeoutMs":60000}}`),
		},
	}

	cm := BuildConfigMap(instance, "", nil)
	content := cm.Data["openclaw.json"]

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	browser := parsed["browser"].(map[string]interface{})
	timeout := browser["remoteCdpTimeoutMs"].(float64)
	if timeout != 60000 {
		t.Errorf("user-set remoteCdpTimeoutMs should be preserved, got %v", timeout)
	}
}

// ---------------------------------------------------------------------------
// Ollama sidecar tests
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_OllamaEnabled(t *testing.T) {
	instance := newTestInstance("ollama-test")
	instance.Spec.Ollama.Enabled = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	containers := sts.Spec.Template.Spec.Containers

	if len(containers) != 4 {
		t.Fatalf("expected 4 containers (main + gateway-proxy + ollama + otel-collector), got %d", len(containers))
	}

	var ollama *corev1.Container
	for i := range containers {
		if containers[i].Name == "ollama" {
			ollama = &containers[i]
			break
		}
	}
	if ollama == nil {
		t.Fatal("ollama container not found")
	}

	// Main container should have OLLAMA_HOST env var
	mainContainer := containers[0]
	foundOllamaHost := false
	for _, env := range mainContainer.Env {
		if env.Name == "OLLAMA_HOST" {
			foundOllamaHost = true
			if env.Value != "http://localhost:11434" {
				t.Errorf("OLLAMA_HOST = %q, want %q", env.Value, "http://localhost:11434")
			}
			break
		}
	}
	if !foundOllamaHost {
		t.Error("main container should have OLLAMA_HOST env var when ollama is enabled")
	}

	// Ollama image defaults
	if ollama.Image != "ollama/ollama:latest" {
		t.Errorf("ollama image = %q, want default", ollama.Image)
	}

	// Ollama port
	if len(ollama.Ports) != 1 {
		t.Fatalf("ollama container should have 1 port, got %d", len(ollama.Ports))
	}
	if ollama.Ports[0].ContainerPort != OllamaPort {
		t.Errorf("ollama port = %d, want %d", ollama.Ports[0].ContainerPort, OllamaPort)
	}
	if ollama.Ports[0].Name != "ollama" {
		t.Errorf("ollama port name = %q, want %q", ollama.Ports[0].Name, "ollama")
	}

	// Ollama security context — runs as root
	osc := ollama.SecurityContext
	if osc == nil {
		t.Fatal("ollama security context is nil")
	}
	if osc.ReadOnlyRootFilesystem == nil || *osc.ReadOnlyRootFilesystem {
		t.Error("ollama: readOnlyRootFilesystem should be false")
	}
	if osc.RunAsUser == nil || *osc.RunAsUser != 0 {
		t.Errorf("ollama: runAsUser = %v, want 0 (root)", osc.RunAsUser)
	}
	if osc.RunAsNonRoot == nil || *osc.RunAsNonRoot {
		t.Error("ollama: runAsNonRoot should be false")
	}

	// Ollama resource defaults
	cpuReq := ollama.Resources.Requests[corev1.ResourceCPU]
	if cpuReq.String() != "500m" {
		t.Errorf("ollama cpu request = %v, want 500m", cpuReq.String())
	}
	memReq := ollama.Resources.Requests[corev1.ResourceMemory]
	if memReq.Cmp(resource.MustParse("1Gi")) != 0 {
		t.Errorf("ollama memory request = %v, want 1Gi", memReq.String())
	}

	// Ollama volume mount
	assertVolumeMount(t, ollama.VolumeMounts, "ollama-models", "/root/.ollama")

	// Volumes — check ollama-models volume exists with emptyDir
	volumes := sts.Spec.Template.Spec.Volumes
	var ollamaVol *corev1.Volume
	for i := range volumes {
		if volumes[i].Name == "ollama-models" {
			ollamaVol = &volumes[i]
			break
		}
	}
	if ollamaVol == nil {
		t.Fatal("ollama-models volume not found")
	}
	if ollamaVol.EmptyDir == nil {
		t.Fatal("ollama-models volume should be emptyDir by default")
	}
	if ollamaVol.EmptyDir.SizeLimit == nil {
		t.Fatal("ollama-models emptyDir should have a sizeLimit")
	}
	if ollamaVol.EmptyDir.SizeLimit.Cmp(resource.MustParse("20Gi")) != 0 {
		t.Errorf("ollama-models sizeLimit = %v, want 20Gi", ollamaVol.EmptyDir.SizeLimit.String())
	}

	// No init container when no models specified
	for _, ic := range sts.Spec.Template.Spec.InitContainers {
		if ic.Name == "init-ollama" {
			t.Error("init-ollama should not be present when no models are specified")
		}
	}
}

func TestBuildStatefulSet_OllamaEnabled_WithModels(t *testing.T) {
	instance := newTestInstance("ollama-models")
	instance.Spec.Ollama.Enabled = true
	instance.Spec.Ollama.Models = []string{"llama3.2", "nomic-embed-text"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	initContainers := sts.Spec.Template.Spec.InitContainers

	var initOllama *corev1.Container
	for i := range initContainers {
		if initContainers[i].Name == "init-ollama" {
			initOllama = &initContainers[i]
			break
		}
	}
	if initOllama == nil {
		t.Fatal("init-ollama container not found")
	}

	// Verify command pulls both models
	cmd := initOllama.Command[2] // ["sh", "-c", "..."]
	if !strings.Contains(cmd, "ollama pull 'llama3.2'") {
		t.Errorf("init-ollama command should pull llama3.2, got: %s", cmd)
	}
	if !strings.Contains(cmd, "ollama pull 'nomic-embed-text'") {
		t.Errorf("init-ollama command should pull nomic-embed-text, got: %s", cmd)
	}
	if !strings.Contains(cmd, "ollama serve") {
		t.Errorf("init-ollama command should start ollama serve, got: %s", cmd)
	}
	if !strings.Contains(cmd, "kill %1 2>/dev/null; exit 0") {
		t.Errorf("init-ollama command should kill server, got: %s", cmd)
	}

	// Verify init container mounts ollama-models
	assertVolumeMount(t, initOllama.VolumeMounts, "ollama-models", "/root/.ollama")

	// Verify init container runs as root
	if initOllama.SecurityContext.RunAsUser == nil || *initOllama.SecurityContext.RunAsUser != 0 {
		t.Errorf("init-ollama: runAsUser = %v, want 0", initOllama.SecurityContext.RunAsUser)
	}
}

func TestBuildStatefulSet_OllamaEnabled_NoModels(t *testing.T) {
	instance := newTestInstance("ollama-no-models")
	instance.Spec.Ollama.Enabled = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Sidecar should be present
	found := false
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "ollama" {
			found = true
			break
		}
	}
	if !found {
		t.Error("ollama sidecar should be present")
	}

	// No init container
	for _, ic := range sts.Spec.Template.Spec.InitContainers {
		if ic.Name == "init-ollama" {
			t.Error("init-ollama should not be present when no models are specified")
		}
	}
}

func TestBuildStatefulSet_OllamaEnabled_GPU(t *testing.T) {
	instance := newTestInstance("ollama-gpu")
	instance.Spec.Ollama.Enabled = true
	instance.Spec.Ollama.GPU = Ptr(int32(2))

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var ollama *corev1.Container
	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == "ollama" {
			ollama = &sts.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if ollama == nil {
		t.Fatal("ollama container not found")
	}

	gpuRes := corev1.ResourceName("nvidia.com/gpu")

	// Check requests
	gpuReq, ok := ollama.Resources.Requests[gpuRes]
	if !ok {
		t.Fatal("nvidia.com/gpu request not found")
	}
	if gpuReq.String() != "2" {
		t.Errorf("gpu request = %v, want 2", gpuReq.String())
	}

	// Check limits
	gpuLim, ok := ollama.Resources.Limits[gpuRes]
	if !ok {
		t.Fatal("nvidia.com/gpu limit not found")
	}
	if gpuLim.String() != "2" {
		t.Errorf("gpu limit = %v, want 2", gpuLim.String())
	}
}

func TestBuildStatefulSet_OllamaEnabled_ExistingClaim(t *testing.T) {
	instance := newTestInstance("ollama-pvc")
	instance.Spec.Ollama.Enabled = true
	instance.Spec.Ollama.Storage.ExistingClaim = "my-model-pvc"

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	volumes := sts.Spec.Template.Spec.Volumes

	var ollamaVol *corev1.Volume
	for i := range volumes {
		if volumes[i].Name == "ollama-models" {
			ollamaVol = &volumes[i]
			break
		}
	}
	if ollamaVol == nil {
		t.Fatal("ollama-models volume not found")
	}
	if ollamaVol.PersistentVolumeClaim == nil {
		t.Fatal("ollama-models should use PVC when existingClaim is set")
	}
	if ollamaVol.PersistentVolumeClaim.ClaimName != "my-model-pvc" {
		t.Errorf("PVC claim name = %q, want %q", ollamaVol.PersistentVolumeClaim.ClaimName, "my-model-pvc")
	}
}

func TestBuildStatefulSet_OllamaEnabled_CustomImage(t *testing.T) {
	instance := newTestInstance("ollama-custom-img")
	instance.Spec.Ollama.Enabled = true
	instance.Spec.Ollama.Image = openclawv1alpha1.OllamaImageSpec{
		Repository: "my-registry.io/ollama",
		Tag:        "v0.3.0",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var ollama *corev1.Container
	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == "ollama" {
			ollama = &sts.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if ollama == nil {
		t.Fatal("ollama container not found")
	}
	if ollama.Image != "my-registry.io/ollama:v0.3.0" {
		t.Errorf("ollama image = %q, want %q", ollama.Image, "my-registry.io/ollama:v0.3.0")
	}
}

func TestBuildStatefulSet_OllamaEnabled_CustomImageDigest(t *testing.T) {
	instance := newTestInstance("ollama-digest")
	instance.Spec.Ollama.Enabled = true
	instance.Spec.Ollama.Image = openclawv1alpha1.OllamaImageSpec{
		Repository: "ollama/ollama",
		Tag:        "v0.3.0",
		Digest:     "sha256:ollamahash",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var ollama *corev1.Container
	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == "ollama" {
			ollama = &sts.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if ollama == nil {
		t.Fatal("ollama container not found")
	}
	if ollama.Image != "ollama/ollama@sha256:ollamahash" {
		t.Errorf("ollama image = %q, want %q", ollama.Image, "ollama/ollama@sha256:ollamahash")
	}
}

func TestBuildStatefulSet_OllamaEnabled_CustomResources(t *testing.T) {
	instance := newTestInstance("ollama-res")
	instance.Spec.Ollama.Enabled = true
	instance.Spec.Ollama.Resources = openclawv1alpha1.ResourcesSpec{
		Requests: openclawv1alpha1.ResourceList{
			CPU:    "1",
			Memory: "4Gi",
		},
		Limits: openclawv1alpha1.ResourceList{
			CPU:    "4",
			Memory: "16Gi",
		},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var ollama *corev1.Container
	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == "ollama" {
			ollama = &sts.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if ollama == nil {
		t.Fatal("ollama container not found")
	}

	cpuReq := ollama.Resources.Requests[corev1.ResourceCPU]
	if cpuReq.String() != "1" {
		t.Errorf("cpu request = %v, want 1", cpuReq.String())
	}
	memLim := ollama.Resources.Limits[corev1.ResourceMemory]
	if memLim.Cmp(resource.MustParse("16Gi")) != 0 {
		t.Errorf("memory limit = %v, want 16Gi", memLim.String())
	}
}

func TestBuildStatefulSet_OllamaAndChromiumEnabled(t *testing.T) {
	instance := newTestInstance("both-sidecars")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Ollama.Enabled = true
	instance.Spec.Ollama.Models = []string{"llama3.2"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	containers := sts.Spec.Template.Spec.Containers
	initContainers := sts.Spec.Template.Spec.InitContainers

	if len(containers) != 4 {
		t.Fatalf("expected 4 containers (main + gateway-proxy + ollama + otel-collector), got %d", len(containers))
	}

	names := make(map[string]bool)
	for _, c := range containers {
		names[c.Name] = true
	}
	if !names["openclaw"] {
		t.Error("main container not found")
	}
	if !names["ollama"] {
		t.Error("ollama container not found")
	}

	// Chromium should be in init containers (native sidecar)
	initNames := make(map[string]bool)
	for _, c := range initContainers {
		initNames[c.Name] = true
	}
	if !initNames["chromium"] {
		t.Error("chromium init container not found")
	}

	// Both env vars should be present on main container
	mainContainer := containers[0]
	foundChromiumCDP := false
	foundOllamaHost := false
	for _, env := range mainContainer.Env {
		if env.Name == "OPENCLAW_CHROMIUM_CDP" {
			foundChromiumCDP = true
		}
		if env.Name == "OLLAMA_HOST" {
			foundOllamaHost = true
		}
	}
	if !foundChromiumCDP {
		t.Error("main container should have OPENCLAW_CHROMIUM_CDP")
	}
	if !foundOllamaHost {
		t.Error("main container should have OLLAMA_HOST")
	}

	// Both volumes should exist
	volumes := sts.Spec.Template.Spec.Volumes
	volumeNames := make(map[string]bool)
	for _, v := range volumes {
		volumeNames[v.Name] = true
	}
	if !volumeNames["chromium-tmp"] {
		t.Error("chromium-tmp volume not found")
	}
	if !volumeNames["chromium-shm"] {
		t.Error("chromium-shm volume not found")
	}
	if !volumeNames["ollama-models"] {
		t.Error("ollama-models volume not found")
	}

	// Init container for model pull
	foundInitOllama := false
	for _, ic := range initContainers {
		if ic.Name == "init-ollama" {
			foundInitOllama = true
			break
		}
	}
	if !foundInitOllama {
		t.Error("init-ollama container not found when models are specified")
	}
}

func TestBuildStatefulSet_OllamaDisabled(t *testing.T) {
	instance := newTestInstance("no-ollama")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// No ollama container
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "ollama" {
			t.Error("ollama container should not be present when disabled")
		}
	}

	// No ollama-models volume
	for _, v := range sts.Spec.Template.Spec.Volumes {
		if v.Name == "ollama-models" {
			t.Error("ollama-models volume should not be present when disabled")
		}
	}

	// No OLLAMA_HOST env var
	mainContainer := sts.Spec.Template.Spec.Containers[0]
	for _, env := range mainContainer.Env {
		if env.Name == "OLLAMA_HOST" {
			t.Error("OLLAMA_HOST should not be set when ollama is disabled")
		}
	}
}

func TestBuildStatefulSet_OllamaEnabled_CustomStorageSize(t *testing.T) {
	instance := newTestInstance("ollama-storage")
	instance.Spec.Ollama.Enabled = true
	instance.Spec.Ollama.Storage.SizeLimit = "50Gi"

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var ollamaVol *corev1.Volume
	for i := range sts.Spec.Template.Spec.Volumes {
		if sts.Spec.Template.Spec.Volumes[i].Name == "ollama-models" {
			ollamaVol = &sts.Spec.Template.Spec.Volumes[i]
			break
		}
	}
	if ollamaVol == nil {
		t.Fatal("ollama-models volume not found")
	}
	if ollamaVol.EmptyDir == nil {
		t.Fatal("expected emptyDir")
	}
	if ollamaVol.EmptyDir.SizeLimit.Cmp(resource.MustParse("50Gi")) != 0 {
		t.Errorf("sizeLimit = %v, want 50Gi", ollamaVol.EmptyDir.SizeLimit.String())
	}
}

func TestBuildStatefulSet_OllamaContainerDefaults(t *testing.T) {
	instance := newTestInstance("ollama-defaults")
	instance.Spec.Ollama.Enabled = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	if len(sts.Spec.Template.Spec.Containers) < 2 {
		t.Fatal("expected ollama sidecar container")
	}

	var ollama corev1.Container
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "ollama" {
			ollama = c
			break
		}
	}

	// Verify Kubernetes default fields are set
	if ollama.TerminationMessagePath != corev1.TerminationMessagePathDefault {
		t.Errorf("TerminationMessagePath = %q, want default", ollama.TerminationMessagePath)
	}
	if ollama.TerminationMessagePolicy != corev1.TerminationMessageReadFile {
		t.Errorf("TerminationMessagePolicy = %v, want ReadFile", ollama.TerminationMessagePolicy)
	}
	if ollama.ImagePullPolicy != corev1.PullIfNotPresent {
		t.Errorf("ImagePullPolicy = %v, want IfNotPresent", ollama.ImagePullPolicy)
	}

	// Seccomp profile
	if ollama.SecurityContext.SeccompProfile == nil {
		t.Fatal("seccomp profile should be set")
	}
	if ollama.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("seccomp type = %v, want RuntimeDefault", ollama.SecurityContext.SeccompProfile.Type)
	}
}

func TestBuildStatefulSet_OllamaEnabled_InitContainerUsesCustomImage(t *testing.T) {
	instance := newTestInstance("ollama-init-img")
	instance.Spec.Ollama.Enabled = true
	instance.Spec.Ollama.Models = []string{"llama3.2"}
	instance.Spec.Ollama.Image = openclawv1alpha1.OllamaImageSpec{
		Repository: "my-registry.io/ollama",
		Tag:        "v0.3.0",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var initOllama *corev1.Container
	for i := range sts.Spec.Template.Spec.InitContainers {
		if sts.Spec.Template.Spec.InitContainers[i].Name == "init-ollama" {
			initOllama = &sts.Spec.Template.Spec.InitContainers[i]
			break
		}
	}
	if initOllama == nil {
		t.Fatal("init-ollama container not found")
	}
	if initOllama.Image != "my-registry.io/ollama:v0.3.0" {
		t.Errorf("init-ollama image = %q, want %q", initOllama.Image, "my-registry.io/ollama:v0.3.0")
	}
}

// ---------------------------------------------------------------------------
// PrometheusRule tests
// ---------------------------------------------------------------------------

func TestPrometheusRuleName(t *testing.T) {
	instance := newTestInstance("my-instance")
	name := PrometheusRuleName(instance)
	if name != "my-instance-alerts" {
		t.Errorf("PrometheusRuleName = %q, want %q", name, "my-instance-alerts")
	}
}

func TestBuildPrometheusRule(t *testing.T) {
	instance := newTestInstance("my-instance")
	instance.Spec.Observability.Metrics.PrometheusRule = &openclawv1alpha1.PrometheusRuleSpec{
		Enabled: Ptr(true),
	}

	pr := BuildPrometheusRule(instance)

	// Check GVK
	gvk := pr.GetObjectKind().GroupVersionKind()
	if gvk != PrometheusRuleGVK() {
		t.Errorf("GVK = %v, want %v", gvk, PrometheusRuleGVK())
	}

	// Check name
	if pr.GetName() != "my-instance-alerts" {
		t.Errorf("name = %q, want %q", pr.GetName(), "my-instance-alerts")
	}

	// Check namespace
	if pr.GetNamespace() != "test-ns" {
		t.Errorf("namespace = %q, want %q", pr.GetNamespace(), "test-ns")
	}

	// Check labels
	labels := pr.GetLabels()
	if labels["app.kubernetes.io/name"] != "openclaw" {
		t.Error("missing app.kubernetes.io/name label")
	}

	// Check spec.groups[0].rules has 7 alerts
	spec, ok := pr.Object["spec"].(map[string]interface{})
	if !ok {
		t.Fatal("missing spec")
	}
	groups, ok := spec["groups"].([]interface{})
	if !ok || len(groups) != 1 {
		t.Fatal("expected exactly 1 rule group")
	}
	group, ok := groups[0].(map[string]interface{})
	if !ok {
		t.Fatal("invalid group")
	}
	rules, ok := group["rules"].([]interface{})
	if !ok {
		t.Fatal("missing rules")
	}
	if len(rules) != 7 {
		t.Errorf("expected 7 alerts, got %d", len(rules))
	}

	// Check all alerts have runbook_url
	for i, r := range rules {
		rule, ok := r.(map[string]interface{})
		if !ok {
			t.Fatalf("rule %d is not a map", i)
		}
		annotations, ok := rule["annotations"].(map[string]interface{})
		if !ok {
			t.Fatalf("rule %d missing annotations", i)
		}
		runbook, ok := annotations["runbook_url"].(string)
		if !ok || runbook == "" {
			t.Errorf("rule %d missing runbook_url", i)
		}
		if !strings.HasPrefix(runbook, "https://paperclip.inc/docs/operators/openclaw/runbooks/") {
			t.Errorf("rule %d runbook_url = %q, expected default base URL", i, runbook)
		}
	}
}

// ---------------------------------------------------------------------------
// Self-Configure tests
// ---------------------------------------------------------------------------

func TestBuildRole_SelfConfigureEnabled(t *testing.T) {
	instance := newTestInstance("sc-role")
	instance.Spec.SelfConfigure = openclawv1alpha1.SelfConfigureSpec{
		Enabled:        true,
		AllowedActions: []openclawv1alpha1.SelfConfigAction{"skills", "config"},
	}

	role := BuildRole(instance)

	// 1 base rule (configmap) + 3 self-configure rules (instances, selfconfigs, secrets) + 0 additional
	if len(role.Rules) < 4 {
		t.Fatalf("expected at least 4 rules, got %d", len(role.Rules))
	}

	// Check for openclawinstances get rule
	foundInstances := false
	for _, rule := range role.Rules {
		for _, res := range rule.Resources {
			if res == "openclawinstances" {
				foundInstances = true
				if len(rule.ResourceNames) != 1 || rule.ResourceNames[0] != "sc-role" {
					t.Errorf("instances rule resourceNames = %v, want [sc-role]", rule.ResourceNames)
				}
				if len(rule.Verbs) != 1 || rule.Verbs[0] != "get" {
					t.Errorf("instances rule verbs = %v, want [get]", rule.Verbs)
				}
			}
		}
	}
	if !foundInstances {
		t.Error("missing openclawinstances rule")
	}

	// Check for openclawselfconfigs rule
	foundSelfConfigs := false
	for _, rule := range role.Rules {
		for _, res := range rule.Resources {
			if res != "openclawselfconfigs" {
				continue
			}
			foundSelfConfigs = true
			if len(rule.ResourceNames) > 0 {
				t.Error("selfconfigs rule should not have resourceNames (create cannot be scoped)")
			}
			verbSet := map[string]bool{}
			for _, v := range rule.Verbs {
				verbSet[v] = true
			}
			for _, expected := range []string{"create", "get", "list"} {
				if !verbSet[expected] {
					t.Errorf("selfconfigs rule missing verb %q", expected)
				}
			}
		}
	}
	if !foundSelfConfigs {
		t.Error("missing openclawselfconfigs rule")
	}

	// Check for secrets rule (gateway token secret should be included)
	foundSecrets := false
	for _, rule := range role.Rules {
		for _, res := range rule.Resources {
			if res != "secrets" {
				continue
			}
			foundSecrets = true
			if len(rule.ResourceNames) == 0 {
				t.Error("secrets rule should have resourceNames for scoping")
			}
			// Should include the gateway token secret
			found := false
			for _, name := range rule.ResourceNames {
				if name == "sc-role-gateway-token" {
					found = true
				}
			}
			if !found {
				t.Errorf("secrets rule resourceNames %v should include sc-role-gateway-token", rule.ResourceNames)
			}
		}
	}
	if !foundSecrets {
		t.Error("missing secrets rule")
	}
}

func TestBuildRole_SelfConfigureDisabled(t *testing.T) {
	instance := newTestInstance("sc-disabled")

	role := BuildRole(instance)

	// Only the 1 base rule (configmap)
	if len(role.Rules) != 1 {
		t.Fatalf("expected 1 rule, got %d", len(role.Rules))
	}

	// No openclaw.rocks rules
	for _, rule := range role.Rules {
		for _, ag := range rule.APIGroups {
			if ag == "openclaw.rocks" {
				t.Error("should not have openclaw.rocks rules when self-configure is disabled")
			}
		}
	}
}

func TestBuildRole_SelfConfigureWithEnvFromSecrets(t *testing.T) {
	instance := newTestInstance("sc-envfrom")
	instance.Spec.SelfConfigure = openclawv1alpha1.SelfConfigureSpec{
		Enabled:        true,
		AllowedActions: []openclawv1alpha1.SelfConfigAction{"skills"},
	}
	instance.Spec.EnvFrom = []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: "api-keys"},
			},
		},
	}

	role := BuildRole(instance)

	// Find secrets rule
	for _, rule := range role.Rules {
		for _, res := range rule.Resources {
			if res == "secrets" {
				// Should include both gateway token and envfrom secret
				nameSet := map[string]bool{}
				for _, name := range rule.ResourceNames {
					nameSet[name] = true
				}
				if !nameSet["api-keys"] {
					t.Error("secrets rule should include envFrom secret 'api-keys'")
				}
				if !nameSet["sc-envfrom-gateway-token"] {
					t.Error("secrets rule should include gateway token secret")
				}
			}
		}
	}
}

func TestBuildServiceAccount_SelfConfigureTokenMount(t *testing.T) {
	instance := newTestInstance("sc-sa-token")
	instance.Spec.SelfConfigure = openclawv1alpha1.SelfConfigureSpec{
		Enabled: true,
	}

	sa := BuildServiceAccount(instance)

	if sa.AutomountServiceAccountToken == nil || !*sa.AutomountServiceAccountToken {
		t.Error("AutomountServiceAccountToken should be true when self-configure is enabled")
	}
}

func TestBuildServiceAccount_SelfConfigureDisabledTokenMount(t *testing.T) {
	instance := newTestInstance("sc-sa-notoken")

	sa := BuildServiceAccount(instance)

	if sa.AutomountServiceAccountToken == nil || *sa.AutomountServiceAccountToken {
		t.Error("AutomountServiceAccountToken should be false when self-configure is disabled")
	}
}

func TestBuildStatefulSet_SelfConfigureEnvVars(t *testing.T) {
	instance := newTestInstance("sc-env")
	instance.Spec.SelfConfigure = openclawv1alpha1.SelfConfigureSpec{
		Enabled: true,
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	mainContainer := sts.Spec.Template.Spec.Containers[0]
	envMap := map[string]string{}
	for _, ev := range mainContainer.Env {
		envMap[ev.Name] = ev.Value
	}

	if envMap["OPENCLAW_INSTANCE_NAME"] != "sc-env" {
		t.Errorf("OPENCLAW_INSTANCE_NAME = %q, want %q", envMap["OPENCLAW_INSTANCE_NAME"], "sc-env")
	}
	if envMap["OPENCLAW_NAMESPACE"] != "test-ns" {
		t.Errorf("OPENCLAW_NAMESPACE = %q, want %q", envMap["OPENCLAW_NAMESPACE"], "test-ns")
	}
}

func TestBuildStatefulSet_SelfConfigureDisabledNoEnvVars(t *testing.T) {
	instance := newTestInstance("sc-noenv")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	mainContainer := sts.Spec.Template.Spec.Containers[0]
	for _, ev := range mainContainer.Env {
		if ev.Name == "OPENCLAW_INSTANCE_NAME" || ev.Name == "OPENCLAW_NAMESPACE" {
			t.Errorf("should not have %s when self-configure is disabled", ev.Name)
		}
	}
}

func TestBuildStatefulSet_SelfConfigureAutoMount(t *testing.T) {
	instance := newTestInstance("sc-automount")
	instance.Spec.SelfConfigure = openclawv1alpha1.SelfConfigureSpec{
		Enabled: true,
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	if sts.Spec.Template.Spec.AutomountServiceAccountToken == nil ||
		!*sts.Spec.Template.Spec.AutomountServiceAccountToken {
		t.Error("pod AutomountServiceAccountToken should be true when self-configure is enabled")
	}
}

func TestBuildNetworkPolicy_SelfConfigureEgress(t *testing.T) {
	instance := newTestInstance("sc-netpol")
	instance.Spec.SelfConfigure = openclawv1alpha1.SelfConfigureSpec{
		Enabled: true,
	}

	np := BuildNetworkPolicy(instance)

	// Check for port 6443 egress rule
	found6443 := false
	for _, rule := range np.Spec.Egress {
		for _, port := range rule.Ports {
			if port.Port != nil && port.Port.IntValue() == 6443 {
				found6443 = true
			}
		}
	}
	if !found6443 {
		t.Error("NetworkPolicy should have egress rule for K8s API port 6443")
	}
}

func TestBuildNetworkPolicy_SelfConfigureDisabledNo6443(t *testing.T) {
	instance := newTestInstance("sc-netpol-off")
	// Both selfConfigure and tailscale are disabled by default

	np := BuildNetworkPolicy(instance)

	for _, rule := range np.Spec.Egress {
		for _, port := range rule.Ports {
			if port.Port != nil && port.Port.IntValue() == 6443 {
				t.Error("NetworkPolicy should NOT have port 6443 when both self-configure and tailscale are disabled")
			}
		}
	}
}

func TestBuildWorkspaceConfigMap_SelfConfigureSkillInjected(t *testing.T) {
	instance := newTestInstance("sc-ws")
	instance.Spec.SelfConfigure = openclawv1alpha1.SelfConfigureSpec{
		Enabled: true,
	}

	cm := BuildWorkspaceConfigMap(instance, nil, nil, nil)

	if cm == nil {
		t.Fatal("BuildWorkspaceConfigMap returned nil when self-configure is enabled")
	}

	if _, ok := cm.Data["SELFCONFIG.md"]; !ok {
		t.Error("workspace ConfigMap missing SELFCONFIG.md")
	}
	if _, ok := cm.Data["selfconfig.sh"]; !ok {
		t.Error("workspace ConfigMap missing selfconfig.sh")
	}
}

func TestBuildWorkspaceConfigMap_SelfConfigureMergedWithUserFiles(t *testing.T) {
	instance := newTestInstance("sc-ws-merge")
	instance.Spec.SelfConfigure = openclawv1alpha1.SelfConfigureSpec{
		Enabled: true,
	}
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{
			"README.md": "# My Project",
			"notes.txt": "some notes",
		},
	}

	cm := BuildWorkspaceConfigMap(instance, nil, nil, nil)

	if cm == nil {
		t.Fatal("BuildWorkspaceConfigMap returned nil")
	}

	// User files preserved
	if cm.Data["README.md"] != "# My Project" {
		t.Error("user file README.md not preserved")
	}
	if cm.Data["notes.txt"] != "some notes" {
		t.Error("user file notes.txt not preserved")
	}

	// Self-configure files injected
	if _, ok := cm.Data["SELFCONFIG.md"]; !ok {
		t.Error("missing SELFCONFIG.md")
	}
	if _, ok := cm.Data["selfconfig.sh"]; !ok {
		t.Error("missing selfconfig.sh")
	}
}

func TestBuildWorkspaceConfigMap_SelfConfigureDisabledNoFiles(t *testing.T) {
	instance := newTestInstance("sc-ws-off")

	cm := BuildWorkspaceConfigMap(instance, nil, nil, nil)

	// Operator files are always injected, so ConfigMap is never nil
	if cm == nil {
		t.Fatal("expected non-nil ConfigMap (operator files are always injected)")
	}
	for _, f := range []string{"ENVIRONMENT.md", "BOOTSTRAP.md"} {
		if _, ok := cm.Data[f]; !ok {
			t.Errorf("expected %s in workspace ConfigMap", f)
		}
	}
	if _, ok := cm.Data["SELFCONFIG.md"]; ok {
		t.Error("SELFCONFIG.md should not be present when self-configure is disabled")
	}
}

func TestHasWorkspaceFiles_SelfConfigureEnabled(t *testing.T) {
	instance := newTestInstance("sc-hasws")
	instance.Spec.SelfConfigure = openclawv1alpha1.SelfConfigureSpec{
		Enabled: true,
	}

	if !hasWorkspaceFiles(instance, nil) {
		t.Error("hasWorkspaceFiles should return true when self-configure is enabled")
	}
}

func TestBuildStatefulSet_SelfConfigureWorkspaceVolume(t *testing.T) {
	instance := newTestInstance("sc-ws-vol")
	instance.Spec.SelfConfigure = openclawv1alpha1.SelfConfigureSpec{
		Enabled: true,
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Should have workspace-init volume
	foundVol := false
	for _, vol := range sts.Spec.Template.Spec.Volumes {
		if vol.Name == "workspace-init" {
			foundVol = true
			if vol.ConfigMap == nil {
				t.Error("workspace-init volume should reference a ConfigMap")
			} else if vol.ConfigMap.Name != "sc-ws-vol-workspace" {
				t.Errorf("workspace-init volume ConfigMap name = %q, want %q", vol.ConfigMap.Name, "sc-ws-vol-workspace")
			}
		}
	}
	if !foundVol {
		t.Error("missing workspace-init volume when self-configure is enabled")
	}
}

func TestBuildPrometheusRule_CustomRunbookURL(t *testing.T) {
	instance := newTestInstance("my-instance")
	instance.Spec.Observability.Metrics.PrometheusRule = &openclawv1alpha1.PrometheusRuleSpec{
		Enabled:        Ptr(true),
		RunbookBaseURL: "https://wiki.example.com/runbooks",
	}

	pr := BuildPrometheusRule(instance)

	spec := pr.Object["spec"].(map[string]interface{})
	groups := spec["groups"].([]interface{})
	group := groups[0].(map[string]interface{})
	rules := group["rules"].([]interface{})

	firstRule := rules[0].(map[string]interface{})
	annotations := firstRule["annotations"].(map[string]interface{})
	runbook := annotations["runbook_url"].(string)
	if !strings.HasPrefix(runbook, "https://wiki.example.com/runbooks/") {
		t.Errorf("runbook_url = %q, expected custom base URL", runbook)
	}
}

func TestBuildPrometheusRule_CustomLabels(t *testing.T) {
	instance := newTestInstance("my-instance")
	instance.Spec.Observability.Metrics.PrometheusRule = &openclawv1alpha1.PrometheusRuleSpec{
		Enabled: Ptr(true),
		Labels: map[string]string{
			"release": "kube-prometheus-stack",
		},
	}

	pr := BuildPrometheusRule(instance)
	labels := pr.GetLabels()

	if labels["release"] != "kube-prometheus-stack" {
		t.Errorf("custom label release = %q, want %q", labels["release"], "kube-prometheus-stack")
	}
	// Standard labels should still be present
	if labels["app.kubernetes.io/name"] != "openclaw" {
		t.Error("missing standard label")
	}
}

// ---------------------------------------------------------------------------
// Grafana dashboard tests
// ---------------------------------------------------------------------------

func TestGrafanaDashboardOperatorName(t *testing.T) {
	instance := newTestInstance("my-instance")
	name := GrafanaDashboardOperatorName(instance)
	if name != "my-instance-dashboard-operator" {
		t.Errorf("GrafanaDashboardOperatorName = %q, want %q", name, "my-instance-dashboard-operator")
	}
}

func TestGrafanaDashboardInstanceName(t *testing.T) {
	instance := newTestInstance("my-instance")
	name := GrafanaDashboardInstanceName(instance)
	if name != "my-instance-dashboard-instance" {
		t.Errorf("GrafanaDashboardInstanceName = %q, want %q", name, "my-instance-dashboard-instance")
	}
}

func TestBuildGrafanaDashboardOperator(t *testing.T) {
	instance := newTestInstance("my-instance")
	instance.Spec.Observability.Metrics.GrafanaDashboard = &openclawv1alpha1.GrafanaDashboardSpec{
		Enabled: Ptr(true),
	}

	cm := BuildGrafanaDashboardOperator(instance)

	// Check name
	if cm.Name != "my-instance-dashboard-operator" {
		t.Errorf("name = %q, want %q", cm.Name, "my-instance-dashboard-operator")
	}

	// Check grafana_dashboard label
	if cm.Labels["grafana_dashboard"] != "1" {
		t.Errorf("grafana_dashboard label = %q, want %q", cm.Labels["grafana_dashboard"], "1")
	}

	// Check grafana_folder annotation
	if cm.Annotations["grafana_folder"] != "OpenClaw" {
		t.Errorf("grafana_folder annotation = %q, want %q", cm.Annotations["grafana_folder"], "OpenClaw")
	}

	// Check data key exists and contains valid JSON
	dashJSON, ok := cm.Data["openclaw-operator.json"]
	if !ok {
		t.Fatal("missing openclaw-operator.json data key")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(dashJSON), &parsed); err != nil {
		t.Fatalf("invalid JSON in dashboard: %v", err)
	}

	// Check title
	if parsed["title"] != "OpenClaw Operator" {
		t.Errorf("dashboard title = %q, want %q", parsed["title"], "OpenClaw Operator")
	}

	// Check UID
	if parsed["uid"] != "openclaw-operator-overview" {
		t.Errorf("dashboard UID = %q, want %q", parsed["uid"], "openclaw-operator-overview")
	}

	// Check template variables exist
	templating, ok := parsed["templating"].(map[string]interface{})
	if !ok {
		t.Fatal("missing templating")
	}
	vars, ok := templating["list"].([]interface{})
	if !ok || len(vars) < 3 {
		t.Fatalf("expected at least 3 template variables, got %d", len(vars))
	}

	// Check panels exist (should be >10)
	panels, ok := parsed["panels"].([]interface{})
	if !ok {
		t.Fatal("missing panels")
	}
	if len(panels) < 10 {
		t.Errorf("expected at least 10 panels, got %d", len(panels))
	}
}

func TestBuildGrafanaDashboardInstance(t *testing.T) {
	instance := newTestInstance("my-instance")
	instance.Spec.Observability.Metrics.GrafanaDashboard = &openclawv1alpha1.GrafanaDashboardSpec{
		Enabled: Ptr(true),
	}

	cm := BuildGrafanaDashboardInstance(instance)

	// Check name
	if cm.Name != "my-instance-dashboard-instance" {
		t.Errorf("name = %q, want %q", cm.Name, "my-instance-dashboard-instance")
	}

	// Check grafana_dashboard label
	if cm.Labels["grafana_dashboard"] != "1" {
		t.Errorf("grafana_dashboard label = %q, want %q", cm.Labels["grafana_dashboard"], "1")
	}

	// Check data key exists and contains valid JSON
	dashJSON, ok := cm.Data["openclaw-instance.json"]
	if !ok {
		t.Fatal("missing openclaw-instance.json data key")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(dashJSON), &parsed); err != nil {
		t.Fatalf("invalid JSON in dashboard: %v", err)
	}

	// Check title
	if parsed["title"] != "OpenClaw Instance" {
		t.Errorf("dashboard title = %q, want %q", parsed["title"], "OpenClaw Instance")
	}

	// Check UID
	if parsed["uid"] != "openclaw-instance-detail" {
		t.Errorf("dashboard UID = %q, want %q", parsed["uid"], "openclaw-instance-detail")
	}

	// Check panels exist (should be >10)
	panels, ok := parsed["panels"].([]interface{})
	if !ok {
		t.Fatal("missing panels")
	}
	if len(panels) < 10 {
		t.Errorf("expected at least 10 panels, got %d", len(panels))
	}
}

func TestBuildGrafanaDashboard_CustomLabelsAndFolder(t *testing.T) {
	instance := newTestInstance("my-instance")
	instance.Spec.Observability.Metrics.GrafanaDashboard = &openclawv1alpha1.GrafanaDashboardSpec{
		Enabled: Ptr(true),
		Labels: map[string]string{
			"custom-label": "custom-value",
		},
		Folder: "Infrastructure",
	}

	cm := BuildGrafanaDashboardOperator(instance)

	// Check custom label
	if cm.Labels["custom-label"] != "custom-value" {
		t.Errorf("custom label = %q, want %q", cm.Labels["custom-label"], "custom-value")
	}

	// Check custom folder
	if cm.Annotations["grafana_folder"] != "Infrastructure" {
		t.Errorf("grafana_folder = %q, want %q", cm.Annotations["grafana_folder"], "Infrastructure")
	}

	// Standard labels should still be present
	if cm.Labels["grafana_dashboard"] != "1" {
		t.Error("missing grafana_dashboard label")
	}
	if cm.Labels["app.kubernetes.io/name"] != "openclaw" {
		t.Error("missing standard label")
	}
}

func TestPrometheusRuleGVK(t *testing.T) {
	gvk := PrometheusRuleGVK()
	if gvk.Group != "monitoring.coreos.com" {
		t.Errorf("group = %q, want %q", gvk.Group, "monitoring.coreos.com")
	}
	if gvk.Version != "v1" {
		t.Errorf("version = %q, want %q", gvk.Version, "v1")
	}
	if gvk.Kind != "PrometheusRule" {
		t.Errorf("kind = %q, want %q", gvk.Kind, "PrometheusRule")
	}
}

// ---------------------------------------------------------------------------
// HPA tests
// ---------------------------------------------------------------------------

func TestHPAName(t *testing.T) {
	instance := newTestInstance("my-app")
	if got := HPAName(instance); got != "my-app" {
		t.Errorf("HPAName() = %q, want %q", got, "my-app")
	}
}

func TestIsHPAEnabled(t *testing.T) {
	tests := []struct {
		name     string
		as       *openclawv1alpha1.AutoScalingSpec
		expected bool
	}{
		{"nil spec", nil, false},
		{"nil enabled", &openclawv1alpha1.AutoScalingSpec{}, false},
		{"enabled false", &openclawv1alpha1.AutoScalingSpec{Enabled: Ptr(false)}, false},
		{"enabled true", &openclawv1alpha1.AutoScalingSpec{Enabled: Ptr(true)}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := newTestInstance("test")
			instance.Spec.Availability.AutoScaling = tt.as
			if got := IsHPAEnabled(instance); got != tt.expected {
				t.Errorf("IsHPAEnabled() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestBuildHPA_Defaults(t *testing.T) {
	instance := newTestInstance("my-app")
	instance.Spec.Availability.AutoScaling = &openclawv1alpha1.AutoScalingSpec{
		Enabled: Ptr(true),
	}

	hpa := BuildHPA(instance)

	if hpa.Name != "my-app" {
		t.Errorf("name = %q, want %q", hpa.Name, "my-app")
	}
	if hpa.Namespace != "test-ns" {
		t.Errorf("namespace = %q, want %q", hpa.Namespace, "test-ns")
	}
	if hpa.Spec.ScaleTargetRef.Kind != "StatefulSet" {
		t.Errorf("target kind = %q, want StatefulSet", hpa.Spec.ScaleTargetRef.Kind)
	}
	if hpa.Spec.ScaleTargetRef.Name != StatefulSetName(instance) {
		t.Errorf("target name = %q, want %q", hpa.Spec.ScaleTargetRef.Name, StatefulSetName(instance))
	}
	if *hpa.Spec.MinReplicas != 1 {
		t.Errorf("minReplicas = %d, want 1", *hpa.Spec.MinReplicas)
	}
	if hpa.Spec.MaxReplicas != 5 {
		t.Errorf("maxReplicas = %d, want 5", hpa.Spec.MaxReplicas)
	}
	if len(hpa.Spec.Metrics) != 1 {
		t.Fatalf("metrics count = %d, want 1", len(hpa.Spec.Metrics))
	}
	if *hpa.Spec.Metrics[0].Resource.Target.AverageUtilization != 80 {
		t.Errorf("cpu target = %d, want 80", *hpa.Spec.Metrics[0].Resource.Target.AverageUtilization)
	}
}

func TestBuildHPA_CustomValues(t *testing.T) {
	instance := newTestInstance("my-app")
	instance.Spec.Availability.AutoScaling = &openclawv1alpha1.AutoScalingSpec{
		Enabled:              Ptr(true),
		MinReplicas:          Ptr(int32(2)),
		MaxReplicas:          Ptr(int32(10)),
		TargetCPUUtilization: Ptr(int32(60)),
	}

	hpa := BuildHPA(instance)

	if *hpa.Spec.MinReplicas != 2 {
		t.Errorf("minReplicas = %d, want 2", *hpa.Spec.MinReplicas)
	}
	if hpa.Spec.MaxReplicas != 10 {
		t.Errorf("maxReplicas = %d, want 10", hpa.Spec.MaxReplicas)
	}
	if *hpa.Spec.Metrics[0].Resource.Target.AverageUtilization != 60 {
		t.Errorf("cpu target = %d, want 60", *hpa.Spec.Metrics[0].Resource.Target.AverageUtilization)
	}
}

func TestBuildHPA_WithMemoryMetric(t *testing.T) {
	instance := newTestInstance("my-app")
	instance.Spec.Availability.AutoScaling = &openclawv1alpha1.AutoScalingSpec{
		Enabled:                 Ptr(true),
		TargetMemoryUtilization: Ptr(int32(70)),
	}

	hpa := BuildHPA(instance)

	if len(hpa.Spec.Metrics) != 2 {
		t.Fatalf("metrics count = %d, want 2", len(hpa.Spec.Metrics))
	}
	memMetric := hpa.Spec.Metrics[1]
	if string(memMetric.Resource.Name) != "memory" {
		t.Errorf("second metric resource = %q, want memory", memMetric.Resource.Name)
	}
	if *memMetric.Resource.Target.AverageUtilization != 70 {
		t.Errorf("memory target = %d, want 70", *memMetric.Resource.Target.AverageUtilization)
	}
}

func TestStatefulSetReplicas_HPAEnabled(t *testing.T) {
	instance := newTestInstance("my-app")
	instance.Spec.Availability.AutoScaling = &openclawv1alpha1.AutoScalingSpec{
		Enabled: Ptr(true),
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	if sts.Spec.Replicas != nil {
		t.Errorf("replicas should be nil when HPA is enabled, got %d", *sts.Spec.Replicas)
	}
}

func TestStatefulSetReplicas_HPADisabled(t *testing.T) {
	instance := newTestInstance("my-app")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 1 {
		t.Errorf("replicas should be 1 when HPA is disabled")
	}
}

func TestStatefulSetReplicas_Suspended(t *testing.T) {
	instance := newTestInstance("my-app")
	instance.Spec.Suspended = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 0 {
		t.Errorf("replicas should be 0 when suspended, got %v", sts.Spec.Replicas)
	}
}

func TestStatefulSetReplicas_SuspendedOverridesHPA(t *testing.T) {
	instance := newTestInstance("my-app")
	instance.Spec.Suspended = true
	instance.Spec.Availability.AutoScaling = &openclawv1alpha1.AutoScalingSpec{
		Enabled: Ptr(true),
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 0 {
		t.Errorf("replicas should be 0 when suspended (even with HPA), got %v", sts.Spec.Replicas)
	}
}

// ---------------------------------------------------------------------------
// Metrics port tests
// ---------------------------------------------------------------------------

func TestIsMetricsEnabled_DefaultTrue(t *testing.T) {
	instance := newTestInstance("metrics-default")
	if !IsMetricsEnabled(instance) {
		t.Error("IsMetricsEnabled() should return true when Enabled is nil (default)")
	}
}

func TestIsMetricsEnabled_ExplicitTrue(t *testing.T) {
	instance := newTestInstance("metrics-enabled")
	instance.Spec.Observability.Metrics.Enabled = Ptr(true)
	if !IsMetricsEnabled(instance) {
		t.Error("IsMetricsEnabled() should return true when explicitly enabled")
	}
}

func TestIsMetricsEnabled_ExplicitFalse(t *testing.T) {
	instance := newTestInstance("metrics-disabled")
	instance.Spec.Observability.Metrics.Enabled = Ptr(false)
	if IsMetricsEnabled(instance) {
		t.Error("IsMetricsEnabled() should return false when explicitly disabled")
	}
}

func TestMetricsPort_Default(t *testing.T) {
	instance := newTestInstance("metrics-port-default")
	if got := MetricsPort(instance); got != DefaultMetricsPort {
		t.Errorf("MetricsPort() = %d, want %d", got, DefaultMetricsPort)
	}
}

func TestMetricsPort_Custom(t *testing.T) {
	instance := newTestInstance("metrics-port-custom")
	instance.Spec.Observability.Metrics.Port = Ptr(int32(8080))
	if got := MetricsPort(instance); got != 8080 {
		t.Errorf("MetricsPort() = %d, want 8080", got)
	}
}

func TestBuildServiceMonitor_MetricsPort(t *testing.T) {
	instance := newTestInstance("sm-port")
	instance.Spec.Observability.Metrics.ServiceMonitor = &openclawv1alpha1.ServiceMonitorSpec{
		Enabled: Ptr(true),
	}

	sm := BuildServiceMonitor(instance)

	endpoints, ok := sm.Object["spec"].(map[string]interface{})["endpoints"].([]interface{})
	if !ok || len(endpoints) == 0 {
		t.Fatal("ServiceMonitor should have at least one endpoint")
	}
	ep := endpoints[0].(map[string]interface{})
	if ep["port"] != "metrics" {
		t.Errorf("ServiceMonitor endpoint port = %q, want %q", ep["port"], "metrics")
	}
}

func TestBuildService_MetricsPortEnabled(t *testing.T) {
	instance := newTestInstance("svc-metrics-enabled")
	// Metrics enabled by default (nil)

	svc := BuildService(instance)

	found := false
	for _, p := range svc.Spec.Ports {
		if p.Name == "metrics" {
			found = true
			if p.Port != DefaultMetricsPort {
				t.Errorf("metrics service port = %d, want %d", p.Port, DefaultMetricsPort)
			}
			if p.TargetPort.IntValue() != int(DefaultMetricsPort) {
				t.Errorf("metrics target port = %d, want %d", p.TargetPort.IntValue(), DefaultMetricsPort)
			}
		}
	}
	if !found {
		t.Error("service should include metrics port when metrics is enabled")
	}
}

func TestBuildService_MetricsPortDisabled(t *testing.T) {
	instance := newTestInstance("svc-metrics-disabled")
	instance.Spec.Observability.Metrics.Enabled = Ptr(false)

	svc := BuildService(instance)

	for _, p := range svc.Spec.Ports {
		if p.Name == "metrics" {
			t.Error("service should not include metrics port when metrics is disabled")
		}
	}
	if len(svc.Spec.Ports) != 2 {
		t.Errorf("expected 2 ports (gateway, canvas) when metrics disabled, got %d", len(svc.Spec.Ports))
	}
}

func TestBuildService_MetricsPortCustom(t *testing.T) {
	instance := newTestInstance("svc-metrics-custom")
	instance.Spec.Observability.Metrics.Port = Ptr(int32(8080))

	svc := BuildService(instance)

	assertServicePort(t, svc.Spec.Ports, "metrics", 8080)
}

func TestBuildStatefulSet_MetricsPortEnabled(t *testing.T) {
	instance := newTestInstance("sts-metrics-enabled")
	// Metrics enabled by default (nil)

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// Metrics port should be on the otel-collector container, not main
	main := sts.Spec.Template.Spec.Containers[0]
	for _, p := range main.Ports {
		if p.Name == "metrics" {
			t.Error("main container should not have metrics port (belongs on otel-collector)")
		}
	}

	// Find otel-collector and verify metrics port
	found := false
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "otel-collector" {
			found = true
			assertContainerPort(t, c.Ports, "metrics", DefaultMetricsPort)
		}
	}
	if !found {
		t.Error("otel-collector container should be present when metrics is enabled")
	}
}

func TestBuildStatefulSet_MetricsPortDisabled(t *testing.T) {
	instance := newTestInstance("sts-metrics-disabled")
	instance.Spec.Observability.Metrics.Enabled = Ptr(false)

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "otel-collector" {
			t.Error("otel-collector should not be present when metrics is disabled")
		}
	}

	main := sts.Spec.Template.Spec.Containers[0]
	if len(main.Ports) != 2 {
		t.Errorf("expected 2 ports (gateway, canvas) when metrics disabled, got %d", len(main.Ports))
	}
}

func TestBuildStatefulSet_MetricsPortCustom(t *testing.T) {
	instance := newTestInstance("sts-metrics-custom")
	instance.Spec.Observability.Metrics.Port = Ptr(int32(8080))

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "otel-collector" {
			assertContainerPort(t, c.Ports, "metrics", 8080)
			return
		}
	}
	t.Error("otel-collector container should be present")
}

// ---------------------------------------------------------------------------
// Web terminal sidecar tests
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_WebTerminalEnabled(t *testing.T) {
	instance := newTestInstance("web-terminal-test")
	instance.Spec.WebTerminal.Enabled = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	containers := sts.Spec.Template.Spec.Containers

	if len(containers) != 4 {
		t.Fatalf("expected 4 containers (main + gateway-proxy + web-terminal + otel-collector), got %d", len(containers))
	}

	var wt *corev1.Container
	for i := range containers {
		if containers[i].Name == "web-terminal" {
			wt = &containers[i]
			break
		}
	}
	if wt == nil {
		t.Fatal("web-terminal container not found")
	}

	// Image defaults
	if wt.Image != "tsl0922/ttyd:latest" {
		t.Errorf("web-terminal image = %q, want default", wt.Image)
	}

	// Port
	if len(wt.Ports) != 1 {
		t.Fatalf("web-terminal container should have 1 port, got %d", len(wt.Ports))
	}
	if wt.Ports[0].ContainerPort != WebTerminalPort {
		t.Errorf("web-terminal port = %d, want %d", wt.Ports[0].ContainerPort, WebTerminalPort)
	}
	if wt.Ports[0].Name != "web-terminal" {
		t.Errorf("web-terminal port name = %q, want %q", wt.Ports[0].Name, "web-terminal")
	}

	// Security context
	sc := wt.SecurityContext
	if sc == nil {
		t.Fatal("web-terminal security context is nil")
	}
	if sc.ReadOnlyRootFilesystem == nil || *sc.ReadOnlyRootFilesystem {
		t.Error("web-terminal: readOnlyRootFilesystem should be false")
	}
	if sc.RunAsUser == nil || *sc.RunAsUser != 1000 {
		t.Errorf("web-terminal: runAsUser = %v, want 1000", sc.RunAsUser)
	}
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Error("web-terminal: runAsNonRoot should be true")
	}
	if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		t.Error("web-terminal: allowPrivilegeEscalation should be false")
	}

	// Resource defaults
	cpuReq := wt.Resources.Requests[corev1.ResourceCPU]
	if cpuReq.String() != "50m" {
		t.Errorf("web-terminal cpu request = %v, want 50m", cpuReq.String())
	}
	memReq := wt.Resources.Requests[corev1.ResourceMemory]
	if memReq.Cmp(resource.MustParse("64Mi")) != 0 {
		t.Errorf("web-terminal memory request = %v, want 64Mi", memReq.String())
	}
	cpuLim := wt.Resources.Limits[corev1.ResourceCPU]
	if cpuLim.String() != "200m" {
		t.Errorf("web-terminal cpu limit = %v, want 200m", cpuLim.String())
	}
	memLim := wt.Resources.Limits[corev1.ResourceMemory]
	if memLim.Cmp(resource.MustParse("128Mi")) != 0 {
		t.Errorf("web-terminal memory limit = %v, want 128Mi", memLim.String())
	}

	// Volume mounts
	assertVolumeMount(t, wt.VolumeMounts, "data", "/home/openclaw/.openclaw")
	assertVolumeMount(t, wt.VolumeMounts, "web-terminal-tmp", "/tmp")

	// Data mount should NOT be read-only by default
	for _, m := range wt.VolumeMounts {
		if m.Name == "data" && m.ReadOnly {
			t.Error("data mount should not be read-only by default")
		}
	}

	// Volumes - check web-terminal-tmp volume exists
	volumes := sts.Spec.Template.Spec.Volumes
	var wtVol *corev1.Volume
	for i := range volumes {
		if volumes[i].Name == "web-terminal-tmp" {
			wtVol = &volumes[i]
			break
		}
	}
	if wtVol == nil {
		t.Fatal("web-terminal-tmp volume not found")
	}
	if wtVol.EmptyDir == nil {
		t.Fatal("web-terminal-tmp volume should be emptyDir")
	}

	// Command
	if len(wt.Command) != 3 {
		t.Fatalf("web-terminal command should have 3 elements, got %d", len(wt.Command))
	}
	if wt.Command[0] != "sh" || wt.Command[1] != "-c" {
		t.Errorf("web-terminal command prefix = %v, want [sh -c ...]", wt.Command[:2])
	}
	if !strings.Contains(wt.Command[2], "exec ttyd") {
		t.Errorf("web-terminal command should contain 'exec ttyd', got %q", wt.Command[2])
	}
	// Default (ReadOnly: false) should include -W flag for writable mode
	if !strings.Contains(wt.Command[2], "-W") {
		t.Errorf("web-terminal command should contain '-W' for writable mode by default, got %q", wt.Command[2])
	}

	// No credential env vars by default
	if len(wt.Env) != 0 {
		t.Errorf("web-terminal should have 0 env vars by default, got %d", len(wt.Env))
	}
}

func TestBuildStatefulSet_WebTerminalCustomImage(t *testing.T) {
	instance := newTestInstance("wt-custom-img")
	instance.Spec.WebTerminal.Enabled = true
	instance.Spec.WebTerminal.Image = openclawv1alpha1.WebTerminalImageSpec{
		Repository: "my-registry.io/ttyd",
		Tag:        "v1.7.0",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var wt corev1.Container
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "web-terminal" {
			wt = c
			break
		}
	}

	if wt.Image != "my-registry.io/ttyd:v1.7.0" {
		t.Errorf("web-terminal image = %q, want my-registry.io/ttyd:v1.7.0", wt.Image)
	}
}

func TestBuildStatefulSet_WebTerminalDigest(t *testing.T) {
	instance := newTestInstance("wt-digest")
	instance.Spec.WebTerminal.Enabled = true
	instance.Spec.WebTerminal.Image = openclawv1alpha1.WebTerminalImageSpec{
		Repository: "tsl0922/ttyd",
		Digest:     "sha256:abcdef1234567890",
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var wt corev1.Container
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "web-terminal" {
			wt = c
			break
		}
	}

	if wt.Image != "tsl0922/ttyd@sha256:abcdef1234567890" {
		t.Errorf("web-terminal image = %q, want tsl0922/ttyd@sha256:abcdef1234567890", wt.Image)
	}
}

func TestBuildStatefulSet_WebTerminalCustomResources(t *testing.T) {
	instance := newTestInstance("wt-resources")
	instance.Spec.WebTerminal.Enabled = true
	instance.Spec.WebTerminal.Resources = openclawv1alpha1.ResourcesSpec{
		Requests: openclawv1alpha1.ResourceList{CPU: "100m", Memory: "128Mi"},
		Limits:   openclawv1alpha1.ResourceList{CPU: "500m", Memory: "256Mi"},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var wt corev1.Container
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "web-terminal" {
			wt = c
			break
		}
	}

	cpuReq := wt.Resources.Requests[corev1.ResourceCPU]
	if cpuReq.String() != "100m" {
		t.Errorf("cpu request = %v, want 100m", cpuReq.String())
	}
	memReq := wt.Resources.Requests[corev1.ResourceMemory]
	if memReq.Cmp(resource.MustParse("128Mi")) != 0 {
		t.Errorf("memory request = %v, want 128Mi", memReq.String())
	}
	cpuLim := wt.Resources.Limits[corev1.ResourceCPU]
	if cpuLim.String() != "500m" {
		t.Errorf("cpu limit = %v, want 500m", cpuLim.String())
	}
	memLim := wt.Resources.Limits[corev1.ResourceMemory]
	if memLim.Cmp(resource.MustParse("256Mi")) != 0 {
		t.Errorf("memory limit = %v, want 256Mi", memLim.String())
	}
}

func TestBuildStatefulSet_WebTerminalReadOnly(t *testing.T) {
	instance := newTestInstance("wt-readonly")
	instance.Spec.WebTerminal.Enabled = true
	instance.Spec.WebTerminal.ReadOnly = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var wt corev1.Container
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "web-terminal" {
			wt = c
			break
		}
	}

	// Command should include -R flag
	if !strings.Contains(wt.Command[2], "-R") {
		t.Errorf("web-terminal command should contain '-R' for read-only, got %q", wt.Command[2])
	}
	// Should NOT include -W flag
	if strings.Contains(wt.Command[2], "-W") {
		t.Errorf("web-terminal command should NOT contain '-W' in read-only mode, got %q", wt.Command[2])
	}

	// Data mount should be read-only
	for _, m := range wt.VolumeMounts {
		if m.Name == "data" && !m.ReadOnly {
			t.Error("data mount should be read-only when readOnly is true")
		}
	}
}

func TestBuildStatefulSet_WebTerminalCredential(t *testing.T) {
	instance := newTestInstance("wt-cred")
	instance.Spec.WebTerminal.Enabled = true
	instance.Spec.WebTerminal.Credential = &openclawv1alpha1.WebTerminalCredentialSpec{
		SecretRef: corev1.LocalObjectReference{Name: "wt-secret"},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var wt corev1.Container
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "web-terminal" {
			wt = c
			break
		}
	}

	// Command should include -c flag with credential env vars
	if !strings.Contains(wt.Command[2], `-c "${TTYD_USERNAME}:${TTYD_PASSWORD}"`) {
		t.Errorf("web-terminal command should contain credential flag, got %q", wt.Command[2])
	}

	// Should have TTYD_USERNAME and TTYD_PASSWORD env vars
	if len(wt.Env) != 2 {
		t.Fatalf("expected 2 env vars, got %d", len(wt.Env))
	}

	var foundUsername, foundPassword bool
	for _, env := range wt.Env {
		if env.Name == "TTYD_USERNAME" {
			foundUsername = true
			if env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil {
				t.Error("TTYD_USERNAME should use secretKeyRef")
			} else {
				if env.ValueFrom.SecretKeyRef.Name != "wt-secret" {
					t.Errorf("TTYD_USERNAME secret name = %q, want wt-secret", env.ValueFrom.SecretKeyRef.Name)
				}
				if env.ValueFrom.SecretKeyRef.Key != "username" {
					t.Errorf("TTYD_USERNAME secret key = %q, want username", env.ValueFrom.SecretKeyRef.Key)
				}
			}
		}
		if env.Name == "TTYD_PASSWORD" {
			foundPassword = true
			if env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil {
				t.Error("TTYD_PASSWORD should use secretKeyRef")
			} else {
				if env.ValueFrom.SecretKeyRef.Name != "wt-secret" {
					t.Errorf("TTYD_PASSWORD secret name = %q, want wt-secret", env.ValueFrom.SecretKeyRef.Name)
				}
				if env.ValueFrom.SecretKeyRef.Key != "password" {
					t.Errorf("TTYD_PASSWORD secret key = %q, want password", env.ValueFrom.SecretKeyRef.Key)
				}
			}
		}
	}
	if !foundUsername {
		t.Error("TTYD_USERNAME env var not found")
	}
	if !foundPassword {
		t.Error("TTYD_PASSWORD env var not found")
	}
}

func TestBuildStatefulSet_WebTerminalDisabled(t *testing.T) {
	instance := newTestInstance("no-web-terminal")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// No web-terminal container
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "web-terminal" {
			t.Error("web-terminal container should not be present when disabled")
		}
	}

	// No web-terminal-tmp volume
	for _, v := range sts.Spec.Template.Spec.Volumes {
		if v.Name == "web-terminal-tmp" {
			t.Error("web-terminal-tmp volume should not be present when disabled")
		}
	}
}

func TestBuildStatefulSet_WebTerminalContainerDefaults(t *testing.T) {
	instance := newTestInstance("wt-defaults")
	instance.Spec.WebTerminal.Enabled = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var wt corev1.Container
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "web-terminal" {
			wt = c
			break
		}
	}

	// Verify Kubernetes default fields are set
	if wt.TerminationMessagePath != corev1.TerminationMessagePathDefault {
		t.Errorf("TerminationMessagePath = %q, want default", wt.TerminationMessagePath)
	}
	if wt.TerminationMessagePolicy != corev1.TerminationMessageReadFile {
		t.Errorf("TerminationMessagePolicy = %v, want ReadFile", wt.TerminationMessagePolicy)
	}
	if wt.ImagePullPolicy != corev1.PullIfNotPresent {
		t.Errorf("ImagePullPolicy = %v, want IfNotPresent", wt.ImagePullPolicy)
	}

	// Seccomp profile
	if wt.SecurityContext.SeccompProfile == nil {
		t.Fatal("seccomp profile should be set")
	}
	if wt.SecurityContext.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Errorf("seccomp type = %v, want RuntimeDefault", wt.SecurityContext.SeccompProfile.Type)
	}
}

func TestBuildService_WithWebTerminal(t *testing.T) {
	instance := newTestInstance("svc-web-terminal")
	instance.Spec.WebTerminal.Enabled = true

	svc := BuildService(instance)

	if len(svc.Spec.Ports) != 4 {
		t.Fatalf("expected 4 ports with web terminal (gateway, canvas, web-terminal, metrics), got %d", len(svc.Spec.Ports))
	}

	// gateway and canvas use proxy targetPorts; web-terminal and metrics are direct
	for _, p := range svc.Spec.Ports {
		switch p.Name {
		case "gateway":
			if p.Port != int32(GatewayPort) {
				t.Errorf("gateway port = %d, want %d", p.Port, GatewayPort)
			}
			if p.TargetPort.IntValue() != int(GatewayProxyPort) {
				t.Errorf("gateway targetPort = %d, want %d", p.TargetPort.IntValue(), GatewayProxyPort)
			}
		case "canvas":
			if p.Port != int32(CanvasPort) {
				t.Errorf("canvas port = %d, want %d", p.Port, CanvasPort)
			}
			if p.TargetPort.IntValue() != int(CanvasProxyPort) {
				t.Errorf("canvas targetPort = %d, want %d", p.TargetPort.IntValue(), CanvasProxyPort)
			}
		case "web-terminal":
			assertServicePort(t, svc.Spec.Ports, "web-terminal", int32(WebTerminalPort))
		case "metrics":
			assertServicePort(t, svc.Spec.Ports, "metrics", DefaultMetricsPort)
		}
	}
}

func TestBuildNetworkPolicy_WebTerminalIngressPort(t *testing.T) {
	instance := newTestInstance("np-web-terminal")
	instance.Spec.WebTerminal.Enabled = true

	np := BuildNetworkPolicy(instance)

	// Default ingress rule should have 4 ports (gateway, canvas, web-terminal, metrics)
	if len(np.Spec.Ingress) == 0 {
		t.Fatal("expected at least one ingress rule")
	}

	ports := np.Spec.Ingress[0].Ports
	if len(ports) != 4 {
		t.Fatalf("expected 4 ingress ports with web terminal, got %d", len(ports))
	}

	// Verify web-terminal port is present
	found := false
	for _, p := range ports {
		if p.Port != nil && p.Port.IntValue() == WebTerminalPort {
			found = true
			break
		}
	}
	if !found {
		t.Error("web-terminal port not found in NetworkPolicy ingress ports")
	}
}

func TestBuildNetworkPolicy_ChromiumIngressAndEgress(t *testing.T) {
	instance := newTestInstance("np-chromium")
	instance.Spec.Chromium.Enabled = true

	np := BuildNetworkPolicy(instance)

	// Ingress: chromium port (9222) should be in ingress ports
	if len(np.Spec.Ingress) == 0 {
		t.Fatal("expected at least one ingress rule")
	}

	ports := np.Spec.Ingress[0].Ports
	if len(ports) != 4 {
		t.Fatalf("expected 4 ingress ports with chromium (gateway, canvas, chromium, metrics), got %d", len(ports))
	}

	foundChromiumIngress := false
	for _, p := range ports {
		if p.Port != nil && p.Port.IntValue() == ChromiumPort {
			foundChromiumIngress = true
		}
	}
	if !foundChromiumIngress {
		t.Error("chromium port not found in NetworkPolicy ingress ports")
	}

	// Egress: should have a rule for chromium CDP self-traffic (port 9222)
	foundChromiumEgress := false
	for _, rule := range np.Spec.Egress {
		for _, p := range rule.Ports {
			if p.Port != nil && p.Port.IntValue() == ChromiumPort {
				foundChromiumEgress = true
				// Verify it targets the same pod via pod selector
				if len(rule.To) != 1 {
					t.Errorf("chromium egress rule should have 1 peer, got %d", len(rule.To))
				} else if rule.To[0].PodSelector == nil {
					t.Error("chromium egress rule should use podSelector for self-traffic")
				}
			}
		}
	}
	if !foundChromiumEgress {
		t.Error("chromium egress rule (port 9222) not found in NetworkPolicy")
	}
}

func TestBuildStatefulSet_WebTerminalReadOnlyWithCredential(t *testing.T) {
	instance := newTestInstance("wt-readonly-cred")
	instance.Spec.WebTerminal.Enabled = true
	instance.Spec.WebTerminal.ReadOnly = true
	instance.Spec.WebTerminal.Credential = &openclawv1alpha1.WebTerminalCredentialSpec{
		SecretRef: corev1.LocalObjectReference{Name: "cred-secret"},
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var wt corev1.Container
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "web-terminal" {
			wt = c
			break
		}
	}

	// Command should include both -R and -c flags
	if !strings.Contains(wt.Command[2], "-R") {
		t.Errorf("command should contain '-R', got %q", wt.Command[2])
	}
	if !strings.Contains(wt.Command[2], `-c "${TTYD_USERNAME}:${TTYD_PASSWORD}"`) {
		t.Errorf("command should contain credential flag, got %q", wt.Command[2])
	}
}

// ---------------------------------------------------------------------------
// Gateway proxy sidecar tests
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_HasGatewayProxyContainer(t *testing.T) {
	instance := newTestInstance("gw-proxy")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var proxy *corev1.Container
	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == "gateway-proxy" {
			proxy = &sts.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if proxy == nil {
		t.Fatal("gateway-proxy container not found")
	}

	// Image
	if proxy.Image != DefaultGatewayProxyImage {
		t.Errorf("proxy image = %q, want %q", proxy.Image, DefaultGatewayProxyImage)
	}

	// Ports
	if len(proxy.Ports) != 2 {
		t.Fatalf("expected 2 proxy ports, got %d", len(proxy.Ports))
	}
	assertContainerPort(t, proxy.Ports, "gw-proxy", GatewayProxyPort)
	assertContainerPort(t, proxy.Ports, "canvas-proxy", CanvasProxyPort)

	// Security context
	sc := proxy.SecurityContext
	if sc == nil {
		t.Fatal("proxy security context is nil")
	}
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		t.Error("proxy should run as non-root")
	}
	if sc.RunAsUser == nil || *sc.RunAsUser != 101 {
		t.Errorf("proxy runAsUser = %v, want 101 (nginx)", sc.RunAsUser)
	}
	if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
		t.Error("proxy should have read-only rootfs")
	}
	if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		t.Error("proxy should not allow privilege escalation")
	}
	if sc.Capabilities == nil || len(sc.Capabilities.Drop) != 1 || sc.Capabilities.Drop[0] != "ALL" {
		t.Error("proxy should drop ALL capabilities")
	}
	if sc.SeccompProfile == nil || sc.SeccompProfile.Type != corev1.SeccompProfileTypeRuntimeDefault {
		t.Error("proxy seccomp should be RuntimeDefault")
	}

	// Volume mounts
	foundConfig := false
	foundTmp := false
	for _, vm := range proxy.VolumeMounts {
		switch vm.Name {
		case "config":
			foundConfig = true
			if vm.SubPath != NginxConfigKey {
				t.Errorf("config mount subPath = %q, want %q", vm.SubPath, NginxConfigKey)
			}
			if !vm.ReadOnly {
				t.Error("config mount should be read-only")
			}
		case "gateway-proxy-tmp":
			foundTmp = true
		}
	}
	if !foundConfig {
		t.Error("proxy should mount config volume")
	}
	if !foundTmp {
		t.Error("proxy should mount gateway-proxy-tmp volume")
	}

	// Resources
	cpuReq := proxy.Resources.Requests[corev1.ResourceCPU]
	if cpuReq.String() != "10m" {
		t.Errorf("proxy cpu request = %v, want 10m", cpuReq.String())
	}
	memReq := proxy.Resources.Requests[corev1.ResourceMemory]
	if memReq.Cmp(resource.MustParse("16Mi")) != 0 {
		t.Errorf("proxy memory request = %v, want 16Mi", memReq.String())
	}
}

func TestBuildStatefulSet_GatewayProxyTmpVolume(t *testing.T) {
	instance := newTestInstance("gw-proxy-vol")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	found := false
	for _, v := range sts.Spec.Template.Spec.Volumes {
		if v.Name == "gateway-proxy-tmp" {
			found = true
			if v.EmptyDir == nil {
				t.Error("gateway-proxy-tmp should be an emptyDir volume")
			}
			break
		}
	}
	if !found {
		t.Error("gateway-proxy-tmp volume not found")
	}
}

func TestBuildConfigMap_ContainsNginxConfig(t *testing.T) {
	instance := newTestInstance("nginx-cfg")
	cm := BuildConfigMap(instance, "", nil)

	nginxConf, ok := cm.Data[NginxConfigKey]
	if !ok {
		t.Fatal("ConfigMap should contain nginx.conf key")
	}
	if !strings.Contains(nginxConf, fmt.Sprintf("%d", GatewayProxyPort)) {
		t.Errorf("nginx config should contain proxy port %d", GatewayProxyPort)
	}
	if !strings.Contains(nginxConf, fmt.Sprintf("%d", CanvasProxyPort)) {
		t.Errorf("nginx config should contain canvas proxy port %d", CanvasProxyPort)
	}
	if !strings.Contains(nginxConf, fmt.Sprintf("127.0.0.1:%d", GatewayPort)) {
		t.Errorf("nginx config should proxy to 127.0.0.1:%d", GatewayPort)
	}
	if !strings.Contains(nginxConf, fmt.Sprintf("127.0.0.1:%d", CanvasPort)) {
		t.Errorf("nginx config should proxy to 127.0.0.1:%d", CanvasPort)
	}
	if !strings.Contains(nginxConf, "pid /tmp/nginx.pid") {
		t.Error("nginx config should use /tmp for pid file")
	}
	if !strings.Contains(nginxConf, "events {") {
		t.Error("nginx config must contain an events block")
	}
}

func TestBuildService_DefaultTargetsProxyPorts(t *testing.T) {
	instance := newTestInstance("svc-proxy")
	svc := BuildService(instance)

	for _, port := range svc.Spec.Ports {
		switch port.Name {
		case "gateway":
			if port.Port != int32(GatewayPort) {
				t.Errorf("gateway service port = %d, want %d", port.Port, GatewayPort)
			}
			if port.TargetPort.IntValue() != int(GatewayProxyPort) {
				t.Errorf("gateway targetPort = %d, want %d (proxy port)", port.TargetPort.IntValue(), GatewayProxyPort)
			}
		case "canvas":
			if port.Port != int32(CanvasPort) {
				t.Errorf("canvas service port = %d, want %d", port.Port, CanvasPort)
			}
			if port.TargetPort.IntValue() != int(CanvasProxyPort) {
				t.Errorf("canvas targetPort = %d, want %d (proxy port)", port.TargetPort.IntValue(), CanvasProxyPort)
			}
		}
	}
}

func TestBuildNetworkPolicy_DefaultUsesProxyPorts(t *testing.T) {
	instance := newTestInstance("np-proxy")
	np := BuildNetworkPolicy(instance)

	if len(np.Spec.Ingress) == 0 {
		t.Fatal("expected at least one ingress rule")
	}

	ports := np.Spec.Ingress[0].Ports
	foundGW := false
	foundCanvas := false
	for _, p := range ports {
		if p.Port != nil {
			switch p.Port.IntValue() {
			case int(GatewayProxyPort):
				foundGW = true
			case int(CanvasProxyPort):
				foundCanvas = true
			}
		}
	}
	if !foundGW {
		t.Errorf("NetworkPolicy should allow port %d (gateway proxy)", GatewayProxyPort)
	}
	if !foundCanvas {
		t.Errorf("NetworkPolicy should allow port %d (canvas proxy)", CanvasProxyPort)
	}
}

func TestBuildNetworkPolicy_MetricsPortIncludedByDefault(t *testing.T) {
	instance := newTestInstance("np-metrics")
	np := BuildNetworkPolicy(instance)

	ports := np.Spec.Ingress[0].Ports
	found := false
	for _, p := range ports {
		if p.Port != nil && p.Port.IntValue() == int(DefaultMetricsPort) {
			found = true
		}
	}
	if !found {
		t.Errorf("NetworkPolicy should allow metrics port %d when metrics are enabled (default)", DefaultMetricsPort)
	}
}

func TestBuildNetworkPolicy_MetricsPortExcludedWhenDisabled(t *testing.T) {
	instance := newTestInstance("np-no-metrics")
	instance.Spec.Observability.Metrics.Enabled = Ptr(false)
	np := BuildNetworkPolicy(instance)

	ports := np.Spec.Ingress[0].Ports
	for _, p := range ports {
		if p.Port != nil && p.Port.IntValue() == int(DefaultMetricsPort) {
			t.Errorf("NetworkPolicy should NOT allow metrics port %d when metrics are disabled", DefaultMetricsPort)
		}
	}
}

func TestBuildNetworkPolicy_CustomMetricsPort(t *testing.T) {
	instance := newTestInstance("np-custom-metrics")
	instance.Spec.Observability.Metrics.Port = Ptr(int32(8080))
	np := BuildNetworkPolicy(instance)

	ports := np.Spec.Ingress[0].Ports
	found := false
	for _, p := range ports {
		if p.Port != nil && p.Port.IntValue() == 8080 {
			found = true
		}
	}
	if !found {
		t.Error("NetworkPolicy should allow custom metrics port 8080")
	}
}

func TestBuildNetworkPolicy_MetricsPortWithCustomServicePorts(t *testing.T) {
	instance := newTestInstance("np-custom-svc-metrics")
	instance.Spec.Networking.Service.Ports = []openclawv1alpha1.ServicePortSpec{
		{Name: "http", Port: 3978},
	}
	np := BuildNetworkPolicy(instance)

	ports := np.Spec.Ingress[0].Ports
	foundCustom := false
	foundMetrics := false
	for _, p := range ports {
		if p.Port != nil {
			switch p.Port.IntValue() {
			case 3978:
				foundCustom = true
			case int(DefaultMetricsPort):
				foundMetrics = true
			}
		}
	}
	if !foundCustom {
		t.Error("NetworkPolicy should allow custom service port 3978")
	}
	if !foundMetrics {
		t.Errorf("NetworkPolicy should allow metrics port %d even with custom service ports", DefaultMetricsPort)
	}
}

func TestBuildNetworkPolicy_MetricsPortOnAllIngressRules(t *testing.T) {
	instance := newTestInstance("np-metrics-all-rules")
	instance.Spec.Security.NetworkPolicy.AllowedIngressNamespaces = []string{"monitoring"}
	np := BuildNetworkPolicy(instance)

	for i, rule := range np.Spec.Ingress {
		found := false
		for _, p := range rule.Ports {
			if p.Port != nil && p.Port.IntValue() == int(DefaultMetricsPort) {
				found = true
			}
		}
		if !found {
			t.Errorf("ingress rule %d should include metrics port %d", i, DefaultMetricsPort)
		}
	}
}

func TestBuildNetworkPolicy_GatewayProxyDisabled_UsesDirectPorts(t *testing.T) {
	instance := newTestInstance("np-no-proxy")
	instance.Spec.Gateway.Enabled = Ptr(false)
	np := BuildNetworkPolicy(instance)

	if len(np.Spec.Ingress) == 0 {
		t.Fatal("expected at least one ingress rule")
	}

	ports := np.Spec.Ingress[0].Ports
	foundGW := false
	foundCanvas := false
	for _, p := range ports {
		if p.Port != nil {
			switch p.Port.IntValue() {
			case int(GatewayPort):
				foundGW = true
			case int(CanvasPort):
				foundCanvas = true
			case int(GatewayProxyPort):
				t.Errorf("NetworkPolicy should not allow proxy port %d when proxy is disabled", GatewayProxyPort)
			case int(CanvasProxyPort):
				t.Errorf("NetworkPolicy should not allow proxy port %d when proxy is disabled", CanvasProxyPort)
			}
		}
	}
	if !foundGW {
		t.Errorf("NetworkPolicy should allow port %d (direct gateway)", GatewayPort)
	}
	if !foundCanvas {
		t.Errorf("NetworkPolicy should allow port %d (direct canvas)", CanvasPort)
	}
}

func TestHasGatewayBindConflict(t *testing.T) {
	t.Run("no conflict when proxy enabled", func(t *testing.T) {
		instance := newTestInstance("gw-conflict-enabled")
		instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
			RawExtension: runtime.RawExtension{Raw: []byte(`{"gateway":{"bind":"loopback"}}`)},
		}
		if HasGatewayBindConflict(instance) {
			t.Error("should not report conflict when proxy is enabled")
		}
	})

	t.Run("conflict when proxy disabled and bind is loopback", func(t *testing.T) {
		instance := newTestInstance("gw-conflict-loopback")
		instance.Spec.Gateway.Enabled = Ptr(false)
		instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
			RawExtension: runtime.RawExtension{Raw: []byte(`{"gateway":{"bind":"loopback"}}`)},
		}
		if !HasGatewayBindConflict(instance) {
			t.Error("should report conflict when proxy is disabled and bind is loopback")
		}
	})

	t.Run("no conflict when proxy disabled and bind is not set", func(t *testing.T) {
		instance := newTestInstance("gw-conflict-default")
		instance.Spec.Gateway.Enabled = Ptr(false)
		instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
			RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
		}
		if HasGatewayBindConflict(instance) {
			t.Error("should not report conflict when bind is not set")
		}
	})

	t.Run("conflict when proxy disabled and bind is raw 127.0.0.1", func(t *testing.T) {
		instance := newTestInstance("gw-conflict-raw-lo")
		instance.Spec.Gateway.Enabled = Ptr(false)
		instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
			RawExtension: runtime.RawExtension{Raw: []byte(`{"gateway":{"bind":"127.0.0.1"}}`)},
		}
		if !HasGatewayBindConflict(instance) {
			t.Error("should report conflict when proxy is disabled and bind is 127.0.0.1")
		}
	})

	t.Run("no conflict when proxy disabled and bind is 0.0.0.0", func(t *testing.T) {
		instance := newTestInstance("gw-conflict-allif")
		instance.Spec.Gateway.Enabled = Ptr(false)
		instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
			RawExtension: runtime.RawExtension{Raw: []byte(`{"gateway":{"bind":"0.0.0.0"}}`)},
		}
		if HasGatewayBindConflict(instance) {
			t.Error("should not report conflict when bind is 0.0.0.0")
		}
	})
}

func TestHtpasswdEntry_Format(t *testing.T) {
	entry := HtpasswdEntry("admin", "secret")
	if !strings.HasPrefix(entry, "admin:{SHA}") {
		t.Errorf("htpasswd entry should start with 'admin:{SHA}', got %q", entry)
	}
}

func TestBuildBasicAuthSecret(t *testing.T) {
	instance := newTestInstance("ba-test")
	instance.Spec.Networking.Ingress.Security.BasicAuth = &openclawv1alpha1.IngressBasicAuthSpec{
		Username: "testuser",
	}
	secret := BuildBasicAuthSecret(instance, "mypassword")

	if secret.Name != BasicAuthSecretName(instance) {
		t.Errorf("secret name = %q, want %q", secret.Name, BasicAuthSecretName(instance))
	}
	auth, ok := secret.Data["auth"]
	if !ok {
		t.Fatal("secret missing 'auth' key")
	}
	if !strings.HasPrefix(string(auth), "testuser:{SHA}") {
		t.Errorf("auth value should start with 'testuser:{SHA}', got %q", string(auth))
	}

	// Verify plaintext username and password keys
	username, ok := secret.Data["username"]
	if !ok {
		t.Fatal("secret missing 'username' key")
	}
	if string(username) != "testuser" {
		t.Errorf("username = %q, want %q", string(username), "testuser")
	}
	pw, ok := secret.Data["password"]
	if !ok {
		t.Fatal("secret missing 'password' key")
	}
	if string(pw) != "mypassword" {
		t.Errorf("password = %q, want %q", string(pw), "mypassword")
	}
}

func TestBuildBasicAuthSecret_DefaultUsername(t *testing.T) {
	instance := newTestInstance("ba-default")
	instance.Spec.Networking.Ingress.Security.BasicAuth = &openclawv1alpha1.IngressBasicAuthSpec{}
	secret := BuildBasicAuthSecret(instance, "randompw")

	if string(secret.Data["username"]) != AppName {
		t.Errorf("username = %q, want default %q", string(secret.Data["username"]), AppName)
	}
	if string(secret.Data["password"]) != "randompw" {
		t.Errorf("password = %q, want %q", string(secret.Data["password"]), "randompw")
	}
}

func TestBuildIngress_BasicAuth_Nginx(t *testing.T) {
	enabled := true
	nginxClass := "nginx"
	instance := newTestInstance("ba-nginx")
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled:   true,
		ClassName: &nginxClass,
		Hosts:     []openclawv1alpha1.IngressHost{{Host: "test.example.com"}},
		Security: openclawv1alpha1.IngressSecuritySpec{
			ForceHTTPS: &enabled,
			EnableHSTS: Ptr(false),
			BasicAuth: &openclawv1alpha1.IngressBasicAuthSpec{
				Enabled:  &enabled,
				Username: "admin",
				Realm:    "My Realm",
			},
		},
	}

	ing := BuildIngress(instance)
	anns := ing.Annotations

	if anns["nginx.ingress.kubernetes.io/auth-type"] != "basic" {
		t.Errorf("auth-type annotation = %q, want %q", anns["nginx.ingress.kubernetes.io/auth-type"], "basic")
	}
	if anns["nginx.ingress.kubernetes.io/auth-secret"] != BasicAuthSecretName(instance) {
		t.Errorf("auth-secret annotation = %q, want %q", anns["nginx.ingress.kubernetes.io/auth-secret"], BasicAuthSecretName(instance))
	}
	if anns["nginx.ingress.kubernetes.io/auth-realm"] != "My Realm" {
		t.Errorf("auth-realm annotation = %q, want %q", anns["nginx.ingress.kubernetes.io/auth-realm"], "My Realm")
	}
}

func TestBuildIngress_BasicAuth_ExistingSecret(t *testing.T) {
	enabled := true
	nginxClass := "nginx"
	instance := newTestInstance("ba-existing")
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled:   true,
		ClassName: &nginxClass,
		Hosts:     []openclawv1alpha1.IngressHost{{Host: "test.example.com"}},
		Security: openclawv1alpha1.IngressSecuritySpec{
			BasicAuth: &openclawv1alpha1.IngressBasicAuthSpec{
				Enabled:        &enabled,
				ExistingSecret: "my-custom-auth",
			},
		},
	}

	ing := BuildIngress(instance)
	anns := ing.Annotations

	if anns["nginx.ingress.kubernetes.io/auth-secret"] != "my-custom-auth" {
		t.Errorf("auth-secret annotation = %q, want %q", anns["nginx.ingress.kubernetes.io/auth-secret"], "my-custom-auth")
	}
}

func TestBuildIngress_BasicAuth_Traefik(t *testing.T) {
	enabled := true
	traefikClass := "traefik"
	instance := newTestInstance("ba-traefik")
	instance.Namespace = "myns"
	instance.Spec.Networking.Ingress = openclawv1alpha1.IngressSpec{
		Enabled:   true,
		ClassName: &traefikClass,
		Hosts:     []openclawv1alpha1.IngressHost{{Host: "test.example.com"}},
		Security: openclawv1alpha1.IngressSecuritySpec{
			BasicAuth: &openclawv1alpha1.IngressBasicAuthSpec{
				Enabled: &enabled,
			},
		},
	}

	ing := BuildIngress(instance)
	anns := ing.Annotations

	expectedMiddleware := "myns-ba-traefik-basic-auth@kubernetescrd"
	if anns["traefik.ingress.kubernetes.io/router.middlewares"] != expectedMiddleware {
		t.Errorf("traefik middleware annotation = %q, want %q",
			anns["traefik.ingress.kubernetes.io/router.middlewares"], expectedMiddleware)
	}
	// Should not have nginx auth annotations
	if anns["nginx.ingress.kubernetes.io/auth-type"] != "" {
		t.Errorf("nginx auth-type should be empty for Traefik, got %q", anns["nginx.ingress.kubernetes.io/auth-type"])
	}
}

// ---------------------------------------------------------------------------
// Idempotency tests - verify all builders produce identical output on
// repeated calls with the same input. Essential for CreateOrUpdate.
// ---------------------------------------------------------------------------

func TestBuildService_Idempotent(t *testing.T) {
	instance := newTestInstance("idem-svc")
	s1 := BuildService(instance)
	s2 := BuildService(instance)
	b1, _ := json.Marshal(s1.Spec)
	b2, _ := json.Marshal(s2.Spec)
	if !bytes.Equal(b1, b2) {
		t.Error("BuildService is not idempotent")
	}
}

func TestBuildConfigMap_Idempotent(t *testing.T) {
	instance := newTestInstance("idem-cm")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{"key":"val"}`)},
	}
	c1 := BuildConfigMap(instance, "token123", nil)
	c2 := BuildConfigMap(instance, "token123", nil)
	b1, _ := json.Marshal(c1.Data)
	b2, _ := json.Marshal(c2.Data)
	if !bytes.Equal(b1, b2) {
		t.Error("BuildConfigMap is not idempotent")
	}
}

func TestBuildNetworkPolicy_Idempotent(t *testing.T) {
	instance := newTestInstance("idem-np")
	n1 := BuildNetworkPolicy(instance)
	n2 := BuildNetworkPolicy(instance)
	b1, _ := json.Marshal(n1.Spec)
	b2, _ := json.Marshal(n2.Spec)
	if !bytes.Equal(b1, b2) {
		t.Error("BuildNetworkPolicy is not idempotent")
	}
}

func TestBuildIngress_Idempotent(t *testing.T) {
	instance := newTestInstance("idem-ing")
	instance.Spec.Networking.Ingress.Enabled = true
	instance.Spec.Networking.Ingress.Hosts = []openclawv1alpha1.IngressHost{
		{Host: "test.example.com"},
	}
	i1 := BuildIngress(instance)
	i2 := BuildIngress(instance)
	b1, _ := json.Marshal(i1.Spec)
	b2, _ := json.Marshal(i2.Spec)
	if !bytes.Equal(b1, b2) {
		t.Error("BuildIngress is not idempotent")
	}
}

func TestBuildPDB_Idempotent(t *testing.T) {
	instance := newTestInstance("idem-pdb")
	p1 := BuildPDB(instance)
	p2 := BuildPDB(instance)
	b1, _ := json.Marshal(p1.Spec)
	b2, _ := json.Marshal(p2.Spec)
	if !bytes.Equal(b1, b2) {
		t.Error("BuildPDB is not idempotent")
	}
}

func TestBuildHPA_Idempotent(t *testing.T) {
	instance := newTestInstance("idem-hpa")
	instance.Spec.Availability.AutoScaling = &openclawv1alpha1.AutoScalingSpec{
		Enabled:              Ptr(true),
		MinReplicas:          Ptr(int32(1)),
		MaxReplicas:          Ptr(int32(5)),
		TargetCPUUtilization: Ptr(int32(80)),
	}
	h1 := BuildHPA(instance)
	h2 := BuildHPA(instance)
	b1, _ := json.Marshal(h1.Spec)
	b2, _ := json.Marshal(h2.Spec)
	if !bytes.Equal(b1, b2) {
		t.Error("BuildHPA is not idempotent")
	}
}

func TestBuildPVC_Idempotent(t *testing.T) {
	instance := newTestInstance("idem-pvc")
	p1 := BuildPVC(instance)
	p2 := BuildPVC(instance)
	b1, _ := json.Marshal(p1.Spec)
	b2, _ := json.Marshal(p2.Spec)
	if !bytes.Equal(b1, b2) {
		t.Error("BuildPVC is not idempotent")
	}
}

func TestBuildWorkspaceConfigMap_Idempotent(t *testing.T) {
	instance := newTestInstance("idem-ws")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		InitialFiles: map[string]string{
			"SOUL.md": "# Personality\nBe helpful.",
		},
	}
	w1 := BuildWorkspaceConfigMap(instance, nil, nil, nil)
	w2 := BuildWorkspaceConfigMap(instance, nil, nil, nil)
	b1, _ := json.Marshal(w1.Data)
	b2, _ := json.Marshal(w2.Data)
	if !bytes.Equal(b1, b2) {
		t.Error("BuildWorkspaceConfigMap is not idempotent")
	}
}

func TestBuildRBAC_Idempotent(t *testing.T) {
	instance := newTestInstance("idem-rbac")

	sa1 := BuildServiceAccount(instance)
	sa2 := BuildServiceAccount(instance)
	b1, _ := json.Marshal(sa1)
	b2, _ := json.Marshal(sa2)
	if !bytes.Equal(b1, b2) {
		t.Error("BuildServiceAccount is not idempotent")
	}

	r1 := BuildRole(instance)
	r2 := BuildRole(instance)
	b1, _ = json.Marshal(r1.Rules)
	b2, _ = json.Marshal(r2.Rules)
	if !bytes.Equal(b1, b2) {
		t.Error("BuildRole is not idempotent")
	}

	rb1 := BuildRoleBinding(instance)
	rb2 := BuildRoleBinding(instance)
	b1, _ = json.Marshal(rb1)
	b2, _ = json.Marshal(rb2)
	if !bytes.Equal(b1, b2) {
		t.Error("BuildRoleBinding is not idempotent")
	}
}

// ---------------------------------------------------------------------------
// Negative / edge case tests
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_NilAvailability(t *testing.T) {
	instance := newTestInstance("nil-avail")
	// Zero-value AvailabilitySpec - should not panic
	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	if sts == nil {
		t.Fatal("BuildStatefulSet returned nil for zero-value availability")
	}
	podSpec := sts.Spec.Template.Spec
	if podSpec.NodeSelector != nil {
		t.Error("expected nil NodeSelector")
	}
	if podSpec.Tolerations != nil {
		t.Error("expected nil Tolerations")
	}
	if podSpec.Affinity != nil {
		t.Error("expected nil Affinity")
	}
	if podSpec.TopologySpreadConstraints != nil {
		t.Error("expected nil TopologySpreadConstraints")
	}
	if podSpec.RuntimeClassName != nil {
		t.Error("expected nil RuntimeClassName")
	}
}

func TestBuildConfigMap_EmptyConfig(t *testing.T) {
	instance := newTestInstance("empty-cfg")
	// No raw config, no configMapRef
	cm := BuildConfigMap(instance, "", nil)
	if cm == nil {
		t.Fatal("BuildConfigMap returned nil for empty config")
	}
	if _, ok := cm.Data["openclaw.json"]; !ok {
		t.Error("ConfigMap should always have openclaw.json key")
	}
}

func TestBuildNetworkPolicy_Disabled(t *testing.T) {
	instance := newTestInstance("np-disabled")
	instance.Spec.Security.NetworkPolicy.Enabled = Ptr(false)
	np := BuildNetworkPolicy(instance)
	// BuildNetworkPolicy still returns a NetworkPolicy object - the controller
	// decides whether to create it based on the Enabled flag
	if np == nil {
		t.Fatal("BuildNetworkPolicy returned nil even with enabled=false")
	}
}

func TestBuildIngress_NoHosts_ReturnsEmpty(t *testing.T) {
	instance := newTestInstance("ing-no-hosts")
	instance.Spec.Networking.Ingress.Enabled = true
	// No hosts set
	ing := BuildIngress(instance)
	if ing == nil {
		t.Fatal("BuildIngress returned nil with no hosts")
	}
	if len(ing.Spec.Rules) != 0 {
		t.Errorf("expected 0 rules, got %d", len(ing.Spec.Rules))
	}
}

func TestNormalizeStatefulSet_DeprecatedServiceAccount(t *testing.T) {
	instance := newTestInstance("norm-test")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	NormalizeStatefulSet(sts)

	podSpec := sts.Spec.Template.Spec
	if podSpec.DeprecatedServiceAccount == "" {
		t.Error("NormalizeStatefulSet should set DeprecatedServiceAccount")
	}
	if podSpec.DeprecatedServiceAccount != podSpec.ServiceAccountName {
		t.Errorf("DeprecatedServiceAccount = %q, want %q (same as ServiceAccountName)",
			podSpec.DeprecatedServiceAccount, podSpec.ServiceAccountName)
	}
}

func TestNormalizeStatefulSet_FieldRefAPIVersion(t *testing.T) {
	instance := newTestInstance("norm-fieldref")
	instance.Spec.Chromium.Enabled = true
	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	NormalizeStatefulSet(sts)

	for _, c := range sts.Spec.Template.Spec.Containers {
		for _, e := range c.Env {
			if e.ValueFrom != nil && e.ValueFrom.FieldRef != nil {
				if e.ValueFrom.FieldRef.APIVersion != "v1" {
					t.Errorf("container %q env %q: FieldRef.APIVersion = %q, want %q",
						c.Name, e.Name, e.ValueFrom.FieldRef.APIVersion, "v1")
				}
			}
		}
	}
}

func TestNormalizeStatefulSet_Idempotent(t *testing.T) {
	// Verifies that normalizing twice produces the same result (stability).
	instance := newTestInstance("norm-idem")
	instance.Spec.Chromium.Enabled = true
	sts := BuildStatefulSet(instance, "gw-secret", nil, nil, nil)
	NormalizeStatefulSet(sts)

	// JSON-serialize as first snapshot
	snap1, _ := json.Marshal(sts.Spec)

	// Normalize again
	NormalizeStatefulSet(sts)
	snap2, _ := json.Marshal(sts.Spec)

	if !bytes.Equal(snap1, snap2) {
		t.Error("NormalizeStatefulSet is not idempotent: second normalize changed the spec")
	}
}

func TestNormalizeStatefulSet_NoSpuriousDiff(t *testing.T) {
	// Simulates the CreateOrUpdate round-trip: build desired, normalize,
	// then build again and normalize. The two should be equal via
	// equality.Semantic.DeepEqual (same comparison controller-runtime uses).
	instance := newTestInstance("norm-roundtrip")
	instance.Spec.Chromium.Enabled = true

	// First build (simulates "existing" after initial create + K8s defaulting)
	sts1 := BuildStatefulSet(instance, "gw-secret", nil, nil, nil)
	NormalizeStatefulSet(sts1)

	// Simulate K8s round-trip: JSON marshal/unmarshal (like reading from API)
	data, err := json.Marshal(sts1)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	existing := &appsv1.StatefulSet{}
	if err := json.Unmarshal(data, existing); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Second build (simulates next reconcile's "desired")
	sts2 := BuildStatefulSet(instance, "gw-secret", nil, nil, nil)
	NormalizeStatefulSet(sts2)

	// Apply mutation like the controller does: replace spec
	mutated := existing.DeepCopy()
	mutated.Labels = sts2.Labels
	mutated.Spec = sts2.Spec

	if !equality.Semantic.DeepEqual(existing, mutated) {
		// Find the difference for a useful error message
		d1, _ := json.MarshalIndent(existing.Spec, "", "  ")
		d2, _ := json.MarshalIndent(mutated.Spec, "", "  ")
		t.Errorf("spurious diff detected between reconcile cycles.\n"+
			"This would cause a reconciliation loop in production.\n"+
			"Existing spec (from K8s):\n%s\n\nDesired spec (from builder):\n%s",
			string(d1), string(d2))
	}
}

func TestNormalizeStatefulSet_UpdateStrategyRollingUpdate(t *testing.T) {
	// Regression test: K8s API server defaults UpdateStrategy.RollingUpdate to &{}
	// when Type == RollingUpdate and RollingUpdate == nil. Without normalizing this,
	// CreateOrUpdate detects a diff on every reconcile (nil vs &{}), issues an
	// unnecessary Update, and triggers a continuous reconcile loop via the StatefulSet watch.
	instance := newTestInstance("norm-update-strategy")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	if sts.Spec.UpdateStrategy.Type != appsv1.RollingUpdateStatefulSetStrategyType {
		t.Fatal("expected RollingUpdate strategy type from builder")
	}

	NormalizeStatefulSet(sts)

	if sts.Spec.UpdateStrategy.RollingUpdate == nil {
		t.Error("NormalizeStatefulSet should set UpdateStrategy.RollingUpdate to &{} to match K8s API server defaulting")
	}
}

func TestNormalizeStatefulSet_ProbeDefaults(t *testing.T) {
	instance := newTestInstance("norm-probe")
	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	NormalizeStatefulSet(sts)

	main := sts.Spec.Template.Spec.Containers[0]
	if main.StartupProbe == nil {
		t.Fatal("expected startup probe to be set")
	}
	if main.StartupProbe.TimeoutSeconds == 0 {
		t.Error("startup probe TimeoutSeconds should not be 0 after normalization")
	}
	if main.StartupProbe.PeriodSeconds == 0 {
		t.Error("startup probe PeriodSeconds should not be 0 after normalization")
	}
	if main.StartupProbe.HTTPGet != nil && main.StartupProbe.HTTPGet.Scheme == "" {
		t.Error("startup probe HTTPGet.Scheme should not be empty after normalization")
	}
}

func TestBuildWorkspaceConfigMap_NilWorkspace(t *testing.T) {
	instance := newTestInstance("ws-nil")
	instance.Spec.Workspace = nil
	cm := BuildWorkspaceConfigMap(instance, nil, nil, nil)
	// Operator files are always injected, so the ConfigMap is never nil
	if cm == nil {
		t.Fatal("expected non-nil ConfigMap (operator files are always injected)")
	}
	for _, f := range []string{"ENVIRONMENT.md", "BOOTSTRAP.md"} {
		if _, ok := cm.Data[f]; !ok {
			t.Errorf("expected %s in workspace ConfigMap", f)
		}
	}
	if len(cm.Data) != 2 {
		t.Errorf("expected exactly 2 files (ENVIRONMENT.md + BOOTSTRAP.md), got %d", len(cm.Data))
	}
}

// ---------------------------------------------------------------------------
// Container security context - runAsNonRoot / runAsUser propagation (#263)
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_RunAsNonRoot_DefaultBehavior(t *testing.T) {
	// When no security context overrides are set, all containers should
	// default to runAsNonRoot: true.
	instance := newTestInstance("default-sec")
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.AuthKeySecretRef = &corev1.LocalObjectReference{Name: "ts-secret"}
	instance.Spec.WebTerminal.Enabled = true
	instance.Spec.Skills = []string{"@test/skill"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	containers := sts.Spec.Template.Spec.Containers
	initContainers := sts.Spec.Template.Spec.InitContainers

	// Main container
	main := containers[0]
	if main.SecurityContext.RunAsNonRoot == nil || !*main.SecurityContext.RunAsNonRoot {
		t.Error("main container: runAsNonRoot should default to true")
	}

	// Check all init containers
	for _, ic := range initContainers {
		if ic.SecurityContext == nil || ic.SecurityContext.RunAsNonRoot == nil {
			t.Errorf("init container %q: security context or runAsNonRoot is nil", ic.Name)
			continue
		}
		// Ollama init runs as root intentionally
		if ic.Name == "init-ollama" {
			continue
		}
		if !*ic.SecurityContext.RunAsNonRoot {
			t.Errorf("init container %q: runAsNonRoot should default to true", ic.Name)
		}
	}

	// Tailscale sidecar
	for _, c := range containers {
		if c.Name == "tailscale" {
			if c.SecurityContext.RunAsNonRoot == nil || !*c.SecurityContext.RunAsNonRoot {
				t.Error("tailscale sidecar: runAsNonRoot should default to true")
			}
		}
		if c.Name == "web-terminal" {
			if c.SecurityContext.RunAsNonRoot == nil || !*c.SecurityContext.RunAsNonRoot {
				t.Error("web-terminal sidecar: runAsNonRoot should default to true")
			}
		}
	}
}

func TestBuildStatefulSet_PodLevelRunAsNonRootFalse_Propagation(t *testing.T) {
	// When podSecurityContext.runAsNonRoot is explicitly set to false,
	// it should propagate to the main container, init containers, and
	// applicable sidecars.
	instance := newTestInstance("pod-nonroot-false")
	instance.Spec.Security.PodSecurityContext = &openclawv1alpha1.PodSecurityContextSpec{
		RunAsNonRoot: Ptr(false),
	}
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.AuthKeySecretRef = &corev1.LocalObjectReference{Name: "ts-secret"}
	instance.Spec.WebTerminal.Enabled = true
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Skills = []string{"@test/skill"}
	instance.Spec.RuntimeDeps.Pnpm = true
	instance.Spec.RuntimeDeps.Python = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	containers := sts.Spec.Template.Spec.Containers
	initContainers := sts.Spec.Template.Spec.InitContainers

	// Main container should inherit pod-level runAsNonRoot: false
	main := containers[0]
	if main.SecurityContext.RunAsNonRoot == nil || *main.SecurityContext.RunAsNonRoot {
		t.Error("main container: runAsNonRoot should be false (inherited from pod)")
	}

	// Init containers that should inherit pod-level runAsNonRoot: false
	wantFalse := map[string]bool{
		"init-config":        true,
		"init-skills":        true,
		"init-pnpm":          true,
		"init-python":        true,
		"init-tailscale-bin": true,
	}
	for _, ic := range initContainers {
		if !wantFalse[ic.Name] {
			continue
		}
		if ic.SecurityContext == nil || ic.SecurityContext.RunAsNonRoot == nil {
			t.Errorf("init container %q: security context or runAsNonRoot is nil", ic.Name)
			continue
		}
		if *ic.SecurityContext.RunAsNonRoot {
			t.Errorf("init container %q: runAsNonRoot should be false (inherited from pod)", ic.Name)
		}
	}

	// Tailscale sidecar should inherit pod-level runAsNonRoot: false
	for _, c := range containers {
		if c.Name == "tailscale" {
			if c.SecurityContext.RunAsNonRoot == nil || *c.SecurityContext.RunAsNonRoot {
				t.Error("tailscale sidecar: runAsNonRoot should be false (inherited from pod)")
			}
		}
		// Web terminal should also inherit pod-level runAsNonRoot: false
		if c.Name == "web-terminal" {
			if c.SecurityContext.RunAsNonRoot == nil || *c.SecurityContext.RunAsNonRoot {
				t.Error("web-terminal sidecar: runAsNonRoot should be false (inherited from pod)")
			}
		}
	}

	// Gateway proxy should STILL have runAsNonRoot: true (has its own RunAsUser: 101)
	for _, c := range containers {
		if c.Name == "gateway-proxy" {
			if c.SecurityContext.RunAsNonRoot == nil || !*c.SecurityContext.RunAsNonRoot {
				t.Error("gateway-proxy: runAsNonRoot should still be true (self-consistent with RunAsUser: 101)")
			}
			if c.SecurityContext.RunAsUser == nil || *c.SecurityContext.RunAsUser != 101 {
				t.Error("gateway-proxy: runAsUser should be 101")
			}
		}
	}

	// Chromium should STILL have runAsNonRoot: true (has its own RunAsUser: 999)
	// Chromium is now a native sidecar in InitContainers
	for _, c := range initContainers {
		if c.Name == "chromium" {
			if c.SecurityContext.RunAsNonRoot == nil || !*c.SecurityContext.RunAsNonRoot {
				t.Error("chromium: runAsNonRoot should still be true (self-consistent with RunAsUser: 65534)")
			}
			if c.SecurityContext.RunAsUser == nil || *c.SecurityContext.RunAsUser != 65534 {
				t.Error("chromium: runAsUser should be 65534")
			}
		}
	}
}

func TestBuildStatefulSet_ContainerLevelRunAsNonRootOverride(t *testing.T) {
	// When containerSecurityContext.runAsNonRoot is explicitly set to false
	// but pod-level is default (true), only the main container should change.
	instance := newTestInstance("container-nonroot-false")
	instance.Spec.Security.ContainerSecurityContext = &openclawv1alpha1.ContainerSecurityContextSpec{
		RunAsNonRoot: Ptr(false),
	}
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.AuthKeySecretRef = &corev1.LocalObjectReference{Name: "ts-secret"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	containers := sts.Spec.Template.Spec.Containers
	initContainers := sts.Spec.Template.Spec.InitContainers

	// Main container should have runAsNonRoot: false (from container-level override)
	main := containers[0]
	if main.SecurityContext.RunAsNonRoot == nil || *main.SecurityContext.RunAsNonRoot {
		t.Error("main container: runAsNonRoot should be false (container-level override)")
	}

	// Init containers should still be true (they use pod-level, not container-level)
	for _, ic := range initContainers {
		if ic.SecurityContext == nil || ic.SecurityContext.RunAsNonRoot == nil {
			continue
		}
		// Skip ollama init (runs as root)
		if ic.Name == "init-ollama" {
			continue
		}
		if !*ic.SecurityContext.RunAsNonRoot {
			t.Errorf("init container %q: runAsNonRoot should still be true (pod-level default)", ic.Name)
		}
	}

	// Tailscale sidecar should still be true (uses pod-level)
	for _, c := range containers {
		if c.Name == "tailscale" {
			if c.SecurityContext.RunAsNonRoot == nil || !*c.SecurityContext.RunAsNonRoot {
				t.Error("tailscale sidecar: runAsNonRoot should still be true (pod-level default)")
			}
		}
	}
}

func TestBuildStatefulSet_ContainerLevelRunAsUser(t *testing.T) {
	// When containerSecurityContext.runAsUser is set, it should appear on
	// the main container only.
	instance := newTestInstance("container-runasuser")
	instance.Spec.Security.ContainerSecurityContext = &openclawv1alpha1.ContainerSecurityContextSpec{
		RunAsUser: Ptr(int64(2000)),
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	main := sts.Spec.Template.Spec.Containers[0]

	if main.SecurityContext.RunAsUser == nil || *main.SecurityContext.RunAsUser != 2000 {
		t.Errorf("main container: runAsUser = %v, want 2000", main.SecurityContext.RunAsUser)
	}
	// runAsNonRoot should still be true (default)
	if main.SecurityContext.RunAsNonRoot == nil || !*main.SecurityContext.RunAsNonRoot {
		t.Error("main container: runAsNonRoot should still be true (default)")
	}
}

func TestBuildStatefulSet_FullNonRootFalseScenario(t *testing.T) {
	// Both pod-level runAsNonRoot: false and container-level runAsNonRoot: false.
	// Verify no contradictions exist in any container.
	instance := newTestInstance("full-nonroot-false")
	instance.Spec.Security.PodSecurityContext = &openclawv1alpha1.PodSecurityContextSpec{
		RunAsNonRoot: Ptr(false),
		RunAsUser:    Ptr(int64(1000)), // non-zero to keep it valid
	}
	instance.Spec.Security.ContainerSecurityContext = &openclawv1alpha1.ContainerSecurityContextSpec{
		RunAsNonRoot: Ptr(false),
		RunAsUser:    Ptr(int64(1000)),
	}
	instance.Spec.Tailscale.Enabled = true
	instance.Spec.Tailscale.AuthKeySecretRef = &corev1.LocalObjectReference{Name: "ts-secret"}
	instance.Spec.WebTerminal.Enabled = true
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Ollama.Enabled = true
	instance.Spec.Skills = []string{"@test/skill"}
	instance.Spec.RuntimeDeps.Pnpm = true
	instance.Spec.RuntimeDeps.Python = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	var allContainers []corev1.Container
	allContainers = append(allContainers, sts.Spec.Template.Spec.InitContainers...)
	allContainers = append(allContainers, sts.Spec.Template.Spec.Containers...)

	for _, c := range allContainers {
		if c.SecurityContext == nil {
			continue
		}
		sc := c.SecurityContext

		// Self-consistent containers (have their own RunAsUser) should not
		// contradict their own RunAsNonRoot
		if sc.RunAsNonRoot != nil && sc.RunAsUser != nil {
			nonRoot := *sc.RunAsNonRoot
			uid := *sc.RunAsUser
			if nonRoot && uid == 0 {
				t.Errorf("container %q: runAsNonRoot=true contradicts runAsUser=0", c.Name)
			}
		}
	}

	// Main container should have runAsNonRoot: false and runAsUser: 1000
	main := sts.Spec.Template.Spec.Containers[0]
	if main.SecurityContext.RunAsNonRoot == nil || *main.SecurityContext.RunAsNonRoot {
		t.Error("main container: runAsNonRoot should be false")
	}
	if main.SecurityContext.RunAsUser == nil || *main.SecurityContext.RunAsUser != 1000 {
		t.Errorf("main container: runAsUser should be 1000, got %v", main.SecurityContext.RunAsUser)
	}

	// Verify init containers with pod-level inheritance
	for _, ic := range sts.Spec.Template.Spec.InitContainers {
		switch ic.Name {
		case "init-config", "init-skills", "init-pnpm", "init-python", "init-tailscale-bin":
			if ic.SecurityContext.RunAsNonRoot == nil || *ic.SecurityContext.RunAsNonRoot {
				t.Errorf("init container %q: runAsNonRoot should be false", ic.Name)
			}
		case "init-ollama":
			// Ollama init always runs as root - should be unchanged
			if ic.SecurityContext.RunAsNonRoot == nil || *ic.SecurityContext.RunAsNonRoot {
				t.Errorf("init-ollama: runAsNonRoot should be false (always root)")
			}
			if ic.SecurityContext.RunAsUser == nil || *ic.SecurityContext.RunAsUser != 0 {
				t.Error("init-ollama: runAsUser should be 0")
			}
		}
	}

	// Verify self-consistent sidecars are unchanged
	for _, c := range sts.Spec.Template.Spec.Containers {
		switch c.Name {
		case "gateway-proxy":
			if !*c.SecurityContext.RunAsNonRoot {
				t.Error("gateway-proxy: runAsNonRoot should be true (self-consistent)")
			}
			if *c.SecurityContext.RunAsUser != 101 {
				t.Error("gateway-proxy: runAsUser should be 101")
			}
		case "ollama":
			if *c.SecurityContext.RunAsNonRoot {
				t.Error("ollama: runAsNonRoot should be false (always root)")
			}
			if *c.SecurityContext.RunAsUser != 0 {
				t.Error("ollama: runAsUser should be 0")
			}
		}
	}

	// Chromium is a native sidecar in InitContainers
	for _, c := range sts.Spec.Template.Spec.InitContainers {
		if c.Name == "chromium" {
			if !*c.SecurityContext.RunAsNonRoot {
				t.Error("chromium: runAsNonRoot should be true (self-consistent)")
			}
			if *c.SecurityContext.RunAsUser != 65534 {
				t.Error("chromium: runAsUser should be 65534")
			}
		}
	}
}

func TestPodRunAsNonRoot_Helper(t *testing.T) {
	tests := []struct {
		name     string
		psc      *openclawv1alpha1.PodSecurityContextSpec
		expected bool
	}{
		{
			name:     "nil pod security context defaults to true",
			psc:      nil,
			expected: true,
		},
		{
			name:     "empty pod security context defaults to true",
			psc:      &openclawv1alpha1.PodSecurityContextSpec{},
			expected: true,
		},
		{
			name: "explicit true",
			psc: &openclawv1alpha1.PodSecurityContextSpec{
				RunAsNonRoot: Ptr(true),
			},
			expected: true,
		},
		{
			name: "explicit false",
			psc: &openclawv1alpha1.PodSecurityContextSpec{
				RunAsNonRoot: Ptr(false),
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			instance := newTestInstance("helper-test")
			instance.Spec.Security.PodSecurityContext = tt.psc
			got := podRunAsNonRoot(instance)
			if got != tt.expected {
				t.Errorf("podRunAsNonRoot() = %v, want %v", got, tt.expected)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Chromium direct Chrome tests (no browserless proxy)
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_NoChromiumProxy(t *testing.T) {
	instance := newTestInstance("no-proxy")
	instance.Spec.Chromium.Enabled = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	for _, c := range sts.Spec.Template.Spec.InitContainers {
		if c.Name == "chromium-proxy" {
			t.Error("chromium-proxy should not exist - Chrome runs via run.sh")
		}
	}
}

// TestBuildStatefulSet_ChromiumEntrypointCommand ensures the default chromium
// image gets a Command override that fixes unquoted $@ in upstream run.sh.
// Without this, args containing spaces (e.g. --user-agent=...) get word-split,
// causing Chrome's "Multiple targets are not supported" error. See #396.
func TestBuildStatefulSet_ChromiumEntrypointCommand(t *testing.T) {
	instance := newTestInstance("entrypoint-cmd")
	instance.Spec.Chromium.Enabled = true

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	for _, c := range sts.Spec.Template.Spec.InitContainers {
		if c.Name != "chromium" {
			continue
		}
		if len(c.Command) != len(ChromiumEntrypointCommand) {
			t.Fatalf("chromium Command length = %d, want %d", len(c.Command), len(ChromiumEntrypointCommand))
		}
		for i, arg := range c.Command {
			if arg != ChromiumEntrypointCommand[i] {
				t.Errorf("chromium Command[%d] = %q, want %q", i, arg, ChromiumEntrypointCommand[i])
			}
		}
		// Verify the script uses quoted "$@"
		script := c.Command[2]
		if !strings.Contains(script, `"$@"`) {
			t.Error("entrypoint script must use quoted \"$@\" to prevent word-splitting")
		}
		return
	}
	t.Fatal("chromium init container not found")
}

// TestBuildStatefulSet_ChromiumCustomImageNoCommand ensures custom chromium
// images do NOT get a Command override - they keep their own entrypoint.
func TestBuildStatefulSet_ChromiumCustomImageNoCommand(t *testing.T) {
	instance := newTestInstance("custom-img-no-cmd")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Chromium.Image.Repository = "my-registry/my-chrome"
	instance.Spec.Chromium.Image.Tag = "v1"

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	for _, c := range sts.Spec.Template.Spec.InitContainers {
		if c.Name == "chromium" {
			if len(c.Command) != 0 {
				t.Errorf("custom image should have nil Command (keep own entrypoint), got %v", c.Command)
			}
			return
		}
	}
	t.Fatal("chromium init container not found")
}

// TestBuildStatefulSet_ChromiumMigratesDeprecatedImage verifies that instances
// created before v0.22.1 with the old ghcr.io/browserless/chromium kubebuilder
// default get normalized to the current default image. Without this migration,
// the old image either fails to pull (doesn't exist) or crashes because its
// entrypoint is incompatible with Chrome launch flags passed as Args. See #396.
func TestBuildStatefulSet_ChromiumMigratesDeprecatedImage(t *testing.T) {
	instance := newTestInstance("migrate-img")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Chromium.Image.Repository = DeprecatedChromiumImage
	instance.Spec.Chromium.Image.Tag = "latest" // old kubebuilder default tag

	sts := BuildStatefulSet(instance, "", nil, nil, nil)
	for _, c := range sts.Spec.Template.Spec.InitContainers {
		if c.Name == "chromium" {
			expectedImage := DefaultChromiumImage + ":" + DefaultChromiumTag
			if c.Image != expectedImage {
				t.Errorf("chromium image = %q, want %q (should migrate deprecated image)", c.Image, expectedImage)
			}
			// After migration to default image, should get the entrypoint wrapper
			if len(c.Command) != len(ChromiumEntrypointCommand) {
				t.Errorf("chromium Command after migration = %v, want ChromiumEntrypointCommand", c.Command)
			}
			return
		}
	}
	t.Fatal("chromium init container not found")
}

func TestChromiumArgs_Deduplication(t *testing.T) {
	instance := newTestInstance("cdp-dedup")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Chromium.ExtraArgs = []string{
		"--no-first-run",
		"--window-size=1920,1080",
	}

	args := ChromiumArgs(instance)
	argsStr := strings.Join(args, " ")

	// --no-first-run should appear only once (deduplicated)
	count := strings.Count(argsStr, "--no-first-run")
	if count != 1 {
		t.Errorf("--no-first-run should appear once (deduplicated), found %d times", count)
	}

	// --window-size should be present
	if !strings.Contains(argsStr, "--window-size=1920,1080") {
		t.Error("args should contain --window-size from ExtraArgs")
	}

	// Default args should be present
	if !strings.Contains(argsStr, "--no-sandbox") {
		t.Error("args should contain --no-sandbox")
	}
}

func TestChromiumArgs_PersistenceAddsUserDataDir(t *testing.T) {
	instance := newTestInstance("cdp-persist")
	instance.Spec.Chromium.Enabled = true
	instance.Spec.Chromium.Persistence.Enabled = true

	args := ChromiumArgs(instance)
	argsStr := strings.Join(args, " ")

	if !strings.Contains(argsStr, "--user-data-dir=/chromium-data") {
		t.Error("persistence should add --user-data-dir=/chromium-data")
	}
}

// TestChromiumArgs_ExtraArgsWithSpacesPreserved verifies that ExtraArgs
// containing spaces (e.g. --user-agent=...) are kept as single elements
// in the args slice. Combined with ChromiumEntrypointCommand's quoted "$@",
// this prevents Chrome's "Multiple targets are not supported" error. See #396.
func TestChromiumArgs_ExtraArgsWithSpacesPreserved(t *testing.T) {
	instance := newTestInstance("cdp-spaces")
	instance.Spec.Chromium.Enabled = true
	userAgent := "--user-agent=Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	instance.Spec.Chromium.ExtraArgs = []string{userAgent}

	args := ChromiumArgs(instance)

	// The user-agent must remain a single element, not split on spaces
	found := false
	for _, arg := range args {
		if arg == userAgent {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("user-agent arg with spaces should be preserved as single element, got args: %v", args)
	}
}

// --- Gateway proxy disabled tests ---

func TestBuildStatefulSet_GatewayProxyDisabled_NoProxyContainer(t *testing.T) {
	instance := newTestInstance("gw-disabled")
	instance.Spec.Gateway.Enabled = Ptr(false)
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "gateway-proxy" {
			t.Fatal("gateway-proxy container should not be present when disabled")
		}
	}
}

func TestBuildStatefulSet_GatewayProxyDisabled_NoProxyTmpVolume(t *testing.T) {
	instance := newTestInstance("gw-disabled-vol")
	instance.Spec.Gateway.Enabled = Ptr(false)
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	for _, v := range sts.Spec.Template.Spec.Volumes {
		if v.Name == "gateway-proxy-tmp" {
			t.Fatal("gateway-proxy-tmp volume should not be present when proxy is disabled")
		}
	}
}

func TestBuildStatefulSet_GatewayProxyDisabled_ProbesTargetGatewayPort(t *testing.T) {
	instance := newTestInstance("gw-disabled-probes")
	instance.Spec.Gateway.Enabled = Ptr(false)
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var main *corev1.Container
	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == "openclaw" {
			main = &sts.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if main == nil {
		t.Fatal("openclaw container not found")
	}

	if main.LivenessProbe == nil || main.LivenessProbe.HTTPGet == nil {
		t.Fatal("liveness probe should be configured")
	}
	if main.LivenessProbe.HTTPGet.Port.IntValue() != int(GatewayPort) {
		t.Errorf("liveness probe port = %d, want %d (direct gateway port)", main.LivenessProbe.HTTPGet.Port.IntValue(), GatewayPort)
	}

	if main.ReadinessProbe == nil || main.ReadinessProbe.HTTPGet == nil {
		t.Fatal("readiness probe should be configured")
	}
	if main.ReadinessProbe.HTTPGet.Port.IntValue() != int(GatewayPort) {
		t.Errorf("readiness probe port = %d, want %d (direct gateway port)", main.ReadinessProbe.HTTPGet.Port.IntValue(), GatewayPort)
	}

	if main.StartupProbe == nil || main.StartupProbe.HTTPGet == nil {
		t.Fatal("startup probe should be configured")
	}
	if main.StartupProbe.HTTPGet.Port.IntValue() != int(GatewayPort) {
		t.Errorf("startup probe port = %d, want %d (direct gateway port)", main.StartupProbe.HTTPGet.Port.IntValue(), GatewayPort)
	}
}

func TestBuildStatefulSet_GatewayProxyEnabled_ProbesTargetProxyPort(t *testing.T) {
	instance := newTestInstance("gw-enabled-probes")
	// Default (nil) means enabled
	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var main *corev1.Container
	for i := range sts.Spec.Template.Spec.Containers {
		if sts.Spec.Template.Spec.Containers[i].Name == "openclaw" {
			main = &sts.Spec.Template.Spec.Containers[i]
			break
		}
	}
	if main == nil {
		t.Fatal("openclaw container not found")
	}

	if main.LivenessProbe == nil || main.LivenessProbe.HTTPGet == nil {
		t.Fatal("liveness probe should be configured")
	}
	if main.LivenessProbe.HTTPGet.Port.IntValue() != int(GatewayProxyPort) {
		t.Errorf("liveness probe port = %d, want %d (proxy port)", main.LivenessProbe.HTTPGet.Port.IntValue(), GatewayProxyPort)
	}
}

func TestBuildService_GatewayProxyDisabled_TargetsDirectPorts(t *testing.T) {
	instance := newTestInstance("svc-no-proxy")
	instance.Spec.Gateway.Enabled = Ptr(false)
	svc := BuildService(instance)

	for _, port := range svc.Spec.Ports {
		switch port.Name {
		case "gateway":
			if port.TargetPort.IntValue() != int(GatewayPort) {
				t.Errorf("gateway targetPort = %d, want %d (direct port)", port.TargetPort.IntValue(), GatewayPort)
			}
		case "canvas":
			if port.TargetPort.IntValue() != int(CanvasPort) {
				t.Errorf("canvas targetPort = %d, want %d (direct port)", port.TargetPort.IntValue(), CanvasPort)
			}
		}
	}
}

func TestBuildConfigMap_GatewayProxyDisabled_NoNginxConfig(t *testing.T) {
	instance := newTestInstance("cm-no-proxy")
	instance.Spec.Gateway.Enabled = Ptr(false)
	cm := BuildConfigMap(instance, "", nil)

	if _, ok := cm.Data[NginxConfigKey]; ok {
		t.Error("ConfigMap should not contain nginx config when gateway proxy is disabled")
	}
}

func TestBuildConfigMap_GatewayProxyDisabled_BindsAllInterfaces(t *testing.T) {
	instance := newTestInstance("cm-no-proxy-bind")
	instance.Spec.Gateway.Enabled = Ptr(false)
	cm := BuildConfigMap(instance, "", nil)

	content := cm.Data["openclaw.json"]
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		t.Fatalf("failed to parse config JSON: %v", err)
	}

	gw, ok := parsed["gateway"].(map[string]interface{})
	if !ok {
		t.Fatal("gateway section not found in config")
	}
	if gw["bind"] != GatewayBindAllInterfaces {
		t.Errorf("gateway.bind = %v, want %q (direct access when proxy disabled)", gw["bind"], GatewayBindAllInterfaces)
	}
}

func TestBuildChromiumCDPService_TargetsProxyPort(t *testing.T) {
	instance := newTestInstance("cdp-svc")
	instance.Spec.Chromium.Enabled = true

	svc := BuildChromiumCDPService(instance)

	if len(svc.Spec.Ports) != 1 {
		t.Fatalf("expected 1 port, got %d", len(svc.Spec.Ports))
	}

	port := svc.Spec.Ports[0]
	if port.Port != int32(ChromiumPort) {
		t.Errorf("service port should be %d (proxy owns this port directly), got %d", ChromiumPort, port.Port)
	}
	if port.TargetPort.IntValue() != int(ChromiumPort) {
		t.Errorf("target port should be %d (proxy), got %d", ChromiumPort, port.TargetPort.IntValue())
	}
}

// ---------------------------------------------------------------------------
// Per-replica PVC (VolumeClaimTemplate) tests
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_VCT_PersistenceAndHPA(t *testing.T) {
	instance := newTestInstance("vct-hpa")
	instance.Spec.Availability.AutoScaling = &openclawv1alpha1.AutoScalingSpec{
		Enabled: Ptr(true),
	}
	instance.Spec.Storage.Persistence.Size = "20Gi"

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// VolumeClaimTemplates should be set
	if len(sts.Spec.VolumeClaimTemplates) != 1 {
		t.Fatalf("VolumeClaimTemplates length = %d, want 1", len(sts.Spec.VolumeClaimTemplates))
	}

	vct := sts.Spec.VolumeClaimTemplates[0]
	if vct.Name != "data" {
		t.Errorf("VCT name = %q, want %q", vct.Name, "data")
	}

	// Access modes default to RWO
	if len(vct.Spec.AccessModes) != 1 || vct.Spec.AccessModes[0] != corev1.ReadWriteOnce {
		t.Errorf("VCT access modes = %v, want [ReadWriteOnce]", vct.Spec.AccessModes)
	}

	// Size should match
	size := vct.Spec.Resources.Requests[corev1.ResourceStorage]
	if size.String() != "20Gi" {
		t.Errorf("VCT storage size = %q, want %q", size.String(), "20Gi")
	}

	// No static "data" volume in pod spec
	dataVol := findVolume(sts.Spec.Template.Spec.Volumes, "data")
	if dataVol != nil {
		t.Error("static data volume should not be present when VCTs are used")
	}
}

func TestBuildStatefulSet_VCT_CustomAccessModes(t *testing.T) {
	instance := newTestInstance("vct-rwx")
	instance.Spec.Availability.AutoScaling = &openclawv1alpha1.AutoScalingSpec{
		Enabled: Ptr(true),
	}
	instance.Spec.Storage.Persistence.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	if len(sts.Spec.VolumeClaimTemplates) != 1 {
		t.Fatalf("VolumeClaimTemplates length = %d, want 1", len(sts.Spec.VolumeClaimTemplates))
	}
	if sts.Spec.VolumeClaimTemplates[0].Spec.AccessModes[0] != corev1.ReadWriteMany {
		t.Errorf("VCT access mode = %v, want ReadWriteMany", sts.Spec.VolumeClaimTemplates[0].Spec.AccessModes)
	}
}

func TestBuildStatefulSet_VCT_StorageClass(t *testing.T) {
	instance := newTestInstance("vct-sc")
	instance.Spec.Availability.AutoScaling = &openclawv1alpha1.AutoScalingSpec{
		Enabled: Ptr(true),
	}
	sc := "fast-ssd"
	instance.Spec.Storage.Persistence.StorageClass = &sc

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	if len(sts.Spec.VolumeClaimTemplates) != 1 {
		t.Fatalf("VolumeClaimTemplates length = %d, want 1", len(sts.Spec.VolumeClaimTemplates))
	}
	vct := sts.Spec.VolumeClaimTemplates[0]
	if vct.Spec.StorageClassName == nil || *vct.Spec.StorageClassName != "fast-ssd" {
		t.Errorf("VCT storage class = %v, want %q", vct.Spec.StorageClassName, "fast-ssd")
	}
}

func TestBuildStatefulSet_VCT_NoStorageClass(t *testing.T) {
	instance := newTestInstance("vct-no-sc")
	instance.Spec.Availability.AutoScaling = &openclawv1alpha1.AutoScalingSpec{
		Enabled: Ptr(true),
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	if len(sts.Spec.VolumeClaimTemplates) != 1 {
		t.Fatalf("VolumeClaimTemplates length = %d, want 1", len(sts.Spec.VolumeClaimTemplates))
	}
	if sts.Spec.VolumeClaimTemplates[0].Spec.StorageClassName != nil {
		t.Errorf("VCT storage class should be nil, got %q", *sts.Spec.VolumeClaimTemplates[0].Spec.StorageClassName)
	}
}

func TestBuildStatefulSet_NoVCT_HPADisabled(t *testing.T) {
	instance := newTestInstance("no-vct")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// No VCTs when HPA is disabled
	if len(sts.Spec.VolumeClaimTemplates) != 0 {
		t.Errorf("VolumeClaimTemplates should be empty when HPA is disabled, got %d", len(sts.Spec.VolumeClaimTemplates))
	}

	// Static PVC volume should be present
	dataVol := findVolume(sts.Spec.Template.Spec.Volumes, "data")
	if dataVol == nil {
		t.Fatal("data volume not found")
	}
	if dataVol.PersistentVolumeClaim == nil {
		t.Error("data volume should use static PVC when HPA is disabled")
	}
}

func TestBuildStatefulSet_NoVCT_PersistenceDisabledWithHPA(t *testing.T) {
	instance := newTestInstance("no-vct-no-pvc")
	instance.Spec.Availability.AutoScaling = &openclawv1alpha1.AutoScalingSpec{
		Enabled: Ptr(true),
	}
	instance.Spec.Storage.Persistence.Enabled = Ptr(false)

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	// No VCTs when persistence is disabled
	if len(sts.Spec.VolumeClaimTemplates) != 0 {
		t.Errorf("VolumeClaimTemplates should be empty when persistence is disabled, got %d", len(sts.Spec.VolumeClaimTemplates))
	}

	// Data volume should be emptyDir
	dataVol := findVolume(sts.Spec.Template.Spec.Volumes, "data")
	if dataVol == nil {
		t.Fatal("data volume not found")
	}
	if dataVol.EmptyDir == nil {
		t.Error("data volume should be emptyDir when persistence is disabled")
	}
}

func TestBuildStatefulSet_VCT_DefaultSize(t *testing.T) {
	instance := newTestInstance("vct-default-size")
	instance.Spec.Availability.AutoScaling = &openclawv1alpha1.AutoScalingSpec{
		Enabled: Ptr(true),
	}
	// Don't set size - should default to 10Gi

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	if len(sts.Spec.VolumeClaimTemplates) != 1 {
		t.Fatalf("VolumeClaimTemplates length = %d, want 1", len(sts.Spec.VolumeClaimTemplates))
	}
	size := sts.Spec.VolumeClaimTemplates[0].Spec.Resources.Requests[corev1.ResourceStorage]
	if size.String() != "10Gi" {
		t.Errorf("VCT default storage size = %q, want %q", size.String(), "10Gi")
	}
}

func TestBuildStatefulSet_VCT_HasLabels(t *testing.T) {
	instance := newTestInstance("vct-labels")
	instance.Spec.Availability.AutoScaling = &openclawv1alpha1.AutoScalingSpec{
		Enabled: Ptr(true),
	}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	if len(sts.Spec.VolumeClaimTemplates) != 1 {
		t.Fatalf("VolumeClaimTemplates length = %d, want 1", len(sts.Spec.VolumeClaimTemplates))
	}
	labels := sts.Spec.VolumeClaimTemplates[0].Labels
	if labels["app.kubernetes.io/name"] != "openclaw" {
		t.Errorf("VCT missing expected label app.kubernetes.io/name=openclaw, got %v", labels)
	}
}

func TestBuildStatefulSet_Idempotent_WithHPAAndPersistence(t *testing.T) {
	instance := newTestInstance("idem-hpa")
	instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
		RawExtension: runtime.RawExtension{Raw: []byte(`{"key":"val"}`)},
	}
	instance.Spec.Availability.AutoScaling = &openclawv1alpha1.AutoScalingSpec{
		Enabled:              Ptr(true),
		MinReplicas:          Ptr(int32(1)),
		MaxReplicas:          Ptr(int32(5)),
		TargetCPUUtilization: Ptr(int32(80)),
	}
	instance.Spec.Storage.Persistence.Size = "20Gi"
	sc := "fast-ssd"
	instance.Spec.Storage.Persistence.StorageClass = &sc

	sts1 := BuildStatefulSet(instance, "", nil, nil, nil)
	sts2 := BuildStatefulSet(instance, "", nil, nil, nil)

	b1, _ := json.Marshal(sts1.Spec)
	b2, _ := json.Marshal(sts2.Spec)

	if !bytes.Equal(b1, b2) {
		t.Error("BuildStatefulSet with HPA and persistence is not idempotent - two calls with the same input produce different specs")
	}
}

// ---------------------------------------------------------------------------
// Additional workspaces (multi-agent support)
// ---------------------------------------------------------------------------

func TestBuildWorkspaceConfigMap_WithAdditionalWorkspaces(t *testing.T) {
	instance := newTestInstance("ws-addl")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{
				Name: "work",
				InitialFiles: map[string]string{
					"SOUL.md": "I am the work agent",
				},
			},
		},
	}

	cm := BuildWorkspaceConfigMap(instance, nil, nil, nil)
	if cm == nil {
		t.Fatal("expected non-nil ConfigMap")
	}

	// Check namespaced key for inline file
	key := AdditionalWorkspaceCMKey("work", "SOUL.md")
	if cm.Data[key] != "I am the work agent" {
		t.Errorf("expected work workspace SOUL.md, got %q", cm.Data[key])
	}

	// ENVIRONMENT.md should be injected for additional workspace
	envKey := AdditionalWorkspaceCMKey("work", "ENVIRONMENT.md")
	if cm.Data[envKey] != EnvironmentSkillContent {
		t.Error("expected ENVIRONMENT.md injected for additional workspace")
	}

	// BOOTSTRAP.md should NOT be injected for additional workspace
	bootstrapKey := AdditionalWorkspaceCMKey("work", "BOOTSTRAP.md")
	if _, ok := cm.Data[bootstrapKey]; ok {
		t.Error("BOOTSTRAP.md should not be injected for additional workspaces")
	}

	// Default workspace files should still be present
	if cm.Data["ENVIRONMENT.md"] != EnvironmentSkillContent {
		t.Error("default workspace ENVIRONMENT.md missing")
	}
	if cm.Data["BOOTSTRAP.md"] != BootstrapContent {
		t.Error("default workspace BOOTSTRAP.md missing")
	}
}

func TestBuildWorkspaceConfigMap_AdditionalWorkspaceMergePriority(t *testing.T) {
	instance := newTestInstance("ws-addl-merge")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{
				Name: "work",
				InitialFiles: map[string]string{
					"SOUL.md": "inline wins",
				},
			},
		},
	}
	additionalExt := map[string]map[string]string{
		"work": {
			"SOUL.md":   "external loses",
			"REMOTE.md": "only from external",
		},
	}

	cm := BuildWorkspaceConfigMap(instance, nil, additionalExt, nil)
	if cm == nil {
		t.Fatal("expected non-nil ConfigMap")
	}

	// Inline should override external
	key := AdditionalWorkspaceCMKey("work", "SOUL.md")
	if cm.Data[key] != "inline wins" {
		t.Errorf("SOUL.md should be inline value, got %q", cm.Data[key])
	}

	// External-only file should still be present
	remoteKey := AdditionalWorkspaceCMKey("work", "REMOTE.md")
	if cm.Data[remoteKey] != "only from external" {
		t.Errorf("REMOTE.md should be from external, got %q", cm.Data[remoteKey])
	}

	// ENVIRONMENT.md overrides everything (operator-injected)
	envKey := AdditionalWorkspaceCMKey("work", "ENVIRONMENT.md")
	if cm.Data[envKey] != EnvironmentSkillContent {
		t.Error("ENVIRONMENT.md should be operator-injected content")
	}
}

func TestBuildWorkspaceConfigMap_AdditionalWorkspaceOperatorFiles(t *testing.T) {
	instance := newTestInstance("ws-addl-ops")
	instance.Spec.SelfConfigure.Enabled = true
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{Name: "research"},
		},
	}

	cm := BuildWorkspaceConfigMap(instance, nil, nil, nil)

	// Default workspace gets SELFCONFIG.md and selfconfig.sh
	if _, ok := cm.Data["SELFCONFIG.md"]; !ok {
		t.Error("default workspace should have SELFCONFIG.md")
	}

	// Additional workspace should NOT get SELFCONFIG.md
	scKey := AdditionalWorkspaceCMKey("research", "SELFCONFIG.md")
	if _, ok := cm.Data[scKey]; ok {
		t.Error("additional workspace should not have SELFCONFIG.md")
	}

	// Additional workspace should get ENVIRONMENT.md
	envKey := AdditionalWorkspaceCMKey("research", "ENVIRONMENT.md")
	if cm.Data[envKey] != EnvironmentSkillContent {
		t.Error("additional workspace should have ENVIRONMENT.md")
	}
}

func TestBuildInitScript_AdditionalWorkspaces(t *testing.T) {
	instance := newTestInstance("ws-addl-init")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{
				Name: "work",
				InitialFiles: map[string]string{
					"SOUL.md": "work soul",
				},
				InitialDirectories: []string{"tools"},
			},
		},
	}

	script := BuildInitScript(instance, nil, nil, nil)

	// shellQuote wraps each path segment in single quotes
	// Should create the workspace directory
	if !strings.Contains(script, "mkdir -p /data/'workspace-work'") {
		t.Errorf("init script should create workspace-work directory, got:\n%s", script)
	}

	// Should create the tools subdirectory
	if !strings.Contains(script, "mkdir -p /data/'workspace-work'/'tools'") {
		t.Errorf("init script should create workspace-work/tools directory, got:\n%s", script)
	}

	// Should copy SOUL.md using namespaced key
	cmKey := AdditionalWorkspaceCMKey("work", "SOUL.md")
	if !strings.Contains(script, cmKey) {
		t.Errorf("init script should reference namespaced key %q, got:\n%s", cmKey, script)
	}

	// Should use seed-once semantics ([ -f ... ] || cp ...)
	if !strings.Contains(script, "[ -f /data/'workspace-work'/'SOUL.md' ] || cp") {
		t.Errorf("init script should use seed-once for SOUL.md, got:\n%s", script)
	}

	// Should also copy ENVIRONMENT.md for additional workspace
	envKey := AdditionalWorkspaceCMKey("work", "ENVIRONMENT.md")
	if !strings.Contains(script, envKey) {
		t.Errorf("init script should reference ENVIRONMENT.md namespaced key, got:\n%s", script)
	}
}

func TestConfigHash_ChangesWithAdditionalWorkspaceExternalFiles(t *testing.T) {
	instance := newTestInstance("ws-addl-hash")
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{Name: "work"},
		},
	}

	sts1 := BuildStatefulSet(instance, "", nil, nil, nil)
	hash1 := sts1.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	// Adding additional external files must change the hash: workspace-init
	// only copies them into the workspace at pod start.
	additionalExt := map[string]map[string]string{
		"work": {"SOUL.md": "work soul"},
	}
	sts2 := BuildStatefulSet(instance, "", nil, nil, additionalExt)
	hash2 := sts2.Spec.Template.Annotations["openclaw.rocks/config-hash"]

	if hash1 == hash2 {
		t.Error("config hash must change when additional-workspace external files change")
	}
}

func TestAdditionalWorkspaceCMKey_Roundtrip(t *testing.T) {
	tests := []struct {
		wsName   string
		filename string
	}{
		{"work", "SOUL.md"},
		{"research", "ENVIRONMENT.md"},
		{"my-agent", "deep-nested-file.txt"},
	}
	for _, tt := range tests {
		key := AdditionalWorkspaceCMKey(tt.wsName, tt.filename)
		gotName, gotFile, ok := ParseAdditionalWorkspaceCMKey(key)
		if !ok {
			t.Errorf("ParseAdditionalWorkspaceCMKey(%q) returned !ok", key)
			continue
		}
		if gotName != tt.wsName || gotFile != tt.filename {
			t.Errorf("roundtrip failed: got (%q, %q), want (%q, %q)", gotName, gotFile, tt.wsName, tt.filename)
		}
	}

	// Non-additional-workspace key should return false
	_, _, ok := ParseAdditionalWorkspaceCMKey("SOUL.md")
	if ok {
		t.Error("ParseAdditionalWorkspaceCMKey should return false for non-namespaced key")
	}
}

// ---------------------------------------------------------------------------
// Plugin NODE_PATH tests (#424)
// ---------------------------------------------------------------------------

func TestBuildStatefulSet_PluginsNodePath(t *testing.T) {
	instance := newTestInstance("plugin-env")
	instance.Spec.Plugins = []string{"@mem0/openclaw-mem0"}

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	mainContainer := sts.Spec.Template.Spec.Containers[0]
	envMap := map[string]string{}
	for _, ev := range mainContainer.Env {
		envMap[ev.Name] = ev.Value
	}

	want := "/home/openclaw/.openclaw/node_modules"
	if envMap["NODE_PATH"] != want {
		t.Errorf("NODE_PATH = %q, want %q", envMap["NODE_PATH"], want)
	}
}

func TestBuildStatefulSet_NoPluginsNoNodePath(t *testing.T) {
	instance := newTestInstance("no-plugin-env")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	mainContainer := sts.Spec.Template.Spec.Containers[0]
	for _, ev := range mainContainer.Env {
		if ev.Name == "NODE_PATH" {
			t.Error("NODE_PATH should not be set when no plugins are defined")
		}
	}
}

// --- Disk-aware readiness guard (spec.probes.diskReadiness) ---

func TestBuildStatefulSet_DiskReadiness_DefaultKeepsHTTPProbe(t *testing.T) {
	// No diskReadiness configured: readiness stays an HTTP GET /readyz probe.
	instance := newTestInstance("disk-readiness-default")
	main := BuildStatefulSet(instance, "", nil, nil, nil).Spec.Template.Spec.Containers[0]

	if main.ReadinessProbe == nil {
		t.Fatal("readiness probe should not be nil")
	}
	if main.ReadinessProbe.HTTPGet == nil {
		t.Fatal("readiness probe should be HTTP GET by default")
	}
	if main.ReadinessProbe.Exec != nil {
		t.Error("readiness probe should not be an exec probe by default")
	}
	if got := main.ReadinessProbe.HTTPGet.Path; got != "/readyz" {
		t.Errorf("readiness HTTP path = %q, want /readyz", got)
	}
}

func TestBuildStatefulSet_DiskReadiness_DisabledKeepsHTTPProbe(t *testing.T) {
	instance := newTestInstance("disk-readiness-disabled")
	instance.Spec.Probes = &openclawv1alpha1.ProbesSpec{
		DiskReadiness: &openclawv1alpha1.DiskReadinessSpec{Enabled: Ptr(false)},
	}
	main := BuildStatefulSet(instance, "", nil, nil, nil).Spec.Template.Spec.Containers[0]

	if main.ReadinessProbe.HTTPGet == nil || main.ReadinessProbe.Exec != nil {
		t.Error("readiness probe should remain HTTP when diskReadiness is disabled")
	}
}

func TestBuildStatefulSet_DiskReadiness_EnabledRendersExec(t *testing.T) {
	instance := newTestInstance("disk-readiness-on")
	instance.Spec.Probes = &openclawv1alpha1.ProbesSpec{
		DiskReadiness: &openclawv1alpha1.DiskReadinessSpec{Enabled: Ptr(true)},
	}
	main := BuildStatefulSet(instance, "", nil, nil, nil).Spec.Template.Spec.Containers[0]

	rp := main.ReadinessProbe
	if rp == nil || rp.Exec == nil {
		t.Fatal("readiness probe should be an exec probe when diskReadiness is enabled")
	}
	if rp.HTTPGet != nil {
		t.Error("readiness probe should not retain an HTTP handler when exec is used")
	}
	if len(rp.Exec.Command) != 3 || rp.Exec.Command[0] != "sh" || rp.Exec.Command[1] != "-c" {
		t.Fatalf("exec command = %v, want [sh -c <script>]", rp.Exec.Command)
	}
	script := rp.Exec.Command[2]
	for _, want := range []string{
		WorkspaceDataMountPath, // default workspace path
		"65536",                // default 64Mi threshold in KiB
		"/readyz",              // gateway readiness still checked
		"df -Pk",               // free-space check
		"-w \"$p\"",            // writability check
		// exec probe targets the same loopback port the HTTP probe would use
		fmt.Sprintf("127.0.0.1:%d/readyz", probeTargetPort(instance)),
	} {
		if !strings.Contains(script, want) {
			t.Errorf("exec script missing %q\nscript:\n%s", want, script)
		}
	}

	// Timing fields preserved.
	if rp.InitialDelaySeconds != 5 || rp.PeriodSeconds != 5 || rp.FailureThreshold != 3 {
		t.Errorf("readiness timing changed: %+v", rp)
	}

	// Liveness/startup remain HTTP /healthz so a full PVC does not CrashLoop.
	if main.LivenessProbe == nil || main.LivenessProbe.HTTPGet == nil || main.LivenessProbe.HTTPGet.Path != "/healthz" {
		t.Error("liveness probe should remain HTTP GET /healthz")
	}
	if main.StartupProbe == nil || main.StartupProbe.HTTPGet == nil || main.StartupProbe.HTTPGet.Path != "/healthz" {
		t.Error("startup probe should remain HTTP GET /healthz")
	}
}

func TestBuildStatefulSet_DiskReadiness_CustomPathAndThreshold(t *testing.T) {
	instance := newTestInstance("disk-readiness-custom")
	instance.Spec.Probes = &openclawv1alpha1.ProbesSpec{
		DiskReadiness: &openclawv1alpha1.DiskReadinessSpec{
			Enabled: Ptr(true),
			Path:    "/data/workspace",
			MinFree: "1Gi",
		},
	}
	main := BuildStatefulSet(instance, "", nil, nil, nil).Spec.Template.Spec.Containers[0]

	script := main.ReadinessProbe.Exec.Command[2]
	if !strings.Contains(script, "'/data/workspace'") {
		t.Errorf("exec script should check custom path, got:\n%s", script)
	}
	if !strings.Contains(script, "1048576") { // 1Gi in KiB
		t.Errorf("exec script should use 1Gi threshold in KiB, got:\n%s", script)
	}
}

// Regression test for #567: awk arithmetic ($4 * 1024) renders large results in
// scientific notation (e.g. 1.2642e+10) under busybox awk, which POSIX
// [ -ge ] rejects with "Illegal number", leaving healthy instances NotReady.
// The script must compare the df -Pk available column (an integer string) as
// KiB and never perform awk arithmetic on it.
func TestBuildStatefulSet_DiskReadiness_ScriptComparesIntegerKiB(t *testing.T) {
	instance := newTestInstance("disk-readiness-int")
	instance.Spec.Probes = &openclawv1alpha1.ProbesSpec{
		DiskReadiness: &openclawv1alpha1.DiskReadinessSpec{
			Enabled: Ptr(true),
			MinFree: "512Mi",
		},
	}
	main := BuildStatefulSet(instance, "", nil, nil, nil).Spec.Template.Spec.Containers[0]

	script := main.ReadinessProbe.Exec.Command[2]
	if strings.Contains(script, "* 1024") {
		t.Errorf("exec script must not multiply in awk (scientific notation breaks [ -ge ]), got:\n%s", script)
	}
	if !strings.Contains(script, "524288") { // 512Mi = 524288 KiB
		t.Errorf("exec script should compare against 524288 KiB, got:\n%s", script)
	}
}

func TestBuildStatefulSet_DiskReadiness_MinFreeKiBRoundsUp(t *testing.T) {
	instance := newTestInstance("disk-readiness-round")
	instance.Spec.Probes = &openclawv1alpha1.ProbesSpec{
		DiskReadiness: &openclawv1alpha1.DiskReadinessSpec{
			Enabled: Ptr(true),
			MinFree: "1025", // 1025 bytes -> must require 2 KiB, not truncate to 1
		},
	}
	main := BuildStatefulSet(instance, "", nil, nil, nil).Spec.Template.Spec.Containers[0]

	script := main.ReadinessProbe.Exec.Command[2]
	if !strings.Contains(script, "-ge 2 ") && !strings.Contains(script, "-ge 2\n") {
		t.Errorf("exec script should round the KiB threshold up to 2, got:\n%s", script)
	}
}
