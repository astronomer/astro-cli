// Package shipcheck holds an image that is pushed to a registry (a deploy's,
// `astro package astro`'s) to what git says about the files it carries. A
// file the project's .gitignore keeps out of the repository but no ignore file
// keeps out of the image is warned about; a carried file that looks like a
// credential and that git does not track stops the build, and so does git
// metadata whose config holds a credential.
//
// A declared Dockerfile is held to the same rule as a generated build:
// everything in its build context counts as shipped, whatever the Dockerfile
// copies.
package shipcheck

import (
	"cmp"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/git"
	"github.com/astronomer/astro-cli/pkg/imagebuild"
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// Report is what git says about the files an image would carry.
type Report struct {
	// Gitignored are the carried files git ignores, slash-separated and
	// relative to the project.
	Gitignored []string
	// Secrets are the carried files that look like credentials
	// (LooksSecret) and that git does not track: ignored, never added, or
	// reached through a link that leaves the project. A committed file is
	// the project's choice and is never here.
	Secrets []string
	// Unanswered says, per repository git could not answer for, which and
	// why. Unchecked are the carried files there that look like
	// credentials, which are warned about rather than refused.
	Unanswered, Unchecked []string
	// VCSSecrets is the git metadata a declared Dockerfile's context carries
	// (imagebuild.ProjectSurvey.VCS) whose config holds a credential.
	VCSSecrets []string
	// IgnoreFile is the ignore file the build reads, for the messages.
	IgnoreFile string
}

// Check asks git about the files an image built from the project at dir
// would carry, as survey lists them (imagebuild.SurveyProject). Outside a git
// work tree there is nothing to ask, and only git metadata a declared build
// would carry is looked at.
func Check(dir string, survey *imagebuild.ProjectSurvey) Report {
	r := Report{IgnoreFile: cmp.Or(survey.IgnoreFile, ".dockerignore")}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		root = dir
	}
	for _, v := range survey.VCS {
		if vcsConfigHoldsCredential(filepath.Join(root, filepath.FromSlash(v))) {
			r.VCSSecrets = append(r.VCSSecrets, v)
		}
	}
	if git.InWorkTree(root) {
		r.checkGit(root, survey.Files)
	}
	return r
}

// Warnings are what a build goes ahead despite.
func (r *Report) Warnings() []string {
	var out []string
	if len(r.Gitignored) > 0 {
		out = append(out, fmt.Sprintf("the image will carry %d file(s) that .gitignore ignores and %s does not: %s. If they do not belong in the image, add them to %s",
			len(r.Gitignored), r.IgnoreFile, cappedList(r.Gitignored), r.IgnoreFile))
	}
	for _, u := range r.Unanswered {
		out = append(out, fmt.Sprintf("could not ask git which files it tracks in %s, so files there that look like credentials were not checked", u))
	}
	if len(r.Unchecked) > 0 {
		out = append(out, fmt.Sprintf("the image will carry these files that look like credentials, which git could not say are committed: %s. If they do not belong in the image, add them to %s",
			cappedList(r.Unchecked), r.IgnoreFile))
	}
	return out
}

// Err refuses a build that would put credentials in an image bound for a
// registry: carried files that look like credentials and that git does not
// track, and git metadata whose config holds one. nil when there are none.
func (r *Report) Err() error {
	if len(r.Secrets) > 0 {
		return fmt.Errorf("refusing to build: %s look like credentials, are not committed to git, and the image would carry them to a registry. Add them to %s, or remove them from the project, and try again",
			cappedList(r.Secrets), r.IgnoreFile)
	}
	if len(r.VCSSecrets) > 0 {
		return fmt.Errorf("refusing to build: the git config under %s holds a credential (an extraheader, or a URL with a user or token in it), and the image would carry it to a registry. Add **/.git to %s and try again",
			cappedList(r.VCSSecrets), r.IgnoreFile)
	}
	return nil
}

// Survey is the check a command runs before it builds an image to push. It
// surveys what the image built from the project at dir would carry
// (imagebuild.SurveyProject; dockerfile is the manifest's [tool.astro]
// dockerfile, podman whether podman builds it), warns about the local and
// per-machine files a declared Dockerfile's context carries
// (scaffold.LocalFilesWarningFor), and holds the image to what git says of its
// files (Check), handing each warning to warn. It returns the survey, for the
// rest of the command to read rather than walk the project again, and the
// refusal.
func Survey(dir, dockerfile string, podman bool, warn func(string)) (*imagebuild.ProjectSurvey, error) {
	survey, err := imagebuild.SurveyProject(dir, dockerfile, podman)
	if err != nil {
		return nil, err
	}
	if w := scaffold.LocalFilesWarningFor(dockerfile, survey.IgnoreFile, slices.Concat(survey.Files, survey.VCS)); w != "" {
		warn(w)
	}
	r := Check(dir, &survey)
	for _, w := range r.Warnings() {
		warn(w)
	}
	return &survey, r.Err()
}

// credentialURL matches a URL carrying userinfo: a user, a password, or a
// token alone (https://ghp_x@github.com/), as a remote or an insteadOf can.
var credentialURL = regexp.MustCompile(`://[^/@\s]+@`)

