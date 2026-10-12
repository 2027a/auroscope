package app

import (
	"bufio"
	"io"
	"os"
	"syscall"
)

// Preserve interruption even if a provider consumes its forwarded signal.
// The review prompt must not hang waiting for input after Ctrl-C or SIGTERM.
type operationInterruption struct {
	done   chan struct{}
	status int
}

func watchOperationSignals(config *runConfig) func() {
	if config.signals == nil {
		return func() {}
	}
	incoming := config.signals
	forward := make(chan os.Signal, 1)
	interruption := &operationInterruption{done: make(chan struct{})}
	config.interruption = interruption
	config.signals = forward
	stop := make(chan struct{})
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		select {
		case sig, ok := <-incoming:
			if !ok {
				return
			}
			interruption.status = 128 + int(syscall.SIGTERM)
			if number, ok := sig.(syscall.Signal); ok {
				interruption.status = 128 + int(number)
			}
			forward <- sig
			close(interruption.done)
		case <-stop:
		}
	}()
	return func() { close(stop); <-stopped }
}

func readDecisionLine(reader *bufio.Reader, config runConfig) (string, error) {
	if config.interruption == nil {
		return reader.ReadString('\n')
	}
	select {
	case <-config.interruption.done:
		return "", io.EOF
	default:
	}
	type result struct {
		line string
		err  error
	}
	read := make(chan result, 1)
	go func() {
		line, err := reader.ReadString('\n')
		read <- result{line, err}
	}()
	select {
	case <-config.interruption.done:
		return "", io.EOF
	case result := <-read:
		return result.line, result.err
	}
}
