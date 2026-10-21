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

// Paru can encounter a selected package as another target's dependency first.
func TestSelectedDependencyOfficialInstall(t *testing.T) {
	for _, search := range []bool{true, false} {
		for _, nativeStatus := range []int{0, 17, 130} {
			t.Run(fmt.Sprintf("search=%t/status=%d", search, nativeStatus), func(t *testing.T) {
				dir := t.TempDir()
				calls := filepath.Join(dir, "calls")
				t.Setenv("CALLS", calls)
				t.Setenv("NATIVE_STATUS", fmt.Sprint(nativeStatus))
				paru := writeExecutable(t, dir, "paru", `#!/bin/sh
printf '%s\n' "$*" >> "$CALLS"
case "$1" in
  -Ssq) printf 'protonmail-bridge\nproton-vpn-gtk-app\nproton-vpn-daemon\n'; exit 1 ;;
  -P) printf 'REPO TARGET extra protonmail-bridge\nREPO DEP extra proton-vpn-daemon\nREPO TARGET extra proton-vpn-gtk-app\nREPO DEP extra unselected-dependency\n'; exit 0 ;;
esac
printf 'native stdout\n'
printf 'native stderr\n' >&2
read -r answer
printf 'answer=%s\n' "$answer"
exit "$NATIVE_STATUS"
`)
				targets := []string{"protonmail-bridge", "proton-vpn-gtk-app", "proton-vpn-daemon"}
				args := append([]string{"-S"}, targets...)
				wantCalls := []string{"-P --order -- " + strings.Join(targets, " ")}
				if search {
					args = []string{"proton"}
					wantCalls = append([]string{"-Ssq --interactive proton"}, wantCalls...)
					// The final native handoff follows Paru's record order.
					wantCalls = append(wantCalls, "-S -- protonmail-bridge proton-vpn-daemon proton-vpn-gtk-app")
				} else {
					wantCalls = append(wantCalls, strings.Join(args, " "))
				}
				input, err := os.CreateTemp(dir, "input")
				if err != nil {
					t.Fatal(err)
				}
				defer input.Close()
				if _, err := input.WriteString("yes\n"); err != nil {
					t.Fatal(err)
				}
				if _, err := input.Seek(0, 0); err != nil {
					t.Fatal(err)
				}
				var stdout, stderr bytes.Buffer
				config := runConfig{paruPath: paru, codexPath: "/must-not-run",
					statePath: filepath.Join(dir, "state"), cloneDir: filepath.Join(dir, "clones"),
					stdin: input, stdout: &stdout, stderr: &stderr}
				status := runOptionFixture(t, args, config)
				if status != nativeStatus || stdout.String() != "native stdout\nanswer=yes\n" || stderr.String() != "native stderr\n" {
					t.Fatalf("status=%d stdout=%q stderr=%q", status, &stdout, &stderr)
				}
				if got := strings.Split(strings.TrimSpace(readString(t, calls)), "\n"); !reflect.DeepEqual(got, wantCalls) {
					t.Fatalf("calls=%q, want %q", got, wantCalls)
				}
				for _, path := range []string{config.statePath, config.cloneDir} {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Fatalf("unexpected audit resource %s: %v", path, err)
					}
				}
			})
		}
	}
}

func TestSelectedDependencyMixedInstall(t *testing.T) {
	for _, decision := range []string{"approve", "skip"} {
		t.Run(decision, func(t *testing.T) {
			dir := t.TempDir()
			repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\npkgver=1\n")
			calls := filepath.Join(dir, "calls")
			paru := fakeParu(t, dir, calls, repo, "REPO DEP extra tree\nAUR DEP hello hello\n")
			codex := fakeCodex(t, dir, filepath.Join(dir, "bundle"), `{"summary":"checked","risk":"low","findings":[],"uncertainty":"","inspect":[]}`)
			var output bytes.Buffer
			status := runOptionFixture(t, []string{"-S", "tree", "hello"}, runConfig{
				paruPath: paru, codexPath: codex,
				statePath: filepath.Join(dir, "state"), cloneDir: filepath.Join(dir, "clones"),
				stdin: strings.NewReader(decision + "\n"), stdout: &output, stderr: &output,
			})
			if status != 0 {
				t.Fatalf("status=%d: %s", status, &output)
			}
			wantFinal := "-S -- tree"
			if decision == "approve" {
				wantFinal = "-S --skipreview -- tree hello"
			}
			want := []string{"-P --order -- tree hello", "-G hello", wantFinal}
			if got := strings.Split(strings.TrimSpace(readString(t, calls)), "\n"); !reflect.DeepEqual(got, want) {
				t.Fatalf("calls=%q, want %q", got, want)
			}
			if !strings.Contains(output.String(), "AUR audit: hello") {
				t.Fatalf("selected AUR dependency bypassed audit: %s", &output)
			}
		})
	}
}

func TestSelectedDependencyPlan(t *testing.T) {
	for _, role := range []string{"DEP", "MAKE"} {
		t.Run(role, func(t *testing.T) {
			result := orderResult{Records: []orderRecord{
				{Kind: "REPO", Fields: []string{role, "extra", "tree"}},
				{Kind: "REPO", Fields: []string{"DEP", "extra", "unselected"}},
				{Kind: "AUR", Fields: []string{role, "split-base", "chosen", "unselected-member"}},
			}}
			plan, err := result.plan("extra/tree", "aur/chosen", "tree")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(plan.RepoTargets, []string{"tree"}) ||
				!reflect.DeepEqual(plan.AURPkgbases, []string{"split-base"}) ||
				!reflect.DeepEqual(plan.AURTargetsByPkgbase["split-base"], []string{"chosen"}) {
				t.Fatalf("selected dependency lost or unrelated dependency promoted: %#v", plan)
			}
			for _, absent := range []string{"missing", "split-base", "virtual-provider"} {
				if _, err := result.plan(absent); err == nil || !strings.Contains(err.Error(), "not classified") {
					t.Fatalf("absent target %q accepted: %v", absent, err)
				}
			}
		})
	}
}
