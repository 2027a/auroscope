package app

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
)

func TestNonBuildingCommandPassesThroughWithStreamsAndStatus(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	paruPath := writeExecutable(t, dir, "paru", `#!/bin/sh
printf '%s\n' "$@" > "$ARGS_FILE"
printf 'native stdout\n'
printf 'native stderr\n' >&2
IFS= read -r answer
printf 'stdin=%s\n' "$answer"
exit 17
`)
	t.Setenv("ARGS_FILE", argsFile)

	var stdout, stderr bytes.Buffer
	status := run([]string{"-Q", "paru"}, runConfig{
		paruPath: paruPath,
		stdin:    strings.NewReader("answer\n"),
		stdout:   &stdout,
		stderr:   &stderr,
	})

	if status != 17 {
		t.Fatalf("status = %d, want 17", status)
	}
	if got := readLines(t, argsFile); !reflect.DeepEqual(got, []string{"-Q", "paru"}) {
		t.Fatalf("args = %#v", got)
	}
	if got := stdout.String(); got != "native stdout\nstdin=answer\n" {
		t.Fatalf("stdout = %q", got)
	}
	if got := stderr.String(); got != "native stderr\n" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestRefreshOnlySyncPassesThroughWithStreamsAndStatus(t *testing.T) {
	for _, args := range [][]string{
		{"-Sy"},
		{"-Syy"},
		{"-yS"},
		{"--sync", "--refresh"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			dir := t.TempDir()
			argsFile := filepath.Join(dir, "args")
			codexMarker := filepath.Join(dir, "codex-called")
			paruPath := writeExecutable(t, dir, "paru", `#!/bin/sh
printf '%s\n' "$@" > "$ARGS_FILE"
printf 'native refresh stdout\n'
printf 'native refresh stderr\n' >&2
IFS= read -r answer
printf 'stdin=%s\n' "$answer"
exit 17
`)
			codexPath := writeExecutable(t, dir, "codex", `#!/bin/sh
: > "$CODEX_MARKER"
exit 99
`)
			t.Setenv("ARGS_FILE", argsFile)
			t.Setenv("CODEX_MARKER", codexMarker)

			var stdout, stderr bytes.Buffer
			status := run(args, runConfig{
				paruPath:  paruPath,
				codexPath: codexPath,
				statePath: filepath.Join(dir, "state.sqlite3"),
				cloneDir:  filepath.Join(dir, "clones"),
				stdin:     strings.NewReader("answer\n"),
				stdout:    &stdout,
				stderr:    &stderr,
			})

			if status != 17 {
				t.Fatalf("status = %d, want 17; stderr = %q", status, stderr.String())
			}
			if got := readLines(t, argsFile); !reflect.DeepEqual(got, args) {
				t.Fatalf("args = %#v, want %#v", got, args)
			}
			if got := stdout.String(); got != "native refresh stdout\nstdin=answer\n" {
				t.Fatalf("stdout = %q", got)
			}
			if got := stderr.String(); got != "native refresh stderr\n" {
				t.Fatalf("stderr = %q", got)
			}
			if _, err := os.Stat(codexMarker); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("Codex was called or marker stat failed: %v", err)
			}
		})
	}
}

func TestPassthroughPreservesPTY(t *testing.T) {
	dir := t.TempDir()
	paruPath := writeExecutable(t, dir, "paru", `#!/bin/sh
if ! test -t 0 || ! test -t 1 || ! test -t 2; then
  printf 'not a tty\n' >&2
  exit 91
fi
printf 'native prompt: '
IFS= read -r answer
printf 'answer=%s\n' "$answer"
exit 17
`)

	cmd := exec.Command(os.Args[0], "-test.run=^TestPTYRunHelper$")
	cmd.Env = append(os.Environ(), "AUROSCOPE_TEST_PTY_HELPER=1", "AUROSCOPE_TEST_PARU="+paruPath)
	ptmx, err := pty.Start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	defer ptmx.Close()
	if _, err := io.WriteString(ptmx, "yes\n"); err != nil {
		t.Fatal(err)
	}

	err = cmd.Wait()
	output, _ := io.ReadAll(ptmx) // Linux PTYs report EIO after the slave closes.
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 17 {
		t.Fatalf("helper error = %v, output = %q", err, output)
	}
	got := strings.ReplaceAll(string(output), "\r\n", "\n")
	if !strings.Contains(got, "native prompt: ") || !strings.Contains(got, "answer=yes\n") {
		t.Fatalf("PTY output did not preserve prompt/answer: %q", got)
	}
}

func TestPTYRunHelper(t *testing.T) {
	if os.Getenv("AUROSCOPE_TEST_PTY_HELPER") != "1" {
		return
	}
	status := run([]string{"-Q"}, runConfig{
		paruPath: os.Getenv("AUROSCOPE_TEST_PARU"),
		stdin:    os.Stdin,
		stdout:   os.Stdout,
		stderr:   os.Stderr,
	})
	os.Exit(status)
}

