// Package shipcheck holds an image that is pushed to a registry (a deploy's,
// `astro package astro`'s) to what git says about the files it carries. A
// file the project's .gitignore keeps out of the repository but no
// .dockerignore keeps out of the image is warned about; a carried file that
// looks like a credential and that git does not track stops the build, and so
// does git metadata whose config holds a credential.
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
	"github.com/astronomer/astro-cli/pkg/scaffold"
)

// Report is what git says about the files an image would carry.
type Report struct {
	// Gitignored are the carried files git ignores, slash-separated and
	// relative to the project.
	Gitignored []string
	// Secrets are the carried files that look like credentials
	// (LooksSecret) and that git does not track: ignored, or never added.
	// A committed file is the project's choice and is never here.
	Secrets []string
	// Unanswered says, per repository git could not answer for, which and
	// why. Unchecked are the carried files there that look like
	// credentials, which are warned about rather than refused.
	Unanswered, Unchecked []string
	// VCS is the git metadata a declared Dockerfile's context carries
	// (imagebuild.ProjectSurvey.VCS), and VCSSecrets those of it whose
	// config holds a credential: an extraheader, or a user:password URL.
	VCS, VCSSecrets []string
	// IgnoreFile is the ignore file the build reads, for the messages.
	IgnoreFile string
	// CopiesContext reports that the build puts the whole context in the
	// image: a generated build always does; a declared Dockerfile does when
	// it COPYs or ADDs the context's root, or builds on an Astro Runtime
	// image, whose ONBUILD `COPY . .` does. Only then does a credential stop
	// the build; otherwise it is warned about.
	CopiesContext bool
	// dockerfile is the manifest's [tool.astro] dockerfile, "" for a
	// generated build.
	dockerfile string
}

