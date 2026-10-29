// Package shipcheck holds an image that is pushed to a registry (a deploy's,
// `astro package astro`'s) to what git says about the files it carries. A
// file the project's .gitignore keeps out of the repository but no ignore file
// keeps out of the image is warned about; a carried file that looks like a
// credential and that git does not track stops the build, and so does one a
// generated build reaches through a link out of the project, and git
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

// Report is what the check found in the files an image would carry.
type Report struct {
	// Gitignored are the carried files git ignores, slash-separated and
	// relative to the project.
	Gitignored []string
	// Secrets are the carried files that look like credentials
	// (LooksSecret) and that git does not track: ignored, or never added.
	// A committed file is the project's choice and is never here.
	Secrets []string
	// Outside are the carried files a generated build reaches through a link
	// out of the project that look like credentials, by their path in the
	// project or where the link leads, each as "path → target". Git is not
	// asked about them.
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
	// carried is every file the image would carry, for the gitignored
	// warning to name only places that hold nothing else that ships.
	carried []string
	// nested are the git checkouts inside the project a declared
	// Dockerfile's context carries (imagebuild.ProjectSurvey.Nested), for
	// the refusal to name the checkout a file sits in: git there may track
	// it, but this project's git does not.
	nested []string
}

// Check looks at the files an image built from the project at dir would
// carry, as survey lists them (imagebuild.SurveyProject). Files reached
// through a link out of the project, and git metadata a declared build
// carries, are looked at always; the rest are asked of git, and outside a git
// work tree there is nothing to ask.
func Check(dir string, survey *imagebuild.ProjectSurvey) Report {
	r := Report{IgnoreFile: cmp.Or(survey.IgnoreFile, ".dockerignore"), carried: survey.Files, nested: survey.Nested}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		root = dir
	}
	for _, v := range survey.VCS {
		if vcsConfigHoldsCredential(filepath.Join(root, filepath.FromSlash(v))) {
			r.VCSSecrets = append(r.VCSSecrets, v)
		}
	}
	inProject, outside := locate(root, survey.Files, survey.Links)
	for _, f := range survey.Files {
		if target, ok := outside[f]; ok && (LooksSecret(f) || LooksSecret(filepath.ToSlash(target))) {
			r.Outside = append(r.Outside, f+" → "+target)
		}
	}
	if git.InWorkTree(root) {
		r.checkGit(root, survey.Files, inProject)
	}
	return r
}

// Warnings are what a build goes ahead despite.
func (r *Report) Warnings() []string {
	var out []string
	if n := len(r.Gitignored); n > 0 {
		out = append(out, fmt.Sprintf("the image will carry %s that .gitignore ignores and %s does not, in %s. If %s not belong in the image, add %s to %s",
			countFiles(n), r.IgnoreFile, placesOf(r.Gitignored, r.carried), agree(n, "it does", "they do"), agree(n, "it", "them"), r.IgnoreFile))
	}
	for _, u := range r.Unanswered {
		out = append(out, fmt.Sprintf("could not ask git which files it tracks in %s, so files there that look like credentials were not checked", u))
	}
	if n := len(r.Unchecked); n > 0 {
		out = append(out, fmt.Sprintf("the image will carry %s, which git could not say %s committed: %s. If %s not belong in the image, add %s to %s",
			agree(n, "this file that looks like credentials", "these files that look like credentials"), agree(n, "is", "are"),
			cappedList(r.Unchecked), agree(n, "it does", "they do"), agree(n, "it", "them"), r.IgnoreFile))
	}
	return out
}

