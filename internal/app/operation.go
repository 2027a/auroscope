package app

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const operationNamespace = ".auroscope-operations-v1"
const operationMarker = "AURoscope temporary operation v1\n"

// Only this private namespace is collected. Legacy clones, user-selected cache
// paths and directories without our marker are never candidates for removal.
// Locks are inherited by recipe subprocesses, so a killed wrapper cannot cause
// the next invocation to collect a still-running build or editor.
type temporaryOperation struct {
	dir  string
	lock *os.File
}

func privateOwnedDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || stat.Uid != uint32(os.Getuid()) {
		return fmt.Errorf("not an owned directory: %s", path)
	}
	return os.Chmod(path, 0o700)
}

func openOperationLock(path string, create bool) (*os.File, error) {
	flags := syscall.O_RDWR | syscall.O_NOFOLLOW | syscall.O_CLOEXEC | syscall.O_NONBLOCK
	if create {
		flags |= syscall.O_CREAT
	}
	fd, err := syscall.Open(path, flags, 0o600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || !ok || stat.Uid != uint32(os.Getuid()) || stat.Nlink != 1 {
		file.Close()
		return nil, fmt.Errorf("not a private operation lock: %s", path)
	}
	return file, nil
}

func beginTemporaryOperation(root string) (*temporaryOperation, error) {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, err
	}
	if err := privateOwnedDirectory(root); err != nil {
		return nil, err
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	namespace := filepath.Join(root, operationNamespace)
	if err := os.Mkdir(namespace, 0o700); err != nil && !os.IsExist(err) {
		return nil, err
	}
	if err := privateOwnedDirectory(namespace); err != nil {
		return nil, err
	}
	// Serialize collection and creation only, not audits or builds.
	registry, err := openOperationLock(filepath.Join(namespace, "registry.lock"), true)
	if err != nil {
		return nil, err
	}
	defer registry.Close()
	if err := syscall.Flock(int(registry.Fd()), syscall.LOCK_EX); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(namespace)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "op-") {
			continue
		}
		if _, err := collectOperation(filepath.Join(namespace, entry.Name())); err != nil {
			return nil, fmt.Errorf("collect abandoned operation: %w", err)
		}
	}
	dir, err := os.MkdirTemp(namespace, "op-")
	if err != nil {
		return nil, err
	}
	lock, err := openOperationLock(filepath.Join(dir, "owner.lock"), true)
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	op := &temporaryOperation{dir: dir, lock: lock}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_SH); err != nil {
		lock.Close()
		os.RemoveAll(dir)
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "owner"), []byte(operationMarker), 0o600); err != nil {
		op.close()
		return nil, err
	}
	if err := os.Mkdir(filepath.Join(dir, "clones"), 0o700); err != nil {
		op.close()
		return nil, err
	}
	return op, nil
}

func collectOperation(dir string) (bool, error) {
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || stat.Uid != uint32(os.Getuid()) || info.Mode().Perm() != 0o700 {
		return false, nil
	}
	markerInfo, err := os.Lstat(filepath.Join(dir, "owner"))
	if err != nil || !markerInfo.Mode().IsRegular() || markerInfo.Size() != int64(len(operationMarker)) {
		return false, nil
	}
	marker, err := os.ReadFile(filepath.Join(dir, "owner"))
	if err != nil || string(marker) != operationMarker {
		return false, nil
	}
	lock, err := openOperationLock(filepath.Join(dir, "owner.lock"), false)
	if err != nil {
		return false, nil // Unknown or substituted content is not ours to delete.
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return false, nil
		}
		return false, err
	}
	// RemoveAll does not follow symlinks. Never chmod/traverse unrelated clones.
	busy, err := operationGroupsAlive(dir)
	if err != nil || busy {
		return false, err
	}
	if err := removeOperationTree(dir); err != nil {
		return false, err
	}
	return true, nil
}

// Builds can leave directories with mode 000. Restore access only within this
// already leased operation, without following links or touching legacy content.
func removeOperationTree(dir string) error {
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if entry != nil && entry.IsDir() {
			if err := os.Chmod(path, 0o700); err != nil {
				return err
			}
			if walkErr != nil {
				return removeOperationTree(path)
			}
			return nil
		}
		return walkErr
	})
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}

func (config runConfig) temporaryDirectory(pattern string) (string, error) {
	root := ""
	if config.operation != nil {
		root = config.operation.dir
	}
	return os.MkdirTemp(root, pattern)
}

func (op *temporaryOperation) recordProcessGroup(pid int) error {
	file, err := os.OpenFile(filepath.Join(op.dir, "groups"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintln(file, pid)
	return err
}

// sudo can close inherited descriptors. A recorded live process group still
// prevents collection after SIGKILL, including during Pacman installation.
// Group reuse is conservative: retain, never kill or delete a stranger's data.
func operationGroupsAlive(dir string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, "groups"))
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	groups := map[string]bool{}
	for _, value := range strings.Fields(string(data)) {
		pid, err := strconv.Atoi(value)
		if err != nil || pid <= 0 {
			return true, fmt.Errorf("invalid temporary operation process group")
		}
		groups[value] = true
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return true, err
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return true, err
		}
		// comm may contain spaces and parentheses; fields start after its last ).
		end := strings.LastIndexByte(string(data), ')')
		if end < 0 {
			return true, fmt.Errorf("cannot read process group")
		}
		fields := strings.Fields(string(data)[end+1:])
		if len(fields) < 3 {
			return true, fmt.Errorf("cannot read process group")
		}
		if fields[0] != "Z" && fields[0] != "X" && groups[fields[2]] {
			return true, nil
		}
	}
	return false, nil
}

func (op *temporaryOperation) close() error {
	// Closing, not LOCK_UN: inherited descriptors must keep their shared lock.
	if op.lock != nil {
		op.lock.Close()
		op.lock = nil
	}
	for i := 0; i < 50; i++ {
		removed, err := collectOperation(op.dir)
		if err != nil || removed {
			return err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("temporary operation still in use; retained for cleanup on next AUR operation")
}

// Recipe Git commands do not take runConfig. Locate only our immediate
// operation layout, and hold a shared lease through exec and its descendants.
func leaseRecipeCommand(cmd *exec.Cmd) (func(), error) {
	dir := cmd.Dir
	for dir != "" && dir != filepath.Dir(dir) {
		if filepath.Base(filepath.Dir(dir)) == operationNamespace && strings.HasPrefix(filepath.Base(dir), "op-") {
			file, err := openOperationLock(filepath.Join(dir, "owner.lock"), false)
			if err != nil {
				return nil, err
			}
			if err := syscall.Flock(int(file.Fd()), syscall.LOCK_SH); err != nil {
				file.Close()
				return nil, err
			}
			cmd.ExtraFiles = append(cmd.ExtraFiles, file)
			return func() { file.Close() }, nil
		}
		dir = filepath.Dir(dir)
	}
	return func() {}, nil
}