func TestRunForwardsSignalsToChild(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	paruPath := writeExecutable(t, dir, "paru", `#!/bin/sh
trap 'exit 23' TERM
: > "$READY_FILE"
while :; do sleep 1; done
`)
	t.Setenv("READY_FILE", ready)
	signals := make(chan os.Signal, 1)
	result := make(chan int, 1)
	go func() {
		result <- run([]string{"-Q"}, runConfig{
			paruPath: paruPath,
			stdin:    strings.NewReader(""),
			stdout:   io.Discard,
			stderr:   io.Discard,
			signals:  signals,
		})
	}()

	waitForFile(t, ready)
	signals <- syscall.SIGTERM
	select {
	case status := <-result:
		if status != 23 {
			t.Fatalf("status = %d, want 23", status)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("child did not receive SIGTERM")
	}
}

func TestRunForwardsSignalsToChildProcessGroup(t *testing.T) {
	dir := t.TempDir()
	ready := filepath.Join(dir, "ready")
	grandchildDone := filepath.Join(dir, "grandchild-done")
	paruPath := writeExecutable(t, dir, "paru", `#!/bin/sh
trap 'exit 23' TERM
(
  trap 'printf grandchild > "$GRANDCHILD_DONE"; exit 0' TERM
  while :; do sleep 1; done
) &
: > "$READY_FILE"
while :; do sleep 1; done
`)
	t.Setenv("READY_FILE", ready)
	t.Setenv("GRANDCHILD_DONE", grandchildDone)
	signals := make(chan os.Signal, 1)
	result := make(chan int, 1)
	go func() {
		result <- run([]string{"-Q"}, runConfig{
			paruPath: paruPath,
			stdin:    strings.NewReader(""),
			stdout:   io.Discard,
			stderr:   io.Discard,
			signals:  signals,
		})
	}()

	waitForFile(t, ready)
	signals <- syscall.SIGTERM
	select {
	case status := <-result:
		if status != 23 {
			t.Fatalf("status = %d, want 23", status)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("child did not receive SIGTERM")
	}
	waitForFile(t, grandchildDone)
}

func TestBareUpdateRunsOfficialPhaseWithoutCodex(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	codexMarker := filepath.Join(dir, "codex-called")
	paruPath := writeExecutable(t, dir, "paru", `#!/bin/sh
printf '%s\n' "$@" >> "$ARGS_FILE"
if test "$1" = "-Syu"; then
  printf 'official prompt\n'
fi
exit 0
`)
	writeExecutable(t, dir, "codex", `#!/bin/sh
: > "$CODEX_MARKER"
exit 99
`)
	t.Setenv("ARGS_FILE", argsFile)
	t.Setenv("CODEX_MARKER", codexMarker)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var stdout bytes.Buffer
	status := run(nil, runConfig{paruPath: paruPath, statePath: filepath.Join(dir, "state.sqlite3"), cloneDir: filepath.Join(dir, "clones"), stdin: strings.NewReader(""), stdout: &stdout, stderr: io.Discard})
	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	if got := readLines(t, argsFile); !reflect.DeepEqual(got, []string{"-Syu", "--repo", "-Qua", "--quiet"}) {
		t.Fatalf("args = %#v", got)
	}
	if stdout.String() != "official prompt\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	if _, err := os.Stat(codexMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Codex was called or marker stat failed: %v", err)
	}
}

func TestBareUpdateStopsOnOfficialFailure(t *testing.T) {
	dir := t.TempDir()
	countFile := filepath.Join(dir, "count")
	paruPath := writeExecutable(t, dir, "paru", `#!/bin/sh
printf 'call\n' >> "$COUNT_FILE"
exit 42
`)
	t.Setenv("COUNT_FILE", countFile)

	status := run(nil, runConfig{paruPath: paruPath, stdin: strings.NewReader(""), stdout: io.Discard, stderr: io.Discard})
	if status != 42 {
		t.Fatalf("status = %d, want 42", status)
	}
	if got := readLines(t, countFile); !reflect.DeepEqual(got, []string{"call"}) {
		t.Fatalf("calls = %#v", got)
	}
}

