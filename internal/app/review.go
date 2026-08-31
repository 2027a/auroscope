package app

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type decision string

const (
	decisionApprove decision = "approve"
	decisionSkip    decision = "skip"
	decisionCancel  decision = "cancel"
)

type reviewedPackage struct {
	Identity recipeIdentity
	Decision decision
}

type orchestrator struct {
	config runConfig
	paru   paruClient
}

type childExit struct {
	status int
}

func (e childExit) Error() string {
	return fmt.Sprintf("child exited with status %d", e.status)
}

func (o orchestrator) run(originalArgs, targets []string) error {
	if err := ensurePrivateDir(o.config.cloneDir); err != nil {
		return fmt.Errorf("prepare clone directory: %w", err)
	}
	store, err := openState(o.config.statePath)
	if err != nil {
		return fmt.Errorf("open state: %w", err)
	}
	defer store.Close()

	if len(targets) == 0 {
		fmt.Fprintln(o.config.stderr, "auroscope: no AUR targets selected")
		return nil
	}
	order, err := o.paru.order(targets)
	if err != nil {
		return err
	}
	plan, err := order.plan()
	if err != nil {
		return err
	}
	if err := order.ensureTargetsClassified(targets); err != nil {
		return err
	}
	if len(plan.AURPkgbases) == 0 {
		return finishNative(o.paru, originalArgs, plan.RepoTargets)
	}

	reader := bufio.NewReader(o.config.reviewInput)
	var reviewed []reviewedPackage
	for _, pkgbase := range plan.AURPkgbases {
		result, err := o.reviewPackage(store, pkgbase, reader)
		if err != nil {
			return err
		}
		reviewed = append(reviewed, result)
		if result.Decision == decisionCancel {
			return childExit{status: statusFailure}
		}
	}

	var approved []recipeIdentity
	var finalTargets []string
	finalTargets = append(finalTargets, plan.RepoTargets...)
	for _, item := range reviewed {
		if item.Decision == decisionApprove {
			approved = append(approved, item.Identity)
			finalTargets = append(finalTargets, plan.AURTargetsByPkgbase[item.Identity.Pkgbase]...)
		}
	}
	if len(finalTargets) == 0 {
		fmt.Fprintln(o.config.stderr, "auroscope: all AUR targets skipped")
		return nil
	}
	if len(approved) == 0 {
		result := o.paru.finalInstall(append([]string{"-S"}, plan.RepoTargets...), nil)
		if result.status != 0 {
			return childExit{status: result.status}
		}
		return nil
	}
	txDir, confPath, err := writeTransactionFiles(approved, o.config.cloneDir)
	if err != nil {
		return err
	}
	defer os.RemoveAll(txDir)
	args := []string{"-S", "--skipreview"}
	args = append(args, finalTargets...)
	result := o.paru.finalInstall(args, []string{"PARU_CONF=" + confPath})
	if result.status != 0 {
		return childExit{status: result.status}
	}
	if err := store.advanceBaselines(approved); err != nil {
		return fmt.Errorf("advance baselines: %w", err)
	}
	return nil
}

func finishNative(paru paruClient, originalArgs, repoTargets []string) error {
	args := originalArgs
	if len(args) == 0 {
		args = append([]string{"-S"}, repoTargets...)
	}
	result := paru.finalInstall(args, nil)
	if result.status != 0 {
		return childExit{status: result.status}
	}
	return nil
}

