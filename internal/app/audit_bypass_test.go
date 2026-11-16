package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Pinned Paru: --repo, --aur/-a and --pkgbuilds replace the mode; --mode adds
// to it. Only a final repository-only mode may skip the audited path.
func TestRepoModeReopenedForAURIsAudited(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		audited bool
	}{
		{[]string{"-S", "--repo", "--mode=aur", "foo"}, true},
		{[]string{"-S", "--repo", "--mode", "aur", "foo"}, true},
		{[]string{"-S", "--repo", "--mode=a", "foo"}, true},
		{[]string{"-S", "--repo", "--mode=ra", "foo"}, true},
		{[]string{"-S", "--repo", "--mode=repo,aur", "foo"}, true},
		{[]string{"-S", "--repo", "--mode=all", "foo"}, true},
		{[]string{"-S", "--repo", "--mode=pkgbuilds", "foo"}, true},
		{[]string{"-S", "--repo", "--aur", "foo"}, true},
		{[]string{"-S", "--repo", "-a", "foo"}, true},
		{[]string{"-S", "--repo", "-ay", "foo"}, true},
		{[]string{"-S", "--repo", "--pkgbuilds", "foo"}, true},
		{[]string{"-S", "--mode=repo", "foo"}, true},
		{[]string{"-S", "--repo", "foo"}, false},
		{[]string{"-Sa", "--repo", "foo"}, false},
		{[]string{"-S", "--aur", "--repo", "foo"}, false},
		{[]string{"-S", "--mode=aur", "--repo", "foo"}, false},
		{[]string{"-S", "--repo", "--mode=repo", "foo"}, false},
		{[]string{"-S", "--repo", "--mode=r", "foo"}, false},
		{[]string{"-S", "--repo", "--ignore", "--aur", "foo"}, false},
		{[]string{"-S", "--repo", "--ignore=-a", "foo"}, false},
		{[]string{"-S", "--repo", "-r", "-a", "foo"}, false},
		{[]string{"-S", "--repo", "--", "--aur"}, false},
	} {
		t.Run(fmt.Sprint(tc.args), func(t *testing.T) {
			if got := commandMayBuildAUR(tc.args); got != tc.audited {
				t.Fatalf("commandMayBuildAUR = %v, want %v", got, tc.audited)
			}
		})
	}
}

// Issue #108: the post-edit snapshot holds hostile recipe content. It must not
// use the user's signing key or run global, template or repository hooks.
func TestEditSnapshotIgnoresSigningAndHooks(t *testing.T) {
	dir := t.TempDir()
	repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\n")
	marker := filepath.Join(dir, "hook-ran")
	hooks := filepath.Join(dir, "hooks")
	if err := os.Mkdir(hooks, 0o755); err != nil {
		t.Fatal(err)
	}
	hook := "#!/bin/sh\n: > " + posixShellQuote(marker) + "\nexit 1\n"
	for _, name := range []string{"pre-commit", "commit-msg", "post-commit"} {
		writeExecutable(t, hooks, name, hook)
		writeExecutable(t, filepath.Join(repo, ".git", "hooks"), name, hook)
	}
	global := filepath.Join(dir, "gitconfig")
	config := "[commit]\n\tgpgsign = true\n[gpg]\n\tformat = ssh\n[user]\n\tsigningkey = " + filepath.Join(dir, "absent-key") + "\n[core]\n\thooksPath = " + hooks + "\n"
	if err := os.WriteFile(global, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	if err := os.WriteFile(filepath.Join(repo, "PKGBUILD"), []byte("pkgname=hello\npkgrel=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := snapshotEditedWorktree(repo, map[string]bool{}); err != nil {
		t.Fatalf("snapshot used signing or hooks: %v", err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("a hook ran during the snapshot")
	}
	raw, err := gitOutput(repo, "cat-file", "commit", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "gpgsig") || !strings.Contains(raw, "AURoscope reviewed edit") {
		t.Fatalf("unexpected snapshot commit:\n%s", raw)
	}
}

// Issue #107: aur/hello must reach the final install qualified. Unqualified,
// Paru may install a repository hello while the audited AUR commit becomes the
// baseline of a recipe that was never built.
func TestQualifiedTargetsReachFinalInstall(t *testing.T) {
	for _, decision := range []string{"approve", "skip"} {
		t.Run(decision, func(t *testing.T) {
			dir := t.TempDir()
			repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\npkgver=1\n")
			calls := filepath.Join(dir, "calls")
			paru := fakeParu(t, dir, calls, repo, "REPO TARGET core tree\nAUR TARGET hello hello\n")
			codex := fakeCodex(t, dir, filepath.Join(dir, "bundle"), `{"summary":"checked","risk":"low","findings":[],"uncertainty":"","inspect":[]}`)
			var output bytes.Buffer
			status := runOptionFixture(t, []string{"-S", "core/tree", "aur/hello"}, runConfig{
				paruPath: paru, codexPath: codex,
				statePath: filepath.Join(dir, "state"), cloneDir: filepath.Join(dir, "clones"),
				stdin: strings.NewReader(decision + "\n"), stdout: &output, stderr: &output,
			})
			if status != 0 {
				t.Fatalf("status=%d: %s", status, &output)
			}
			wantFinal := "-S -- core/tree"
			if decision == "approve" {
				wantFinal = "-S --skipreview -- core/tree aur/hello"
			}
			want := []string{"-P --order -- core/tree aur/hello", "-G hello", wantFinal}
			if got := strings.Split(strings.TrimSpace(readString(t, calls)), "\n"); !reflect.DeepEqual(got, want) {
				t.Fatalf("calls=%q, want %q", got, want)
			}
		})
	}
}
