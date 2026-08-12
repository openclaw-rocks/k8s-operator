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
	"sort"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// BuildServiceAccount creates a ServiceAccount for the OpenClawInstance
func BuildServiceAccount(instance *openclawv1alpha1.OpenClawInstance) *corev1.ServiceAccount {
	labels := Labels(instance)

	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:        ServiceAccountName(instance),
			Namespace:   instance.Namespace,
			Labels:      labels,
			Annotations: instance.Spec.Security.RBAC.ServiceAccountAnnotations,
		},
		AutomountServiceAccountToken: Ptr(instance.Spec.SelfConfigure.Enabled || MeshNeedsServiceAccountToken(instance)),
	}
}

// BuildRole creates a Role for the OpenClawInstance
// This implements the principle of least privilege - only granting what's needed
func BuildRole(instance *openclawv1alpha1.OpenClawInstance) *rbacv1.Role {
	labels := Labels(instance)

	// Base rules - minimal permissions needed by OpenClaw
	rules := []rbacv1.PolicyRule{
		// OpenClaw only needs to read its own config
		{
			APIGroups:     []string{""},
			Resources:     []string{"configmaps"},
			ResourceNames: []string{ConfigMapName(instance)},
			Verbs:         []string{"get", "watch"},
		},
	}

	// Self-configure RBAC rules - give the agent access to K8s API
	if instance.Spec.SelfConfigure.Enabled {
		// Read own OpenClawInstance (scoped by resourceNames) + create/read self-config requests
		rules = append(rules,
			rbacv1.PolicyRule{
				APIGroups:     []string{"openclaw.rocks"},
				Resources:     []string{"openclawinstances"},
				ResourceNames: []string{instance.Name},
				Verbs:         []string{"get"},
			},
			rbacv1.PolicyRule{
				APIGroups: []string{"openclaw.rocks"},
				Resources: []string{"openclawselfconfigs"},
				Verbs:     []string{"create", "get", "list"},
			},
		)

		// Read own referenced secrets (scoped by resourceNames)
		if secretNames := selfConfigSecretNames(instance); len(secretNames) > 0 {
			rules = append(rules, rbacv1.PolicyRule{
				APIGroups:     []string{""},
				Resources:     []string{"secrets"},
				ResourceNames: secretNames,
				Verbs:         []string{"get"},
			})
		}
	}

	// Mesh provider state Secret -- e.g. Tailscale's containerboot needs to
	// read and write its node identity. Providers that keep state on a volume
	// return "" and get no rule (#560).
	if stateSecret := MeshStateSecretName(instance); stateSecret != "" {
		rules = append(rules, rbacv1.PolicyRule{
			APIGroups:     []string{""},
			Resources:     []string{"secrets"},
			ResourceNames: []string{stateSecret},
			Verbs:         []string{"get", "update", "patch"},
		})
	}

	// Add additional rules from spec
	for _, rule := range instance.Spec.Security.RBAC.AdditionalRules {
		rules = append(rules, rbacv1.PolicyRule{
			APIGroups: rule.APIGroups,
			Resources: rule.Resources,
			Verbs:     rule.Verbs,
		})
	}

	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:      RoleName(instance),
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		Rules: rules,
	}
}

// selfConfigSecretNames collects all secret names referenced by the instance (deduplicated).
func selfConfigSecretNames(instance *openclawv1alpha1.OpenClawInstance) []string {
	seen := make(map[string]bool)

	// Gateway token secret (auto-generated)
	if instance.Spec.Gateway.ExistingSecret == "" {
		seen[GatewayTokenSecretName(instance)] = true
	} else {
		seen[instance.Spec.Gateway.ExistingSecret] = true
	}

	// EnvFrom secret refs
	for _, ef := range instance.Spec.EnvFrom {
		if ef.SecretRef != nil && ef.SecretRef.Name != "" {
			seen[ef.SecretRef.Name] = true
		}
	}

	// Mesh provider join credential (Tailscale auth key, NetBird setup key)
	if credential := MeshCredentialSecretName(instance); credential != "" {
		seen[credential] = true
	}

	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// BuildRoleBinding creates a RoleBinding for the OpenClawInstance
func BuildRoleBinding(instance *openclawv1alpha1.OpenClawInstance) *rbacv1.RoleBinding {
	labels := Labels(instance)

	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      RoleBindingName(instance),
			Namespace: instance.Namespace,
			Labels:    labels,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "Role",
			Name:     RoleName(instance),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      ServiceAccountName(instance),
				Namespace: instance.Namespace,
			},
		},
	}
}
