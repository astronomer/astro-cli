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
	// (LooksSecret) and that git does not track: ignored, or never added.
	// A committed file is the project's choice and is never here.
	Secrets []string
	// Outside are the carried files that look like credentials and whose
	// real location, reached through a link, is in no git repository, so
	// nothing says they were meant to be shared.
	Outside []string
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
		r.checkGit(root, survey.Files, survey.Links)
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
// track or that sit outside any git repository, and git metadata whose
// config holds one. nil when there are none.
func (r *Report) Err() error {
	var why []string
	if len(r.Secrets) > 0 {
		why = append(why, fmt.Sprintf("%s look like credentials and are not tracked by git", cappedList(r.Secrets)))
	}
	if len(r.Outside) > 0 {
		why = append(why, fmt.Sprintf("%s look like credentials and are reached through a link to a place outside any git repository", cappedList(r.Outside)))
	}
	if len(why) > 0 {
		return fmt.Errorf("refusing to build: %s, and the image would carry them to a registry. Add them to %s, or remove them from the project, and try again",
			strings.Join(why, "; "), r.IgnoreFile)
	}
	if len(r.VCSSecrets) > 0 {
		return fmt.Errorf("refusing to build: the git config under %s holds a credential (an extraheader, or a URL with a password or token in it), and the image would carry it to a registry. Add **/.git to %s and try again",
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

// credentialURL matches a URL whose userinfo carries a credential: a
// password (user:pass@), or a user that is a token, by a known token prefix
// or by being a long run of token characters. A plain user, as in
// ssh://git@github.com or https://user@bitbucket.org, is not one.
var credentialURL = regexp.MustCompile(`://(?:[^/@\s:]*:[^/@\s]+|(?:ghp_|gho_|ghu_|ghs_|ghr_|github_pat_|glpat-|x-access-token|oauth2)[^/@\s]*|[A-Za-z0-9_-]{30,})@`)

// vcsConfigHoldsCredential reports whether the git metadata at dir (a .git
// directory) has a config holding a credential: an http extraheader, which
// CI checkouts use for a token, or a URL with one (credentialURL). A .git
// file (a submodule's, a worktree's) points elsewhere and holds no config.
func vcsConfigHoldsCredential(dir string) bool {
	data, err := os.ReadFile(filepath.Join(dir, "config"))
	if err != nil {
		return false
	}
	text := string(data)
	return strings.Contains(strings.ToLower(text), "extraheader") || credentialURL.MatchString(text)
}

// verdict is what git said of one carried file.
type verdict struct {
	ignored, tracked, unanswered, outside bool
}

// checkGit asks git about the carried files. A file in the project (or
// reached through a link that stays in it) is asked of the project's
// repository, or of a repository nested in it, at its real path. A file
// reached through a link out of the project is asked of the repository that
// holds where the link leads, and one that no repository holds is outside.
func (r *Report) checkGit(root string, files, links []string) {
	inProject, outside := locate(root, files, links)
	verdicts := askProject(root, inProject, &r.Unanswered)
	for f, v := range askOutside(outside, &r.Unanswered) {
		verdicts[f] = v
	}
	for _, f := range files {
		v := verdicts[f]
		switch {
		case v.outside:
			if LooksSecret(f) {
				r.Outside = append(r.Outside, f)
			}
			continue
		case v.unanswered:
			if LooksSecret(f) {
				r.Unchecked = append(r.Unchecked, f)
			}
			continue
		}
		if v.ignored {
			r.Gitignored = append(r.Gitignored, f)
		}
		if LooksSecret(f) && (v.ignored || !v.tracked) {
			r.Secrets = append(r.Secrets, f)
		}
	}
}

// outsideFile is a carried file whose real location is outside the project:
// the directory git is asked from, and its path there.
type outsideFile struct{ dir, rel string }

// locate maps each carried file to where git is asked about it. Only a file
// that is, or sits under, one of links (the symlinks a generated build went
// through) can be anywhere but its listed path; the rest are asked about as
// listed. A linked one is asked about at its real path when that is in the
// project; when it is not, from the directory the link leads to (or the
// file's own, when a further link leads elsewhere). A path that cannot be
// resolved is asked about as its link, never as a path beyond one.
func locate(root string, files, links []string) (inProject map[string]string, outside map[string]outsideFile) {
	inProject = make(map[string]string, len(files))
	outside = map[string]outsideFile{}
	linkTargets := map[string]string{}
	for _, f := range files {
		link := linkOf(f, links)
		if link == "" {
			inProject[f] = f
			continue
		}
		resolved, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			inProject[f] = link
			continue
		}
		if rel, ok := within(root, resolved); ok {
			inProject[f] = rel
			continue
		}
		target, ok := linkTargets[link]
		if !ok {
			resolvedLink, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(link)))
			if err != nil {
				resolvedLink = "" // asked from the file's own directory instead
			}
			target = resolvedLink
			linkTargets[link] = target
		}
		if rel, ok := within(target, resolved); ok && target != "" && rel != "." {
			outside[f] = outsideFile{dir: target, rel: rel}
		} else {
			outside[f] = outsideFile{dir: filepath.Dir(resolved), rel: filepath.Base(resolved)}
		}
	}
	return inProject, outside
}

