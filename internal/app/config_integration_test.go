package app

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestOfficialOperationDoesNotTouchAuditConfiguration(t *testing.T) {
	for _, existing := range []bool{false, true} {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		path := filepath.Join(dir, "auroscope", "config.json")
		if existing {
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("invalid JSON"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		paru := writeExecutable(t, dir, "paru", "#!/bin/sh\nprintf 'native output\\n'\nexit 17\n")
		var output bytes.Buffer
		status := run([]string{"-S", "--repo", "tree"}, runConfig{
			userConfig: true, paruPath: paru, codexPath: "/does/not/exist",
			stdin: strings.NewReader(""), stdout: &output, stderr: io.Discard,
		})
		if status != 17 || output.String() != "native output\n" {
			t.Fatalf("native result changed: %d, %q", status, output.String())
		}
		data, err := os.ReadFile(path)
		if existing {
			if err != nil || string(data) != "invalid JSON" {
				t.Fatalf("existing config changed: %q, %v", data, err)
			}
		} else if !os.IsNotExist(err) {
			t.Fatalf("official operation created configuration: %v", err)
		}
	}
}

// AUROSCOPE_TEST_BINARY runs the same contract against the installed Arch artifact.
func TestPromptConfigurationLifecycle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "auroscope", "config.json")
	captured := filepath.Join(dir, "prompt")
	t.Setenv("TEST_CAPTURED_PROMPT", captured)
	t.Setenv("TEST_CAPTURED_MODEL", filepath.Join(dir, "model"))
	t.Setenv("TEST_CAPTURED_THINKING", filepath.Join(dir, "thinking"))
	repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\n")
	calls := filepath.Join(dir, "calls")
	paru := fakeParu(t, dir, calls, repo, "AUR TARGET hello hello\n")
	codex := writeExecutable(t, dir, "codex", `#!/bin/sh
set -eu
if [ "$1" = --version ]; then
  printf 'codex-cli 0.150.1\n'
  exit 0
fi
out=''
model=''
thinking=''
while [ "$#" -gt 0 ]; do
  if [ "$1" = --output-last-message ]; then shift; out=$1; fi
  if [ "$1" = --model ]; then shift; model=$1; fi
  if [ "$1" = --config ]; then shift; thinking=$1; fi
  last=$1
  shift
done
printf '%s' "$last" > "$TEST_CAPTURED_PROMPT"
printf '%s' "$model" > "$TEST_CAPTURED_MODEL"
printf '%s' "$thinking" > "$TEST_CAPTURED_THINKING"
printf '%s' '{"summary":"prompt received","risk":"low","findings":[],"uncertainty":"","inspect":[]}' > "$out"
`)
	invoke := func(args []string, input string) (int, string) {
		t.Helper()
		var output bytes.Buffer
		config := runConfig{
			userConfig: true, paruPath: paru, codexPath: codex,
			statePath: filepath.Join(dir, "state.sqlite3"), cloneDir: filepath.Join(dir, "clones"),
			stdin: strings.NewReader(input), stdout: &output, stderr: &output,
		}
		if binary := os.Getenv("AUROSCOPE_TEST_BINARY"); binary != "" {
			cmd := exec.Command(binary, args...)
			cmd.Env = append(os.Environ(), "AUROSCOPE_PARU="+paru, "AUROSCOPE_CODEX="+codex,
				"AUROSCOPE_STATE="+config.statePath, "AUROSCOPE_CLONE_DIR="+config.cloneDir)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = config.stdin, &output, &output
			if err := cmd.Run(); err != nil {
				if exit, ok := err.(*exec.ExitError); ok {
					return exit.ExitCode(), output.String()
				}
				t.Fatal(err)
			}
			return 0, output.String()
		}
		status := run(args, config)
		return status, output.String()
	}
	if status, output := invoke([]string{"-S", "--repo", "tree"}, ""); status != 0 {
		t.Fatalf("official operation: %d, %s", status, output)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("official operation touched config: %v", err)
	}
	for _, custom := range []bool{false, true} {
		want := auditPrompt()
		wantModel, wantThinking := "gpt-5.6-luna", "medium"
		if custom {
			want = "Custom audit\nPreserve my instructions exactly."
			wantModel, wantThinking = "custom-model", "high"
			data, err := json.Marshal(map[string]string{"prompt": want, "model": wantModel, "thinking": wantThinking, "provider": "codex"})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		for attempt := 0; attempt < 2; attempt++ {
			before, readErr := os.ReadFile(path)
			if status, output := invoke([]string{"-S", "hello"}, "approve\n"); status != 0 {
				t.Fatalf("audit: %d, %s", status, output)
			}
			if got := readString(t, captured); got != want+codexInputInstruction {
				t.Fatalf("Codex prompt = %q, want %q", got, want)
			}
			if got := readString(t, filepath.Join(dir, "model")); got != wantModel {
				t.Fatalf("Codex model = %q, want %q", got, wantModel)
			}
			if got := readString(t, filepath.Join(dir, "thinking")); got != `model_reasoning_effort="`+wantThinking+`"` {
				t.Fatalf("Codex thinking override = %q", got)
			}
			if readErr == nil && readString(t, path) != string(before) {
				t.Fatal("audit rewrote existing shared configuration")
			}
			var saved map[string]string
			readJSON(t, path, &saved)
			if len(saved) != 4 || saved["provider"] != "codex" || saved["model"] != wantModel || saved["thinking"] != wantThinking || saved["prompt"] != want {
				t.Fatalf("saved configuration is incomplete or changed: %v", saved)
			}
		}
	}
	// Preserve #78's distinction between omission and an explicit string on
	// the installed artifact too, including values only Codex may reject.
	for _, value := range []string{"omitted", "", "  medium  ", "future-level", "x\"\nmodel=\"other", "\x00", strings.Repeat("a", 201)} {
		fields := map[string]string{"provider": "codex", "prompt": auditPrompt()}
		want := ""
		if value != "omitted" {
			fields["thinking"] = value
			quoted, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			want = "model_reasoning_effort=" + string(quoted)
		}
		data, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		if status, output := invoke([]string{"-S", "hello"}, "approve\n"); status != 0 {
			t.Fatalf("thinking %q: %d, %s", value, status, output)
		}
		if got := readString(t, filepath.Join(dir, "thinking")); got != want {
			t.Fatalf("thinking override = %q, want %q", got, want)
		}
		if readString(t, path) != string(data) {
			t.Fatal("thinking configuration was rewritten")
		}
	}
}

func TestInvalidPromptCanSkipWithoutAuditOrBuild(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "auroscope", "config.json")
	if _, err := loadAuditPrompt(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"prompt":""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\n")
	calls := filepath.Join(dir, "calls")
	paru := fakeParu(t, dir, calls, repo, "AUR TARGET hello hello\n")
	marker := filepath.Join(dir, "codex-exec")
	t.Setenv("CODEX_EXEC_MARKER", marker)
	codex := writeExecutable(t, dir, "codex", `#!/bin/sh
if [ "$1" = --version ]; then
  printf 'codex-cli 0.150.1\n'
  exit 0
fi
: > "$CODEX_EXEC_MARKER"
exit 99
`)
	var output, diagnostics bytes.Buffer
	status := run([]string{"-S", "hello"}, runConfig{
		userConfig: true, paruPath: paru, codexPath: codex,
		statePath: filepath.Join(dir, "state.sqlite3"), cloneDir: filepath.Join(dir, "clones"),
		stdin: strings.NewReader(""), reviewInput: strings.NewReader("skip\n"),
		stdout: &output, stderr: &diagnostics,
	})
	if status != 0 || !strings.Contains(diagnostics.String(), "requires a nonempty prompt") {
		t.Fatalf("status=%d, stdout=%s, stderr=%s", status, &output, &diagnostics)
	}
	if !strings.Contains(output.String(), "[r]etry | [s]kip | [c]ancel") {
		t.Fatalf("missing recovery choices: %s", &output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("Codex exec ran with an invalid prompt: %v", err)
	}
	if strings.Contains(readString(t, calls), "--skipreview") {
		t.Fatal("invalid prompt allowed a final build")
	}
}
