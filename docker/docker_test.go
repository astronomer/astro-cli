package docker

import "testing"

func TestExecVersion(t *testing.T) {
	err := Exec("version")
	if err != nil {
		t.Error(err)
	}
}