func TestExplicitOfficialOnlyInstallPassesThroughWithoutCodex(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	codexMarker := filepath.Join(dir, "codex-called")
	paruPath := writeExecutable(t, dir, "paru", `#!/bin/sh
printf '%s\n' "$@" > "$ARGS_FILE"
exit 0
`)
	writeExecutable(t, dir, "codex", `#!/bin/sh
: > "$CODEX_MARKER"
exit 99
`)
	t.Setenv("ARGS_FILE", argsFile)
	t.Setenv("CODEX_MARKER", codexMarker)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	status := run([]string{"-S", "--repo", "tree"}, runConfig{paruPath: paruPath, stdin: strings.NewReader(""), stdout: io.Discard, stderr: io.Discard})
	if status != 0 {
		t.Fatalf("status = %d", status)
	}
	if got := readLines(t, argsFile); !reflect.DeepEqual(got, []string{"-S", "--repo", "tree"}) {
		t.Fatalf("args = %#v", got)
	}
	if _, err := os.Stat(codexMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Codex was called or marker stat failed: %v", err)
	}
}

func TestChildSignalExitStatusIsPreserved(t *testing.T) {
	dir := t.TempDir()
	paruPath := writeExecutable(t, dir, "paru", `#!/bin/sh
kill -TERM $$
`)

	status := run([]string{"-Q"}, runConfig{paruPath: paruPath, stdin: strings.NewReader(""), stdout: io.Discard, stderr: io.Discard})
	if status != 128+int(syscall.SIGTERM) {
		t.Fatalf("status = %d, want %d", status, 128+int(syscall.SIGTERM))
	}
}

