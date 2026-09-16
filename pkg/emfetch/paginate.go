package emfetch

import (
	"context"
	"errors"
	"fmt"
)

const (
	// PageLimit is the number of rows requested per window. A server may serve
	// fewer, and the paging does not assume it serves this many.
	PageLimit = 1000
	// MaxPages bounds one read, so a server that keeps serving rows cannot hold
	// a caller in the loop indefinitely.
	MaxPages = 100
)

// ErrTooManyPages reports that a read stopped at the MaxPages bound with rows
// still arriving, so the list it would have returned is incomplete.
var ErrTooManyPages = errors.New("too many pages")

// Paginate accumulates every row of an offset-paginated list endpoint.
//
// page reads one window at the given offset and reports the rows in it along
// with the total the server claims for the whole list. The offset advances by
// the rows in hand, so a server free to serve fewer rows than the limit asked
// for is paged correctly.
//
// A read ends on an empty window, or when the rows in hand are exactly the
// claimed total. Those two conditions are chosen together, and neither is the
// obvious one:
//
//   - A window shorter than the limit does not end the read. A server that caps
//     its page size below PageLimit returns a short first window for a list of
//     any length, and treating that as the end truncates the list to one page.
//   - A total is believed only when the rows in hand match it exactly. Reaching
//     a total is the cheap ending, and it saves the extra request the empty
//     window costs, but a total of zero (which is what an absent field decodes
//     to) or one lower than the list would otherwise end the read early. Having
//     passed the claimed total is evidence the claim was wrong, so the read
//     continues to the empty window instead.
//
// Exhausting MaxPages returns ErrTooManyPages and no rows. A bounded read and a
// complete one are indistinguishable from the rows alone, so returning what
// arrived would hand a caller a silently short answer to treat as the whole
// list.
func Paginate[T any](ctx context.Context, page func(ctx context.Context, offset, limit int) (rows []T, total int, err error)) ([]T, error) {
	var all []T
	for range MaxPages {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		rows, total, err := page(ctx, len(all), PageLimit)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return all, nil
		}
		all = append(all, rows...)
		if total > 0 && len(all) == total {
			return all, nil
		}
	}
	return nil, fmt.Errorf("%w: stopped after %d windows with %d rows in hand and more arriving", ErrTooManyPages, MaxPages, len(all))
}
