package scaffold

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/astronomer/astro-cli/pkg/manifest"
)

func TestParseRequirementsClassifies(t *testing.T) {
	in := []byte(`# top comment
apache-airflow==2.9.1
flask>=2.0,<3  # web
requests

-r base.txt
--index-url https://example.test/simple
!!!garbage!!!
pandas[performance]==2.2.0 ; python_version >= "3.11"
`)
	lines := parseRequirements(in)

	// Blank lines are dropped; everything with content is kept in order.
	require.Len(t, lines, 8)
	assert.Equal(t, reqLine{kind: reqComment, text: "top comment"}, lines[0])
	assert.Equal(t, reqLine{kind: reqDependency, text: "apache-airflow==2.9.1"}, lines[1])
	assert.Equal(t, reqLine{kind: reqDependency, text: "flask>=2.0,<3", inline: "web"}, lines[2])
	assert.Equal(t, reqLine{kind: reqDependency, text: "requests"}, lines[3])
	assert.Equal(t, reqCarried, lines[4].kind) // -r base.txt
	assert.Equal(t, reqCarried, lines[5].kind) // --index-url
	assert.Equal(t, reqCarried, lines[6].kind) // garbage
	assert.Equal(t, reqDependency, lines[7].kind)
	assert.Equal(t, `pandas[performance]==2.2.0 ; python_version >= "3.11"`, lines[7].text)
}

func TestParseRequirementsJoinsContinuations(t *testing.T) {
	lines := parseRequirements([]byte("flask==2.0 \\\n    --hash=sha256:abc\n"))
	require.Len(t, lines, 1)
	assert.Equal(t, reqDependency, lines[0].kind)
	assert.Contains(t, lines[0].text, "flask==2.0")
	assert.Contains(t, lines[0].text, "--hash")
}

func TestRenderDependenciesPreservesCommentsAndParses(t *testing.T) {
	lines := parseRequirements([]byte("# a note\nflask==2.0  # web\n-e ./local\n"))
	literal, count := renderDependencies(lines)
	assert.Equal(t, 1, count)
	assert.Contains(t, literal, `"flask==2.0",  # web`)
	assert.Contains(t, literal, "# a note")
	assert.Contains(t, literal, "carried from requirements.txt")

	// The rendered array must sit inside a valid, loadable manifest.
	doc := "[project]\nname = 'x'\nrequires-python = '>=3.10'\ndependencies = " + literal + "\n\n[tool.astro]\nairflow = '3.1'\n"
	m, err := manifest.Parse([]byte(doc))
	require.NoError(t, err)
	assert.Equal(t, []string{"flask==2.0"}, m.Project.Dependencies)
}

func TestRenderDependenciesEmpty(t *testing.T) {
	literal, count := renderDependencies(nil)
	assert.Equal(t, "[]", literal)
	assert.Equal(t, 0, count)
}

func TestTomlStringEscapesQuotesAndMarkers(t *testing.T) {
	// A marker with double quotes must survive as valid TOML.
	got := tomlString(`pandas==2.0; python_version >= "3.11"`)
	doc := "[project]\nname = 'x'\nrequires-python = '>=3.10'\ndependencies = [" + got + "]\n\n[tool.astro]\nairflow = '3.1'\n"
	m, err := manifest.Parse([]byte(doc))
	require.NoError(t, err)
	assert.Equal(t, `pandas==2.0; python_version >= "3.11"`, m.Project.Dependencies[0])
}

func TestAirflowPin(t *testing.T) {
	cases := []struct {
		spec   string
		want   string
		wantOK bool
	}{
		{"apache-airflow==2.9.1", "2.9.1", true},
		{"apache-airflow==2.9", "2.9", true},
		{"apache_airflow==2.10.0", "2.10.0", true},
		{"apache-airflow[amazon]==2.9.1", "2.9.1", true},
		{`apache-airflow==2.9.1 ; python_version < "3.12"`, "2.9.1", true},
		{"apache-airflow>=2.9", "", false},
		{"apache-airflow==2.9.*", "", false},
		{"apache-airflow==2.9,<3", "", false},
		{"flask==2.0", "", false},
	}
	for _, tc := range cases {
		lines := []reqLine{{kind: reqDependency, text: tc.spec}}
		got, ok := airflowPin(lines)
		assert.Equal(t, tc.wantOK, ok, tc.spec)
		assert.Equal(t, tc.want, got, tc.spec)
	}
}

func TestCarriedWarnings(t *testing.T) {
	lines := parseRequirements([]byte("flask\n-r base.txt\n!!!bad\n"))
	ws := carriedWarnings(lines)
	require.Len(t, ws, 2)
	assert.True(t, strings.Contains(ws[0], "base.txt"))
}
