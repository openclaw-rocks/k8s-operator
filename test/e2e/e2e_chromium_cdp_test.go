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
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
	"github.com/paperclipinc/openclaw-operator/internal/resources"
)

// cdpCommand represents a Chrome DevTools Protocol command sent over WebSocket.
type cdpCommand struct {
	ID     int                    `json:"id"`
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params,omitempty"`
}

// cdpResponse represents a Chrome DevTools Protocol response received over WebSocket.
type cdpResponse struct {
	ID     int                    `json:"id"`
	Result map[string]interface{} `json:"result,omitempty"`
	Error  map[string]interface{} `json:"error,omitempty"`
	Method string                 `json:"method,omitempty"`
}

// cdpSessionCommand represents a CDP command sent to a specific target session.
type cdpSessionCommand struct {
	ID        int                    `json:"id"`
	Method    string                 `json:"method"`
	Params    map[string]interface{} `json:"params,omitempty"`
	SessionID string                 `json:"sessionId"`
}

var _ = Describe("Chromium CDP Functional Tests", Ordered, func() {
	var (
		namespace    string
		instanceName string
		localPort    int
		portFwdCmd   *exec.Cmd
		podName      string
	)

	BeforeAll(func() {
		if os.Getenv("E2E_SKIP_CDP_FUNCTIONAL") == "true" {
			Skip("Skipping CDP functional tests (E2E_SKIP_CDP_FUNCTIONAL=true)")
		}
		if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
			Skip("Skipping CDP functional tests in minimal mode")
		}

		instanceName = "cdp-func-test"
		namespace = "test-cdp-" + time.Now().Format("20060102150405")

		By("Creating test namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

		By("Creating OpenClawInstance with chromium enabled")
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

		By("Waiting for StatefulSet to be created")
		sts := &appsv1.StatefulSet{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      resources.StatefulSetName(instance),
				Namespace: namespace,
			}, sts)
		}, 60*time.Second, 2*time.Second).Should(Succeed())

		By("Waiting for pod to exist")
		Eventually(func() string {
			podList := &corev1.PodList{}
			err := k8sClient.List(ctx, podList,
				client.InNamespace(namespace),
				client.MatchingLabels{
					"app.kubernetes.io/instance": instanceName,
					"app.kubernetes.io/name":     "openclaw",
				},
			)
			if err != nil || len(podList.Items) == 0 {
				return ""
			}
			podName = podList.Items[0].Name
			return podName
		}, 120*time.Second, 3*time.Second).ShouldNot(BeEmpty())

		By("Waiting for pod to be in Running phase with chromium init container ready")
		Eventually(func() bool {
			pod := &corev1.Pod{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      podName,
				Namespace: namespace,
			}, pod)
			if err != nil {
				return false
			}
			if pod.Status.Phase != corev1.PodRunning {
				GinkgoWriter.Printf("Pod phase: %s (waiting for Running)\n", pod.Status.Phase)
				return false
			}
			for _, cs := range pod.Status.InitContainerStatuses {
				if cs.Name == "chromium" && cs.Ready {
					return true
				}
			}
			return false
		}, 5*time.Minute, 3*time.Second).Should(BeTrue())

		By("Finding a free local port for port-forward")
		listener, err := net.Listen("tcp", ":0")
		Expect(err).NotTo(HaveOccurred())
		localPort = listener.Addr().(*net.TCPAddr).Port
		listener.Close()

		By(fmt.Sprintf("Starting port-forward to pod %s on local port %d", podName, localPort))
		portFwdCmd = exec.Command("kubectl", "port-forward",
			fmt.Sprintf("pod/%s", podName),
			fmt.Sprintf("%d:%d", localPort, resources.ChromiumPort),
			"-n", namespace,
		)
		portFwdCmd.Stdout = GinkgoWriter
		portFwdCmd.Stderr = GinkgoWriter
		Expect(portFwdCmd.Start()).To(Succeed())

		By("Waiting for port-forward to be ready")
		Eventually(func() error {
			// Check if port-forward process exited unexpectedly
			if portFwdCmd.ProcessState != nil {
				return fmt.Errorf("port-forward process exited: %s", portFwdCmd.ProcessState)
			}
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/json/version", localPort))
			if err != nil {
				return err
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected status: %d", resp.StatusCode)
			}
			return nil
		}, 60*time.Second, 2*time.Second).Should(Succeed())

		GinkgoWriter.Printf("CDP port-forward ready on localhost:%d\n", localPort)
	})

	AfterAll(func() {
		if portFwdCmd != nil && portFwdCmd.Process != nil {
			By("Killing port-forward process")
			_ = portFwdCmd.Process.Kill()
			_ = portFwdCmd.Wait()
		}

		if namespace != "" {
			By("Deleting test namespace")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: namespace},
			}
			_ = k8sClient.Delete(ctx, ns)
		}
	})

	It("Tier 1: /json/version endpoint responds with Chrome version info", func() {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/json/version", localPort))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())

		var versionInfo map[string]interface{}
		Expect(json.Unmarshal(body, &versionInfo)).To(Succeed())

		GinkgoWriter.Printf("CDP version info: %s\n", string(body))

		browser, ok := versionInfo["Browser"].(string)
		Expect(ok).To(BeTrue(), "response should have a Browser field")
		Expect(browser).To(SatisfyAny(
			ContainSubstring("HeadlessChrome"),
			ContainSubstring("Chrome"),
		), "Browser field should contain Chrome or HeadlessChrome")

		wsURL, ok := versionInfo["webSocketDebuggerUrl"].(string)
		Expect(ok).To(BeTrue(), "response should have a webSocketDebuggerUrl field")
		Expect(wsURL).NotTo(BeEmpty(), "webSocketDebuggerUrl should not be empty")
	})

	It("Tier 2: navigates to a page and captures screenshot via CDP WebSocket", func() {
		By("Getting browser debugger WebSocket URL from /json/version")
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/json/version", localPort))
		Expect(err).NotTo(HaveOccurred())
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		Expect(err).NotTo(HaveOccurred())

		var versionInfo map[string]interface{}
		Expect(json.Unmarshal(body, &versionInfo)).To(Succeed())

		browserWSURL, ok := versionInfo["webSocketDebuggerUrl"].(string)
		Expect(ok).To(BeTrue(), "response should have webSocketDebuggerUrl")

		// Rewrite the WebSocket URL to use our local port-forward port.
		// Chrome returns ws://127.0.0.1:9222/... but we need ws://localhost:<localPort>/...
		browserWSURL = rewriteCDPWebSocketURL(browserWSURL, localPort)

		By(fmt.Sprintf("Connecting to browser CDP WebSocket at %s", browserWSURL))
		dialer := websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
		}
		browserWS, _, err := dialer.Dial(browserWSURL, nil)
		Expect(err).NotTo(HaveOccurred())
		defer browserWS.Close()

		By("Creating a new target via Target.createTarget")
		createCmd := cdpCommand{
			ID:     1,
			Method: "Target.createTarget",
			Params: map[string]interface{}{
				"url": "about:blank",
			},
		}
		Expect(browserWS.WriteJSON(createCmd)).To(Succeed())

		createResp := readCDPResponse(browserWS, 1, 10*time.Second)
		Expect(createResp).NotTo(BeNil(), "should receive response for Target.createTarget")
		Expect(createResp.Error).To(BeNil(), "Target.createTarget should not return an error")

		targetID, ok := createResp.Result["targetId"].(string)
		Expect(ok).To(BeTrue(), "createTarget result should have targetId")
		GinkgoWriter.Printf("Created target: %s\n", targetID)

		By("Attaching to target via Target.attachToTarget")
		attachCmd := cdpCommand{
			ID:     2,
			Method: "Target.attachToTarget",
			Params: map[string]interface{}{
				"targetId": targetID,
				"flatten":  true,
			},
		}
		Expect(browserWS.WriteJSON(attachCmd)).To(Succeed())

		attachResp := readCDPResponse(browserWS, 2, 10*time.Second)
		Expect(attachResp).NotTo(BeNil(), "should receive response for Target.attachToTarget")
		Expect(attachResp.Error).To(BeNil(), "Target.attachToTarget should not return an error")

		sessionID, ok := attachResp.Result["sessionId"].(string)
		Expect(ok).To(BeTrue(), "attachToTarget result should have sessionId")
		GinkgoWriter.Printf("Attached to session: %s\n", sessionID)

		By("Enabling Page domain")
		enableCmd := cdpSessionCommand{
			ID:        3,
			Method:    "Page.enable",
			SessionID: sessionID,
		}
		Expect(browserWS.WriteJSON(enableCmd)).To(Succeed())

		enableResp := readCDPResponse(browserWS, 3, 10*time.Second)
		Expect(enableResp).NotTo(BeNil(), "should receive response for Page.enable")

		By("Navigating to https://example.com")
		navigateCmd := cdpSessionCommand{
			ID:     4,
			Method: "Page.navigate",
			Params: map[string]interface{}{
				"url": "https://example.com",
			},
			SessionID: sessionID,
		}
		Expect(browserWS.WriteJSON(navigateCmd)).To(Succeed())

		navigateResp := readCDPResponse(browserWS, 4, 30*time.Second)
		Expect(navigateResp).NotTo(BeNil(), "should receive response for Page.navigate")
		Expect(navigateResp.Error).To(BeNil(), "Page.navigate should not return an error")

		By("Waiting for page load")
		waitForLoadEvent(browserWS, 15*time.Second)

		By("Capturing screenshot via Page.captureScreenshot")
		screenshotCmd := cdpSessionCommand{
			ID:     5,
			Method: "Page.captureScreenshot",
			Params: map[string]interface{}{
				"format": "png",
			},
			SessionID: sessionID,
		}
		Expect(browserWS.WriteJSON(screenshotCmd)).To(Succeed())

		screenshotResp := readCDPResponse(browserWS, 5, 15*time.Second)
		Expect(screenshotResp).NotTo(BeNil(), "should receive response for Page.captureScreenshot")
		Expect(screenshotResp.Error).To(BeNil(), "Page.captureScreenshot should not return an error")

		data, ok := screenshotResp.Result["data"].(string)
		Expect(ok).To(BeTrue(), "screenshot result should have a data field")
		Expect(data).NotTo(BeEmpty(), "screenshot data should not be empty")

		GinkgoWriter.Printf("Screenshot captured: %d bytes of base64 PNG data\n", len(data))

		By("Closing the target")
		closeCmd := cdpCommand{
			ID:     6,
			Method: "Target.closeTarget",
			Params: map[string]interface{}{
				"targetId": targetID,
			},
		}
		Expect(browserWS.WriteJSON(closeCmd)).To(Succeed())
		readCDPResponse(browserWS, 6, 5*time.Second)
	})

	It("Tier 3: CDP is reachable via headless Service DNS from within cluster", func() {
		cdpServiceName := resources.ChromiumCDPServiceName(&openclawv1alpha1.OpenClawInstance{
			ObjectMeta: metav1.ObjectMeta{Name: instanceName},
		})
		cdpURL := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d/json/version",
			cdpServiceName, namespace, resources.ChromiumPort,
		)

		// Use a unique pod name to avoid conflicts
		testPodName := fmt.Sprintf("cdp-test-%d", time.Now().UnixNano()%100000)

		// Verify the headless CDP Service has endpoints pointing to the pod,
		// then use curl from a temporary pod to confirm CDP responds via
		// the Service DNS name.
		By("Verifying CDP headless Service has endpoints")
		endpoints := &corev1.Endpoints{}
		Eventually(func() bool {
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      cdpServiceName,
				Namespace: namespace,
			}, endpoints)
			if err != nil {
				return false
			}
			for _, subset := range endpoints.Subsets {
				if len(subset.Addresses) > 0 {
					return true
				}
			}
			return false
		}, 30*time.Second, 2*time.Second).Should(BeTrue(),
			"CDP headless Service should have at least one endpoint address")

		By(fmt.Sprintf("Running curl from a temporary pod to %s", cdpURL))
		// The endpoint gate above only proves the pod is Ready -- Chrome's
		// DevTools HTTP server may still be starting, so the first attempt can
		// legitimately fail. Retry rather than failing the spec on a race (#585).
		//
		// -sS keeps the progress meter quiet but still prints transport errors,
		// and -f is deliberately absent: with it, a non-2xx produced an empty
		// body and the assertion reported a missing webSocketDebuggerUrl when
		// the real cause was an HTTP error. The status code is captured
		// separately so a failure says what actually happened.
		var outputStr string
		attempt := 0
		Eventually(func() string {
			attempt++
			// Chrome's DevTools HTTP server returns 500 for requests with long
			// Host headers (like Kubernetes service DNS names). Override the
			// Host header to localhost so Chrome handles the request correctly.
			cmd := exec.Command("kubectl", "run", fmt.Sprintf("%s-%d", testPodName, attempt),
				"--rm", "-i",
				"--restart=Never",
				"--timeout=60s",
				"--namespace", namespace,
				// Fully qualified: short names are ambiguous under CRI-O short
				// name enforcing, same reason as #562.
				"--image=docker.io/curlimages/curl:8.11.1",
				"--", "curl", "-sS", "--max-time", "10",
				"-w", "\nHTTP_STATUS:%{http_code}\n",
				"-H", "Host: localhost",
				cdpURL,
			)

			output, err := cmd.CombinedOutput()
			outputStr = string(output)
			GinkgoWriter.Printf("kubectl run output (err=%v): %s\n", err, outputStr)
			return outputStr
		}, 90*time.Second, 5*time.Second).Should(ContainSubstring("webSocketDebuggerUrl"),
			"response from CDP headless Service should contain webSocketDebuggerUrl")

		Expect(outputStr).To(ContainSubstring("HTTP_STATUS:200"),
			"CDP endpoint should return 200, output: %s", outputStr)
	})
})

