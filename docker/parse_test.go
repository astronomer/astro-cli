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
	dockerfile := "FROM quay.io/astronomer/astro-runtime:3.0.2\nCMD [\"echo\", 1]"
	_, err := ParseReader(bytes.NewBufferString(dockerfile))
	assert.IsType(t, ParseError{}, err)
}

func TestParseReader(t *testing.T) {
	dockerfile := `FROM quay.io/astronomer/astro-runtime:3.0.2`
	cmds, err := ParseReader(bytes.NewBufferString(dockerfile))
	assert.Nil(t, err)
	expected := []Command{
		{
			Cmd:       "from",
			Original:  "FROM quay.io/astronomer/astro-runtime:3.0.2",
			StartLine: 1,
			EndLine:   1,
			Flags:     []string{},
			Value:     []string{"quay.io/astronomer/astro-runtime:3.0.2"},
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
			Original:  "FROM quay.io/astronomer/astro-runtime:3.0.2",
			StartLine: 1,
			EndLine:   1,
			Flags:     []string{},
			Value:     []string{"quay.io/astronomer/astro-runtime:3.0.2"},
		},
	}
	assert.Equal(t, expected, cmds)
}

func TestGetImageFromParsedFile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cmds := []Command{
			{
				Cmd:       "from",
				Original:  "FROM quay.io/astronomer/astro-runtime:3.0.2",
				StartLine: 1,
				EndLine:   1,
				Flags:     []string{},
				Value:     []string{"quay.io/astronomer/astro-runtime:3.0.2"},
			},
		}
		image := GetImageFromParsedFile(cmds)
		assert.Equal(t, "quay.io/astronomer/astro-runtime:3.0.2", image)
	})

	t.Run("no image name found", func(t *testing.T) {
		cmds := []Command{
			{
				Cmd:       "echo",
				Original:  "echo $?",
				StartLine: 1,
				EndLine:   1,
				Flags:     []string{},
				Value:     []string{"$?"},
			},
		}
		image := GetImageFromParsedFile(cmds)
		assert.Equal(t, "", image)
	})
}

func TestGetImageTagFromParsedFile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cmds := []Command{
			{
				Cmd:       "from",
				Original:  "FROM quay.io/astronomer/astro-runtime:3.0.2",
				StartLine: 1,
				EndLine:   1,
				Flags:     []string{},
				Value:     []string{"quay.io/astronomer/astro-runtime:3.0.2"},
			},
		}
		image, tag := GetImageTagFromParsedFile(cmds)
		assert.Equal(t, "quay.io/astronomer/astro-runtime", image)
		assert.Equal(t, "3.0.2", tag)
	})

	t.Run("no image name found", func(t *testing.T) {
		cmds := []Command{
			{
				Cmd:       "echo",
				Original:  "echo $?",
				StartLine: 1,
				EndLine:   1,
				Flags:     []string{},
				Value:     []string{"$?"},
			},
		}
		image, tag := GetImageTagFromParsedFile(cmds)
		assert.Equal(t, "", image)
		assert.Equal(t, "", tag)
	})
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
