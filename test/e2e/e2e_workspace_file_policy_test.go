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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
	"github.com/paperclipinc/openclaw-operator/internal/resources"
)

// Workspace file update policy (#576). Verifies the rendered init container --
// the thing that actually decides whether a file is seeded once or converges --
// and the status reporting that explains why.
var _ = Describe("Workspace file update policy", func() {
	const (
		timeout  = time.Second * 60
		interval = time.Second * 1
	)

	var namespace string

	newInstance := func(name string) *openclawv1alpha1.OpenClawInstance {
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
			},
		}
	}

	// initScript returns the rendered init container command for an instance.
	initScript := func(instance *openclawv1alpha1.OpenClawInstance) string {
		sts := &appsv1.StatefulSet{}
		Eventually(func() error {
			return k8sClient.Get(ctx, types.NamespacedName{
				Name:      resources.StatefulSetName(instance),
				Namespace: namespace,
			}, sts)
		}, timeout, interval).Should(Succeed())

		for _, ic := range sts.Spec.Template.Spec.InitContainers {
			for _, arg := range ic.Args {
				if arg != "" {
					return arg
				}
			}
		}
		return ""
	}

	BeforeEach(func() {
		if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
			Skip("Skipping resource validation in minimal mode")
		}
		namespace = "test-wsfile-" + time.Now().Format("20060102150405")
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
	})

	AfterEach(func() {
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		_ = k8sClient.Delete(ctx, ns)
	})

	It("Should keep seed-once semantics by default", func() {
		instance := newInstance("ws-policy-default")
		instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
			InitialFiles: map[string]string{"AGENTS.md": "seed once"},
		}
		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

		script := initScript(instance)
		Expect(script).To(ContainSubstring("[ -f /data/workspace/'AGENTS.md' ] || cp"),
			"the default must stay CreateOnly for backward compatibility")
		Expect(script).NotTo(ContainSubstring("_ocw_apply"),
			"no converge helper should be emitted without a Replace policy")
	})

	It("Should converge Replace-managed files and report them in status", func() {
		instance := newInstance("ws-policy-replace")
		instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
			InitialFiles: map[string]string{
				"AGENTS.md": "managed content",
				"STATE.md":  "runtime owned",
			},
			ManagedFiles: []openclawv1alpha1.ManagedWorkspaceFile{
				{Path: "AGENTS.md", UpdatePolicy: openclawv1alpha1.WorkspaceFileUpdatePolicyReplace},
			},
		}
		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

		script := initScript(instance)

		By("emitting a converge call for the managed file only")
		Expect(script).To(ContainSubstring("_ocw_apply()"))
		Expect(script).To(ContainSubstring(resources.SourceContentHash("managed content")))
		Expect(script).To(ContainSubstring("[ -f /data/workspace/'STATE.md' ] || cp"),
			"an unlisted file stays seed-once")

		By("reporting the resolved policy and source hash in status")
		Eventually(func() []openclawv1alpha1.ManagedWorkspaceFileStatus {
			current := &openclawv1alpha1.OpenClawInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name: instance.Name, Namespace: namespace,
			}, current); err != nil {
				return nil
			}
			return current.Status.ManagedResources.WorkspaceFiles
		}, timeout, interval).Should(ContainElement(openclawv1alpha1.ManagedWorkspaceFileStatus{
			Path:         "AGENTS.md",
			UpdatePolicy: openclawv1alpha1.WorkspaceFileUpdatePolicyReplace,
			SourceHash:   resources.SourceContentHash("managed content"),
		}))
	})

	// The recorded hash is what makes a local edit survive until the source
	// genuinely changes, so a content change has to move it -- and moving it
	// rolls the pod, which is what re-runs the init container.
	It("Should change the recorded source hash when content changes", func() {
		instance := newInstance("ws-policy-hash")
		instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
			FileUpdatePolicy: openclawv1alpha1.WorkspaceFileUpdatePolicyReplace,
			InitialFiles:     map[string]string{"AGENTS.md": "v1"},
		}
		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

		Expect(initScript(instance)).To(ContainSubstring(resources.SourceContentHash("v1")))

		By("updating the source content")
		Eventually(func() error {
			current := &openclawv1alpha1.OpenClawInstance{}
			if err := k8sClient.Get(ctx, types.NamespacedName{
				Name: instance.Name, Namespace: namespace,
			}, current); err != nil {
				return err
			}
			current.Spec.Workspace.InitialFiles["AGENTS.md"] = "v2"
			return k8sClient.Update(ctx, current)
		}, timeout, interval).Should(Succeed())

		Eventually(func() string {
			return initScript(instance)
		}, timeout, interval).Should(ContainSubstring(resources.SourceContentHash("v2")),
			"the init script must carry the new source hash so the pod converges")
	})

	It("Should let additional workspaces inherit the top-level policy", func() {
		instance := newInstance("ws-policy-addl")
		instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
			FileUpdatePolicy: openclawv1alpha1.WorkspaceFileUpdatePolicyReplace,
			AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
				{Name: "print", InitialFiles: map[string]string{"AGENTS.md": "print rules"}},
			},
		}
		Expect(k8sClient.Create(ctx, instance)).Should(Succeed())

		script := initScript(instance)
		Expect(script).To(ContainSubstring(resources.SourceContentHash("print rules")))
		Expect(script).To(ContainSubstring("/data/.workspace-managed/ws-print--AGENTS.md"),
			"markers must be scoped per workspace so identical paths track independently")
	})

	// An absolute destination is refused by the CRD schema itself, so it holds
	// whether or not the validating webhook is deployed. Traversal paths are
	// covered by the webhook unit tests.
	It("Should reject an absolute managed file path via CRD validation", func() {
		instance := newInstance("ws-policy-absolute")
		instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
			ManagedFiles: []openclawv1alpha1.ManagedWorkspaceFile{{Path: "/etc/passwd"}},
		}

		Expect(k8sClient.Create(ctx, instance)).ShouldNot(Succeed())
	})
})
