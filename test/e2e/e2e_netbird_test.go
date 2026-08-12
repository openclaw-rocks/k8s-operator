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
	"fmt"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
	"github.com/paperclipinc/openclaw-operator/internal/resources"
)

// NetBird mesh provider (#560). The sidecar cannot actually enroll without a
// real control plane and setup key, so what is verified here is the rendered
// topology: the right sidecar, the right security posture, the right egress, and
// no Kubernetes API access.
var _ = Describe("NetBird mesh provider", func() {
	const (
		timeout  = time.Second * 60
		interval = time.Second * 1
	)

	var (
		namespace string
		specNum   int
	)

	newNetBirdInstance := func(name string) *openclawv1alpha1.OpenClawInstance {
		return &openclawv1alpha1.OpenClawInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
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
				NetBird: &openclawv1alpha1.NetBirdSpec{
					Enabled: true,
					SetupKeySecretRef: &corev1.LocalObjectReference{
						Name: "netbird-setup-key",
					},
				},
			},
		}
	}

	getStatefulSet := func(instance *openclawv1alpha1.OpenClawInstance) *appsv1.StatefulSet {
		sts := &appsv1.StatefulSet{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      resources.StatefulSetName(instance),
				Namespace: namespace,
			}, sts)
		}, timeout, interval).Should(Succeed())
		return sts
	}

	BeforeEach(func() {
		if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
			Skip("Skipping resource validation in minimal mode")
		}
		// Second granularity alone collides between specs that start within the
		// same second, and namespace creation then fails with AlreadyExists.
		specNum++
		namespace = fmt.Sprintf("test-netbird-%s-%d", time.Now().Format("20060102150405"), specNum)
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

		// The referenced Secret need not hold a working key for the pod spec to
		// render, but it should exist so the instance is not marked degraded.
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "netbird-setup-key", Namespace: namespace},
			StringData: map[string]string{"setupkey": "not-a-real-key"},
		}
		Expect(k8sClient.Create(ctx, secret)).Should(Succeed())
	})

	AfterEach(func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		_ = k8sClient.Delete(ctx, ns)
	})

	It("Should render the NetBird sidecar with a hardened security context", func() {
		instance := newNetBirdInstance("netbird-sidecar")
		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

		sts := getStatefulSet(instance)

		var sidecar *corev1.Container
		for i := range sts.Spec.Template.Spec.Containers {
			if sts.Spec.Template.Spec.Containers[i].Name == "netbird" {
				sidecar = &sts.Spec.Template.Spec.Containers[i]
			}
			Expect(sts.Spec.Template.Spec.Containers[i].Name).NotTo(Equal("tailscale"),
				"the tailscale sidecar must not appear when netbird is the provider")
		}
		Expect(sidecar).NotTo(BeNil(), "netbird sidecar not found")

		By("running in netstack mode so no elevated capabilities are needed")
		var netstack string
		for _, e := range sidecar.Env {
			if e.Name == "NB_USE_NETSTACK_MODE" {
				netstack = e.Value
			}
		}
		Expect(netstack).To(Equal("true"))

		Expect(sidecar.SecurityContext).NotTo(BeNil())
		Expect(sidecar.SecurityContext.Capabilities.Drop).To(ConsistOf(corev1.Capability("ALL")))
		Expect(*sidecar.SecurityContext.ReadOnlyRootFilesystem).To(BeTrue())
		Expect(*sidecar.SecurityContext.AllowPrivilegeEscalation).To(BeFalse())

		By("sourcing the setup key from the Secret rather than inlining it")
		var found bool
		for _, e := range sidecar.Env {
			if e.Name == "NB_SETUP_KEY" {
				found = true
				Expect(e.Value).To(BeEmpty())
				Expect(e.ValueFrom).NotTo(BeNil())
				Expect(e.ValueFrom.SecretKeyRef.Name).To(Equal("netbird-setup-key"))
			}
		}
		Expect(found).To(BeTrue(), "NB_SETUP_KEY not found")
	})

	It("Should not mount a ServiceAccount token for NetBird", func() {
		instance := newNetBirdInstance("netbird-no-token")
		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

		sts := getStatefulSet(instance)

		// Unlike Tailscale's containerboot, nothing in the NetBird sidecar talks
		// to the Kubernetes API.
		Expect(sts.Spec.Template.Spec.AutomountServiceAccountToken).NotTo(BeNil())
		Expect(*sts.Spec.Template.Spec.AutomountServiceAccountToken).To(BeFalse())
	})

	It("Should open NetBird control and data plane egress", func() {
		instance := newNetBirdInstance("netbird-egress")
		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

		np := &networkingv1.NetworkPolicy{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      resources.NetworkPolicyName(instance),
				Namespace: namespace,
			}, np)
		}, timeout, interval).Should(Succeed())

		allowed := map[int]bool{}
		for _, rule := range np.Spec.Egress {
			for _, p := range rule.Ports {
				if p.Port != nil {
					allowed[p.Port.IntValue()] = true
				}
			}
		}
		for _, port := range []int{
			resources.NetBirdManagementPort,
			resources.NetBirdSignalPort,
			resources.NetBirdWireGuardPort,
		} {
			Expect(allowed).To(HaveKey(port), "egress should allow port %d", port)
		}

		// Tailscale's WireGuard port must not be opened for a NetBird instance.
		Expect(allowed).NotTo(HaveKey(41641))
	})

	It("Should create no state Secret for NetBird", func() {
		instance := newNetBirdInstance("netbird-no-state")
		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

		// Wait for reconciliation to have produced the StatefulSet first.
		getStatefulSet(instance)

		secret := &corev1.Secret{}
		err := k8sClient.Get(ctx, types.NamespacedName{
			Name:      resources.TailscaleStateSecretName(instance),
			Namespace: namespace,
		}, secret)
		Expect(err).To(HaveOccurred(), "netbird keeps peer state on a volume, not in a Secret")
	})
})