// agree is one when n is 1 and many otherwise, for the words of a message
// that name or refer back to the files it lists.
func agree(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Err refuses a build that would put credentials in an image bound for a
// registry: carried files that look like credentials and that git does not
// track, ones reached through a link out of the project, ones inside another
// git checkout, and git metadata whose config holds one. Every reason found
// is in the one refusal, so fixing it fixes the build. nil when there are
// none.
func (r *Report) Err() error {
	inCheckout, own := r.byCheckout(r.Secrets)
	var sentences, why []string
	if n := len(own); n > 0 {
		why = append(why, fmt.Sprintf("%s %s and %s not tracked by git",
			cappedList(own), agree(n, "looks like credentials", "look like credentials"), agree(n, "is", "are")))
	}
	if n := len(r.Outside); n > 0 {
		why = append(why, fmt.Sprintf("%s %s and %s reached through a link out of the project",
			cappedList(r.Outside), agree(n, "looks like credentials", "look like credentials"), agree(n, "is", "are")))
	}
	if len(why) > 0 {
		it := agree(len(own)+len(r.Outside), "it", "them")
		sentences = append(sentences, fmt.Sprintf("%s, and the image would carry %s to a registry. Add %s to %s, or remove %s from the project",
			strings.Join(why, "; "), it, it, r.IgnoreFile, it))
	}
	if len(inCheckout) > 0 {
		sentences = append(sentences, r.checkoutAdvice(inCheckout))
	}
	if n := len(r.VCSSecrets); n > 0 {
		// It follows "refusing to build: " in lower case, and starts a
		// sentence of its own after another reason.
		the := "the"
		if len(sentences) > 0 {
			the = "The"
		}
		sentences = append(sentences, fmt.Sprintf("%s git %s under %s %s (an extraheader, or a URL with a password or a known token in it), and the image would carry %s to a registry. Add **/.git to %s",
			the, agree(n, "config", "configs"), cappedList(r.VCSSecrets), agree(n, "holds a credential", "hold credentials"), agree(n, "it", "them"), r.IgnoreFile))
	}
	if len(sentences) == 0 {
		return nil
	}
	return fmt.Errorf("refusing to build: %s, and try again", strings.Join(sentences, ". "))
}

// byCheckout splits files into those inside one of the nested checkouts a
// declared build carries (r.nested), mapped to that checkout, and the rest.
func (r *Report) byCheckout(files []string) (inCheckout map[string][]string, rest []string) {
	inCheckout = map[string][]string{}
	for _, f := range files {
		if c := checkoutOf(f, r.nested); c != "" {
			inCheckout[c] = append(inCheckout[c], f)
			continue
		}
		rest = append(rest, f)
	}
	return inCheckout, rest
}

// checkoutOf is the innermost of checkouts holding f, "" for none.
func checkoutOf(f string, checkouts []string) string {
	best := ""
	for _, c := range checkouts {
		if strings.HasPrefix(f, c+"/") && len(c) > len(best) {
			best = c
		}
	}
	return best
}

// checkoutAdvice says that the files flagged inside nested checkouts sit in
// another git checkout rather than in this project's repository: git there
// may track them, which is why "not tracked by git" alone reads wrong, and
// the fix is to leave the checkout out, not to commit or remove a file.
func (r *Report) checkoutAdvice(inCheckout map[string][]string) string {
	var files, dirs []string
	for c, fs := range inCheckout {
		files = append(files, fs...)
		dirs = append(dirs, c+"/")
	}
	slices.Sort(files)
	slices.Sort(dirs)
	n, d := len(files), len(dirs)
	return fmt.Sprintf("%s %s and %s in %s, %s inside the project, which the image would carry to a registry. Add %s to %s",
		cappedList(files), agree(n, "looks like credentials", "look like credentials"), agree(n, "sits", "sit"),
		joinAnd(dirs), agree(d, "another git checkout", "other git checkouts"), joinAnd(dirs), r.IgnoreFile)
}

// LeftOutNote says which linked worktrees of the project's own repository a
// generated build left out (imagebuild.ProjectSurvey.LeftOut), summarized as
// the gitignored warning is: a count and at most three places. It is a note,
// not a warning: nothing is wrong, and the image is the project without a
// second copy of itself. "" when none were.
func LeftOutNote(survey *imagebuild.ProjectSurvey) string {
	n := len(survey.LeftOut)
	if n == 0 {
		return ""
	}
	// Each is a directory: with its slash, one at the root is named as one.
	dirs := make([]string, n)
	for i, d := range survey.LeftOut {
		dirs[i] = d + "/"
	}
	return fmt.Sprintf("left out %s of this repository, in %s",
		agree(n, "1 git worktree", fmt.Sprintf("%d git worktrees", n)), placesOf(dirs, survey.Files))
}

// CosmosBoostWarning is the warning for the Cosmos Boost files an earlier
// Astro CLI wrote into the project that a declared Dockerfile's context
// carries (imagebuild.ProjectSurvey.CosmosBoost), or "" when there are none.
// A generated image leaves them out; a declared Dockerfile decides for itself
// what it copies, so the build goes ahead, but the plugin would read any it
// copies as current.
func CosmosBoostWarning(survey *imagebuild.ProjectSurvey) string {
	n := len(survey.CosmosBoost)
	if n == 0 {
		return ""
	}
	it := agree(n, "it", "them")
	return fmt.Sprintf("the build context carries %s an earlier Astro CLI wrote into the project: %s. The Cosmos Boost plugin would read %s as current if the Dockerfile copies %s into the image. This CLI no longer writes %s; delete %s",
		agree(n, "a Cosmos Boost file", "Cosmos Boost files"), cappedList(survey.CosmosBoost), it, it, it, it)
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
	if w := CosmosBoostWarning(&survey); w != "" {
		warn(w)
	}
	r := Check(dir, &survey)
	for _, w := range r.Warnings() {
		warn(w)
	}
	return &survey, r.Err()
}

// credentialURL matches a URL whose userinfo carries a credential: a
// password (user:pass@), or a user that starts with a known token prefix. A
// plain user, as in ssh://git@github.com or https://user@bitbucket.org, is
// not one; neither is an opaque token of an unknown kind used as a bare
// username, which this cannot tell from a user name and does not detect.
var credentialURL = regexp.MustCompile(`://(?:[^/@\s:]*:[^/@\s]+|(?:ghp_|gho_|ghu_|ghs_|ghr_|github_pat_|glpat-|x-access-token|oauth2)[^/@\s]*)@`)

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
	ignored, tracked, unanswered bool
}

