package scaffold

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/astronomer/astro-cli/pkg/airflowrt"
	"github.com/astronomer/astro-cli/pkg/manifest"
	"github.com/astronomer/astro-cli/pkg/manifest/tomledit"
)

// A 1.x Dockerfile commonly installs a package or two the requirements file
// could not name, a private one from git behind the netrc secret most often:
//
//	USER root
//	RUN --mount=type=secret,id=netrc,target=/root/.netrc \
//	    pip install --no-cache-dir "corona @ git+https://github.com/org/corona.git@4bfeaf8"
//	USER astro
//
// Left in the Dockerfile, the image has the package and nothing else does:
// standalone mode, astro local check and the editor's .venv install from
// pyproject.toml. So astro init, and only when its caller asks
// (Options.MovePipSteps), moves such steps into [project] dependencies: the
// install base installs them from uv.lock, pinned to what the step named, with
// the netrc secret the step mounted. The caller then has to lock: a package
// that uv cannot resolve on this machine (a private repository it cannot
// reach) is put back where it was (Result.RevertMovedPackages), so a move
// never leaves a project that cannot lock.
//
// Only what can be moved without a guess moves. A step moves when its whole
// command is one pip install of requirement specs with options that change
// nothing about what is installed, and it is in the leading run of the final
// stage's own steps, where only USER and other such steps come before it: the
// install base installs ahead of every step the Dockerfile has, so a pip
// install after an apt-get it may need would run without it. Its only build
// secret may be netrc, the one the install base mounts.

// MovedPackage is a requirement astro init moved from a Dockerfile RUN step
// into [project] dependencies.
type MovedPackage struct {
	// Spec is the requirement as the step named it.
	Spec string `json:"spec"`
	// Line is the Dockerfile line, from 1, of the step that installed it.
	Line int `json:"line"`
}

// pipStepMove is what a run that moved pip steps needs to put them back
// (Result.RevertMovedPackages): the Dockerfile as it is without the move, and
// what the move changed in the manifest and in the Result.
type pipStepMove struct {
	// dockerfile is the rewritten Dockerfile with its pip steps still in it.
	dockerfile []byte
	// dropped reports that the move removed the Dockerfile and its
	// [tool.astro] dockerfile declaration, since it then built what a
	// generated build does.
	dropped bool
	// specs are the requirements added to [project] dependencies.
	specs []string
	// notesBefore are the notes the Dockerfile had without the move, and
	// notesAfter the ones the move gave instead.
	notesBefore, notesAfter []string
	// advisories say what moved; updatedBefore and updatedAfter are the
	// Dockerfile's Updated label without and with the move.
	advisories                  []string
	updatedBefore, updatedAfter string
	// deleted is the Dockerfile's Deleted label when the move removed it.
	deleted string
	// ignore is the .dockerignore change a Dockerfile the move removed would
	// have had, which a revert makes along with putting the file back.
	ignore *Change
	// addedSecret reports that the move added 'id=netrc' to [tool.astro]
	// build-secrets, for the netrc secret the moved steps mounted.
	addedSecret bool
}

// dockerInstr is one entry of a Dockerfile's lines: an instruction with its
// continuation lines, a comment, or a blank line. first and last index the
// lines it spans.
type dockerInstr struct {
	first, last int
	// keyword is the instruction's, upper-cased, and rest what follows it,
	// continuation lines joined; both empty for a comment or a blank line.
	keyword, rest  string
	comment, blank bool
	// messy is an instruction whose continuation has a comment or a blank line
	// inside it, which docker drops and this does not try to.
	messy bool
}

// dockerInstrs splits lines into instructions, comments and blank lines.
func dockerInstrs(lines []string) []dockerInstr {
	var out []dockerInstr
	for i := 0; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		switch {
		case t == "":
			out = append(out, dockerInstr{first: i, last: i, blank: true})
			continue
		case strings.HasPrefix(t, "#"):
			out = append(out, dockerInstr{first: i, last: i, comment: true})
			continue
		}
		in := dockerInstr{first: i}
		var text strings.Builder
		for {
			line := strings.TrimRight(lines[i], " \t\r")
			cont := strings.HasSuffix(line, "\\")
			text.WriteString(strings.TrimSuffix(line, "\\"))
			if !cont || i+1 >= len(lines) {
				break
			}
			i++
			if n := strings.TrimSpace(lines[i]); n == "" || strings.HasPrefix(n, "#") {
				in.messy = true
			}
			text.WriteString(" ")
		}
		in.last = i
		keyword, rest, _ := strings.Cut(strings.TrimSpace(text.String()), " ")
		in.keyword, in.rest = strings.ToUpper(keyword), strings.TrimSpace(rest)
		out = append(out, in)
	}
	return out
}

