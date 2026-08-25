package tomledit

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	toml "github.com/pelletier/go-toml/v2"
)

var update = flag.Bool("update", false, "rewrite the golden files")

// op is one edit: a Set, or a Delete when del is true.
type op struct {
	key []string
	val any
	del bool
}

func set(val any, key ...string) op { return op{key: key, val: val} }
func del(key ...string) op          { return op{key: key, del: true} }

func apply(t *testing.T, e Editor, ops []op) {
	t.Helper()
	for _, o := range ops {
		if o.del {
			if !e.Delete(o.key) {
				t.Fatalf("Delete(%v) = false, key should exist", o.key)
			}
			continue
		}
		if err := e.Set(o.key, o.val); err != nil {
			t.Fatalf("Set(%v, %v): %v", o.key, o.val, err)
		}
	}
}

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestSurgicalRoundTripUnedited: with no edits, output is the input,
// byte for byte.
func TestSurgicalRoundTripUnedited(t *testing.T) {
	inputs, err := filepath.Glob(filepath.Join("testdata", "*.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(inputs) == 0 {
		t.Fatal("no testdata")
	}
	for _, path := range inputs {
		if strings.HasSuffix(path, ".edited.toml") {
			continue
		}
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			e, err := NewSurgical(src)
			if err != nil {
				t.Fatal(err)
			}
			got, err := e.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, src) {
				t.Errorf("round trip changed the document:\n%s", got)
			}
		})
	}
}

