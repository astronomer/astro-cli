package pagination

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// pagedSource serves items in pages of two, recording the offsets requested.
func pagedSource(items []int, offsets *[]int) FetchPage[int] {
	const pageSize = 2
	return func(offset int) ([]int, int, error) {
		*offsets = append(*offsets, offset)
		if offset >= len(items) {
			return nil, len(items), nil
		}
		end := offset + pageSize
		if end > len(items) {
			end = len(items)
		}
		return items[offset:end], len(items), nil
	}
}

func TestCollect(t *testing.T) {
	t.Run("aggregates across pages and advances offset", func(t *testing.T) {
		var offsets []int
		got, err := Collect(context.Background(), "ints", pagedSource([]int{1, 2, 3, 4, 5}, &offsets))
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3, 4, 5}, got)
		assert.Equal(t, []int{0, 2, 4}, offsets)
	})

	t.Run("single full page stops without an extra call", func(t *testing.T) {
		var offsets []int
		got, err := Collect(context.Background(), "ints", pagedSource([]int{1, 2}, &offsets))
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2}, got)
		assert.Equal(t, []int{0}, offsets)
	})

	t.Run("propagates fetch errors", func(t *testing.T) {
		sentinel := errors.New("boom")
		_, err := Collect(context.Background(), "ints", func(int) ([]int, int, error) { return nil, 0, sentinel })
		assert.ErrorIs(t, err, sentinel)
	})

	t.Run("aborts when the total is never reached (mis-reported total)", func(t *testing.T) {
		// Always returns a full page but claims a huge total, so it never terminates
		// naturally — the MaxListPages cap must stop it with an error.
		_, err := Collect(context.Background(), "ints", func(offset int) ([]int, int, error) {
			return []int{offset}, 1 << 30, nil
		})
		assert.ErrorContains(t, err, "aborted listing ints")
	})
}

func TestFind(t *testing.T) {
	t.Run("returns match on a later page and stops early", func(t *testing.T) {
		var offsets []int
		got, err := Find(context.Background(), "ints", pagedSource([]int{1, 2, 3, 4, 5}, &offsets), func(v int) bool { return v == 3 })
		assert.NoError(t, err)
		assert.NotNil(t, got)
		assert.Equal(t, 3, *got)
		// Found on page 2 (offsets 0 then 2); never requested page 3 (offset 4).
		assert.Equal(t, []int{0, 2}, offsets)
	})

	t.Run("returns nil when nothing matches", func(t *testing.T) {
		var offsets []int
		got, err := Find(context.Background(), "ints", pagedSource([]int{1, 2, 3}, &offsets), func(int) bool { return false })
		assert.NoError(t, err)
		assert.Nil(t, got)
	})
}

func TestCollectUpTo(t *testing.T) {
	t.Run("stops paging once the limit is reached and trims to it", func(t *testing.T) {
		var offsets []int
		got, err := CollectUpTo(context.Background(), "ints", 3, pagedSource([]int{1, 2, 3, 4, 5}, &offsets))
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3}, got)
		assert.Equal(t, []int{0, 2}, offsets, "the third page is never asked for")
	})

	t.Run("a limit beyond the total returns everything", func(t *testing.T) {
		var offsets []int
		got, err := CollectUpTo(context.Background(), "ints", 50, pagedSource([]int{1, 2, 3}, &offsets))
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3}, got)
	})

	t.Run("no limit is every item", func(t *testing.T) {
		var offsets []int
		got, err := CollectUpTo(context.Background(), "ints", 0, pagedSource([]int{1, 2, 3, 4, 5}, &offsets))
		assert.NoError(t, err)
		assert.Equal(t, []int{1, 2, 3, 4, 5}, got)
	})
}

// A canceled context stops the paging between pages and returns its error, not
// the pages read so far; a nil context pages as Background does.
func TestCollectHonorsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	pages := 0
	got, err := Collect(ctx, "ints", func(offset int) ([]int, int, error) {
		pages++
		cancel() // the interrupt arrives while page one is in flight
		return []int{offset}, 10, nil
	})
	assert.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, got, "no partial listing")
	assert.Equal(t, 1, pages, "no page is fetched after the cancel")

	_, err = Find(ctx, "ints", pagedSource([]int{1, 2, 3}, &[]int{}), func(int) bool { return true })
	assert.ErrorIs(t, err, context.Canceled)

	var offsets []int
	//nolint:staticcheck // a nil context is the case under test
	got, err = Collect(nil, "ints", pagedSource([]int{1, 2, 3}, &offsets))
	assert.NoError(t, err)
	assert.Equal(t, []int{1, 2, 3}, got)
}
