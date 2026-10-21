package app

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unicode"

	"github.com/creack/pty"
)

type paruClient struct {
	config runConfig
}

type orderResult struct {
	Status  int
	Records []orderRecord
}

type orderRecord struct {
	Kind   string
	Fields []string
}

type resolvedPlan struct {
	RepoTargets         []string
	AURPkgbases         []string
	AURTargetsByPkgbase map[string][]string
}

func (paru paruClient) run(stdout io.Writer, args []string) commandResult {
	return paru.runInDir(stdout, "", args)
}

func (paru paruClient) runInDir(stdout io.Writer, dir string, args []string) commandResult {
	return paru.runInDirWithStdin(stdout, dir, args, paru.config.withDefaults().stdin)
}

func (paru paruClient) runMachine(stdout io.Writer, dir string, args []string) commandResult {
	return paru.runInDirWithStdin(stdout, dir, args, nil)
}

func (paru paruClient) runInDirWithStdin(stdout io.Writer, dir string, args []string, stdin io.Reader) commandResult {
	return paru.runInDirWithStdinEnv(stdout, dir, args, stdin, nil)
}

func (paru paruClient) runInDirWithStdinEnv(stdout io.Writer, dir string, args []string, stdin io.Reader, env []string) commandResult {
	config := paru.config.withDefaults()
	if stdout == nil {
		stdout = config.stdout
	}
	cmd := exec.Command(config.paruPath, args...)
	cmd.Dir = dir
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = config.stderr
	if config.operation != nil {
		env = append(env, "AUROSCOPE_CLONE_DIR="+config.cloneDir)
	}
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	return execute(config, cmd)
}

// selectPackages uses the pinned Paru native interactive-search contract. Its
// menu remains on stderr while stdout contains selected targets, and status 1
// denotes successful completion of this special selection path.
func (paru paruClient) selectPackages(terms []string) ([]string, error) {
	var selected bytes.Buffer
	args := append([]string{"-Ssq", "--interactive"}, terms...)
	result := paru.runSelection(&selected, args)
	if result.status < 0 && result.err != nil {
		return nil, fmt.Errorf("run Paru selection: %w", result.err)
	}
	if result.status != 1 {
		return nil, incompatibleParu("interactive selection returned status %d, want 1", result.status)
	}

	var packages []string
	scanner := bufio.NewScanner(&selected)
	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" || strings.TrimSpace(line) != line || !validSelectedTarget(line) {
			return nil, incompatibleParu("interactive selection emitted non-target stdout %q", line)
		}
		packages = append(packages, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read Paru selection: %w", err)
	}
	return packages, nil
}

// Paru's auto colors require both output descriptors to be terminals before it
// redirects the interactive menu to stderr. Preserve that detection only when
// the caller's outputs really are terminals. Do not force --color: Paru still
// owns the user's pacman.conf Color setting. Only machine targets traverse this
// private PTY; the menu and stdin stay on the user's original terminal.
func (paru paruClient) runSelection(selected io.Writer, args []string) commandResult {
	config := paru.config.withDefaults()
	stdout, stdoutOK := config.stdout.(*os.File)
	stderr, stderrOK := config.stderr.(*os.File)
	if !stdoutOK || !stderrOK {
		return paru.run(selected, args)
	}
	size, err := pty.GetsizeFull(stdout)
	if err != nil {
		return paru.run(selected, args)
	}
	if _, err := pty.GetsizeFull(stderr); err != nil {
		return paru.run(selected, args)
	}
	master, slave, err := pty.Open()
	if err != nil {
		return commandResult{status: -1, err: err}
	}
	defer master.Close()
	defer slave.Close()
	if err := pty.Setsize(slave, size); err != nil {
		return commandResult{status: -1, err: err}
	}
	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(selected, master)
		done <- err
	}()
	result := paru.run(slave, args)
	// Closing our slave after the child is reaped terminates the reader, including
	// when exec fails. Linux signals PTY EOF with EIO rather than io.EOF.
	_ = slave.Close()
	if err := <-done; err != nil && !errors.Is(err, syscall.EIO) {
		return commandResult{status: -1, err: err}
	}
	return result
}

func validSelectedTarget(target string) bool {
	for _, r := range target {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("@._+:/=-", r) {
			continue
		}
		return false
	}
	return target != ""
}

