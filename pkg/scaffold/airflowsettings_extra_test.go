package scaffold

import (
	"strings"
	"testing"
)

// A conn_extra that is not valid JSON is refused without quoting any of it:
// the decoder's error names the byte it stopped on, which can be a secret's.
func TestConnExtraErrorQuotesNoneOfTheExtra(t *testing.T) {
	for _, extra := range []string{
		`{"token": "s3cr3tXYZ" oops}`,
		`s3cr3tXYZ`,
		`{"token": "s3cr3tXYZ"`,
		`{"token": Qs3cr3t}`,
	} {
		_, err := connExtra(extra)
		if err == nil {
			t.Fatalf("connExtra(%d bytes) accepted invalid JSON", len(extra))
		}
		if err.Error() != "conn_extra is neither a mapping nor JSON" {
			t.Fatalf("error for a %d-byte extra carries more than the fixed message", len(extra))
		}
		for _, frag := range []string{"s3cr3t", "XYZ", "oops", "Q"} {
			if strings.Contains(err.Error(), frag) {
				t.Fatalf("error for a %d-byte extra quotes part of it (%d bytes of message)", len(extra), len(err.Error()))
			}
		}
	}
}
