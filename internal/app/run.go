package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
)

const (
	statusFailure = 1
	statusUsage   = 2
)

type runConfig struct {
	paruPath string
	stdin    io.Reader
	stdout   io.Writer
	stderr   io.Writer
	signals  <-chan os.Signal
}

// Run executes AURoscope with the process terminal and forwards termination
// signals to the active Paru child.
func Run(args []string) int {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer signal.Stop(signals)

	return run(args, runConfig{
		paruPath: "paru",
		stdin:    os.Stdin,
		stdout:   os.Stdout,
		stderr:   os.Stderr,
		signals:  signals,
	})
}

func run(args []string, config runConfig) int {
	config = config.withDefaults()
	paru := paruClient{config: config}

	if len(args) == 0 {
		return reportCommandError(config.stderr, paru.run(nil, []string{"-Syu", "--repo"}), "official update")
	}
	if commandMayBuildAUR(args) {
		fmt.Fprintln(config.stderr, "auroscope: AUR build orchestration is not available in this implementation slice")
		return statusUsage
	}
	return reportCommandError(config.stderr, paru.run(nil, args), "Paru")
}

func (config runConfig) withDefaults() runConfig {
	if config.paruPath == "" {
		config.paruPath = "paru"
	}
	if config.stdin == nil {
		config.stdin = os.Stdin
	}
	if config.stdout == nil {
		config.stdout = os.Stdout
	}
	if config.stderr == nil {
		config.stderr = os.Stderr
	}
	return config
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