// order consumes only the records emitted by the pinned issue-21 `-P
// --order` implementation. Unknown or malformed records are compatibility
// failures rather than input for a fallback human-output parser.
func (paru paruClient) order(targets []string) (orderResult, error) {
	var output bytes.Buffer
	args := append([]string{"-P", "--order", "--"}, targets...)
	command := paru.runMachine(&output, "", args)
	if command.status != 0 && command.status != 1 {
		return orderResult{}, incompatibleParu("order returned status %d", command.status)
	}

	result := orderResult{Status: command.status}
	hasMissing := false
	scanner := bufio.NewScanner(&output)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			return orderResult{}, incompatibleParu("order emitted an empty record")
		}
		if err := validateOrderRecord(fields); err != nil {
			return orderResult{}, err
		}
		if fields[0] == "MISSING" {
			hasMissing = true
		}
		result.Records = append(result.Records, orderRecord{
			Kind:   fields[0],
			Fields: append([]string(nil), fields[1:]...),
		})
	}
	if err := scanner.Err(); err != nil {
		return orderResult{}, fmt.Errorf("read Paru order output: %w", err)
	}
	if (command.status == 1) != hasMissing {
		return orderResult{}, incompatibleParu("order status %d does not match MISSING records", command.status)
	}
	return result, nil
}

func (paru paruClient) pendingAURUpdates() ([]string, error) {
	var output bytes.Buffer
	command := paru.runMachine(&output, "", []string{"-Qua", "--quiet"})
	// Pinned Paru query.rs returns 1 when no upgrades were printed, including
	// installed packages absent from AUR. The native guarded update below must
	// still confirm this result: runtime/network errors can also return 1.
	if command.status == 1 && output.Len() == 0 {
		return nil, nil
	}
	if command.status != 0 {
		if command.err != nil && command.status < 0 {
			return nil, command.err
		}
		return nil, childExit{status: command.status}
	}
	var targets []string
	scanner := bufio.NewScanner(&output)
	for scanner.Scan() {
		target := strings.TrimSpace(scanner.Text())
		if target == "" {
			continue
		}
		if !validSelectedTarget(target) || strings.Contains(target, " ") {
			return nil, incompatibleParu("AUR update output emitted non-target stdout %q", target)
		}
		targets = append(targets, target)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read Paru AUR update output: %w", err)
	}
	return targets, nil
}

// A quiet query cannot present Paru's missing/out-of-date warnings and its
// native "nothing to do" result. Let Paru do that with native terminal streams,
// but approve no recipe: a newly discovered update must not bypass the audit.
func (paru paruClient) finishEmptyAURUpdate() int {
	config := paru.config.withDefaults()
	op, err := beginTemporaryOperation(config.cloneDir)
	if err != nil {
		fmt.Fprintf(config.stderr, "auroscope: prepare clone directory: %v\n", err)
		return statusFailure
	}
	defer func() {
		if err := op.close(); err != nil {
			fmt.Fprintf(config.stderr, "auroscope: temporary cleanup: %v\n", err)
		}
	}()
	config.operation = op
	config.cloneDir = filepath.Join(op.dir, "clones")
	paru.config = config
	txDir, confPath, err := writeTransactionFiles(nil, config.cloneDir)
	if err != nil {
		fmt.Fprintf(config.stderr, "auroscope: prepare empty AUR transaction: %v\n", err)
		return statusFailure
	}
	defer os.RemoveAll(txDir)
	// Reset configured PKGBUILD-only mode. Never refresh or upgrade official
	// repositories a second time, or enter local recipe repositories here.
	result := paru.finalInstall([]string{"-Su", "--mode=aur", "--skipreview"}, []string{"PARU_CONF=" + confPath})
	return reportCommandError(config.stderr, result, "AUR update")
}

