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
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

func newClusterDefaults(spec *openclawv1alpha1.OpenClawClusterDefaultsSpec) *openclawv1alpha1.OpenClawClusterDefaults {
	return &openclawv1alpha1.OpenClawClusterDefaults{
		ObjectMeta: metav1.ObjectMeta{Name: openclawv1alpha1.ClusterDefaultsSingletonName},
		Spec:       *spec,
	}
}

func TestApplyClusterDefaults_NilDefaults(t *testing.T) {
	instance := newTestInstance("nil-defaults")
	instance.Spec.Registry = "instance-registry.example.com"

	out := ApplyClusterDefaults(instance, nil)

	if out == instance {
		t.Fatal("expected a deep copy, got the same pointer")
	}
	if out.Spec.Registry != "instance-registry.example.com" {
		t.Errorf("instance registry mutated: got %q", out.Spec.Registry)
	}
}

func TestApplyClusterDefaults_FillsUnsetRegistry(t *testing.T) {
	instance := newTestInstance("fills-registry")
	defaults := newClusterDefaults(&openclawv1alpha1.OpenClawClusterDefaultsSpec{
		Registry: "mirror.example.com",
	})

	out := ApplyClusterDefaults(instance, defaults)

	if out.Spec.Registry != "mirror.example.com" {
		t.Errorf("Registry: got %q, want %q", out.Spec.Registry, "mirror.example.com")
	}
}

func TestApplyClusterDefaults_InstanceRegistryWins(t *testing.T) {
	instance := newTestInstance("instance-wins")
	instance.Spec.Registry = "instance-mirror.example.com"
	defaults := newClusterDefaults(&openclawv1alpha1.OpenClawClusterDefaultsSpec{
		Registry: "cluster-mirror.example.com",
	})

	out := ApplyClusterDefaults(instance, defaults)

	if out.Spec.Registry != "instance-mirror.example.com" {
		t.Errorf("instance registry should win: got %q", out.Spec.Registry)
	}
}

func TestApplyClusterDefaults_ImageFieldsMergeIndependently(t *testing.T) {
	instance := newTestInstance("image-merge")
	instance.Spec.Image.Repository = "private.example.com/openclaw"
	defaults := newClusterDefaults(&openclawv1alpha1.OpenClawClusterDefaultsSpec{
		Image: openclawv1alpha1.ImageSpec{
			Repository: "mirror.example.com/openclaw",
			Tag:        "v1.2.3",
			PullPolicy: corev1.PullAlways,
		},
	})

	out := ApplyClusterDefaults(instance, defaults)

	if out.Spec.Image.Repository != "private.example.com/openclaw" {
		t.Errorf("instance repository should win: got %q", out.Spec.Image.Repository)
	}
	if out.Spec.Image.Tag != "v1.2.3" {
		t.Errorf("tag should inherit default: got %q", out.Spec.Image.Tag)
	}
	if out.Spec.Image.PullPolicy != corev1.PullAlways {
		t.Errorf("pullPolicy should inherit default: got %q", out.Spec.Image.PullPolicy)
	}
}

func TestApplyClusterDefaults_DigestBlocksDefaultTag(t *testing.T) {
	instance := newTestInstance("digest-pin")
	instance.Spec.Image.Digest = "sha256:abc123"
	defaults := newClusterDefaults(&openclawv1alpha1.OpenClawClusterDefaultsSpec{
		Image: openclawv1alpha1.ImageSpec{Tag: "v1.2.3"},
	})

	out := ApplyClusterDefaults(instance, defaults)

	if out.Spec.Image.Tag != "" {
		t.Errorf("default tag must not overwrite digest-pinned instance: got tag %q", out.Spec.Image.Tag)
	}
	if out.Spec.Image.Digest != "sha256:abc123" {
		t.Errorf("digest mutated: got %q", out.Spec.Image.Digest)
	}
}

