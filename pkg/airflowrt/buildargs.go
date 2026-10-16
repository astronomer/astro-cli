package airflowrt

import (
	"regexp"
	"slices"
	"strings"
)

// ArgProblem says why a FROM built from build arguments has no answer.
type ArgProblem uint8

const (
	// ArgNone is a FROM whose build arguments all expanded, or one that uses
	// none.
	ArgNone ArgProblem = iota
	// ArgUnset is a FROM that expands to no image reference because it needs
	// a build argument that was never given a value: declared without a
	// default, or not declared before the first FROM at all. Docker expands it
	// to empty, and astro passes no --build-arg, so astro's own build of the
	// file fails; only a build given that argument works, and what it builds
	// on is up to that build.
	ArgUnset
	// ArgUnsupported is a FROM this package does not expand: one using a
	// modifier other than :- and :+, an escape, an unbalanced brace or quote,
	// a quote inside ${...}, one of the platform arguments BuildKit sets
	// itself (TARGETARCH and the rest), or a result that is not an image
	// reference for any other reason.
	ArgUnsupported
)

// platformArgs are the arguments BuildKit sets on its own in the global
// scope. Their value is the platform of the build, which nothing in the file
// says, so a FROM using one cannot be read.
var platformArgs = []string{
	"TARGETPLATFORM", "TARGETOS", "TARGETARCH", "TARGETVARIANT",
	"BUILDPLATFORM", "BUILDOS", "BUILDARCH", "BUILDVARIANT",
}

// buildArgs is the build arguments a FROM line can use: the ARGs declared
// before the first FROM and given a value, by name. An ARG never given one is
// absent.
type buildArgs map[string]argValue

// argValue is a build argument's default, or a FROM reference, as expanded.
// unset reports that an argument with no value went into it as empty, which is
// what a reference that turns out not to be one is put down to. problem is
// ArgUnsupported when something in it could not be expanded at all.
type argValue struct {
	value   string
	unset   bool
	problem ArgProblem
}

func (v *argValue) absorb(w argValue) {
	v.value += w.value
	v.unset = v.unset || w.unset
	if w.problem > v.problem {
		v.problem = w.problem
	}
}

var unsupported = argValue{problem: ArgUnsupported}

// globalBuildArgs reads the ARG instructions before the first FROM, in order,
// each default expanded against the ones declared before it, as Docker does.
// An ARG with no default leaves an earlier value in place.
//
// Line by line, as readStages reads its FROM lines. An instruction is split on
// whitespace, so a quoted default with a space in it reads as an unclosed
// quote, and is unsupported rather than a guess.
func globalBuildArgs(data []byte) buildArgs {
	args := buildArgs{}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch strings.ToUpper(fields[0]) {
		case "FROM":
			return args
		case "ARG":
			for _, f := range fields[1:] {
				if name, value, hasDefault := strings.Cut(f, "="); hasDefault {
					args[name] = args.lex(value, true)
				}
			}
		}
	}
	return args
}

// expandRef expands a FROM reference and says whether the result is one.
// A reference with nothing to expand is returned as it is.
func (args buildArgs) expandRef(ref string) (string, ArgProblem) {
	if !strings.ContainsAny(ref, "$\"'\\") {
		return ref, ArgNone
	}
	v := args.lex(ref, true)
	switch {
	case v.problem != ArgNone:
		return "", v.problem
	case v.value == "":
		return "", ArgUnset
	case !imageRefRe.MatchString(v.value):
		if v.unset {
			return "", ArgUnset
		}
		return "", ArgUnsupported
	}
	return v.value, ArgNone
}

