package app

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestApprovedAURPackageEndToEndWithFirstAudit(t *testing.T) {
	dir := t.TempDir()
	repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\npkgver=1\n")
	calls := filepath.Join(dir, "calls")
	paruPath := fakeParu(t, dir, calls, repo, "AUR TARGET hello hello\n")
	codexPath := fakeCodex(t, dir, filepath.Join(dir, "bundle.json"), `{"summary":"looks bounded","risk":"low","findings":[],"uncertainty":"","inspect":["PKGBUILD"]}`)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	status := run([]string{"-S", "hello"}, runConfig{
		paruPath:    paruPath,
		codexPath:   codexPath,
		statePath:   filepath.Join(dir, "state.sqlite3"),
		cloneDir:    filepath.Join(dir, "clones"),
		stdin:       strings.NewReader(""),
		reviewInput: strings.NewReader("approve\n"),
		stdout:      &stdout,
		stderr:      &stderr,
	})
	if status != 0 {
		t.Fatalf("status = %d; stdout = %s; stderr = %s; calls = %s", status, stdout.String(), stderr.String(), readString(t, calls))
	}
	callsText := readString(t, calls)
	if !strings.Contains(callsText, "-P --order hello") || !strings.Contains(callsText, "-G hello") || !strings.Contains(callsText, "-S --skipreview --config") {
		t.Fatalf("calls = %s", callsText)
	}
	if !strings.Contains(stdout.String(), "AUR audit: hello") {
		t.Fatalf("stdout = %q", stdout.String())
	}
	assertBaseline(t, filepath.Join(dir, "state.sqlite3"), "hello")
	var bundle auditBundle
	readJSON(t, filepath.Join(dir, "bundle.json"), &bundle)
	if bundle.Mode != "full" || bundle.Identity.Pkgbase != "hello" || len(bundle.Files) == 0 {
		t.Fatalf("bundle = %#v", bundle)
	}
}