func TestApplyClusterDefaults_PullSecretsReplaceOnly(t *testing.T) {
	defaults := newClusterDefaults(&openclawv1alpha1.OpenClawClusterDefaultsSpec{
		Image: openclawv1alpha1.ImageSpec{
			PullSecrets: []corev1.LocalObjectReference{{Name: "cluster-registry-creds"}},
		},
	})

	// Empty instance pull secrets -> inherit cluster defaults.
	instance := newTestInstance("pull-secrets-inherit")
	out := ApplyClusterDefaults(instance, defaults)
	if len(out.Spec.Image.PullSecrets) != 1 || out.Spec.Image.PullSecrets[0].Name != "cluster-registry-creds" {
		t.Errorf("empty instance pull secrets should inherit defaults: got %+v", out.Spec.Image.PullSecrets)
	}

	// Instance pull secrets -> defaults are NOT merged in.
	instance2 := newTestInstance("pull-secrets-override")
	instance2.Spec.Image.PullSecrets = []corev1.LocalObjectReference{{Name: "instance-creds"}}
	out2 := ApplyClusterDefaults(instance2, defaults)
	if len(out2.Spec.Image.PullSecrets) != 1 || out2.Spec.Image.PullSecrets[0].Name != "instance-creds" {
		t.Errorf("instance pull secrets should win entirely: got %+v", out2.Spec.Image.PullSecrets)
	}
}

func TestApplyClusterDefaults_EnvMergeAndOverride(t *testing.T) {
	instance := newTestInstance("env-merge")
	instance.Spec.Env = []corev1.EnvVar{
		{Name: "PIP_INDEX_URL", Value: "https://instance.example.com/pypi"},
		{Name: "EXTRA", Value: "instance-only"},
	}
	defaults := newClusterDefaults(&openclawv1alpha1.OpenClawClusterDefaultsSpec{
		Env: []corev1.EnvVar{
			{Name: "NPM_CONFIG_REGISTRY", Value: "https://mirror/npm"},
			{Name: "PIP_INDEX_URL", Value: "https://mirror/pypi"},
		},
	})

	out := ApplyClusterDefaults(instance, defaults)

	got := map[string]string{}
	var order []string
	for _, e := range out.Spec.Env {
		got[e.Name] = e.Value
		order = append(order, e.Name)
	}

	if got["NPM_CONFIG_REGISTRY"] != "https://mirror/npm" {
		t.Errorf("NPM_CONFIG_REGISTRY should come from cluster defaults: got %q", got["NPM_CONFIG_REGISTRY"])
	}
	if got["PIP_INDEX_URL"] != "https://instance.example.com/pypi" {
		t.Errorf("instance PIP_INDEX_URL should override cluster default: got %q", got["PIP_INDEX_URL"])
	}
	if got["EXTRA"] != "instance-only" {
		t.Errorf("instance-only EXTRA should survive merge: got %q", got["EXTRA"])
	}

	// Defaults come first, instance-only names appended.
	wantOrder := []string{"NPM_CONFIG_REGISTRY", "PIP_INDEX_URL", "EXTRA"}
	if len(order) != len(wantOrder) {
		t.Fatalf("env order: got %v, want %v", order, wantOrder)
	}
	for i, name := range wantOrder {
		if order[i] != name {
			t.Errorf("env order at %d: got %q, want %q (full: %v)", i, order[i], name, order)
		}
	}
}

func TestApplyClusterDefaults_EmptyEnvs(t *testing.T) {
	instance := newTestInstance("empty-env")
	out := ApplyClusterDefaults(instance, newClusterDefaults(&openclawv1alpha1.OpenClawClusterDefaultsSpec{}))
	if out.Spec.Env != nil {
		t.Errorf("empty defaults + empty instance env should yield nil, got %+v", out.Spec.Env)
	}
}

