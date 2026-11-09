package team

import (
	"errors"
	"fmt"

	"github.com/astronomer/astro-cli/pkg/input"
)

// A team picker that failed is an invalid team, unless the run refused to
// ask or was interrupted at it: Ctrl-C at the picker is the interrupt.
func (s *Suite) TestPickFailedPassesTheInterruptThrough() {
	s.ErrorIs(pickFailed(fmt.Errorf("picking: %w", input.ErrInterrupted)), input.ErrInterrupted)
	refused := input.Required(errors.New("pass --team-id"))
	s.Same(refused, pickFailed(refused))
	s.ErrorIs(pickFailed(errors.New("3 is not a choice")), ErrInvalidTeamKey)
	s.ErrorIs(pickFailed(nil), ErrInvalidTeamKey, "no team picked")
}