func (result orderResult) plan(targets ...string) (resolvedPlan, error) {
	if len(result.Records) == 0 {
		return resolvedPlan{}, incompatibleParu("order emitted no records")
	}
	// Paru keeps the role from the first dependency traversal. A requested
	// package may therefore appear only as DEP or MAKE, not TARGET. Match exact
	// emitted names; never infer a provider or accept an absent package.
	requested := map[string]bool{}
	for _, target := range targets {
		requested[unqualifiedTarget(target)] = true
	}
	classified := map[string]bool{}
	seenRepo := map[string]bool{}
	seenAUR := map[string]bool{}
	seenAURTarget := map[string]map[string]bool{}
	plan := resolvedPlan{AURTargetsByPkgbase: map[string][]string{}}
	for _, record := range result.Records {
		switch record.Kind {
		case "REPO":
			name := record.Fields[2]
			classified[name] = true
			if (record.Fields[0] == "TARGET" || requested[name]) && !seenRepo[name] {
				seenRepo[name] = true
				plan.RepoTargets = append(plan.RepoTargets, name)
			}
		case "AUR":
			pkgbase := record.Fields[1]
			if !seenAUR[pkgbase] {
				seenAUR[pkgbase] = true
				plan.AURPkgbases = append(plan.AURPkgbases, pkgbase)
			}
			for _, name := range record.Fields[2:] {
				classified[name] = true
				if record.Fields[0] != "TARGET" && !requested[name] {
					continue
				}
				if seenAURTarget[pkgbase] == nil {
					seenAURTarget[pkgbase] = map[string]bool{}
				}
				if !seenAURTarget[pkgbase][name] {
					seenAURTarget[pkgbase][name] = true
					plan.AURTargetsByPkgbase[pkgbase] = append(plan.AURTargetsByPkgbase[pkgbase], name)
				}
			}
		case "MISSING", "CONFLICT":
			return resolvedPlan{}, fmt.Errorf("Paru resolution reported %s: %s", record.Kind, strings.Join(record.Fields, " "))
		case "SRCINFO":
			return resolvedPlan{}, incompatibleParu("order emitted unsupported SRCINFO record")
		}
	}
	for _, target := range targets {
		if !classified[unqualifiedTarget(target)] {
			return resolvedPlan{}, incompatibleParu("requested target %q was not classified", target)
		}
	}
	return plan, nil
}

func unqualifiedTarget(target string) string {
	if slash := strings.IndexByte(target, '/'); slash >= 0 {
		return target[slash+1:]
	}
	return target
}

func validateOrderRecord(fields []string) error {
	switch fields[0] {
	case "REPO":
		if len(fields) != 4 || !validPackageRole(fields[1]) {
			return incompatibleParu("malformed %s order record %q", fields[0], strings.Join(fields, " "))
		}
	case "AUR":
		if len(fields) < 4 || !validPackageRole(fields[1]) {
			return incompatibleParu("malformed AUR order record %q", strings.Join(fields, " "))
		}
	case "SRCINFO":
		if len(fields) != 6 || !validPackageRole(fields[1]) {
			return incompatibleParu("malformed SRCINFO order record %q", strings.Join(fields, " "))
		}
	case "CONFLICT":
		if (len(fields) != 4 && len(fields) != 5) || (fields[1] != "LOCAL" && fields[1] != "INNER") {
			return incompatibleParu("malformed CONFLICT order record %q", strings.Join(fields, " "))
		}
	case "MISSING":
		if len(fields) < 2 {
			return incompatibleParu("malformed MISSING order record %q", strings.Join(fields, " "))
		}
	default:
		return incompatibleParu("unknown order record %q", fields[0])
	}
	return nil
}

func validPackageRole(role string) bool {
	return role == "TARGET" || role == "MAKE" || role == "DEP"
}

// acquire asks Paru to obtain an AUR package base in cloneDir. The command's
// native streams and status are preserved, and AURoscope does not inspect by
// executing package-supplied content.
func (paru paruClient) acquire(pkgbase, cloneDir string) error {
	result := paru.runMachine(nil, cloneDir, []string{"-G", pkgbase})
	if result.status != 0 {
		if result.err != nil && result.status < 0 {
			return fmt.Errorf("run Paru -G: %w", result.err)
		}
		return fmt.Errorf("Paru -G exited with status %d", result.status)
	}
	return nil
}

func (paru paruClient) finalInstall(args []string, env []string) commandResult {
	return paru.runInDirWithStdinEnv(nil, "", args, paru.config.withDefaults().stdin, env)
}

func worktreePath(cloneDir, pkgbase string) (string, error) {
	if !validSelectedTarget(pkgbase) || strings.Contains(pkgbase, "/") {
		return "", fmt.Errorf("invalid pkgbase %q", pkgbase)
	}
	return filepath.Join(cloneDir, pkgbase), nil
}

func incompatibleParu(format string, args ...any) error {
	return fmt.Errorf("incompatible Paru issue-21 contract: "+format, args...)
}
