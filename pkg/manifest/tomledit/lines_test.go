package tomledit

import (
	"strings"
	"testing"
)

func TestArraysAreWrittenOneElementPerLine(t *testing.T) {
	cases := []struct {
		name string
		src  string
		ops  []op
		want string
	}{
		{
			name: "a new array",
			src:  "[project]\nname = 'x'\n",
			ops:  []op{set([]any{"pandas==2.2.3", "requests"}, "project", "dependencies")},
			want: "[project]\nname = 'x'\ndependencies = [\n    'pandas==2.2.3',\n    'requests',\n]\n",
		},
		{
			name: "one element, as uv writes it",
			src:  "[project]\ndependencies = []\n",
			ops:  []op{set([]any{"apache-airflow==3.3.*"}, "project", "dependencies")},
			want: "[project]\ndependencies = [\n    'apache-airflow==3.3.*',\n]\n",
		},
		{
			name: "an empty array stays inline",
			src:  "[project]\ndependencies = ['a']\n",
			ops:  []op{set([]any{}, "project", "dependencies")},
			want: "[project]\ndependencies = []\n",
		},
		{
			name: "a dotted key",
			src:  "project.name = 'x'\n",
			ops:  []op{set([]any{"a"}, "project", "dependencies")},
			want: "project.name = 'x'\nproject.dependencies = [\n    'a',\n]\n",
		},
		{
			name: "CRLF line endings",
			src:  "[project]\r\nname = 'x'\r\n",
			ops:  []op{set([]any{"a", "b"}, "project", "dependencies")},
			want: "[project]\r\nname = 'x'\r\ndependencies = [\r\n    'a',\r\n    'b',\r\n]\r\n",
		},
		{
			name: "setting the value already there changes nothing",
			src:  "[tool.astro]\npackages = ['libpq-dev']\n",
			ops:  []op{set([]any{"libpq-dev"}, "tool", "astro", "packages")},
			want: "[tool.astro]\npackages = ['libpq-dev']\n",
		},
		{
			name: "an append to an empty array",
			src:  "[project]\ndependencies = []\n",
			ops:  []op{set("a", "project", "dependencies", "0")},
			want: "[project]\ndependencies = [\n    'a',\n]\n",
		},
		{
			name: "an append to a one-line array keeps the elements as spelled",
			src:  "[project]\ndependencies = [\"a\", 'b']  # pinned\n",
			ops:  []op{set("c", "project", "dependencies", "2")},
			want: "[project]\ndependencies = [\n    \"a\",\n    'b',\n    'c',\n]  # pinned\n",
		},
		{
			name: "an append follows the indent and the comments already there",
			src:  "[project]\ndependencies = [\n  # first\n  'a',\n  'b'  # why b\n]\n",
			ops:  []op{set("c", "project", "dependencies", "2"), set("d", "project", "dependencies", "3")},
			want: "[project]\ndependencies = [\n  # first\n  'a',\n  'b',  # why b\n  'c',\n  'd',\n]\n",
		},
		{
			name: "an append before a bracket on the last element's line",
			src:  "[project]\ndependencies = [\n    'a',\n    'b']\n",
			ops:  []op{set("c", "project", "dependencies", "2")},
			want: "[project]\ndependencies = [\n    'a',\n    'b',\n    'c',\n]\n",
		},
		{
			name: "an append to an empty multi-line array",
			src:  "[project]\ndependencies = [\n]\n",
			ops:  []op{set("a", "project", "dependencies", "0")},
			want: "[project]\ndependencies = [\n    'a',\n]\n",
		},
		{
			name: "an array inside an inline table stays inline",
			src:  "[tool.astro.env]\nX = { type = 'str' }\n",
			ops:  []op{set([]any{"a", "b"}, "tool", "astro", "env", "X", "enum")},
			want: "[tool.astro.env]\nX = { type = 'str', enum = ['a', 'b'] }\n",
		},
		{
			name: "an array of arrays stays inline",
			src:  "[x]\n",
			ops:  []op{set([]any{[]any{"a"}}, "x", "y")},
			want: "[x]\ny = [['a']]\n",
		},
		{
			name: "a replaced element stays where it is",
			src:  "[project]\ndependencies = ['a', 'b']\n",
			ops:  []op{set("c", "project", "dependencies", "1")},
			want: "[project]\ndependencies = ['a', 'c']\n",
		},
		{
			name: "an append after a comma on a line of its own",
			src:  "[project]\ndependencies = [\n    'a'\n    ,\n]\n",
			ops:  []op{set("b", "project", "dependencies", "1")},
			want: "[project]\ndependencies = [\n    'a'\n    ,\n    'b',\n]\n",
		},
		{
			name: "an append after a comment and then a comma",
			src:  "[project]\ndependencies = [\n    'a' # why\n    ,\n]\n",
			ops:  []op{set("b", "project", "dependencies", "1")},
			want: "[project]\ndependencies = [\n    'a' # why\n    ,\n    'b',\n]\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, err := NewSurgical([]byte(tc.src))
			if err != nil {
				t.Fatal(err)
			}
			apply(t, e, tc.ops)
			got, err := e.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}

// A key-value under [[project.plugins]] follows [project] in the file but is
// not project's, so an append to project's missing array does not land in it.
func TestAnAppendDoesNotReachIntoAnArrayOfTables(t *testing.T) {
	src := "[project]\nname = 'x'\n\n[[project.plugins]]\ndependencies = ['p']\n"
	e, err := NewSurgical([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Set([]string{"project", "dependencies", "1"}, "b"); err != nil {
		t.Fatal(err)
	}
	got, err := e.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "[[project.plugins]]\ndependencies = ['p']\n") {
		t.Errorf("the plugins array changed:\n%s", got)
	}
}
