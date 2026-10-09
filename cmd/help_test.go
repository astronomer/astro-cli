package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/internal/telemetry"
)

const testHelpWidth = 80

// pinHelpWidth renders help at a fixed width, not the terminal the test runs in.
func pinHelpWidth(t *testing.T) {
	t.Helper()
	orig := helpWidth
	helpWidth = func() int { return testHelpWidth }
	t.Cleanup(func() { helpWidth = orig })
}

// helpTree is a small root shaped like the real one: a group, a platform
// command with grouped flags and an example, and an offline command that
// skips the platform pre-run.
func helpTree() (root, offline *cobra.Command) {
	root = &cobra.Command{Use: "astro", Long: "The root."}
	root.PersistentFlags().String(verbosityFlag, "warning", "Log level")
	root.AddGroup(&cobra.Group{ID: "ship", Title: "Ship:"})

	deploy := &cobra.Command{
		Use:     "deploy [DEPLOYMENT_ID]",
		Short:   "Ship this project's code to a Deployment",
		Long:    strings.Repeat("Deploy words wrap here. ", 12),
		Example: "\n  # Deploy to the linked Deployment\n  astro deploy\n",
		GroupID: "ship",
		Run:     func(*cobra.Command, []string) {},
		Annotations: map[string]string{
			flagGroupOrderAnnotation: "Image,Test",
		},
	}
	deploy.Flags().Bool("force", false, "Deploy even with uncommitted changes")
	deploy.Flags().String("test", "", "Pytest file to run first")
	deploy.Flags().String("image-name", "", "Image to deploy")
	_ = deploy.Flags().SetAnnotation("test", flagGroupAnnotation, []string{"Test"})
	_ = deploy.Flags().SetAnnotation("image-name", flagGroupAnnotation, []string{"Image"})

	offline = &cobra.Command{
		Use:         "local",
		Short:       "Run Apache Airflow locally from your project",
		Aliases:     []string{"lo", "locally-running-airflow-instance"},
		Run:         func(*cobra.Command, []string) {},
		Annotations: map[string]string{telemetry.SkipPreRunAnnotation: "true"},
	}
	offline.Flags().StringP("output", "o", "text", "Output format: text or json")

	root.AddCommand(deploy, offline)
	installHelp(root, cloudPlatform, "")
	return root, offline
}

func renderHelp(t *testing.T, root *cobra.Command, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(append(args, "--help"))
	require.NoError(t, root.Execute())
	return out.String()
}

func TestHelpPageSections(t *testing.T) {
	pinHelpWidth(t)
	root, _ := helpTree()
	page := renderHelp(t, root, "deploy")

	for _, line := range strings.Split(page, "\n") {
		assert.LessOrEqual(t, len(line), testHelpWidth, "line wider than the help width: %q", line)
	}
	assert.Contains(t, page, "Show help for this command", "the -h description is the house one, not cobra's")
	assert.NotContains(t, page, "--verbosity", "a child page leaves the root's --verbosity out")
	assert.Contains(t, page, "Current context: ", "a platform command says which context it acts in")

	// Ungrouped flags, then the groups in the order the command declared,
	// then examples — after the flags they use.
	order := []string{"Usage:", "Flags:", "Image Flags:", "Test Flags:", "Examples:", "Current context:"}
	last := -1
	for _, heading := range order {
		at := strings.Index(page, heading)
		require.GreaterOrEqual(t, at, 0, "missing %q in:\n%s", heading, page)
		assert.Greater(t, at, last, "%q is out of order in:\n%s", heading, page)
		last = at
	}
	assert.Contains(t, page, "Examples:\n  # Deploy", "examples start right under their heading")
}

func TestRootHelpKeepsVerbosityAndGroups(t *testing.T) {
	pinHelpWidth(t)
	root, _ := helpTree()
	page := renderHelp(t, root)

	assert.True(t, strings.HasPrefix(page, rootBanner), "the root page opens with the banner")
	assert.Contains(t, page, "--verbosity")
	assert.Contains(t, page, "Ship:\n  deploy ")
	// cobra adds help and completion beside it when the root executes, and
	// whether they sort ahead of it is cobra.EnableCommandSorting's, a global
	// other tests set; so only membership of the section is asserted.
	_, additional, found := strings.Cut(page, "Additional Commands:")
	require.True(t, found, page)
	additional, _, _ = strings.Cut(additional, "\n\n")
	assert.Contains(t, additional, "\n  local, lo, locally-running-airflow-instance\n")
}

func TestOfflineCommandHasNoContextLine(t *testing.T) {
	pinHelpWidth(t)
	root, _ := helpTree()
	page := renderHelp(t, root, "local")
	assert.NotContains(t, page, "Current context:", "the core tree reads the same in every context")
	assert.Contains(t, page, "Aliases:\n  local, lo, locally-running-airflow-instance")
}

