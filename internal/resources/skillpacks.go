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
	"strings"
)

const (
	// SkillPackPrefix is the prefix for skill pack entries in the skills list.
	SkillPackPrefix = "pack:"

	// SkillPackUpdatePolicyReplace converges seeded pack files to the declared
	// pack revision on every pod start (overwrite changed files, remove files
	// no longer declared). This is the default.
	SkillPackUpdatePolicyReplace = "Replace"

	// SkillPackUpdatePolicyCreateOnly seeds pack files only when absent and
	// never overwrites or removes them (legacy behavior, see #564).
	SkillPackUpdatePolicyCreateOnly = "CreateOnly"

	// SkillPackManifestPath is where the init script records the set of
	// pack-seeded workspace paths on the data volume. It lives outside
	// /data/workspace so it is not visible to the agent, and on the data
	// volume because the init container rootfs is read-only.
	SkillPackManifestPath = "/data/.skillpack-manifest"
)

// ResolvedSkillPacks contains resolved workspace files, directories, and config for skill packs.
type ResolvedSkillPacks struct {
	// Files maps ConfigMap-safe keys to file content.
	Files map[string]string

	// PathMapping maps ConfigMap-safe keys to workspace-relative paths.
	PathMapping map[string]string

	// Directories to create in the workspace.
	Directories []string

	// SkillEntries to inject into config.raw.skills.entries.
	SkillEntries map[string]interface{}

	// Workspaces maps additional workspace names to the skill packs resolved
	// from spec.workspace.additionalWorkspaces[].skills pack: entries. Only
	// populated on the top-level ResolvedSkillPacks; nested values never
	// populate Workspaces themselves.
	Workspaces map[string]*ResolvedSkillPacks
}

// WorkspacePacks returns the resolved skill packs for the named additional
// workspace, or nil if none were resolved.
func (sp *ResolvedSkillPacks) WorkspacePacks(name string) *ResolvedSkillPacks {
	if sp == nil {
		return nil
	}
	return sp.Workspaces[name]
}

// HasWorkspacePackFiles returns true if any additional workspace has resolved
// skill pack files.
func HasWorkspacePackFiles(sp *ResolvedSkillPacks) bool {
	if sp == nil {
		return false
	}
	for _, wsp := range sp.Workspaces {
		if HasSkillPackFiles(wsp) {
			return true
		}
	}
	return false
}

// ExtractPackSkills returns pack names from the skills list (entries with "pack:" prefix).
func ExtractPackSkills(skills []string) []string {
	var packs []string
	for _, s := range skills {
		if name, ok := strings.CutPrefix(s, SkillPackPrefix); ok {
			packs = append(packs, name)
		}
	}
	return packs
}

// FilterNonPackSkills returns skills that are NOT pack: prefixed.
func FilterNonPackSkills(skills []string) []string {
	var result []string
	for _, s := range skills {
		if !strings.HasPrefix(s, SkillPackPrefix) {
			result = append(result, s)
		}
	}
	return result
}

// SkillPackCMKey converts a workspace-relative path to a ConfigMap-safe key.
// Replaces "/" with "--" to comply with ConfigMap key naming rules. Also used
// for inline spec.workspace.initialFiles paths that contain '/' (#482); the
// init script decodes the key back to the original path when seeding.
func SkillPackCMKey(wsPath string) string {
	return strings.ReplaceAll(wsPath, "/", "--")
}

// HasSkillPackFiles returns true if the resolved skill packs contain any workspace files.
func HasSkillPackFiles(sp *ResolvedSkillPacks) bool {
	return sp != nil && len(sp.Files) > 0
}

// combinedSkillEntries merges the config.raw.skills.entries contributions from
// the default-workspace packs and every additional workspace's packs.
// config.raw.skills.entries is instance-global in OpenClaw, so workspace packs
// contribute to the same map. Workspaces are merged in name order for
// determinism; on key collisions the last merged workspace wins.
func combinedSkillEntries(sp *ResolvedSkillPacks) map[string]interface{} {
	if sp == nil {
		return nil
	}
	merged := make(map[string]interface{}, len(sp.SkillEntries))
	for k, v := range sp.SkillEntries {
		merged[k] = v
	}
	names := make([]string, 0, len(sp.Workspaces))
	for name := range sp.Workspaces {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		wsp := sp.Workspaces[name]
		if wsp == nil {
			continue
		}
		for k, v := range wsp.SkillEntries {
			merged[k] = v
		}
	}
	return merged
}
