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

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// ApplyClusterDefaults returns a deep copy of instance with unset fields filled
// from defaults. The original instance is never mutated so status/finalizer
// updates in the reconciler always write back the user's true spec, not a
// defaulted one.
//
// Precedence rules:
//   - Scalar fields (Registry, Image.Repository, Image.Tag, etc.) inherit
//     from the default only when the instance value is the zero value.
//   - Env is the exception. Cluster defaults and instance env are merged by
//     Name: cluster entries appear first, and any instance entry with the
//     same Name overrides the cluster default's value in place.
//   - RuntimeDeps enablement fields are plain booleans so "unset" and "false"
//     are indistinguishable; cluster defaults are OR-merged (a cluster default
//     of true always applies unless the instance has already set true). The uv
//     image follows normal per-instance precedence.
//
// If defaults is nil, the instance is returned unchanged (still deep-copied so
// callers can safely mutate it for in-memory derivation).
func ApplyClusterDefaults(instance *openclawv1alpha1.OpenClawInstance, defaults *openclawv1alpha1.OpenClawClusterDefaults) *openclawv1alpha1.OpenClawInstance {
	out := instance.DeepCopy()
	if defaults == nil {
		return out
	}

	d := defaults.Spec

	if out.Spec.Registry == "" {
		out.Spec.Registry = d.Registry
	}

	if out.Spec.Image.Repository == "" {
		out.Spec.Image.Repository = d.Image.Repository
	}
	// Only inherit the default Tag when the instance pinned neither a tag nor
	// a digest. Otherwise a cluster-default tag could silently override a
	// user-pinned digest resolution.
	if out.Spec.Image.Tag == "" && out.Spec.Image.Digest == "" {
		out.Spec.Image.Tag = d.Image.Tag
	}
	if out.Spec.Image.Digest == "" {
		out.Spec.Image.Digest = d.Image.Digest
	}
	if out.Spec.Image.PullPolicy == "" {
		out.Spec.Image.PullPolicy = d.Image.PullPolicy
	}
	if len(out.Spec.Image.PullSecrets) == 0 && len(d.Image.PullSecrets) > 0 {
		out.Spec.Image.PullSecrets = append([]corev1.LocalObjectReference(nil), d.Image.PullSecrets...)
	}

	out.Spec.Env = mergeEnvVars(d.Env, out.Spec.Env)

	if d.RuntimeDeps.Pnpm {
		out.Spec.RuntimeDeps.Pnpm = true
	}
	if d.RuntimeDeps.Python {
		out.Spec.RuntimeDeps.Python = true
	}
	if out.Spec.RuntimeDeps.UVImage == nil && d.RuntimeDeps.UVImage != nil {
		out.Spec.RuntimeDeps.UVImage = d.RuntimeDeps.UVImage.DeepCopy()
	}

	return out
}

// mergeEnvVars returns a merged env list where cluster defaults appear first
// and any instance entry with a matching Name overrides the default's value
// in place. Instance-only names are appended in their original order.
func mergeEnvVars(defaults, instance []corev1.EnvVar) []corev1.EnvVar {
	if len(defaults) == 0 {
		if len(instance) == 0 {
			return nil
		}
		return append([]corev1.EnvVar(nil), instance...)
	}

	instanceByName := make(map[string]corev1.EnvVar, len(instance))
	for _, e := range instance {
		instanceByName[e.Name] = e
	}

	merged := make([]corev1.EnvVar, 0, len(defaults)+len(instance))
	seen := make(map[string]bool, len(defaults))
	for _, d := range defaults {
		seen[d.Name] = true
		if override, ok := instanceByName[d.Name]; ok {
			merged = append(merged, override)
			continue
		}
		merged = append(merged, d)
	}
	for _, e := range instance {
		if seen[e.Name] {
			continue
		}
		merged = append(merged, e)
	}
	return merged
}
