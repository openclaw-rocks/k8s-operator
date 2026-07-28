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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
	"github.com/paperclipinc/openclaw-operator/internal/resources"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "E2E Suite")
}

var _ = BeforeSuite(func() {
	ctx, cancel = context.WithCancel(context.Background())

	By("bootstrapping test environment")
	var err error
	cfg, err = config.GetConfig()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	err = openclawv1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	cancel()
})

// kubectlExec runs a command inside the openclaw container via kubectl exec.
func kubectlExec(namespace, podName string, command ...string) (string, error) {
	args := []string{"exec", podName, "-n", namespace, "-c", "openclaw", "--"}
	args = append(args, command...)
	cmd := exec.Command("kubectl", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

var _ = Describe("OpenClawInstance Controller", func() {
	const (
		timeout  = time.Second * 60
		interval = time.Second * 1
	)

	Context("When creating an OpenClawInstance", func() {
		var namespace string

		BeforeEach(func() {
			// Create a unique namespace for each test
			namespace = "test-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			// Clean up the namespace
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should create managed resources", func() {
			instanceName := "test-instance"

			// Skip if running in minimal mode (no actual OpenClaw image)
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			// Create OpenClawInstance
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

			// Verify the instance was created
			createdInstance := &openclawv1alpha1.OpenClawInstance{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, createdInstance)
			}, timeout, interval).Should(Succeed())

			// Verify StatefulSet is created
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			// Verify Service is created
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, service)
			}, timeout, interval).Should(Succeed())

			// Verify the StatefulSet has main + gateway-proxy + otel-collector containers
			Expect(statefulSet.Spec.Template.Spec.Containers).To(HaveLen(3))
			Expect(statefulSet.Spec.Template.Spec.Containers[0].Image).To(Equal("ghcr.io/openclaw/openclaw:latest"))

			// Verify handshake timeout env var is injected (workaround for upstream #46892)
			mainContainer := statefulSet.Spec.Template.Spec.Containers[0]
			var foundHandshakeTimeout bool
			for _, env := range mainContainer.Env {
				if env.Name == "OPENCLAW_GATEWAY_HANDSHAKE_TIMEOUT_MS" {
					foundHandshakeTimeout = true
					Expect(env.Value).To(Equal(fmt.Sprintf("%d", resources.DefaultHandshakeTimeoutMs)),
						"handshake timeout should be set to DefaultHandshakeTimeoutMs")
					break
				}
			}
			Expect(foundHandshakeTimeout).To(BeTrue(),
				"OPENCLAW_GATEWAY_HANDSHAKE_TIMEOUT_MS env var should be injected for K8s startup overhead")

			// Clean up
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())

			// Verify the StatefulSet is deleted (due to owner reference)
			Eventually(func() bool {
				err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
				return err != nil
			}, timeout, interval).Should(BeTrue())
		})

		It("Should create gateway proxy sidecar", func() {
			instanceName := "proxy-sidecar-instance"

			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

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

			// Verify StatefulSet has gateway-proxy container
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			var proxyContainer *corev1.Container
			for i := range statefulSet.Spec.Template.Spec.Containers {
				if statefulSet.Spec.Template.Spec.Containers[i].Name == "gateway-proxy" {
					proxyContainer = &statefulSet.Spec.Template.Spec.Containers[i]
					break
				}
			}
			Expect(proxyContainer).NotTo(BeNil(), "StatefulSet should have gateway-proxy container")
			Expect(proxyContainer.Image).To(Equal(resources.DefaultGatewayProxyImage),
				"gateway-proxy should use the default nginx image")

			// Verify Service targetPort points to proxy ports
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, service)
			}, timeout, interval).Should(Succeed())

			for _, port := range service.Spec.Ports {
				if port.Name == "gateway" {
					Expect(port.Port).To(Equal(int32(resources.GatewayPort)),
						"gateway service port should be the original port")
					Expect(port.TargetPort.IntValue()).To(Equal(int(resources.GatewayProxyPort)),
						"gateway targetPort should point to the proxy port")
				}
				if port.Name == "canvas" {
					Expect(port.Port).To(Equal(int32(resources.CanvasPort)),
						"canvas service port should be the original port")
					Expect(port.TargetPort.IntValue()).To(Equal(int(resources.CanvasProxyPort)),
						"canvas targetPort should point to the proxy port")
				}
			}

			// Verify ConfigMap has nginx.conf key
			configMap := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.ConfigMapName(instance),
					Namespace: namespace,
				}, configMap)
			}, timeout, interval).Should(Succeed())

			Expect(configMap.Data).To(HaveKey(resources.NginxConfigKey),
				"ConfigMap should contain nginx.conf key")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should skip gateway proxy sidecar when disabled", func() {
			instanceName := "proxy-disabled-instance"

			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

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
					Gateway: openclawv1alpha1.GatewaySpec{
						Enabled: resources.Ptr(false),
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify StatefulSet has no gateway-proxy container
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			for _, c := range statefulSet.Spec.Template.Spec.Containers {
				Expect(c.Name).NotTo(Equal("gateway-proxy"),
					"StatefulSet should not have gateway-proxy container when disabled")
			}

			// Verify probes target the gateway directly
			mainContainer := statefulSet.Spec.Template.Spec.Containers[0]
			Expect(mainContainer.LivenessProbe.HTTPGet.Port.IntValue()).To(Equal(int(resources.GatewayPort)),
				"liveness probe should target gateway port directly")
			Expect(mainContainer.ReadinessProbe.HTTPGet.Port.IntValue()).To(Equal(int(resources.GatewayPort)),
				"readiness probe should target gateway port directly")

			// Verify Service targetPort points to direct ports
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, service)
			}, timeout, interval).Should(Succeed())

			for _, port := range service.Spec.Ports {
				if port.Name == "gateway" {
					Expect(port.TargetPort.IntValue()).To(Equal(int(resources.GatewayPort)),
						"gateway targetPort should point to direct port")
				}
				if port.Name == "canvas" {
					Expect(port.TargetPort.IntValue()).To(Equal(int(resources.CanvasPort)),
						"canvas targetPort should point to direct port")
				}
			}

			// Verify ConfigMap does not have nginx.conf key
			configMap := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.ConfigMapName(instance),
					Namespace: namespace,
				}, configMap)
			}, timeout, interval).Should(Succeed())

			Expect(configMap.Data).NotTo(HaveKey(resources.NginxConfigKey),
				"ConfigMap should not contain nginx.conf key when proxy is disabled")

			// Verify gateway.bind is 0.0.0.0 in openclaw.json
			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(configMap.Data["openclaw.json"]), &parsed)).Should(Succeed())
			gw, ok := parsed["gateway"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "config should have gateway key")
			Expect(gw["bind"]).To(Equal(resources.GatewayBindAllInterfaces),
				"gateway.bind should be 0.0.0.0 when proxy is disabled")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should use shell-capable image for merge mode init container", func() {
			instanceName := "merge-mode-instance"

			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

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
					Config: openclawv1alpha1.ConfigSpec{
						MergeMode: "merge",
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			// Find init-config container
			var initConfig *corev1.Container
			for i := range statefulSet.Spec.Template.Spec.InitContainers {
				if statefulSet.Spec.Template.Spec.InitContainers[i].Name == "init-config" {
					initConfig = &statefulSet.Spec.Template.Spec.InitContainers[i]
					break
				}
			}
			Expect(initConfig).NotTo(BeNil(), "merge mode should have init-config container")

			// Must use the OpenClaw image (has shell), NOT the distroless jq image
			Expect(initConfig.Image).To(Equal("ghcr.io/openclaw/openclaw:latest"),
				"merge mode init container should use the OpenClaw image (shell-capable)")

			// Command should use node deep merge, not jq
			Expect(initConfig.Command).To(HaveLen(3))
			Expect(initConfig.Command[0]).To(Equal("sh"))
			Expect(initConfig.Command[2]).To(ContainSubstring("node -e"),
				"merge script should use Node.js deep merge")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should use shell-capable uv image for python runtime deps", func() {
			instanceName := "python-deps-instance"

			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

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
					RuntimeDeps: openclawv1alpha1.RuntimeDepsSpec{
						Python: true,
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			// Find init-python container
			var initPython *corev1.Container
			for i := range statefulSet.Spec.Template.Spec.InitContainers {
				if statefulSet.Spec.Template.Spec.InitContainers[i].Name == "init-python" {
					initPython = &statefulSet.Spec.Template.Spec.InitContainers[i]
					break
				}
			}
			Expect(initPython).NotTo(BeNil(), "python runtime deps should have init-python container")

			// Must use bookworm-slim variant (has shell), NOT the distroless base tag
			Expect(initPython.Image).To(Equal(resources.UvImage),
				"init-python should use the shell-capable uv image")
			Expect(initPython.Image).To(ContainSubstring("bookworm-slim"),
				"uv image must be a Debian variant with shell")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should mount default config for vanilla deployment", func() {
			instanceName := "vanilla-instance"

			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			// Create vanilla OpenClawInstance (image only, no config)
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

			// Verify StatefulSet has init-config init container
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			initContainers := statefulSet.Spec.Template.Spec.InitContainers
			var initConfig *corev1.Container
			for i := range initContainers {
				if initContainers[i].Name == "init-config" {
					initConfig = &initContainers[i]
					break
				}
			}
			Expect(initConfig).NotTo(BeNil(), "vanilla deployment should have init-config container")

			// Verify config volume references the operator-managed ConfigMap
			var configVol *corev1.Volume
			for i := range statefulSet.Spec.Template.Spec.Volumes {
				if statefulSet.Spec.Template.Spec.Volumes[i].Name == "config" {
					configVol = &statefulSet.Spec.Template.Spec.Volumes[i]
					break
				}
			}
			Expect(configVol).NotTo(BeNil(), "config volume should exist for vanilla deployment")
			Expect(configVol.ConfigMap).NotTo(BeNil())
			Expect(configVol.ConfigMap.Name).To(Equal(resources.ConfigMapName(instance)))

			// Verify ConfigMap exists and contains gateway.bind=lan
			cm := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.ConfigMapName(instance),
					Namespace: namespace,
				}, cm)
			}, timeout, interval).Should(Succeed())

			configContent, ok := cm.Data["openclaw.json"]
			Expect(ok).To(BeTrue(), "ConfigMap should have openclaw.json key")

			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(configContent), &parsed)).To(Succeed())
			gw, ok := parsed["gateway"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "config should have gateway key")
			Expect(gw["bind"]).To(Equal("loopback"), "gateway.bind should be loopback")

			// Device auth should be disabled (incompatible with K8s)
			controlUI, ok := gw["controlUi"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "gateway should have controlUi key")
			Expect(controlUI["dangerouslyDisableDeviceAuth"]).To(Equal(true),
				"gateway.controlUi.dangerouslyDisableDeviceAuth should be true")

			// Clean up via owner-reference garbage collection
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should apply topology spread constraints", func() {
			instanceName := "tsc-instance"

			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

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
					Availability: openclawv1alpha1.AvailabilitySpec{
						TopologySpreadConstraints: []corev1.TopologySpreadConstraint{
							{
								MaxSkew:           1,
								TopologyKey:       "topology.kubernetes.io/zone",
								WhenUnsatisfiable: corev1.DoNotSchedule,
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"app.kubernetes.io/instance": "tsc-instance",
									},
								},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			Expect(statefulSet.Spec.Template.Spec.TopologySpreadConstraints).To(HaveLen(1))
			tsc := statefulSet.Spec.Template.Spec.TopologySpreadConstraints[0]
			Expect(tsc.TopologyKey).To(Equal("topology.kubernetes.io/zone"))
			Expect(tsc.MaxSkew).To(Equal(int32(1)))
			Expect(tsc.WhenUnsatisfiable).To(Equal(corev1.DoNotSchedule))

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should apply runtimeClassName", func() {
			instanceName := "rtc-instance"

			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

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
					Availability: openclawv1alpha1.AvailabilitySpec{
						RuntimeClassName: resources.Ptr("kata-fc"),
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			Expect(statefulSet.Spec.Template.Spec.RuntimeClassName).ToNot(BeNil())
			Expect(*statefulSet.Spec.Template.Spec.RuntimeClassName).To(Equal("kata-fc"))

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should default shareProcessNamespace to true", func() {
			instanceName := "spn-default"

			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

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

			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			Expect(statefulSet.Spec.Template.Spec.ShareProcessNamespace).ToNot(BeNil(),
				"ShareProcessNamespace should be set so the pause container reaps zombies")
			Expect(*statefulSet.Spec.Template.Spec.ShareProcessNamespace).To(BeTrue(),
				"ShareProcessNamespace should default to true")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should honor shareProcessNamespace=false override", func() {
			instanceName := "spn-disabled"

			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

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
					ShareProcessNamespace: resources.Ptr(false),
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			Expect(statefulSet.Spec.Template.Spec.ShareProcessNamespace).ToNot(BeNil())
			Expect(*statefulSet.Spec.Template.Spec.ShareProcessNamespace).To(BeFalse(),
				"ShareProcessNamespace should be false when explicitly disabled")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})

	Context("When deleting an OpenClawInstance without S3 backup credentials", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-no-s3-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should delete cleanly when S3 backup credentials are not configured", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "no-s3-delete"

			// No S3 secret exists in the namespace or operator namespace
			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      instanceName,
					Namespace: namespace,
				},
				Spec: openclawv1alpha1.OpenClawInstanceSpec{
					Image: openclawv1alpha1.ImageSpec{
						Repository: "ghcr.io/openclaw/openclaw",
						Tag:        "latest",
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			instanceKey := types.NamespacedName{Name: instanceName, Namespace: namespace}

			// Wait for StatefulSet to be created (proves reconciliation happened)
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			// Delete the instance - should succeed without S3 credentials
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())

			// Instance should be fully garbage collected (finalizer removed)
			Eventually(func() bool {
				inst := &openclawv1alpha1.OpenClawInstance{}
				err := k8sClient.Get(ctx, instanceKey, inst)
				return err != nil // NotFound means fully deleted
			}, timeout, interval).Should(BeTrue())
		})
	})

	Context("When creating an OpenClawInstance with Ingress", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-ingress-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should emit only nginx annotations for nginx className", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "ingress-nginx"
			className := "nginx"

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
					Networking: openclawv1alpha1.NetworkingSpec{
						Ingress: openclawv1alpha1.IngressSpec{
							Enabled:   true,
							ClassName: &className,
							Hosts: []openclawv1alpha1.IngressHost{
								{Host: "test.example.com"},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			ingress := &networkingv1.Ingress{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.IngressName(instance),
					Namespace: namespace,
				}, ingress)
			}, timeout, interval).Should(Succeed())

			ann := ingress.Annotations
			Expect(ann).To(HaveKey("nginx.ingress.kubernetes.io/ssl-redirect"))
			Expect(ann).To(HaveKey("nginx.ingress.kubernetes.io/proxy-read-timeout"))
			Expect(ann).NotTo(HaveKey("traefik.ingress.kubernetes.io/router.entrypoints"))
			Expect(ann).NotTo(HaveKey("traefik.ingress.kubernetes.io/router.middlewares"))

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should emit only traefik annotations for traefik className", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "ingress-traefik"
			className := "traefik"

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
					Networking: openclawv1alpha1.NetworkingSpec{
						Ingress: openclawv1alpha1.IngressSpec{
							Enabled:   true,
							ClassName: &className,
							Hosts: []openclawv1alpha1.IngressHost{
								{Host: "test.example.com"},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			ingress := &networkingv1.Ingress{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.IngressName(instance),
					Namespace: namespace,
				}, ingress)
			}, timeout, interval).Should(Succeed())

			ann := ingress.Annotations
			Expect(ann).To(HaveKeyWithValue("traefik.ingress.kubernetes.io/router.entrypoints", "websecure"))
			Expect(ann).NotTo(HaveKey("nginx.ingress.kubernetes.io/ssl-redirect"))
			Expect(ann).NotTo(HaveKey("nginx.ingress.kubernetes.io/proxy-read-timeout"))
			Expect(ann).NotTo(HaveKey("traefik.ingress.kubernetes.io/router.middlewares"))

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should emit no provider-specific annotations when className is nil", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "ingress-nil-class"

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
					Networking: openclawv1alpha1.NetworkingSpec{
						Ingress: openclawv1alpha1.IngressSpec{
							Enabled: true,
							// ClassName intentionally nil
							Hosts: []openclawv1alpha1.IngressHost{
								{Host: "test.example.com"},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			ingress := &networkingv1.Ingress{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.IngressName(instance),
					Namespace: namespace,
				}, ingress)
			}, timeout, interval).Should(Succeed())

			ann := ingress.Annotations
			// No provider-specific annotations for nil className
			Expect(ann).NotTo(HaveKey("nginx.ingress.kubernetes.io/ssl-redirect"))
			Expect(ann).NotTo(HaveKey("nginx.ingress.kubernetes.io/proxy-read-timeout"))
			Expect(ann).NotTo(HaveKey("traefik.ingress.kubernetes.io/router.entrypoints"))
			Expect(ann).NotTo(HaveKey("traefik.ingress.kubernetes.io/router.middlewares"))

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})

	Context("When creating an OpenClawInstance with custom service ports (#144)", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-svcports-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should create a Service with custom ports replacing defaults", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "custom-ports"

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
					Networking: openclawv1alpha1.NetworkingSpec{
						Service: openclawv1alpha1.ServiceSpec{
							Ports: []openclawv1alpha1.ServicePortSpec{
								{Name: "http", Port: 3978},
								{Name: "grpc", Port: 50051, TargetPort: resources.Ptr(int32(50051))},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify Service has custom ports
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, service)
			}, timeout, interval).Should(Succeed())

			// Two custom ports plus the operator-managed metrics port, which is
			// enabled by default and must stay present so the ServiceMonitor
			// endpoint resolves (#577).
			Expect(service.Spec.Ports).To(HaveLen(3))

			var foundHTTP, foundGRPC, foundMetrics bool
			for _, p := range service.Spec.Ports {
				if p.Name == "http" && p.Port == 3978 {
					foundHTTP = true
					Expect(p.TargetPort.IntValue()).To(Equal(3978))
				}
				if p.Name == "grpc" && p.Port == 50051 {
					foundGRPC = true
					Expect(p.TargetPort.IntValue()).To(Equal(50051))
				}
				if p.Name == resources.MetricsPortName {
					foundMetrics = true
				}
				// The workload defaults must still be replaced by the custom ports.
				Expect(p.Name).NotTo(BeElementOf("gateway", "canvas", "chromium"))
			}
			Expect(foundMetrics).To(BeTrue(), "managed metrics port must survive custom ports")
			Expect(foundHTTP).To(BeTrue(), "Service should have http port 3978")
			Expect(foundGRPC).To(BeTrue(), "Service should have grpc port 50051")

			// Default gateway/canvas ports should NOT be present
			for _, p := range service.Spec.Ports {
				Expect(p.Port).NotTo(Equal(int32(resources.GatewayPort)),
					"custom ports should replace default gateway port")
				Expect(p.Port).NotTo(Equal(int32(resources.CanvasPort)),
					"custom ports should replace default canvas port")
			}

			// Verify NetworkPolicy allows custom ports
			np := &networkingv1.NetworkPolicy{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.NetworkPolicyName(instance),
					Namespace: namespace,
				}, np)
			}, timeout, interval).Should(Succeed())

			var foundNP3978, foundNP50051 bool
			for _, rule := range np.Spec.Ingress {
				for _, p := range rule.Ports {
					if p.Port != nil {
						switch p.Port.IntValue() {
						case 3978:
							foundNP3978 = true
						case 50051:
							foundNP50051 = true
						}
					}
				}
			}
			Expect(foundNP3978).To(BeTrue(), "NetworkPolicy should allow port 3978")
			Expect(foundNP50051).To(BeTrue(), "NetworkPolicy should allow port 50051")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should set custom backend port on Ingress paths", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "custom-ingress-port"
			className := "nginx"

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
					Networking: openclawv1alpha1.NetworkingSpec{
						Service: openclawv1alpha1.ServiceSpec{
							Ports: []openclawv1alpha1.ServicePortSpec{
								{Name: "http", Port: 3978},
							},
						},
						Ingress: openclawv1alpha1.IngressSpec{
							Enabled:   true,
							ClassName: &className,
							Hosts: []openclawv1alpha1.IngressHost{
								{
									Host: "aibot.example.com",
									Paths: []openclawv1alpha1.IngressPath{
										{
											Path:     "/api/messages",
											PathType: "Prefix",
											Port:     resources.Ptr(int32(3978)),
										},
									},
								},
							},
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			ingress := &networkingv1.Ingress{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.IngressName(instance),
					Namespace: namespace,
				}, ingress)
			}, timeout, interval).Should(Succeed())

			Expect(ingress.Spec.Rules).To(HaveLen(1))
			paths := ingress.Spec.Rules[0].HTTP.Paths
			Expect(paths).To(HaveLen(1))
			Expect(paths[0].Path).To(Equal("/api/messages"))
			Expect(paths[0].Backend.Service.Port.Number).To(Equal(int32(3978)),
				"Ingress backend should route to custom port 3978")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})

	Context("When creating an instance with Tailscale enabled", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-ts-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should create Tailscale sidecar, init container, serve config, and NetworkPolicy egress", func() {
			instanceName := "ts-e2e-instance"

			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			// Create auth key Secret
			tsSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ts-auth",
					Namespace: namespace,
				},
				StringData: map[string]string{
					"authkey": "tskey-auth-test-XXXXX",
				},
			}
			Expect(k8sClient.Create(ctx, tsSecret)).Should(Succeed())

			// Create instance with Tailscale enabled
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
					Tailscale: openclawv1alpha1.TailscaleSpec{
						Enabled: true,
						Mode:    "serve",
						AuthKeySecretRef: &corev1.LocalObjectReference{
							Name: "ts-auth",
						},
						AuthSSO: true,
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify ConfigMap
			cm := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.ConfigMapName(instance),
					Namespace: namespace,
				}, cm)
			}, timeout, interval).Should(Succeed())

			// Verify tailscale-serve.json key exists
			serveJSON, ok := cm.Data[resources.TailscaleServeConfigKey]
			Expect(ok).To(BeTrue(), "ConfigMap should have tailscale-serve.json key")
			var serveCfg map[string]interface{}
			Expect(json.Unmarshal([]byte(serveJSON), &serveCfg)).To(Succeed())
			Expect(serveCfg).To(HaveKey("TCP"), "serve config should have TCP key")

			// Verify config does NOT have gateway.tailscale.mode/resetOnExit (sidecar handles it)
			configContent, ok := cm.Data["openclaw.json"]
			Expect(ok).To(BeTrue(), "ConfigMap should have openclaw.json key")
			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(configContent), &parsed)).To(Succeed())
			gw, ok := parsed["gateway"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "config should have gateway key")
			_, hasTailscaleKey := gw["tailscale"]
			Expect(hasTailscaleKey).To(BeFalse(), "gateway.tailscale should NOT be set - sidecar handles serve/funnel")

			// Verify AuthSSO sets gateway.auth.allowTailscale
			auth, ok := gw["auth"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "gateway should have auth key when AuthSSO is enabled")
			Expect(auth["allowTailscale"]).To(BeTrue(), "auth.allowTailscale should be true")

			// Verify gateway.bind=loopback
			Expect(gw["bind"]).To(Equal("loopback"), "gateway.bind should be loopback")

			// Verify StatefulSet
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			// Verify tailscale sidecar container exists with correct env vars
			var tsSidecar *corev1.Container
			for i := range statefulSet.Spec.Template.Spec.Containers {
				if statefulSet.Spec.Template.Spec.Containers[i].Name == "tailscale" {
					tsSidecar = &statefulSet.Spec.Template.Spec.Containers[i]
					break
				}
			}
			Expect(tsSidecar).NotTo(BeNil(), "tailscale sidecar container should be present")

			var foundSidecarAuthKey, foundSidecarHostname bool
			for _, env := range tsSidecar.Env {
				if env.Name == "TS_AUTHKEY" {
					foundSidecarAuthKey = true
					Expect(env.ValueFrom).NotTo(BeNil(), "TS_AUTHKEY should use ValueFrom")
					Expect(env.ValueFrom.SecretKeyRef).NotTo(BeNil(), "TS_AUTHKEY should use SecretKeyRef")
					Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal("ts-auth"))
					Expect(env.ValueFrom.SecretKeyRef.Key).To(Equal("authkey"))
				}
				if env.Name == "TS_HOSTNAME" {
					foundSidecarHostname = true
					Expect(env.Value).To(Equal(instanceName), "TS_HOSTNAME should default to instance name")
				}
			}
			Expect(foundSidecarAuthKey).To(BeTrue(), "TS_AUTHKEY should be on sidecar")
			Expect(foundSidecarHostname).To(BeTrue(), "TS_HOSTNAME should be on sidecar")

			// Verify main container does NOT have TS_AUTHKEY/TS_HOSTNAME, but has TS_SOCKET and PATH
			mainContainer := statefulSet.Spec.Template.Spec.Containers[0]
			var foundTSSocket, foundPATH bool
			for _, env := range mainContainer.Env {
				Expect(env.Name).NotTo(Equal("TS_AUTHKEY"), "TS_AUTHKEY should NOT be on main container")
				Expect(env.Name).NotTo(Equal("TS_HOSTNAME"), "TS_HOSTNAME should NOT be on main container")
				if env.Name == "TS_SOCKET" {
					foundTSSocket = true
					Expect(env.Value).To(Equal(resources.TailscaleSocketPath))
				}
				if env.Name == "PATH" {
					foundPATH = true
					Expect(env.Value).To(ContainSubstring(resources.TailscaleBinPath))
				}
			}
			Expect(foundTSSocket).To(BeTrue(), "TS_SOCKET should be on main container")
			Expect(foundPATH).To(BeTrue(), "PATH with tailscale-bin should be on main container")

			// Verify init-tailscale-bin init container exists
			var foundTSInit bool
			for _, c := range statefulSet.Spec.Template.Spec.InitContainers {
				if c.Name == "init-tailscale-bin" {
					foundTSInit = true
					break
				}
			}
			Expect(foundTSInit).To(BeTrue(), "init-tailscale-bin init container should be present")

			// Verify probes use HTTPGet via the nginx proxy sidecar
			Expect(mainContainer.LivenessProbe).NotTo(BeNil(), "liveness probe should be set")
			Expect(mainContainer.LivenessProbe.HTTPGet).NotTo(BeNil(), "liveness probe should use HTTPGet")
			Expect(mainContainer.LivenessProbe.HTTPGet.Path).To(Equal("/healthz"))
			Expect(mainContainer.LivenessProbe.HTTPGet.Port.IntValue()).To(Equal(int(resources.GatewayProxyPort)))
			Expect(mainContainer.ReadinessProbe).NotTo(BeNil(), "readiness probe should be set")
			Expect(mainContainer.ReadinessProbe.HTTPGet).NotTo(BeNil(), "readiness probe should use HTTPGet")
			Expect(mainContainer.ReadinessProbe.HTTPGet.Path).To(Equal("/readyz"))
			Expect(mainContainer.ReadinessProbe.HTTPGet.Port.IntValue()).To(Equal(int(resources.GatewayProxyPort)))
			Expect(mainContainer.StartupProbe).NotTo(BeNil(), "startup probe should be set")
			Expect(mainContainer.StartupProbe.HTTPGet).NotTo(BeNil(), "startup probe should use HTTPGet")
			Expect(mainContainer.StartupProbe.HTTPGet.Path).To(Equal("/healthz"))

			// Verify NetworkPolicy has STUN and WireGuard egress
			np := &networkingv1.NetworkPolicy{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.NetworkPolicyName(instance),
					Namespace: namespace,
				}, np)
			}, timeout, interval).Should(Succeed())

			var foundSTUN, foundWG bool
			for _, rule := range np.Spec.Egress {
				for _, p := range rule.Ports {
					if p.Protocol != nil && *p.Protocol == corev1.ProtocolUDP && p.Port != nil {
						switch p.Port.IntValue() {
						case 3478:
							foundSTUN = true
						case 41641:
							foundWG = true
						}
					}
				}
			}
			Expect(foundSTUN).To(BeTrue(), "NetworkPolicy should have STUN egress (UDP 3478)")
			Expect(foundWG).To(BeTrue(), "NetworkPolicy should have WireGuard egress (UDP 41641)")

			// Verify Tailscale state Secret is created
			tsStateSecret := &corev1.Secret{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.TailscaleStateSecretName(instance),
					Namespace: namespace,
				}, tsStateSecret)
			}, timeout, interval).Should(Succeed())

			// Verify TS_KUBE_SECRET env var points to the state Secret
			var foundTSKubeSecret bool
			for _, env := range tsSidecar.Env {
				if env.Name == "TS_KUBE_SECRET" {
					foundTSKubeSecret = true
					Expect(env.Value).To(Equal(resources.TailscaleStateSecretName(instance)),
						"TS_KUBE_SECRET should point to the state Secret")
				}
			}
			Expect(foundTSKubeSecret).To(BeTrue(), "TS_KUBE_SECRET should be set on sidecar")

			// Verify KUBERNETES_SERVICE_HOST is NOT set (containerboot needs kube API access)
			for _, env := range tsSidecar.Env {
				Expect(env.Name).NotTo(Equal("KUBERNETES_SERVICE_HOST"),
					"KUBERNETES_SERVICE_HOST should not be set when using TS_KUBE_SECRET")
			}

			// Verify AutomountServiceAccountToken is enabled
			Expect(statefulSet.Spec.Template.Spec.AutomountServiceAccountToken).NotTo(BeNil())
			Expect(*statefulSet.Spec.Template.Spec.AutomountServiceAccountToken).To(BeTrue(),
				"AutomountServiceAccountToken should be true when Tailscale is enabled")

			// Verify Role includes Tailscale state Secret rule
			role := &rbacv1.Role{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.RoleName(instance),
					Namespace: namespace,
				}, role)
			}, timeout, interval).Should(Succeed())

			var foundTSSecretRule bool
			for _, rule := range role.Rules {
				for _, res := range rule.Resources {
					if res == "secrets" {
						for _, name := range rule.ResourceNames {
							if name == resources.TailscaleStateSecretName(instance) {
								foundTSSecretRule = true
							}
						}
					}
				}
			}
			Expect(foundTSSecretRule).To(BeTrue(), "Role should include Tailscale state Secret rule")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})

	Context("When creating an OpenClawInstance with Ollama", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-ollama-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should create Ollama sidecar when enabled", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "ollama-test"

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
					Ollama: openclawv1alpha1.OllamaSpec{
						Enabled: true,
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify StatefulSet has ollama sidecar container
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			// Verify ollama container exists
			var ollamaContainer *corev1.Container
			for i := range statefulSet.Spec.Template.Spec.Containers {
				if statefulSet.Spec.Template.Spec.Containers[i].Name == "ollama" {
					ollamaContainer = &statefulSet.Spec.Template.Spec.Containers[i]
					break
				}
			}
			Expect(ollamaContainer).NotTo(BeNil(), "ollama sidecar container should exist")
			Expect(ollamaContainer.Image).To(Equal("docker.io/ollama/ollama:latest"))

			// Verify ollama-models volume exists
			var ollamaVol *corev1.Volume
			for i := range statefulSet.Spec.Template.Spec.Volumes {
				if statefulSet.Spec.Template.Spec.Volumes[i].Name == "ollama-models" {
					ollamaVol = &statefulSet.Spec.Template.Spec.Volumes[i]
					break
				}
			}
			Expect(ollamaVol).NotTo(BeNil(), "ollama-models volume should exist")

			// Verify main container has OLLAMA_HOST env var
			mainContainer := statefulSet.Spec.Template.Spec.Containers[0]
			var foundOllamaHost bool
			for _, env := range mainContainer.Env {
				if env.Name == "OLLAMA_HOST" {
					foundOllamaHost = true
					Expect(env.Value).To(Equal("http://localhost:11434"))
					break
				}
			}
			Expect(foundOllamaHost).To(BeTrue(), "OLLAMA_HOST env var should be set")

			// No init-ollama since no models specified
			for _, ic := range statefulSet.Spec.Template.Spec.InitContainers {
				Expect(ic.Name).NotTo(Equal("init-ollama"), "init-ollama should not be present without models")
			}

			// Clean up
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should create chromium sidecar running Chrome directly on port 9222", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "chromium-test"

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
					Chromium: openclawv1alpha1.ChromiumSpec{
						Enabled: true,
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify StatefulSet has chromium sidecar container
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			// Verify chromium container exists as native sidecar init container
			var chromiumContainer *corev1.Container
			for i := range statefulSet.Spec.Template.Spec.InitContainers {
				if statefulSet.Spec.Template.Spec.InitContainers[i].Name == "chromium" {
					chromiumContainer = &statefulSet.Spec.Template.Spec.InitContainers[i]
					break
				}
			}
			Expect(chromiumContainer).NotTo(BeNil(), "chromium sidecar init container should exist")
			Expect(chromiumContainer.Image).To(Equal(resources.DefaultChromiumImage + ":" + resources.DefaultChromiumTag))
			Expect(chromiumContainer.Ports).To(HaveLen(1))
			Expect(chromiumContainer.Ports[0].ContainerPort).To(Equal(int32(resources.ChromiumPort)))

			// Command must be the entrypoint wrapper that fixes unquoted $@
			// in upstream run.sh, preventing word-splitting of args with spaces (#396).
			Expect(chromiumContainer.Command).To(Equal(resources.ChromiumEntrypointCommand),
				"chromium Command must be the entrypoint wrapper with quoted \"$@\" (#396)")

			// Args should contain launch flags passed to run.sh
			Expect(chromiumContainer.Args).NotTo(BeEmpty(), "chromium should have Args with launch flags")
			argsStr := strings.Join(chromiumContainer.Args, " ")
			Expect(argsStr).To(ContainSubstring("--no-sandbox"))

			// No chromium-proxy container should exist
			for _, c := range statefulSet.Spec.Template.Spec.InitContainers {
				Expect(c.Name).NotTo(Equal("chromium-proxy"),
					"chromium-proxy should not exist - Chrome runs via run.sh")
			}

			// Verify main container has OPENCLAW_CHROMIUM_CDP using localhost
			mainContainer := statefulSet.Spec.Template.Spec.Containers[0]
			var foundChromiumCDP bool
			expectedCDP := fmt.Sprintf("http://127.0.0.1:%d", resources.ChromiumPort)
			for _, env := range mainContainer.Env {
				if env.Name == "OPENCLAW_CHROMIUM_CDP" {
					foundChromiumCDP = true
					Expect(env.Value).To(Equal(expectedCDP),
						"OPENCLAW_CHROMIUM_CDP should use localhost (sidecar shares pod network)")
				}
			}
			Expect(foundChromiumCDP).To(BeTrue(), "OPENCLAW_CHROMIUM_CDP env var should be set")

			// Verify ConfigMap has browser profiles with cdpUrl
			configMap := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.ConfigMapName(instance),
					Namespace: namespace,
				}, configMap)
			}, timeout, interval).Should(Succeed())

			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(configMap.Data["openclaw.json"]), &parsed)).Should(Succeed())

			browser, ok := parsed["browser"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "config should have browser key")
			Expect(browser["attachOnly"]).To(BeTrue(), "browser.attachOnly should be true")
			Expect(browser["remoteCdpTimeoutMs"]).To(BeNumerically("==", 30000),
				"browser.remoteCdpTimeoutMs should be 30000")

			profiles, ok := browser["profiles"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "browser should have profiles key")

			expectedCDPURL := "${OPENCLAW_CHROMIUM_CDP}"
			for _, profileName := range []string{"default", "chrome"} {
				profile, ok := profiles[profileName].(map[string]interface{})
				Expect(ok).To(BeTrue(), "profiles should have %s key", profileName)
				Expect(profile["cdpUrl"]).To(Equal(expectedCDPURL),
					"browser.profiles.%s.cdpUrl should use env var reference", profileName)
			}

			// Verify Service has chromium port
			service := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, service)
			}, timeout, interval).Should(Succeed())

			var foundChromiumPort bool
			for _, port := range service.Spec.Ports {
				if port.Name == "chromium" {
					foundChromiumPort = true
					Expect(port.Port).To(Equal(int32(resources.ChromiumPort)))
					break
				}
			}
			Expect(foundChromiumPort).To(BeTrue(), "Service should have chromium port")

			// ConfigMap should NOT contain chromium proxy config (no proxy needed)
			_, hasProxyConfig := configMap.Data["chromium-proxy-nginx.conf"]
			Expect(hasProxyConfig).To(BeFalse(),
				"ConfigMap should not contain chromium proxy config - Chrome runs directly")

			// Clean up
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		// Regression test for #396: instances created before v0.22.1 have the
		// deprecated ghcr.io/browserless/chromium image stored via kubebuilder
		// defaults. The operator must normalize it to docker.io/chromedp/headless-shell
		// and must NOT set a Command override (which bypasses run.sh and breaks
		// CDP on Chrome M136+).
		It("Should migrate deprecated browserless image to current default", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "chromium-migrate-test"

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
					Chromium: openclawv1alpha1.ChromiumSpec{
						Enabled: true,
						Image: openclawv1alpha1.ChromiumImageSpec{
							// Simulate a pre-v0.22.1 instance with old kubebuilder defaults
							Repository: resources.DeprecatedChromiumImage,
							Tag:        "latest",
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			// Find chromium init container
			var chromiumContainer *corev1.Container
			for i := range statefulSet.Spec.Template.Spec.InitContainers {
				if statefulSet.Spec.Template.Spec.InitContainers[i].Name == "chromium" {
					chromiumContainer = &statefulSet.Spec.Template.Spec.InitContainers[i]
					break
				}
			}
			Expect(chromiumContainer).NotTo(BeNil(), "chromium sidecar should exist")

			// Image must be normalized to current default
			expectedImage := resources.DefaultChromiumImage + ":" + resources.DefaultChromiumTag
			Expect(chromiumContainer.Image).To(Equal(expectedImage),
				"deprecated browserless image should be migrated to %s", expectedImage)

			// Command must be the entrypoint wrapper with quoted "$@" (#396)
			Expect(chromiumContainer.Command).To(Equal(resources.ChromiumEntrypointCommand),
				"chromium Command must be the entrypoint wrapper with quoted \"$@\" (#396)")

			// Args should still contain launch flags
			Expect(chromiumContainer.Args).NotTo(BeEmpty(),
				"chromium should have Args with launch flags")
			argsStr := strings.Join(chromiumContainer.Args, " ")
			Expect(argsStr).To(ContainSubstring("--no-sandbox"))

			// Clean up
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should create persistent chromium volume when persistence is enabled", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "chromium-persist-test"

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
					Chromium: openclawv1alpha1.ChromiumSpec{
						Enabled: true,
						Persistence: openclawv1alpha1.ChromiumPersistenceSpec{
							Enabled: true,
							Size:    "1Gi",
						},
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify StatefulSet is created
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			// Verify chromium-data volume references a PVC
			var dataVol *corev1.Volume
			for i := range statefulSet.Spec.Template.Spec.Volumes {
				if statefulSet.Spec.Template.Spec.Volumes[i].Name == "chromium-data" {
					dataVol = &statefulSet.Spec.Template.Spec.Volumes[i]
					break
				}
			}
			Expect(dataVol).NotTo(BeNil(), "chromium-data volume should exist")
			Expect(dataVol.PersistentVolumeClaim).NotTo(BeNil(), "chromium-data should be a PVC when persistence is enabled")
			Expect(dataVol.PersistentVolumeClaim.ClaimName).To(Equal(instanceName + "-chromium-data"))

			// Verify chromium container has --user-data-dir in Args for persistence
			var chromiumContainer *corev1.Container
			for i := range statefulSet.Spec.Template.Spec.InitContainers {
				if statefulSet.Spec.Template.Spec.InitContainers[i].Name == "chromium" {
					chromiumContainer = &statefulSet.Spec.Template.Spec.InitContainers[i]
					break
				}
			}
			Expect(chromiumContainer).NotTo(BeNil(), "chromium sidecar init container should exist")

			argsStr := strings.Join(chromiumContainer.Args, " ")
			Expect(argsStr).To(ContainSubstring("--user-data-dir=/chromium-data"),
				"persistence should add --user-data-dir=/chromium-data to Args")

			// Verify chromium container has chromium-data volume mount
			var foundDataMount bool
			for _, mount := range chromiumContainer.VolumeMounts {
				if mount.Name == "chromium-data" && mount.MountPath == "/chromium-data" {
					foundDataMount = true
					break
				}
			}
			Expect(foundDataMount).To(BeTrue(), "chromium container should mount chromium-data at /chromium-data")

			// Verify Chromium PVC was created
			pvc := &corev1.PersistentVolumeClaim{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName + "-chromium-data",
					Namespace: namespace,
				}, pvc)
			}, timeout, interval).Should(Succeed())
			Expect(pvc.Spec.Resources.Requests[corev1.ResourceStorage]).To(Equal(resource.MustParse("1Gi")))

			// Clean up
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})

	Context("When creating an OpenClawInstance with WebTerminal", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-webterminal-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: namespace,
				},
			}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should create web terminal sidecar when enabled", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "web-terminal-test"

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
					WebTerminal: openclawv1alpha1.WebTerminalSpec{
						Enabled: true,
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify StatefulSet has web-terminal sidecar container
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			// Verify web-terminal container exists
			var wtContainer *corev1.Container
			for i := range statefulSet.Spec.Template.Spec.Containers {
				if statefulSet.Spec.Template.Spec.Containers[i].Name == "web-terminal" {
					wtContainer = &statefulSet.Spec.Template.Spec.Containers[i]
					break
				}
			}
			Expect(wtContainer).NotTo(BeNil(), "web-terminal sidecar container should exist")
			Expect(wtContainer.Image).To(Equal("tsl0922/ttyd:latest"))

			// Verify web-terminal-tmp volume exists
			var wtVol *corev1.Volume
			for i := range statefulSet.Spec.Template.Spec.Volumes {
				if statefulSet.Spec.Template.Spec.Volumes[i].Name == "web-terminal-tmp" {
					wtVol = &statefulSet.Spec.Template.Spec.Volumes[i]
					break
				}
			}
			Expect(wtVol).NotTo(BeNil(), "web-terminal-tmp volume should exist")

			// Verify Service has web-terminal port
			svc := &corev1.Service{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, svc)
			}, timeout, interval).Should(Succeed())

			var foundWTPort bool
			for _, port := range svc.Spec.Ports {
				if port.Name == "web-terminal" && port.Port == int32(resources.WebTerminalPort) {
					foundWTPort = true
					break
				}
			}
			Expect(foundWTPort).To(BeTrue(), "Service should have web-terminal port")

			// Clean up
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})

	Context("When creating an instance with npm-prefixed skills (issue #131)", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-npm-skills-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should produce an init-skills container with npm install for npm: prefixed skills", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "npm-skills-test"

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
					Skills: []string{"npm:@openclaw/matrix", "@anthropic/mcp-server-fetch"},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.StatefulSetName(instance),
					Namespace: namespace,
				}, statefulSet)
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			var skillsContainer *corev1.Container
			for i := range statefulSet.Spec.Template.Spec.InitContainers {
				if statefulSet.Spec.Template.Spec.InitContainers[i].Name == "init-skills" {
					skillsContainer = &statefulSet.Spec.Template.Spec.InitContainers[i]
					break
				}
			}
			Expect(skillsContainer).NotTo(BeNil(), "init-skills container should exist")

			// Script should contain npm install for npm: prefixed skill
			script := skillsContainer.Command[2]
			Expect(script).To(ContainSubstring("npm install -g '@openclaw/matrix'"),
				"npm: prefixed skill should use global npm install")
			// Script should also contain clawhub for non-prefixed skill
			Expect(script).To(ContainSubstring("_install_skill '@anthropic/mcp-server-fetch'"),
				"non-prefixed skill should use _install_skill wrapper with the owner-qualified ref preserved (#558)")
			// Script should redirect /app/skills to PVC (#313)
			Expect(script).To(ContainSubstring("mkdir -p /home/openclaw/.openclaw/skills"),
				"script should create PVC-backed skills directory")
			Expect(script).To(ContainSubstring("ln -s /home/openclaw/.openclaw/skills /app/skills"),
				"script should symlink /app/skills to PVC")

			// NPM_CONFIG_IGNORE_SCRIPTS should be set (#91)
			envMap := map[string]string{}
			for _, e := range skillsContainer.Env {
				envMap[e.Name] = e.Value
			}
			Expect(envMap["NPM_CONFIG_IGNORE_SCRIPTS"]).To(Equal("true"),
				"NPM_CONFIG_IGNORE_SCRIPTS should be true for supply chain security")

			// Main container should mount PVC at /app/skills (#313)
			var mainContainer *corev1.Container
			for i := range statefulSet.Spec.Template.Spec.Containers {
				if statefulSet.Spec.Template.Spec.Containers[i].Name == "openclaw" {
					mainContainer = &statefulSet.Spec.Template.Spec.Containers[i]
					break
				}
			}
			Expect(mainContainer).NotTo(BeNil(), "main container should exist")
			var hasSkillsMount bool
			for _, m := range mainContainer.VolumeMounts {
				if m.MountPath == "/app/skills" && m.Name == "data" && m.SubPath == "skills" {
					hasSkillsMount = true
					break
				}
			}
			Expect(hasSkillsMount).To(BeTrue(),
				"main container should mount PVC subpath 'skills' at /app/skills")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})

	Context("When creating an instance with plugins (issue #372)", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-plugins-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should produce an init-plugins container that installs via the openclaw CLI", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "plugins-test"

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
					// `@openclaw/matrix` is the first-party plugin whose npm
					// tarball declares `workspace:*` deps and triggered #505.
					// Raw `npm install` rejects it with EUNSUPPORTEDPROTOCOL;
					// going through the openclaw CLI's clawhub installer
					// resolves it correctly.
					Plugins: []string{"@openclaw/matrix", "@martian-engineering/lossless-claw"},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.StatefulSetName(instance),
					Namespace: namespace,
				}, statefulSet)
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			var pluginsContainer *corev1.Container
			for i := range statefulSet.Spec.Template.Spec.InitContainers {
				if statefulSet.Spec.Template.Spec.InitContainers[i].Name == "init-plugins" {
					pluginsContainer = &statefulSet.Spec.Template.Spec.InitContainers[i]
					break
				}
			}
			Expect(pluginsContainer).NotTo(BeNil(), "init-plugins container should exist")

			script := pluginsContainer.Command[2]
			Expect(script).To(ContainSubstring("mkdir -p /home/openclaw/.openclaw/extensions"),
				"init-plugins must ensure the extensions directory exists")
			Expect(script).To(ContainSubstring("openclaw plugins install --force 'clawhub:@openclaw/matrix'"),
				"matrix must be installed via the openclaw CLI (issue #505)")
			Expect(script).To(ContainSubstring("openclaw plugins install --force 'clawhub:@martian-engineering/lossless-claw'"),
				"third-party plugins must also go through the openclaw CLI")
			// Regression guards: previous layouts that fail for workspace:*-publishing plugins.
			Expect(script).NotTo(ContainSubstring("cd /home/openclaw/.openclaw && npm install"),
				"plugins must not be installed into ~/.openclaw/node_modules (issue #474)")
			Expect(script).NotTo(ContainSubstring("npm pack"),
				"plugins must not be installed via npm pack + npm install (issue #505)")

			// Env vars must mirror the user's verified workaround in #505 so the
			// CLI's data and npm state land on the PVC.
			envMap := map[string]string{}
			for _, e := range pluginsContainer.Env {
				envMap[e.Name] = e.Value
			}
			Expect(envMap["HOME"]).To(Equal("/home/openclaw"),
				"HOME must point at the openclaw user dir so the CLI finds its data dir")
			Expect(envMap["NPM_CONFIG_PREFIX"]).To(Equal("/home/openclaw/.local"))
			Expect(envMap["NPM_CONFIG_CACHE"]).To(Equal("/home/openclaw/.cache/npm"))
			Expect(envMap["NPM_CONFIG_IGNORE_SCRIPTS"]).To(Equal("true"),
				"NPM_CONFIG_IGNORE_SCRIPTS should be true for supply chain security")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})

	Context("When validating postStart config restoration (issue #125)", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-poststart-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should restore config via postStart hook after container restart", func() {
			instanceName := "poststart-e2e"
			podName := instanceName + "-0"

			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping postStart validation in minimal mode")
			}

			// Disable all probes so the pod stays Running regardless of
			// whether OpenClaw can fully start without API keys.
			falseVal := false
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
					Probes: &openclawv1alpha1.ProbesSpec{
						Liveness:  &openclawv1alpha1.ProbeSpec{Enabled: &falseVal},
						Readiness: &openclawv1alpha1.ProbeSpec{Enabled: &falseVal},
						Startup:   &openclawv1alpha1.ProbeSpec{Enabled: &falseVal},
					},
				},
			}

			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Wait for the openclaw container to be Running.
			// K8s does not set Running until the postStart hook completes,
			// so the config file is guaranteed to exist by this point.
			Eventually(func() bool {
				pod := &corev1.Pod{}
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name: podName, Namespace: namespace,
				}, pod); err != nil {
					return false
				}
				for _, cs := range pod.Status.ContainerStatuses {
					if cs.Name == "openclaw" && cs.State.Running != nil {
						return true
					}
				}
				return false
			}, 5*time.Minute, 5*time.Second).Should(BeTrue(),
				"openclaw container should be Running")

			// Verify config was written by the postStart hook.
			// Use Eventually because kubectl exec may briefly fail even after
			// the container status shows Running.
			var out string
			var err error
			Eventually(func(g Gomega) {
				out, err = kubectlExec(namespace, podName,
					"cat", "/home/openclaw/.openclaw/openclaw.json")
				g.Expect(err).NotTo(HaveOccurred(), "should read config file: %s", out)
				g.Expect(out).To(ContainSubstring(`"loopback"`),
					"config should contain gateway.bind=loopback from operator enrichment")
			}, 1*time.Minute, 2*time.Second).Should(Succeed())

			// Corrupt the config file on the PVC
			_, err = kubectlExec(namespace, podName,
				"sh", "-c", `echo '{"corrupted":true}' > /home/openclaw/.openclaw/openclaw.json`)
			Expect(err).NotTo(HaveOccurred(), "should be able to write to PVC")

			// Verify config is corrupted
			out, err = kubectlExec(namespace, podName,
				"cat", "/home/openclaw/.openclaw/openclaw.json")
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(ContainSubstring("corrupted"),
				"config should contain corrupted content")

			// Record current restart count
			pod := &corev1.Pod{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name: podName, Namespace: namespace,
			}, pod)).To(Succeed())
			var prevRestarts int32
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.Name == "openclaw" {
					prevRestarts = cs.RestartCount
					break
				}
			}

			// Crash the main process to trigger a container restart (not pod
			// recreation). Init containers do NOT re-run on container restarts -
			// only the postStart lifecycle hook runs again.
			//
			// Linux protects PID 1 from SIGKILL sent within the same PID
			// namespace, so "kill -9 1" silently fails. Instead we:
			// 1. Try to find PID 1's child (works when tini/dumb-init wraps the app)
			//    and SIGKILL the child (not PID 1, so kernel allows it).
			// 2. Fall back to SIGTERM to PID 1 (delivered if the app registered a
			//    handler, which Node.js/libuv does).
			_, _ = kubectlExec(namespace, podName, "sh", "-c",
				`cpid=$(cat /proc/1/task/*/children 2>/dev/null | tr ' ' '\n' | head -1); `+
					`[ -n "$cpid" ] && kill -9 "$cpid" || kill 1`)

			// Wait for restart count to increase
			Eventually(func() int32 {
				p := &corev1.Pod{}
				if getErr := k8sClient.Get(ctx, types.NamespacedName{
					Name: podName, Namespace: namespace,
				}, p); getErr != nil {
					return -1
				}
				for _, cs := range p.Status.ContainerStatuses {
					if cs.Name == "openclaw" {
						return cs.RestartCount
					}
				}
				return -1
			}, 2*time.Minute, 2*time.Second).Should(BeNumerically(">", prevRestarts),
				"restart count should increase after killing main process")

			// Wait for container to be Running again (postStart must complete first)
			Eventually(func() bool {
				p := &corev1.Pod{}
				if getErr := k8sClient.Get(ctx, types.NamespacedName{
					Name: podName, Namespace: namespace,
				}, p); getErr != nil {
					return false
				}
				for _, cs := range p.Status.ContainerStatuses {
					if cs.Name == "openclaw" && cs.State.Running != nil {
						return true
					}
				}
				return false
			}, 2*time.Minute, 2*time.Second).Should(BeTrue(),
				"openclaw container should be Running after restart")

			// Verify the postStart hook restored the config.
			// Use Eventually because kubectl exec may briefly fail after a
			// container restart even though the status already shows Running.
			Eventually(func(g Gomega) {
				out, err = kubectlExec(namespace, podName,
					"cat", "/home/openclaw/.openclaw/openclaw.json")
				g.Expect(err).NotTo(HaveOccurred(),
					"should read config after restart: %s", out)
				g.Expect(out).To(ContainSubstring(`"loopback"`),
					"gateway.bind=loopback should be restored by postStart hook")
				g.Expect(out).NotTo(ContainSubstring("corrupted"),
					"corrupted content should be overwritten by postStart hook")
			}, 1*time.Minute, 2*time.Second).Should(Succeed())

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})

	Context("When creating an instance with configMapRef (#136)", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-cmref-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should enrich external ConfigMap config with gateway auth", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "cmref-enriched"

			// Create external ConfigMap with partial config (no gateway auth)
			externalCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "my-external-config",
					Namespace: namespace,
				},
				Data: map[string]string{
					"openclaw.json": `{"mcpServers":{"test":{"url":"http://localhost:3000"}}}`,
				},
			}
			Expect(k8sClient.Create(ctx, externalCM)).Should(Succeed())

			// Create instance referencing the external ConfigMap
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
					Config: openclawv1alpha1.ConfigSpec{
						ConfigMapRef: &openclawv1alpha1.ConfigMapKeySelector{
							Name: "my-external-config",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify operator-managed ConfigMap is created with enriched content
			cm := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.ConfigMapName(instance),
					Namespace: namespace,
				}, cm)
			}, timeout, interval).Should(Succeed())

			configContent, ok := cm.Data["openclaw.json"]
			Expect(ok).To(BeTrue(), "operator-managed ConfigMap should have openclaw.json key")

			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(configContent), &parsed)).To(Succeed())

			// User config should be preserved
			_, hasMCP := parsed["mcpServers"]
			Expect(hasMCP).To(BeTrue(), "user's mcpServers should be preserved")

			// Gateway auth should be injected
			gw, ok := parsed["gateway"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "config should have gateway key")
			Expect(gw["bind"]).To(Equal("loopback"), "gateway.bind should be loopback")
			auth, ok := gw["auth"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "gateway should have auth key (injected by operator)")
			Expect(auth["mode"]).To(Equal("token"), "gateway.auth.mode should be token")
			Expect(auth["token"]).NotTo(BeEmpty(), "gateway.auth.token should be set")

			// Device auth should be disabled (incompatible with K8s)
			controlUI, ok := gw["controlUi"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "gateway should have controlUi key (injected by operator)")
			Expect(controlUI["dangerouslyDisableDeviceAuth"]).To(Equal(true),
				"gateway.controlUi.dangerouslyDisableDeviceAuth should be true")

			// Verify StatefulSet config volume points to operator-managed CM (not external)
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			var configVol *corev1.Volume
			for i := range statefulSet.Spec.Template.Spec.Volumes {
				if statefulSet.Spec.Template.Spec.Volumes[i].Name == "config" {
					configVol = &statefulSet.Spec.Template.Spec.Volumes[i]
					break
				}
			}
			Expect(configVol).NotTo(BeNil(), "config volume should exist")
			Expect(configVol.ConfigMap).NotTo(BeNil())
			Expect(configVol.ConfigMap.Name).To(Equal(resources.ConfigMapName(instance)),
				"config volume should reference operator-managed CM, not external CM")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})

		It("Should preserve trusted-proxy auth mode without injecting token", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "cmref-trusted-proxy"

			// Create external ConfigMap with trusted-proxy auth mode
			externalCM := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "trusted-proxy-config",
					Namespace: namespace,
				},
				Data: map[string]string{
					"openclaw.json": `{"gateway":{"auth":{"mode":"trusted-proxy"}}}`,
				},
			}
			Expect(k8sClient.Create(ctx, externalCM)).Should(Succeed())

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
					Config: openclawv1alpha1.ConfigSpec{
						ConfigMapRef: &openclawv1alpha1.ConfigMapKeySelector{
							Name: "trusted-proxy-config",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify operator-managed ConfigMap preserves the auth mode
			cm := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.ConfigMapName(instance),
					Namespace: namespace,
				}, cm)
			}, timeout, interval).Should(Succeed())

			configContent, ok := cm.Data["openclaw.json"]
			Expect(ok).To(BeTrue(), "operator-managed ConfigMap should have openclaw.json key")

			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(configContent), &parsed)).To(Succeed())

			gw, ok := parsed["gateway"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "config should have gateway key")
			auth, ok := gw["auth"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "gateway should have auth key")

			// Mode should be preserved as trusted-proxy (not overwritten to token)
			Expect(auth["mode"]).To(Equal("trusted-proxy"),
				"gateway.auth.mode should be preserved as trusted-proxy")
			// Token must NOT be present in trusted-proxy mode
			Expect(auth).NotTo(HaveKey("token"),
				"gateway.auth.token must not be set in trusted-proxy mode")

			// OPENCLAW_GATEWAY_TOKEN env var must NOT be present on the StatefulSet
			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.StatefulSetName(instance),
					Namespace: namespace,
				}, sts)
			}, timeout, interval).Should(Succeed())

			mainContainer := sts.Spec.Template.Spec.Containers[0]
			for _, env := range mainContainer.Env {
				Expect(env.Name).NotTo(Equal("OPENCLAW_GATEWAY_TOKEN"),
					"OPENCLAW_GATEWAY_TOKEN env var must not be set in trusted-proxy mode")
			}

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})

	Context("When creating an instance with auto-scaling enabled", func() {
		const hpaTestName = "e2e-hpa-test"
		const hpaTestNs = "default"

		It("Should create an HPA targeting the StatefulSet", func() {
			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      hpaTestName,
					Namespace: hpaTestNs,
				},
				Spec: openclawv1alpha1.OpenClawInstanceSpec{
					Availability: openclawv1alpha1.AvailabilitySpec{
						AutoScaling: &openclawv1alpha1.AutoScalingSpec{
							Enabled:              resources.Ptr(true),
							MinReplicas:          resources.Ptr(int32(1)),
							MaxReplicas:          resources.Ptr(int32(3)),
							TargetCPUUtilization: resources.Ptr(int32(70)),
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Wait for HPA to be created
			hpa := &autoscalingv2.HorizontalPodAutoscaler{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.HPAName(instance),
					Namespace: hpaTestNs,
				}, hpa)
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			Expect(hpa.Spec.ScaleTargetRef.Kind).To(Equal("StatefulSet"))
			Expect(hpa.Spec.ScaleTargetRef.Name).To(Equal(resources.StatefulSetName(instance)))
			Expect(*hpa.Spec.MinReplicas).To(Equal(int32(1)))
			Expect(hpa.Spec.MaxReplicas).To(Equal(int32(3)))
			Expect(hpa.Spec.Metrics).To(HaveLen(1))
			Expect(*hpa.Spec.Metrics[0].Resource.Target.AverageUtilization).To(Equal(int32(70)))

			// Verify StatefulSet has nil replicas (HPA manages it)
			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.StatefulSetName(instance),
					Namespace: hpaTestNs,
				}, sts)
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			// Cleanup
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})

	Context("When creating an instance with auto-scaling and persistence enabled", func() {
		const vctTestName = "e2e-vct-test"
		const vctTestNs = "default"

		It("Should use VolumeClaimTemplates for per-replica PVCs", func() {
			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      vctTestName,
					Namespace: vctTestNs,
				},
				Spec: openclawv1alpha1.OpenClawInstanceSpec{
					Availability: openclawv1alpha1.AvailabilitySpec{
						AutoScaling: &openclawv1alpha1.AutoScalingSpec{
							Enabled:              resources.Ptr(true),
							MinReplicas:          resources.Ptr(int32(1)),
							MaxReplicas:          resources.Ptr(int32(3)),
							TargetCPUUtilization: resources.Ptr(int32(70)),
						},
					},
					Storage: openclawv1alpha1.StorageSpec{
						Persistence: openclawv1alpha1.PersistenceSpec{
							Size: "5Gi",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify StatefulSet uses VolumeClaimTemplates
			sts := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.StatefulSetName(instance),
					Namespace: vctTestNs,
				}, sts)
			}, 60*time.Second, 2*time.Second).Should(Succeed())

			Expect(sts.Spec.VolumeClaimTemplates).To(HaveLen(1))
			vct := sts.Spec.VolumeClaimTemplates[0]
			Expect(vct.Name).To(Equal("data"))
			Expect(vct.Spec.AccessModes).To(ContainElement(corev1.ReadWriteOnce))
			Expect(vct.Spec.Resources.Requests[corev1.ResourceStorage]).To(Equal(resource.MustParse("5Gi")))

			// No static "data" volume in pod spec (VCT auto-provides it)
			var dataVol *corev1.Volume
			for i := range sts.Spec.Template.Spec.Volumes {
				if sts.Spec.Template.Spec.Volumes[i].Name == "data" {
					dataVol = &sts.Spec.Template.Spec.Volumes[i]
					break
				}
			}
			Expect(dataVol).To(BeNil(), "static data volume should not be present when VCTs are used")

			// No standalone PVC should be created
			pvc := &corev1.PersistentVolumeClaim{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      resources.PVCName(instance),
				Namespace: vctTestNs,
			}, pvc)
			Expect(apierrors.IsNotFound(err)).To(BeTrue(), "standalone PVC should not exist when VCTs are used")

			// Cleanup
			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})

	Context("When creating an instance with selfConfigure enabled (#168)", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-selfcfg-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should accept selfConfigure in the CRD schema and configure RBAC", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "selfcfg-e2e"

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
					SelfConfigure: openclawv1alpha1.SelfConfigureSpec{
						Enabled: true,
						AllowedActions: []openclawv1alpha1.SelfConfigAction{
							openclawv1alpha1.SelfConfigActionSkills,
							openclawv1alpha1.SelfConfigActionConfig,
						},
					},
				},
			}

			// The core assertion: selfConfigure must be accepted by the API server
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify the instance was created with selfConfigure preserved
			created := &openclawv1alpha1.OpenClawInstance{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, created)
			}, timeout, interval).Should(Succeed())
			Expect(created.Spec.SelfConfigure.Enabled).To(BeTrue())
			Expect(created.Spec.SelfConfigure.AllowedActions).To(HaveLen(2))

			// Verify ServiceAccount has automountServiceAccountToken=true
			sa := &corev1.ServiceAccount{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.ServiceAccountName(instance),
					Namespace: namespace,
				}, sa)
			}, timeout, interval).Should(Succeed())
			Expect(sa.AutomountServiceAccountToken).NotTo(BeNil())
			Expect(*sa.AutomountServiceAccountToken).To(BeTrue(),
				"selfConfigure should enable SA token automount")

			// Verify StatefulSet has automountServiceAccountToken=true
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())
			Expect(statefulSet.Spec.Template.Spec.AutomountServiceAccountToken).NotTo(BeNil())
			Expect(*statefulSet.Spec.Template.Spec.AutomountServiceAccountToken).To(BeTrue(),
				"selfConfigure should enable pod SA token automount")

			// Verify selfconfig env vars are injected
			mainContainer := statefulSet.Spec.Template.Spec.Containers[0]
			envMap := map[string]string{}
			for _, e := range mainContainer.Env {
				envMap[e.Name] = e.Value
			}
			Expect(envMap).To(HaveKey("OPENCLAW_INSTANCE_NAME"),
				"selfConfigure should inject OPENCLAW_INSTANCE_NAME env var")
			Expect(envMap["OPENCLAW_INSTANCE_NAME"]).To(Equal(instanceName))
			Expect(envMap).To(HaveKey("OPENCLAW_NAMESPACE"),
				"selfConfigure should inject OPENCLAW_NAMESPACE env var")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})

	Context("When updating an OpenClawInstance spec", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-update-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should reflect spec changes in StatefulSet without drift", func() {
			instanceName := "update-drift"

			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

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

			// Wait for StatefulSet to be created
			statefulSet := &appsv1.StatefulSet{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet)
			}, timeout, interval).Should(Succeed())

			// Update the spec - add an env var
			Eventually(func() error {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, instance); err != nil {
					return err
				}
				instance.Spec.Env = []corev1.EnvVar{
					{Name: "CONFORMANCE_TEST", Value: "true"},
				}
				return k8sClient.Update(ctx, instance)
			}, timeout, interval).Should(Succeed())

			// Wait for the StatefulSet to reflect the change
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, statefulSet); err != nil {
					return false
				}
				for _, env := range statefulSet.Spec.Template.Spec.Containers[0].Env {
					if env.Name == "CONFORMANCE_TEST" && env.Value == "true" {
						return true
					}
				}
				return false
			}, timeout, interval).Should(BeTrue(), "StatefulSet should reflect updated env var")

			// Verify ObservedGeneration matches instance generation
			updatedInstance := &openclawv1alpha1.OpenClawInstance{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      instanceName,
					Namespace: namespace,
				}, updatedInstance); err != nil {
					return false
				}
				return updatedInstance.Status.ObservedGeneration == updatedInstance.Generation
			}, timeout, interval).Should(BeTrue(), "ObservedGeneration should match Generation")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})

	Context("When creating an instance with ingress hosts (#234)", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-origins-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should auto-inject controlUi.allowedOrigins from ingress hosts", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			instanceName := "origins-ingress"

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
					Networking: openclawv1alpha1.NetworkingSpec{
						Ingress: openclawv1alpha1.IngressSpec{
							Enabled: true,
							Hosts: []openclawv1alpha1.IngressHost{
								{Host: "openclaw.example.com"},
							},
							TLS: []openclawv1alpha1.IngressTLS{
								{Hosts: []string{"openclaw.example.com"}, SecretName: "tls-secret"},
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

			// Verify ConfigMap contains allowedOrigins
			cm := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.ConfigMapName(instance),
					Namespace: namespace,
				}, cm)
			}, timeout, interval).Should(Succeed())

			configContent, ok := cm.Data["openclaw.json"]
			Expect(ok).To(BeTrue(), "ConfigMap should have openclaw.json key")

			var parsed map[string]interface{}
			Expect(json.Unmarshal([]byte(configContent), &parsed)).To(Succeed())

			gw, ok := parsed["gateway"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "config should have gateway key")
			controlUI, ok := gw["controlUi"].(map[string]interface{})
			Expect(ok).To(BeTrue(), "gateway should have controlUi key")
			origins, ok := controlUI["allowedOrigins"].([]interface{})
			Expect(ok).To(BeTrue(), "controlUi should have allowedOrigins array")

			originStrs := make([]string, len(origins))
			for i, o := range origins {
				originStrs[i] = o.(string)
			}

			Expect(originStrs).To(ContainElement("http://localhost:18789"),
				"should contain localhost origin for port-forwarding")
			Expect(originStrs).To(ContainElement("https://openclaw.example.com"),
				"should contain ingress host origin with https scheme")

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})

	Context("When the operator is running", func() {
		It("Should have the controller manager deployment available", func() {
			deployment := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      "openclaw-operator-controller-manager",
				Namespace: "openclaw-operator-system",
			}, deployment)
			Expect(err).NotTo(HaveOccurred())
			Expect(deployment.Status.AvailableReplicas).To(BeNumerically(">=", 1))
		})
	})

	// Smoke test: verify an OpenClaw instance with default settings boots
	// successfully. This catches config injection bugs like #373 where
	// invalid keys in openclaw.json crash the application on startup.
	Context("When deploying an instance with default config (smoke test)", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-smoke-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should have the pod become Ready with injected config", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping smoke test in minimal mode")
			}

			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "smoke-test",
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

			// Wait for the StatefulSet to have at least 1 Ready replica.
			// This verifies that:
			// 1. The injected config (openclaw.json) passes OpenClaw's strict validation
			// 2. All containers (including sidecars) start successfully
			// 3. Readiness probes pass (gateway is serving traffic)
			sts := &appsv1.StatefulSet{}
			Eventually(func() int32 {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.StatefulSetName(instance),
					Namespace: namespace,
				}, sts); err != nil {
					return 0
				}
				return sts.Status.ReadyReplicas
			}, 5*time.Minute, 5*time.Second).Should(BeNumerically(">=", 1),
				"StatefulSet should have at least 1 Ready replica - if this fails, "+
					"check pod logs for config validation errors or container crashes")

			// If the pod is not becoming Ready, capture pod events for debugging
			if sts.Status.ReadyReplicas < 1 {
				podList := &corev1.PodList{}
				_ = k8sClient.List(ctx, podList, client.InNamespace(namespace))
				for _, pod := range podList.Items {
					for _, cs := range pod.Status.ContainerStatuses {
						if cs.State.Waiting != nil {
							GinkgoWriter.Printf("Container %s waiting: %s - %s\n",
								cs.Name, cs.State.Waiting.Reason, cs.State.Waiting.Message)
						}
					}
				}
			}

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})
})
