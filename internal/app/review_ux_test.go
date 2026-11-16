package app

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func TestDecisionMenuAcceptsNumbersInitialsAndWords(t *testing.T) {
	allowed := []string{"approve", "inspect", "edit", "skip", "cancel"}
	tests := map[string]string{
		"1\n":       "approve",
		"a\n":       "approve",
		"approve\n": "approve",
		"2\n":       "inspect",
		"i\n":       "inspect",
		"3\n":       "edit",
		"e\n":       "edit",
		"4\n":       "skip",
		"s\n":       "skip",
		"5\n":       "cancel",
		"c\n":       "cancel",
	}
	for input, want := range tests {
		t.Run(strings.TrimSpace(input), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			got := askDecision(bufio.NewReader(strings.NewReader(input)), runConfig{stdout: &stdout, stderr: &stderr}, allowed)
			if got != want {
				t.Fatalf("askDecision(%q) = %q, want %q; stdout=%q stderr=%q", input, got, want, stdout.String(), stderr.String())
			}
		})
	}
}

func TestDecisionMenuNamesFullReportAndShowsMnemonicChoices(t *testing.T) {
	var stdout bytes.Buffer
	got := askDecision(
		bufio.NewReader(strings.NewReader("1\n")),
		runConfig{stdout: &stdout, stderr: io.Discard},
		[]string{"approve", "inspect", "edit", "skip", "cancel"},
	)
	if got != "approve" {
		t.Fatalf("decision = %q", got)
	}
	want := "Decision : [a]pprove | [i]nspect full report | [e]dit and re-audit | [s]kip | [c]ancel \n"
	if stdout.String() != want {
		t.Fatalf("menu = %q, want %q", stdout.String(), want)
	}
}

func TestReviewSummarySeparatesSections(t *testing.T) {
	var stdout bytes.Buffer
	printReviewSummary(runConfig{stdout: &stdout}, "hello", auditReport{Summary: "packaging looks conventional", Risk: "low"})

	want := "AUR audit: hello\n\n---\nAssessment: packaging looks conventional\n\nRisk: low\n---\n\n"
	if stdout.String() != want {
		t.Fatalf("summary = %q, want %q", stdout.String(), want)
	}
}

func TestCodexProgressUsesSingleLine(t *testing.T) {
	var stdout bytes.Buffer
	printCodexProgress(&stdout, 15*time.Second)

	want := "AURoscope: Codex audit still running (15s elapsed)...\n"
	if stdout.String() != want {
		t.Fatalf("progress = %q, want %q", stdout.String(), want)
	}
}

func TestAuditFailureMenuAcceptsNumberAndInitial(t *testing.T) {
	allowed := []string{"retry", "skip", "cancel"}
	for input, want := range map[string]string{"1\n": "retry", "r\n": "retry", "2\n": "skip", "s\n": "skip", "3\n": "cancel", "c\n": "cancel"} {
		var stdout bytes.Buffer
		got := askDecision(bufio.NewReader(strings.NewReader(input)), runConfig{stdout: &stdout, stderr: io.Discard}, allowed)
		if got != want {
			t.Fatalf("askDecision(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestAuditPromptScopesRiskToPackaging(t *testing.T) {
	prompt := strings.ToLower(auditPrompt())
	for _, want := range []string{
		"packaging",
		"pkgbuild",
		"install scripts",
		"source url",
		"checksums",
		"upstream software",
		"opacity of upstream or downloaded binaries alone",
		"recipe file supplied without text",
		"nul characters",
		"unchanged",
		"official upstream",
		"first full audit",
		"do not list inability to inspect upstream binary internals",
		"mode is unchanged",
		"low rather than unknown",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("audit prompt missing %q:\n%s", want, prompt)
		}
	}
}