// lex expands s as Docker's word lexer does for a FROM or an ARG default:
// $NAME, ${NAME}, ${NAME:-word} and ${NAME:+word}, where word may hold
// references of its own, and an argument with no value is empty. With quotes,
// double quotes are removed and their contents expanded, and single quotes
// removed and their contents kept as they are, a '$' included, which no image
// reference holds. Inside ${...} a quote is unsupported rather than guessed
// at.
func (args buildArgs) lex(s string, quotes bool) argValue {
	var v argValue
	inDouble := false
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\\':
			return unsupported
		case (c == '"' || c == '\'') && !quotes:
			return unsupported
		case c == '"':
			inDouble = !inDouble
			i++
			continue
		case c == '\'' && !inDouble:
			end := strings.IndexByte(s[i+1:], '\'')
			if end < 0 {
				return unsupported
			}
			v.value += s[i+1 : i+1+end]
			i += end + 2
			continue
		case c != '$':
			v.value += string(c)
			i++
			continue
		}
		// A '$'.
		if i+1 < len(s) && s[i+1] == '{' {
			end := matchingBrace(s, i+1)
			if end < 0 {
				return unsupported
			}
			v.absorb(args.braced(s[i+2 : end]))
			i = end + 1
			continue
		}
		n := nameLen(s[i+1:])
		if n == 0 {
			return unsupported
		}
		v.absorb(args.plain(s[i+1 : i+1+n]))
		i += 1 + n
	}
	if inDouble {
		return unsupported
	}
	return v
}

// braced expands the inside of ${...}: a name, optionally followed by :-word
// or :+word. Like Docker, :- and :+ treat an empty value as no value.
func (args buildArgs) braced(inner string) argValue {
	n := nameLen(inner)
	if n == 0 {
		return unsupported
	}
	name, rest := inner[:n], inner[n:]
	if rest == "" {
		return args.plain(name)
	}
	if len(rest) < 2 || slices.Contains(platformArgs, name) {
		return unsupported
	}
	v := args[name]
	word := rest[2:]
	switch rest[:2] {
	case ":-":
		if v.problem != ArgNone || v.value != "" {
			return v
		}
		return args.lex(word, false)
	case ":+":
		if v.problem != ArgNone {
			return v
		}
		if v.value == "" {
			return argValue{}
		}
		return args.lex(word, false)
	default:
		return unsupported
	}
}

// plain is a reference to name with no modifier.
func (args buildArgs) plain(name string) argValue {
	if slices.Contains(platformArgs, name) {
		return unsupported
	}
	v, ok := args[name]
	if !ok {
		return argValue{unset: true}
	}
	return v
}

// matchingBrace returns the index of the '}' closing the '{' at open, or -1
// when there is none. Only a nested ${ opens another level, as in Docker's
// lexer, where a lone '{' in a word is a literal character.
func matchingBrace(s string, open int) int {
	depth := 1
	for i := open + 1; i < len(s); i++ {
		switch {
		case s[i] == '$' && i+1 < len(s) && s[i+1] == '{':
			depth++
			i++
		case s[i] == '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// nameLen is the length of the build argument name s starts with: a letter or
// underscore, then letters, digits and underscores.
func nameLen(s string) int {
	for i := 0; i < len(s); i++ {
		c := s[i]
		letter := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		if letter || (i > 0 && c >= '0' && c <= '9') {
			continue
		}
		return i
	}
	return len(s)
}

// imageRefRe is the grammar of an image reference (distribution's
// reference.ReferenceRegexp, without IPv6 hosts): an optional registry host
// and port, lower-case path components, then an optional tag and digest.
// Only an expanded reference is checked against it, which is how an unset
// argument that left "registry/image:" or "/runtime:3.1-12" behind is told
// from an image.
var imageRefRe = func() *regexp.Regexp {
	const (
		domainComponent = `(?:[a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9-]*[a-zA-Z0-9])`
		domain          = domainComponent + `(?:\.` + domainComponent + `)*(?::[0-9]+)?`
		pathComponent   = `[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*`
		name            = `(?:` + domain + `/)?` + pathComponent + `(?:/` + pathComponent + `)*`
		tag             = `[\w][\w.-]{0,127}`
		digest          = `[A-Za-z][A-Za-z0-9]*(?:[-_+.][A-Za-z][A-Za-z0-9]*)*:[0-9a-fA-F]{32,}`
	)
	return regexp.MustCompile(`^` + name + `(?::` + tag + `)?(?:@` + digest + `)?$`)
}()
