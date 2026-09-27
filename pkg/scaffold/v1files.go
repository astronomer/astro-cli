package scaffold

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/astronomer/astro-cli/pkg/envschema"
)

// A v1 Astro project states its shape in four files v2 replaces:
// requirements.txt (Python dependencies), packages.txt (OS packages), a
// Dockerfile whose image tag names the runtime it runs on, and
// .astro/env.schema.yaml (the environment the project expects). Reading them is
// what makes init in an existing project a CONVERSION rather than a scaffold
// beside files nobody looked at.
//
// The fourth is the desktop app's: the CLI has never written one. It is read
// here anyway, because a conversion run from a terminal and one run from the
// app have to produce the same project — and a file left behind by one of them
// is a second source for something the manifest now owns.
//
// Everything here reads. Nothing in this file writes or deletes — the decision
// to retire one of these files is planRetirements' in scaffold.go, and it is
// made after the manifest is rendered, because only then is it known what
// actually reached it.
//
// That decision was a separate one until O27 and 2c settled it, and the
// reasoning it replaced is worth keeping: the v1 files used to stay put so that
// a conversion the user had not reviewed was reversible by ignoring it. What
// replaces that guarantee is narrower and worth stating plainly — a file is
// removed only when everything it said is in the manifest, so ignoring the
// conversion is no longer how you undo it. The undo is the diff, which is why
// the desktop shows one before applying and why `astro init` now prints a
// Removed line per file.
//
// The rule for anything ambiguous is a note, never a guess. These files feed a
// preview a person approves, so "this line was not carried, and here is why" is
// a useful answer and a wrong dependency is not.

// v1Project is what the v1 files state. The zero value is a project that has
// none of them, which is the greenfield case and needs no special handling.
type v1Project struct {
	// dependencies are the requirement lines carried from requirements.txt, in
	// file order, minus anything that needed a note instead.
	dependencies []string
	// packages are the apt package names from packages.txt.
	packages []string
	// airflow is the Airflow version the Dockerfile's runtime tag names, or ""
	// when there was no Dockerfile or its tag said nothing usable.
	airflow string
	// statedVersion reports that a file named an Airflow version, whether or
	// not it could be used. It is what separates "this project never said"
	// from "this project said and we could not read it" when warning about a
	// defaulted pin.
	statedVersion bool
	// settings is what airflow_settings.yaml yielded: declarations that join
	// envSchema's, and the connection and variable values that go to the vault.
	settings carriedSettings
	// envSchema is what .astro/env.schema.yaml declared, split into what the
	// manifest grammar accepts and what it does not.
	envSchema carriedEnvSchema
	// notes is what could not be carried, with the reason.
	notes []string
	// present names the v1 files this directory actually has, in read order.
	//
	// Presence only. Whether a file can be RETIRED is decided by planRetirements
	// once the manifest has been rendered, and it cannot be decided here,
	// because the carrying happens later: mergeDependencies can drop every
	// requirement in the file (dynamic dependencies), keep the manifest's
	// specifier over this file's, or dedup two spellings of one distribution,
	// and pickAirflowVersion can rank an --airflow-version or an existing pin
	// above the Dockerfile's tag. An earlier version decided retirement from
	// parseRequirements' notes alone, and deleted a requirements.txt whose
	// contents had been carried nowhere — while the run's own notes said so.
	present []string
	// dockerfilePinOnly reports that the Dockerfile says nothing except which
	// runtime image to build on. A fact about the file's text, which is why it
	// is read here, and only half of whether the file can go: the other half is
	// whether its tag is the pin that won.
	dockerfilePinOnly bool
	// dockerfileBody is kept so a retirement can ask what the file references.
	// A Dockerfile that survives because it has a RUN may name requirements.txt
	// inside it, and deleting that file while keeping the build that reads it is
	// worse than either outcome alone.
	dockerfileBody []byte
	// projectName is the name .astro/config.yaml states, before any
	// sanitizing. Empty when the file is absent, says nothing, or will not
	// parse.
	//
	// The file itself is not retired, so this is read from a file that stays.
	projectName string
	// deployment is `project.deployment` from the same file: the target
	// `astro deploy --save` remembers, or empty when nothing saved one. The key
	// is a single string and always has been, so there is at most one.
	deployment string
	// workspace is `project.workspace` beside it. Never written by a command,
	// only by `astro config set`, so it is usually absent even when deployment
	// is not — but when it is there it is the other half of a manifest link.
	workspace string
}