func (o orchestrator) reviewPackage(store *stateStore, pkgbase string, reader *bufio.Reader) (reviewedPackage, error) {
	if err := o.paru.acquire(pkgbase, o.config.cloneDir); err != nil {
		return reviewedPackage{}, err
	}
	dir, err := worktreePath(o.config.cloneDir, pkgbase)
	if err != nil {
		return reviewedPackage{}, err
	}
	for {
		baseline, err := store.baseline(pkgbase)
		if err != nil {
			return reviewedPackage{}, err
		}
		bundle, err := buildAuditBundle(pkgbase, dir, baseline)
		if err != nil {
			return reviewedPackage{}, err
		}
		report, err := (codexClient{path: o.config.codexPath, model: o.config.codexModel, timeout: o.config.codexTimeout}).audit(bundle)
		if err != nil {
			fmt.Fprintf(o.config.stderr, "auroscope: audit failed for %s: %v\n", pkgbase, err)
			switch askDecision(reader, o.config, []string{"retry", "skip", "cancel"}) {
			case "retry":
				continue
			case "skip":
				if err := store.recordAudit(pkgbase, bundle.Identity, bundle.PreviousCommit, auditReport{Summary: "audit failed", Risk: "unknown"}, string(decisionSkip)); err != nil {
					return reviewedPackage{}, err
				}
				return reviewedPackage{Identity: bundle.Identity, Decision: decisionSkip}, nil
			default:
				return reviewedPackage{Identity: bundle.Identity, Decision: decisionCancel}, nil
			}
		}
		printReview(o.config, pkgbase, bundle, report)
		switch askDecision(reader, o.config, []string{"approve", "inspect", "edit", "skip", "cancel"}) {
		case "approve":
			if err := store.recordAudit(pkgbase, bundle.Identity, bundle.PreviousCommit, report, string(decisionApprove)); err != nil {
				return reviewedPackage{}, err
			}
			return reviewedPackage{Identity: bundle.Identity, Decision: decisionApprove}, nil
		case "inspect":
			printReview(o.config, pkgbase, bundle, report)
		case "edit":
			if err := editAndSnapshot(o.config, dir); err != nil {
				return reviewedPackage{}, err
			}
		case "skip":
			if err := store.recordAudit(pkgbase, bundle.Identity, bundle.PreviousCommit, report, string(decisionSkip)); err != nil {
				return reviewedPackage{}, err
			}
			return reviewedPackage{Identity: bundle.Identity, Decision: decisionSkip}, nil
		case "cancel":
			if err := store.recordAudit(pkgbase, bundle.Identity, bundle.PreviousCommit, report, string(decisionCancel)); err != nil {
				return reviewedPackage{}, err
			}
			return reviewedPackage{Identity: bundle.Identity, Decision: decisionCancel}, nil
		}
	}
}

func printReview(config runConfig, pkgbase string, bundle auditBundle, report auditReport) {
	fmt.Fprintf(config.stdout, "\nAUR audit: %s\ncommit: %s\nmanifest: %s\nrisk: %s\n%s\n", escapeTerminal(pkgbase), bundle.Identity.Commit, bundle.Identity.ManifestDigest, escapeTerminal(report.Risk), escapeTerminal(report.Summary))
	for _, finding := range report.Findings {
		location := escapeTerminal(finding.File)
		if finding.LineStart > 0 {
			location = fmt.Sprintf("%s:%d-%d", location, finding.LineStart, finding.LineEnd)
		} else if finding.Line > 0 {
			location = fmt.Sprintf("%s:%d", location, finding.Line)
		}
		if finding.Range != "" {
			location = fmt.Sprintf("%s %s", location, escapeTerminal(finding.Range))
		}
		fmt.Fprintf(config.stdout, "- %s\n  evidence: %s\n  explanation: %s\n", location, escapeTerminal(finding.Evidence), escapeTerminal(finding.Explanation))
	}
	if len(report.Uncertainties) > 0 {
		fmt.Fprintln(config.stdout, "\nUncertainties:")
		for _, uncertainty := range report.Uncertainties {
			fmt.Fprintf(config.stdout, "- %s\n", escapeTerminal(uncertainty))
		}
	}
	if len(report.SuggestedInspection) > 0 {
		fmt.Fprintln(config.stdout, "\nSuggested inspection:")
		for _, path := range report.SuggestedInspection {
			fmt.Fprintf(config.stdout, "- %s\n", escapeTerminal(path))
		}
	}
	if bundle.Diff != "" {
		fmt.Fprintf(config.stdout, "\nDiff:\n%s\n", escapeTerminal(bundle.Diff))
	}
}