func TestDifferentialAuditUsesSuccessfulBaseline(t *testing.T) {
	dir := t.TempDir()
	repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\npkgver=1\n")
	firstCommit := gitTrim(t, repo, "rev-parse", "HEAD")
	appendRecipeCommit(t, repo, "pkgrel=2\n")
	calls := filepath.Join(dir, "calls")
	paruPath := fakeParu(t, dir, calls, repo, "AUR TARGET hello hello\n")
	bundlePath := filepath.Join(dir, "bundle.json")
	codexPath := fakeCodex(t, dir, bundlePath, `{"summary":"diff reviewed","risk":"medium","findings":[{"file":"PKGBUILD","line":3,"evidence":"pkgrel","explanation":"changed release"}],"uncertainty":"","inspect":[]}`)
	store, err := openState(filepath.Join(dir, "state.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	firstIdentity := recipeIdentity{Pkgbase: "hello", Commit: firstCommit, ManifestDigest: strings.Repeat("a", 64)}
	if err := store.advanceBaselines([]recipeIdentity{firstIdentity}); err != nil {
		t.Fatal(err)
	}
	store.Close()

	var stderr bytes.Buffer
	status := run([]string{"-S", "hello"}, runConfig{
		paruPath:    paruPath,
		codexPath:   codexPath,
		statePath:   filepath.Join(dir, "state.sqlite3"),
		cloneDir:    filepath.Join(dir, "clones"),
		stdin:       strings.NewReader(""),
		reviewInput: strings.NewReader("approve\n"),
		stdout:      io.Discard,
		stderr:      &stderr,
	})
	if status != 0 {
		t.Fatalf("status = %d; stderr = %s", status, stderr.String())
	}
	var bundle auditBundle
	readJSON(t, bundlePath, &bundle)
	if bundle.Mode != "diff" || bundle.PreviousCommit != firstCommit || !strings.Contains(bundle.Diff, "pkgrel=2") {
		t.Fatalf("bundle = %#v", bundle)
	}
}

func TestMixedInstallSkipKeepsOfficialNativeAndOmitsAUR(t *testing.T) {
	dir := t.TempDir()
	repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\n")
	calls := filepath.Join(dir, "calls")
	paruPath := fakeParu(t, dir, calls, repo, "REPO TARGET extra tree\nAUR TARGET hello hello\n")
	codexPath := fakeCodex(t, dir, filepath.Join(dir, "bundle.json"), `{"summary":"skip me","risk":"unknown","findings":[],"uncertainty":"","inspect":[]}`)

	var stderr bytes.Buffer
	status := run([]string{"-S", "tree", "hello"}, runConfig{
		paruPath:    paruPath,
		codexPath:   codexPath,
		statePath:   filepath.Join(dir, "state.sqlite3"),
		cloneDir:    filepath.Join(dir, "clones"),
		stdin:       strings.NewReader(""),
		reviewInput: strings.NewReader("skip\n"),
		stdout:      io.Discard,
		stderr:      &stderr,
	})
	if status != 0 {
		t.Fatalf("status = %d; stderr = %s", status, stderr.String())
	}
	callsText := readString(t, calls)
	if !strings.Contains(callsText, "-S tree\n") || strings.Contains(callsText, "--skipreview") || strings.Contains(callsText, "-S tree hello") {
		t.Fatalf("calls = %s", callsText)
	}
}

func TestBareUpdateAuditsPendingAURAfterOfficialPhase(t *testing.T) {
	dir := t.TempDir()
	repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\n")
	calls := filepath.Join(dir, "calls")
	paruPath := writeExecutable(t, dir, "paru", `#!/bin/sh
printf '%s\n' "$*" >> "$CALLS"
if test "$1 $2" = "-Syu --repo"; then
  exit 0
fi
if test "$1 $2" = "-Qua --quiet"; then
  printf 'hello\n'
  exit 0
fi
if test "$1 $2" = "-P --order"; then
  printf 'AUR TARGET hello hello\n'
  exit 0
fi
if test "$1" = "-G"; then
  rm -rf "$PWD/$2"
  cp -R "$RECIPE_REPO" "$PWD/$2"
  exit 0
fi
exit 0
`)
	t.Setenv("CALLS", calls)
	t.Setenv("RECIPE_REPO", repo)
	codexPath := fakeCodex(t, dir, filepath.Join(dir, "bundle.json"), `{"summary":"update reviewed","risk":"low","findings":[],"uncertainty":"","inspect":[]}`)
	status := run(nil, runConfig{
		paruPath:    paruPath,
		codexPath:   codexPath,
		statePath:   filepath.Join(dir, "state.sqlite3"),
		cloneDir:    filepath.Join(dir, "clones"),
		stdin:       strings.NewReader(""),
		reviewInput: strings.NewReader("approve\n"),
		stdout:      io.Discard,
		stderr:      io.Discard,
	})
	if status != 0 {
		t.Fatalf("status = %d; calls = %s", status, readString(t, calls))
	}
	callsText := readString(t, calls)
	for _, want := range []string{"-Syu --repo", "-Qua --quiet", "-P --order hello", "-G hello", "-S --skipreview --config"} {
		if !strings.Contains(callsText, want) {
			t.Fatalf("missing %q in calls %s", want, callsText)
		}
	}
}

func TestExplicitSystemUpgradeUsesOfficialFirstPath(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	paruPath := writeExecutable(t, dir, "paru", `#!/bin/sh
printf '%s\n' "$*" >> "$CALLS"
exit 0
`)
	t.Setenv("CALLS", calls)
	status := run([]string{"-Syu"}, runConfig{
		paruPath:  paruPath,
		statePath: filepath.Join(dir, "state.sqlite3"),
		cloneDir:  filepath.Join(dir, "clones"),
		stdin:     strings.NewReader(""),
		stdout:    io.Discard,
		stderr:    io.Discard,
	})
	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	if got := readLines(t, calls); !reflect.DeepEqual(got, []string{"-Syu", "--repo", "-Qua", "--quiet"}) {
		t.Fatalf("calls = %#v", got)
	}
}

func TestAURDependencyIsAuditedBeforeFinalRun(t *testing.T) {
	dir := t.TempDir()
	repo := createRecipeRepo(t, dir, "depbase", "pkgname=depbase\n")
	calls := filepath.Join(dir, "calls")
	paruPath := fakeParu(t, dir, calls, repo, "AUR DEP dep depbase\n")
	codexPath := fakeCodex(t, dir, filepath.Join(dir, "bundle.json"), `{"summary":"dep reviewed","risk":"low","findings":[],"uncertainty":"","inspect":[]}`)
	status := run([]string{"-S", "dep"}, runConfig{
		paruPath:    paruPath,
		codexPath:   codexPath,
		statePath:   filepath.Join(dir, "state.sqlite3"),
		cloneDir:    filepath.Join(dir, "clones"),
		stdin:       strings.NewReader(""),
		reviewInput: strings.NewReader("approve\n"),
		stdout:      io.Discard,
		stderr:      io.Discard,
	})
	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	if callsText := readString(t, calls); !strings.Contains(callsText, "-G depbase") || !strings.Contains(callsText, "depbase") {
		t.Fatalf("calls = %s", callsText)
	}
}

func TestSplitAURRecordsAcquirePkgbaseOnceAndPreserveTarget(t *testing.T) {
	dir := t.TempDir()
	repo := createRecipeRepo(t, dir, "split-base", "pkgbase=split-base\npkgname=('split-member' 'split-helper')\n")
	calls := filepath.Join(dir, "calls")
	paruPath := fakeParu(t, dir, calls, repo, "AUR TARGET split-member split-base\nAUR DEP split-helper split-base\n")
	codexPath := fakeCodex(t, dir, filepath.Join(dir, "bundle.json"), `{"summary":"split package reviewed","risk":"low","findings":[],"uncertainty":"","inspect":[]}`)

	status := run([]string{"-S", "split-member"}, runConfig{
		paruPath:    paruPath,
		codexPath:   codexPath,
		statePath:   filepath.Join(dir, "state.sqlite3"),
		cloneDir:    filepath.Join(dir, "clones"),
		stdin:       strings.NewReader(""),
		reviewInput: strings.NewReader("approve\n"),
		stdout:      io.Discard,
		stderr:      io.Discard,
	})
	if status != 0 {
		t.Fatalf("status = %d; calls = %s", status, readString(t, calls))
	}

	callLines := strings.Split(strings.TrimSpace(readString(t, calls)), "\n")
	var acquisitions []string
	var final string
	for _, call := range callLines {
		if strings.HasPrefix(call, "-G ") {
			acquisitions = append(acquisitions, call)
		}
		if strings.HasPrefix(call, "-S --skipreview --config ") {
			final = call
		}
	}
	if !reflect.DeepEqual(acquisitions, []string{"-G split-base"}) {
		t.Fatalf("acquisitions = %#v, want one pkgbase acquisition; calls = %s", acquisitions, readString(t, calls))
	}
	if final == "" || !strings.HasSuffix(final, " split-member") {
		t.Fatalf("final Paru target did not preserve split member: %q", final)
	}
	assertBaseline(t, filepath.Join(dir, "state.sqlite3"), "split-base")
}

func TestCodexFailureCanSkipWithoutFinalBuild(t *testing.T) {
	dir := t.TempDir()
	repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\n")
	calls := filepath.Join(dir, "calls")
	paruPath := fakeParu(t, dir, calls, repo, "AUR TARGET hello hello\n")
	codexPath := writeExecutable(t, dir, "codex", `#!/bin/sh
printf 'not json'
`)
	status := run([]string{"-S", "hello"}, runConfig{
		paruPath:    paruPath,
		codexPath:   codexPath,
		statePath:   filepath.Join(dir, "state.sqlite3"),
		cloneDir:    filepath.Join(dir, "clones"),
		stdin:       strings.NewReader(""),
		reviewInput: strings.NewReader("skip\n"),
		stdout:      io.Discard,
		stderr:      io.Discard,
	})
	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	if strings.Contains(readString(t, calls), "--skipreview") {
		t.Fatalf("final build was invoked: %s", readString(t, calls))
	}
}

func TestFinalParuFailureDoesNotAdvanceBaseline(t *testing.T) {
	dir := t.TempDir()
	repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\n")
	calls := filepath.Join(dir, "calls")
	paruPath := writeExecutable(t, dir, "paru", `#!/bin/sh
printf '%s\n' "$*" >> "$CALLS"
if test "$1 $2" = "-P --order"; then
  printf 'AUR TARGET hello hello\n'
  exit 0
fi
if test "$1" = "-G"; then
  rm -rf "$PWD/$2"
  cp -R "$RECIPE_REPO" "$PWD/$2"
  exit 0
fi
if test "$1" = "-S"; then
  exit 37
fi
exit 0
`)
	t.Setenv("CALLS", calls)
	t.Setenv("RECIPE_REPO", repo)
	codexPath := fakeCodex(t, dir, filepath.Join(dir, "bundle.json"), `{"summary":"reviewed","risk":"low","findings":[],"uncertainty":"","inspect":[]}`)
	statePath := filepath.Join(dir, "state.sqlite3")
	status := run([]string{"-S", "hello"}, runConfig{
		paruPath:    paruPath,
		codexPath:   codexPath,
		statePath:   statePath,
		cloneDir:    filepath.Join(dir, "clones"),
		stdin:       strings.NewReader(""),
		reviewInput: strings.NewReader("approve\n"),
		stdout:      io.Discard,
		stderr:      io.Discard,
	})
	if status != 37 {
		t.Fatalf("status = %d", status)
	}
	db, err := sql.Open("sqlite3", statePath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM packages`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("baseline count = %d", count)
	}
}

func TestEditSnapshotsAndReauditsSameWorktree(t *testing.T) {
	dir := t.TempDir()
	repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\n")
	calls := filepath.Join(dir, "calls")
	paruPath := fakeParu(t, dir, calls, repo, "AUR TARGET hello hello\n")
	codexCalls := filepath.Join(dir, "codex-calls")
	codexPath := writeExecutable(t, dir, "codex", `#!/bin/sh
printf 'call\n' >> "$CODEX_CALLS"
printf '{"summary":"review","risk":"low","findings":[],"uncertainty":"","inspect":[]}'
`)
	editorPath := writeExecutable(t, dir, "editor", `#!/bin/sh
printf 'pkgrel=2\n' >> "$1/PKGBUILD"
`)
	t.Setenv("CODEX_CALLS", codexCalls)

	var stderr bytes.Buffer
	status := run([]string{"-S", "hello"}, runConfig{
		paruPath:    paruPath,
		codexPath:   codexPath,
		editorPath:  editorPath,
		statePath:   filepath.Join(dir, "state.sqlite3"),
		cloneDir:    filepath.Join(dir, "clones"),
		stdin:       strings.NewReader(""),
		reviewInput: strings.NewReader("edit\napprove\n"),
		stdout:      io.Discard,
		stderr:      &stderr,
	})
	if status != 0 {
		t.Fatalf("status = %d; stderr = %s", status, stderr.String())
	}
	if got := readLines(t, codexCalls); !reflect.DeepEqual(got, []string{"call", "call"}) {
		t.Fatalf("codex calls = %#v", got)
	}
	clone := filepath.Join(dir, "clones", "hello")
	if got := gitTrim(t, clone, "status", "--porcelain"); got != "" {
		t.Fatalf("edited worktree not committed: %q", got)
	}
	if !strings.Contains(readString(t, filepath.Join(clone, "PKGBUILD")), "pkgrel=2") {
		t.Fatal("edit did not persist in audited worktree")
	}
}

func TestGuardRejectsIdentityDriftAndUnexpectedPackage(t *testing.T) {
	dir := t.TempDir()
	repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\n")
	identity, _, err := readRecipeIdentity("hello", repo)
	if err != nil {
		t.Fatal(err)
	}
	txPath := filepath.Join(dir, "tx.json")
	data, _ := json.Marshal([]recipeIdentity{identity})
	if err := os.WriteFile(txPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PKGBASE", "hello")
	if err := runGuard(txPath, repo); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "PKGBUILD"), []byte("pkgname=hello\npkgrel=drift\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runGuard(txPath, repo); err == nil || !strings.Contains(err.Error(), "drift") {
		t.Fatalf("drift error = %v", err)
	}
	t.Setenv("PKGBASE", "other")
	if err := runGuard(txPath, repo); err == nil || !strings.Contains(err.Error(), "unexpected pkgbase") {
		t.Fatalf("unexpected error = %v", err)
	}
}

func TestCodexValidationRejectsActionAndHostilePath(t *testing.T) {
	if _, err := validateAuditReport([]byte(`{"summary":"x","risk":"low","findings":[],"uncertainty":"","inspect":[],"action":"allow"}`)); err == nil {
		t.Fatal("accepted action field")
	}
	_, err := validateAuditReport([]byte(`{"summary":"x","risk":"low","findings":[{"file":"../PKGBUILD","evidence":"x","explanation":"x"}],"uncertainty":"","inspect":[]}`))
	if err == nil || !strings.Contains(err.Error(), "invalid recipe path") {
		t.Fatalf("error = %v", err)
	}
}

func TestStateContainsOnlyPackagesAndAuditsTables(t *testing.T) {
	store, err := openState(filepath.Join(t.TempDir(), "state.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	names, err := store.tableNames()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(names, []string{"audits", "packages"}) {
		t.Fatalf("tables = %#v", names)
	}
}

func createRecipeRepo(t *testing.T, root, pkgbase, pkgbuild string) string {
	t.Helper()
	dir := filepath.Join(root, pkgbase+"-src")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PKGBUILD"), []byte(pkgbuild), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".SRCINFO"), []byte("pkgbase = "+pkgbase+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.name", "Test")
	gitRun(t, dir, "config", "user.email", "test@example.invalid")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "initial")
	return dir
}

func appendRecipeCommit(t *testing.T, dir, text string) {
	t.Helper()
	f, err := os.OpenFile(filepath.Join(dir, "PKGBUILD"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "update")
}

func fakeParu(t *testing.T, dir, calls, repo, orderOutput string) string {
	t.Helper()
	path := writeExecutable(t, dir, "paru", `#!/bin/sh
printf '%s\n' "$*" >> "$CALLS"
if test "$1 $2" = "-P --order"; then
  printf '%s' "$ORDER_OUTPUT"
  exit 0
fi
if test "$1" = "-G"; then
  rm -rf "$PWD/$2"
  cp -R "$RECIPE_REPO" "$PWD/$2"
  exit 0
fi
exit 0
`)
	t.Setenv("CALLS", calls)
	t.Setenv("RECIPE_REPO", repo)
	t.Setenv("ORDER_OUTPUT", orderOutput)
	return path
}

func fakeCodex(t *testing.T, dir, bundleCopy, output string) string {
	t.Helper()
	path := writeExecutable(t, dir, "codex", `#!/bin/sh
cp bundle.json "$BUNDLE_COPY"
printf '%s' "$CODEX_OUTPUT"
`)
	t.Setenv("BUNDLE_COPY", bundleCopy)
	t.Setenv("CODEX_OUTPUT", output)
	return path
}

func assertBaseline(t *testing.T, dbPath, pkgbase string) {
	t.Helper()
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM packages WHERE pkgbase = ?`, pkgbase).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("baseline count = %d", count)
	}
}

func readJSON(t *testing.T, path string, v any) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		t.Fatal(err)
	}
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func gitTrim(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}
