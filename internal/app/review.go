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

func (o orchestrator) run(originalArgs, targets []string) (runErr error) {
	if len(targets) == 0 {
		fmt.Fprintln(o.config.stderr, "auroscope: no AUR targets selected")
		return nil
	}
	order, err := o.paru.order(targets)
	if err != nil {
		return err
	}
	plan, err := order.plan(targets...)
	if err != nil {
		return err
	}
	if len(plan.AURPkgbases) == 0 {
		return finishNative(o.paru, originalArgs, plan.RepoTargets)
	}

	// Audit resources belong only to transactions containing AUR recipes.
	parsed, err := parseParuArguments(originalArgs)
	if err != nil {
		return err
	}
	o.config.cloneDir, _ = operationCloneOptions(parsed.options, o.config.cloneDir, false)
	op, err := beginTemporaryOperation(o.config.cloneDir)
	if err != nil {
		return fmt.Errorf("prepare clone directory: %w", err)
	}
	o.config.operation = op
	o.config.cloneDir = filepath.Join(op.dir, "clones")
	stop := watchOperationSignals(&o.config)
	defer func() {
		stop()
		if o.config.interruption != nil {
			select {
			case <-o.config.interruption.done:
				runErr = childExit{status: o.config.interruption.status}
			default:
			}
		}
		if err := op.close(); err != nil {
			fmt.Fprintf(o.config.stderr, "auroscope: temporary cleanup: %v\n", err)
			if runErr == nil {
				runErr = err
			}
		}
	}()
	o.paru.config = o.config
	store, err := openState(o.config.statePath)
	if err != nil {
		return fmt.Errorf("open state: %w", err)
	}
	defer store.Close()

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

	for {
		err := o.installReviewed(store, originalArgs, plan, reviewed)
		var drift recipeDrift
		if !errors.As(err, &drift) {
			return err
		}
		fmt.Fprintf(o.config.stdout, "\nAURoscope: %s.\nRetry acquires and audits this package again, then asks for approval.\nSkip omits it; Paru may refuse remaining targets that depend on it.\n", drift)
		switch askDecision(reader, o.config, []string{"retry", "skip", "cancel"}) {
		case "retry":
			for i := range reviewed {
				if reviewed[i].Identity.Pkgbase != drift.Pkgbase {
					continue
				}
				item, err := o.reviewPackage(store, drift.Pkgbase, reader)
				if err != nil {
					return err
				}
				if item.Decision == decisionCancel {
					return childExit{status: statusFailure}
				}
				reviewed[i] = item
			}
		case "skip":
			for i := range reviewed {
				if reviewed[i].Identity.Pkgbase == drift.Pkgbase {
					reviewed[i].Decision = decisionSkip
				}
			}
		default:
			fmt.Fprintln(o.config.stdout, "AURoscope: AUR phase cancelled; no retry was started.")
			return childExit{status: statusFailure}
		}
	}
}

func (o orchestrator) installReviewed(store *stateStore, originalArgs []string, plan resolvedPlan, reviewed []reviewedPackage) error {
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
		args, err := auditedInstallArgs(originalArgs, plan.RepoTargets)
		if err != nil {
			return err
		}
		result := o.paru.finalInstall(args, nil)
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
	args, err := auditedInstallArgs(originalArgs, finalTargets, "--skipreview")
	if err != nil {
		return err
	}
	_, args = operationCloneOptions(args, o.config.cloneDir, true)

	result := o.paru.finalInstall(args, []string{"PARU_CONF=" + confPath})
	if result.status != 0 {
		// Only a guard-produced result for an approved package is recoverable.
		// Signals, startup errors and ordinary Paru failures keep their status.
		if result.status > 0 && result.status < 128 {
			data, err := os.ReadFile(filepath.Join(txDir, "approved.json.drift.json"))
			var drift recipeDrift
			if err == nil && json.Unmarshal(data, &drift) == nil {
				for _, identity := range approved {
					if drift.Pkgbase == identity.Pkgbase {
						return drift
					}
				}
			}
		}
		return childExit{status: result.status}
	}
	if o.config.interruption != nil {
		select {
		case <-o.config.interruption.done:
			return childExit{status: o.config.interruption.status}
		default:
		}
	}
	if err := store.advanceBaselines(approved); err != nil {
		return fmt.Errorf("advance baselines: %w", err)
	}
	return nil
}