func TestCommandRowPutsWideSpellingsOnTheirOwnLine(t *testing.T) {
	_, offline := helpTree()
	row := commandRow(offline, 12, testHelpWidth)
	lines := strings.Split(row, "\n")
	require.Len(t, lines, 2, row)
	assert.Equal(t, "  local, lo, locally-running-airflow-instance", lines[0])
	assert.Equal(t, strings.Repeat(" ", 2+12+2)+offline.Short, lines[1])
}

func TestWrapTextLeavesIndentedLinesWhole(t *testing.T) {
	long := strings.Repeat("word ", 30)
	command := "  astro deploy " + strings.Repeat("--flag value ", 10)
	got := wrapText(long+"\n"+command+"\n- "+long, 40)
	lines := strings.Split(got, "\n")

	assert.Contains(t, lines, command, "an indented line is copied, so it is never broken")
	for _, line := range lines {
		if line != command {
			assert.LessOrEqual(t, len(line), 40, "%q", line)
		}
	}
	assert.Contains(t, got, "\n- word", "a list item keeps its marker")
	assert.Contains(t, got, "\n  word", "a list item's continuation hangs under its text")
}

func TestFlagUsagesWrapsPastAWordWiderThanTheRoom(t *testing.T) {
	cmd := &cobra.Command{Use: "x"}
	url := "https://docs.docker.com/build/building/secrets/"
	cmd.Flags().StringArray("build-secret", nil,
		"Secret to expose to the build. See "+url+". Repeat to specify multiple secrets.")
	cmd.Flags().String("format", "text", "Output format")

	got := flagUsages(cmd.Flags(), 60)
	for _, line := range strings.Split(got, "\n") {
		if !strings.Contains(line, url) {
			assert.LessOrEqual(t, len(line), 60, "%q", line)
		}
	}
	words := strings.Join(strings.Fields(got), " ")
	assert.Contains(t, words, "Repeat to specify multiple secrets.", "the words after the URL are kept")
	assert.Contains(t, words, `Output format (default "text")`, "pflag's default is kept")
	assert.Contains(t, got, "--build-secret stringArray")
}

// TestExampleRunsCatchesWhatCobraWouldRefuse proves the example-runs rule is
// not vacuous: each failing line here is one cobra would refuse, and the rule
// names why.
func TestExampleRunsCatchesWhatCobraWouldRefuse(t *testing.T) {
	root := &cobra.Command{Use: "astro"}
	af := &cobra.Command{Use: "af", Run: func(*cobra.Command, []string) {}}
	dags := &cobra.Command{Use: "dags"}
	get := &cobra.Command{Use: "get <DAG_ID>", Args: cobra.ExactArgs(1), Run: func(*cobra.Command, []string) {}}
	get.Flags().BoolP("details", "d", false, "Show details")
	dags.AddCommand(get)
	af.AddCommand(dags)
	root.AddCommand(af)

	for _, tc := range []struct {
		line, want string
	}{
		{"astro af dags get <DAG_ID> --details", ""},
		{"astro af dags get <DAG_ID> -d   # with a trailing comment", ""},
		{`echo "x" | astro af dags get "my dag" | jq .`, ""},
		// A command after ||, && or ; is a command of its own, and checked.
		{"true || astro af dags get <DAG_ID>", ""},
		{"true && astro af dags get <DAG_ID> --bogus", "unknown flag: --bogus"},
		{"true; astro af dags get", "accepts 1 arg(s), received 0"},
		// Every astro command on a line is checked, not only the first.
		{"astro af dags get <DAG_ID> || astro af dags get <DAG_ID> --bogus", "unknown flag: --bogus"},
		{"astro af dags get <DAG_ID> --bogus", "unknown flag: --bogus"},
		{"astro af dags get", "accepts 1 arg(s), received 0"},
		{"astro af dagz get <DAG_ID>", `"astro af" has no subcommand "dagz"`},
		{"astro af dags", `"astro af dags" is a group, not a command`},
		{"astro AF dags get x", `unknown command "AF"`},
		{"ASTRO_LOCAL_HEALTH_TIMEOUT=10m astro af dags get <DAG_ID>", ""},
		{`A=1 B="two words" astro af dags get <DAG_ID> -d`, ""},
		{"ASTRO_LOCAL_HEALTH_TIMEOUT=10m astro af dags get <DAG_ID> --bogus", "unknown flag: --bogus"},
		{"ASTRO_LOCAL_HEALTH_TIMEOUT=10m astro af dagz get <DAG_ID>", `"astro af" has no subcommand "dagz"`},
		{"echo x | TOKEN=y astro af dags get", "accepts 1 arg(s), received 0"},
		{"astro af dags get <DAG_ID> \\\n    --bogus", "unknown flag: --bogus"},
	} {
		get.Example = "  " + tc.line
		got := checkExampleRuns(get)
		if tc.want == "" {
			assert.Empty(t, got, tc.line)
		} else {
			assert.Contains(t, got, tc.want, tc.line)
		}
	}
	assert.False(t, get.Flags().Changed("details"), "linting an example sets nothing on the command")
}