func TestBuildingCommandDoesNotBypassAudit(t *testing.T) {
	dir := t.TempDir()
	calls := filepath.Join(dir, "calls")
	paruPath := writeExecutable(t, dir, "paru", `#!/bin/sh
printf '%s\n' "$*" >> "$CALLS"
if test "$1 $2" = "-P --order"; then
  printf 'AUR TARGET aur-package aur-package\n'
fi
if test "$1" = "-G"; then
  mkdir -p "$2/.git"
fi
`)
	codexPath := writeExecutable(t, dir, "codex", `#!/bin/sh
printf '{"summary":"broken","risk":"low","findings":[],"uncertainty":"","inspect":[],"action":"allow"}'
`)
	t.Setenv("CALLS", calls)
	var stderr bytes.Buffer

	status := run([]string{"-S", "aur-package"}, runConfig{paruPath: paruPath, codexPath: codexPath, statePath: filepath.Join(dir, "state.sqlite3"), cloneDir: filepath.Join(dir, "clones"), stdin: strings.NewReader("cancel\n"), stdout: io.Discard, stderr: &stderr})
	if status == 0 {
		t.Fatal("building command unexpectedly succeeded")
	}
	for _, call := range readLines(t, calls) {
		if call == "--skipreview" {
			t.Fatalf("final Paru build was invoked: %v", readLines(t, calls))
		}
	}
	if !strings.Contains(stderr.String(), "read recipe commit") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestParuNativeSelectionContract(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	paruPath := writeExecutable(t, dir, "paru", `#!/bin/sh
printf '%s\n' "$@" > "$ARGS_FILE"
printf '1 aur/hello 2.12\nChoose packages: ' >&2
printf 'aur/hello\nworld-bin\n'
exit 1
`)
	t.Setenv("ARGS_FILE", argsFile)
	var menu bytes.Buffer
	client := paruClient{config: runConfig{paruPath: paruPath, stdin: strings.NewReader("1\n"), stdout: io.Discard, stderr: &menu}}

	selected, err := client.selectPackages([]string{"hello", "world"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(selected, []string{"aur/hello", "world-bin"}) {
		t.Fatalf("selected = %#v", selected)
	}
	if got := readLines(t, argsFile); !reflect.DeepEqual(got, []string{"-Ssaq", "--interactive", "hello", "world"}) {
		t.Fatalf("args = %#v", got)
	}
	if !strings.Contains(menu.String(), "Choose packages") {
		t.Fatalf("menu = %q", menu.String())
	}
}

func TestParuNativeSelectionRejectsWrongStatusOrOutput(t *testing.T) {
	for _, tc := range []struct {
		name   string
		output string
		status string
	}{
		{name: "status zero", output: "hello\\n", status: "0"},
		{name: "human menu on stdout", output: "1 aur/hello 2.12\\nhello\\n", status: "1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			paruPath := writeExecutable(t, dir, "paru", "#!/bin/sh\nprintf '"+tc.output+"'\nexit "+tc.status+"\n")
			client := paruClient{config: runConfig{paruPath: paruPath, stdin: strings.NewReader(""), stdout: io.Discard, stderr: io.Discard}}
			if _, err := client.selectPackages([]string{"hello"}); err == nil || !strings.Contains(err.Error(), "incompatible Paru") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestParuOrderContractRecords(t *testing.T) {
	dir := t.TempDir()
	paruPath := writeExecutable(t, dir, "paru", `#!/bin/sh
printf 'REPO DEP core glibc\n'
printf 'AUR TARGET hello hello\n'
printf 'CONFLICT LOCAL hello old-hello <=1.0\n'
printf 'MISSING absent-dep hello\n'
exit 1
`)
	client := paruClient{config: runConfig{paruPath: paruPath, stdin: strings.NewReader(""), stdout: io.Discard, stderr: io.Discard}}

	result, err := client.order([]string{"hello"})
	if err != nil {
		t.Fatal(err)
	}
	want := orderResult{
		Status: 1,
		Records: []orderRecord{
			{Kind: "REPO", Fields: []string{"DEP", "core", "glibc"}},
			{Kind: "AUR", Fields: []string{"TARGET", "hello", "hello"}},
			{Kind: "CONFLICT", Fields: []string{"LOCAL", "hello", "old-hello", "<=1.0"}},
			{Kind: "MISSING", Fields: []string{"absent-dep", "hello"}},
		},
	}
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("result = %#v, want %#v", result, want)
	}
}

func TestParuPlanRejectsSRCINFORecordsBeforeExecution(t *testing.T) {
	result := orderResult{Records: []orderRecord{{Kind: "SRCINFO", Fields: []string{"TARGET", "hello", "hello", "pkgver", "1"}}}}
	if _, err := result.plan(); err == nil || !strings.Contains(err.Error(), "SRCINFO") {
		t.Fatalf("error = %v", err)
	}
}

func TestParuPlanSupportsVariableLengthAURSplitRecords(t *testing.T) {
	result := orderResult{Records: []orderRecord{{Kind: "AUR", Fields: []string{"TARGET", "split-member", "split-base", "extra", "metadata"}}}}
	plan, err := result.plan()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(plan.AURPkgbases, []string{"split-base"}) {
		t.Fatalf("pkgbases = %#v", plan.AURPkgbases)
	}
	if !reflect.DeepEqual(plan.AURTargetsByPkgbase["split-base"], []string{"split-member"}) {
		t.Fatalf("targets = %#v", plan.AURTargetsByPkgbase)
	}
	if err := result.ensureTargetsClassified([]string{"split-member"}); err != nil {
		t.Fatal(err)
	}
}

func TestParuOrderRejectsIncompatibleRecords(t *testing.T) {
	for _, output := range []string{
		"",
		"INSTALL DEP core glibc\n",
		"REPO TARGET only-two-fields\n",
		"SRCINFO TARGET hello hello extra\n",
		"MISSING\n",
	} {
		name := "empty"
		if fields := strings.Fields(output); len(fields) > 0 {
			name = fields[0]
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			paruPath := writeExecutable(t, dir, "paru", "#!/bin/sh\nprintf '%s' '"+output+"'\nexit 0\n")
			client := paruClient{config: runConfig{paruPath: paruPath, stdin: strings.NewReader(""), stdout: io.Discard, stderr: io.Discard}}
			result, err := client.order([]string{"hello"})
			if err == nil {
				_, err = result.plan()
			}
			if err == nil || !strings.Contains(err.Error(), "incompatible Paru") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestParuPlanRejectsUnclassifiedRequestedTarget(t *testing.T) {
	result := orderResult{Records: []orderRecord{{Kind: "AUR", Fields: []string{"DEP", "dep", "depbase"}}}}
	if _, err := result.plan(); err != nil {
		t.Fatal(err)
	}
	if err := result.ensureTargetsClassified([]string{"hello"}); err == nil || !strings.Contains(err.Error(), "not classified") {
		t.Fatalf("error = %v", err)
	}
}

func TestParuAcquireUsesGetPkgbuildInRequestedDirectory(t *testing.T) {
	dir := t.TempDir()
	cloneDir := filepath.Join(dir, "clones")
	if err := os.Mkdir(cloneDir, 0o755); err != nil {
		t.Fatal(err)
	}
	argsFile := filepath.Join(dir, "args")
	pwdFile := filepath.Join(dir, "pwd")
	paruPath := writeExecutable(t, dir, "paru", `#!/bin/sh
printf '%s\n' "$@" > "$ARGS_FILE"
pwd > "$PWD_FILE"
printf 'acquiring\n'
exit 0
`)
	t.Setenv("ARGS_FILE", argsFile)
	t.Setenv("PWD_FILE", pwdFile)
	var stdout bytes.Buffer
	client := paruClient{config: runConfig{paruPath: paruPath, stdin: strings.NewReader(""), stdout: &stdout, stderr: io.Discard}}

	if err := client.acquire("hello", cloneDir); err != nil {
		t.Fatal(err)
	}
	if got := readLines(t, argsFile); !reflect.DeepEqual(got, []string{"-G", "hello"}) {
		t.Fatalf("args = %#v", got)
	}
	if got := strings.TrimSpace(readString(t, pwdFile)); got != cloneDir {
		t.Fatalf("pwd = %q", got)
	}
	if stdout.String() != "acquiring\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
}

func writeExecutable(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func readLines(t *testing.T, path string) []string {
	t.Helper()
	return strings.Fields(readString(t, path))
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}
