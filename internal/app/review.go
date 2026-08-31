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
	if len(plan.AURPkgbases) == 0 {
		return finishNative(o.paru, originalArgs, plan.RepoTargets)
	}

	var reviewed []reviewedPackage
	for _, pkgbase := range plan.AURPkgbases {
		result, err := o.reviewPackage(store, pkgbase)
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
			finalTargets = append(finalTargets, item.Identity.Pkgbase)
		}
	}
	if len(finalTargets) == 0 {
		fmt.Fprintln(o.config.stderr, "auroscope: all AUR targets skipped")
		return nil
	}
	if len(approved) == 0 {
		result := o.paru.finalInstall(append([]string{"-S"}, plan.RepoTargets...))
		if result.status != 0 {
			return childExit{status: result.status}
		}
		return nil
	}
	txPath, confPath, err := writeTransactionFiles(approved)
	if err != nil {
		return err
	}
	defer os.Remove(txPath)
	defer os.Remove(confPath)
	args := []string{"-S", "--skipreview", "--config", confPath}
	args = append(args, finalTargets...)
	result := o.paru.finalInstall(args)
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
	result := paru.finalInstall(args)
	if result.status != 0 {
		return childExit{status: result.status}
	}
	return nil
}

func (o orchestrator) reviewPackage(store *stateStore, pkgbase string) (reviewedPackage, error) {
	if err := o.paru.acquire(pkgbase, o.config.cloneDir); err != nil {
		return reviewedPackage{}, err
	}
	dir, err := worktreePath(o.config.cloneDir, pkgbase)
	if err != nil {
		return reviewedPackage{}, err
	}
	reader := bufio.NewReader(o.config.reviewInput)
	for {
		baseline, err := store.baseline(pkgbase)
		if err != nil {
			return reviewedPackage{}, err
		}
		bundle, err := buildAuditBundle(pkgbase, dir, baseline)
		if err != nil {
			return reviewedPackage{}, err
		}
		report, err := (codexClient{path: o.config.codexPath}).audit(bundle)
		if err != nil {
			fmt.Fprintf(o.config.stderr, "auroscope: audit failed for %s: %v\n", pkgbase, err)
			switch askDecision(reader, o.config, []string{"retry", "skip", "cancel"}) {
			case "retry":
				continue
			case "skip":
				_ = store.recordAudit(pkgbase, bundle.Identity, bundle.PreviousCommit, auditReport{Summary: "audit failed", Risk: "unknown"}, string(decisionSkip))
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
			_ = store.recordAudit(pkgbase, bundle.Identity, bundle.PreviousCommit, report, string(decisionCancel))
			return reviewedPackage{Identity: bundle.Identity, Decision: decisionCancel}, nil
		}
	}
}

func printReview(config runConfig, pkgbase string, bundle auditBundle, report auditReport) {
	fmt.Fprintf(config.stdout, "\nAUR audit: %s\ncommit: %s\nmanifest: %s\nrisk: %s\n%s\n", pkgbase, bundle.Identity.Commit, bundle.Identity.ManifestDigest, report.Risk, report.Summary)
	for _, finding := range report.Findings {
		fmt.Fprintf(config.stdout, "- %s:%d %s\n", finding.File, finding.Line, finding.Explanation)
	}
	if bundle.Diff != "" {
		fmt.Fprintf(config.stdout, "\nDiff:\n%s\n", bundle.Diff)
	}
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

func writeTransactionFiles(approved []recipeIdentity) (string, string, error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	dir, err := os.MkdirTemp(runtimeDir, "auroscope-tx-*")
	if err != nil && runtimeDir != "" {
		dir, err = os.MkdirTemp("", "auroscope-tx-*")
	}
	if err != nil {
		return "", "", err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", "", err
	}
	txPath := filepath.Join(dir, "approved.json")
	confPath := filepath.Join(dir, "paru.conf")
	data, err := json.MarshalIndent(approved, "", "  ")
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
	conf := fmt.Sprintf("[options]\nPreBuildCommand = %s __guard %s\n", exe, txPath)
	if err := os.WriteFile(confPath, []byte(conf), 0o600); err != nil {
		return "", "", err
	}
	return txPath, confPath, nil
}