// v1Config is the three fields of .astro/config.yaml this reads. The rest of
// the file is v1 CLI configuration that a v2 project does not carry.
//
// Deployment is `project.deployment`, and reading it is what keeps the note
// about it honest. The note used to fire on the file merely existing, which is
// every v1 project, since `astro dev init` wrote it, while the key it spoke
// about is set only by `astro deploy --save`, whose flag defaults to false. So
// a conversion was routinely told to move a Deployment the file did not name.
//
// Deployment and Workspace are `any` rather than `string` on purpose. A struct
// field types the whole decode: `deployment:` holding a map or a sequence makes
// yaml fail the file, and readV1Config's error path drops the name with it, so
// declaring these as strings would rename a project after its directory over a
// key the conversion does not need. Decoding loose and taking the value only
// when it is a scalar keeps a malformed deploy target from costing the name.
type v1Config struct {
	Project struct {
		Name       string `yaml:"name"`
		Deployment any    `yaml:"deployment"`
		Workspace  any    `yaml:"workspace"`
	} `yaml:"project"`
}

// yamlString is the value of a loosely-decoded scalar string, and "" for every
// other shape. Numbers and booleans are not ids, so they read as absent.
func yamlString(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

// v1ConfigRelPath is where a v1 project states its own name.
const v1ConfigRelPath = ".astro/config.yaml"

// readV1Project reads whatever v1 files dir has. A missing file is not an
// error: most of these are optional even in a v1 project.
//
// A file that exists and cannot be READ is an error, though. Silently treating
// an unreadable requirements.txt as an empty one would convert the project to a
// manifest declaring no dependencies, which installs nothing and fails at
// import time with a traceback that says nothing about this.
func readV1Project(dir string) (*v1Project, error) {
	v1 := &v1Project{}

	if data, err := readIfPresent(filepath.Join(dir, "requirements.txt")); err != nil {
		return nil, err
	} else if data != nil {
		deps, notes := parseRequirements(data)
		v1.dependencies, v1.notes = deps, append(v1.notes, notes...)
		if pinsAirflow(deps) {
			v1.statedVersion = true
		}
		v1.present = append(v1.present, "requirements.txt")
	}

	if data, err := readIfPresent(filepath.Join(dir, "packages.txt")); err != nil {
		return nil, err
	} else if data != nil {
		v1.packages = parsePackages(data)
		v1.present = append(v1.present, "packages.txt")
	}

	// The project's own name, which is the one thing in .astro/config.yaml a
	// v2 project keeps, and the Deployment id that decides whether this file
	// gets a note at all. Not added to present: the file holds v1 CLI
	// configuration this conversion neither reads nor replaces, so it is not a
	// candidate for retirement, and recording it there would offer it for
	// deletion.
	name, deployment, workspace, note, err := readV1Config(dir)
	if err != nil {
		return nil, err
	}
	v1.projectName, v1.deployment, v1.workspace = name, deployment, workspace
	if note != "" {
		v1.notes = append(v1.notes, note)
	}

	if data, err := readIfPresent(filepath.Join(dir, filepath.FromSlash(envschema.LegacyRelPath))); err != nil {
		return nil, err
	} else if data != nil {
		v1.envSchema = readEnvSchema(data)
		v1.notes = append(v1.notes, v1.envSchema.blockers...)
		v1.present = append(v1.present, envschema.LegacyRelPath)
	}

	if data, err := readIfPresent(filepath.Join(dir, SettingsRelPath)); err != nil {
		return nil, err
	} else if data != nil {
		v1.settings = readAirflowSettings(data)
		v1.notes = append(v1.notes, v1.settings.blockers...)
		v1.notes = append(v1.notes, v1.settings.notes()...)
		v1.present = append(v1.present, SettingsRelPath)
	}

	if data, err := readIfPresent(filepath.Join(dir, "Dockerfile")); err != nil {
		return nil, err
	} else if data != nil {
		version, stated, notes := airflowFromDockerfile(data)
		v1.airflow = version
		build := buildStepsNote(data)
		v1.notes = append(v1.notes, notes...)
		v1.notes = append(v1.notes, build...)
		if stated {
			v1.statedVersion = true
		}
		v1.present = append(v1.present, "Dockerfile")
		v1.dockerfilePinOnly = dockerfileIsPinOnly(data)
		v1.dockerfileBody = data
		// Two sources for one list, so say it rather than let someone find out.
		//
		// A declared Dockerfile is the build, and imagebuild ignores the
		// manifest's dependencies and packages in that mode, so requirements.txt
		// and packages.txt are KEPT (planRetirements spares them) because the
		// runtime base's ONBUILD reads them from the context. The manifest gets
		// the lists too, for the day the Dockerfile goes away. Both being live
		// at once is deliberate and is exactly the kind of thing that reads as a
		// conversion bug when nothing mentions it.
		if !v1.dockerfilePinOnly {
			for _, kept := range []string{"requirements.txt", "packages.txt"} {
				if slices.Contains(v1.present, kept) {
					v1.notes = append(v1.notes, kept+": kept, because your Dockerfile's base image reads it "+
						"during the build. Its contents are in pyproject.toml as well, which is what a project "+
						"without a Dockerfile installs from")
				}
			}
		}
		// A Dockerfile that is not pin-only and drew no note is kept and
		// unmentioned, which O27 says it must not be: the point of keeping it
		// is that the project still has a build the manifest does not describe,
		// and the user has to be told. buildStepsNote covers the RUN/COPY/ENV
		// shapes; this covers the rest — a second stage, an instruction no
		// regex here knows — where the file survives precisely because nothing
		// recognized it.
		if !v1.dockerfilePinOnly && len(notes) == 0 && len(build) == 0 {
			v1.notes = append(v1.notes,
				"Dockerfile: it does more than name a base image, so it was kept and declared as this "+
					"project's build. What it does is not described in pyproject.toml, which is the point of "+
					"declaring it")
		}
	}

	v1.mergeDeclarationSources()
	return v1, nil
}

// dockerfileIsPinOnly reports a Dockerfile that says nothing except which
// runtime image to build on, so the manifest's airflow pin can replace it whole.
//
// This is the test, and it is an allowlist of ONE thing rather than a denylist
// of instructions. The first version asked buildInstructionRe — the regex naming
// RUN, COPY, ENV and eleven others for the note it writes — and read "no match"
// as "nothing here". That is backwards for a decision that deletes. MAINTAINER
// is not in that list, so `FROM runtime:3.1-12` plus `MAINTAINER someone` matched
// nothing, produced no note, and the file was destroyed; every instruction Docker
// adds in future would do the same.
//
// So the question is inverted. Every line must be blank, a comment, or the one
// FROM, and anything this does not positively recognize keeps the file — which
// is what "conservative by construction" was supposed to mean and did not.
//
// It says nothing about whether the tag WON. A pin-only Dockerfile can still be
// the only record of a version the manifest did not take, so planRetirements
// checks that separately.
func dockerfileIsPinOnly(data []byte) bool {
	seenFrom := false
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(raw, "\r"))
		switch {
		case line == "", strings.HasPrefix(line, "#"):
			continue
		case strings.HasPrefix(strings.ToUpper(line), "FROM "):
			if seenFrom {
				// A second stage is a build, even with no RUN of its own.
				return false
			}
			seenFrom = true
		default:
			return false
		}
	}
	return seenFrom
}

