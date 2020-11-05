package docker

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAllCmds(t *testing.T) {
	ret := AllCmds()
	assert.Equal(t, ret[:3], []string{"add", "arg", "cmd"})
}

func TestParseReaderParseError(t *testing.T) {
	dockerfile := "FROM quay.io/astronomer/ap-airflow:latest-onbuild\nCMD [\"echo\", 1]"
	_, err := ParseReader(bytes.NewBufferString(dockerfile))
	assert.IsType(t, ParseError{}, err)
}

func TestParseReader(t *testing.T) {
	dockerfile := `FROM quay.io/astronomer/ap-airflow:latest-onbuild`
	cmds, err := ParseReader(bytes.NewBufferString(dockerfile))
	assert.Nil(t, err)
	expected := []Command{
		{
			Cmd:       "from",
			Original:  "FROM quay.io/astronomer/ap-airflow:latest-onbuild",
			StartLine: 1,
			EndLine:   1,
			Flags:     []string{},
			Value:     []string{"quay.io/astronomer/ap-airflow:latest-onbuild"},
		},
	}
	assert.Equal(t, expected, cmds)
}

func TestParseFileIOError(t *testing.T) {
	_, err := ParseFile("Dockerfile.dne")
	assert.IsType(t, IOError{}, err)
	assert.Regexp(t, "^.*Dockerfile.dne.*$", err.Error())
}

func TestParseFile(t *testing.T) {
	cmds, err := ParseFile("testfiles/Dockerfile.ok")
	assert.Nil(t, err)
	expected := []Command{
		{
			Cmd:       "from",
			Original:  "FROM quay.io/astronomer/ap-airflow:latest-onbuild",
			StartLine: 1,
			EndLine:   1,
			Flags:     []string{},
			Value:     []string{"quay.io/astronomer/ap-airflow:latest-onbuild"},
		},
	}
	assert.Equal(t, expected, cmds)
}

func TestGetImageTagFromParsedFile(t *testing.T) {
	cmds := []Command{
		{
			Cmd:       "from",
			Original:  "FROM quay.io/astronomer/ap-airflow:latest-onbuild",
			StartLine: 1,
			EndLine:   1,
			Flags:     []string{},
			Value:     []string{"quay.io/astronomer/ap-airflow:latest-onbuild"},
		},
	}
	image, tag := GetImageTagFromParsedFile(cmds)
	assert.Equal(t, "quay.io/astronomer/ap-airflow", image)
	assert.Equal(t, "latest-onbuild", tag)
}
