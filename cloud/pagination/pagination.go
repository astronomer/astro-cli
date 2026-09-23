// Package pagination provides helpers for reading every result from the paged
// list endpoints of the Astro API, so callers don't each reimplement the loop.
package pagination

import "fmt"

// MaxListPages caps a single paginated list call as a safety bound against a
// server that mis-reports the total count.
const MaxListPages = 100

// FetchPage returns one page of items starting at offset, along with the total
// number of items available across all pages.
type FetchPage[T any] func(offset int) (items []T, total int, err error)

// Collect pages through every item returned by fetch. It advances the offset by
// the number of items returned and stops on an empty page or once the reported
// total is reached, erroring if MaxListPages is exceeded. what names the entity
// being listed, for the error message (e.g. "workspaces").
func Collect[T any](what string, fetch FetchPage[T]) ([]T, error) {
	var all []T
	offset := 0
	for page := 0; page < MaxListPages; page++ {
		items, total, err := fetch(offset)
		if err != nil {
			return nil, err
		}
		all = append(all, items...)
		if len(items) == 0 || len(all) >= total {
			return all, nil
		}
		offset = len(all)
	}
	return nil, fmt.Errorf("aborted listing %s after %d pages", what, MaxListPages)
}

// Find pages through the items returned by fetch and returns the first one for
// which pred is true, without fetching later pages once a match is found. It
// returns (nil, nil) when no item matches.
func Find[T any](what string, fetch FetchPage[T], pred func(T) bool) (*T, error) {
	scanned := 0
	for page := 0; page < MaxListPages; page++ {
		items, total, err := fetch(scanned)
		if err != nil {
			return nil, err
		}
		for i := range items {
			if pred(items[i]) {
				return &items[i], nil
			}
		}
		scanned += len(items)
		if len(items) == 0 || scanned >= total {
			return nil, nil
		}
	}
	return nil, fmt.Errorf("aborted listing %s after %d pages", what, MaxListPages)
}
