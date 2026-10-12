package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestTemporaryOperationOwnershipAndRecovery(t *testing.T) {
	root := filepath.Join(t.TempDir(), "clones")
	first, err := beginTemporaryOperation(root)
	if err != nil {
		t.Fatal(err)
	}
	defer first.close()
	legacy := filepath.Join(root, "legacy-source.tar.gz")
	if err := os.WriteFile(legacy, []byte("user data"), 0o600); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	keep := filepath.Join(outside, "keep")
	if err := os.WriteFile(keep, []byte("user data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(first.dir, "outside")); err != nil {
		t.Fatal(err)
	}
	unreadable := filepath.Join(first.dir, "clones", "pkg", "nested")
	if err := os.MkdirAll(unreadable, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(unreadable, "archive.zip"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(unreadable), 0); err != nil {
		t.Fatal(err)
	}
	second, err := beginTemporaryOperation(root)
	if err != nil {
		t.Fatal(err)
	}
	if first.dir == second.dir {
		t.Fatal("shared worktree")
	}
	if _, err := os.Stat(first.dir); err != nil {
		t.Fatal("collected active operation", err)
	}
	if err := second.close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first.dir); err != nil {
		t.Fatal("second operation removed first", err)
	}
	// Simulate abrupt parent loss: only release its lease, not its data.
	first.lock.Close()
	first.lock = nil
	third, err := beginTemporaryOperation(root)
	if err != nil {
		t.Fatal(err)
	}
	defer third.close()
	if _, err := os.Stat(first.dir); !os.IsNotExist(err) {
		t.Fatal("abandoned operation not collected", err)
	}
	for _, path := range []string{legacy, keep} {
		if readString(t, path) != "user data" {
			t.Fatal("unrelated content changed", path)
		}
	}
}

func TestTemporaryOperationRejectsSubstitutedRoots(t *testing.T) {
	for _, namespace := range []bool{false, true} {
		dir := t.TempDir()
		root := filepath.Join(dir, "clones")
		target := t.TempDir()
		path := root
		if namespace {
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatal(err)
			}
			path = filepath.Join(root, operationNamespace)
		}
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		if _, err := beginTemporaryOperation(root); err == nil {
			t.Fatal("accepted symlink root")
		}
		entries, err := os.ReadDir(target)
		if err != nil || len(entries) != 0 {
			t.Fatal("modified symlink target", err)
		}
	}
}

func TestTemporaryOperationWaitsForLeaseAndDescriptorlessGroup(t *testing.T) {
	for _, inherit := range []bool{true, false} {
		root := filepath.Join(t.TempDir(), "clones")
		op, err := beginTemporaryOperation(root)
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command("sleep", "60")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if inherit {
			cmd.ExtraFiles = []*os.File{op.lock}
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		if err := op.recordProcessGroup(cmd.Process.Pid); err != nil {
			t.Fatal(err)
		}
		op.lock.Close()
		op.lock = nil
		func() {
			defer func() { cmd.Process.Kill(); cmd.Wait() }()
			next, err := beginTemporaryOperation(root)
			if err != nil {
				t.Fatal(err)
			}
			defer next.close()
			if _, err := os.Stat(op.dir); err != nil {
				t.Fatal("collected live child after parent loss", err)
			}
		}()
		next, err := beginTemporaryOperation(root)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(op.dir); !os.IsNotExist(err) {
			t.Fatal("dead child still prevents cleanup", err)
		}
		if err := next.close(); err != nil {
			t.Fatal(err)
		}
	}
}

func assertNoTemporaryOperations(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, operationNamespace))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "op-") {
			t.Fatalf("temporary operation remains: %s", entry.Name())
		}
	}
}

func TestTemporaryOperationSignalAtReview(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT} {
		t.Run(sig.String(), func(t *testing.T) {
			dir := t.TempDir()
			repo := createRecipeRepo(t, dir, "hello", "pkgname=hello\n")
			paru := fakeParu(t, dir, filepath.Join(dir, "calls"), repo, "AUR TARGET hello hello\n")
			codex := fakeCodex(t, dir, filepath.Join(dir, "bundle.json"), `{"summary":"reviewed","risk":"low","findings":[],"uncertainty":"","inspect":[]}`)
			signals := make(chan os.Signal, 1)
			read, write, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer read.Close()
			defer write.Close()
			root := filepath.Join(dir, "clones")
			// The writer signals when the prompt is reached, without shared buffers.
			prompt := make(chan struct{}, 1)
			out := &reviewPromptWriter{prompt: prompt}
			status := make(chan int, 1)
			go func() {
				status <- run([]string{"-S", "hello"}, runConfig{paruPath: paru, codexPath: codex, cloneDir: root, statePath: filepath.Join(dir, "state"), stdin: read, stdout: out, stderr: out, signals: signals})
			}()
			select {
			case <-prompt:
			case <-time.After(5 * time.Second):
				t.Fatal("review not reached")
			}
			signals <- sig
			select {
			case got := <-status:
				if got != 128+int(sig) {
					t.Fatalf("status %d", got)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("signal did not end review")
			}
			assertNoTemporaryOperations(t, root)
		})
	}
}

func TestOperationCloneOptionsPreserveOpaqueOperands(t *testing.T) {
	args := []string{"-S", "--config", "--clonedir", "--clonedir", "first", "--builddir=second", "-b", "--builddir", "--ignore=--clonedir", "--", "hello"}
	parsed, err := parseParuArguments(args)
	if err != nil {
		t.Fatal(err)
	}
	root, _ := operationCloneOptions(parsed.options, "default", false)
	if root != "second" {
		t.Fatalf("wrong root: %s", root)
	}
	_, replaced := operationCloneOptions(args, "/private/operation/clones", true)
	want := "-S --config --clonedir --clonedir /private/operation/clones --builddir=/private/operation/clones -b --builddir --ignore=--clonedir -- hello"
	if strings.Join(replaced, " ") != want {
		t.Fatalf("changed opaque operands: %v", replaced)
	}
}

type reviewPromptWriter struct{ prompt chan struct{} }

func (w *reviewPromptWriter) Write(data []byte) (int, error) {
	if strings.Contains(string(data), "Decision :") {
		select {
		case w.prompt <- struct{}{}:
		default:
		}
	}
	return len(data), nil
}
