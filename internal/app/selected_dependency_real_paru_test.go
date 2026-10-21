package app

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// The real resolver sees a private, deterministic official repository. No
// external AUR data, PKGBUILD execution or package installation is involved.
func TestSelectedDependencyRealParu(t *testing.T) {
	if os.Getenv("AUROSCOPE_E2E_INSIDE") != "1" || os.Getenv("AUROSCOPE_TEST_REAL_PARU") == "" {
		t.Skip("requires disposable Arch and pinned Paru")
	}
	dir := t.TempDir()
	db := filepath.Join(dir, "db")
	for _, subdir := range []string{"local", "sync"} {
		if err := os.MkdirAll(filepath.Join(db, subdir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(db, "local", "ALPM_DB_VERSION"), []byte("9\n"), 0644); err != nil {
		t.Fatal(err)
	}
	var repo bytes.Buffer
	compressed := gzip.NewWriter(&repo)
	archive := tar.NewWriter(compressed)
	for _, name := range []string{"protonmail-bridge", "proton-vpn-gtk-app", "proton-vpn-daemon"} {
		desc := fmt.Sprintf("%%FILENAME%%\n%s-1-1-x86_64.pkg.tar.zst\n\n%%NAME%%\n%s\n\n%%BASE%%\n%s\n\n%%VERSION%%\n1-1\n\n%%DESC%%\nPrivate Proton selection fixture\n\n%%ARCH%%\nx86_64\n\n", name, name, name)
		if name == "proton-vpn-gtk-app" {
			desc += "%DEPENDS%\nproton-vpn-daemon\n\n"
		}
		if err := archive.WriteHeader(&tar.Header{Name: name + "-1-1/desc", Mode: 0644, Size: int64(len(desc))}); err != nil {
			t.Fatal(err)
		}
		if _, err := archive.Write([]byte(desc)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(db, "sync", "extra.db"), repo.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	pacmanConf := filepath.Join(dir, "pacman.conf")
	if err := os.WriteFile(pacmanConf, []byte("[options]\nArchitecture = x86_64\nSigLevel = Never\n[extra]\nServer = file:///unused\n"), 0600); err != nil {
		t.Fatal(err)
	}
	paruConf := filepath.Join(dir, "paru.conf")
	if err := os.WriteFile(paruConf, []byte("[options]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PARU_CONF", paruConf)
	t.Setenv("SELECTED_DEPENDENCY_DB", db)
	t.Setenv("SELECTED_DEPENDENCY_CONFIG", pacmanConf)
	t.Setenv("SELECTED_DEPENDENCY_FINAL", filepath.Join(dir, "final"))
	t.Setenv("LC_ALL", "C")
	paru := writeExecutable(t, dir, "paru", `#!/bin/sh
if [ "$1" = -S ]; then
  printf '%s\n' "$@" > "$SELECTED_DEPENDENCY_FINAL"
  printf 'native final handoff\n'
  exit 17
fi
exec "$AUROSCOPE_TEST_REAL_PARU" --config "$SELECTED_DEPENDENCY_CONFIG" --dbpath "$SELECTED_DEPENDENCY_DB" --repo "$@"
`)
	// Put the application before its dependency to reproduce the resolver's
	// first-encounter role. The dependency later requested is still DEP/MAKE.
	targets := []string{"protonmail-bridge", "proton-vpn-gtk-app", "proton-vpn-daemon"}
	cmd := exec.Command(paru, append([]string{"-P", "--order", "--"}, targets...)...)
	order, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("real order failed: %v: %s", err, order)
	}
	if !strings.Contains(string(order), "REPO DEP extra proton-vpn-daemon\n") &&
		!strings.Contains(string(order), "REPO MAKE extra proton-vpn-daemon\n") {
		t.Fatalf("real Paru did not reproduce first-encounter dependency role: %s", order)
	}
	t.Logf("real Paru records:\n%s", order)
	reversed := []string{"proton-vpn-daemon", "proton-vpn-gtk-app", "protonmail-bridge"}
	cmd = exec.Command(paru, append([]string{"-P", "--order", "--"}, reversed...)...)
	order, err = cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(order), "REPO TARGET extra proton-vpn-daemon\n") {
		t.Fatalf("dependency-first control failed: %v: %s", err, order)
	}
	t.Logf("dependency-first real Paru records:\n%s", order)
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"explicit application first", append([]string{"-S"}, targets...)},
		{"native search", []string{"proton"}},
		{"explicit dependency first", append([]string{"-S"}, reversed...)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.Remove(os.Getenv("SELECTED_DEPENDENCY_FINAL")); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			args := tc.args
			var output bytes.Buffer
			config := runConfig{paruPath: paru, codexPath: "/must-not-run",
				statePath: filepath.Join(dir, "state"), cloneDir: filepath.Join(dir, "clones"),
				stdin: strings.NewReader("1 2 3\n"), stdout: &output, stderr: &output}
			if status := runOptionFixture(t, args, config); status != 17 || !strings.Contains(output.String(), "native final handoff") {
				t.Fatalf("real selected dependency failed: status=%d: %s", status, &output)
			}
			got := readLines(t, os.Getenv("SELECTED_DEPENDENCY_FINAL"))
			if len(got) < 1 || got[0] != "-S" {
				t.Fatalf("final handoff: %q", got)
			}
			got = got[1:]
			if len(got) > 0 && got[0] == "--" {
				got = got[1:]
			}
			sort.Strings(got)
			want := append([]string(nil), targets...)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("selection lost a package: %q, want %q", got, want)
			}
			for _, path := range []string{config.statePath, config.cloneDir} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Fatalf("official path created audit state %s: %v", path, err)
				}
			}
		})
	}
}