// TestGoldenEdits drives both implementations through the same edits on
// each hand-authored manifest. The surgical output must match the golden
// file exactly (comments, order, and formatting preserved); the rewrite
// output must carry the same data, checked by decoding both.
func TestGoldenEdits(t *testing.T) {
	cases := []struct {
		file string
		ops  []op
		// keep are comments that must survive the edits; drop are
		// comments that must go with their deleted expression.
		keep []string
		drop []string
	}{
		{
			file: "01-basic.toml",
			ops: []op{
				set("3.2", "tool", "astro", "airflow"),
				set("dep-new", "tool", "astro", "deployments", "my-prod-deployment", "deployment"),
				set("astro", "tool", "astro", "deployments", "my-dev-deployment", "target"),
				set("ws-abc", "tool", "astro", "deployments", "my-dev-deployment", "workspace"),
				set("dep-dev", "tool", "astro", "deployments", "my-dev-deployment", "deployment"),
				del("tool", "astro", "deployments", "my-preview-deployment"),
			},
			keep: []string{
				"# floats to the newest patch by default",
				"# meaningless to other targets",
			},
		},
		{
			file: "02-minimal.toml",
			ops: []op{
				set("3.1", "tool", "astro", "airflow"),
				set("astro", "tool", "astro", "deployments", "prod", "target"),
				set("ws-etl", "tool", "astro", "deployments", "prod", "workspace"),
				set("dep-etl-prod", "tool", "astro", "deployments", "prod", "deployment"),
			},
		},
		{
			file: "03-multiline-deps.toml",
			ops: []op{
				set("polars", "project", "dependencies", "4"),
				set("3.1", "tool", "astro", "airflow"),
			},
			keep: []string{
				"# pinned after the 4.x dialect break",
				"# kept for the legacy S3 sensors; drop with AIRFLOW-1893",
			},
		},
		{
			file: "04-dotted-keys.toml",
			ops: []op{
				set("3.2", "tool", "astro", "airflow"),
				set("astro", "tool", "astro", "deployments", "prod", "target"),
				set("ws-dot", "tool", "astro", "deployments", "prod", "workspace"),
				set("dep-dot", "tool", "astro", "deployments", "prod", "deployment"),
			},
			keep: []string{"# everything under one [tool] header, dotted"},
		},
		{
			file: "05-uv-and-ruff.toml",
			ops: []op{
				set("3.0", "tool", "astro", "airflow"),
				set("https://pypi.internal.example.com/v2/simple", "tool", "uv", "index", "0", "url"),
				del("tool", "ruff", "lint", "ignore"),
			},
			keep: []string{
				"# credential lives in the astro vault, not here",
				"# AIR = airflow ruleset",
				"# resolution must stay reproducible across CI and laptops",
			},
		},
		{
			file: "06-env-schema.toml",
			ops: []op{
				set(true, "tool", "astro", "env", "connections", "crm", "required"),
				set("postgres", "tool", "astro", "env", "connections", "metrics", "conn_type"),
				set("dep-prod-2", "tool", "astro", "deployments", "prod", "deployment"),
				del("tool", "astro", "env", "env-vars", "LOG_LEVEL"),
			},
			keep: []string{"# same conn_id everywhere; only the environment behind it changes"},
			drop: []string{"# defaults to info"},
		},
		{
			file: "07-literal-strings.toml",
			ops: []op{
				set("msodbcsql18", "tool", "astro", "target", "astro", "system-packages", "2"),
				set("3.2", "tool", "astro", "airflow"),
			},
			keep: []string{"# literal strings so the regexes below stay un-escaped"},
		},
		{
			file: "08-array-of-tables.toml",
			ops: []op{
				set("mirror", "tool", "uv", "index", "2", "name"),
				set("https://mirror.example.com/simple", "tool", "uv", "index", "2", "url"),
				set("dep-dev-2", "tool", "astro", "deployments", "dev", "deployment"),
			},
			keep: []string{"# ordered: uv checks the first index that has the package"},
		},
		{
			file: "09-comments-everywhere.toml",
			ops: []op{
				set("3.2", "tool", "astro", "airflow"),
				set("ws-finance-2", "tool", "astro", "deployments", "prod", "workspace"),
				del("project", "dependencies", "1"),
				set("astro", "tool", "astro", "deployments", "staging", "target"),
				set("ws-finance", "tool", "astro", "deployments", "staging", "workspace"),
				set("dep-stg-1a9d", "tool", "astro", "deployments", "staging", "deployment"),
			},
			keep: []string{
				"# every change here goes through CODEOWNERS review",
				"# also the local hostname: finance-dags.localhost",
				"# 3.1 until the 3.2 provider audit closes",
				"# the only deployment CI may touch",
			},
			drop: []string{"# alerting only"},
		},
		{
			file: "10-mixed-formatting.toml",
			ops: []op{
				set("3.13", "tool", "astro", "target", "astro", "image", "python"),
				set("ws-y", "tool", "astro", "deployments", "weird name", "workspace"),
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			src := readTestdata(t, tc.file)

			sur, err := NewSurgical(src)
			if err != nil {
				t.Fatal(err)
			}
			apply(t, sur, tc.ops)
			got, err := sur.Bytes()
			if err != nil {
				t.Fatal(err)
			}

			golden := filepath.Join("testdata", strings.TrimSuffix(tc.file, ".toml")+".edited.toml")
			if *update {
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("surgical output differs from %s:\n--- got ---\n%s", golden, got)
			}
			for _, c := range tc.keep {
				if !bytes.Contains(got, []byte(c)) {
					t.Errorf("comment lost: %s", c)
				}
			}
			for _, c := range tc.drop {
				if bytes.Contains(got, []byte(c)) {
					t.Errorf("comment should have gone with its expression: %s", c)
				}
			}

			// The fallback loses layout, never data: both outputs must
			// decode to the same document.
			rw, err := NewRewrite(src)
			if err != nil {
				t.Fatal(err)
			}
			apply(t, rw, tc.ops)
			rwOut, err := rw.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			var fromSur, fromRw map[string]any
			if err := toml.Unmarshal(got, &fromSur); err != nil {
				t.Fatalf("surgical output does not parse: %v", err)
			}
			if err := toml.Unmarshal(rwOut, &fromRw); err != nil {
				t.Fatalf("rewrite output does not parse: %v", err)
			}
			if !reflect.DeepEqual(fromSur, fromRw) {
				t.Errorf("implementations disagree on data:\nsurgical: %#v\nrewrite:  %#v", fromSur, fromRw)
			}
		})
	}
}

// both runs a subtest against each implementation.
func both(t *testing.T, src []byte, fn func(t *testing.T, e Editor)) {
	t.Helper()
	for name, ctor := range map[string]func([]byte) (Editor, error){
		"surgical": NewSurgical,
		"rewrite":  NewRewrite,
	} {
		t.Run(name, func(t *testing.T) {
			e, err := ctor(src)
			if err != nil {
				t.Fatal(err)
			}
			fn(t, e)
		})
	}
}