// readV1Config reads the project's own name out of .astro/config.yaml, plus
// the deploy target and workspace that decide whether the file earns a note.
//
// A malformed file is not an error. v1's own loader tolerated one, nothing
// else in a conversion depends on this file, and failing `astro init` over
// unparseable YAML in a file being left behind anyway would be the least
// useful outcome available.
//
// It is not silent either, which is the pattern readAirflowSettings already
// set: a file that will not parse is a blocker, not an error. The name falls
// back to the directory and the run says why, rather than renaming somebody's
// project without comment.
//
// The PROJECT's config only, never the home one, though v1 resolved this key
// with a fallback to it. A global project.name would otherwise rename every
// project converted on that machine to the same thing.
func readV1Config(dir string) (name, deployment, workspace, note string, err error) {
	data, err := readIfPresent(filepath.Join(dir, filepath.FromSlash(v1ConfigRelPath)))
	if err != nil || data == nil {
		return "", "", "", "", err
	}
	var cfg v1Config
	if uerr := yaml.Unmarshal(data, &cfg); uerr != nil {
		return "", "", "", v1ConfigRelPath +
			": could not be read, so the project is named after its directory. " + uerr.Error(), nil
	}
	return strings.TrimSpace(cfg.Project.Name),
		yamlString(cfg.Project.Deployment), yamlString(cfg.Project.Workspace), "", nil
}

