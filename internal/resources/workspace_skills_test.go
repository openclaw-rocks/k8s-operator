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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	openclawv1alpha1 "github.com/paperclipinc/openclaw-operator/api/v1alpha1"
)

// newWorkspaceSkillsInstance returns an instance with one additional workspace
// named "secondary" declaring the given skills.
func newWorkspaceSkillsInstance(name string, skills ...string) *openclawv1alpha1.OpenClawInstance {
	instance := newTestInstance(name)
	instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
		AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
			{Name: "secondary", Skills: skills},
		},
	}
	return instance
}

// wsTestPacks wraps testPacks output as workspace-scoped packs for "secondary".
func wsTestPacks(files map[string]string) *ResolvedSkillPacks {
	return &ResolvedSkillPacks{
		Workspaces: map[string]*ResolvedSkillPacks{
			"secondary": testPacks(files),
		},
	}
}

func TestBuildWorkspaceConfigMap_AdditionalWorkspaceSkillPacks(t *testing.T) {
	instance := newWorkspaceSkillsInstance("ws-packs", "pack:acme/skills/example@v1")
	packs := wsTestPacks(map[string]string{
		"skills/example/SKILL.md": "# from pack",
	})

	cm := BuildWorkspaceConfigMap(instance, nil, nil, packs)
	if cm == nil {
		t.Fatal("expected workspace ConfigMap")
	}

	key := AdditionalWorkspaceCMKey("secondary", SkillPackCMKey("skills/example/SKILL.md"))
	if got := cm.Data[key]; got != "# from pack" {
		t.Errorf("expected workspace pack file under %q, got %q", key, got)
	}

	// Default-workspace keys must not contain the workspace pack file.
	if _, ok := cm.Data[SkillPackCMKey("skills/example/SKILL.md")]; ok {
		t.Error("workspace pack file must not leak into default workspace keys")
	}
}

func TestBuildWorkspaceConfigMap_InlineFilesOverrideWorkspacePacks(t *testing.T) {
	instance := newWorkspaceSkillsInstance("ws-packs-priority", "pack:acme/skills/example@v1")
	instance.Spec.Workspace.AdditionalWorkspaces[0].InitialFiles = map[string]string{
		"skills/example/SKILL.md": "# user override",
	}
	packs := wsTestPacks(map[string]string{
		"skills/example/SKILL.md": "# from pack",
	})

	cm := BuildWorkspaceConfigMap(instance, nil, nil, packs)

	key := AdditionalWorkspaceCMKey("secondary", SkillPackCMKey("skills/example/SKILL.md"))
	if got := cm.Data[key]; got != "# user override" {
		t.Errorf("inline initialFiles should override pack files, got %q", got)
	}
}

func TestBuildInitScript_AdditionalWorkspaceSkillPacks_Replace(t *testing.T) {
	instance := newWorkspaceSkillsInstance("ws-replace", "pack:acme/skills/example@v1")
	packs := wsTestPacks(map[string]string{
		"skills/example/SKILL.md": "# v1",
	})
	packs.Workspaces["secondary"].Directories = []string{"skills/example"}

	script := BuildInitScript(instance, nil, nil, packs)

	srcKey := AdditionalWorkspaceCMKey("secondary", SkillPackCMKey("skills/example/SKILL.md"))
	if !strings.Contains(script, "mkdir -p /data/'workspace-secondary'/'skills/example'") {
		t.Errorf("expected pack directory creation in the workspace, got:\n%s", script)
	}
	if !strings.Contains(script, "cp /workspace-init/'"+srcKey+"' /data/workspace-secondary/'skills/example/SKILL.md'") {
		t.Errorf("expected unconditional cp with namespaced source key, got:\n%s", script)
	}
	if !strings.Contains(script, "printf '%s\\n' 'skills/example/SKILL.md' > /data/.skillpack-manifest-ws-secondary.new") {
		t.Errorf("expected per-workspace manifest write, got:\n%s", script)
	}
	if !strings.Contains(script, "mv /data/.skillpack-manifest-ws-secondary.new /data/.skillpack-manifest-ws-secondary") {
		t.Errorf("expected per-workspace manifest promotion, got:\n%s", script)
	}
	if strings.Contains(script, "[ -f /data/workspace-secondary/'skills/example/SKILL.md' ] ||") {
		t.Error("Replace policy must not seed workspace pack files conditionally")
	}
}