func TestSetOverTableFails(t *testing.T) {
	src := readTestdata(t, "01-basic.toml")
	both(t, src, func(t *testing.T, e Editor) {
		err := e.Set([]string{"tool", "astro"}, "nope")
		if err == nil {
			t.Fatal("Set over an existing table should fail")
		}
		var ke *KeyError
		if !errors.As(err, &ke) {
			t.Fatalf("want *KeyError, got %T: %v", err, err)
		}
	})
}

func TestDeleteMissing(t *testing.T) {
	src := readTestdata(t, "02-minimal.toml")
	both(t, src, func(t *testing.T, e Editor) {
		if e.Delete([]string{"tool", "astro", "no-such-key"}) {
			t.Error("Delete of a missing key reported true")
		}
		if e.Delete([]string{"no", "such", "table"}) {
			t.Error("Delete under a missing table reported true")
		}
	})
}

func TestArrayAppendAndOutOfRange(t *testing.T) {
	src := []byte("packages = [\"a\", \"b\"]\n")
	both(t, src, func(t *testing.T, e Editor) {
		// Append at the index Get reports as the current length.
		cur, ok := e.Get([]string{"packages"})
		if !ok {
			t.Fatal("Get(packages) reported missing")
		}
		next := strconv.Itoa(len(cur.([]any)))
		if err := e.Set([]string{"packages", next}, "c"); err != nil {
			t.Fatalf("append: %v", err)
		}
		if err := e.Set([]string{"packages", "9"}, "z"); err == nil {
			t.Error("Set past the end should fail")
		}
		for _, idx := range []string{"-0", "+1", "1x", ""} {
			if err := e.Set([]string{"packages", idx}, "z"); err == nil {
				t.Errorf("Set at index %q should fail", idx)
			}
		}
		// Leading zeros are digits, like the surgical editor's addressing.
		if err := e.Set([]string{"packages", "01"}, "b2"); err != nil {
			t.Fatalf("Set at index 01: %v", err)
		}
		out, err := e.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Packages []string `toml:"packages"`
		}
		if err := toml.Unmarshal(out, &doc); err != nil {
			t.Fatal(err)
		}
		want := []string{"a", "b2", "c"}
		if !reflect.DeepEqual(doc.Packages, want) {
			t.Errorf("packages = %v, want %v", doc.Packages, want)
		}
	})
}

func TestParseErrors(t *testing.T) {
	bad := []byte("not = valid = toml\n")
	for name, ctor := range map[string]func([]byte) (Editor, error){
		"surgical": NewSurgical,
		"rewrite":  NewRewrite,
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ctor(bad)
			var pe *ParseError
			if !errors.As(err, &pe) {
				t.Fatalf("want *ParseError, got %T: %v", err, err)
			}
		})
	}
}

func TestGet(t *testing.T) {
	src := readTestdata(t, "01-basic.toml")
	both(t, src, func(t *testing.T, e Editor) {
		v, ok := e.Get([]string{"tool", "astro", "airflow"})
		if !ok || v != "3.1" {
			t.Errorf("Get(tool.astro.airflow) = %v, %v", v, ok)
		}
		v, ok = e.Get([]string{"tool", "astro", "deployments", "my-prod-deployment"})
		if !ok {
			t.Fatal("Get of a table reported missing")
		}
		table, isMap := v.(map[string]any)
		if !isMap || table["workspace"] != "ws-abc" {
			t.Errorf("table decode = %#v", v)
		}
		if _, ok := e.Get([]string{"tool", "astro", "nope"}); ok {
			t.Error("Get of a missing key reported present")
		}
		// Values come back as their TOML decoding, whatever Go type went in.
		if err := e.Set([]string{"tool", "astro", "retries"}, 3); err != nil {
			t.Fatal(err)
		}
		v, ok = e.Get([]string{"tool", "astro", "retries"})
		if !ok || v != int64(3) {
			t.Errorf("Get after Set(int) = %v (%T), want int64(3)", v, v)
		}
	})
}