// movablePipSteps finds the steps among a final stage's own lines that move
// into [project] dependencies. own are the lines after the stage's FROM, and
// offset the index of own[0] in the file. It returns own without them (and
// without the comments directly above each, and a USER root … USER astro pair
// that wrapped only them), what moved, and whether a moved step mounted the
// netrc secret.
func movablePipSteps(own []string, offset int) (rest []string, moved []MovedPackage, netrc bool) {
	instrs := dockerInstrs(own)
	drop := make([]bool, len(instrs))
	for i, in := range instrs {
		if in.comment || in.blank || in.keyword == instrUser {
			continue
		}
		specs, mounts, ok := pipOnlyRun(in)
		if !ok {
			break
		}
		netrc = netrc || mounts
		drop[i] = true
		dropCommentsAbove(instrs, drop, i)
		for _, s := range specs {
			moved = append(moved, MovedPackage{Spec: s, Line: offset + in.first + 1})
		}
	}
	if len(moved) == 0 {
		return own, nil, false
	}
	dropEmptyRootPairs(instrs, drop)
	return keptLines(own, instrs, drop), moved, netrc
}

// instrUser is the instruction that sets the user later steps run as.
const instrUser = "USER"

// dropCommentsAbove marks the comment lines directly above instrs[i], which
// are about it, to go with it.
func dropCommentsAbove(instrs []dockerInstr, drop []bool, i int) {
	for j := i - 1; j >= 0 && instrs[j].comment && instrs[j].last+1 == instrs[j+1].first; j-- {
		drop[j] = true
	}
}

// dropEmptyRootPairs marks a USER root … USER astro pair whose steps all go:
// it has nothing left to run as root.
func dropEmptyRootPairs(instrs []dockerInstr, drop []bool) {
	for a := range instrs {
		if instrs[a].keyword != instrUser || drop[a] || !isRootUser(instrs[a].rest) {
			continue
		}
		b, wrapped := a+1, false
		for ; b < len(instrs); b++ {
			if instrs[b].comment || instrs[b].blank {
				continue
			}
			if !drop[b] {
				break
			}
			wrapped = true
		}
		if !wrapped || b == len(instrs) || instrs[b].keyword != instrUser || strings.TrimSpace(instrs[b].rest) != "astro" {
			continue
		}
		for j := a; j <= b; j++ {
			drop[j] = true
		}
		dropCommentsAbove(instrs, drop, a)
	}
}

// keptLines are own's lines without the instructions drop marks, with one
// blank line where a removed step leaves two and none leading.
func keptLines(own []string, instrs []dockerInstr, drop []bool) (rest []string) {
	blank := false
	for i, in := range instrs {
		if drop[i] {
			continue
		}
		if in.blank {
			// One blank line where a removed step leaves two.
			if blank || len(rest) == 0 {
				continue
			}
			blank = true
		} else {
			blank = false
		}
		rest = append(rest, own[in.first:in.last+1]...)
	}
	return rest
}

// isRootUser reports a USER instruction's argument naming root.
func isRootUser(arg string) bool {
	user, _, _ := strings.Cut(strings.TrimSpace(arg), ":")
	return user == "root" || user == "0"
}

// pipQuietFlags are the pip install options a moved step may carry: none of
// them changes what is installed.
var pipQuietFlags = []string{"--no-cache-dir", "--no-cache", "-q", "--quiet", "--disable-pip-version-check", "--system"}

// secretMountRe is a RUN --mount of a build secret.
var secretMountRe = regexp.MustCompile(`^--mount=(.*)$`)

