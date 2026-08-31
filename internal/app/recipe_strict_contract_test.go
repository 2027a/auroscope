package app

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDifferentialReportAcceptsUnchangedSuppliedSRCINFOFinding(t *testing.T) {
	dir := t.TempDir()
	repo := createRecipeRepo(t, dir, "srcinfo", "pkgname=srcinfo\n")
	baselineIdentity, _, err := readRecipeIdentity("srcinfo", repo)
	if err != nil {
		t.Fatal(err)
	}
	appendRecipeCommit(t, repo, "pkgrel=2\n")
	bundle, err := buildAuditBundle("srcinfo", repo, &packageBaseline{
		Commit:         baselineIdentity.Commit,
		ManifestDigest: baselineIdentity.ManifestDigest,
	})
	if err != nil {
		t.Fatal(err)
	}
	report := []byte(`{"summary":"checked metadata","risk":"low","findings":[{"file":".SRCINFO","line_start":1,"line_end":1,"evidence":"pkgbase","explanation":"metadata identity"}],"uncertainties":[],"suggested_inspection":[]}`)
	if _, err := validateAuditReportForBundle(report, bundle); err != nil {
		t.Fatalf("unchanged supplied .SRCINFO finding rejected: %v", err)
	}
}

func TestRecipeIdentityRejectsIgnoredUntrackedPayload(t *testing.T) {
	dir := t.TempDir()
	repo := createRecipeRepo(t, dir, "ignored", "pkgname=ignored\n")
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte("hidden-payload\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", ".gitignore")
	gitRun(t, repo, "commit", "-m", "track ignore rule")
	if err := os.WriteFile(filepath.Join(repo, "hidden-payload"), []byte("hostile bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readRecipeIdentity("ignored", repo); err == nil || !strings.Contains(err.Error(), "untracked") {
		t.Fatalf("ignored untracked payload error = %v", err)
	}
}

func TestSnapshotEditedWorktreeDisablesReferenceTransactionHook(t *testing.T) {
	dir := t.TempDir()
	repo := createRecipeRepo(t, dir, "hooks", "pkgname=hooks\n")
	marker := filepath.Join(dir, "REFERENCE_TRANSACTION_RAN")
	hook := filepath.Join(repo, ".git", "hooks", "reference-transaction")
	script := "#!/bin/sh\n: > " + strconvQuote(marker) + "\ncat >/dev/null\n"
	if err := os.WriteFile(hook, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "PKGBUILD"), []byte("pkgname=hooks\npkgrel=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := snapshotEditedWorktree(repo); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("reference-transaction hook executed or stat failed: %v", err)
	}
	if got := gitTrim(t, repo, "show", "HEAD:PKGBUILD"); got != "pkgname=hooks\npkgrel=2" {
		t.Fatalf("snapshotted bytes = %q", got)
	}
}

func TestRecipeReadersRejectOversizedFiles(t *testing.T) {
	t.Run("current worktree", func(t *testing.T) {
		dir := t.TempDir()
		repo := createRecipeRepo(t, dir, "large-current", "pkgname=large-current\n")
		oversized := make([]byte, maxRecipeFileBytes+1)
		if err := os.WriteFile(filepath.Join(repo, "PKGBUILD"), oversized, 0o644); err != nil {
			t.Fatal(err)
		}
		_, _, err := readRecipeIdentity("large-current", repo)
		if err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("oversized current file error = %v", err)
		}
	})

	t.Run("edited snapshot", func(t *testing.T) {
		dir := t.TempDir()
		repo := createRecipeRepo(t, dir, "large-edit", "pkgname=large-edit\n")
		oversized := make([]byte, maxRecipeFileBytes+1)
		if err := os.WriteFile(filepath.Join(repo, "new.patch"), oversized, 0o644); err != nil {
			t.Fatal(err)
		}
		err := snapshotEditedWorktree(repo)
		if err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("oversized snapshot file error = %v", err)
		}
	})

	t.Run("baseline blob", func(t *testing.T) {
		dir := t.TempDir()
		repo := createRecipeRepo(t, dir, "large-baseline", "pkgname=large-baseline\n")
		oversized := make([]byte, maxRecipeFileBytes+1)
		if err := os.WriteFile(filepath.Join(repo, "old.bin"), oversized, 0o644); err != nil {
			t.Fatal(err)
		}
		gitRun(t, repo, "add", "old.bin")
		gitRun(t, repo, "commit", "-m", "oversized baseline")
		commit := gitTrim(t, repo, "rev-parse", "HEAD")
		_, err := readCommitRecipeFiles(repo, commit)
		if err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("oversized baseline blob error = %v", err)
		}
	})
}

