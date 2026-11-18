package usageerr

import (
	"errors"
	"fmt"
	"testing"
)

func TestUsageMarksAndUnwraps(t *testing.T) {
	cause := errors.New("bad invocation")
	err := fmt.Errorf("wrapped: %w", Usage(cause))

	var m Marker
	if !errors.As(err, &m) || !m.IsUsageError() {
		t.Fatalf("%v is not marked as a usage error", err)
	}
	var o HelpOptOut
	if errors.As(err, &o) {
		t.Fatalf("Usage opts out of the help pointer")
	}
	if !errors.Is(err, cause) || Usage(cause).Error() != cause.Error() {
		t.Fatalf("Usage changed the error: %v", Usage(cause))
	}
}

func TestNoHelpOptsOutOfThePointer(t *testing.T) {
	cause := errors.New("use the other verb")
	err := fmt.Errorf("wrapped: %w", NoHelp(cause))

	var m Marker
	var o HelpOptOut
	if !errors.As(err, &m) || !m.IsUsageError() {
		t.Fatalf("%v is not marked as a usage error", err)
	}
	if !errors.As(err, &o) || !o.OmitsHelpPointer() {
		t.Fatalf("%v does not opt out of the help pointer", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("NoHelp does not unwrap to its cause")
	}
}

func TestNilStaysNil(t *testing.T) {
	if Usage(nil) != nil || NoHelp(nil) != nil {
		t.Fatal("a nil error was given a mark")
	}
}