// Regression test for #396: verify that an instance with the deprecated
// ghcr.io/browserless/chromium image (from pre-v0.22.1 kubebuilder defaults)
// gets migrated and CDP actually works end-to-end.
var _ = Describe("Chromium Deprecated Image Migration", Ordered, func() {
	var (
		namespace    string
		instanceName string
		localPort    int
		portFwdCmd   *exec.Cmd
		podName      string
	)

	BeforeAll(func() {
		if os.Getenv("E2E_SKIP_CDP_FUNCTIONAL") == "true" {
			Skip("Skipping CDP functional tests (E2E_SKIP_CDP_FUNCTIONAL=true)")
		}
		if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
			Skip("Skipping CDP functional tests in minimal mode")
		}

		instanceName = "cdp-migrate-test"
		namespace = "test-migrate-" + time.Now().Format("20060102150405")

		By("Creating test namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

		By("Creating OpenClawInstance with deprecated browserless image")
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
						// Simulate pre-v0.22.1 kubebuilder defaults
						Repository: resources.DeprecatedChromiumImage,
						Tag:        "latest",
					},
				},
			},
		}
		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

		By("Waiting for StatefulSet to be created")
		sts := &appsv1.StatefulSet{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      resources.StatefulSetName(instance),
				Namespace: namespace,
			}, sts)
		}, 60*time.Second, 2*time.Second).Should(Succeed())

		By("Verifying image was migrated in StatefulSet")
		var chromiumContainer *corev1.Container
		for i := range sts.Spec.Template.Spec.InitContainers {
			if sts.Spec.Template.Spec.InitContainers[i].Name == "chromium" {
				chromiumContainer = &sts.Spec.Template.Spec.InitContainers[i]
				break
			}
		}
		Expect(chromiumContainer).NotTo(BeNil())
		expectedImage := resources.DefaultChromiumImage + ":" + resources.DefaultChromiumTag
		Expect(chromiumContainer.Image).To(Equal(expectedImage),
			"deprecated image should be migrated to %s", expectedImage)
		Expect(chromiumContainer.Command).To(Equal(resources.ChromiumEntrypointCommand),
			"Command must be the entrypoint wrapper with quoted \"$@\" (#396)")

		By("Waiting for pod to exist")
		Eventually(func() string {
			podList := &corev1.PodList{}
			err := k8sClient.List(ctx, podList,
				client.InNamespace(namespace),
				client.MatchingLabels{
					"app.kubernetes.io/instance": instanceName,
					"app.kubernetes.io/name":     "openclaw",
				},
			)
			if err != nil || len(podList.Items) == 0 {
				return ""
			}
			podName = podList.Items[0].Name
			return podName
		}, 120*time.Second, 3*time.Second).ShouldNot(BeEmpty())

		By("Waiting for chromium init container to be ready")
		Eventually(func() bool {
			pod := &corev1.Pod{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      podName,
				Namespace: namespace,
			}, pod)
			if err != nil {
				return false
			}
			if pod.Status.Phase != corev1.PodRunning {
				GinkgoWriter.Printf("Pod phase: %s (waiting for Running)\n", pod.Status.Phase)
				return false
			}
			for _, cs := range pod.Status.InitContainerStatuses {
				if cs.Name == "chromium" && cs.Ready {
					return true
				}
			}
			return false
		}, 5*time.Minute, 3*time.Second).Should(BeTrue())

		By("Starting port-forward to chromium CDP port")
		listener, err := net.Listen("tcp", ":0")
		Expect(err).NotTo(HaveOccurred())
		localPort = listener.Addr().(*net.TCPAddr).Port
		listener.Close()

		portFwdCmd = exec.Command("kubectl", "port-forward",
			fmt.Sprintf("pod/%s", podName),
			fmt.Sprintf("%d:%d", localPort, resources.ChromiumPort),
			"-n", namespace,
		)
		portFwdCmd.Stdout = GinkgoWriter
		portFwdCmd.Stderr = GinkgoWriter
		Expect(portFwdCmd.Start()).To(Succeed())

		By("Waiting for CDP to respond")
		Eventually(func() error {
			if portFwdCmd.ProcessState != nil {
				return fmt.Errorf("port-forward process exited: %s", portFwdCmd.ProcessState)
			}
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d/json/version", localPort))
			if err != nil {
				return err
			}
			resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return fmt.Errorf("unexpected status: %d", resp.StatusCode)
			}
			return nil
		}, 60*time.Second, 2*time.Second).Should(Succeed())
	})

	AfterAll(func() {
		if portFwdCmd != nil && portFwdCmd.Process != nil {
			_ = portFwdCmd.Process.Kill()
			_ = portFwdCmd.Wait()
		}
		if namespace != "" {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: namespace},
			}
			_ = k8sClient.Delete(ctx, ns)
		}
	})

	It("CDP responds after migrating from deprecated browserless image", func() {
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/json/version", localPort))
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		Expect(err).NotTo(HaveOccurred())

		var version map[string]interface{}
		Expect(json.Unmarshal(body, &version)).To(Succeed())

		GinkgoWriter.Printf("CDP /json/version after migration: %s\n", string(body))

		Expect(version).To(HaveKey("Browser"),
			"migrated chromium should report Browser in /json/version")
		Expect(version).To(HaveKey("webSocketDebuggerUrl"),
			"migrated chromium should report webSocketDebuggerUrl")
	})
})

