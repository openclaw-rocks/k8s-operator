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

package e2e

import (
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
	"github.com/paperclipinc/openclaw-operator/internal/resources"
)

// Regression coverage for #587, which reported that
// security.networkPolicy.allowedIngressNamespaces was accepted but never
// applied. The builder and the reconcile path both handle the field and unit
// tests cover the builder, so this asserts the same thing end-to-end on a real
// cluster: the namespaces reach the generated NetworkPolicy, and they keep
// reaching it after the field is edited on an existing instance.
var _ = Describe("NetworkPolicy ingress allowlists", func() {
	const (
		timeout  = time.Second * 60
		interval = time.Second * 1
	)

	var namespace string

	BeforeEach(func() {
		if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
			Skip("Skipping resource validation in minimal mode")
		}
		namespace = "test-netpol-" + time.Now().Format("20060102150405")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
	})

	AfterEach(func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		_ = k8sClient.Delete(ctx, ns)
	})

	// ingressNamespaces returns every namespace named by the policy's ingress peers.
	ingressNamespaces := func(np *networkingv1.NetworkPolicy) []string {
		var out []string
		for _, rule := range np.Spec.Ingress {
			for _, peer := range rule.From {
				if peer.NamespaceSelector != nil {
					if ns, ok := peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"]; ok {
						out = append(out, ns)
					}
				}
			}
		}
		return out
	}

	It("Should apply allowedIngressNamespaces to the generated NetworkPolicy", func() {
		instanceName := "netpol-allowlist"
		instance := &openclawv1alpha1.OpenClawInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      instanceName,
				Namespace: namespace,
				Annotations: map[string]string{
					"openclaw.rocks/skip-backup": "true",
				},
			},
			Spec: openclawv1alpha1.OpenClawInstanceSpec{
				Image: openclawv1alpha1.ImageSpec{
					Repository: "ghcr.io/openclaw/openclaw",
					Tag:        "latest",
				},
			},
		}
		instance.Spec.Security.NetworkPolicy.AllowedIngressNamespaces = []string{"some-other-namespace"}
		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

		np := &networkingv1.NetworkPolicy{}
		npKey := types.NamespacedName{
			Name:      resources.NetworkPolicyName(instance),
			Namespace: namespace,
		}
		Eventually(func() error {
			return k8sClient.Get(ctx, npKey, np)
		}, timeout, interval).Should(Succeed())

		Expect(ingressNamespaces(np)).To(ContainElement("some-other-namespace"),
			"allowedIngressNamespaces should produce a namespaceSelector ingress peer")
		Expect(ingressNamespaces(np)).To(ContainElement(namespace),
			"the same-namespace default rule should still be present")
	})

	It("Should converge the NetworkPolicy when allowedIngressNamespaces is edited", func() {
		instanceName := "netpol-allowlist-edit"
		instance := &openclawv1alpha1.OpenClawInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      instanceName,
				Namespace: namespace,
				Annotations: map[string]string{
					"openclaw.rocks/skip-backup": "true",
				},
			},
			Spec: openclawv1alpha1.OpenClawInstanceSpec{
				Image: openclawv1alpha1.ImageSpec{
					Repository: "ghcr.io/openclaw/openclaw",
					Tag:        "latest",
				},
			},
		}
		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

		npKey := types.NamespacedName{
			Name:      resources.NetworkPolicyName(instance),
			Namespace: namespace,
		}
		np := &networkingv1.NetworkPolicy{}
		Eventually(func() error {
			return k8sClient.Get(ctx, npKey, np)
		}, timeout, interval).Should(Succeed())
		Expect(ingressNamespaces(np)).NotTo(ContainElement("added-later"))

		By("adding a namespace to an existing instance")
		Eventually(func() error {
			current := &openclawv1alpha1.OpenClawInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name: instanceName, Namespace: namespace,
			}, current); err != nil {
				return err
			}
			current.Spec.Security.NetworkPolicy.AllowedIngressNamespaces = []string{"added-later"}
			return k8sClient.Update(ctx, current)
		}, timeout, interval).Should(Succeed())

		Eventually(func() []string {
			updated := &networkingv1.NetworkPolicy{}
			if err := k8sClient.Get(ctx, npKey, updated); err != nil {
				return nil
			}
			return ingressNamespaces(updated)
		}, timeout, interval).Should(ContainElement("added-later"),
			"editing allowedIngressNamespaces should be reflected in the NetworkPolicy")
	})
})
