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
	"strings"
	"testing"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// Workspace file update policy (#576).

func TestWorkspaceFilePolicy_DefaultsToCreateOnly(t *testing.T) {
	if got := WorkspaceFilePolicy(nil, "AGENTS.md"); got != openclawv1alpha1.WorkspaceFileUpdatePolicyCreateOnly {
		t.Errorf("nil workspace policy = %q, want CreateOnly", got)
	}
	ws := &openclawv1alpha1.WorkspaceSpec{}
	if got := WorkspaceFilePolicy(ws, "AGENTS.md"); got != openclawv1alpha1.WorkspaceFileUpdatePolicyCreateOnly {
		t.Errorf("unset policy = %q, want CreateOnly (backward compatible)", got)
	}
}

func TestWorkspaceFilePolicy_WorkspaceLevelReplace(t *testing.T) {
	ws := &openclawv1alpha1.WorkspaceSpec{
		FileUpdatePolicy: openclawv1alpha1.WorkspaceFileUpdatePolicyReplace,
	}
	if got := WorkspaceFilePolicy(ws, "AGENTS.md"); got != openclawv1alpha1.WorkspaceFileUpdatePolicyReplace {
		t.Errorf("policy = %q, want Replace", got)
	}
}

// A per-path entry overrides the workspace default in both directions.
func TestWorkspaceFilePolicy_ManagedFilesOverride(t *testing.T) {
	ws := &openclawv1alpha1.WorkspaceSpec{
		ManagedFiles: []openclawv1alpha1.ManagedWorkspaceFile{
			{Path: "AGENTS.md", UpdatePolicy: openclawv1alpha1.WorkspaceFileUpdatePolicyReplace},
		},
	}
	if got := WorkspaceFilePolicy(ws, "AGENTS.md"); got != openclawv1alpha1.WorkspaceFileUpdatePolicyReplace {
		t.Errorf("AGENTS.md = %q, want Replace", got)
	}
	if got := WorkspaceFilePolicy(ws, "NOTES.md"); got != openclawv1alpha1.WorkspaceFileUpdatePolicyCreateOnly {
		t.Errorf("unlisted NOTES.md = %q, want CreateOnly", got)
	}

	pinned := &openclawv1alpha1.WorkspaceSpec{
		FileUpdatePolicy: openclawv1alpha1.WorkspaceFileUpdatePolicyReplace,
		ManagedFiles: []openclawv1alpha1.ManagedWorkspaceFile{
			{Path: "STATE.md", UpdatePolicy: openclawv1alpha1.WorkspaceFileUpdatePolicyCreateOnly},
		},
	}
	if got := WorkspaceFilePolicy(pinned, "STATE.md"); got != openclawv1alpha1.WorkspaceFileUpdatePolicyCreateOnly {
		t.Errorf("STATE.md = %q, want CreateOnly override", got)
	}
}

// Listing a path is an explicit ownership statement, so an omitted policy means
// Replace rather than falling back to the workspace default.
func TestWorkspaceFilePolicy_ManagedFileWithoutPolicyIsReplace(t *testing.T) {
	ws := &openclawv1alpha1.WorkspaceSpec{
		ManagedFiles: []openclawv1alpha1.ManagedWorkspaceFile{{Path: "AGENTS.md"}},
	}
	if got := WorkspaceFilePolicy(ws, "AGENTS.md"); got != openclawv1alpha1.WorkspaceFileUpdatePolicyReplace {
		t.Errorf("policy = %q, want Replace", got)
	}
}

// An additional workspace with no policy of its own inherits the top-level
// default, so the two cannot drift apart.
func TestAdditionalWorkspaceFilePolicy_InheritsTopLevel(t *testing.T) {
	ws := &openclawv1alpha1.WorkspaceSpec{
		FileUpdatePolicy:     openclawv1alpha1.WorkspaceFileUpdatePolicyReplace,
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{{Name: "print"}},
	}
	aw := &ws.AdditionalWorkspaces[0]
	if got := AdditionalWorkspaceFilePolicy(ws, aw, "AGENTS.md"); got != openclawv1alpha1.WorkspaceFileUpdatePolicyReplace {
		t.Errorf("inherited policy = %q, want Replace", got)
	}
}

