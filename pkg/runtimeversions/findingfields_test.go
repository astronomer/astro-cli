package runtimeversions

import (
	"encoding/json"
	"strings"
	"testing"
)

// The structured fields carry the facts the Message is written from, so a
// caller can phrase a finding itself without asking the catalog again.
func TestFindingCarriesItsFacts(t *testing.T) {
	c := parse(t, checkCatalog)
	cases := []struct {
		name, runtime, pin string
		want               Finding
	}{
		{
			name: "excluded patch", runtime: "3.3-8", pin: "3.3.1",
			want: Finding{Kind: FindingAirflowExcluded, Runtime: "3.3-8", AirflowPin: "3.3.1", AirflowVersion: "3.3.2"},
		},
		{
			name: "yanked", runtime: "3.2-1", pin: "3.2",
			want: Finding{
				Kind: FindingYanked, Runtime: "3.2-1", AirflowPin: "3.2", AirflowVersion: "3.2.0",
				YankedReason: "This version has issues with environment manager connections not being found",
			},
		},
		{
			name: "series mismatch", runtime: "13.11.0", pin: "2.10",
			want: Finding{
				Kind: FindingSeriesMismatch, Blocking: true, Runtime: "13.11.0", AirflowPin: "2.10",
				AirflowVersion: "2.11.2", Suggested: "12.9.0",
			},
		},
		{
			name: "unknown", runtime: "3.3-99", pin: "3.3",
			want: Finding{Kind: FindingUnknown, Runtime: "3.3-99", AirflowPin: "3.3"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := c.CheckRuntime(tc.runtime, tc.pin)
			if len(got) != 1 {
				t.Fatalf("findings = %+v, want one", got)
			}
			f := got[0]
			if f.Message == "" {
				t.Error("the Message is kept")
			}
			f.Message = ""
			if f != tc.want {
				t.Errorf("finding = %+v, want %+v", f, tc.want)
			}
		})
	}

	var none *Catalog
	got := none.CheckRuntime("13.11.0", "2.10")
	if len(got) != 1 || got[0].Runtime != "13.11.0" || got[0].AirflowPin != "2.10" || got[0].AirflowVersion != "" {
		t.Errorf("offline finding = %+v, want the runtime and pin and no carried version", got)
	}
}

func TestFindingJSONIsAdditive(t *testing.T) {
	b, err := json.Marshal(Finding{Kind: FindingUnknown, Message: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if s := string(b); s != `{"kind":"runtime_unknown","message":"m","blocking":false}` || strings.Contains(s, "yanked") {
		t.Errorf("empty facts serialize as %s, want them omitted", s)
	}
}
