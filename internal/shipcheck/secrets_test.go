package shipcheck

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/astronomer/astro-cli/pkg/imagebuild"
)

// A warning about the gitignored files an image carries names where they
// are, most files first, so the place to add to an ignore file is what the
// reader sees, not a list of thousands of paths.
func TestPlacesOfNamesTopLevelPlaces(t *testing.T) {
	var paths []string
	for i := range 5509 {
		paths = append(paths, fmt.Sprintf(".claude/worktrees/agent-%d/README.md", i))
	}
	paths = append(paths, ".astro/config.local.lock")
	assert.Equal(t, ".claude/ and .astro/", placesOf(paths))

	assert.Equal(t, "notes.txt", placesOf([]string{"notes.txt"}), "a file at the root is its own place")
	assert.Equal(t, "a/, b/ and c.txt", placesOf([]string{"b/1", "a/1", "a/2", "c.txt", "b/2", "a/3"}),
		"most files first, then by name")
	assert.Equal(t, "a/, b/, c/ and 1 more place", placesOf([]string{"a/1", "a/2", "a/3", "b/1", "b/2", "c/1", "d/1"}))
	assert.Equal(t, "a/, b/, c/ and 2 more places", placesOf([]string{"a/1", "b/1", "c/1", "d/1", "e/1"}))
}

// Every message that lists files agrees with how many it lists: one file is
// "it" and "looks", several are "them" and "look".
func TestMessagesAgreeWithTheirCount(t *testing.T) {
	one := Report{IgnoreFile: ".dockerignore", Gitignored: []string{"build/out.txt"}, Unchecked: []string{"prod.pem"}}
	several := Report{IgnoreFile: ".dockerignore", Gitignored: []string{"build/a", "build/b"}, Unchecked: []string{"a.pem", "b.pem"}}
	assert.Equal(t, []string{
		"the image will carry 1 file that .gitignore ignores and .dockerignore does not, under build/. If it does not belong in the image, add it to .dockerignore",
		"the image will carry this file that looks like credentials, which git could not say is committed: prod.pem. If it does not belong in the image, add it to .dockerignore",
	}, one.Warnings())
	assert.Equal(t, []string{
		"the image will carry 2 files that .gitignore ignores and .dockerignore does not, under build/. If they do not belong in the image, add them to .dockerignore",
		"the image will carry these files that look like credentials, which git could not say are committed: a.pem, b.pem. If they do not belong in the image, add them to .dockerignore",
	}, several.Warnings())

	refusals := map[string]struct {
		r    Report
		want string
	}{
		"one secret": {
			Report{IgnoreFile: ".dockerignore", Secrets: []string{"sandbox.env"}},
			"refusing to build: sandbox.env looks like credentials and is not tracked by git, and the image would carry it to a registry. Add it to .dockerignore, or remove it from the project, and try again",
		},
		"several secrets": {
			Report{IgnoreFile: ".dockerignore", Secrets: []string{"a.env", "b.env"}},
			"refusing to build: a.env, b.env look like credentials and are not tracked by git, and the image would carry them to a registry. Add them to .dockerignore, or remove them from the project, and try again",
		},
		"one through a link": {
			Report{IgnoreFile: ".dockerignore", Outside: []string{"cfg.json → /home/u/.docker/config.json"}},
			"refusing to build: cfg.json → /home/u/.docker/config.json looks like credentials and is reached through a link out of the project, and the image would carry it to a registry. Add it to .dockerignore, or remove it from the project, and try again",
		},
		"one of each is two": {
			Report{IgnoreFile: ".dockerignore", Secrets: []string{"a.env"}, Outside: []string{"k → /k.pem"}},
			"refusing to build: a.env looks like credentials and is not tracked by git; k → /k.pem looks like credentials and is reached through a link out of the project, and the image would carry them to a registry. Add them to .dockerignore, or remove them from the project, and try again",
		},
		"one git config": {
			Report{IgnoreFile: ".dockerignore", VCSSecrets: []string{".git"}},
			"refusing to build: the git config under .git holds a credential (an extraheader, or a URL with a password or a known token in it), and the image would carry it to a registry. Add **/.git to .dockerignore and try again",
		},
		"several git configs": {
			Report{IgnoreFile: ".dockerignore", VCSSecrets: []string{".git", "sub/.git"}},
			"refusing to build: the git configs under .git, sub/.git hold credentials (an extraheader, or a URL with a password or a known token in it), and the image would carry them to a registry. Add **/.git to .dockerignore and try again",
		},
	}
	for name, tc := range refusals {
		assert.EqualError(t, tc.r.Err(), tc.want, name)
	}

	assert.Equal(t,
		"the build context carries a Cosmos Boost file an earlier Astro CLI wrote into the project: include/cosmos.json. The Cosmos Boost plugin would read it as current if the Dockerfile copies it into the image. This CLI no longer writes it; delete it",
		CosmosBoostWarning(&imagebuild.ProjectSurvey{CosmosBoost: []string{"include/cosmos.json"}}))
	assert.Equal(t,
		"the build context carries Cosmos Boost files an earlier Astro CLI wrote into the project: a.json, b.json. The Cosmos Boost plugin would read them as current if the Dockerfile copies them into the image. This CLI no longer writes them; delete them",
		CosmosBoostWarning(&imagebuild.ProjectSurvey{CosmosBoost: []string{"a.json", "b.json"}}))
}

func TestCountFiles(t *testing.T) {
	for n, want := range map[int]string{
		1:       "1 file",
		2:       "2 files",
		999:     "999 files",
		5510:    "5,510 files",
		1234567: "1,234,567 files",
	} {
		assert.Equal(t, want, countFiles(n))
	}
}
