package docker

import (
	"bytes"
)

func (s *Suite) TestAllCmds() {
	ret := AllCmds()
	s.Equal(ret[:3], []string{"add", "arg", "cmd"})
}

func (s *Suite) TestParseReaderParseError() {
	dockerfile := "FROM quay.io/astronomer/astro-runtime:3.0.2\nCMD [\"echo\", 1]"
	_, err := ParseReader(bytes.NewBufferString(dockerfile))
	s.IsType(ParseError{}, err)
}

func (s *Suite) TestParseReader() {
	dockerfile := `FROM quay.io/astronomer/astro-runtime:3.0.2`
	cmds, err := ParseReader(bytes.NewBufferString(dockerfile))
	s.NoError(err)
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
	s.Equal(expected, cmds)
}

func (s *Suite) TestParseFileIOError() {
	_, err := ParseFile("Dockerfile.dne")
	s.IsType(IOError{}, err)
	s.Regexp("^.*Dockerfile.dne.*$", err.Error())
}

func (s *Suite) TestParseFile() {
	cmds, err := ParseFile("testfiles/Dockerfile.ok")
	s.NoError(err)
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
	s.Equal(expected, cmds)
}

func (s *Suite) TestGetImageFromParsedFile() {
	s.Run("success", func() {
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
		s.Equal("quay.io/astronomer/astro-runtime:3.0.2", image)
	})

	s.Run("no image name found", func() {
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
		s.Equal("", image)
	})
}

func (s *Suite) TestGetImageTagFromParsedFile() {
	s.Run("success", func() {
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
		s.Equal("quay.io/astronomer/astro-runtime", image)
		s.Equal("3.0.2", tag)
	})

	s.Run("no image name found", func() {
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
		s.Equal("", image)
		s.Equal("", tag)
	})
}

func (s *Suite) TestParseImageName() {
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
		s.Equal(tt.expectedBaseImage, baseImage)
		s.Equal(tt.expectedTag, tag)
	}
}