func TestExampleStyleAcceptsAnEnvironmentPrefix(t *testing.T) {
	for _, tc := range []struct {
		example string
		ok      bool
	}{
		{"  ASTRO_LOCAL_HEALTH_TIMEOUT=10m astro local start", true},
		{"  A=1 B=2 astro local start", true},
		{"  echo x | TOKEN=y astro local env variable set API_TOKEN --stdin", true},
		{"  ASTRO_LOCAL_HEALTH_TIMEOUT=10m", false},
		{"  ASTRO_LOCAL_HEALTH_TIMEOUT=10m airflow standalone", false},
		{"  export ASTRO_LOCAL_HEALTH_TIMEOUT=10m", false},
		{"  astro deploy \\\n    --dags", true},
		{"  astro local stop || astro local reset --yes", true},
		{"  true || astro local reset --yes", true},
	} {
		got := checkExampleStyle(&cobra.Command{Use: "x", Example: tc.example})
		if tc.ok {
			assert.Empty(t, got, tc.example)
		} else {
			assert.Contains(t, got, "neither an astro command nor a # comment", tc.example)
		}
	}
}

// TestExampleStyleRefusesSpaceAfterAContinuation: in a shell a backslash
// continues a line only as its last character, so ` \ ` would run the next
// line on its own, and example-runs would check neither half.
func TestExampleStyleRefusesSpaceAfterAContinuation(t *testing.T) {
	got := checkExampleStyle(&cobra.Command{Use: "x", Example: "  astro deploy \\ \n    --dags"})
	assert.Contains(t, got, "whitespace after its continuation backslash")
}

// TestExampleStylePairsContinuationsWithBackslashes: a continuation line
// without the backslash before it pastes as a second command made only of
// flags, and a backslash without a continuation after it runs on into the
// next example.
func TestExampleStylePairsContinuationsWithBackslashes(t *testing.T) {
	assert.Empty(t, checkExampleStyle(&cobra.Command{Use: "x", Example: "  astro deploy \\\n    --dags \\\n    --wait"}))
	assert.Contains(t, checkExampleStyle(&cobra.Command{Use: "x", Example: "  astro deploy\n    --dags"}),
		"the line before does not end in ` \\`")
	assert.Contains(t, checkExampleStyle(&cobra.Command{Use: "x", Example: "  astro deploy \\\n  astro deploy --dags"}),
		"follows a continuation backslash but is not indented four spaces")
}

// TestWrapCountsCharactersNotBytes: an em dash is three bytes and one column,
// so measuring bytes wrapped prose with one early.
func TestWrapCountsCharactersNotBytes(t *testing.T) {
	line := strings.Repeat("—", 30) + " end"
	assert.Equal(t, line, wrapText(line, 34), "34 characters fit in 34 columns")
	assert.Equal(t, line, wrapHanging(line, 34, 0))
}

func TestExampleWidthMeasuresEachLineAsPrinted(t *testing.T) {
	fits := "  astro " + strings.Repeat("x", helpMaxWidth-len("  astro "))
	assert.Empty(t, checkExampleWidth(&cobra.Command{Example: fits}))
	assert.Contains(t, checkExampleWidth(&cobra.Command{Example: fits + "x"}), "101 columns")
	// The continuation backslash is part of the line it ends.
	assert.Contains(t, checkExampleWidth(&cobra.Command{Example: fits + " \\\n    --flag"}), "102 columns")
	assert.Empty(t, checkExampleWidth(&cobra.Command{Example: fits[:helpMaxWidth-2] + " \\\n    --flag"}))
	assert.Contains(t, checkExampleWidth(&cobra.Command{Example: "  # " + strings.Repeat("x", helpMaxWidth)}), "104 columns")
}

func TestUseSpellsAPassthroughOneWay(t *testing.T) {
	for _, tc := range []struct {
		use string
		ok  bool
	}{
		{"add [NAME] [-- COMMAND...]", true},
		{"run [-- ARGS...]", true},
		{"add [-- COMMAND...] [NAME]", false},
		{"add [-- command...]", false},
		{"add [-- COMMAND]", false},
		{"add -- COMMAND...", false},
		{"add [--COMMAND...]", false},
	} {
		got := checkUse(&cobra.Command{Use: tc.use})
		if tc.ok {
			assert.Empty(t, got, tc.use)
		} else {
			assert.NotEmpty(t, got, tc.use)
		}
	}
}

