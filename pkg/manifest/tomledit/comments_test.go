package tomledit

import (
	"errors"
	"reflect"
	"testing"
)

var pinsKey = []string{"tool", "uv", "constraint-dependencies"}

func TestCommentsInAnArrayAreWrittenAndReadBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		set  []any
		want string
		// back is what GetLines reads from want, when it is not set.
		back []any
	}{
		{
			name: "a new array with a comment between its elements",
			src:  "[tool.uv]\n",
			set:  []any{"apache-airflow==3.3.2+astro.2", Comment("Astro Runtime 3.3-9 ships these."), "a2wsgi==1.10.10"},
			want: "[tool.uv]\nconstraint-dependencies = [\n    'apache-airflow==3.3.2+astro.2',\n    # Astro Runtime 3.3-9 ships these.\n    'a2wsgi==1.10.10',\n]\n",
		},
		{
			name: "an array replaced keeps nothing of the old one",
			src:  "[tool.uv]\nconstraint-dependencies = [\n    'x',\n    # old\n    'y',\n]\n",
			set:  []any{"x", Comment("new"), "z"},
			want: "[tool.uv]\nconstraint-dependencies = [\n    'x',\n    # new\n    'z',\n]\n",
		},
		{
			name: "a comment that is empty, or would break the line",
			src:  "[tool.uv]\n",
			set:  []any{Comment(""), Comment("a\nb"), "x"},
			want: "[tool.uv]\nconstraint-dependencies = [\n    #\n    # a b\n    'x',\n]\n",
			back: []any{Comment(""), Comment("a b"), "x"},
		},
		{
			name: "the lines already there change nothing",
			src:  "[tool.uv]\nconstraint-dependencies = [\n  'x',  # spelled\n  # mine\n  \"y\",\n]\n",
			set:  []any{"x", Comment("spelled"), Comment("mine"), "y"},
			want: "[tool.uv]\nconstraint-dependencies = [\n  'x',  # spelled\n  # mine\n  \"y\",\n]\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, err := NewSurgical([]byte(tc.src))
			if err != nil {
				t.Fatal(err)
			}
			if err := e.Set(pinsKey, tc.set); err != nil {
				t.Fatal(err)
			}
			got, err := e.Bytes()
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("got:\n%s\nwant:\n%s", got, tc.want)
			}
			back, err := NewSurgical(got)
			if err != nil {
				t.Fatal(err)
			}
			wantLines := tc.set
			if tc.back != nil {
				wantLines = tc.back
			}
			lines, ok := back.GetLines(pinsKey)
			if !ok || !reflect.DeepEqual(lines, wantLines) {
				t.Errorf("GetLines = %#v, want %#v", lines, wantLines)
			}
			values, _ := back.Get(pinsKey)
			if !reflect.DeepEqual(values, valuesOf(wantLines)) {
				t.Errorf("Get = %#v, want the values alone", values)
			}
		})
	}
}

// A '#' inside a string is the string's, not a comment: a direct reference
// can carry a fragment.
func TestGetLinesReadsNoCommentInsideAString(t *testing.T) {
	src := "[tool.uv]\nconstraint-dependencies = [\n    'x @ https://h/x.whl#sha256=ab',  # pinned\n]\n"
	e, err := NewSurgical([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	got, _ := e.GetLines(pinsKey)
	want := []any{"x @ https://h/x.whl#sha256=ab", Comment("pinned")}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("GetLines = %#v, want %#v", got, want)
	}
}

func TestGetLinesOfWhatIsNotAnArray(t *testing.T) {
	for _, newEditor := range []func([]byte) (Editor, error){NewSurgical, NewRewrite} {
		e, err := newEditor([]byte("[tool.uv]\nx = 'y'\n"))
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := e.GetLines([]string{"tool", "uv", "x"}); ok {
			t.Error("GetLines of a string reported an array")
		}
		if _, ok := e.GetLines([]string{"tool", "uv", "missing"}); ok {
			t.Error("GetLines of a missing key reported an array")
		}
	}
}

// The rewrite editor keeps no comments, so it writes the values and reads
// them back alone.
func TestTheRewriteEditorLeavesCommentsOut(t *testing.T) {
	e, err := NewRewrite([]byte("[tool.uv]\n"))
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Set(pinsKey, []any{"x", Comment("dropped"), "y"}); err != nil {
		t.Fatal(err)
	}
	got, _ := e.GetLines(pinsKey)
	if !reflect.DeepEqual(got, []any{"x", "y"}) {
		t.Errorf("GetLines = %#v, want the values alone", got)
	}
}

// An array inside an inline table has nowhere to put a comment line, so a Set
// that holds one is refused and leaves the document as it was, rather than
// writing the values without the comments that say what they are.
func TestCommentsInAnInlineTableAreRefused(t *testing.T) {
	src := "[tool]\nuv = { constraint-dependencies = ['x'] }\n"
	e, err := NewSurgical([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	err = e.Set(pinsKey, []any{"x", Comment("managed"), "y"})
	if !errors.Is(err, ErrCommentsUnsupported) {
		t.Fatalf("err = %v, want ErrCommentsUnsupported", err)
	}
	got, _ := e.Bytes()
	if string(got) != src {
		t.Errorf("the refused Set changed the document:\n%s", got)
	}
	if err := e.Set(pinsKey, []any{"x", "y"}); err != nil {
		t.Errorf("values alone: %v", err)
	}
}

// ArrayLines reads the comments out of bytes already decoded, without an
// editor.
func TestArrayLinesReadsBytesAlreadyDecoded(t *testing.T) {
	src := []byte("[tool.uv]\nconstraint-dependencies = [\n    'x',\n    # mine\n    'y',\n]\n")
	got := ArrayLines(src, pinsKey, []any{"x", "y"})
	if want := []any{"x", Comment("mine"), "y"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ArrayLines = %#v, want %#v", got, want)
	}
}

func TestACommentAloneIsRefused(t *testing.T) {
	for _, newEditor := range []func([]byte) (Editor, error){NewSurgical, NewRewrite} {
		e, err := newEditor([]byte("[tool.uv]\nconstraint-dependencies = ['x']\n"))
		if err != nil {
			t.Fatal(err)
		}
		if err := e.Set(append(pinsKey, "1"), Comment("c")); err == nil {
			t.Error("a Comment set on its own was accepted")
		}
	}
}
