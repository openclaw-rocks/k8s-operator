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
	"encoding/json"
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

var _ = Describe("Additional Workspace Skills", func() {
	// Reuses the test skill pack at test/testdata/skill-packs/test-skill/,
	// resolved from GitHub via the branch ref in E2E_SKILL_PACK_REF (#568).

	Context("When an additional workspace declares a pack: skill", func() {
		var namespace string

		BeforeEach(func() {
			namespace = "test-ws-skills-" + time.Now().Format("20060102150405")
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})

		AfterEach(func() {
			ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
			_ = k8sClient.Delete(ctx, ns)
		})

		It("Should resolve the pack and scope it to the workspace", func() {
			if os.Getenv("E2E_SKIP_RESOURCE_VALIDATION") == "true" {
				Skip("Skipping resource validation in minimal mode")
			}

			ref := os.Getenv("E2E_SKILL_PACK_REF")
			if ref == "" {
				ref = "main"
			}
			packRef := "pack:paperclipinc/openclaw-operator/test/testdata/skill-packs/test-skill@" + ref

			instance := &openclawv1alpha1.OpenClawInstance{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ws-skills-test",
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
					Workspace: &openclawv1alpha1.WorkspaceSpec{
						AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
							{
								Name:   "secondary",
								Skills: []string{packRef},
							},
						},
					},
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

			// The workspace ConfigMap holds the pack files under keys
			// namespaced to the "secondary" workspace.
			workspaceCM := &corev1.ConfigMap{}
			Eventually(func() error {
				return k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.WorkspaceConfigMapName(instance),
					Namespace: namespace,
				}, workspaceCM)
			}, 30*time.Second, 2*time.Second).Should(Succeed())

			skillMDKey := resources.AdditionalWorkspaceCMKey("secondary",
				resources.SkillPackCMKey("skills/test-skill/SKILL.md"))
			helloKey := resources.AdditionalWorkspaceCMKey("secondary",
				resources.SkillPackCMKey("skills/test-skill/scripts/hello.sh"))

			// Pack resolution happens on a later reconcile if GitHub is slow;
			// wait for the namespaced keys to appear.
			Eventually(func() map[string]string {
				_ = k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.WorkspaceConfigMapName(instance),
					Namespace: namespace,
				}, workspaceCM)
				return workspaceCM.Data
			}, 60*time.Second, 2*time.Second).Should(HaveKey(skillMDKey),
				"workspace ConfigMap should contain the namespaced SKILL.md key")
			Expect(workspaceCM.Data[skillMDKey]).To(ContainSubstring("test-skill"))
			Expect(workspaceCM.Data).To(HaveKey(helloKey))

			// The pack files must not leak into the default workspace keys.
			Expect(workspaceCM.Data).NotTo(HaveKey(resources.SkillPackCMKey("skills/test-skill/SKILL.md")),
				"workspace-scoped pack must not seed the default workspace")

			// The init script seeds into /data/workspace-secondary and tracks
			// the files in a per-workspace manifest.
			Eventually(func() string {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.StatefulSetName(instance),
					Namespace: namespace,
				}, statefulSet); err != nil {
					return ""
				}
				for i := range statefulSet.Spec.Template.Spec.InitContainers {
					if statefulSet.Spec.Template.Spec.InitContainers[i].Name == "init-config" {
						return statefulSet.Spec.Template.Spec.InitContainers[i].Command[2]
					}
				}
				return ""
			}, 60*time.Second, 2*time.Second).Should(SatisfyAll(
				ContainSubstring("cp /workspace-init/'"+skillMDKey+"' /data/workspace-secondary/'skills/test-skill/SKILL.md'"),
				ContainSubstring("mv /data/.skillpack-manifest-ws-secondary.new /data/.skillpack-manifest-ws-secondary"),
			), "init script should sync the pack into the secondary workspace")

			// Manifest-mode packs inject config.raw.skills.entries.
			configCM := &corev1.ConfigMap{}
			Eventually(func() bool {
				if err := k8sClient.Get(ctx, types.NamespacedName{
					Name:      resources.ConfigMapName(instance),
					Namespace: namespace,
				}, configCM); err != nil {
					return false
				}
				var config map[string]interface{}
				if err := json.Unmarshal([]byte(configCM.Data["openclaw.json"]), &config); err != nil {
					return false
				}
				skillsSection, ok := config["skills"].(map[string]interface{})
				if !ok {
					return false
				}
				entries, ok := skillsSection["entries"].(map[string]interface{})
				if !ok {
					return false
				}
				_, ok = entries["test-skill"]
				return ok
			}, 60*time.Second, 2*time.Second).Should(BeTrue(),
				"workspace pack config entries should be injected into config.raw.skills.entries")

			// No installable (clawhub/npm) skills anywhere: no init-skills container.
			for _, c := range statefulSet.Spec.Template.Spec.InitContainers {
				Expect(c.Name).NotTo(Equal("init-skills"),
					"pack-only workspace skills must not create an init-skills container")
			}

			Expect(k8sClient.Delete(ctx, instance)).Should(Succeed())
		})
	})
})