// Check asks git about the files an image built from the project at dir
// would carry, as survey lists them (imagebuild.SurveyProject; dockerfile is
// the manifest's [tool.astro] dockerfile, "" for a generated build). Outside a
// git work tree there is nothing to ask, and only the git metadata a declared
// build would carry is reported.
func Check(dir, dockerfile string, survey *imagebuild.ProjectSurvey) Report {
	r := Report{dockerfile: dockerfile, IgnoreFile: survey.IgnoreFile, CopiesContext: true}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		root = dir
	}
	if dockerfile != "" {
		r.CopiesContext = copiesContext(filepath.Join(root, filepath.FromSlash(dockerfile)))
		r.VCS = survey.VCS
		for _, v := range survey.VCS {
			if vcsConfigHoldsCredential(filepath.Join(root, filepath.FromSlash(v))) {
				r.VCSSecrets = append(r.VCSSecrets, v)
			}
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
	ignoreFile := cmpOr(r.IgnoreFile, ".dockerignore")
	if len(r.Gitignored) > 0 {
		out = append(out, fmt.Sprintf("the image will carry %d file(s) that .gitignore ignores and %s does not: %s. If they do not belong in the image, add them to %s",
			len(r.Gitignored), ignoreFile, cappedList(r.Gitignored), ignoreFile))
	}
	for _, u := range r.Unanswered {
		out = append(out, fmt.Sprintf("could not ask git which files it tracks in %s, so files there that look like credentials were not checked", u))
	}
	if len(r.Unchecked) > 0 {
		out = append(out, fmt.Sprintf("the image will carry these files that look like credentials, which git could not say are committed: %s. If they do not belong in the image, add them to %s",
			cappedList(r.Unchecked), ignoreFile))
	}
	// Git metadata in the context is named by scaffold.LocalFilesWarningFor,
	// whose rules include **/.git; a credential in it is said here.
	if len(r.VCSSecrets) > 0 && !r.CopiesContext {
		out = append(out, fmt.Sprintf("the build context of %s carries git metadata whose config holds a credential (%s). %s does not copy the whole context, so the build goes ahead; add **/.git to %s to keep it out of the context",
			r.dockerfile, cappedList(r.VCSSecrets), r.dockerfile, ignoreFile))
	}
	if !r.CopiesContext && len(r.Secrets) > 0 {
		out = append(out, fmt.Sprintf("the build context of %s carries files that look like credentials and that git does not track: %s. %s does not copy the whole context, so the build goes ahead; add them to %s to keep them out of the context",
			r.dockerfile, cappedList(r.Secrets), r.dockerfile, ignoreFile))
	}
	return out
}

// Err refuses a build that would put credentials in an image bound for a
// registry: carried files that look like credentials and that git does not
// track, and git metadata whose config holds one. nil when there are none, or
// when the build does not copy the whole context (Warnings says so then).
func (r *Report) Err() error {
	if !r.CopiesContext {
		return nil
	}
	ignoreFile := cmpOr(r.IgnoreFile, ".dockerignore")
	if len(r.Secrets) > 0 {
		return fmt.Errorf("refusing to build: %s look like credentials, are not committed to git, and the image would carry them to a registry. Add them to %s, or remove them from the project, and try again",
			cappedList(r.Secrets), ignoreFile)
	}
	if len(r.VCSSecrets) > 0 {
		return fmt.Errorf("refusing to build: the git config under %s holds a credential (an extraheader, or a password in a remote URL), and the image would carry it to a registry. Add **/.git to %s and try again",
			cappedList(r.VCSSecrets), ignoreFile)
	}
	return nil
}

// Survey is the check a command runs before it builds an image to push. It
// surveys what the image built from the project at dir would carry
// (imagebuild.SurveyProject; dockerfile is the manifest's [tool.astro]
// dockerfile), warns about the local and per-machine files a declared
// Dockerfile's context carries (scaffold.LocalFilesWarningFor), and holds the
// image to what git says of its files (Check), handing each warning to warn.
// It returns the survey, for the rest of the command to read rather than walk
// the project again, and the refusal.
func Survey(dir, dockerfile string, warn func(string)) (*imagebuild.ProjectSurvey, error) {
	survey, err := imagebuild.SurveyProject(dir, dockerfile)
	if err != nil {
		return nil, err
	}
	if w := scaffold.LocalFilesWarningFor(dockerfile, survey.IgnoreFile, slices.Concat(survey.Files, survey.VCS)); w != "" {
		warn(w)
	}
	r := Check(dir, dockerfile, &survey)
	for _, w := range r.Warnings() {
		warn(w)
	}
	return &survey, r.Err()
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// checkGit asks git about the carried files, once per repository: the
// project's own, and each one nested in it, for the files under it. A nested
// repository the enclosing one ignores as a whole has every one of its files
// counted as ignored.
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
		q := queries[f]
		owner := ownerOf(q, repos)
		if unanswered[owner] {
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

// queryPaths maps each carried file to the path git is asked about for it.
// Git refuses a path beyond a symbolic link, and one such path would fail the
// whole query, so a file under a linked directory is asked about at the path
// the link resolves to when that is in the project's work tree (which may
// climb out of the project with ..), and otherwise as the link itself.
func queryPaths(root string, files []string) map[string]string {
	top, topErr := git.TopLevel(root)
	queries := make(map[string]string, len(files))
	linkCache := map[string]bool{}
	isLink := func(rel string) bool {
		if v, ok := linkCache[rel]; ok {
			return v
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
		v := err == nil && info.Mode()&os.ModeSymlink != 0
		linkCache[rel] = v
		return v
	}
	for _, f := range files {
		queries[f] = f
		parts := strings.Split(f, "/")
		for n := 1; n < len(parts); n++ {
			link := strings.Join(parts[:n], "/")
			if !isLink(link) {
				continue
			}
			queries[f] = link
			resolved, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(f)))
			if err != nil || topErr != nil {
				break
			}
			if inTop, err := filepath.Rel(top, resolved); err != nil || inTop == ".." || strings.HasPrefix(inTop, ".."+string(filepath.Separator)) {
				break
			}
			if rel, err := filepath.Rel(root, resolved); err == nil {
				queries[f] = filepath.ToSlash(rel)
			}
			break
		}
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
		if strings.HasPrefix(q, "../") {
			continue
		}
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
