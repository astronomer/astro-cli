package deploy

import "testing"

type askingDeployer struct {
	Deployer
	log *[]string
}

func (a askingDeployer) ConfirmTarget([]Choice, string) (string, error) {
	*a.log = append(*a.log, "confirm")
	return "prod", nil
}

func (a askingDeployer) ResolveUnlinked(string) (Deployment, error) {
	*a.log = append(*a.log, "resolve")
	return Deployment{}, nil
}

// Each question is preceded by the hook, so held warnings print before it.
func TestBeforeAskingRunsBeforeEachQuestion(t *testing.T) {
	var log []string
	d := BeforeAsking(askingDeployer{log: &log}, func() { log = append(log, "before") })
	if _, err := d.ConfirmTarget(nil, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ResolveUnlinked("ws"); err != nil {
		t.Fatal(err)
	}
	want := []string{"before", "confirm", "before", "resolve"}
	if len(log) != len(want) {
		t.Fatalf("log = %v, want %v", log, want)
	}
	for i := range want {
		if log[i] != want[i] {
			t.Fatalf("log = %v, want %v", log, want)
		}
	}
}