// pipOnlyRun returns the requirement specs a RUN instruction installs when its
// whole command is one pip install of them (pipQuietFlags aside), and its only
// flags mount the netrc secret, which netrc reports.
func pipOnlyRun(in dockerInstr) (specs []string, netrc, ok bool) {
	if in.keyword != "RUN" || in.messy {
		return nil, false, false
	}
	cmd := in.rest
	for strings.HasPrefix(cmd, "--") {
		flag, after, _ := strings.Cut(cmd, " ")
		m := secretMountRe.FindStringSubmatch(flag)
		if len(m) < 2 || !netrcSecretMount(m[1]) {
			return nil, false, false
		}
		netrc = true
		cmd = strings.TrimSpace(after)
	}
	words, ok := plainWords(cmd)
	if !ok {
		return nil, false, false
	}
	switch {
	case len(words) > 0 && words[0] == "uv":
		words = words[1:]
	case len(words) > 1 && pythonCommandRe.MatchString(words[0]) && words[1] == "-m":
		words = words[2:]
	}
	if len(words) < 3 || !pipCommandRe.MatchString(words[0]) || words[1] != "install" {
		return nil, false, false
	}
	for i := 2; i < len(words); i++ {
		w := words[i]
		switch {
		case slices.Contains(pipQuietFlags, w):
		case strings.HasPrefix(w, "--root-user-action="):
		case w == "--root-user-action":
			i++
		case strings.HasPrefix(w, "-"), !isPackageSpec(w), manifest.NamesAirflow(w), manifest.DistName(w) == "":
			return nil, false, false
		default:
			specs = append(specs, w)
		}
	}
	return specs, netrc, len(specs) > 0
}

// netrcSecretMount reports a --mount value that mounts the build secret netrc,
// the one secret the install base mounts too.
func netrcSecretMount(value string) bool {
	kv := map[string]string{}
	for _, part := range strings.Split(value, ",") {
		k, v, _ := strings.Cut(part, "=")
		switch k {
		case "type", "id", "target", "dst", "destination", "required", "mode", "uid", "gid":
			kv[k] = v
		default:
			return false
		}
	}
	return kv["type"] == "secret" && kv["id"] == manifest.RuntimeSecretID
}

// shellSyntax are the characters outside quotes that make a shell command
// more than its words: escapes, chaining, redirection, subshells, expansion
// and globbing. quotedExpansion are the ones that still expand inside double
// quotes.
const (
	shellSyntax     = "\\;&|<>()`$*?{}"
	quotedExpansion = "$`\\"
)

// plainWords splits a shell command into its words, quotes removed, stopping
// at a comment. It reports false for anything the shell would expand, chain or
// redirect, since the step is then more than its words.
func plainWords(s string) ([]string, bool) {
	var words []string
	var cur strings.Builder
	inWord := false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '#' && !inWord:
			i = len(s)
		case strings.IndexByte(shellSyntax, c) >= 0:
			return nil, false
		case c == '\'' || c == '"':
			end := strings.IndexByte(s[i+1:], c)
			if end < 0 {
				return nil, false
			}
			quoted := s[i+1 : i+1+end]
			if c == '"' && strings.ContainsAny(quoted, quotedExpansion) {
				return nil, false
			}
			cur.WriteString(quoted)
			inWord = true
			i += end + 1
		case c == ' ' || c == '\t':
			if inWord {
				words = append(words, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteByte(c)
			inWord = true
		}
	}
	if inWord {
		words = append(words, cur.String())
	}
	return words, true
}

// hasInstruction reports any instruction among lines.
func hasInstruction(lines []string) bool {
	return slices.ContainsFunc(dockerInstrs(lines), func(in dockerInstr) bool { return !in.comment && !in.blank })
}

