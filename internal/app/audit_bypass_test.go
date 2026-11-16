package app

import (
	"fmt"
	"testing"
)

// Pinned Paru: --repo, --aur/-a and --pkgbuilds replace the mode; --mode adds
// to it. Only a final repository-only mode may skip the audited path.
func TestRepoModeReopenedForAURIsAudited(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		audited bool
	}{
		{[]string{"-S", "--repo", "--mode=aur", "foo"}, true},
		{[]string{"-S", "--repo", "--mode", "aur", "foo"}, true},
		{[]string{"-S", "--repo", "--mode=a", "foo"}, true},
		{[]string{"-S", "--repo", "--mode=ra", "foo"}, true},
		{[]string{"-S", "--repo", "--mode=repo,aur", "foo"}, true},
		{[]string{"-S", "--repo", "--mode=all", "foo"}, true},
		{[]string{"-S", "--repo", "--mode=pkgbuilds", "foo"}, true},
		{[]string{"-S", "--repo", "--aur", "foo"}, true},
		{[]string{"-S", "--repo", "-a", "foo"}, true},
		{[]string{"-S", "--repo", "-ay", "foo"}, true},
		{[]string{"-S", "--repo", "--pkgbuilds", "foo"}, true},
		{[]string{"-S", "--mode=repo", "foo"}, true},
		{[]string{"-S", "--repo", "foo"}, false},
		{[]string{"-Sa", "--repo", "foo"}, false},
		{[]string{"-S", "--aur", "--repo", "foo"}, false},
		{[]string{"-S", "--mode=aur", "--repo", "foo"}, false},
		{[]string{"-S", "--repo", "--mode=repo", "foo"}, false},
		{[]string{"-S", "--repo", "--mode=r", "foo"}, false},
		{[]string{"-S", "--repo", "--ignore", "--aur", "foo"}, false},
		{[]string{"-S", "--repo", "--ignore=-a", "foo"}, false},
		{[]string{"-S", "--repo", "-r", "-a", "foo"}, false},
		{[]string{"-S", "--repo", "--", "--aur"}, false},
	} {
		t.Run(fmt.Sprint(tc.args), func(t *testing.T) {
			if got := commandMayBuildAUR(tc.args); got != tc.audited {
				t.Fatalf("commandMayBuildAUR = %v, want %v", got, tc.audited)
			}
		})
	}
}