func TestRootRowCatchesAShortThatWraps(t *testing.T) {
	run := func(*cobra.Command, []string) {}
	root := &cobra.Command{Use: "astro"}
	// The column is sized to "deployment, de" (14), so a description starts
	// at column 2+14+2 and has 100-18 = 82 before it wraps.
	deployment := &cobra.Command{Use: "deployment", Aliases: []string{"de"}, Short: "Manage Deployments", Run: run}
	fits := &cobra.Command{Use: "fits", Short: strings.Repeat("x", 82), Run: run}
	wraps := &cobra.Command{Use: "wraps", Short: "Two words " + strings.Repeat("x", 73), Run: run}
	child := &cobra.Command{Use: "child", Short: "Not on the root page " + strings.Repeat("x", 100), Run: run}
	fits.AddCommand(child)
	root.AddCommand(deployment, fits, wraps)
	assert.Empty(t, checkRootRow(deployment))
	assert.Empty(t, checkRootRow(fits))
	assert.Contains(t, checkRootRow(wraps), "Short is 83 columns and the root page has 82 before it wraps at 100")
	assert.Empty(t, checkRootRow(child))

	// Wider than helpNameColumnMax: the column is capped there, these
	// spellings take a line of their own, and the description on the next
	// has 100-(2+30+2) = 66.
	wide := &cobra.Command{Use: strings.Repeat("w", helpNameColumnMax+1), Short: strings.Repeat("x", 66), Run: run}
	wideWraps := &cobra.Command{Use: strings.Repeat("v", helpNameColumnMax+1), Short: "Two words " + strings.Repeat("x", 57), Run: run}
	root.AddCommand(wide, wideWraps)
	assert.Empty(t, checkRootRow(wide))
	assert.Contains(t, checkRootRow(wideWraps), "Short is 67 columns and the root page has 66 before it wraps at 100")

	// One word the renderer cannot break overruns the width without
	// wrapping, which counting wrapped lines alone would miss.
	overruns := &cobra.Command{Use: "overruns", Short: strings.Repeat("x", 90), Run: run}
	root.AddCommand(overruns)
	assert.Contains(t, checkRootRow(overruns), "past the 100 the root page wraps at")
}

// TestLongUnwrappedFindsHandWrappedProse proves the long-unwrapped rule is not
// vacuous, and that it leaves alone the line breaks a Long means to keep.
func TestLongUnwrappedFindsHandWrappedProse(t *testing.T) {
	for _, tc := range []struct {
		name, long string
		flagged    bool
	}{
		{"a paragraph wrapped by hand", "Declare a name in pyproject.toml, or change how it is\ndeclared.", true},
		{"a break before a code span", "Link it with\n`astro link`, as in Desktop.", true},
		{"a break inside a parenthesis", "It is optional (see\nbelow).", true},
		{"one line per paragraph", "One paragraph, however long.\n\nAnother.", false},
		{"a heading and its list", "Shows, including:\n- Path parameters\n- Response schema", false},
		{"list items without periods", "- CLI version\n- Operating system", false},
		{"an indented list and its continuation", "The flag converts:\n  - literal values, which get\n    converted", false},
		{"a heading and its command", "Run it as:\n  astro deploy\n\nto ship.", false},
		{"a list, then a paragraph after a blank line", "We collect:\n- The version\n\nNothing else.", false},
		// One sentence per line renders as a run of short lines, and a
		// sentence end cannot be told from an abbreviation or a version.
		{"a sentence per line", "Start Otto.\nFlags are forwarded.", true},
		{"a sentence ending in a code span", "Use `astro use`.\nIt lists them.", true},
		{"a break after an abbreviation", "Works with providers, hooks, etc.\nand sensors too", true},
		{"a break inside a version number", "Runs on Airflow 2.\n10 and later", true},
		{"a list item wrapped by hand", "We collect:\n- Invocation context (CI,\ninteractive, etc.)", true},
		{"a list item continued by indenting", "We collect:\n- Invocation context (CI,\n  interactive, etc.)", true},
		{"a break after a colon in mid-sentence", "Store one unencrypted:\nin the project's .env.", true},
		{"a continuation with a stray space", "Link a global value to projects, so it\n reaches those projects", true},
		{"a command inside a sentence", "Run it as\n  astro deploy\nto ship.", true},
		{"a paragraph straight after a list", "We collect:\n- The version\nNothing else.", true},
	} {
		got := checkLongUnwrapped(&cobra.Command{Use: "x", Long: tc.long})
		if tc.flagged {
			assert.NotEmpty(t, got, tc.name)
		} else {
			assert.Empty(t, got, tc.name)
		}
	}
}