func escapeTerminal(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069):
			fmt.Fprintf(&b, "\\u%04x", r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func askDecision(reader *bufio.Reader, config runConfig, allowed []string) string {
	allowedSet := map[string]bool{}
	for _, value := range allowed {
		allowedSet[value] = true
	}
	for {
		fmt.Fprintf(config.stdout, "Decision [%s]: ", strings.Join(allowed, "/"))
		line, err := reader.ReadString('\n')
		if err != nil {
			return "cancel"
		}
		choice := strings.ToLower(strings.TrimSpace(line))
		if allowedSet[choice] {
			return choice
		}
		fmt.Fprintf(config.stderr, "auroscope: unsupported decision %q\n", choice)
	}
}

func editAndSnapshot(config runConfig, dir string) error {
	if config.editorPath == "" {
		return errors.New("EDITOR is required for edit")
	}
	cmd := exec.Command(config.editorPath, dir)
	cmd.Dir = dir
	cmd.Stdin = config.stdin
	cmd.Stdout = config.stdout
	cmd.Stderr = config.stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("editor failed: %w", err)
	}
	return snapshotEditedWorktree(dir)
}

func writeTransactionFiles(approved []recipeIdentity, cloneDir string) (string, string, error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	dir, err := os.MkdirTemp(runtimeDir, "auroscope-tx-*")
	if err != nil && runtimeDir != "" {
		dir, err = os.MkdirTemp("", "auroscope-tx-*")
	}
	if err != nil {
		return "", "", err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(dir)
		}
	}()
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", "", err
	}
	txPath := filepath.Join(dir, "approved.json")
	confPath := filepath.Join(dir, "paru.conf")
	transactionApprovals := make([]transactionApproval, 0, len(approved))
	for _, identity := range approved {
		worktree, err := worktreePath(cloneDir, identity.Pkgbase)
		if err != nil {
			return "", "", err
		}
		worktree, err = canonicalWorktree(worktree)
		if err != nil {
			return "", "", fmt.Errorf("resolve approved worktree for %s: %w", identity.Pkgbase, err)
		}
		transactionApprovals = append(transactionApprovals, transactionApproval{recipeIdentity: identity, Worktree: worktree})
	}
	data, err := json.MarshalIndent(transactionApprovals, "", "  ")
	if err != nil {
		return "", "", err
	}
	if err := os.WriteFile(txPath, data, 0o600); err != nil {
		return "", "", err
	}
	exe, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	conf := "[options]\n"
	original, err := effectiveParuConfig()
	if err != nil {
		return "", "", err
	}
	if original != "" {
		conf += fmt.Sprintf("Include = %s\n[options]\n", original)
	}
	conf += fmt.Sprintf("CloneDir = %s\n", cloneDir)
	conf += fmt.Sprintf("\n[bin]\nPreBuildCommand = %s __guard %s\n", posixShellQuote(exe), posixShellQuote(txPath))
	if err := os.WriteFile(confPath, []byte(conf), 0o600); err != nil {
		return "", "", err
	}
	complete = true
	return dir, confPath, nil
}

func effectiveParuConfig() (string, error) {
	if path := os.Getenv("PARU_CONF"); path != "" {
		if !configFileExists(path) {
			return "", fmt.Errorf("PARU_CONF does not name a regular readable config file: %s", path)
		}
		return path, nil
	}
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		if path := filepath.Join(xdg, "paru", "paru.conf"); configFileExists(path) {
			return path, nil
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if path := filepath.Join(home, ".config", "paru", "paru.conf"); configFileExists(path) {
			return path, nil
		}
	}
	if configFileExists("/etc/paru.conf") {
		return "/etc/paru.conf", nil
	}
	return "", nil
}

func configFileExists(path string) bool {
	if strings.ContainsAny(path, "\x00\r\n") {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func posixShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
