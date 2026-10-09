package docker

import (
	"bytes"
)

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
			Cmd:       "FROM",
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
			Cmd:       "FROM",
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
	s.Run("success with real parser output", func() {
		dockerfile := `FROM quay.io/astronomer/astro-runtime:3.0.2`
		cmds, err := ParseReader(bytes.NewBufferString(dockerfile))
		s.NoError(err)

		image := GetImageFromParsedFile(cmds)
		s.Equal("quay.io/astronomer/astro-runtime:3.0.2", image)
	})

	s.Run("success with artificial lowercase cmd (legacy)", func() {
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

	s.Run("success with uppercase cmd", func() {
		cmds := []Command{
			{
				Cmd:       "FROM",
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

	s.Run("empty value array should not panic", func() {
		cmds := []Command{
			{
				Cmd:       "FROM",
				Original:  "FROM",
				StartLine: 1,
				EndLine:   1,
				Flags:     []string{},
				Value:     []string{}, // Empty value array - this would panic without bounds check
			},
		}
		// This should return empty string, not panic
		image := GetImageFromParsedFile(cmds)
		s.Equal("", image)
	})
}