// readIfPresent returns nil bytes and no error when the file is not there.
//
// path is always a fixed filename joined onto the project directory the caller
// named, never a value out of a file.
func readIfPresent(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		return data, nil
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	default:
		return nil, err
	}
}

// parseRequirements carries the plain requirement lines and notes the rest.
//
// requirements.txt is a pip input format, not a list of PEP 508 requirements,
// and the difference is the whole reason this returns notes. A line naming
// another file, an index, or a hash means something pip does and pyproject.toml
// does not, so carrying it would produce a manifest that either fails to parse
// or quietly resolves differently. Each of those is reported and left in place.
func parseRequirements(data []byte) (deps, notes []string) {
	for _, line := range logicalLines(data) {
		switch {
		case line == "":
			continue
		case strings.HasSuffix(line, "\\"):
			// A logical line that still ends in a backslash is malformed: the
			// continuation had nothing to join to, or trailing whitespace broke
			// it. Emitting it produced a dependency whose text ended in a
			// backslash, which manifest.Parse accepts and uv then chokes on.
			notes = append(notes, "requirements.txt: "+line+
				" ends in a line continuation with nothing following it, so it was not migrated")
		case strings.HasPrefix(line, "-"):
			notes = append(notes, requirementOptionNote(line))
		case isBareURL(line):
			// pip accepts a bare URL or VCS reference as a requirement; PEP 508
			// needs it named ("pkg @ git+https://..."), and the name is not in
			// the line to take.
			notes = append(notes, "requirements.txt: "+line+
				" is a bare URL, which [project.dependencies] cannot express: add it as 'name @ "+line+"'")
		case isLocalPath(line):
			// pip installs a path as the project at that path. PEP 508 needs a
			// name and a file:// URL, plus a [tool.uv.sources] entry to make it
			// resolvable, and the name is in that directory's own metadata
			// rather than in this line. Same substance as the -e form, which is
			// already reported, so reporting only one of them was inconsistent.
			notes = append(notes, "requirements.txt: "+line+
				" is a local path, which [project.dependencies] cannot express: declare it under [tool.uv.sources]")
		default:
			deps = append(deps, line)
		}
	}
	return deps, notes
}

// requirementOptionNote explains one pip option that cannot cross into a
// manifest. The wording names the option rather than the whole line, so a long
// --extra-index-url does not bury the reason.
func requirementOptionNote(line string) string {
	opt, _, _ := strings.Cut(line, "=")
	opt, _, _ = strings.Cut(opt, " ")
	const prefix = "requirements.txt: "
	switch opt {
	case "-r", "--requirement":
		return prefix + line + " includes another requirements file, which was not read: convert it too, or inline its pins"
	case "-c", "--constraint":
		return prefix + line + " names a constraints file, which [project.dependencies] cannot express: pin the versions it constrains, or keep it for pip"
	case "-e", "--editable":
		return prefix + line + " is an editable install: declare it under [tool.uv.sources] or install it separately"
	case "--index-url", "-i", "--extra-index-url", "--find-links", "-f", "--trusted-host", "--no-index":
		return prefix + line + " names a package index, which [project.dependencies] cannot express: move it to [tool.uv] or your pip configuration"
	case "--hash":
		return prefix + line + " pins a hash, which [project.dependencies] cannot express: uv.lock records hashes instead"
	default:
		return prefix + line + " is a pip option, not a requirement, and was not migrated"
	}
}

