package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEffectiveParuConfigRejectsInvalidExplicitPath(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-paru.conf")
	t.Setenv("PARU_CONF", missing)

	if _, err := effectiveParuConfig(); err == nil || !strings.Contains(err.Error(), "PARU_CONF") {
		t.Fatalf("effectiveParuConfig error = %v", err)
	}
}

func TestEffectiveParuConfigRejectsExplicitDirectory(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PARU_CONF", dir)

	if _, err := effectiveParuConfig(); err == nil || !strings.Contains(err.Error(), "regular") {
		t.Fatalf("effectiveParuConfig error = %v", err)
	}
}

func TestEffectiveParuConfigUsesExplicitRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "paru.conf")
	if err := os.WriteFile(path, []byte("[options]\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PARU_CONF", path)

	got, err := effectiveParuConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got != path {
		t.Fatalf("effectiveParuConfig = %q, want %q", got, path)
	}
}
