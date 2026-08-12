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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// managedMarkerDir holds one marker file per Replace-managed workspace file,
// recording the source hash that was last applied. The init container rootfs is
// read-only and /tmp is not always mounted, so /data is the only writable spot
// (same constraint as the skill pack manifest).
const managedMarkerDir = "/data/.workspace-managed"

// WorkspaceFilePolicy resolves the update policy for a path in the default
// workspace. A path listed in managedFiles wins over the workspace-level
// default; everything else falls back to that default, which is CreateOnly when
// unset.
func WorkspaceFilePolicy(ws *openclawv1alpha1.WorkspaceSpec, filePath string) openclawv1alpha1.WorkspaceFileUpdatePolicy {
	if ws == nil {
		return openclawv1alpha1.WorkspaceFileUpdatePolicyCreateOnly
	}
	if p, ok := managedFilePolicy(ws.ManagedFiles, filePath); ok {
		return p
	}
	return defaultedPolicy(ws.FileUpdatePolicy)
}

// AdditionalWorkspaceFilePolicy resolves the update policy for a path in an
// additional workspace. An unset per-workspace fileUpdatePolicy inherits the
// top-level default rather than defaulting independently, so the two cannot
// drift apart.
func AdditionalWorkspaceFilePolicy(ws *openclawv1alpha1.WorkspaceSpec, aw *openclawv1alpha1.AdditionalWorkspace, filePath string) openclawv1alpha1.WorkspaceFileUpdatePolicy {
	if aw == nil {
		return openclawv1alpha1.WorkspaceFileUpdatePolicyCreateOnly
	}
	if p, ok := managedFilePolicy(aw.ManagedFiles, filePath); ok {
		return p
	}
	if aw.FileUpdatePolicy != nil {
		return defaultedPolicy(*aw.FileUpdatePolicy)
	}
	if ws != nil {
		return defaultedPolicy(ws.FileUpdatePolicy)
	}
	return openclawv1alpha1.WorkspaceFileUpdatePolicyCreateOnly
}

// managedFilePolicy looks up an explicit per-path override.
func managedFilePolicy(managed []openclawv1alpha1.ManagedWorkspaceFile, filePath string) (openclawv1alpha1.WorkspaceFileUpdatePolicy, bool) {
	for i := range managed {
		if managed[i].Path != filePath {
			continue
		}
		// Listing a path is an explicit ownership statement, so an omitted
		// policy means Replace rather than the workspace default.
		if managed[i].UpdatePolicy == "" {
			return openclawv1alpha1.WorkspaceFileUpdatePolicyReplace, true
		}
		return managed[i].UpdatePolicy, true
	}
	return "", false
}

func defaultedPolicy(p openclawv1alpha1.WorkspaceFileUpdatePolicy) openclawv1alpha1.WorkspaceFileUpdatePolicy {
	if p == openclawv1alpha1.WorkspaceFileUpdatePolicyReplace {
		return openclawv1alpha1.WorkspaceFileUpdatePolicyReplace
	}
	return openclawv1alpha1.WorkspaceFileUpdatePolicyCreateOnly
}

// WorkspaceHasReplaceFiles reports whether any workspace declares a Replace
// policy, so the init script only carries the converge helper when it is used.
func WorkspaceHasReplaceFiles(ws *openclawv1alpha1.WorkspaceSpec) bool {
	if ws == nil {
		return false
	}
	if defaultedPolicy(ws.FileUpdatePolicy) == openclawv1alpha1.WorkspaceFileUpdatePolicyReplace {
		return true
	}
	for i := range ws.ManagedFiles {
		if p, _ := managedFilePolicy(ws.ManagedFiles, ws.ManagedFiles[i].Path); p == openclawv1alpha1.WorkspaceFileUpdatePolicyReplace {
			return true
		}
	}
	for i := range ws.AdditionalWorkspaces {
		aw := &ws.AdditionalWorkspaces[i]
		if aw.FileUpdatePolicy != nil && defaultedPolicy(*aw.FileUpdatePolicy) == openclawv1alpha1.WorkspaceFileUpdatePolicyReplace {
			return true
		}
		for j := range aw.ManagedFiles {
			if p, _ := managedFilePolicy(aw.ManagedFiles, aw.ManagedFiles[j].Path); p == openclawv1alpha1.WorkspaceFileUpdatePolicyReplace {
				return true
			}
		}
	}
	return false
}