// TestEnsureTablesAtTop: a table the manifest does not have yet is created at
// the top, above the tables already there, and everything else keeps its
// place and its bytes.
func TestEnsureTablesAtTop(t *testing.T) {
	astro := [][]string{{"project"}, {"tool", "astro"}}
	cases := map[string]struct {
		src  string
		want string
	}{
		"tool sections only": {
			src:  "[tool.ruff]\nline-length = 100\n\n[tool.mypy]\nstrict = true\n",
			want: "[project]\n\n[tool.astro]\n\n[tool.ruff]\nline-length = 100\n\n[tool.mypy]\nstrict = true\n",
		},
		"leading comment block stays first": {
			src:  "# Copyright ACME\n# vim: ft=toml\n\n[tool.ruff]\nline-length = 100\n",
			want: "# Copyright ACME\n# vim: ft=toml\n\n[project]\n\n[tool.astro]\n\n[tool.ruff]\nline-length = 100\n",
		},
		"comment attached to the first header travels with it": {
			src:  "# ruff, not the linter you think\n[tool.ruff]\nline-length = 100\n",
			want: "[project]\n\n[tool.astro]\n\n# ruff, not the linter you think\n[tool.ruff]\nline-length = 100\n",
		},
		"a table already there keeps its place": {
			src:  "[project]\nname = 'orders'\n\n[project.urls]\nhome = 'https://example.com'\n\n[tool.ruff]\nline-length = 100\n",
			want: "[project]\nname = 'orders'\n\n[project.urls]\nhome = 'https://example.com'\n\n[tool.astro]\n\n[tool.ruff]\nline-length = 100\n",
		},
		"key-values outside any table stay above": {
			src:  "requires = 'nothing'\n\n[tool.ruff]\nline-length = 100\n",
			want: "requires = 'nothing'\n\n[project]\n\n[tool.astro]\n\n[tool.ruff]\nline-length = 100\n",
		},
		"empty document": {
			src:  "",
			want: "[project]\n\n[tool.astro]\n",
		},
		"document with no tables": {
			src:  "# nothing but a comment\n",
			want: "# nothing but a comment\n\n[project]\n\n[tool.astro]\n",
		},
		"nothing to create": {
			src:  "[project]\nname = 'orders'\n\n[tool.astro]\nairflow = '3.1'\n",
			want: "[project]\nname = 'orders'\n\n[tool.astro]\nairflow = '3.1'\n",
		},
		"CRLF document": {
			src:  "[tool.ruff]\r\nline-length = 100\r\n",
			want: "[project]\r\n\r\n[tool.astro]\r\n\r\n[tool.ruff]\r\nline-length = 100\r\n",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			e, err := NewSurgical([]byte(tc.src))
			if err != nil {
				t.Fatal(err)
			}
			if err := e.EnsureTablesAtTop(astro); err != nil {
				t.Fatalf("EnsureTablesAtTop: %v", err)
			}
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

// TestEnsureTablesAtTopThenSet: the keys written afterwards land in the new
// tables, where they now are, and both implementations carry the same data.
func TestEnsureTablesAtTopThenSet(t *testing.T) {
	src := []byte("[tool.ruff]\nline-length = 100\n")
	both(t, src, func(t *testing.T, e Editor) {
		if err := e.EnsureTablesAtTop([][]string{{"project"}, {"tool", "astro"}}); err != nil {
			t.Fatal(err)
		}
		apply(t, e, []op{
			set("orders", "project", "name"),
			set("0.1.0", "project", "version"),
			set("3.1", "tool", "astro", "airflow"),
		})
		out, err := e.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Project struct {
				Name    string `toml:"name"`
				Version string `toml:"version"`
			} `toml:"project"`
			Tool struct {
				Astro struct {
					Airflow string `toml:"airflow"`
				} `toml:"astro"`
				Ruff struct {
					LineLength int `toml:"line-length"`
				} `toml:"ruff"`
			} `toml:"tool"`
		}
		if err := toml.Unmarshal(out, &doc); err != nil {
			t.Fatal(err)
		}
		if doc.Project.Name != "orders" || doc.Project.Version != "0.1.0" {
			t.Errorf("project = %+v", doc.Project)
		}
		if doc.Tool.Astro.Airflow != "3.1" || doc.Tool.Ruff.LineLength != 100 {
			t.Errorf("tool = %+v", doc.Tool)
		}
	})
}

func TestEmptyKeySetFails(t *testing.T) {
	both(t, []byte(""), func(t *testing.T, e Editor) {
		if err := e.Set(nil, "x"); err == nil {
			t.Error("Set with an empty key should fail")
		}
		if err := e.EnsureTablesAtTop([][]string{nil}); err == nil {
			t.Error("EnsureTablesAtTop with an empty key should fail")
		}
	})
}
