package app

import (
	"encoding/json"
	"fmt"
	"os"
)

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
	var approved []recipeIdentity
	if err := json.Unmarshal(data, &approved); err != nil {
		return fmt.Errorf("parse transaction file: %w", err)
	}
	var expected *recipeIdentity
	for i := range approved {
		if approved[i].Pkgbase == pkgbase {
			expected = &approved[i]
			break
		}
	}
	if expected == nil {
		return fmt.Errorf("unexpected pkgbase %q", pkgbase)
	}
	actual, _, err := readRecipeIdentity(pkgbase, worktree)
	if err != nil {
		return err
	}
	if actual.Commit != expected.Commit || actual.ManifestDigest != expected.ManifestDigest {
		return fmt.Errorf("recipe identity drift for %s: got %s/%s want %s/%s", pkgbase, actual.Commit, actual.ManifestDigest, expected.Commit, expected.ManifestDigest)
	}
	return nil
}
