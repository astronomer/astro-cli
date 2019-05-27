package docker

import "testing"

func TestExecVersion(t *testing.T) {
	err := Exec("version")
	if err != nil {
		t.Error(err)
	}
}

func TestLoginFailed(t *testing.T) {
	err := ExecLogin("https://quay.io/v1", "", "")
	if err == nil {
		t.Error(err)
	}
}