// isBareURL reports a requirement given as a URL or VCS reference with no
// distribution name in front of it.
//
// The scheme is tested at the START of the line, and that is the whole test.
// An earlier version short-circuited on any '@' in the line, reasoning that '@'
// meant the PEP 508 "name @ url" form — but '@' is also how a VCS URL carries a
// ref, and `git+https://github.com/org/repo.git@main` is the most common way a
// requirements.txt names a dependency from git. It was carried into the manifest
// as an unparseable requirement with no note. A PEP 508 direct reference begins
// with the distribution NAME, never with a scheme, so a leading scheme is
// sufficient and needs no '@' rule at all.
func isBareURL(line string) bool {
	for _, p := range []string{"git+", "hg+", "bzr+", "svn+", "http://", "https://", "file://"} {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

// isLocalPath reports a requirement given as a filesystem path.
func isLocalPath(line string) bool {
	return line == "." || line == ".." ||
		strings.HasPrefix(line, "./") || strings.HasPrefix(line, "../") ||
		strings.HasPrefix(line, "/") || strings.HasPrefix(line, ".\\") ||
		filepath.IsAbs(line)
}

// parsePackages reads packages.txt: one apt package per line, comments and
// blanks ignored. No notes, because there is nothing here that cannot be
// carried — a line is a package name.
func parsePackages(data []byte) []string {
	var out []string
	for _, line := range logicalLines(data) {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// logicalLines splits pip/apt list input into meaningful lines: continuations
// joined, comments stripped, each result trimmed. A blank string in the result
// means a line that was empty or entirely a comment; callers skip it.
//
// Continuations are joined FIRST, then comments stripped, which is pip's order
// and was originally the other way round here. It matters both ways: pip joins
// `pandas  # note \` to the following line and reads one requirement, and a
// backslash with trailing whitespace after it is not a continuation at all, so
// stripping first turned `pandas \   ` into a dependency whose text ended in a
// backslash. A logical line that still ends in a backslash reaches the caller as
// one, which reports it rather than emitting it.
//
// The comment rule is pip's too: a '#' starts a comment at the beginning of a
// line or after whitespace. That matters because a '#' can be a URL fragment
// ("...@v1#egg=pkg"), where it is part of the requirement rather than the end
// of it.
func logicalLines(data []byte) []string {
	var out []string
	var pending string
	for _, raw := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		// A continuation is a backslash as the last character of the PHYSICAL
		// line, before any comment handling.
		if strings.HasSuffix(raw, "\\") {
			pending += strings.TrimSuffix(raw, "\\")
			continue
		}
		out = append(out, strings.TrimSpace(stripComment(pending+raw)))
		pending = ""
	}
	if pending != "" {
		// Ends in a continuation with no following line. Kept with its
		// backslash so the caller can say so.
		out = append(out, strings.TrimSpace(stripComment(pending))+"\\")
	}
	return out
}

// stripComment removes a pip-style trailing comment.
func stripComment(line string) string {
	for i, r := range line {
		if r != '#' {
			continue
		}
		if i == 0 {
			return ""
		}
		if prev := line[i-1]; prev == ' ' || prev == '\t' {
			return line[:i]
		}
	}
	return line
}

// fromLineRe finds a Dockerfile FROM instruction and captures the rest of the
// line, flags included. The flags are stripped afterwards rather than in the
// pattern, because `FROM --platform=linux/amd64 image:tag` is ordinary — it is
// the standard Apple-silicon workaround — and a pattern that captured \S+ took
// the flag as the image and read no version at all.
var fromLineRe = regexp.MustCompile(`(?im)^[ \t]*FROM[ \t]+(.+)$`)

// runtimeTagRe is the Astro Runtime tag format introduced with Airflow 3:
// "<airflow-major>.<airflow-minor>-<build>", as in "3.1-12", optionally with a
// flavor suffix ("3.1-12-base", "3.1-12.rc1").
//
// A deliberate second copy of airflow_versions.newFormatRegex, which cannot be
// imported here: this is a sub-module over pkg/*, and the root module already
// depends on it, so reaching back would be a cycle. Kept small on purpose, and
// pinned by TestRuntimeTagFormats so a change upstream shows up as a failure rather than
// as a project pinned to the wrong Airflow.
var runtimeTagRe = regexp.MustCompile(`^(\d+)\.(\d+)-\d+(?:[.-].+)?$`)

// oldRuntimeTagRe is the tag format Astro Runtime used before Airflow 3: a
// version like "12.1.0". It carries the RUNTIME version, a different namespace
// from the Airflow version, and tells us only that the project is on Airflow 2.
//
// Two widenings, both from review rather than from design, and both in the same
// direction: this copy was stricter than the code it mirrors, and every case it
// wrongly rejected became a real Airflow 2 project silently defaulted to
// Airflow 3.
//
//   - One to three segments, not three. airflow_versions accepts anything semver
//     accepts, and Go's semver takes "v9" and "v9.1" as readily as "v9.1.0".
//   - A flavor suffix. "9.1.0-base" and "9.1.0-python-3.10" are real published
//     tags, and semver reads the suffix as a prerelease, so upstream accepts
//     them. The asymmetry was visible in the code: runtimeTagRe carried a suffix
//     group and this did not.
//
// A leading "v" is trimmed before matching, mirroring stripVersionPrefix.
var oldRuntimeTagRe = regexp.MustCompile(`^\d+(\.\d+){0,2}(?:[.-].+)?$`)

// runtimeImageHint is what makes an image reference an Astro Runtime image.
//
// Substring rather than an exact repository, matching isAirflow3Runtime in the
// desktop: the published names are ".../runtime" and ".../astro-runtime", and an
// earlier hand-rolled check elsewhere matched only a path segment literally
// named "runtime" and missed the second.
const runtimeImageHint = "runtime"

// airflowFromDockerfile reads the Airflow version a v1 Dockerfile's runtime tag
// names. The second return reports that the Dockerfile stated a version at all,
// whether or not it could be used.
//
// The image is checked before the tag, and that is the whole point rather than a
// detail. Reading the tag alone made every version-shaped tag an Airflow 2
// image: `FROM apache/airflow:3.0.1` — the ordinary OSS Airflow Dockerfile, and
// exactly the repo shape adopt exists for — became airflow = "2", pinning an
// Airflow 3.0.1 project back a whole generation, and `FROM ubuntu:22.04` became
// Airflow 2 as well. A tag only means an Airflow version if the image it belongs
// to is an Astro Runtime image.
//
// Every stage is scanned and the LAST runtime stage wins. A multi-stage
// Dockerfile ordinarily starts `FROM python:3.11-slim AS builder`, so taking the
// first FROM read the builder: at best no version, and with a numeric builder
// tag ("alpine:3.19") a confident wrong one.
//
// The two tag formats are the remaining subtlety, and they are not
// interchangeable:
//
//   - "3.1-12" is the format Airflow 3 introduced, where the leading X.Y IS the
//     Airflow version. Fully derivable, offline.
//   - "12.1.0" is the older format, which names the RUNTIME version. The Airflow
//     minor is not in it. Recovering it means a reverse lookup through the
//     published release index, which is a network fetch — and Plan is offline by
//     contract, because it produces a preview.
//
// So an old tag yields "2": a pin naming no minor is legal and means "the newest
// Airflow 2" (pkg/imagebuild matches a partial pin by prefix), which is exactly
// what is known. It is honest rather than approximate, and a caller that wants
// the exact minor resolves the tag itself and passes Options.AirflowVersion,
// which wins over anything read here.
func airflowFromDockerfile(data []byte) (version string, stated bool, notes []string) {
	const prefix = "Dockerfile: "

	stages := fromLineRe.FindAllStringSubmatch(string(data), -1)
	if len(stages) == 0 {
		return "", false, []string{prefix + "no FROM instruction, so it names no Airflow version"}
	}

	// Last runtime stage wins. Anything else in the file is a builder.
	var ref, tag string
	for _, m := range stages {
		image, t := splitImageRef(m[1])
		if strings.Contains(image, runtimeImageHint) {
			ref, tag = image, t
		}
	}
	if ref == "" {
		// Not an Astro Runtime project. Naming the images keeps this actionable
		// rather than mysterious.
		return "", false, []string{prefix + "no stage builds on an Astro Runtime image (" +
			strings.Join(allImages(stages), ", ") + "), so it names no Airflow version"}
	}
	if tag == "" {
		return "", false, []string{prefix + "the image " + ref + " carries no tag, so it names no Airflow version"}
	}

	bare := strings.TrimPrefix(tag, "v")
	switch {
	case runtimeTagRe.MatchString(bare):
		p := runtimeTagRe.FindStringSubmatch(bare)
		return p[1] + "." + p[2], true, nil
	case oldRuntimeTagRe.MatchString(bare):
		return "2", true, []string{prefix + "runtime " + tag + " is an Airflow 2 image whose tag does not name the Airflow minor, " +
			"so the pin is \"2\", meaning the newest Airflow 2. Set it explicitly if this project needs a particular one"}
	default:
		return "", true, []string{prefix + "the tag " + tag + " is not an Astro Runtime version, so the Airflow version was not read from it"}
	}
}

// buildInstructionRe finds a Dockerfile instruction that is not FROM.
var buildInstructionRe = regexp.MustCompile(`(?im)^[ \t]*(RUN|COPY|ADD|ENV|ARG|USER|WORKDIR|ENTRYPOINT|CMD|VOLUME|EXPOSE|LABEL|HEALTHCHECK|SHELL|STOPSIGNAL|ONBUILD)\b`)

// buildStepsNote reports that the Dockerfile does more than name a base image.
//
// Reading the tag is not reading the file. A v1 Dockerfile commonly carries
// `RUN apt-get install -y unixodbc-dev`, a COPY of certificates, an ENV — real
// build customization that v2 does not perform, because there is no Dockerfile
// in the shape it produces. leftovers used to say so with an unconditional
// "Dockerfile: not read" entry; taking that out because the TAG is now read
// meant a Dockerfile that parsed cleanly went unmentioned entirely, which is a
// worse answer than the one it replaced.
//
// Only the instructions are named, never their arguments: the point is to say
// there is work here, and a COPY line's paths are the user's own to read.
func buildStepsNote(data []byte) []string {
	found := buildInstructionRe.FindAllStringSubmatch(string(data), -1)
	if len(found) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var kinds []string
	for _, m := range found {
		kind := strings.ToUpper(m[1])
		if !seen[kind] {
			seen[kind] = true
			kinds = append(kinds, kind)
		}
	}
	// "were not read" is the accurate half: nothing here parses a RUN or a COPY.
	// The old second half said "v2 builds no Dockerfile: move what they install
	// or set into pyproject.toml", which the dockerfile key made false — the file
	// IS the build now, so these instructions run, and moving them into the
	// manifest would be work for nothing. Saying so kept a v1-era sentence
	// giving v2 users the opposite of the right advice.
	return []string{"Dockerfile: its " + strings.Join(kinds, ", ") +
		" instructions were not read here, so the manifest does not describe them. The Dockerfile stays your build and they still run"}
}

// splitImageRef turns the text after FROM into an image reference and its tag,
// dropping build flags and a stage alias.
//
// `--platform=...` and any other `--flag` lead; `AS <name>` trails. Neither is
// part of the image, and both appear in ordinary Dockerfiles.
func splitImageRef(rest string) (image, tag string) {
	fields := strings.Fields(rest)
	ref := ""
	for _, f := range fields {
		if strings.HasPrefix(f, "--") {
			continue
		}
		ref = f
		break
	}
	if ref == "" {
		return "", ""
	}
	// A ':' in a registry host:port is not the tag separator, so only the last
	// path segment is split. The digest form (image@sha256:...) names no tag.
	seg := ref
	if i := strings.LastIndex(seg, "/"); i >= 0 {
		seg = seg[i+1:]
	}
	if i := strings.Index(seg, "@"); i >= 0 {
		seg = seg[:i]
	}
	_, t, found := strings.Cut(seg, ":")
	if !found {
		return ref, ""
	}
	// image keeps its full reference for messages; the tag is stripped off it.
	return strings.TrimSuffix(ref, ":"+t), t
}

// allImages lists the image references a Dockerfile builds on, for a message
// that has to explain why none of them was usable.
func allImages(stages [][]string) []string {
	out := make([]string, 0, len(stages))
	for _, m := range stages {
		if image, _ := splitImageRef(m[1]); image != "" {
			out = append(out, image)
		}
	}
	return out
}

// mergeDeclarationSources folds airflow_settings.yaml's declarations into the
// env schema's, so [tool.astro.env] has one producer no matter how many files
// fed it.
//
// A name both files declare is a BLOCKER for both, not a merge with a winner.
// The two spell different things — .astro/env.schema.yaml describes a value,
// airflow_settings.yaml supplies one — so a rule picking a winner would be this
// package deciding which of the user's two answers it prefers, silently, about
// a connection. Neither file is carried, both are kept and still authoritative,
// and the note names the collision so one pass fixes it.
//
// All-or-nothing per file survives this: a blocker from either side already
// emptied that side's schema before it got here.
func (v1 *v1Project) mergeDeclarationSources() {
	if v1.settings.schema == nil {
		return
	}
	if v1.envSchema.schema == nil {
		// Either there was no env schema file, or it was blocked. Blocked means
		// the manifest must not gain declarations that file also holds, so this
		// file cannot be carried either — and its VALUES must not be stored,
		// which is the half that is easy to miss. Storing them anyway writes a
		// project's credentials to the shared vault while the manifest declares
		// none of them: the inverse of what applySecrets exists to guarantee,
		// and silent.
		if len(v1.envSchema.blockers) > 0 {
			v1.dropSettingsCarry(SettingsRelPath + " was not carried either, and is kept as it is: " +
				envschema.LegacyRelPath + " has to be fixed first, because the two share [tool.astro.env]")
			return
		}
		v1.envSchema.schema = &envschema.Schema{}
	}
	dst := v1.envSchema.schema
	if dst.AirflowVariables == nil {
		dst.AirflowVariables = map[string]envschema.ValueSpec{}
	}
	if dst.Connections == nil {
		dst.Connections = map[string]envschema.ValueSpec{}
	}

	var clashes []string
	for _, sec := range []struct {
		what string
		src  map[string]envschema.ValueSpec
		dst  map[string]envschema.ValueSpec
	}{
		{"Airflow variable", v1.settings.schema.AirflowVariables, dst.AirflowVariables},
		{"connection", v1.settings.schema.Connections, dst.Connections},
	} {
		for _, name := range sortedSpecNames(sec.src) {
			if _, taken := sec.dst[name]; taken {
				clashes = append(clashes, sec.what+" "+name)
				continue
			}
			sec.dst[name] = sec.src[name]
		}
	}
	if len(clashes) > 0 {
		v1.envSchema = carriedEnvSchema{blockers: []string{
			SettingsRelPath + " and " + envschema.LegacyRelPath + " both declare " +
				strings.Join(clashes, ", ") + ". Neither file was carried, and both are kept as they are. " +
				"Remove the duplicate from one of them and convert again",
		}}
		v1.dropSettingsCarry(v1.envSchema.blockers[0])
		return
	}
	v1.envSchema.advisories = append(v1.envSchema.advisories, v1.settings.advisories...)
}

// dropSettingsCarry abandons airflow_settings.yaml entirely: no declarations,
// and no values.
//
// Both halves together, always. A path that clears one and not the other either
// writes credentials to the vault that nothing declares, or declares required
// connections whose values were never stored — and each leaves a project that
// cannot start for a reason nothing on screen explains. The pools survive
// because the note about them is about the file, which is being kept.
func (v1 *v1Project) dropSettingsCarry(reason string) {
	v1.settings = carriedSettings{blockers: []string{reason}, pools: v1.settings.pools}
	v1.notes = append(v1.notes, reason)
}