func TestBoundedRegularReaderUsesOpenedDescriptorAndNeverFollowsSymlinks(t *testing.T) {
	dir := t.TempDir()
	regular := filepath.Join(dir, "regular")
	payload := []byte("exact bytes\n")
	if err := os.WriteFile(regular, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readRegularFileBounded(regular, int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(payload) {
		t.Fatalf("bounded read = %q", got)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(regular, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readRegularFileBounded(link, int64(len(payload))); err == nil {
		t.Fatal("bounded regular reader followed a symlink")
	}
	if _, err := readRegularFileBounded(regular, int64(len(payload)+1)); err == nil {
		t.Fatal("bounded regular reader accepted descriptor size drift")
	}
}

func TestRecipeReadersRejectOversizedAggregate(t *testing.T) {
	dir := t.TempDir()
	repo := createRecipeRepo(t, dir, "aggregate", "pkgname=aggregate\n")
	chunk := make([]byte, maxRecipeFileBytes)
	for i := 0; i < maxRecipeTotalBytes/maxRecipeFileBytes; i++ {
		name := fmt.Sprintf("payload-%d.bin", i)
		if err := os.WriteFile(filepath.Join(repo, name), chunk, 0o644); err != nil {
			t.Fatal(err)
		}
		gitRun(t, repo, "add", name)
	}
	gitRun(t, repo, "commit", "-m", "at aggregate limit")
	if err := os.WriteFile(filepath.Join(repo, "overflow"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "overflow")
	gitRun(t, repo, "commit", "-m", "over aggregate limit")

	_, _, err := readRecipeIdentity("aggregate", repo)
	if err == nil || !strings.Contains(err.Error(), "aggregate") {
		t.Fatalf("aggregate error = %v", err)
	}
}

func TestDifferentialBundleOmitsUnchangedContent(t *testing.T) {
	dir := t.TempDir()
	repo := createRecipeRepo(t, dir, "delta-shape", "pkgname=delta-shape\npkgrel=1\n")
	sentinel := []byte("UNCHANGED-SENTINEL-0c52fd57c86b4d719f98\n")
	if err := os.WriteFile(filepath.Join(repo, "unchanged.patch"), sentinel, 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "unchanged.patch")
	gitRun(t, repo, "commit", "-m", "baseline sentinel")
	baseline, _, err := readRecipeIdentity("delta-shape", repo)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(repo, "PKGBUILD"), []byte("pkgname=delta-shape\npkgrel=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "PKGBUILD")
	gitRun(t, repo, "commit", "-m", "change pkgrel")
	bundle, err := buildAuditBundle("delta-shape", repo, &packageBaseline{Commit: baseline.Commit, ManifestDigest: baseline.ManifestDigest})
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Mode != "diff" {
		t.Fatalf("bundle mode = %q", bundle.Mode)
	}
	if len(bundle.Files) != 1 || bundle.Files[0].Path != "PKGBUILD" {
		t.Fatalf("differential files = %#v", bundle.Files)
	}
	manifestFile := recipeFileByPath(t, bundle.Manifest, "unchanged.patch")
	if manifestFile.SHA256 == "" || manifestFile.Size != int64(len(sentinel)) {
		t.Fatalf("manifest metadata = %#v", manifestFile)
	}
	serialized, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(serialized), string(sentinel)) || strings.Contains(string(serialized), base64.StdEncoding.EncodeToString(sentinel)) {
		t.Fatalf("unchanged content leaked into differential bundle: %s", serialized)
	}

	full, err := buildAuditBundle("delta-shape", repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := decodeRecipeContent(t, recipeFileByPath(t, full.Files, "unchanged.patch")); got != string(sentinel) {
		t.Fatalf("full bundle sentinel = %q", got)
	}
}

func TestGitDiffOutputIsBoundedWhileCommandRuns(t *testing.T) {
	dir := t.TempDir()
	repo := createRecipeRepo(t, dir, "large-diff", "pkgname=large-diff\n")
	before := deterministicBytes(450 * 1024)
	if err := os.WriteFile(filepath.Join(repo, "payload.bin"), before, 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "payload.bin")
	gitRun(t, repo, "commit", "-m", "binary baseline")
	baseline, _, err := readRecipeIdentity("large-diff", repo)
	if err != nil {
		t.Fatal(err)
	}

	after := deterministicBytesWithSeed(450*1024, 1)
	if err := os.WriteFile(filepath.Join(repo, "payload.bin"), after, 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, repo, "add", "payload.bin")
	gitRun(t, repo, "commit", "-m", "large binary change")
	_, err = buildAuditBundle("large-diff", repo, &packageBaseline{Commit: baseline.Commit, ManifestDigest: baseline.ManifestDigest})
	if err == nil || !strings.Contains(err.Error(), fmt.Sprint(maxRecipeDiffBytes)) {
		t.Fatalf("large diff error = %v", err)
	}
}

func deterministicBytes(size int) []byte {
	return deterministicBytesWithSeed(size, 0)
}

func deterministicBytesWithSeed(size int, seed uint64) []byte {
	out := make([]byte, 0, size)
	var counter [16]byte
	binary.BigEndian.PutUint64(counter[:8], seed)
	for i := uint64(0); len(out) < size; i++ {
		binary.BigEndian.PutUint64(counter[8:], i)
		sum := sha256.Sum256(counter[:])
		remaining := size - len(out)
		if remaining < len(sum) {
			out = append(out, sum[:remaining]...)
		} else {
			out = append(out, sum[:]...)
		}
	}
	return out
}