func TestBuildInitScript_AdditionalWorkspaceSkillPacks_CreateOnly(t *testing.T) {
	instance := newWorkspaceSkillsInstance("ws-createonly", "pack:acme/skills/example@v1")
	instance.Spec.SkillPackUpdatePolicy = SkillPackUpdatePolicyCreateOnly
	packs := wsTestPacks(map[string]string{
		"skills/example/SKILL.md": "# v1",
	})

	script := BuildInitScript(instance, nil, nil, packs)

	srcKey := AdditionalWorkspaceCMKey("secondary", SkillPackCMKey("skills/example/SKILL.md"))
	want := "[ -f /data/'workspace-secondary'/'skills/example/SKILL.md' ] || cp /workspace-init/'" + srcKey + "' /data/'workspace-secondary'/'skills/example/SKILL.md'"
	if !strings.Contains(script, want) {
		t.Errorf("CreateOnly policy should seed workspace pack files only when absent, got:\n%s", script)
	}
	if strings.Contains(script, ".skillpack-manifest-ws-secondary") {
		t.Errorf("CreateOnly policy must not touch the workspace pack manifest, got:\n%s", script)
	}
}

func TestBuildInitScript_AdditionalWorkspacePacksRemoved_Cleanup(t *testing.T) {
	// Workspace declared but no packs resolved: the script must still run the
	// manifest-guarded cleanup so files seeded by a previous revision converge.
	instance := newWorkspaceSkillsInstance("ws-removed")

	script := BuildInitScript(instance, nil, nil, nil)

	if !strings.Contains(script, "if [ -f /data/.skillpack-manifest-ws-secondary ]; then") {
		t.Errorf("expected manifest-guarded cleanup for the workspace, got:\n%s", script)
	}
	if !strings.Contains(script, "rm -f /data/.skillpack-manifest-ws-secondary.new /data/.skillpack-manifest-ws-secondary") {
		t.Errorf("expected workspace manifest removal after cleanup, got:\n%s", script)
	}
}

func TestBuildSkillsScript_WorkspaceSkills(t *testing.T) {
	instance := newWorkspaceSkillsInstance("ws-skills",
		"@acme/browser-use", "npm:@acme/cli-tool", "pack:acme/skills/example@v1")

	script := BuildSkillsScript(instance)

	if !strings.Contains(script, "_install_skill '@acme/browser-use' '/home/openclaw/.openclaw/workspace-secondary'") {
		t.Errorf("expected workspace-scoped clawhub install with workdir, got:\n%s", script)
	}
	if !strings.Contains(script, "mkdir -p '/home/openclaw/.openclaw/workspace-secondary'") {
		t.Errorf("expected workspace dir creation before clawhub install, got:\n%s", script)
	}
	if !strings.Contains(script, "npm install -g '@acme/cli-tool'") {
		t.Errorf("expected global npm install for npm: workspace entry, got:\n%s", script)
	}
	if strings.Contains(script, "pack:") {
		t.Errorf("pack: entries must not reach the skills init container, got:\n%s", script)
	}
	// Only workspace ClawHub skills: the wrapper is needed, the global
	// /app/skills symlink setup is not.
	if !strings.Contains(script, "_install_skill() {") {
		t.Errorf("expected the install wrapper to be emitted, got:\n%s", script)
	}
	if strings.Contains(script, "rm -rf /app/skills") {
		t.Errorf("global skills setup should not run without top-level clawhub skills, got:\n%s", script)
	}
}

func TestBuildSkillsScript_TopLevelOutputUnchanged(t *testing.T) {
	// Instances without workspace skills must produce the same script as
	// before so existing pods do not roll on operator upgrade.
	instance := newTestInstance("top-level-only")
	instance.Spec.Skills = []string{"@acme/browser-use", "npm:@acme/cli-tool"}

	script := BuildSkillsScript(instance)

	wantOrder := []string{
		"set -e",
		"rm -rf /app/skills",
		"_install_skill() {",
		"_install_skill '@acme/browser-use'\n",
		"npm install -g '@acme/cli-tool'",
	}
	pos := 0
	for _, want := range wantOrder {
		idx := strings.Index(script[pos:], want)
		if idx < 0 {
			t.Fatalf("expected %q (in order) in script:\n%s", want, script)
		}
		pos += idx
	}
}

