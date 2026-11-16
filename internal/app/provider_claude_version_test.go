package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Issue #103: the Claude Code floor mirrors ADR-0046's Codex admission.
func TestClaudeMinimumVersionAdmission(t *testing.T) {
	for _, tc := range []struct {
		banner string
		admit  bool
	}{
		{"2.1.325 (Claude Code)", true},
		{"2.1.325 (Claude Code)\n", true},
		{"2.1.326 (Claude Code)\r\n", true},
		{"2.1.1000 (Claude Code)", true},
		{"2.2.0 (Claude Code)", true},
		{"2.10.0 (Claude Code)", true},
		{"3.0.0 (Claude Code)", true},
		{"1000000000000000000000000000000.0.0 (Claude Code)", true},
		{"2.1.324 (Claude Code)", false},
		{"2.1.99 (Claude Code)", false},
		{"2.0.999 (Claude Code)", false},
		{"1.9.999 (Claude Code)", false},
		{"0.0.0 (Claude Code)", false},
		{"2.1.325-beta.1 (Claude Code)", false},
		{"2.1.325+build (Claude Code)", false},
		{"02.1.325 (Claude Code)", false},
		{"2.01.325 (Claude Code)", false},
		{"2.1.0325 (Claude Code)", false},
		{"v2.1.325 (Claude Code)", false},
		{"2.1 (Claude Code)", false},
		{"2.1.325.0 (Claude Code)", false},
		{"2.1.325", false},
		{"2.1.325 (Claude)", false},
		{"2.1.325  (Claude Code)", false},
		{" 2.1.325 (Claude Code)", false},
		{"2.1.325 (Claude Code) ", false},
		{"2.1.325 (Claude Code)\nwarning", false},
		{"2.1.325 (Claude Code)\n\n", false},
		{"2.1.325 (Claude Code)\r", false},
		{"２.1.325 (Claude Code)", false},
		{"codex-cli 2.1.325", false},
		{"", false},
	} {
		t.Run(tc.banner, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("CLAUDE_VERSION_BANNER", tc.banner)
			marker := filepath.Join(dir, "audit-started")
			t.Setenv("CLAUDE_AUDIT_MARKER", marker)
			t.Setenv("AUROSCOPE_CLAUDE", writeExecutable(t, dir, "claude", `#!/bin/sh
if test "$1" = "--version"; then
  printf '%s' "$CLAUDE_VERSION_BANNER"
  exit 0
fi
: > "$CLAUDE_AUDIT_MARKER"
exit 42
`))
			err := verifyClaudeVersion(runConfig{}.withDefaults())
			if tc.admit {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), "minimum stable version is 2.1.325") {
				t.Fatalf("version error = %v", err)
			}
			if _, err := auditClaude(auditBundle{}, auditConfig{}, runConfig{}); err == nil {
				t.Fatal("rejected version reached an audit")
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("audit ran for rejected version: %v", err)
			}
		})
	}
}

// A failing version probe is an audit failure whose diagnostics stay hidden.
func TestClaudeVersionFailureHidesDiagnostics(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AUROSCOPE_CLAUDE", writeExecutable(t, dir, "claude", "#!/bin/sh\nprintf 'credential-fixture' >&2\nprintf '2.1.325 (Claude Code)'\nexit 1\n"))
	_, err := auditClaude(auditBundle{}, auditConfig{}, runConfig{})
	if err == nil || strings.Contains(err.Error(), "credential-fixture") || !strings.Contains(err.Error(), "version check failed") {
		t.Fatalf("error = %v", err)
	}
	t.Setenv("AUROSCOPE_CLAUDE", writeExecutable(t, dir, "claude2", "#!/bin/sh\nprintf '1.0.0 (Claude Code)\\033]0;x\\007'\n"))
	if err := verifyClaudeVersion(runConfig{}.withDefaults()); err == nil || strings.ContainsRune(err.Error(), 0x1b) {
		t.Fatalf("unescaped banner: %v", err)
	}
}
