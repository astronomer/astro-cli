package airflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetFmtEnvFileSuccess(t *testing.T) {
	resp, err := getFmtEnvFile("testfiles/env.test.valid", "podman")
	validLines := []string{"- name: test1", "value: \"1\"", "- name: test2", "value: astro", "- name: test3", "value: astro123"}
	for idx := range validLines {
		assert.Contains(t, resp, validLines[idx])
	}
	assert.NoError(t, err)

	resp, err = getFmtEnvFile("testfiles/env.test.valid", "docker")
	assert.NoError(t, err)
	assert.Equal(t, "env_file: testfiles/env.test.valid", resp)
}
