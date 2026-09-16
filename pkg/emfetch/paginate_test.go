package emfetch

import (
	"context"
	"errors"
	"math"
	"testing"
)

// rowsOf builds a window of n rows. The value carried does not matter to
// Paginate, only how many arrived.
func rowsOf(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// server serves a list of `size` rows, never more than `perWindow` at a time,
// and claims `claims` as the total. It records the offset of each request.
type server struct {
	size      int
	perWindow int
	claims    int
	offsets   []int
}

func (s *server) page(_ context.Context, offset, limit int) (rows []int, total int, err error) {
	s.offsets = append(s.offsets, offset)
	window := limit
	if s.perWindow > 0 && s.perWindow < window {
		window = s.perWindow
	}
	remaining := s.size - offset
	if remaining < 0 {
		remaining = 0
	}
	if remaining > window {
		remaining = window
	}
	return rowsOf(remaining), s.claims, nil
}

func (s *server) calls() int { return len(s.offsets) }

func TestPaginateReadsAShortListInOneWindow(t *testing.T) {
	s := &server{size: 3, claims: 3}
	rows, err := Paginate(context.Background(), s.page)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("got %d rows, want 3", len(rows))
	}
	// The claimed total ends the read, so no second request is spent
	// discovering that the list is over.
	if s.calls() != 1 {
		t.Errorf("read %d windows, want 1", s.calls())
	}
}

func TestPaginateAdvancesTheOffsetByRowsInHand(t *testing.T) {
	const size = 2*PageLimit + 500
	s := &server{size: size, claims: size}
	rows, err := Paginate(context.Background(), s.page)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != size {
		t.Errorf("got %d rows, want %d", len(rows), size)
	}
	want := []int{0, PageLimit, 2 * PageLimit}
	if len(s.offsets) != len(want) {
		t.Fatalf("read offsets %v, want %v", s.offsets, want)
	}
	for i := range want {
		if s.offsets[i] != want[i] {
			t.Fatalf("read offsets %v, want %v", s.offsets, want)
		}
	}
}

// A server free to cap its page size below the limit asked for returns a short
// first window for a list of any length. Ending the read there would return a
// sixth of this list with no error.
func TestPaginateReadsPastAWindowShorterThanTheLimit(t *testing.T) {
	s := &server{size: 3000, perWindow: 500, claims: 3000}
	rows, err := Paginate(context.Background(), s.page)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 3000 {
		t.Errorf("got %d rows, want the whole list of 3000", len(rows))
	}
	if s.calls() != 6 {
		t.Errorf("read %d windows, want 6 of 500", s.calls())
	}
}

// An absent totalCount decodes to zero in the response model, so a claimed
// total of zero has to mean "unknown" rather than "the list is empty".
func TestPaginateReadsAWholeListWhenNoTotalIsClaimed(t *testing.T) {
	s := &server{size: 3000, claims: 0}
	rows, err := Paginate(context.Background(), s.page)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 3000 {
		t.Errorf("got %d rows, want the whole list of 3000", len(rows))
	}
	// Three full windows, then the empty one that ends it.
	if s.calls() != 4 {
		t.Errorf("read %d windows, want 4", s.calls())
	}
}

// Having passed the claimed total is evidence the claim was wrong, so the read
// keeps going rather than stopping on a number it has already exceeded.
func TestPaginateReadsPastATotalLowerThanTheList(t *testing.T) {
	s := &server{size: 3000, claims: 5}
	rows, err := Paginate(context.Background(), s.page)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 3000 {
		t.Errorf("got %d rows, want the whole list of 3000", len(rows))
	}
}

func TestPaginateStopsOnAnEmptyFirstWindow(t *testing.T) {
	s := &server{size: 0, claims: 100}
	rows, err := Paginate(context.Background(), s.page)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
	if s.calls() != 1 {
		t.Errorf("read %d windows, want 1", s.calls())
	}
}

// The bound is an error a caller can match, and it yields no rows: a caller
// handed a truncated list with a nil error would treat a partial answer as the
// whole one.
func TestPaginateReportsTheBoundAndDropsRows(t *testing.T) {
	s := &server{size: math.MaxInt, claims: math.MaxInt}
	rows, err := Paginate(context.Background(), s.page)
	if !errors.Is(err, ErrTooManyPages) {
		t.Fatalf("got error %v, want it to match ErrTooManyPages", err)
	}
	if rows != nil {
		t.Errorf("got %d rows alongside the error, want none", len(rows))
	}
	if s.calls() != MaxPages {
		t.Errorf("read %d windows, want the %d-page bound", s.calls(), MaxPages)
	}
}

func TestPaginateReturnsAWindowsErrorAndDropsWhatItHad(t *testing.T) {
	wantErr := errors.New("the workspace no longer exists")
	calls := 0
	rows, err := Paginate(context.Background(), func(_ context.Context, _, limit int) ([]int, int, error) {
		calls++
		if calls == 1 {
			return rowsOf(limit), 5 * PageLimit, nil
		}
		return nil, 0, wantErr
	})
	// Compared by identity rather than errors.Is: the assertion is that the
	// error arrives unwrapped, which errors.Is would not distinguish.
	if err != wantErr {
		t.Fatalf("got error %v, want it returned unchanged", err)
	}
	if rows != nil {
		t.Errorf("got %d rows alongside the error, want none", len(rows))
	}
}

// Paginate takes a context, so it honors one rather than leaving cancellation
// to whether a caller's closure happens to thread it through.
func TestPaginateStopsOnACanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := &server{size: 3000, claims: 3000}
	rows, err := Paginate(ctx, s.page)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got error %v, want context.Canceled", err)
	}
	if rows != nil {
		t.Errorf("got %d rows, want none", len(rows))
	}
	if s.calls() != 0 {
		t.Errorf("read %d windows on a canceled context, want 0", s.calls())
	}
}

func TestPaginateStopsPartwayOnCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	rows, err := Paginate(ctx, func(_ context.Context, _, limit int) ([]int, int, error) {
		calls++
		if calls == 2 {
			cancel()
		}
		return rowsOf(limit), math.MaxInt, nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got error %v, want context.Canceled", err)
	}
	if rows != nil {
		t.Errorf("got %d rows, want none", len(rows))
	}
	if calls != 2 {
		t.Errorf("read %d windows, want the read to stop after the canceling one", calls)
	}
}