func TestBuildSkillsScript_WorkspaceOnlyPackSkills_Empty(t *testing.T) {
	instance := newWorkspaceSkillsInstance("ws-pack-only", "pack:acme/skills/example@v1")
	if script := BuildSkillsScript(instance); script != "" {
		t.Errorf("pack-only workspace skills should not create a skills script, got:\n%s", script)
	}
}

func TestCalculateConfigHash_WorkspaceSkillsKeyedByName(t *testing.T) {
	base := newTestInstance("hash-base")

	inWs := func(name, skill string) *openclawv1alpha1.OpenClawInstance {
		instance := newTestInstance("hash-ws")
		instance.Spec.Workspace = &openclawv1alpha1.WorkspaceSpec{
			AdditionalWorkspaces: []openclawv1alpha1.AdditionalWorkspace{
				{Name: "first"},
				{Name: "second"},
			},
		}
		for i := range instance.Spec.Workspace.AdditionalWorkspaces {
			if instance.Spec.Workspace.AdditionalWorkspaces[i].Name == name {
				instance.Spec.Workspace.AdditionalWorkspaces[i].Skills = []string{skill}
			}
		}
		return instance
	}

	skill := "pack:acme/skills/example@v1"
	inFirst := calculateConfigHash(inWs("first", skill), nil, nil, nil)
	inSecond := calculateConfigHash(inWs("second", skill), nil, nil, nil)
	baseHash := calculateConfigHash(base, nil, nil, nil)

	if inFirst == inSecond {
		t.Error("moving a skill between workspaces must change the config hash")
	}
	if inFirst == baseHash || inSecond == baseHash {
		t.Error("declaring workspace skills must change the config hash")
	}

	// Top-level skill vs the same skill in a workspace must differ too.
	topLevel := newTestInstance("hash-top")
	topLevel.Spec.Skills = []string{skill}
	if calculateConfigHash(topLevel, nil, nil, nil) == inFirst {
		t.Error("workspace-scoped skill must hash differently from a top-level skill")
	}
}

func TestCombinedSkillEntries_MergesWorkspaces(t *testing.T) {
	sp := &ResolvedSkillPacks{
		SkillEntries: map[string]interface{}{"default-skill": map[string]interface{}{"enabled": true}},
		Workspaces: map[string]*ResolvedSkillPacks{
			"secondary": {SkillEntries: map[string]interface{}{"ws-skill": map[string]interface{}{"enabled": true}}},
		},
	}

	merged := combinedSkillEntries(sp)
	if _, ok := merged["default-skill"]; !ok {
		t.Error("expected default workspace skill entry")
	}
	if _, ok := merged["ws-skill"]; !ok {
		t.Error("expected additional workspace skill entry")
	}

	if combinedSkillEntries(nil) != nil {
		t.Error("nil skill packs should produce nil entries")
	}
}

func TestBuildStatefulSet_WorkspaceSkills_VolumesAndPath(t *testing.T) {
	instance := newWorkspaceSkillsInstance("ws-sts", "@acme/browser-use", "npm:@acme/cli-tool")

	sts := BuildStatefulSet(instance, "", nil, nil, nil)

	var hasSkillsInit, hasSkillsTmp bool
	for _, c := range sts.Spec.Template.Spec.InitContainers {
		if c.Name == "init-skills" {
			hasSkillsInit = true
		}
	}
	for _, v := range sts.Spec.Template.Spec.Volumes {
		if v.Name == "skills-tmp" {
			hasSkillsTmp = true
		}
	}
	if !hasSkillsInit {
		t.Error("expected init-skills container for workspace-only skills")
	}
	if !hasSkillsTmp {
		t.Error("expected skills-tmp volume for workspace-only skills")
	}

	// npm: workspace skills install binaries into ~/.local/bin - PATH must include it.
	var mainPath string
	for _, c := range sts.Spec.Template.Spec.Containers {
		if c.Name == "openclaw" {
			for _, e := range c.Env {
				if e.Name == "PATH" {
					mainPath = e.Value
				}
			}
		}
	}
	if !strings.Contains(mainPath, RuntimeDepsLocalBin) {
		t.Errorf("expected PATH to include %s for workspace npm skills, got %q", RuntimeDepsLocalBin, mainPath)
	}
}