func TestAdditionalWorkspaceFilePolicy_OwnPolicyWins(t *testing.T) {
	createOnly := openclawv1alpha1.WorkspaceFileUpdatePolicyCreateOnly
	ws := &openclawv1alpha1.WorkspaceSpec{
		FileUpdatePolicy: openclawv1alpha1.WorkspaceFileUpdatePolicyReplace,
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{Name: "print", FileUpdatePolicy: &createOnly},
		},
	}
	aw := &ws.AdditionalWorkspaces[0]
	if got := AdditionalWorkspaceFilePolicy(ws, aw, "AGENTS.md"); got != openclawv1alpha1.WorkspaceFileUpdatePolicyCreateOnly {
		t.Errorf("policy = %q, want the workspace's own CreateOnly", got)
	}
}

func TestValidateManagedWorkspacePath(t *testing.T) {
	valid := []string{"AGENTS.md", "docs/BOUNDARIES.md", "a/b/c.txt"}
	for _, p := range valid {
		if err := ValidateManagedWorkspacePath(p); err != nil {
			t.Errorf("path %q should be valid, got %v", p, err)
		}
	}

	invalid := []string{"", "/etc/passwd", "../escape.md", "a/../../escape.md"}
	for _, p := range invalid {
		if err := ValidateManagedWorkspacePath(p); err == nil {
			t.Errorf("path %q should be rejected", p)
		}
	}
}

// "a/../b.md" cleans to "b.md" -- inside the root, so it is allowed rather than
// rejected for merely containing "..".
func TestValidateManagedWorkspacePath_InternalTraversalStaysInRoot(t *testing.T) {
	if err := ValidateManagedWorkspacePath("a/../b.md"); err != nil {
		t.Errorf("a/../b.md resolves inside the workspace, got %v", err)
	}
}

// Init script rendering.

func newWorkspaceInstance(name string) *openclawv1alpha1.OpenClawInstance {
	instance := newTestInstance(name)
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{}
	return instance
}

func TestBuildInitScript_CreateOnlyKeepsSeedOnce(t *testing.T) {
	instance := newWorkspaceInstance("ws-create-only")
	instance.Spec.Workspace.InitialFiles = map[string]string{"AGENTS.md": "hello"}

	script := BuildInitScript(instance, nil, nil, nil)

	if !strings.Contains(script, "[ -f /data/workspace/'AGENTS.md' ] || cp") {
		t.Errorf("CreateOnly should emit the seed-once copy, got:\n%s", script)
	}
	if strings.Contains(script, "_ocw_apply") {
		t.Error("CreateOnly should not emit the converge helper")
	}
}

func TestBuildInitScript_ReplaceEmitsConverge(t *testing.T) {
	instance := newWorkspaceInstance("ws-replace")
	instance.Spec.Workspace.InitialFiles = map[string]string{"AGENTS.md": "hello"}
	instance.Spec.Workspace.ManagedFiles = []openclawv1alpha1.ManagedWorkspaceFile{
		{Path: "AGENTS.md", UpdatePolicy: openclawv1alpha1.WorkspaceFileUpdatePolicyReplace},
	}

	script := BuildInitScript(instance, nil, nil, nil)

	if !strings.Contains(script, "_ocw_apply()") {
		t.Errorf("Replace should emit the converge helper, got:\n%s", script)
	}
	hash := SourceContentHash("hello")
	if !strings.Contains(script, hash) {
		t.Errorf("script should carry the source hash %s, got:\n%s", hash, script)
	}
	if strings.Contains(script, "[ -f /data/workspace/'AGENTS.md' ] || cp") {
		t.Error("a Replace-managed file must not also be seeded once")
	}

	// Safety properties the issue requires.
	if !strings.Contains(script, `if [ -L "$2" ]; then`) {
		t.Error("converge helper must refuse symlink destinations")
	}
	if !strings.Contains(script, `[ -e "$2" ] && [ ! -f "$2" ]`) {
		t.Error("converge helper must refuse non-regular destinations")
	}
	if !strings.Contains(script, `mv -f "$2.ocwtmp" "$2"`) {
		t.Error("converge helper must use temp-file plus atomic rename")
	}
	if !strings.Contains(script, `chmod 0644`) {
		t.Error("converge helper must set deterministic permissions")
	}
	if strings.Contains(script, "rm -rf /data/workspace") {
		t.Error("converge must never recursively delete the workspace")
	}
}

