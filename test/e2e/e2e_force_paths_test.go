/*
Copyright 2026 OpenClaw.rocks

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
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// E2E coverage for spec.config.forcePaths (#500).
//
// forcePaths has two enforcement sites: the init container (pod restart) and
// the postStart lifecycle hook (container restart without pod recreate). This
// test exercises the init-container path via a full pod delete -- the most
// reproducible scenario across CI runners. The postStart path uses an
// identical Node.js script (same dm/dp functions, same fp env var, same merge
// order), and that structural symmetry is asserted by the unit tests in
// internal/resources/resources_test.go (TestBuildInitScript_MergeMode_WithForcePaths
// pins ordering; TestBuildInitScript_OverwriteMode_IgnoresForcePaths pins
// negative behavior under overwrite); so init correctness empirically
// validates the postStart path as well.
//
// Tenant-write payload deliberately reuses an operator-managed value
// (gateway.auth.token). This sidesteps the openclaw runtime's strict
// top-level schema -- which rejects unknown keys even when they live deep
// under a known top-level -- and keeps the test focused on the security
// property the change is meant to deliver: an attacker-tampered value under
// a forcePath is overwritten with the CR's value on every restart.
var _ = Describe("OpenClawInstance forcePaths (#500)", func() {
	var namespace string

	BeforeEach(func() {
		namespace = "test-forcepaths-" + time.Now().Format("20060102150405")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
	})

	AfterEach(func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		_ = k8sClient.Delete(ctx, ns)
	})

	It("Should rebuild a forcePath subtree from the CR after a pod restart", func() {
		if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
			Skip("Skipping forcePaths e2e in minimal mode")
		}

		instanceName := "forcepaths-e2e"
		podName := instanceName + "-0"
		configPath := "/home/openclaw/.openclaw/openclaw.json"

		// Probes disabled so the pod stays Running without API keys. A small
		// PVC is required: the test deletes the pod to trigger the init
		// container, and we need /data (and the openclaw.json on it) to
		// survive the recreation. emptyDir would reset to the operator's
		// config volume on every pod restart and the test would prove
		// nothing.
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
				Config: openclawv1alpha1.ConfigSpec{
					MergeMode: "merge",
					// gateway.auth is operator-owned (the token is auto-
					// generated into a Secret and surfaced through the
					// config), and listing "gateway" as a forcePath is the
					// concrete attack scenario the PR description targets:
					// a tenant must not be able to substitute their own
					// gateway auth and intercept inference traffic.
					ForcePaths: []string{"gateway"},
				},
				Storage: openclawv1alpha1.StorageSpec{
					Persistence: openclawv1alpha1.PersistenceSpec{
						Size: "1Gi",
					},
				},
				Probes: &openclawv1alpha1.ProbesSpec{
					Liveness:  &openclawv1alpha1.ProbeSpec{Enabled: &falseVal},
					Readiness: &openclawv1alpha1.ProbeSpec{Enabled: &falseVal},
					Startup:   &openclawv1alpha1.ProbeSpec{Enabled: &falseVal},
				},
			},
		}

		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

		// Wait for the openclaw container to be Running. The polling closure
		// logs current pod state via GinkgoWriter so that on a timeout the
		// failure output shows exactly what was blocking (image pull, init
		// container, postStart, etc.) instead of a bare "expected true".
		Eventually(func() bool {
			pod := &corev1.Pod{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name: podName, Namespace: namespace,
			}, pod); err != nil {
				GinkgoWriter.Printf("pod get error: %v\n", err)
				return false
			}
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.Name == "openclaw" && cs.State.Running != nil {
					return true
				}
			}
			GinkgoWriter.Printf("pod %s phase=%s init=%s containers=%s\n",
				podName, pod.Status.Phase,
				summarizeStatuses(pod.Status.InitContainerStatuses),
				summarizeStatuses(pod.Status.ContainerStatuses))
			return false
		}, 10*time.Minute, 5*time.Second).Should(BeTrue(),
			"openclaw container should be Running")

		// Capture the operator-managed gateway.auth.token before tampering.
		// That's the value the init container must restore on restart -- not
		// some literal we made up, which keeps this test resilient to changes
		// in how the operator generates tokens.
		var originalToken string
		Eventually(func(g Gomega) {
			out, err := kubectlExec(namespace, podName, "ls", "-la", configPath)
			g.Expect(err).NotTo(HaveOccurred(), "ls %s: %s", configPath, out)

			out, err = kubectlExec(namespace, podName, "sh", "-c",
				`node -e 'const c=JSON.parse(require("fs").readFileSync("`+configPath+`","utf8"));`+
					`process.stdout.write(c.gateway&&c.gateway.auth&&c.gateway.auth.token||"")'`)
			g.Expect(err).NotTo(HaveOccurred(), "read token: %s", out)
			g.Expect(out).NotTo(BeEmpty(),
				"operator should have set gateway.auth.token in the on-disk config")
			originalToken = out
		}, 2*time.Minute, 3*time.Second).Should(Succeed())

		// Simulate a tenant rewriting gateway.auth.token via the Control UI.
		// The substituted value is a sentinel string we own, so the post-
		// restart assertion can distinguish "wiped and rebuilt from CR" from
		// "still has the tampered value".
		const tamperedToken = "tampered-by-tenant-do-not-restore"
		tamperScript := `node -e '` +
			`const fs=require("fs");` +
			`const p="` + configPath + `";` +
			`const j=JSON.parse(fs.readFileSync(p,"utf8"));` +
			`j.gateway=j.gateway||{};` +
			`j.gateway.auth=j.gateway.auth||{};` +
			`j.gateway.auth.token="` + tamperedToken + `";` +
			`fs.writeFileSync(p,JSON.stringify(j,null,2));` +
			`'`
		tamperOut, err := kubectlExec(namespace, podName, "sh", "-c", tamperScript)
		Expect(err).NotTo(HaveOccurred(),
			"tenant write should succeed; exec stdout+stderr: %s", tamperOut)

		out, err := kubectlExec(namespace, podName, "cat", configPath)
		Expect(err).NotTo(HaveOccurred(), "cat after tamper: %s", out)
		Expect(out).To(ContainSubstring(tamperedToken),
			"tampered token should be on disk before restart; got: %s", out)

		// Trigger init-container path via pod delete. The StatefulSet
		// recreates pod-0 with the same name but a new UID; the data PVC
		// is reused so the tampered config persists across the restart and
		// becomes the input to init-config. Killing the main process to
		// exercise the postStart path is intentionally not done here -- the
		// openclaw runtime traps SIGTERM and config-watches the file, both
		// of which interfere with a deterministic test in this image. The
		// postStart path uses an identical merge script and is covered by
		// the unit tests cited in the file-level comment.
		oldPod := &corev1.Pod{}
		Expect(k8sClient.Get(ctx, types.NamespacedName{
			Name: podName, Namespace: namespace,
		}, oldPod)).To(Succeed())
		oldUID := oldPod.UID
		Expect(k8sClient.Delete(ctx, oldPod)).To(Succeed())

		// Every instance gets init-uv, init-pip and init-plugin-runtime-deps
		// unconditionally, so a pod becoming Running is gated on three init
		// containers doing network and filesystem work. On a loaded kind runner
		// that has exceeded five minutes with init-uv not yet even started, so
		// the ceiling is generous here -- a slow runner should not read as a
		// broken forcePaths rebuild.
		//
		// The closure logs pod state so a timeout shows what was actually
		// blocking (image pull, init container, scheduling) instead of a bare
		// "expected true", matching the pre-restart wait above.
		Eventually(func() bool {
			pod := &corev1.Pod{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name: podName, Namespace: namespace,
			}, pod); err != nil {
				GinkgoWriter.Printf("post-restart pod get error: %v\n", err)
				return false
			}
			if pod.UID == oldUID {
				GinkgoWriter.Printf("post-restart pod %s still has the old UID\n", podName)
				return false
			}
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.Name == "openclaw" && cs.State.Running != nil {
					return true
				}
			}
			GinkgoWriter.Printf("post-restart pod %s phase=%s init=%s containers=%s\n",
				podName, pod.Status.Phase,
				summarizeStatuses(pod.Status.InitContainerStatuses),
				summarizeStatuses(pod.Status.ContainerStatuses))
			return false
		}, 10*time.Minute, 5*time.Second).Should(BeTrue(),
			"new openclaw pod should be Running after restart")

		// The contract: gateway.auth.token was under "gateway" (a forcePath),
		// so init-config deleted the gateway subtree before deep-merging the
		// operator-managed config back in. The tampered sentinel must not
		// survive, and the token must reappear (either the original
		// operator-generated value, or a freshly generated one -- both prove
		// the rebuild ran).
		Eventually(func(g Gomega) {
			out, err := kubectlExec(namespace, podName, "cat", configPath)
			g.Expect(err).NotTo(HaveOccurred(),
				"cat config after pod restart: %s", out)

			g.Expect(out).NotTo(ContainSubstring(tamperedToken),
				"forcePaths should have wiped the tampered gateway.auth.token; got: %s", out)

			tokenOut, err := kubectlExec(namespace, podName, "sh", "-c",
				`node -e 'const c=JSON.parse(require("fs").readFileSync("`+configPath+`","utf8"));`+
					`process.stdout.write(c.gateway&&c.gateway.auth&&c.gateway.auth.token||"")'`)
			g.Expect(err).NotTo(HaveOccurred(), "read token after restart: %s", tokenOut)
			g.Expect(tokenOut).NotTo(BeEmpty(),
				"gateway.auth.token must be rebuilt from the operator-managed config")
			g.Expect(tokenOut).NotTo(Equal(tamperedToken),
				"gateway.auth.token must not equal the tampered sentinel")
			// Suppress unused-variable lint if the original token capture
			// was useful only for the pre-restart assertion above.
			_ = originalToken
		}, 2*time.Minute, 5*time.Second).Should(Succeed())

		Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
	})
})

// summarizeStatuses renders a compact one-line view of a container-status
// slice for embedding in GinkgoWriter diagnostics. Empty slice prints "[]".
func summarizeStatuses(statuses []corev1.ContainerStatus) string {
	if len(statuses) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(statuses))
	for i := range statuses {
		cs := &statuses[i]
		s := cs.Name + "="
		switch {
		case cs.State.Waiting != nil:
			s += fmt.Sprintf("Waiting(%s: %s)",
				cs.State.Waiting.Reason, cs.State.Waiting.Message)
		case cs.State.Terminated != nil:
			s += fmt.Sprintf("Terminated(%s, exit=%d)",
				cs.State.Terminated.Reason, cs.State.Terminated.ExitCode)
		case cs.State.Running != nil:
			s += "Running"
		default:
			s += "Unknown"
		}
		parts = append(parts, s)
	}
	return "[" + strings.Join(parts, " ") + "]"
}