// runWorkspaceSkillPackSync renders the per-workspace Replace-mode sync,
// rebases the hardcoded /data and /workspace-init paths into temp dirs, and
// executes it with sh (mirrors runSkillPackSync for the default workspace).
func runWorkspaceSkillPackSync(t *testing.T, dataDir, initDir, wsName string, files map[string]string) {
	t.Helper()

	var packs *ResolvedSkillPacks
	if len(files) > 0 {
		packs = testPacks(files)
		for cmKey, content := range packs.Files {
			nsKey := AdditionalWorkspaceCMKey(wsName, cmKey)
			if err := os.WriteFile(filepath.Join(initDir, nsKey), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	script := strings.Join(buildWorkspaceSkillPackSyncLines(packs, wsName), "\n")
	script = strings.ReplaceAll(script, "/data", dataDir)
	script = strings.ReplaceAll(script, "/workspace-init", initDir)

	cmd := exec.Command("sh", "-c", script)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("workspace sync script failed: %v\noutput: %s\nscript:\n%s", err, out, script)
	}
}

// TestWorkspaceSkillPackSync_ShellBehavior verifies the per-workspace sync
// converges the workspace to the declared revision and never touches other
// workspaces or the default workspace (#568).
func TestWorkspaceSkillPackSync_ShellBehavior(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}

	dataDir := t.TempDir()
	initDir := t.TempDir()
	wsDir := filepath.Join(dataDir, "workspace-secondary")
	otherDir := filepath.Join(dataDir, "workspace-other")
	defaultDir := filepath.Join(dataDir, "workspace")
	for _, d := range []string{wsDir, otherDir, defaultDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// The same relative path exists in the other and default workspaces; the
	// secondary sync must never touch them.
	otherFile := filepath.Join(otherDir, "skills", "example", "SKILL.md")
	defaultFile := filepath.Join(defaultDir, "skills", "example", "SKILL.md")
	for _, f := range []string{otherFile, defaultFile} {
		if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(f, []byte("untouched"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Revision 1.
	runWorkspaceSkillPackSync(t, dataDir, initDir, "secondary", map[string]string{
		"skills/example/SKILL.md": "# rev1",
	})
	got, err := os.ReadFile(filepath.Join(wsDir, "skills", "example", "SKILL.md"))
	if err != nil || string(got) != "# rev1" {
		t.Fatalf("rev1 seed failed: %v, content %q", err, got)
	}

	// Revision 2: renamed skill converges, stale files pruned.
	runWorkspaceSkillPackSync(t, dataDir, initDir, "secondary", map[string]string{
		"skills/renamed/SKILL.md": "# rev2",
	})
	got, err = os.ReadFile(filepath.Join(wsDir, "skills", "renamed", "SKILL.md"))
	if err != nil || string(got) != "# rev2" {
		t.Fatalf("rev2 content not converged: %v, content %q", err, got)
	}
	if _, err := os.Stat(filepath.Join(wsDir, "skills", "example")); !os.IsNotExist(err) {
		t.Error("stale rev1 skill directory should be pruned")
	}

	// All packs removed: workspace cleaned up, manifest dropped.
	runWorkspaceSkillPackSync(t, dataDir, initDir, "secondary", nil)
	if _, err := os.Stat(filepath.Join(wsDir, "skills", "renamed")); !os.IsNotExist(err) {
		t.Error("removing all workspace packs should clean up seeded files")
	}
	if _, err := os.Stat(filepath.Join(dataDir, ".skillpack-manifest-ws-secondary")); !os.IsNotExist(err) {
		t.Error("workspace manifest should be removed when no packs are declared")
	}

	// Other workspaces and the default workspace were never touched.
	for _, f := range []string{otherFile, defaultFile} {
		if got, _ := os.ReadFile(f); string(got) != "untouched" {
			t.Errorf("file %s must not be touched by the secondary workspace sync", f)
		}
	}
}