func finishNative(paru paruClient, originalArgs, repoTargets []string) error {
	args := originalArgs
	if len(args) == 0 || !strings.HasPrefix(args[0], "-") {
		// Search terms have already been resolved by native selection. Install
		// only those selected targets rather than opening the search again.
		args = append([]string{"-S", "--"}, repoTargets...)
	}
	result := paru.finalInstall(args, nil)
	if result.status != 0 {
		return childExit{status: result.status}
	}
	return nil
}

func auditedInstallArgs(originalArgs, finalTargets []string, extraOptions ...string) ([]string, error) {
	if len(originalArgs) == 0 || isSystemUpgrade(originalArgs) || !strings.HasPrefix(originalArgs[0], "-") {
		args := append([]string{"-S"}, extraOptions...)
		args = append(args, "--")
		return append(args, finalTargets...), nil
	}
	parsed, err := parseParuArguments(originalArgs)
	if err != nil {
		return nil, err
	}
	args := append(parsed.options, extraOptions...)
	args = append(args, "--")
	return append(args, finalTargets...), nil
}

func (o orchestrator) reviewPackage(store *stateStore, pkgbase string, reader *bufio.Reader) (reviewedPackage, error) {
	fmt.Fprintf(o.config.stdout, "AURoscope: acquiring AUR recipe %s with Paru...\n", escapeTerminal(pkgbase))
	if err := o.paru.acquire(pkgbase, o.config.cloneDir); err != nil {
		return reviewedPackage{}, err
	}
	dir, err := worktreePath(o.config.cloneDir, pkgbase)
	if err != nil {
		return reviewedPackage{}, err
	}
auditLoop:
	for {
		baseline, err := store.baseline(pkgbase)
		if err != nil {
			return reviewedPackage{}, err
		}
		bundle, err := buildAuditBundle(pkgbase, dir, baseline)
		if err != nil {
			fmt.Fprintf(o.config.stderr, "auroscope: audit preparation failed for %s: %s\n", escapeTerminal(pkgbase), escapeTerminal(err.Error()))
			switch askDecision(reader, o.config, []string{"retry", "skip", "cancel"}) {
			case "retry":
				continue
			case "skip":
				// Only the package name is known. Do not persist a fictitious
				// audit or put this incomplete identity in the approval list.
				return reviewedPackage{Identity: recipeIdentity{Pkgbase: pkgbase}, Decision: decisionSkip}, nil
			default:
				return reviewedPackage{Identity: recipeIdentity{Pkgbase: pkgbase}, Decision: decisionCancel}, nil
			}
		}
		if bundle.Warning != "" {
			fmt.Fprintf(o.config.stdout, "AURoscope: %s\n", escapeTerminal(bundle.Warning))
		}
		report, err := auditWithProvider(bundle, o.config)
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
		printReviewSummary(o.config, pkgbase, report)
		for {
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
				continue auditLoop
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
}

func printReviewSummary(config runConfig, pkgbase string, report auditReport) {
	fmt.Fprintf(config.stdout, "AUR audit: %s\n\n---\nAssessment: %s\n\nRisk: %s\n---\n\n", escapeTerminal(pkgbase), escapeTerminal(report.Summary), escapeTerminal(report.Risk))
}

func printReview(config runConfig, pkgbase string, bundle auditBundle, report auditReport) {
	fmt.Fprintf(config.stdout, "\nAUR audit: %s\ncommit: %s\nmanifest: %s\nrisk: %s\n%s\n", escapeTerminal(pkgbase), bundle.Identity.Commit, bundle.Identity.ManifestDigest, escapeTerminal(report.Risk), escapeTerminal(report.Summary))
	if bundle.Warning != "" {
		fmt.Fprintf(config.stdout, "\n%s\nPrevious successful commit: %s\n", escapeTerminal(bundle.Warning), escapeTerminal(bundle.PreviousCommit))
	}
	for _, finding := range report.Findings {
		location := escapeTerminal(finding.File)
		if finding.Line > 0 {
			location = fmt.Sprintf("%s:%d", location, finding.Line)
		}
		if finding.Range != "" {
			location = fmt.Sprintf("%s %s", location, escapeTerminal(finding.Range))
		}
		fmt.Fprintf(config.stdout, "- %s\n  evidence: %s\n  explanation: %s\n", location, escapeTerminal(finding.Evidence), escapeTerminal(finding.Explanation))
	}
	if report.Uncertainty != "" {
		fmt.Fprintf(config.stdout, "\nUncertainty:\n%s\n", escapeTerminal(report.Uncertainty))
	}
	if len(report.Inspect) > 0 {
		fmt.Fprintln(config.stdout, "\nInspect:")
		for _, path := range report.Inspect {
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
	aliases := map[string]string{}
	for index, value := range allowed {
		allowedSet[value] = true
		aliases[fmt.Sprint(index+1)] = value
		initial := value[:1]
		if previous, exists := aliases[initial]; !exists || previous == value {
			aliases[initial] = value
		} else {
			delete(aliases, initial)
		}
	}
	for {
		items := make([]string, 0, len(allowed))
		for _, value := range allowed {
			label := decisionLabel(value)
			items = append(items, fmt.Sprintf("[%s]%s", value[:1], strings.TrimPrefix(label, value[:1])))
		}
		fmt.Fprintf(config.stdout, "Decision : %s ", strings.Join(items, " | "))
		line, err := readDecisionLine(reader, config)
		// The terminal echoes Enter; add a blank line before the next phase.
		fmt.Fprintln(config.stdout)
		if err != nil {
			return "cancel"
		}
		choice := strings.ToLower(strings.TrimSpace(line))
		if allowedSet[choice] {
			return choice
		}
		if value, ok := aliases[choice]; ok {
			return value
		}
		fmt.Fprintf(config.stderr, "auroscope: unsupported decision %q\n", choice)
	}
}

func decisionLabel(value string) string {
	switch value {
	case "inspect":
		return "inspect full report"
	case "edit":
		return "edit and re-audit"
	default:
		return value
	}
}

func editAndSnapshot(config runConfig, dir string) error {
	if config.editorPath == "" {
		return errors.New("EDITOR is required for edit")
	}
	preExistingUntracked, err := untrackedRecipePaths(dir)
	if err != nil {
		return err
	}
	cmd := exec.Command(config.editorPath, dir)
	cmd.Dir = dir
	cmd.Stdin = config.stdin
	cmd.Stdout = config.stdout
	cmd.Stderr = config.stderr
	if result := execute(config, cmd); result.status != 0 {
		return childExit{status: reportCommandError(config.stderr, result, "editor")}
	}
	return snapshotEditedWorktree(dir, preExistingUntracked)
}

func writeTransactionFiles(approved []recipeIdentity, cloneDir string) (string, string, error) {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if filepath.Base(filepath.Dir(filepath.Dir(cloneDir))) == operationNamespace {
		runtimeDir = filepath.Dir(cloneDir)
	}
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
	for label, value := range map[string]string{"clone directory": cloneDir, "executable": exe, "transaction path": txPath} {
		if strings.ContainsAny(value, "\r\n") {
			return "", "", fmt.Errorf("%s contains a newline", label)
		}
	}
	var conf strings.Builder
	if original := effectiveParuConfig(); original != "" {
		if strings.ContainsAny(original, "\r\n") {
			return "", "", fmt.Errorf("Paru config path contains a newline")
		}
		fmt.Fprintf(&conf, "[options]\nInclude = %s\n", original)
	}
	fmt.Fprintf(&conf, "\n[options]\nCloneDir = %s\n", cloneDir)
	fmt.Fprintf(&conf, "\n[bin]\nPreBuildCommand = %s __guard %s\n", posixShellQuote(exe), posixShellQuote(txPath))
	if err := os.WriteFile(confPath, []byte(conf.String()), 0o600); err != nil {
		return "", "", err
	}
	complete = true
	return dir, confPath, nil
}

func effectiveParuConfig() string {
	if path := os.Getenv("PARU_CONF"); path != "" {
		return path
	}
	if root := os.Getenv("XDG_CONFIG_HOME"); root != "" {
		path := filepath.Join(root, "paru", "paru.conf")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		path := filepath.Join(home, ".config", "paru", "paru.conf")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	if _, err := os.Stat("/etc/paru.conf"); err == nil {
		return "/etc/paru.conf"
	}
	return ""
}

func posixShellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