// gwMessage represents a message in the OpenClaw gateway WebSocket protocol.
type gwMessage struct {
	Type    string          `json:"type"`
	ID      string          `json:"id,omitempty"`
	Event   string          `json:"event,omitempty"`
	Method  string          `json:"method,omitempty"`
	OK      *bool           `json:"ok,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// gwConnectPayload is used to extract session defaults from the connect response.
type gwConnectPayload struct {
	Snapshot struct {
		SessionDefaults struct {
			MainSessionKey string `json:"mainSessionKey"`
		} `json:"sessionDefaults"`
	} `json:"snapshot"`
}

// gwChatPayload represents the payload of a chat event from the gateway.
type gwChatPayload struct {
	State      string `json:"state"`
	StopReason string `json:"stopReason,omitempty"`
}

// randomHex generates a random hex string suitable for use as a request ID.
func randomHex() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}

var _ = Describe("Chromium Full Integration Tests", Ordered, func() {
	var (
		namespace    string
		instanceName string
		localPort    int
		portFwdCmd   *exec.Cmd
		podName      string
	)

	BeforeAll(func() {
		apiKey := os.Getenv("OPENROUTER_API_KEY")
		if apiKey == "" {
			Skip("Skipping full integration tests (OPENROUTER_API_KEY not set)")
		}

		instanceName = "cdp-integration"
		namespace = "test-cdp-int-" + time.Now().Format("20060102150405")

		By("Creating test namespace")
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: namespace},
		}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())

		By("Creating OpenClawInstance with chromium and OpenRouter config")
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
				Env: []corev1.EnvVar{
					{
						Name:  "OPENROUTER_API_KEY",
						Value: apiKey,
					},
				},
			},
		}
		instance.Spec.Config.Raw = &openclawv1alpha1.RawConfig{
			RawExtension: runtime.RawExtension{Raw: []byte(`{
				"models": {
					"providers": {
						"openrouter": {
							"baseUrl": "https://openrouter.ai/api/v1",
							"apiKey": "${OPENROUTER_API_KEY}",
							"api": "openai-completions",
							"models": [
								{
									"id": "deepseek/deepseek-chat",
									"name": "DeepSeek V3",
									"contextWindow": 163840
								}
							]
						}
					}
				},
				"agents": {
					"defaults": {
						"model": {
							"primary": "openrouter/deepseek/deepseek-chat"
						}
					}
				}
			}`)},
		}
		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

		By("Waiting for StatefulSet to be created")
		sts := &appsv1.StatefulSet{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      resources.StatefulSetName(instance),
				Namespace: namespace,
			}, sts)
		}, 60*time.Second, 2*time.Second).Should(Succeed())

		By("Waiting for pod to exist")
		Eventually(func() string {
			podList := &corev1.PodList{}
			err := k8sClient.List(ctx, podList,
				client.InNamespace(namespace),
				client.MatchingLabels{
					"app.kubernetes.io/instance": instanceName,
					"app.kubernetes.io/name":     "openclaw",
				},
			)
			if err != nil || len(podList.Items) == 0 {
				return ""
			}
			podName = podList.Items[0].Name
			return podName
		}, 120*time.Second, 3*time.Second).ShouldNot(BeEmpty())

		By("Waiting for all containers to be ready")
		Eventually(func() bool {
			pod := &corev1.Pod{}
			err := k8sClient.Get(ctx, types.NamespacedName{
				Name:      podName,
				Namespace: namespace,
			}, pod)
			if err != nil {
				return false
			}
			if pod.Status.Phase != corev1.PodRunning {
				GinkgoWriter.Printf("Pod phase: %s (waiting for Running)\n", pod.Status.Phase)
				return false
			}
			// Check all regular containers are ready
			for _, cs := range pod.Status.ContainerStatuses {
				if !cs.Ready {
					GinkgoWriter.Printf("Container %s not ready\n", cs.Name)
					return false
				}
			}
			// Check init containers (chromium runs as a restartable init container)
			for _, cs := range pod.Status.InitContainerStatuses {
				if !cs.Ready {
					GinkgoWriter.Printf("Init container %s not ready\n", cs.Name)
					return false
				}
			}
			return true
		}, 10*time.Minute, 5*time.Second).Should(BeTrue())

		By("Finding a free local port for port-forward")
		listener, err := net.Listen("tcp", ":0")
		Expect(err).NotTo(HaveOccurred())
		localPort = listener.Addr().(*net.TCPAddr).Port
		listener.Close()

		By(fmt.Sprintf("Starting port-forward to pod %s gateway on local port %d", podName, localPort))
		portFwdCmd = exec.Command("kubectl", "port-forward",
			fmt.Sprintf("pod/%s", podName),
			fmt.Sprintf("%d:%d", localPort, resources.GatewayPort),
			"-n", namespace,
		)
		portFwdCmd.Stdout = GinkgoWriter
		portFwdCmd.Stderr = GinkgoWriter
		Expect(portFwdCmd.Start()).To(Succeed())

		By("Waiting for port-forward to be ready")
		Eventually(func() error {
			if portFwdCmd.ProcessState != nil {
				return fmt.Errorf("port-forward process exited: %s", portFwdCmd.ProcessState)
			}
			resp, err := http.Get(fmt.Sprintf("http://localhost:%d", localPort))
			if err != nil {
				return err
			}
			resp.Body.Close()
			return nil
		}, 60*time.Second, 2*time.Second).Should(Succeed())

		GinkgoWriter.Printf("Gateway port-forward ready on localhost:%d\n", localPort)
	})

	AfterAll(func() {
		if portFwdCmd != nil && portFwdCmd.Process != nil {
			By("Killing port-forward process")
			_ = portFwdCmd.Process.Kill()
			_ = portFwdCmd.Wait()
		}

		if namespace != "" {
			By("Deleting test namespace")
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{Name: namespace},
			}
			_ = k8sClient.Delete(ctx, ns)
		}
	})

	// Skip: This test depends on an external LLM (OpenRouter/DeepSeek) choosing
	// to invoke the browser tool correctly AND device pairing resolving before the
	// LLM commits to a response path. Both are non-deterministic and cannot be made
	// reliable in CI. The Tier 1 and Tier 2 CDP tests already validate the browser
	// pipeline (CDP connectivity + screenshot via direct WebSocket commands).
	// Run manually with: E2E_RUN_LLM_INTEGRATION=true go test ./test/e2e/... -run "agent pipeline"
	It("Should take a screenshot of paperclip.inc via the agent pipeline", func() {
		if os.Getenv("E2E_RUN_LLM_INTEGRATION") != "true" {
			Skip("Skipping LLM integration test (set E2E_RUN_LLM_INTEGRATION=true to run)")
		}
		By("Reading the gateway token from the auto-generated Secret")
		tokenSecret := &corev1.Secret{}
		secretName := resources.GatewayTokenSecretName(&openclawv1alpha1.OpenClawInstance{
			ObjectMeta: metav1.ObjectMeta{Name: instanceName},
		})
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name:      secretName,
			Namespace: namespace,
		}, tokenSecret)).To(Succeed())

		gatewayToken := string(tokenSecret.Data["token"])
		Expect(gatewayToken).NotTo(BeEmpty(), "gateway token should not be empty")
		GinkgoWriter.Printf("Gateway token retrieved (length=%d)\n", len(gatewayToken))

		By("Connecting WebSocket to gateway")
		dialer := websocket.Dialer{
			HandshakeTimeout: 10 * time.Second,
		}
		wsURL := fmt.Sprintf("ws://localhost:%d", localPort)
		// Origin header must match the gateway's allowedOrigins config (which uses
		// the gateway's own port, not our random port-forward port).
		wsHeaders := http.Header{}
		wsHeaders.Set("Origin", fmt.Sprintf("http://localhost:%d", resources.GatewayPort))
		ws, _, err := dialer.Dial(wsURL, wsHeaders)
		Expect(err).NotTo(HaveOccurred(), "should connect to gateway WebSocket")
		defer ws.Close()

		By("Waiting for connect.challenge event")
		var challengeMsg gwMessage
		Expect(ws.SetReadDeadline(time.Now().Add(15 * time.Second))).To(Succeed())
		err = ws.ReadJSON(&challengeMsg)
		Expect(err).NotTo(HaveOccurred(), "should receive challenge event")
		Expect(challengeMsg.Type).To(Equal("event"))
		Expect(challengeMsg.Event).To(Equal("connect.challenge"))
		GinkgoWriter.Printf("Received connect.challenge: %s\n", string(challengeMsg.Payload))

		By("Sending connect request with gateway token")
		connectID := randomHex()
		connectReq := map[string]interface{}{
			"type":   "req",
			"id":     connectID,
			"method": "connect",
			"params": map[string]interface{}{
				"minProtocol": 3,
				"maxProtocol": 3,
				"client": map[string]interface{}{
					// Use control-ui identity with dangerouslyDisableDeviceAuth to
					// preserve operator scopes without requiring device key pairing.
					"id":       "openclaw-control-ui",
					"version":  "1.0.0",
					"platform": "linux",
					"mode":     "ui",
				},
				"auth": map[string]interface{}{
					"token": gatewayToken,
				},
				"scopes": []string{
					"operator.admin",
					"operator.read",
					"operator.write",
					"operator.approvals",
					"operator.pairing",
				},
			},
		}
		Expect(ws.WriteJSON(connectReq)).To(Succeed())

		By("Waiting for connect response")
		Expect(ws.SetReadDeadline(time.Now().Add(15 * time.Second))).To(Succeed())
		_, connectRaw, err := ws.ReadMessage()
		Expect(err).NotTo(HaveOccurred(), "should receive connect response")
		GinkgoWriter.Printf("Connect response: %s\n", string(connectRaw))

		var connectResp gwMessage
		Expect(json.Unmarshal(connectRaw, &connectResp)).To(Succeed())
		Expect(connectResp.Type).To(Equal("res"))
		Expect(connectResp.ID).To(Equal(connectID))
		Expect(connectResp.OK).NotTo(BeNil())
		Expect(*connectResp.OK).To(BeTrue(),
			"connect response should be ok=true, got: %s", string(connectRaw))
		GinkgoWriter.Println("Gateway connection established")

		// Extract session key from connect response for chat.send
		var gwPayload gwConnectPayload
		Expect(json.Unmarshal(connectResp.Payload, &gwPayload)).To(Succeed())
		sessionKey := gwPayload.Snapshot.SessionDefaults.MainSessionKey
		Expect(sessionKey).NotTo(BeEmpty(), "connect response should contain mainSessionKey")
		GinkgoWriter.Printf("Session key: %s\n", sessionKey)

		By("Sending message to take a screenshot of paperclip.inc")
		sendID := randomHex()
		idempotencyKey := randomHex()
		sendReq := map[string]interface{}{
			"type":   "req",
			"id":     sendID,
			"method": "chat.send",
			"params": map[string]interface{}{
				"message":        "Navigate to https://paperclip.inc and take a screenshot. Use the browser tool with the default profile.",
				"sessionKey":     sessionKey,
				"idempotencyKey": idempotencyKey,
			},
		}
		Expect(ws.WriteJSON(sendReq)).To(Succeed())
		GinkgoWriter.Println("Message sent via chat.send, waiting for agent response...")

		By("Reading events until screenshot or completion")
		// The agent streams assistant text containing a screenshot file path
		// (e.g., sandbox:/home/openclaw/.openclaw/media/browser/<uuid>.png).
		// Collect the full assistant text and verify it references a screenshot.
		//
		// Use a goroutine to read WebSocket messages and send them to a channel.
		// This avoids using ws.SetReadDeadline which permanently breaks Gorilla
		// WebSocket reads on timeout (readErr is cached, subsequent reads fail
		// immediately without attempting I/O).
		type wsResult struct {
			data []byte
			err  error
		}
		msgCh := make(chan wsResult, 1)
		go func() {
			for {
				_, data, err := ws.ReadMessage()
				msgCh <- wsResult{data, err}
				if err != nil {
					return
				}
			}
		}()

		var assistantText string
		agentCompleted := false
		chatSendOK := false
		timeout := time.After(3 * time.Minute)

	eventLoop:
		for !agentCompleted {
			select {
			case <-timeout:
				GinkgoWriter.Println("Timed out waiting for agent response")
				break eventLoop
			case result := <-msgCh:
				if result.err != nil {
					GinkgoWriter.Printf("WebSocket read error: %v\n", result.err)
					break eventLoop
				}

				var msg gwMessage
				if jsonErr := json.Unmarshal(result.data, &msg); jsonErr != nil {
					GinkgoWriter.Printf("Failed to unmarshal message: %v\n", jsonErr)
					continue
				}

				switch {
				case msg.Type == "event" && msg.Event == "device.pair.requested":
					// Auto-approve device pairing requests from internal agent
					// processes. Without this, the agent cannot use browser/node
					// features because the internal connection triggers pairing.
					var pairPayload map[string]interface{}
					if jsonErr := json.Unmarshal(msg.Payload, &pairPayload); jsonErr == nil {
						if reqID, ok := pairPayload["requestId"].(string); ok {
							GinkgoWriter.Printf("Auto-approving device pair request: %s\n", reqID)
							approveReq := map[string]interface{}{
								"type":   "req",
								"id":     randomHex(),
								"method": "device.pair.approve",
								"params": map[string]interface{}{
									"requestId": reqID,
								},
							}
							if wErr := ws.WriteJSON(approveReq); wErr != nil {
								GinkgoWriter.Printf("Failed to send pair approve: %v\n", wErr)
							}
						}
					}

				case msg.Type == "event" && msg.Event == "agent":
					// Extract assistant text from agent stream events
					var agentPayload map[string]interface{}
					if jsonErr := json.Unmarshal(msg.Payload, &agentPayload); jsonErr == nil {
						if agentPayload["stream"] == "assistant" {
							if data, ok := agentPayload["data"].(map[string]interface{}); ok {
								if text, ok := data["text"].(string); ok {
									assistantText = text
								}
							}
						}
					}

				case msg.Type == "event" && msg.Event == "chat":
					var chatPayload gwChatPayload
					if jsonErr := json.Unmarshal(msg.Payload, &chatPayload); jsonErr == nil {
						GinkgoWriter.Printf("Chat event: state=%s stopReason=%s\n",
							chatPayload.State, chatPayload.StopReason)
						if chatPayload.State == "final" {
							agentCompleted = true
						}
					}

				case msg.Type == "res" && msg.ID == sendID:
					if msg.OK != nil && *msg.OK {
						chatSendOK = true
						GinkgoWriter.Println("chat.send accepted")
					} else {
						GinkgoWriter.Printf("chat.send failed: %s\n", string(result.data))
					}

				default:
					if msg.Event != "tick" && msg.Event != "health" && msg.Event != "presence" {
						logMsg := string(result.data)
						if len(logMsg) > 300 {
							logMsg = logMsg[:300] + "..."
						}
						GinkgoWriter.Printf("Event: type=%s event=%s - %s\n", msg.Type, msg.Event, logMsg)
					}
				}
			}
		}

		By("Verifying screenshot was taken")
		Expect(chatSendOK).To(BeTrue(), "chat.send should have been accepted by the gateway")
		GinkgoWriter.Printf("Final assistant text: %s\n", assistantText)
		Expect(assistantText).To(SatisfyAny(
			ContainSubstring(".png"),
			ContainSubstring(".jpg"),
			ContainSubstring("screenshot"),
			ContainSubstring("Screenshot"),
		), "agent response should reference a screenshot file")
	})
})

// readCDPResponse reads CDP WebSocket messages until a response with the given
// ID is found or the timeout expires. Non-matching messages (events and
// responses for other IDs) are logged and discarded.
func readCDPResponse(ws *websocket.Conn, id int, timeout time.Duration) *cdpResponse {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ws.SetReadDeadline(deadline); err != nil {
			GinkgoWriter.Printf("Failed to set read deadline: %v\n", err)
			return nil
		}

		_, msg, err := ws.ReadMessage()
		if err != nil {
			GinkgoWriter.Printf("WebSocket read error: %v\n", err)
			return nil
		}

		var resp cdpResponse
		if err := json.Unmarshal(msg, &resp); err != nil {
			GinkgoWriter.Printf("Failed to unmarshal CDP message: %v\n", err)
			continue
		}

		if resp.ID == id {
			return &resp
		}

		// Log events and other responses for debugging
		if resp.Method != "" {
			GinkgoWriter.Printf("CDP event: %s\n", resp.Method)
		} else {
			GinkgoWriter.Printf("CDP response for id=%d (waiting for id=%d)\n", resp.ID, id)
		}
	}
	return nil
}

// waitForLoadEvent reads CDP messages looking for a Page.loadEventFired event.
// If the event is not received within the timeout, the function returns
// silently (the screenshot may still succeed on a partially loaded page).
func waitForLoadEvent(ws *websocket.Conn, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := ws.SetReadDeadline(deadline); err != nil {
			return
		}

		_, msg, err := ws.ReadMessage()
		if err != nil {
			GinkgoWriter.Printf("waitForLoadEvent: read error: %v\n", err)
			return
		}

		var resp cdpResponse
		if err := json.Unmarshal(msg, &resp); err != nil {
			continue
		}

		GinkgoWriter.Printf("waitForLoadEvent: %s (id=%d)\n", resp.Method, resp.ID)

		if resp.Method == "Page.loadEventFired" {
			GinkgoWriter.Println("Page load event received")
			return
		}
	}
	GinkgoWriter.Println("waitForLoadEvent: timed out waiting for Page.loadEventFired, continuing anyway")
}

// rewriteCDPWebSocketURL replaces the host:port in a Chrome DevTools WebSocket
// URL with localhost:<localPort> so it works through kubectl port-forward.
func rewriteCDPWebSocketURL(wsURL string, localPort int) string {
	wsURL = strings.Replace(wsURL,
		fmt.Sprintf("localhost:%d", resources.ChromiumPort),
		fmt.Sprintf("localhost:%d", localPort), 1)
	wsURL = strings.Replace(wsURL,
		fmt.Sprintf("127.0.0.1:%d", resources.ChromiumPort),
		fmt.Sprintf("localhost:%d", localPort), 1)
	wsURL = strings.Replace(wsURL,
		fmt.Sprintf("0.0.0.0:%d", resources.ChromiumPort),
		fmt.Sprintf("localhost:%d", localPort), 1)
	return wsURL
}