// The source hash is what drives convergence, so a content change has to change
// the rendered script -- that is also what rolls the pod.
func TestBuildInitScript_ReplaceHashTracksSourceContent(t *testing.T) {
	build := func(content string) string {
		instance := newWorkspaceInstance("ws-hash")
		instance.Spec.Workspace.FileUpdatePolicy = openclawv1alpha1.WorkspaceFileUpdatePolicyReplace
		instance.Spec.Workspace.InitialFiles = map[string]string{"AGENTS.md": content}
		return BuildInitScript(instance, nil, nil, nil)
	}

	v1, v2 := build("v1"), build("v2")
	if v1 == v2 {
		t.Error("changing source content must change the rendered init script")
	}
	// Deterministic output matters beyond tidiness: a script that varied between
	// reconciles would roll the pod on every pass.
	if again := build("v1"); v1 != again {
		t.Error("identical input must render identically (deterministic output)")
	}
}

func TestBuildInitScript_ReplaceFromConfigMapRef(t *testing.T) {
	instance := newWorkspaceInstance("ws-replace-cmref")
	instance.Spec.Workspace.FileUpdatePolicy = openclawv1alpha1.WorkspaceFileUpdatePolicyReplace
	external := map[string]string{"BOUNDARIES.md": "no touching"}

	script := BuildInitScript(instance, external, nil, nil)

	if !strings.Contains(script, SourceContentHash("no touching")) {
		t.Errorf("configMapRef content should be Replace-managed, got:\n%s", script)
	}
}

// Operator-injected files keep their own lifecycles and are not swept into
// Replace by a workspace-level default.
func TestBuildInitScript_ReplaceLeavesOperatorFilesSeedOnce(t *testing.T) {
	instance := newWorkspaceInstance("ws-replace-operator")
	instance.Spec.Workspace.FileUpdatePolicy = openclawv1alpha1.WorkspaceFileUpdatePolicyReplace
	instance.Spec.Workspace.InitialFiles = map[string]string{"AGENTS.md": "hello"}

	script := BuildInitScript(instance, nil, nil, nil)

	if !strings.Contains(script, "[ -f /data/workspace/'ENVIRONMENT.md' ] || cp") {
		t.Errorf("ENVIRONMENT.md should stay seed-once, got:\n%s", script)
	}
	if !strings.Contains(script, "[ -f /data/workspace/'BOOTSTRAP.md' ] || cp") {
		t.Errorf("BOOTSTRAP.md should stay seed-once (it has bootstrap.enabled), got:\n%s", script)
	}
}

func TestBuildInitScript_ReplaceNestedPath(t *testing.T) {
	instance := newWorkspaceInstance("ws-replace-nested")
	instance.Spec.Workspace.InitialFiles = map[string]string{"docs/BOUNDARIES.md": "rules"}
	instance.Spec.Workspace.ManagedFiles = []openclawv1alpha1.ManagedWorkspaceFile{
		{Path: "docs/BOUNDARIES.md", UpdatePolicy: openclawv1alpha1.WorkspaceFileUpdatePolicyReplace},
	}

	script := BuildInitScript(instance, nil, nil, nil)

	if !strings.Contains(script, "mkdir -p /data/workspace/'docs'") {
		t.Errorf("nested Replace path should create its parent, got:\n%s", script)
	}
	if !strings.Contains(script, SourceContentHash("rules")) {
		t.Errorf("nested Replace path should carry its source hash, got:\n%s", script)
	}
	// The marker filename must encode the nested path so it cannot collide.
	if !strings.Contains(script, "/data/.workspace-managed/docs--BOUNDARIES.md") {
		t.Errorf("marker should encode the nested path, got:\n%s", script)
	}
}

func TestBuildInitScript_AdditionalWorkspaceReplace(t *testing.T) {
	instance := newWorkspaceInstance("ws-addl-replace")
	instance.Spec.Workspace.FileUpdatePolicy = openclawv1alpha1.WorkspaceFileUpdatePolicyReplace
	instance.Spec.Workspace.AdditionalWorkspaces = []openclawv1alpha1.AdditionalWorkspace{
		{Name: "print", InitialFiles: map[string]string{"AGENTS.md": "print rules"}},
	}

	script := BuildInitScript(instance, nil, nil, nil)

	if !strings.Contains(script, SourceContentHash("print rules")) {
		t.Errorf("additional workspace should inherit Replace, got:\n%s", script)
	}
	// Markers are namespaced per workspace so the same path in two workspaces
	// tracks independently.
	if !strings.Contains(script, "/data/.workspace-managed/ws-print--AGENTS.md") {
		t.Errorf("marker should be scoped to the workspace, got:\n%s", script)
	}
}

