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
	dockerfile := "FROM quay.io/astronomer/ap-airflow:2.0.0-buster-onbuild\nCMD [\"echo\", 1]"
	_, err := ParseReader(bytes.NewBufferString(dockerfile))
	assert.IsType(t, ParseError{}, err)
}

func TestParseReader(t *testing.T) {
	dockerfile := `FROM quay.io/astronomer/ap-airflow:2.0.0-buster-onbuild`
	cmds, err := ParseReader(bytes.NewBufferString(dockerfile))
	assert.Nil(t, err)
	expected := []Command{
		{
			Cmd:       "FROM",
			Original:  "FROM quay.io/astronomer/ap-airflow:2.0.0-buster-onbuild",
			StartLine: 1,
			EndLine:   1,
			Flags:     []string{},
			Value:     []string{"quay.io/astronomer/ap-airflow:2.0.0-buster-onbuild"},
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
			Cmd:       "FROM",
			Original:  "FROM quay.io/astronomer/ap-airflow:2.0.0-buster-onbuild",
			StartLine: 1,
			EndLine:   1,
			Flags:     []string{},
			Value:     []string{"quay.io/astronomer/ap-airflow:2.0.0-buster-onbuild"},
		},
	}
	assert.Equal(t, expected, cmds)
}

func TestGetImageTagFromParsedFile(t *testing.T) {
	cmds := []Command{
		{
			Cmd:       "from",
			Original:  "FROM quay.io/astronomer/ap-airflow:2.0.0-buster-onbuild",
			StartLine: 1,
			EndLine:   1,
			Flags:     []string{},
			Value:     []string{"quay.io/astronomer/ap-airflow:2.0.0-buster-onbuild"},
		},
	}
	image, tag := GetImageTagFromParsedFile(cmds)
	assert.Equal(t, "quay.io/astronomer/ap-airflow", image)
	assert.Equal(t, "2.0.0-buster-onbuild", tag)
}

func TestParseImageName(t *testing.T) {
	tests := []struct {
		imageName         string
		expectedBaseImage string
		expectedTag       string
	}{
		{imageName: "localhost:5000/airflow:latest", expectedBaseImage: "localhost:5000/airflow", expectedTag: "latest"},
		{imageName: "localhost/airflow:1.2.0", expectedBaseImage: "localhost/airflow", expectedTag: "1.2.0"},
		{imageName: "quay.io:5000/airflow", expectedBaseImage: "quay.io:5000/airflow", expectedTag: "latest"},
		{imageName: "airflow:latest", expectedBaseImage: "airflow", expectedTag: "latest"},
		{imageName: "airflow", expectedBaseImage: "airflow", expectedTag: "latest"},
	}

	for _, tt := range tests {
		baseImage, tag := parseImageName(tt.imageName)
		assert.Equal(t, tt.expectedBaseImage, baseImage)
		assert.Equal(t, tt.expectedTag, tag)
	}
}
