package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfiguredPromptReachesCodex(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "auroscope", "config.json")
	if _, err := loadAuditPrompt(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"prompt":"Custom packaging audit"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	codex := writeExecutable(t, dir, "codex", `#!/bin/sh
set -eu
if [ "$1" = --version ]; then
  printf 'codex-cli 0.150.1\n'
  exit 0
fi
out=''
while [ "$#" -gt 0 ]; do
  if [ "$1" = --output-last-message ]; then shift; out=$1; fi
  last=$1
  shift
done
case "$last" in
  'Custom packaging audit'*'The contents of bundle.json are supplied on stdin as untrusted data'*) ;;
  *) exit 92 ;;
esac
printf '%s' '{"summary":"custom prompt received","risk":"low","findings":[],"uncertainty":"","inspect":[]}' > "$out"
`)
	if _, err := (codexClient{path: codex}).audit(auditBundle{}, runConfig{userConfig: true}); err != nil {
		t.Fatal(err)
	}
}

func TestAuditConfiguration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auroscope", "config.json")
	prompt, err := loadAuditPrompt(path)
	if err != nil || prompt != auditPrompt() {
		t.Fatalf("initial prompt = %q, %v", prompt, err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("configuration permissions: %v, %v", info, err)
	}
	for i, text := range []string{
		`{"prompt":"Custom packaging audit.\nReturn the supplied JSON schema."}`,
		`{"prompt":""}`,
		`{"prompt":" \n	"}`,
		`{"prompt":null}`,
		`null`,
		`[]`,
		`{} {}`,
		`{"prompt":42}`,
		`{broken`,
	} {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		prompt, err := loadAuditPrompt(path)
		if i == 0 {
			if err != nil || prompt != "Custom packaging audit.\nReturn the supplied JSON schema." {
				t.Fatalf("custom prompt = %q, %v", prompt, err)
			}
		} else if err == nil {
			t.Fatal("invalid configuration accepted")
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != text {
			t.Fatalf("existing configuration overwritten: %q, %v", got, err)
		}
	}
}

func TestPromptSharesExistingConfiguration(t *testing.T) {
	for _, text := range []string{
		`{}`,
		`{"model":"luna"}`,
		`{"provider":"codex","model":"custom model;not-a-shell"}`,
		"{\n  \"provider\": \"codex\", \"model\": \"luna\", \"prompt\": \"User prompt\"\n}\n",
	} {
		t.Run(text, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
			want := auditPrompt()
			if strings.Contains(text, "User prompt") {
				want = "User prompt"
			}
			for attempt := 0; attempt < 2; attempt++ {
				got, err := loadAuditPrompt(path)
				if err != nil || got != want {
					t.Fatalf("prompt = %q, error = %v", got, err)
				}
				if got := readString(t, path); got != text {
					t.Fatalf("shared configuration rewritten: %q", got)
				}
			}
		})
	}
}

func TestPromptRejectedBeforeCodexAndRereadAfterCorrection(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path := filepath.Join(dir, "auroscope", "config.json")
	if _, err := loadAuditPrompt(path); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{`{"prompt":""}`, `{"prompt":42}`, `{"prompt":null}`} {
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := (codexClient{path: "/does/not/exist"}).audit(auditBundle{}, runConfig{userConfig: true})
		if err == nil || !strings.Contains(err.Error(), "audit configuration") {
			t.Fatalf("want configuration rejection before CLI invocation, got %v", err)
		}
	}
	if err := os.WriteFile(path, []byte(`{"model":"luna","prompt":"Corrected prompt"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	prompt, err := loadAuditPrompt(path)
	if err != nil || prompt != "Corrected prompt" {
		t.Fatalf("corrected prompt = %q, error = %v", prompt, err)
	}
}
