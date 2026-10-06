package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrinter_PrintJSON(t *testing.T) {
	data := map[string]any{
		"name":  "test",
		"count": 42,
		"tags":  []string{"a", "b", "c"},
	}

	var buf bytes.Buffer
	p := New(Options{
		Format:  FormatJSON,
		Out:     &buf,
		NoColor: true,
	})

	err := p.Print(data)
	require.NoError(t, err)

	// Verify it's valid JSON
	var result map[string]any
	err = json.Unmarshal(buf.Bytes(), &result)
	require.NoError(t, err)
	assert.Equal(t, "test", result["name"])
	assert.Equal(t, float64(42), result["count"]) // JSON numbers are float64
}

// TestPrinter_PrintJSONBytes pins the exact bytes -o json writes: two-space
// indent, no HTML escaping, one trailing newline. Scripts and Astro Desktop
// parse this, so a change here is a contract change.
func TestPrinter_PrintJSONBytes(t *testing.T) {
	type item struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	type list struct {
		Items []item `json:"items"`
	}

	var buf bytes.Buffer
	err := New(Options{Format: FormatJSON, Out: &buf, NoColor: true}).Print(&list{
		Items: []item{{Name: "a<b>&c", URL: "https://example.com/?x=1&y=2"}},
	})
	require.NoError(t, err)
	assert.Equal(t, `{
  "items": [
    {
      "name": "a<b>&c",
      "url": "https://example.com/?x=1&y=2"
    }
  ]
}
`, buf.String())
}

func TestOptions_GetOut(t *testing.T) {
	t.Run("returns configured writer", func(t *testing.T) {
		var buf bytes.Buffer
		opts := Options{Out: &buf}
		assert.Equal(t, &buf, opts.GetOut())
	})
}

type testItem struct {
	Name      string
	ID        string
	IsCurrent bool
}

type testList struct {
	Items []testItem
}

func TestPrinter_PrintTable(t *testing.T) {
	data := &testList{
		Items: []testItem{
			{Name: "alpha", ID: "id-1", IsCurrent: false},
			{Name: "beta", ID: "id-2", IsCurrent: true},
		},
	}

	t.Run("renders table with dynamic padding", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := BuildTableConfig(
			[]Column[testItem]{
				{Header: "NAME", Value: func(i testItem) string { return i.Name }},
				{Header: "ID", Value: func(i testItem) string { return i.ID }},
			},
			func(d any) []testItem { return d.(*testList).Items },
		)

		p := New(Options{Format: FormatText, Out: &buf, Table: cfg})
		err := p.Print(data)
		assert.NoError(t, err)

		out := buf.String()
		assert.Contains(t, out, "NAME")
		assert.Contains(t, out, "ID")
		assert.Contains(t, out, "alpha")
		assert.Contains(t, out, "id-2")
	})

	t.Run("applies row coloring", func(t *testing.T) {
		var buf bytes.Buffer
		cfg := BuildTableConfig(
			[]Column[testItem]{
				{Header: "NAME", Value: func(i testItem) string { return i.Name }},
			},
			func(d any) []testItem { return d.(*testList).Items },
			WithColorRow(func(i testItem) bool { return i.IsCurrent }, [2]string{"\033[1;32m", "\033[0m"}),
		)

		p := New(Options{Format: FormatText, Out: &buf, Table: cfg})
		err := p.Print(data)
		assert.NoError(t, err)

		out := buf.String()
		assert.Contains(t, out, "\033[1;32m") // color code present for "beta"
		assert.Contains(t, out, "beta")
	})

	t.Run("shows no results message", func(t *testing.T) {
		var buf bytes.Buffer
		emptyData := &testList{Items: []testItem{}}
		cfg := BuildTableConfig(
			[]Column[testItem]{
				{Header: "NAME", Value: func(i testItem) string { return i.Name }},
			},
			func(d any) []testItem { return d.(*testList).Items },
			WithNoResultsMsg("No items found"),
		)

		p := New(Options{Format: FormatText, Out: &buf, Table: cfg})
		err := p.Print(emptyData)
		assert.NoError(t, err)
		assert.Contains(t, buf.String(), "No items found")
	})

	t.Run("errors without table config", func(t *testing.T) {
		var buf bytes.Buffer
		p := New(Options{Format: FormatText, Out: &buf})
		err := p.Print(data)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "table config required")
	})
}

func TestBuildTableConfig(t *testing.T) {
	t.Run("wraps columns correctly", func(t *testing.T) {
		cfg := BuildTableConfig(
			[]Column[testItem]{
				{Header: "NAME", Value: func(i testItem) string { return i.Name }},
				{Header: "ID", Value: func(i testItem) string { return i.ID }},
			},
			func(d any) []testItem { return d.(*testList).Items },
		)

		assert.Len(t, cfg.Columns, 2)
		assert.Equal(t, "NAME", cfg.Columns[0].Header)
		assert.Equal(t, "ID", cfg.Columns[1].Header)

		// Verify column extractors work
		item := testItem{Name: "test", ID: "123"}
		assert.Equal(t, "test", cfg.Columns[0].Value(item))
		assert.Equal(t, "123", cfg.Columns[1].Value(item))
	})

	t.Run("applies options", func(t *testing.T) {
		cfg := BuildTableConfig(
			[]Column[testItem]{
				{Header: "NAME", Value: func(i testItem) string { return i.Name }},
			},
			func(d any) []testItem { return d.(*testList).Items },
			WithPadding([]int{30, 50}),
			WithNoResultsMsg("empty"),
		)

		assert.Equal(t, []int{30, 50}, cfg.Padding)
		assert.Equal(t, "empty", cfg.NoResultsMsg)
	})
}

func TestPrintData(t *testing.T) {
	data := &testList{
		Items: []testItem{
			{Name: "alpha", ID: "id-1"},
			{Name: "beta", ID: "id-2"},
		},
	}

	cfg := BuildTableConfig(
		[]Column[testItem]{
			{Header: "NAME", Value: func(i testItem) string { return i.Name }},
			{Header: "ID", Value: func(i testItem) string { return i.ID }},
		},
		func(d any) []testItem { return d.(*testList).Items },
	)

	t.Run("json output", func(t *testing.T) {
		var buf bytes.Buffer
		err := PrintData(
			func() (*testList, error) { return data, nil },
			cfg, FormatJSON, &buf,
		)
		require.NoError(t, err)

		var result testList
		require.NoError(t, json.Unmarshal(buf.Bytes(), &result))
		assert.Equal(t, "alpha", result.Items[0].Name)
	})

	t.Run("text output", func(t *testing.T) {
		var buf bytes.Buffer
		err := PrintData(
			func() (*testList, error) { return data, nil },
			cfg, FormatText, &buf,
		)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "alpha")
	})

	t.Run("propagates fetch error", func(t *testing.T) {
		var buf bytes.Buffer
		err := PrintData(
			func() (*testList, error) { return nil, assert.AnError },
			cfg, FormatJSON, &buf,
		)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
