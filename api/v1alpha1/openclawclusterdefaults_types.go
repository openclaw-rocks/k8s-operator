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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ClusterDefaultsSingletonName is the only accepted name for the cluster-scoped
// OpenClawClusterDefaults singleton. Any other name is ignored by the operator.
const ClusterDefaultsSingletonName = "cluster"

// OpenClawClusterDefaultsSpec defines cluster-wide defaults that the operator
// applies to every OpenClawInstance at reconcile time. Per-instance fields
// always win: a default is only applied when the instance field is unset.
type OpenClawClusterDefaultsSpec struct {
	// Registry is the default container image registry override applied to
	// instances where spec.registry is unset. Replaces the registry prefix of
	// all container images (main, sidecars, init containers).
	// Example: "my-registry.example.com".
	// +optional
	Registry string `json:"registry,omitempty"`

	// Image is the default container image configuration applied to instances
	// where the corresponding instance fields are unset. Each sub-field is
	// merged independently (e.g. a cluster-default tag still applies even when
	// the instance sets its own repository).
	// +optional
	Image ImageSpec `json:"image,omitempty"`

	// Env is a list of default environment variables merged into every
	// instance's container env. Instance-level env entries with the same Name
	// override the cluster default for that name. Defaults appear first in
	// the resulting env list, followed by instance-only names.
	// +listType=map
	// +listMapKey=name
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// RuntimeDeps configures the default set of built-in init containers
	// (pnpm, Python) applied to instances where the corresponding fields are
	// unset. A cluster default of true for a runtime dep is always applied
	// unless the instance explicitly opts out (sets the field to false).
	// NOTE: because the enablement fields are plain booleans, "unset" and
	// "false" are indistinguishable; those fields are OR-merged here. UVImage
	// follows normal precedence and applies only when the instance omits it.
	// +optional
	RuntimeDeps RuntimeDepsSpec `json:"runtimeDeps,omitempty"`
}

// OpenClawClusterDefaultsStatus reports which singleton (if any) is currently
// being applied by the operator.
type OpenClawClusterDefaultsStatus struct {
	// Conditions describes the current state of the singleton, including
	// whether the name matches the expected "cluster" singleton.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the generation of the spec most recently
	// processed by the operator.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,shortName=occd
// +kubebuilder:printcolumn:name="Registry",type=string,JSONPath=`.spec.registry`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// OpenClawClusterDefaults is a cluster-scoped singleton (name must be "cluster")
// that provides default values merged into every OpenClawInstance at reconcile
// time. It exists so platform operators managing air-gapped or restricted-network
// environments can set a single source of truth for image registry mirrors,
// shared environment variables (e.g. NPM_CONFIG_REGISTRY, PIP_INDEX_URL), and
// runtime-dep init containers without duplicating the same boilerplate in every
// OpenClawInstance manifest.
//
// Precedence: per-instance fields always win over cluster defaults. A default
// is only applied when the corresponding instance field is unset.
type OpenClawClusterDefaults struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpenClawClusterDefaultsSpec   `json:"spec,omitempty"`
	Status OpenClawClusterDefaultsStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpenClawClusterDefaultsList contains a list of OpenClawClusterDefaults.
type OpenClawClusterDefaultsList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpenClawClusterDefaults `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpenClawClusterDefaults{}, &OpenClawClusterDefaultsList{})
}