// checkGit asks git about the carried files in the project, inProject
// mapping each to the project path git is asked about (locate). A symlink
// listed as itself, which only a declared build carries, ships as a link and
// is never a credential.
func (r *Report) checkGit(root string, files []string, inProject map[string]string) {
	verdicts := askProject(root, inProject, &r.Unanswered)
	for _, f := range files {
		v, ok := verdicts[f]
		if !ok {
			continue // outside the project, looked at in Check
		}
		secret := LooksSecret(f) && !isLink(root, f, inProject[f])
		if v.unanswered {
			if secret {
				r.Unchecked = append(r.Unchecked, f)
			}
			continue
		}
		if v.ignored {
			r.Gitignored = append(r.Gitignored, f)
		}
		if secret && (v.ignored || !v.tracked) {
			r.Secrets = append(r.Secrets, f)
		}
	}
}

// isLink reports whether f, asked about as itself (q == f), is a symlink: a
// declared build carries it as a link, not as what it points at.
func isLink(root, f, q string) bool {
	if q != f {
		return false
	}
	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(f)))
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

// locate maps each carried file to where it really is. Only a file that is,
// or sits under, one of links (the symlinks a generated build went through)
// can be anywhere but its listed path; the rest are in the project as
// listed. A linked file whose real path is in the project maps to that path
// in inProject, and one whose real path is outside it maps to that real path
// in outside. A linked file that cannot be resolved maps to its outermost
// link, so git is never asked about a path beyond one.
func locate(root string, files, links []string) (inProject, outside map[string]string) {
	inProject = make(map[string]string, len(files))
	outside = map[string]string{}
	for _, f := range files {
		link := outermostLink(f, links)
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
		outside[f] = resolved
	}
	return inProject, outside
}

// outermostLink is the shortest of links that is f or a parent of it, "" for
// none.
func outermostLink(f string, links []string) string {
	best := ""
	for _, l := range links {
		if (f == l || strings.HasPrefix(f, l+"/")) && (best == "" || len(l) < len(best)) {
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
