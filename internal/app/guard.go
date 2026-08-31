package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type transactionApproval struct {
	recipeIdentity
	Worktree string `json:"worktree"`
}

func runGuard(txPath, worktree string) error {
	if worktree == "" {
		var err error
		worktree, err = os.Getwd()
		if err != nil {
			return err
		}
	}
	pkgbase := os.Getenv("PKGBASE")
	if pkgbase == "" {
		return fmt.Errorf("PKGBASE is not set")
	}
	data, err := os.ReadFile(txPath)
	if err != nil {
		return fmt.Errorf("read transaction file: %w", err)
	}
	var approved []transactionApproval
	if err := json.Unmarshal(data, &approved); err != nil {
		return fmt.Errorf("parse transaction file: %w", err)
	}
	var expected *transactionApproval
	for i := range approved {
		if approved[i].Pkgbase == pkgbase {
			expected = &approved[i]
			break
		}
	}
	if expected == nil {
		return fmt.Errorf("unexpected pkgbase %q", pkgbase)
	}
	actualWorktree, err := canonicalWorktree(worktree)
	if err != nil {
		return fmt.Errorf("resolve guard worktree: %w", err)
	}
	if actualWorktree != expected.Worktree {
		return fmt.Errorf("unexpected worktree for %s: got %q want %q", pkgbase, actualWorktree, expected.Worktree)
	}
	actual, _, err := readRecipeIdentity(pkgbase, actualWorktree)
	if err != nil {
		return err
	}
	if actual.Commit != expected.Commit || actual.ManifestDigest != expected.ManifestDigest {
		return fmt.Errorf("recipe identity drift for %s: got %s/%s want %s/%s", pkgbase, actual.Commit, actual.ManifestDigest, expected.Commit, expected.ManifestDigest)
	}
	return nil
}

func canonicalWorktree(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(abs)
}
