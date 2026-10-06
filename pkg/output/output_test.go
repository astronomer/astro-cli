package output

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	testUtil "github.com/astronomer/astro-cli/pkg/testing"
)

type testItem struct {
	Name      string
	ID        string
	IsCurrent bool
}

type testList struct {
	Items []testItem
}

// text renders data through PrintData in text mode.
func text(t *testing.T, data *testList, cfg *TableConfig) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	err := PrintData(func() (*testList, error) { return data, nil }, cfg, testUtil.Renderer{Out: &buf})
	return buf.String(), err
}

func TestPrintTable(t *testing.T) {
	data := &testList{
		Items: []testItem{
			{Name: "alpha", ID: "id-1", IsCurrent: false},
			{Name: "beta", ID: "id-2", IsCurrent: true},
		},
	}

	t.Run("renders table with dynamic padding", func(t *testing.T) {
		cfg := BuildTableConfig(
			[]Column[testItem]{
				{Header: "NAME", Value: func(i testItem) string { return i.Name }},
				{Header: "ID", Value: func(i testItem) string { return i.ID }},
			},
			func(d any) []testItem { return d.(*testList).Items },
		)

		out, err := text(t, data, cfg)
		assert.NoError(t, err)

		assert.Contains(t, out, "NAME")
		assert.Contains(t, out, "ID")
		assert.Contains(t, out, "alpha")
		assert.Contains(t, out, "id-2")
	})

	t.Run("applies row coloring", func(t *testing.T) {
		cfg := BuildTableConfig(
			[]Column[testItem]{
				{Header: "NAME", Value: func(i testItem) string { return i.Name }},
			},
			func(d any) []testItem { return d.(*testList).Items },
			WithColorRow(func(i testItem) bool { return i.IsCurrent }, [2]string{"\033[1;32m", "\033[0m"}),
		)

		out, err := text(t, data, cfg)
		assert.NoError(t, err)

		assert.Contains(t, out, "\033[1;32m") // color code present for "beta"
		assert.Contains(t, out, "beta")
	})

	t.Run("shows no results message", func(t *testing.T) {
		emptyData := &testList{Items: []testItem{}}
		cfg := BuildTableConfig(
			[]Column[testItem]{
				{Header: "NAME", Value: func(i testItem) string { return i.Name }},
			},
			func(d any) []testItem { return d.(*testList).Items },
			WithNoResultsMsg("No items found"),
		)

		out, err := text(t, emptyData, cfg)
		assert.NoError(t, err)
		assert.Contains(t, out, "No items found")
	})

	t.Run("errors without table config", func(t *testing.T) {
		_, err := text(t, data, nil)
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
			cfg, testUtil.Renderer{JSON: true, Out: &buf},
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
			cfg, testUtil.Renderer{Out: &buf},
		)
		require.NoError(t, err)
		assert.Contains(t, buf.String(), "alpha")
	})

	t.Run("propagates fetch error", func(t *testing.T) {
		var buf bytes.Buffer
		err := PrintData(
			func() (*testList, error) { return nil, assert.AnError },
			cfg, testUtil.Renderer{JSON: true, Out: &buf},
		)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