// vcsConfigHoldsCredential reports whether the git metadata at dir (a .git
// directory) has a config holding a credential: an http extraheader, which
// CI checkouts use for a token, or a URL with userinfo. A .git file (a
// submodule's, a worktree's) points elsewhere and holds no config.
func vcsConfigHoldsCredential(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "config"))
	if err != nil {
		return false
	}
	text := string(data)
	return strings.Contains(strings.ToLower(text), "extraheader") || credentialURL.MatchString(text)
}

// checkGit asks git about the carried files, once per repository: the
// project's own, and each one nested in it, for the files under it. A nested
// repository the enclosing one ignores as a whole has every one of its files
// counted as ignored. A file whose real location is outside the project
// (through a link) is not git's to answer for, and counts as untracked.
func (r *Report) checkGit(root string, files []string) {
	queries := queryPaths(root, files)
	repos := nestedRepos(root, queries)
	byRepo := partitionByRepo(queries, repos)
	owners := make([]string, 0, len(byRepo))
	for o := range byRepo {
		owners = append(owners, o)
	}
	slices.Sort(owners)

	ignored := map[string]bool{}
	tracked := map[string]bool{}
	unanswered := map[string]bool{}
	for _, o := range owners {
		ign, trk, err := askRepo(root, o, byRepo[o])
		if err != nil {
			r.Unanswered = append(r.Unanswered, fmt.Sprintf("%s (%v)", repoName(o), err))
			unanswered[o] = true
			continue
		}
		for _, q := range ign {
			ignored[q] = true
		}
		for _, q := range trk {
			tracked[q] = true
		}
	}
	ignoredRepos := map[string]bool{}
	for _, repo := range repos {
		if ignored[repo] {
			ignoredRepos[repo] = true
		}
	}
	for _, f := range files {
		q, inProject := queries[f]
		if !inProject {
			if LooksSecret(f) {
				r.Secrets = append(r.Secrets, f)
			}
			continue
		}
		if unanswered[ownerOf(q, repos)] {
			if LooksSecret(f) {
				r.Unchecked = append(r.Unchecked, f)
			}
			continue
		}
		isIgnored := ignored[q] || underAny(q, ignoredRepos)
		if isIgnored {
			r.Gitignored = append(r.Gitignored, f)
		}
		if LooksSecret(f) && (isIgnored || !tracked[q]) {
			r.Secrets = append(r.Secrets, f)
		}
	}
}

// queryPaths maps each carried file whose real location is inside the
// project to the path git is asked about for it: that real location, relative
// to the project, so a file reached through a link is never a path beyond
// one, which git refuses. A file whose real location is outside the project
// has no entry.
func queryPaths(root string, files []string) map[string]string {
	queries := make(map[string]string, len(files))
	for _, f := range files {
		resolved, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			queries[f] = f // gone since the survey: ask about it as listed
			continue
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		queries[f] = filepath.ToSlash(rel)
	}
	return queries
}

// nestedRepos lists the directories inside the project, among those holding
// queried paths, that are repositories of their own (a submodule, a nested
// clone), slash-separated. Git answers for their files only from inside them.
func nestedRepos(root string, queries map[string]string) []string {
	seen := map[string]bool{}
	var repos []string
	for _, q := range queries {
		for d := path.Dir(q); d != "."; d = path.Dir(d) {
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

// ownerOf is the repository that answers for q: the innermost of repos
// holding it, "" for the project's own.
func ownerOf(q string, repos []string) string {
	best := ""
	for _, r := range repos {
		if strings.HasPrefix(q, r+"/") && len(r) > len(best) {
			best = r
		}
	}
	return best
}

// partitionByRepo groups the queried paths, without repeats, by the
// repository that answers for them. Each nested repository is also listed,
// as a directory, under the one it sits in, which can ignore it whole.
func partitionByRepo(queries map[string]string, repos []string) map[string][]string {
	byRepo := map[string][]string{"": nil}
	seen := map[string]bool{}
	add := func(owner, q string) {
		if !seen[q] {
			seen[q] = true
			byRepo[owner] = append(byRepo[owner], q)
		}
	}
	for _, q := range queries {
		add(ownerOf(q, repos), q)
	}
	for _, r := range repos {
		add(ownerOf(r, repos), r)
		if _, ok := byRepo[r]; !ok {
			byRepo[r] = nil
		}
	}
	for _, qs := range byRepo {
		slices.Sort(qs)
	}
	return byRepo
}

// askRepo asks the repository at repo which of paths (project-relative) it
// ignores and which it tracks, and returns both project-relative.
func askRepo(root, repo string, paths []string) (ignored, tracked []string, err error) {
	if len(paths) == 0 {
		return nil, nil, nil
	}
	dir := filepath.Join(root, filepath.FromSlash(repo))
	rel := make([]string, len(paths))
	for i, p := range paths {
		rel[i] = strings.TrimPrefix(p, repo+"/")
	}
	ign, err := git.CheckIgnored(dir, rel)
	if err != nil {
		return nil, nil, err
	}
	trk, err := git.Tracked(dir, rel)
	if err != nil {
		return nil, nil, err
	}
	back := func(ps []string) []string {
		out := make([]string, len(ps))
		for i, p := range ps {
			out[i] = path.Join(repo, p)
		}
		return out
	}
	return back(ign), back(trk), nil
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
