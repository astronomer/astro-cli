// Package shipcheck holds an image that is pushed to a registry (a deploy's,
// `astro package astro`'s) to what git says about the files it carries. A
// file the project's .gitignore keeps out of the repository but no
// .dockerignore keeps out of the image is warned about; one of those that
// looks like a credential stops the build.
package shipcheck

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/git"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
)

// Report is what git says about the files an image would carry.
type Report struct {
	// Gitignored are the carried files git ignores, slash-separated and
	// relative to the project.
	Gitignored []string
	// Secrets are those that look like credentials (LooksSecret), plus, in a
	// repository git could not answer for, every carried file that looks like
	// one: only git could have said it was committed on purpose.
	Secrets []string
	// Unanswered says, per repository git could not answer for, which and
	// why.
	Unanswered []string
}

// Check surveys the files the image built from the project at dir would carry
// (imagebuild.SurveyProject; dockerfile is the manifest's [tool.astro]
// dockerfile, "" for a generated build) and asks git which of them it
// ignores. Outside a git work tree there is nothing to ask, and it reports
// nothing.
func Check(dir, dockerfile string) (Report, error) {
	if !git.InWorkTree(dir) {
		return Report{}, nil
	}
	survey, err := imagebuild.SurveyProject(dir, dockerfile)
	if err != nil {
		return Report{}, err
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return Report{}, err
	}
	var r Report
	r.checkIgnored(root, survey.Files, nestedRepos(root, survey.Files))
	return r, nil
}

// Warnings are what a build goes ahead despite: carried files git ignores,
// and repositories git could not answer for.
func (r *Report) Warnings() []string {
	var out []string
	if len(r.Gitignored) > 0 {
		out = append(out, fmt.Sprintf("the image will carry %d file(s) that .gitignore ignores and .dockerignore does not: %s. If they do not belong in the image, add them to .dockerignore",
			len(r.Gitignored), cappedList(r.Gitignored)))
	}
	for _, u := range r.Unanswered {
		out = append(out, fmt.Sprintf("could not ask git which files it ignores in %s; every file there that looks like a credential is refused instead", u))
	}
	return out
}

// Err refuses a build that would carry files that look like credentials, and
// that git ignores or could not vouch for; nil when there are none.
func (r *Report) Err() error {
	if len(r.Secrets) == 0 {
		return nil
	}
	why := "git ignores them"
	if len(r.Unanswered) > 0 {
		why = "git ignores them, or could not say whether they are committed"
	}
	return fmt.Errorf("refusing to build: %s look like credentials, %s, and the image would carry them to a registry. Add them to .dockerignore, or remove them from the project, and try again",
		cappedList(r.Secrets), why)
}

// Run is Check for a command: it hands each warning to warn and returns the
// refusal, or the error that kept the check from running.
func Run(dir, dockerfile string, warn func(string)) error {
	r, err := Check(dir, dockerfile)
	if err != nil {
		return fmt.Errorf("checking the files the image would carry: %w", err)
	}
	for _, w := range r.Warnings() {
		warn(w)
	}
	return r.Err()
}

// nestedRepos lists the directories, among those holding files, that are
// repositories of their own (a submodule, a nested clone), slash-separated.
// Git answers for their files only from inside them.
func nestedRepos(root string, files []string) []string {
	seen := map[string]bool{}
	var repos []string
	for _, f := range files {
		for d := path.Dir(f); d != "."; d = path.Dir(d) {
			if seen[d] {
				break
			}
			seen[d] = true
			if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(d), ".git")); err == nil {
				repos = append(repos, d)
			}
		}
	}
	slices.Sort(repos)
	return repos
}

// checkIgnored asks git which files it ignores, once per repository: the
// project's own, and each one nested in it (repos), for the files under it. A
// nested repository the enclosing one ignores as a whole has every one of its
// files counted as ignored.
func (r *Report) checkIgnored(root string, files, repos []string) {
	byRepo := partitionByRepo(files, repos)
	owners := make([]string, 0, len(byRepo))
	for o := range byRepo {
		owners = append(owners, o)
	}
	slices.Sort(owners)
	ignoredRepos := map[string]bool{}
	for _, o := range owners {
		for _, p := range r.askRepo(root, o, byRepo[o], repos) {
			if slices.Contains(repos, p) {
				ignoredRepos[p] = true
			} else {
				r.Gitignored = append(r.Gitignored, p)
			}
		}
	}
	for _, f := range files {
		if !slices.Contains(r.Gitignored, f) && underAny(f, ignoredRepos) {
			r.Gitignored = append(r.Gitignored, f)
		}
	}
	for _, f := range r.Gitignored {
		if LooksSecret(f) && !slices.Contains(r.Secrets, f) {
			r.Secrets = append(r.Secrets, f)
		}
	}
}

// partitionByRepo groups files by the repository that answers for them, the
// innermost of repos holding each ("" for the project's own). Each nested
// repository is also listed, as a directory, under the one it sits in, which
// can ignore it whole.
func partitionByRepo(files, repos []string) map[string][]string {
	owner := func(p string) string {
		best := ""
		for _, r := range repos {
			if strings.HasPrefix(p, r+"/") && len(r) > len(best) {
				best = r
			}
		}
		return best
	}
	byRepo := map[string][]string{"": nil}
	for _, f := range files {
		byRepo[owner(f)] = append(byRepo[owner(f)], f)
	}
	for _, r := range repos {
		byRepo[owner(r)] = append(byRepo[owner(r)], r)
		if _, ok := byRepo[r]; !ok {
			byRepo[r] = nil
		}
	}
	return byRepo
}

// askRepo asks the repository at repo which of paths (project-relative) git
// ignores, and returns them project-relative. When git cannot answer it notes
// why in Unanswered and puts every credential-looking file of paths in
// Secrets.
func (r *Report) askRepo(root, repo string, paths, repos []string) []string {
	if len(paths) == 0 {
		return nil
	}
	rel := make([]string, len(paths))
	for i, p := range paths {
		rel[i] = strings.TrimPrefix(p, repo+"/")
	}
	ignored, err := git.CheckIgnored(filepath.Join(root, filepath.FromSlash(repo)), rel)
	if err != nil {
		r.Unanswered = append(r.Unanswered, fmt.Sprintf("%s (%v)", repoName(repo), err))
		for _, p := range paths {
			if !slices.Contains(repos, p) && LooksSecret(p) {
				r.Secrets = append(r.Secrets, p)
			}
		}
		return nil
	}
	out := make([]string, len(ignored))
	for i, p := range ignored {
		out[i] = path.Join(repo, p)
	}
	return out
}

// underAny reports whether p is below any of dirs.
func underAny(p string, dirs map[string]bool) bool {
	for d := range dirs {
		if strings.HasPrefix(p, d+"/") {
			return true
		}
	}
	return false
}

func repoName(repo string) string {
	if repo == "" {
		return "the project's repository"
	}
	return "the repository at " + repo
}