// planPipStepMoves moves the rewritten Dockerfile's pip steps into the
// manifest the run is about to write, out, when the caller asked for it
// (Options.MovePipSteps) and every one of them is a package the manifest does
// not already name. The Dockerfile change becomes the file without the steps,
// or, when that builds nothing a generated build does not, the file goes and
// its declaration with it. notes are the run's notes so far, from which the
// Dockerfile's step notes are swapped for the ones the move leaves. Anything
// that stops the move (a package the manifest names, a spec it cannot hold)
// leaves the steps where they are, with their notes: not moving is always
// an answer.
func planPipStepMoves(move bool, from1x *project1x, out *[]byte, cs *Changeset, notes []string) {
	rw := from1x.rewrite
	if !move || rw == nil || len(rw.moved) == 0 {
		return
	}
	ed, err := tomledit.NewSurgical(*out)
	if err != nil {
		return
	}
	have := map[string]bool{}
	for _, d := range asStrings(mustGet(ed, "project", "dependencies")) {
		have[manifest.DistName(d)] = true
	}
	var specs []string
	for _, m := range rw.moved {
		name := manifest.DistName(m.Spec)
		if have[name] {
			// The manifest's own entry would win and the step's pin would be
			// lost with the step, so nothing moves.
			return
		}
		have[name] = true
		specs = append(specs, m.Spec)
	}
	at := arrayLen(mustGet(ed, "project", "dependencies"))
	for i, s := range specs {
		if ed.Set([]string{"project", "dependencies", fmt.Sprint(at + i)}, s) != nil {
			return
		}
	}

	mv := &pipStepMove{dockerfile: rw.body, specs: specs, notesBefore: from1x.stepNotes}
	mv.notesAfter = stepNotes(rw.bodyMoved, true)
	if rw.emptyAfterMove {
		// What is left after the steps' notes went names the file only if
		// something else about it is still to do, and then it stays.
		rest := slices.DeleteFunc(slices.Clone(notes), func(n string) bool { return slices.Contains(from1x.stepNotes, n) })
		mv.dropped = !namedInAny(rest, fileDockerfile)
	}
	if mv.dropped {
		mv.notesAfter = nil
		ed.Delete([]string{"tool", "astro", manifestKeyDockerfile})
	}
	if rw.movedNetrc && !namesNetrcSecret(asStrings(mustGet(ed, "tool", "astro", manifestKeyBuildSecrets))) {
		// The steps mounted netrc themselves; now the install base does,
		// and only when the build is given it. A source-less entry has the
		// build read ~/.netrc, or else git's credentials for the hosts the
		// packages come from, as uv lock reached them.
		secrets := append(asStrings(mustGet(ed, "tool", "astro", manifestKeyBuildSecrets)), bareNetrcSecret)
		if ed.Set([]string{"tool", "astro", manifestKeyBuildSecrets}, asAny(secrets)) != nil {
			return
		}
		mv.addedSecret = true
	}
	data, err := ed.Bytes()
	if err != nil {
		return
	}
	if _, err := loadable(nil, data); err != nil {
		// A spec the manifest cannot hold stays in its step.
		return
	}
	*out = data

	for i := range cs.Changes {
		c := &cs.Changes[i]
		if c.Path != fileDockerfile || c.Kind != UpdateFile {
			continue
		}
		mv.updatedBefore = c.Labels[0]
		if mv.dropped {
			cs.Changes = slices.Delete(cs.Changes, i, i+1)
		} else {
			c.Content = rw.bodyMoved
			c.Labels = []string{fileDockerfile + " (now builds on the install base the Astro CLI makes from " + manifest.Marker +
				" and uv.lock, which installs what its pip steps did)"}
			mv.updatedAfter = c.Labels[0]
		}
		break
	}
	// The lines are in MovedPackages, for json; the text says what moved, not
	// where from.
	moved := manifest.Marker + ": moved in " + joinNames(specNames(specs)) + " from the Dockerfile's pip steps"
	if mv.dropped {
		from1x.dropDockerfile = true
	}
	mv.advisories = []string{moved}
	if mv.addedSecret {
		mv.advisories = append(mv.advisories, manifest.Marker + ": added the netrc build secret, read from ~/.netrc or your git credentials")
	}
	from1x.notes = slices.DeleteFunc(from1x.notes, func(n string) bool { return slices.Contains(from1x.stepNotes, n) })
	from1x.notes = append(from1x.notes, mv.notesAfter...)
	cs.Advisories = append(cs.Advisories, mv.advisories...)
	cs.MovedPackages = rw.moved
	cs.pipMove = mv
}

// specNames are the distribution names of specs, for a sentence.
func specNames(specs []string) []string {
	out := make([]string, len(specs))
	for i, s := range specs {
		out[i] = manifest.DistName(s)
	}
	return out
}

// bareNetrcSecret is the build-secrets entry for a netrc the build supplies
// itself: ~/.netrc, or git's credentials for the hosts it installs from.
const bareNetrcSecret = "id=" + manifest.RuntimeSecretID

// namesNetrcSecret reports build-secrets that already give the netrc secret.
func namesNetrcSecret(secrets []string) bool {
	return slices.ContainsFunc(secrets, func(spec string) bool {
		s, err := manifest.ParseBuildSecret(spec)
		return err == nil && s.ID == manifest.RuntimeSecretID
	})
}

