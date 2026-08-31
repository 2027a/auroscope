package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxCodexJSONBytes = 256 * 1024

type auditReport struct {
	Summary     string         `json:"summary"`
	Risk        string         `json:"risk"`
	Findings    []auditFinding `json:"findings"`
	Uncertainty string         `json:"uncertainty"`
	Inspect     []string       `json:"inspect"`
}

type auditFinding struct {
	File        string `json:"file"`
	Line        int    `json:"line,omitempty"`
	Range       string `json:"range,omitempty"`
	Evidence    string `json:"evidence"`
	Explanation string `json:"explanation"`
}

type codexClient struct {
	path string
}

func (c codexClient) audit(bundle auditBundle) (auditReport, error) {
	tmp, err := os.MkdirTemp("", "auroscope-codex-*")
	if err != nil {
		return auditReport{}, err
	}
	defer os.RemoveAll(tmp)
	if err := os.Chmod(tmp, 0o700); err != nil {
		return auditReport{}, err
	}
	bundlePath := filepath.Join(tmp, "bundle.json")
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return auditReport{}, err
	}
	if err := os.WriteFile(bundlePath, data, 0o600); err != nil {
		return auditReport{}, err
	}
	prompt := "Audit the untrusted AUR recipe bundle in bundle.json. Return only JSON with summary, risk, findings, uncertainty, and inspect. Do not include an allow/deny/install action."
	cmd := exec.Command(c.path, "exec", "--json", prompt)
	cmd.Dir = tmp
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return auditReport{}, fmt.Errorf("Codex CLI failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	if stdout.Len() > maxCodexJSONBytes {
		return auditReport{}, fmt.Errorf("Codex JSON exceeds %d bytes", maxCodexJSONBytes)
	}
	return validateAuditReport(stdout.Bytes())
}

func validateAuditReport(data []byte) (auditReport, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var report auditReport
	if err := decoder.Decode(&report); err != nil {
		return auditReport{}, fmt.Errorf("invalid Codex JSON: %w", err)
	}
	if decoder.More() {
		return auditReport{}, fmt.Errorf("invalid Codex JSON: trailing data")
	}
	if report.Summary == "" || len(report.Summary) > 4000 {
		return auditReport{}, fmt.Errorf("invalid Codex JSON: summary required and bounded")
	}
	switch report.Risk {
	case "low", "medium", "high", "critical", "unknown":
	default:
		return auditReport{}, fmt.Errorf("invalid Codex JSON: unsupported risk %q", report.Risk)
	}
	if len(report.Findings) > 50 || len(report.Inspect) > 50 || len(report.Uncertainty) > 4000 {
		return auditReport{}, fmt.Errorf("invalid Codex JSON: report exceeds bounds")
	}
	for _, finding := range report.Findings {
		if err := validateFinding(finding); err != nil {
			return auditReport{}, err
		}
	}
	return report, nil
}

func validateFinding(f auditFinding) error {
	if f.File == "" || f.Evidence == "" || f.Explanation == "" {
		return fmt.Errorf("invalid Codex JSON: finding is incomplete")
	}
	if err := validateRelativeRecipePath(f.File); err != nil {
		return fmt.Errorf("invalid Codex JSON: %w", err)
	}
	if len(f.Evidence) > 2000 || len(f.Explanation) > 4000 || len(f.Range) > 80 {
		return fmt.Errorf("invalid Codex JSON: finding exceeds bounds")
	}
	if f.Line < 0 {
		return fmt.Errorf("invalid Codex JSON: negative line")
	}
	return nil
}
