package localstandalone

// UVOptions are the consumer's preferences for the venv provisioning a start
// does. Every field is optional; the zero value is the CLI's own behavior.
//
// Untagged on purpose. Nothing in it is OS-dependent, and while it lived in the
// Unix implementation the Windows stub carried a hand-synced copy — so adding a
// field compiled everywhere the author looked and broke a build only CI's
// Windows job runs. One definition makes that impossible rather than merely
// remembered.
type UVOptions struct {
	HermeticEnv    bool
	OnCertFallback func()

	// BinDir is where to look for uv before PATH. For an embedder that ships
	// its own copy: the desktop bundles one inside its .app precisely so a user
	// who has never installed uv can still build an environment, and without
	// this the search falls through to PATH and the three installer locations
	// and never finds it.
	BinDir string

	// CacheDir overrides the shared cache under the astro cache root. An
	// embedder that already has a cache wants both to be the same one, or a
	// Python toolchain and an Airflow wheel are downloaded twice — once for
	// whatever it provisions itself, once for whatever it provisions through
	// here. Empty keeps the default.
	//
	// Must be absolute, and is rejected otherwise. uv runs with its working
	// directory set to the project, so a relative path would resolve per
	// project and produce exactly the per-project caches this exists to avoid
	// — silently, since uv would create each one quite happily.
	//
	// Creating it is the embedder's, not this package's: uv makes it at its own
	// default mode on first use, and a caller that wants a particular mode has
	// to get there first, because MkdirAll never revises the mode of one that
	// already exists.
	CacheDir string

	// NoConfig passes --no-config, so uv ignores uv.toml and any [tool.uv]
	// table it would otherwise discover from the project directory upwards.
	//
	// Off by default, deliberately: a uv a user configured for their own shell
	// is theirs, and the CLI honors it. An embedder that supplies python,
	// dependencies and constraints itself owns the whole input and can only be
	// contradicted — most concretely by an exclude-newer that filters out the
	// build the caller pinned. Same argument as HermeticEnv, which covers the
	// environment while this covers the files.
	//
	// The cost is not only the user's config, and it is easy to read this as
	// if it were: --no-config also discards the PROJECT's own [tool.uv] table.
	// A project pinning a private index there resolves against public PyPI
	// instead, and nothing in the failure names the flag. pkg/uv's
	// Options.NoConfig says the same; do not set this to overrule an ambient
	// uv without accepting that.
	NoConfig bool
}
