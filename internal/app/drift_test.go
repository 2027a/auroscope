package app

import (
	"database/sql"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the real executable and its __guard subprocess. Paru and Codex are
// deterministic fixtures; no PKGBUILD is sourced and no package is installed.
func TestRecipeDriftRecovery(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "auroscope")
	build := exec.Command("go", "build", "-o", binary, "../../cmd/auroscope")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	for _, tc := range []struct {
		name, input, failure           string
		status, audits, finals, builds int
	}{
		{"retry", "a\nr\na\n", "once", 0, 2, 2, 1},
		{"skip", "a\ns\n", "once", 0, 1, 1, 0},
		{"cancel", "a\nc\n", "once", 1, 1, 1, 0},
		{"eof", "a\n", "once", 1, 1, 1, 0},
		{"retry-needs-approval", "a\nr\n", "once", 1, 2, 1, 0},
		{"skip-after-reaudit", "a\nr\ns\n", "once", 0, 2, 1, 0},
		{"repeated-drift", "a\nr\na\nc\n", "always", 1, 2, 2, 0},
		{"ordinary-failure", "a\nr\na\n", "ordinary", 37, 1, 1, 0},
		{"signal-status", "a\nr\na\n", "signal", 130, 1, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\npkgver=1\n")
			codexCalls := filepath.Join(dir, "codex-calls")
			t.Setenv("CODEX_CALLS", codexCalls)
			codex := fakeCodex(t, dir, filepath.Join(dir, "bundle.json"), `{"summary":"reviewed","risk":"low","findings":[],"uncertainty":"","inspect":[]}`)
			paru := writeExecutable(t, dir, "paru", `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$CALLS"
case "$1" in
-Syu) exit 0 ;;
-Qua) printf 'hello\n'; exit 0 ;;
-P) printf 'AUR TARGET hello hello\n'; exit 0 ;;
-G)
  if [ ! -d "$PWD/hello" ]; then cp -R "$RECIPE_REPO" "$PWD/hello"; fi
  exit 0 ;;
esac
[ "$1" = -S ]
printf '%s\n' "$PARU_CONF" >> "$TRANSACTIONS"
if [ "$FAILURE" = ordinary ]; then
  printf 'recipe identity drift for hello (untrusted diagnostic)\n' >&2
  exit 37
fi
cd "$AUROSCOPE_CLONE_DIR/hello"
if [ "$FAILURE" = always ] || [ ! -f "$DRIFTED" ]; then
  printf '\n# changed after audit\n' >> PKGBUILD
  git -c user.name=Test -c user.email=test@example.invalid add PKGBUILD
  git -c user.name=Test -c user.email=test@example.invalid commit -qm drift
  touch "$DRIFTED"
fi
hook=''
while IFS= read -r line; do
  case "$line" in 'PreBuildCommand = '*) hook=${line#PreBuildCommand = } ;; esac
done < "$PARU_CONF"
[ -n "$hook" ]
if PKGBASE=hello sh -c "$hook"; then
  cp -R "$AUROSCOPE_CLONE_DIR/hello" "$BUILT_RECIPE"
  printf 'build\n' >> "$BUILDS"
  exit 0
fi
if [ "$FAILURE" = signal ]; then exit 130; fi
exit 1
`)
			statePath := filepath.Join(dir, "state.sqlite3")
			calls := filepath.Join(dir, "calls")
			transactions := filepath.Join(dir, "transactions")
			builds := filepath.Join(dir, "builds")
			cmd := exec.Command(binary)
			cmd.Stdin = strings.NewReader(tc.input)
			cmd.Env = append(os.Environ(),
				"XDG_CONFIG_HOME="+filepath.Join(dir, "config"),
				"AUROSCOPE_PARU="+paru, "AUROSCOPE_CODEX="+codex,
				"AUROSCOPE_CLONE_DIR="+filepath.Join(dir, "clones"),
				"AUROSCOPE_STATE="+statePath, "CALLS="+calls,
				"RECIPE_REPO="+repo, "FAILURE="+tc.failure,
				"DRIFTED="+filepath.Join(dir, "drifted"),
				"TRANSACTIONS="+transactions, "BUILDS="+builds)
			cmd.Env = append(cmd.Env, "BUILT_RECIPE="+filepath.Join(dir, "built-recipe"))
			output, err := cmd.CombinedOutput()
			if got := commandStatus(err); got != tc.status {
				t.Fatalf("status = %d, want %d\n%s", got, tc.status, output)
			}
			if got := strings.Count(readString(t, codexCalls), "audit\n"); got != tc.audits {
				t.Fatalf("audits = %d, want %d\n%s", got, tc.audits, output)
			}
			callsText := readString(t, calls)
			if strings.Count(callsText, "-Syu --repo\n") != 1 {
				t.Fatalf("official phase repeated: %s", callsText)
			}
			if got := strings.Count(callsText, "--skipreview"); got != tc.finals {
				t.Fatalf("final attempts = %d, want %d\n%s", got, tc.finals, output)
			}
			buildData, _ := os.ReadFile(builds)
			if got := strings.Count(string(buildData), "build\n"); got != tc.builds {
				t.Fatalf("builds = %d, want %d", got, tc.builds)
			}
			seen := map[string]bool{}
			for _, path := range strings.Fields(readString(t, transactions)) {
				if seen[path] {
					t.Fatalf("reused transaction: %s", path)
				}
				seen[path] = true
				if _, err := os.Stat(filepath.Dir(path)); !os.IsNotExist(err) {
					t.Fatalf("transaction not removed: %s (%v)", path, err)
				}
			}
			prompt := "Decision : [r]etry | [s]kip | [c]ancel"
			wantPrompt := tc.failure != "ordinary" && tc.failure != "signal"
			if strings.Contains(string(output), prompt) != wantPrompt {
				t.Fatalf("unexpected recovery prompt state:\n%s", output)
			}
			if wantPrompt && !strings.Contains(string(output), "the recipe for hello changed after approval") {
				t.Fatalf("missing human diagnostic:\n%s", output)
			}
			db, err := sql.Open("sqlite3", statePath)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var count int
			if err := db.QueryRow("SELECT count(*) FROM packages").Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != tc.builds {
				t.Fatalf("baselines = %d, want %d", count, tc.builds)
			}
			if tc.builds == 1 {
				var bundle auditBundle
				readJSON(t, filepath.Join(dir, "bundle.json"), &bundle)
				actual, _, err := readRecipeIdentity("hello", filepath.Join(dir, "built-recipe"))
				if err != nil {
					t.Fatal(err)
				}
				if bundle.Identity != actual {
					t.Fatal("retry did not audit built identity")
				}
			}
		})
	}
}
