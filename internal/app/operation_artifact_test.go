package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Runs against the installed artifact in the Arch gate. Recipes are inert;
// only test-owned executables create archives and simulate Paru completion.
func TestTemporaryArchivesLifecycle(t *testing.T) {
	binary := os.Getenv("AUROSCOPE_TEST_BINARY")
	if binary == "" {
		binary = filepath.Join(t.TempDir(), "auroscope")
		if out, err := exec.Command("go", "build", "-o", binary, "../../cmd/auroscope").CombinedOutput(); err != nil {
			t.Fatalf("build: %v\n%s", err, out)
		}
	}
	// TMPDIR may be on disk or tmpfs. Only explicitly selected roots claim a
	// filesystem type; keep exercising the normal temporary root on either.
	roots := []struct{ name, parent string }{{"default", ""}}
	if parent := os.Getenv("AUROSCOPE_TEST_DISK_ROOT"); parent != "" {
		roots = append(roots, struct{ name, parent string }{"disk", parent})
	}
	if parent := os.Getenv("AUROSCOPE_TEST_TMPFS_ROOT"); parent != "" {
		roots = append(roots, struct{ name, parent string }{"tmpfs", parent})
	}
	for _, fs := range roots {
		t.Run(fs.name, func(t *testing.T) {
			dir := t.TempDir()
			root := filepath.Join(dir, "clones")
			if fs.parent != "" {
				var err error
				root, err = os.MkdirTemp(fs.parent, "auroscope-cleanup-test-")
				if err != nil {
					t.Fatal(err)
				}
				defer os.RemoveAll(root)
			}
			if err := os.MkdirAll(root, 0o700); err != nil {
				t.Fatal(err)
			}
			var stat syscall.Statfs_t
			if err := syscall.Statfs(root, &stat); err != nil {
				t.Fatal(err)
			}
			if fs.name != "default" && (stat.Type == 0x01021994) != (fs.name == "tmpfs") {
				t.Fatalf("unexpected filesystem %x for %s", stat.Type, fs.name)
			}
			t.Logf("verified %s root filesystem type %x", fs.name, stat.Type)
			repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\npkgver=1\n")
			paru := writeExecutable(t, dir, "paru", `#!/bin/sh
set -eu
case "$1" in
-P) printf 'AUR TARGET hello hello\n'; exit 0 ;;
-G)
  cp -R "$RECIPE_REPO" "$PWD/hello"
  printf source > "$PWD/hello/source.tar.gz"
  printf '%s' "$PWD" > "$ACTIVE_CLONES"
  if [ "$SCENARIO" = acquisition-failure ]; then exit 38; fi
  exit 0 ;;
esac
[ "$1" = -S ]
clones=''
while IFS= read -r line; do
  case "$line" in 'CloneDir = '*) clones=${line#CloneDir = } ;; esac
done < "$PARU_CONF"
[ "$clones" = "$(cat "$ACTIVE_CLONES")" ]
[ -f "$clones/hello/source.tar.gz" ]
printf package > "$clones/hello/hello.pkg.tar.zst"
mkdir -p "$clones/hello/pkg/nested"
printf artifact > "$clones/hello/pkg/nested/file"
chmod 000 "$clones/hello/pkg"
if [ "$SCENARIO" = failure ]; then exit 37; fi
if [ "$SCENARIO" = signal ] || [ "$SCENARIO" = kill ]; then
  printf ready > "$READY"
  while :; do sleep 1; done
fi
[ -f "$clones/hello/hello.pkg.tar.zst" ]
`)
			codex := fakeCodex(t, dir, filepath.Join(dir, "bundle.json"), `{"summary":"reviewed","risk":"low","findings":[],"uncertainty":"","inspect":[]}`)
			state := filepath.Join(dir, "state.sqlite3")
			active, ready := filepath.Join(dir, "active"), filepath.Join(dir, "ready")
			for _, tc := range []struct {
				name, input string
				status      int
			}{
				{"success", "approve\n", 0}, {"success", "approve\n", 0}, {"success", "approve\n", 0},
				{"failure", "approve\n", 37}, {"acquisition-failure", "", 1}, {"cancel", "cancel\n", 1}, {"eof", "", 1}, {"skip", "skip\n", 0},
				{"signal", "approve\n", 143}, {"kill", "approve\n", 137},
			} {
				t.Run(tc.name, func(t *testing.T) {
					os.Remove(ready)
					cmd := exec.Command(binary, "-S", "hello")
					cmd.Env = append(os.Environ(), "AUROSCOPE_PARU="+paru, "AUROSCOPE_CODEX="+codex,
						"AUROSCOPE_CLONE_DIR="+root, "AUROSCOPE_STATE="+state, "RECIPE_REPO="+repo,
						"ACTIVE_CLONES="+active, "READY="+ready, "SCENARIO="+tc.name)
					cmd.Stdin = strings.NewReader(tc.input)
					// File output avoids Wait depending on an orphan's pipe descriptors.
					log, err := os.Create(filepath.Join(dir, "output"))
					if err != nil {
						t.Fatal(err)
					}
					defer log.Close()
					cmd.Stdout, cmd.Stderr = log, log
					if err := cmd.Start(); err != nil {
						t.Fatal(err)
					}
					if tc.name == "signal" || tc.name == "kill" {
						deadline := time.Now().Add(5 * time.Second)
						for {
							if _, err := os.Stat(ready); err == nil {
								break
							}
							if time.Now().After(deadline) {
								cmd.Process.Kill()
								cmd.Wait()
								t.Fatal("build not reached")
							}
							time.Sleep(10 * time.Millisecond)
						}
						sig := syscall.SIGTERM
						if tc.name == "kill" {
							sig = syscall.SIGKILL
						}
						if err := cmd.Process.Signal(sig); err != nil {
							t.Fatal(err)
						}
					}
					err = cmd.Wait()
					if got := commandStatus(err); got != tc.status {
						t.Fatalf("status %d want %d: %s", got, tc.status, readString(t, log.Name()))
					}
					if tc.name == "kill" {
						opDir := filepath.Dir(readString(t, active))
						// The killed parent has not cleaned anything; its child still owns
						// the operation. A concurrent invocation must leave it intact.
						next, err := beginTemporaryOperation(root)
						if err != nil {
							t.Fatal(err)
						}
						if _, err := os.Stat(filepath.Join(opDir, "clones", "hello", "source.tar.gz")); err != nil {
							t.Fatal("deleted live orphan", err)
						}
						if err := next.close(); err != nil {
							t.Fatal(err)
						}
						data := strings.Fields(readString(t, filepath.Join(opDir, "groups")))
						// Stop only this fixture's final Paru group, not old provider PIDs.
						pid, err := strconv.Atoi(data[len(data)-1])
						if err != nil {
							t.Fatal(err)
						}
						syscall.Kill(-pid, syscall.SIGKILL)
						deadline := time.Now().Add(3 * time.Second)
						for {
							busy, err := operationGroupsAlive(opDir)
							if err != nil {
								t.Fatal(err)
							}
							if !busy {
								break
							}
							if time.Now().After(deadline) {
								t.Fatal("fixture group did not stop")
							}
							time.Sleep(10 * time.Millisecond)
						}
						next, err = beginTemporaryOperation(root)
						if err != nil {
							t.Fatal(err)
						}
						if err := next.close(); err != nil {
							t.Fatal(err)
						}
					}
					assertNoTemporaryOperations(t, root)
				})
			}
			assertBaseline(t, state, "hello")
		})
	}
}