func TestApplyClusterDefaults_RuntimeDepsORMerge(t *testing.T) {
	tests := []struct {
		name       string
		instance   openclawv1alpha1.RuntimeDepsSpec
		defaults   openclawv1alpha1.RuntimeDepsSpec
		wantPnpm   bool
		wantPython bool
	}{
		{"both off", openclawv1alpha1.RuntimeDepsSpec{}, openclawv1alpha1.RuntimeDepsSpec{}, false, false},
		{"default pnpm", openclawv1alpha1.RuntimeDepsSpec{}, openclawv1alpha1.RuntimeDepsSpec{Pnpm: true}, true, false},
		{"default python", openclawv1alpha1.RuntimeDepsSpec{}, openclawv1alpha1.RuntimeDepsSpec{Python: true}, false, true},
		{"instance pnpm overrides default false", openclawv1alpha1.RuntimeDepsSpec{Pnpm: true}, openclawv1alpha1.RuntimeDepsSpec{}, true, false},
		{"both set true", openclawv1alpha1.RuntimeDepsSpec{Pnpm: true, Python: true}, openclawv1alpha1.RuntimeDepsSpec{Pnpm: true, Python: true}, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			instance := newTestInstance("rd")
			instance.Spec.RuntimeDeps = tc.instance
			defaults := newClusterDefaults(&openclawv1alpha1.OpenClawClusterDefaultsSpec{RuntimeDeps: tc.defaults})

			out := ApplyClusterDefaults(instance, defaults)

			if out.Spec.RuntimeDeps.Pnpm != tc.wantPnpm {
				t.Errorf("Pnpm: got %v, want %v", out.Spec.RuntimeDeps.Pnpm, tc.wantPnpm)
			}
			if out.Spec.RuntimeDeps.Python != tc.wantPython {
				t.Errorf("Python: got %v, want %v", out.Spec.RuntimeDeps.Python, tc.wantPython)
			}
		})
	}
}

func TestApplyClusterDefaults_RuntimeDepsUVImagePrecedence(t *testing.T) {
	defaultImage := &openclawv1alpha1.UVImageSpec{
		Repository: "mirror.example.com/astral-sh/uv",
		Digest:     "sha256:default",
	}
	defaults := newClusterDefaults(&openclawv1alpha1.OpenClawClusterDefaultsSpec{
		RuntimeDeps: openclawv1alpha1.RuntimeDepsSpec{UVImage: defaultImage},
	})

	inherited := ApplyClusterDefaults(newTestInstance("inherits-uv"), defaults)
	if inherited.Spec.RuntimeDeps.UVImage == nil {
		t.Fatal("cluster-default uv image was not inherited")
	}
	if inherited.Spec.RuntimeDeps.UVImage.Digest != "sha256:default" {
		t.Errorf("inherited digest = %q, want sha256:default", inherited.Spec.RuntimeDeps.UVImage.Digest)
	}
	if inherited.Spec.RuntimeDeps.UVImage == defaultImage {
		t.Error("inherited uv image must be deep-copied")
	}

	instance := newTestInstance("overrides-uv")
	instance.Spec.RuntimeDeps.UVImage = &openclawv1alpha1.UVImageSpec{
		Repository: "registry.example.com/custom/uv",
		Tag:        "custom",
	}
	overridden := ApplyClusterDefaults(instance, defaults)
	if overridden.Spec.RuntimeDeps.UVImage.Repository != "registry.example.com/custom/uv" {
		t.Errorf("instance uv image repository was overridden: %q", overridden.Spec.RuntimeDeps.UVImage.Repository)
	}
	if overridden.Spec.RuntimeDeps.UVImage.Tag != "custom" {
		t.Errorf("instance uv image tag was overridden: %q", overridden.Spec.RuntimeDeps.UVImage.Tag)
	}
}

func TestApplyClusterDefaults_DoesNotMutateInstance(t *testing.T) {
	instance := newTestInstance("immutability")
	defaults := newClusterDefaults(&openclawv1alpha1.OpenClawClusterDefaultsSpec{
		Registry: "mirror.example.com",
		Env: []corev1.EnvVar{
			{Name: "PIP_INDEX_URL", Value: "https://mirror/pypi"},
		},
	})

	_ = ApplyClusterDefaults(instance, defaults)

	if instance.Spec.Registry != "" {
		t.Errorf("instance.Spec.Registry was mutated: got %q", instance.Spec.Registry)
	}
	if instance.Spec.Env != nil {
		t.Errorf("instance.Spec.Env was mutated: got %+v", instance.Spec.Env)
	}
}