// netrcSecretSpec is the build-secrets entry for the netrc secret, read from
// the file it conventionally lives in.
const netrcSecretSpec = "'id=netrc,src=~/.netrc'" //nolint:gosec // where the secret is read from, not a secret

// stepNotes are the notes about a Dockerfile's own steps: the steps that stay,
// the packages its RUN steps install that the manifest does not, and the build
// secrets it mounts. rewritten is a file this run moved onto the install base,
// whose own lines (the base's ARG, FROM and COPY) are the CLI's and not
// counted.
func stepNotes(data []byte, rewritten bool) []string {
	if rewritten {
		var own []string
		for _, line := range strings.Split(string(data), "\n") {
			if t := strings.TrimSpace(line); strings.HasPrefix(t, "ARG ") && strings.Contains(t, "ASTRO_BASE") ||
				strings.HasPrefix(t, "FROM ") || t == airflowrt.ProjectCopy {
				continue
			}
			own = append(own, line)
		}
		data = []byte(strings.Join(own, "\n"))
	}
	notes := buildStepsNote(data)
	notes = append(notes, pipInstallNote(data)...)
	return append(notes, buildSecretNotesOf(data)...)
}

// RevertMovedPackages puts back the pip steps a run moved from the Dockerfile
// into [project] dependencies (MovedPackages), because the lock the caller
// ran with them failed: why says so, for the notes. The Dockerfile is put back
// first and the manifest edited second, so a run that dies in between leaves
// both installing the packages, which builds, rather than neither.
func (r *Result) RevertMovedPackages(why string) error {
	mv := r.pipMove
	if mv == nil {
		return nil
	}
	kind := UpdateFile
	if mv.dropped {
		kind = CreateFile
	}
	c := Change{Kind: kind, Path: fileDockerfile, Content: mv.dockerfile}
	if err := c.apply(r.Dir); err != nil {
		return err
	}
	err := EditManifest(r.Dir, nil, func(_ *manifest.Manifest, ed tomledit.Editor) error {
		deps := slices.DeleteFunc(asStrings(mustGet(ed, "project", "dependencies")), func(d string) bool {
			return slices.Contains(mv.specs, d)
		})
		if err := ed.Set([]string{"project", "dependencies"}, asAny(deps)); err != nil {
			return err
		}
		if mv.addedSecret {
			secrets := slices.DeleteFunc(asStrings(mustGet(ed, "tool", "astro", manifestKeyBuildSecrets)), func(s string) bool { return s == bareNetrcSecret })
			if len(secrets) == 0 {
				ed.Delete([]string{"tool", "astro", manifestKeyBuildSecrets})
			} else if err := ed.Set([]string{"tool", "astro", manifestKeyBuildSecrets}, asAny(secrets)); err != nil {
				return err
			}
		}
		if mv.dropped {
			return ed.Set([]string{"tool", "astro", manifestKeyDockerfile}, fileDockerfile)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("putting the pip steps back in %s: %w", filepath.Join(r.Dir, fileDockerfile), err)
	}
	if mv.ignore != nil {
		if err := mv.ignore.apply(r.Dir); err != nil {
			return err
		}
		if mv.ignore.Kind == CreateFile {
			r.Created = append(r.Created, mv.ignore.Labels...)
		} else {
			r.Updated = append(r.Updated, mv.ignore.Labels...)
		}
	}
	r.Notes = slices.DeleteFunc(r.Notes, func(n string) bool { return slices.Contains(mv.notesAfter, n) })
	r.Notes = append(r.Notes, fileDockerfile+": its pip steps stay, because uv lock could not resolve them here ("+why+")")
	r.Notes = append(r.Notes, mv.notesBefore...)
	r.Advisories = slices.DeleteFunc(r.Advisories, func(a string) bool { return slices.Contains(mv.advisories, a) })
	r.Deleted = slices.DeleteFunc(r.Deleted, func(d string) bool { return d == mv.deleted })
	for i, u := range r.Updated {
		if u == mv.updatedAfter && mv.updatedAfter != "" {
			r.Updated[i] = mv.updatedBefore
		}
	}
	if mv.dropped {
		r.Updated = append(r.Updated, mv.updatedBefore)
	}
	r.MovedPackages = nil
	r.pipMove = nil
	return nil
}