// SourceContentHash returns the SHA-256 of a workspace file's source content.
// The operator computes it at render time and bakes it into the init script, so
// the pod needs no hashing tools and the value is stable across reconciles.
func SourceContentHash(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// managedMarkerPath returns the marker file recording the last applied source
// hash for a workspace-relative path. Workspace name and path are encoded into
// a single flat filename the same way ConfigMap keys are, so nested paths and
// per-workspace scoping cannot collide.
func managedMarkerPath(workspace, filePath string) string {
	key := SkillPackCMKey(filePath)
	if workspace != "" {
		key = "ws-" + workspace + "--" + key
	}
	return managedMarkerDir + "/" + key
}

// managedFileApplyFunc is the shell helper that applies one Replace-managed
// file. It is emitted once per init script and then called per file.
//
// Semantics (#576):
//   - a symlink or non-regular destination is refused, never followed
//   - the destination is rewritten only when the recorded source hash differs
//     from the declared one, so a local edit survives until the source really
//     changes
//   - the write is temp-file plus atomic rename, with a fixed mode so ownership
//     and permissions stay deterministic
//   - nothing is ever pruned: a path whose source disappeared is simply not
//     applied, and the destination is left in place
const managedFileApplyFunc = `_ocw_apply() {
# $1=source $2=destination $3=source hash $4=marker file
if [ -L "$2" ]; then
echo "openclaw: refusing to replace symlink $2" >&2
return 0
fi
if [ -e "$2" ] && [ ! -f "$2" ]; then
echo "openclaw: refusing to replace non-regular file $2" >&2
return 0
fi
if [ -f "$2" ] && [ -f "$4" ] && [ "$(cat "$4")" = "$3" ]; then
return 0
fi
mkdir -p "$(dirname -- "$2")" "$(dirname -- "$4")"
cp "$1" "$2.ocwtmp" && chmod 0644 "$2.ocwtmp" && mv -f "$2.ocwtmp" "$2"
printf '%s\n' "$3" > "$4"
}`

// managedFileApplyLine renders a single _ocw_apply call.
func managedFileApplyLine(srcKey, wsRoot, workspace, filePath, hash string) string {
	return fmt.Sprintf("_ocw_apply /workspace-init/%s %s/%s %s %s",
		shellQuote(srcKey),
		wsRoot, shellQuote(filePath),
		shellQuote(hash),
		shellQuote(managedMarkerPath(workspace, filePath)))
}

// ManagedWorkspaceFileStatuses reports the resolved policy and source hash for
// every declaratively managed file, for status.managedResources.workspaceFiles.
//
// Paths listed in managedFiles are always reported, even when no source
// provides them yet -- that is what makes a later source addition take effect
// without a CR change. Such entries carry an empty hash.
func ManagedWorkspaceFileStatuses(
	instance *openclawv1alpha1.OpenClawInstance,
	externalFiles map[string]string,
	additionalExternalFiles map[string]map[string]string,
) []openclawv1alpha1.ManagedWorkspaceFileStatus {
	ws := instance.Spec.Workspace
	if ws == nil {
		return nil
	}

	var out []openclawv1alpha1.ManagedWorkspaceFileStatus

	add := func(workspace, filePath string, policy openclawv1alpha1.WorkspaceFileUpdatePolicy, content string, hasSource bool) {
		st := openclawv1alpha1.ManagedWorkspaceFileStatus{
			Workspace:    workspace,
			Path:         filePath,
			UpdatePolicy: policy,
		}
		if hasSource {
			st.SourceHash = SourceContentHash(content)
		}
		out = append(out, st)
	}

	// Default workspace.
	sources := defaultWorkspaceSources(ws, externalFiles)
	for _, filePath := range sortedKeys(sources) {
		policy := WorkspaceFilePolicy(ws, filePath)
		if policy != openclawv1alpha1.WorkspaceFileUpdatePolicyReplace {
			continue
		}
		add("", filePath, policy, sources[filePath], true)
	}
	// managedFiles paths with no source yet.
	for i := range ws.ManagedFiles {
		p := ws.ManagedFiles[i].Path
		if _, ok := sources[p]; ok {
			continue
		}
		add("", p, WorkspaceFilePolicy(ws, p), "", false)
	}

	// Additional workspaces.
	addl := make([]openclawv1alpha1.AdditionalWorkspace, len(ws.AdditionalWorkspaces))
	copy(addl, ws.AdditionalWorkspaces)
	sort.Slice(addl, func(i, j int) bool { return addl[i].Name < addl[j].Name })
	for i := range addl {
		aw := &addl[i]
		awSources := additionalWorkspaceSources(aw, additionalExternalFiles[aw.Name])
		for _, filePath := range sortedKeys(awSources) {
			policy := AdditionalWorkspaceFilePolicy(ws, aw, filePath)
			if policy != openclawv1alpha1.WorkspaceFileUpdatePolicyReplace {
				continue
			}
			add(aw.Name, filePath, policy, awSources[filePath], true)
		}
		for j := range aw.ManagedFiles {
			p := aw.ManagedFiles[j].Path
			if _, ok := awSources[p]; ok {
				continue
			}
			add(aw.Name, p, AdditionalWorkspaceFilePolicy(ws, aw, p), "", false)
		}
	}

	return out
}

// workspaceReplaceSources maps each Replace-managed path in the default
// workspace to the hash of its source content. Only paths that actually have a
// source appear -- a managedFiles entry with no source yet has nothing to copy.
func workspaceReplaceSources(instance *openclawv1alpha1.OpenClawInstance, externalFiles map[string]string) map[string]string {
	ws := instance.Spec.Workspace
	if ws == nil {
		return nil
	}
	out := make(map[string]string)
	for filePath, content := range defaultWorkspaceSources(ws, externalFiles) {
		if WorkspaceFilePolicy(ws, filePath) == openclawv1alpha1.WorkspaceFileUpdatePolicyReplace {
			out[filePath] = SourceContentHash(content)
		}
	}
	return out
}

// additionalWorkspaceReplaceSources is workspaceReplaceSources for a secondary workspace.
func additionalWorkspaceReplaceSources(
	instance *openclawv1alpha1.OpenClawInstance,
	aw *openclawv1alpha1.AdditionalWorkspace,
	externalFiles map[string]string,
) map[string]string {
	if aw == nil {
		return nil
	}
	ws := instance.Spec.Workspace
	out := make(map[string]string)
	for filePath, content := range additionalWorkspaceSources(aw, externalFiles) {
		if AdditionalWorkspaceFilePolicy(ws, aw, filePath) == openclawv1alpha1.WorkspaceFileUpdatePolicyReplace {
			out[filePath] = SourceContentHash(content)
		}
	}
	return out
}

// defaultWorkspaceSources returns the user-supplied source content for the
// default workspace, keyed by workspace-relative destination path.
//
// Operator-injected files (ENVIRONMENT.md, BOOTSTRAP.md, SELFCONFIG.md) and
// skill pack files are deliberately excluded: they have their own lifecycles
// (bootstrap.enabled, skillPackUpdatePolicy) and are not user-declared sources.
func defaultWorkspaceSources(ws *openclawv1alpha1.WorkspaceSpec, externalFiles map[string]string) map[string]string {
	out := make(map[string]string, len(externalFiles)+len(ws.InitialFiles))
	for k, v := range externalFiles {
		out[k] = v
	}
	// Inline files win over configMapRef, matching the seed merge priority.
	for k, v := range ws.InitialFiles {
		out[k] = v
	}
	return out
}

// additionalWorkspaceSources is defaultWorkspaceSources for a secondary workspace.
func additionalWorkspaceSources(aw *openclawv1alpha1.AdditionalWorkspace, externalFiles map[string]string) map[string]string {
	out := make(map[string]string, len(externalFiles)+len(aw.InitialFiles))
	for k, v := range externalFiles {
		out[k] = v
	}
	for k, v := range aw.InitialFiles {
		out[k] = v
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// ValidateManagedWorkspacePath rejects destinations that must never be written:
// absolute paths and ".." traversal, either of which would let a workspace
// declaration escape the workspace root.
func ValidateManagedWorkspacePath(filePath string) error {
	if filePath == "" {
		return fmt.Errorf("path must not be empty")
	}
	if strings.HasPrefix(filePath, "/") {
		return fmt.Errorf("path %q must be relative to the workspace root", filePath)
	}
	cleaned := path.Clean(filePath)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return fmt.Errorf("path %q must not traverse outside the workspace root", filePath)
	}
	return nil
}