// linkOf is the longest of links that is f or a parent of it, "" for none.
func linkOf(f string, links []string) string {
	best := ""
	for _, l := range links {
		if (f == l || strings.HasPrefix(f, l+"/")) && len(l) > len(best) {
			best = l
		}
	}
	return best
}

// within reports p relative to base, slash-separated, when p is base or
// below it.
func within(base, p string) (string, bool) {
	rel, err := filepath.Rel(base, p)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

// askProject asks the project's repository, and each one nested in it, about
// the files queries maps to project paths: once per repository, for the
// files under it. A nested repository the enclosing one ignores as a whole
// has every one of its files counted as ignored.
func askProject(root string, queries map[string]string, unanswered *[]string) map[string]verdict {
	repos := nestedRepos(root, queries)
	byRepo := partitionByRepo(queries, repos)
	owners := make([]string, 0, len(byRepo))
	for o := range byRepo {
		owners = append(owners, o)
	}
	slices.Sort(owners)
	ignored, tracked, failed := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, o := range owners {
		ign, trk, err := ask(filepath.Join(root, filepath.FromSlash(o)), o, byRepo[o])
		if err != nil {
			*unanswered = append(*unanswered, fmt.Sprintf("%s (%v)", repoName(o), err))
			failed[o] = true
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
	out := make(map[string]verdict, len(queries))
	for f, q := range queries {
		out[f] = verdict{
			ignored:    ignored[q] || underAny(q, ignoredRepos),
			tracked:    tracked[q],
			unanswered: failed[ownerOf(q, repos)],
		}
	}
	return out
}

// askOutside asks about the files reached through links out of the project,
// from the directories they lead to: once per directory, of the repository
// holding it. A directory no repository holds makes its files outside.
func askOutside(files map[string]outsideFile, unanswered *[]string) map[string]verdict {
	byDir := map[string][]string{}
	for f, o := range files {
		byDir[o.dir] = append(byDir[o.dir], f)
	}
	out := make(map[string]verdict, len(files))
	for dir, fs := range byDir {
		if !git.InWorkTree(dir) {
			for _, f := range fs {
				out[f] = verdict{outside: true}
			}
			continue
		}
		paths := make([]string, len(fs))
		for i, f := range fs {
			paths[i] = files[f].rel
		}
		ign, trk, err := ask(dir, "", paths)
		if err != nil {
			*unanswered = append(*unanswered, fmt.Sprintf("the repository holding %s (%v)", dir, err))
			for _, f := range fs {
				out[f] = verdict{unanswered: true}
			}
			continue
		}
		ignored, tracked := setOf(ign), setOf(trk)
		for _, f := range fs {
			rel := files[f].rel
			out[f] = verdict{ignored: ignored[rel], tracked: tracked[rel]}
		}
	}
	return out
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

// ask asks the repository holding dir which of paths it ignores and which it
// tracks. paths are relative to the project and sit under prefix, which is
// dir's own path in the project ("" for none); both answers come back in the
// same form.
func ask(dir, prefix string, paths []string) (ignored, tracked []string, err error) {
	if len(paths) == 0 {
		return nil, nil, nil
	}
	rel := make([]string, len(paths))
	for i, p := range paths {
		rel[i] = strings.TrimPrefix(p, prefix+"/")
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
			out[i] = path.Join(prefix, p)
		}
		return out
	}
	return back(ign), back(trk), nil
}

func setOf(ps []string) map[string]bool {
	set := make(map[string]bool, len(ps))
	for _, p := range ps {
		set[p] = true
	}
	return set
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
