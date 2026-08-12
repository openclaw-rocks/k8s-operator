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

package controller

import (
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
	"github.com/paperclipinc/openclaw-operator/internal/resources"
)

// ServiceMonitor lifecycle (#589). Disabling the flag used to leave a stale
// scrape target behind, and metrics=false + serviceMonitor=true produced a
// target for an endpoint that was never served.
var _ = Describe("ServiceMonitor lifecycle", func() {
	const (
		timeout  = time.Second * 30
		interval = time.Millisecond * 250
	)

	var (
		instanceName string
		instanceKey  types.NamespacedName
		smKey        types.NamespacedName
		counter      int
	)

	// getServiceMonitor fetches the managed ServiceMonitor, if it exists.
	getServiceMonitor := func() (*unstructured.Unstructured, error) {
		sm := &unstructured.Unstructured{}
		sm.SetGroupVersionKind(resources.ServiceMonitorGVK())
		err := k8sClient.Get(ctx, smKey, sm)
		return sm, err
	}

	serviceMonitorExists := func() bool {
		_, err := getServiceMonitor()
		return err == nil
	}

	setServiceMonitorEnabled := func(enabled bool) {
		Eventually(func() error {
			instance := &openclawv1alpha1.OpenClawInstance{}
			if err := k8sClient.Get(ctx, instanceKey, instance); err != nil {
				return err
			}
			instance.Spec.Observability.Metrics.ServiceMonitor = &openclawv1alpha1.ServiceMonitorSpec{
				Enabled: resources.Ptr(enabled),
			}
			return k8sClient.Update(ctx, instance)
		}, timeout, interval).Should(Succeed())
	}

	setMetricsEnabled := func(enabled bool) {
		Eventually(func() error {
			instance := &openclawv1alpha1.OpenClawInstance{}
			if err := k8sClient.Get(ctx, instanceKey, instance); err != nil {
				return err
			}
			instance.Spec.Observability.Metrics.Enabled = resources.Ptr(enabled)
			return k8sClient.Update(ctx, instance)
		}, timeout, interval).Should(Succeed())
	}

	BeforeEach(func() {
		counter++
		instanceName = fmt.Sprintf("sm-lifecycle-%d", counter)
		instanceKey = types.NamespacedName{Name: instanceName, Namespace: "default"}
		smKey = instanceKey

		instance := &openclawv1alpha1.OpenClawInstance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      instanceName,
				Namespace: "default",
			},
			Spec: openclawv1alpha1.OpenClawInstanceSpec{
				Observability: openclawv1alpha1.ObservabilitySpec{
					Metrics: openclawv1alpha1.MetricsSpec{
						Enabled: resources.Ptr(true),
						ServiceMonitor: &openclawv1alpha1.ServiceMonitorSpec{
							Enabled: resources.Ptr(true),
						},
					},
				},
			},
		}
		instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
			RawExtension: runtime.RawExtension{Raw: []byte(`{}`)},
		}
		Expect(k8sClient.Create(ctx, instance)).To(Succeed())

		By("waiting for the ServiceMonitor to be created")
		Eventually(serviceMonitorExists, timeout, interval).Should(BeTrue())
	})

	AfterEach(func() {
		instance := &openclawv1alpha1.OpenClawInstance{}
		if err := k8sClient.Get(ctx, instanceKey, instance); err == nil {
			Expect(k8sClient.Delete(ctx, instance)).To(Succeed())
		}
	})

	It("removes the ServiceMonitor when the flag is disabled and recreates it on re-enable", func() {
		By("disabling the ServiceMonitor")
		setServiceMonitorEnabled(false)

		Eventually(serviceMonitorExists, timeout, interval).Should(BeFalse(),
			"a disabled ServiceMonitor must not leave a stale scrape target behind")

		By("clearing it from status.managedResources")
		Eventually(func() string {
			instance := &openclawv1alpha1.OpenClawInstance{}
			if err := k8sClient.Get(ctx, instanceKey, instance); err != nil {
				return "error"
			}
			return instance.Status.ManagedResources.ServiceMonitor
		}, timeout, interval).Should(BeEmpty())

		By("re-enabling the ServiceMonitor")
		setServiceMonitorEnabled(true)

		Eventually(serviceMonitorExists, timeout, interval).Should(BeTrue())
	})

	It("does not keep a ServiceMonitor when metrics are disabled", func() {
		By("disabling metrics while leaving serviceMonitor.enabled true")
		setMetricsEnabled(false)

		Eventually(serviceMonitorExists, timeout, interval).Should(BeFalse(),
			"a ServiceMonitor without a metrics endpoint would scrape nothing")

		By("surfacing the invalid combination as a status condition")
		Eventually(func() string {
			instance := &openclawv1alpha1.OpenClawInstance{}
			if err := k8sClient.Get(ctx, instanceKey, instance); err != nil {
				return ""
			}
			for _, c := range instance.Status.Conditions {
				if c.Type == openclawv1alpha1.ConditionTypeServiceMonitorReady {
					return c.Reason
				}
			}
			return ""
		}, timeout, interval).Should(Equal("MetricsDisabled"))

		By("restoring it once metrics are re-enabled")
		setMetricsEnabled(true)
		Eventually(serviceMonitorExists, timeout, interval).Should(BeTrue())
	})

	It("recreates the ServiceMonitor after an out-of-band deletion", func() {
		sm, err := getServiceMonitor()
		Expect(err).NotTo(HaveOccurred())

		By("deleting the ServiceMonitor behind the operator's back")
		Expect(k8sClient.Delete(ctx, sm)).To(Succeed())

		By("converging it back without waiting for a resync")
		Eventually(serviceMonitorExists, timeout, interval).Should(BeTrue())
	})

	// Deletion of the parent is what garbage-collects the ServiceMonitor, and
	// that is driven entirely by this owner reference. envtest runs no garbage
	// collector, so the collection itself belongs to e2e -- what is checked
	// here is that the reference GC acts on is present and well-formed.
	It("sets a controller owner reference for garbage collection", func() {
		sm, err := getServiceMonitor()
		Expect(err).NotTo(HaveOccurred())

		owners := sm.GetOwnerReferences()
		Expect(owners).To(HaveLen(1))
		Expect(owners[0].Name).To(Equal(instanceName))
		Expect(owners[0].Kind).To(Equal("OpenClawInstance"))
		Expect(owners[0].Controller).NotTo(BeNil())
		Expect(*owners[0].Controller).To(BeTrue())
	})
})
