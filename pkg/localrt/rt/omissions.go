package rt

// OmissionKind names something a project declares that standalone mode cannot
// honor. The values are stable strings, so a consumer can key its own
// (translated) copy off them or carry them in machine-readable output.
type OmissionKind string

const (
	// OmissionDockerfile is a declared [tool.astro] dockerfile. Standalone mode
	// builds no image, so nothing that file installs or copies is applied.
	OmissionDockerfile OmissionKind = "dockerfile"
	// OmissionPackages is a non-empty [tool.astro] packages list. Standalone
	// mode has no image to install OS packages into.
	OmissionPackages OmissionKind = "packages"
	// OmissionComposeOverride is a ComposeOverrideFile in the project
	// directory. Standalone mode runs no compose project to merge it into.
	OmissionComposeOverride OmissionKind = "compose_override"
)

// Omission is one thing a Plan declares that its mode will not apply. It
// carries data, not a sentence: each consumer words it for its own surface,
// in its own language.
type Omission struct {
	Kind OmissionKind
	// Dockerfile is the declared project-relative path. Set only for
	// OmissionDockerfile.
	Dockerfile string
	// Packages are the declared OS package names. Set only for
	// OmissionPackages.
	Packages []string
}

// StandaloneOmissions lists what this plan declares that standalone mode
// cannot honor, most consequential first: a declared Dockerfile (which is the
// whole build) before OS packages (one line of it), then a compose override
// file. A project can declare all three, and then all three are reported.
//
// The override is the one entry not read off the plan: it is a file in
// ProjectPath, which this checks for.
//
// Docker mode honors all three, so a Docker-mode plan has none. The zero Mode is
// treated as standalone, because that is what Start runs it as.
//
// Start does not report these itself; the runtime has no channel for a
// warning. Whoever starts the plan calls this and tells its user.
func (p Plan) StandaloneOmissions() []Omission {
	if p.Mode == ModeDocker {
		return nil
	}
	var out []Omission
	if p.Dockerfile != "" {
		out = append(out, Omission{Kind: OmissionDockerfile, Dockerfile: p.Dockerfile})
	}
	if len(p.Packages) > 0 {
		out = append(out, Omission{Kind: OmissionPackages, Packages: append([]string(nil), p.Packages...)})
	}
	if p.ProjectPath != "" && HasComposeOverride(p.ProjectPath) {
		out = append(out, Omission{Kind: OmissionComposeOverride})
	}
	return out
}
