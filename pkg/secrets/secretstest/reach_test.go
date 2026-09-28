package secretstest_test

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/astronomer/astro-cli/pkg/secrets"
	"github.com/astronomer/astro-cli/pkg/secrets/secretstest"
)

// The table through the predicate itself: OpenLinks on the case's index, then
// ReachOf and Includes for every key in the listing. A failed open fails the
// global tier closed, so nothing reaches.
func TestReachCasesAgainstIncludes(t *testing.T) {
	l := secretstest.NewLayout(t)
	for _, c := range secretstest.ReachCases() {
		t.Run(c.Name, func(t *testing.T) {
			if why := c.Skip(l); why != "" {
				t.Skip(why)
			}
			vault := filepath.Join(t.TempDir(), "secrets")
			c.WriteIndex(t, vault, l)

			links, err := secrets.OpenLinks(vault)
			if c.WantErr != nil {
				if !errors.Is(err, c.WantErr) {
					t.Fatalf("OpenLinks err = %v, want %v", err, c.WantErr)
				}
			} else if err != nil {
				t.Fatalf("OpenLinks: %v", err)
			}
			got := []string{}
			if err == nil {
				checkout := l.Checkout(c.Place)
				for _, key := range secretstest.ReachVault {
					if links.ReachOf(key).Includes(checkout) {
						got = append(got, key)
					}
				}
			}
			slices.Sort(got)
			if !slices.Equal(got, c.Want) {
				t.Errorf("reaching keys = %v, want %v", got, c.Want)
			}
		})
	}
}

// The table has to cover what it claims to, or a consumer running it proves
// less than it thinks.
func TestReachCasesCoverEveryStateAndPlace(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range secretstest.ReachCases() {
		seen[c.Name] = true
	}
	for _, state := range []string{"no-file", "empty-list", "stale-path", "linked", "newer-version", "corrupt"} {
		for _, p := range []secretstest.Place{
			secretstest.PlaceMain, secretstest.PlaceMainSub, secretstest.PlaceWorktree,
			secretstest.PlaceCustomWorktree, secretstest.PlaceCustomWorktreeSub, secretstest.PlaceSymlink,
			secretstest.PlaceCaseVariant, secretstest.PlaceOther, secretstest.PlaceSandbox, secretstest.PlaceOutside,
		} {
			if name := state + "/" + string(p); !seen[name] {
				t.Errorf("no case %s", name)
			}
		}
	}
}
