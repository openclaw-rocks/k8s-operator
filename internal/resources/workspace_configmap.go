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
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

const additionalWorkspaceKeySep = "--ws--"

// BuildWorkspaceConfigMap creates a ConfigMap containing workspace seed files.
// Returns nil if the instance has no workspace files (user-defined, operator-injected, or skill packs).
// Skill pack files and inline initialFiles paths use ConfigMap-safe keys (/ replaced
// with --); the init script maps them back to the correct workspace paths.
//
// externalFiles are the resolved contents of spec.workspace.configMapRef (may be nil).
// additionalExternalFiles maps workspace name to resolved configMapRef contents (may be nil).
//
// Merge priority (highest wins):
//  1. Operator-injected (ENVIRONMENT.md, BOOTSTRAP.md, SELFCONFIG.md, selfconfig.sh)
//  2. Inline initialFiles
//  3. External configMapRef entries
//  4. Skill pack files
func BuildWorkspaceConfigMap(instance *openclawv1alpha1.OpenClawInstance, externalFiles map[string]string, additionalExternalFiles map[string]map[string]string, skillPacks *ResolvedSkillPacks) *corev1.ConfigMap {
	files := make(map[string]string)

	// 4. Skill pack files (lowest priority, ConfigMap-safe keys)
	if skillPacks != nil {
		for cmKey, content := range skillPacks.Files {
			files[cmKey] = content
		}
	}

	// 3. External configMapRef entries (keys already flat — ConfigMap data
	// keys cannot contain '/').
	for k, v := range externalFiles {
		files[k] = v
	}

	// 2. User-defined inline workspace files. Keys may contain '/' (nested
	// paths) which is illegal for ConfigMap data keys, so encode them the same
	// way skill packs do (#482).
	if instance.Spec.Workspace != nil {
		for k, v := range instance.Spec.Workspace.InitialFiles {
			files[SkillPackCMKey(k)] = v
		}
	}

	// 1. Operator-injected files (highest priority)
	files["ENVIRONMENT.md"] = EnvironmentSkillContent

	// BOOTSTRAP.md is opt-out via spec.workspace.bootstrap.enabled=false.
	// Skipping injection also skips the init-script re-copy, so agents that
	// have already completed bootstrap don't re-run it on every pod restart (#463).
	if bootstrapEnabled(instance) {
		files["BOOTSTRAP.md"] = BootstrapContent
	}

	// Operator-injected self-configure files
	if instance.Spec.SelfConfigure.Enabled {
		files["SELFCONFIG.md"] = SelfConfigureSkillContent
		files["selfconfig.sh"] = SelfConfigureHelperScript
	}

	// Additional workspaces - each gets namespaced keys in the same ConfigMap.
	// Merge priority per workspace (highest wins):
	//  1. Operator-injected (ENVIRONMENT.md only - no BOOTSTRAP.md for secondary agents)
	//  2. Inline initialFiles
	//  3. External configMapRef entries
	//  4. Skill pack files (from additionalWorkspaces[].skills pack: entries)
	if instance.Spec.Workspace != nil {
		for i := range instance.Spec.Workspace.AdditionalWorkspaces {
			ws := &instance.Spec.Workspace.AdditionalWorkspaces[i]

			// 4. Skill pack files (lowest, ConfigMap-safe keys namespaced per workspace)
			if wsPacks := skillPacks.WorkspacePacks(ws.Name); wsPacks != nil {
				for cmKey, content := range wsPacks.Files {
					files[AdditionalWorkspaceCMKey(ws.Name, cmKey)] = content
				}
			}

			// 3. External configMapRef entries (override pack files)
			if extFiles, ok := additionalExternalFiles[ws.Name]; ok {
				for k, v := range extFiles {
					files[AdditionalWorkspaceCMKey(ws.Name, k)] = v
				}
			}

			// 2. Inline initialFiles (overrides external). Encode '/' in the
			// filename portion before namespacing it under the workspace (#482).
			for k, v := range ws.InitialFiles {
				files[AdditionalWorkspaceCMKey(ws.Name, SkillPackCMKey(k))] = v
			}

			// 1. ENVIRONMENT.md only (no BOOTSTRAP.md for secondary agents)
			files[AdditionalWorkspaceCMKey(ws.Name, "ENVIRONMENT.md")] = EnvironmentSkillContent
		}
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      WorkspaceConfigMapName(instance),
			Namespace: instance.Namespace,
			Labels:    Labels(instance),
		},
		Data: files,
	}
}

// AdditionalWorkspaceCMKey returns the ConfigMap key for a file in an additional workspace.
// Uses a namespaced format: "--ws--<name>--<filename>" to avoid collisions with default workspace keys.
func AdditionalWorkspaceCMKey(workspaceName, filename string) string {
	return fmt.Sprintf("%s%s--%s", additionalWorkspaceKeySep, workspaceName, filename)
}

// ParseAdditionalWorkspaceCMKey extracts the workspace name and filename from a namespaced key.
// Returns ("", "", false) if the key is not an additional workspace key.
func ParseAdditionalWorkspaceCMKey(cmKey string) (workspaceName, filename string, ok bool) {
	if !strings.HasPrefix(cmKey, additionalWorkspaceKeySep) {
		return "", "", false
	}
	rest := cmKey[len(additionalWorkspaceKeySep):]
	// Split on first "--" to get workspace name and filename
	idx := strings.Index(rest, "--")
	if idx < 0 {
		return "", "", false
	}
	return rest[:idx], rest[idx+2:], true
}
