// Parse a dockerfile into a high-level representation using the official go parser
package docker

import (
	"io"
	"os"
	"sort"
	"strings"

	"github.com/distribution/reference"
	"github.com/moby/buildkit/frontend/dockerfile/command"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
)

// Represents a single line (layer) in a Dockerfile.
// For example `FROM ubuntu:xenial`
type Command struct {
	Cmd       string   // lower cased command name (ex: `from`)
	SubCmd    string   // for ONBUILD only this holds the sub-command
	JSON      bool     // whether the value is written in json form
	Original  string   // The original source line
	StartLine int      // The original source line number which starts this command
	EndLine   int      // The original source line number which ends this command
	Flags     []string // Any flags such as `--from=...` for `COPY`.
	Value     []string // The contents of the command (ex: `ubuntu:xenial`)
}

// A failure in opening a file for reading.
type IOError struct {
	Msg string
}

func (e IOError) Error() string {
	return e.Msg
}

// A failure in parsing the file as a dockerfile.
type ParseError struct {
	Msg string
}

func (e ParseError) Error() string {
	return e.Msg
}

// List all legal cmds in a dockerfile
func AllCmds() []string {
	ret := make([]string, 0, len(command.Commands))
	for k := range command.Commands {
		ret = append(ret, k)
	}
	sort.Strings(ret)
	return ret
}

// Parse a Dockerfile from a reader.  A ParseError may occur.
func ParseReader(file io.Reader) ([]Command, error) {
	res, err := parser.Parse(file)
	if err != nil {
		return nil, ParseError{err.Error()}
	}

	ret := make([]Command, 0, len(res.AST.Children))
	for _, child := range res.AST.Children {
		cmd := Command{
			Cmd:       child.Value,
			Original:  child.Original,
			StartLine: child.StartLine,
			EndLine:   child.EndLine,
			Flags:     child.Flags,
		}

		// Only happens for ONBUILD
		if child.Next != nil && len(child.Next.Children) > 0 {
			cmd.SubCmd = child.Next.Children[0].Value
			child = child.Next.Children[0]
		}

		cmd.JSON = child.Attributes["json"]
		for n := child.Next; n != nil; n = n.Next {
			cmd.Value = append(cmd.Value, n.Value)
		}

		ret = append(ret, cmd)
	}
	return ret, nil
}

// ParseFile a Dockerfile from a filename.  An IOError or ParseError may occur.
var ParseFile = func(filename string) ([]Command, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, IOError{err.Error()}
	}
	defer file.Close()

	return ParseReader(file)
}

// Parse image from parsed dockerfile:
// e.g. FROM ubuntu:xenial returns "ubuntu:xenial"
func GetImageFromParsedFile(cmds []Command) (image string) {
	for _, cmd := range cmds {
		if strings.EqualFold(cmd.Cmd, command.From) {
			if len(cmd.Value) > 0 {
				from := cmd.Value[0]
				return from
			}
		}
	}
	return ""
}

// Parse tag from parsed dockerfile:
// e.g. FROM ubuntu:xenial returns "ubuntu", "xenial"
func GetImageTagFromParsedFile(cmds []Command) (baseImage, tag string) {
	for _, cmd := range cmds {
		if strings.EqualFold(cmd.Cmd, command.From) {
			if len(cmd.Value) > 0 {
				from := cmd.Value[0]
				baseImage, tag := parseImageName(from)
				return baseImage, tag
			}
		}
	}
	return "", ""
}

func parseImageName(imageName string) (baseImage, tag string) {
	ref, err := reference.Parse(imageName)
	if err != nil {
		return baseImage, tag
	}
	parsedName, ok := ref.(reference.Named)
	if ok {
		baseImage = parsedName.Name()
	}
	tag = "latest"
	parsedTag, ok := ref.(reference.Tagged)
	if ok {
		tag = parsedTag.Tag()
	}
	return baseImage, tag
}
