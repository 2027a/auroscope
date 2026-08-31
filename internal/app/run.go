package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	statusFailure = 1
	statusUsage   = 2
)

type runConfig struct {
	paruPath    string
	codexPath   string
	statePath   string
	cloneDir    string
	editorPath  string
	stdin       io.Reader
	reviewInput io.Reader
	stdout      io.Writer
	stderr      io.Writer
	signals     <-chan os.Signal
}

// Run executes AURoscope with the process terminal and forwards termination
// signals to the active Paru child.
func Run(args []string) int {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer signal.Stop(signals)

	return run(args, runConfig{
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		stderr:  os.Stderr,
		signals: signals,
	})
}

func run(args []string, config runConfig) int {
	config = config.withDefaults()
	if len(args) >= 2 && args[0] == "__guard" {
		if err := runGuard(args[1], ""); err != nil {
			fmt.Fprintf(config.stderr, "auroscope guard: %v\n", err)
			return statusFailure
		}
		return 0
	}

	paru := paruClient{config: config}

	if len(args) == 0 || isSystemUpgrade(args) {
		result := paru.run(nil, []string{"-Syu", "--repo"})
		if status := reportCommandError(config.stderr, result, "official update"); status != 0 {
			return status
		}
		targets, err := paru.pendingAURUpdates()
		if err != nil {
			fmt.Fprintf(config.stderr, "auroscope: %v\n", err)
			return statusFailure
		}
		return runAuditedAUR(args, targets, config)
	}
	if commandMayBuildAUR(args) {
		targets, err := initialTargets(args, paru)
		if err != nil {
			fmt.Fprintf(config.stderr, "auroscope: %v\n", err)
			return statusFailure
		}
		return runAuditedAUR(args, targets, config)
	}
	return reportCommandError(config.stderr, paru.run(nil, args), "Paru")
}

func (config runConfig) withDefaults() runConfig {
	if config.paruPath == "" {
		config.paruPath = os.Getenv("AUROSCOPE_PARU")
		if config.paruPath == "" {
			config.paruPath = "paru"
		}
	}
	if config.codexPath == "" {
		config.codexPath = os.Getenv("AUROSCOPE_CODEX")
		if config.codexPath == "" {
			config.codexPath = "codex"
		}
	}
	if config.editorPath == "" {
		config.editorPath = os.Getenv("EDITOR")
	}
	if config.cloneDir == "" {
		config.cloneDir = os.Getenv("AUROSCOPE_CLONE_DIR")
		if config.cloneDir == "" {
			config.cloneDir = filepath.Join(os.TempDir(), "auroscope-clones")
		}
	}
	if config.statePath == "" {
		config.statePath = os.Getenv("AUROSCOPE_STATE")
		if config.statePath == "" {
			if home, err := os.UserHomeDir(); err == nil && home != "" {
				config.statePath = filepath.Join(home, ".local", "state", "auroscope", "state.sqlite3")
			} else {
				config.statePath = filepath.Join(os.TempDir(), "auroscope-state.sqlite3")
			}
		}
	}
	if config.stdin == nil {
		config.stdin = os.Stdin
	}
	if config.reviewInput == nil {
		config.reviewInput = config.stdin
	}
	if config.stdout == nil {
		config.stdout = os.Stdout
	}
	if config.stderr == nil {
		config.stderr = os.Stderr
	}
	return config
}

func initialTargets(args []string, paru paruClient) ([]string, error) {
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		return paru.selectPackages(args)
	}
	var targets []string
	takeNext := false
	for _, arg := range args {
		if takeNext {
			targets = append(targets, arg)
			takeNext = false
			continue
		}
		if arg == "--" {
			takeNext = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		targets = append(targets, arg)
	}
	return targets, nil
}

func isSystemUpgrade(args []string) bool {
	for _, arg := range args {
		if arg == "--sysupgrade" {
			return true
		}
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") {
			short := strings.TrimPrefix(arg, "-")
			if strings.ContainsRune(short, 'S') && strings.ContainsRune(short, 'u') {
				return true
			}
		}
		if arg == "--sync" && hasArgument(args, "--sysupgrade") {
			return true
		}
	}
	return false
}

func runAuditedAUR(originalArgs, targets []string, config runConfig) int {
	orch := orchestrator{config: config.withDefaults(), paru: paruClient{config: config.withDefaults()}}
	if err := orch.run(originalArgs, targets); err != nil {
		var exit childExit
		if errors.As(err, &exit) {
			return exit.status
		}
		fmt.Fprintf(config.stderr, "auroscope: %v\n", err)
		return statusFailure
	}
	return 0
}

func commandMayBuildAUR(args []string) bool {
	if hasArgument(args, "--repo") {
		return false
	}

	sync := false
	nonBuildingSyncAction := false
	hasOperation := false
	for _, arg := range args {
		switch arg {
		case "--sync":
			sync, hasOperation = true, true
		case "--database", "--files", "--getpkgbuild", "--query", "--remove", "--show", "--deptest", "--upgrade", "--version":
			hasOperation = true
		case "--clean", "--groups", "--info", "--list", "--print", "--print-format", "--search":
			nonBuildingSyncAction = true
		}
		if len(arg) >= 2 && arg[0] == '-' && arg[1] != '-' {
			short := strings.TrimPrefix(arg, "-")
			if strings.ContainsRune(short, 'S') {
				sync, hasOperation = true, true
			}
			if strings.ContainsAny(short, "DFGQRTUVP") {
				hasOperation = true
			}
			if strings.ContainsAny(short, "cgilspp") {
				nonBuildingSyncAction = true
			}
		}
	}
	if sync {
		return !nonBuildingSyncAction
	}
	if hasOperation {
		return false
	}
	for _, arg := range args {
		if arg != "" && arg[0] != '-' {
			return true
		}
	}
	return false
}

func hasArgument(args []string, target string) bool {
	for _, arg := range args {
		if arg == target {
			return true
		}
	}
	return false
}

func reportCommandError(stderr io.Writer, result commandResult, operation string) int {
	if result.err != nil && result.status < 0 {
		fmt.Fprintf(stderr, "auroscope: %s: %v\n", operation, result.err)
		return statusFailure
	}
	return result.status
}

type commandResult struct {
	status int
	err    error
}

func execute(config runConfig, cmd *exec.Cmd) commandResult {
	if err := cmd.Start(); err != nil {
		return commandResult{status: -1, err: err}
	}

	done := make(chan struct{})
	if config.signals != nil {
		go func() {
			for {
				select {
				case sig, ok := <-config.signals:
					if !ok {
						return
					}
					if sig != nil {
						_ = cmd.Process.Signal(sig)
					}
				case <-done:
					return
				}
			}
		}()
	}

	err := cmd.Wait()
	close(done)
	return commandResult{status: commandStatus(err), err: err}
}

func commandStatus(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return -1
	}
	if waitStatus, ok := exitErr.ProcessState.Sys().(syscall.WaitStatus); ok && waitStatus.Signaled() {
		return 128 + int(waitStatus.Signal())
	}
	return exitErr.ExitCode()
}