func TestBuildInitScript_AdditionalWorkspaceCreateOnlyOverride(t *testing.T) {
	createOnly := openclawv1alpha1.WorkspaceFileUpdatePolicyCreateOnly
	instance := newWorkspaceInstance("ws-addl-create-only")
	instance.Spec.Workspace.FileUpdatePolicy = openclawv1alpha1.WorkspaceFileUpdatePolicyReplace
	instance.Spec.Workspace.AdditionalWorkspaces = []openclawv1alpha1.AdditionalWorkspace{
		{
			Name:             "print",
			FileUpdatePolicy: &createOnly,
			InitialFiles:     map[string]string{"AGENTS.md": "print rules"},
		},
	}

	script := BuildInitScript(instance, nil, nil, nil)

	if !strings.Contains(script, "[ -f /data/'workspace-print'/'AGENTS.md' ] || cp") {
		t.Errorf("workspace-level CreateOnly should keep seed-once, got:\n%s", script)
	}
	if strings.Contains(script, SourceContentHash("print rules")) {
		t.Error("a CreateOnly workspace must not emit a converge call")
	}
}

// Status reporting.

func TestManagedWorkspaceFileStatuses(t *testing.T) {
	instance := newWorkspaceInstance("ws-status")
	instance.Spec.Workspace.InitialFiles = map[string]string{"AGENTS.md": "hello"}
	instance.Spec.Workspace.ManagedFiles = []openclawv1alpha1.ManagedWorkspaceFile{
		{Path: "AGENTS.md", UpdatePolicy: openclawv1alpha1.WorkspaceFileUpdatePolicyReplace},
		// Declared but not yet provided by any source.
		{Path: "FUTURE.md", UpdatePolicy: openclawv1alpha1.WorkspaceFileUpdatePolicyReplace},
	}

	statuses := ManagedWorkspaceFileStatuses(instance, nil, nil)

	byPath := make(map[string]openclawv1alpha1.ManagedWorkspaceFileStatus)
	for _, s := range statuses {
		byPath[s.Path] = s
	}

	agents, ok := byPath["AGENTS.md"]
	if !ok {
		t.Fatal("AGENTS.md should be reported")
	}
	if agents.SourceHash != SourceContentHash("hello") {
		t.Errorf("AGENTS.md hash = %q, want the source hash", agents.SourceHash)
	}
	if agents.UpdatePolicy != openclawv1alpha1.WorkspaceFileUpdatePolicyReplace {
		t.Errorf("AGENTS.md policy = %q, want Replace", agents.UpdatePolicy)
	}

	future, ok := byPath["FUTURE.md"]
	if !ok {
		t.Fatal("a managedFiles path with no source should still be reported as managed")
	}
	if future.SourceHash != "" {
		t.Errorf("FUTURE.md hash = %q, want empty (no source yet)", future.SourceHash)
	}
}

// CreateOnly files aren't declaratively managed, so they don't clutter status.
func TestManagedWorkspaceFileStatuses_OmitsCreateOnly(t *testing.T) {
	instance := newWorkspaceInstance("ws-status-create-only")
	instance.Spec.Workspace.InitialFiles = map[string]string{"AGENTS.md": "hello"}

	if statuses := ManagedWorkspaceFileStatuses(instance, nil, nil); len(statuses) != 0 {
		t.Errorf("expected no managed file statuses, got %+v", statuses)
	}
}

func TestManagedWorkspaceFileStatuses_AdditionalWorkspace(t *testing.T) {
	instance := newWorkspaceInstance("ws-status-addl")
	instance.Spec.Workspace.AdditionalWorkspaces = []openclawv1alpha1.AdditionalWorkspace{
		{
			Name:         "print",
			InitialFiles: map[string]string{"AGENTS.md": "print rules"},
			ManagedFiles: []openclawv1alpha1.ManagedWorkspaceFile{
				{Path: "AGENTS.md", UpdatePolicy: openclawv1alpha1.WorkspaceFileUpdatePolicyReplace},
			},
		},
	}

	statuses := ManagedWorkspaceFileStatuses(instance, nil, nil)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d: %+v", len(statuses), statuses)
	}
	if statuses[0].Workspace != "print" {
		t.Errorf("workspace = %q, want print", statuses[0].Workspace)
	}
	if statuses[0].SourceHash != SourceContentHash("print rules") {
		t.Errorf("hash = %q, want the source hash", statuses[0].SourceHash)
	}
}
