package airflowrt

import "strings"

// ArgProblem says why a FROM built from build arguments has no answer.
type ArgProblem uint8

const (
	// ArgNone is a FROM whose build arguments all expanded, or one that uses
	// none.
	ArgNone ArgProblem = iota
	// ArgUnset is a FROM that needs a build argument with no default: one
	// declared without a value, or not declared before the first FROM at all.
	// Docker expands it to empty, and astro passes no --build-arg, so astro's
	// own build of the file fails; only a build given that argument works,
	// and what it builds on is up to that build.
	ArgUnset
	// ArgUnsupported is a FROM written in a form this package does not
	// expand: a modifier other than :- and :+, an escape, an unbalanced brace,
	// or a value that still holds a '$' once expanded.
	ArgUnsupported
)

// buildArgs is the build arguments a FROM line can use: the ARGs declared
// before the first FROM, by name.
type buildArgs map[string]argValue

// argValue is one build argument's default as astro's build sees it, with no
// --build-arg given. problem is what expanding its default ran into, which
// travels with the value to every reference that uses it.
type argValue struct {
	value   string
	problem ArgProblem
}

// globalBuildArgs reads the ARG instructions before the first FROM, in order,
// each default expanded against the ones declared before it, as Docker does.
// An ARG with no default is recorded with an empty value, which a plain
// reference reports as ArgUnset.
//
// Line by line, as ParseDockerfileAt reads its FROM lines. A value is split on
// whitespace, so a quoted default with a space in it reads as unsupported
// rather than as a guess.
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
				name, value, hasDefault := strings.Cut(f, "=")
				if !hasDefault {
					args[name] = argValue{}
					continue
				}
				args[name] = args.expandDefault(value)
			}
		}
	}
	return args
}

// expandDefault expands one ARG's default against the arguments declared
// before it. Double quotes are removed and their contents expanded; single
// quotes are removed and their contents kept literally, which leaves any '$'
// in them for the FROM's own check to refuse.
func (args buildArgs) expandDefault(raw string) argValue {
	if raw != "" && (raw[0] == '"' || raw[0] == '\'') {
		q := raw[0]
		if len(raw) < 2 || raw[len(raw)-1] != q {
			return argValue{problem: ArgUnsupported}
		}
		inner := raw[1 : len(raw)-1]
		if q == '\'' {
			return argValue{value: inner}
		}
		raw = inner
	}
	if strings.ContainsAny(raw, `"'`) {
		return argValue{problem: ArgUnsupported}
	}
	return args.expand(raw)
}

// expand replaces the build arguments in s as Docker does in a FROM line or an
// ARG default: $NAME, ${NAME}, ${NAME:-word} and ${NAME:+word}, where word may
// hold references of its own. Anything else is ArgUnsupported.
//
// An argument with no value is ArgUnset when it is used as it stands, and is
// not when :- supplies one or :+ asks only whether it is set: those two forms
// say what the FROM is without a --build-arg, which is the build astro runs.
func (args buildArgs) expand(s string) argValue {
	var out strings.Builder
	problem := ArgNone
	note := func(p ArgProblem) {
		if p > problem {
			problem = p
		}
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '\\':
			// An escape: `\$` is a literal dollar, which is not a base this
			// can name, and anything else is a form not read here.
			return argValue{problem: ArgUnsupported}
		case c != '$':
			out.WriteByte(c)
			i++
			continue
		}
		// A '$'.
		if i+1 < len(s) && s[i+1] == '{' {
			end := matchingBrace(s, i+1)
			if end < 0 {
				return argValue{problem: ArgUnsupported}
			}
			v := args.expandBraced(s[i+2 : end])
			note(v.problem)
			out.WriteString(v.value)
			i = end + 1
			continue
		}
		n := nameLen(s[i+1:])
		if n == 0 {
			return argValue{problem: ArgUnsupported}
		}
		v := args.plain(s[i+1 : i+1+n])
		note(v.problem)
		out.WriteString(v.value)
		i += 1 + n
	}
	return argValue{value: out.String(), problem: problem}
}

// expandBraced expands the inside of ${...}: a name, optionally followed by
// :-word or :+word.
func (args buildArgs) expandBraced(inner string) argValue {
	n := nameLen(inner)
	if n == 0 {
		return argValue{problem: ArgUnsupported}
	}
	name, rest := inner[:n], inner[n:]
	if rest == "" {
		return args.plain(name)
	}
	if len(rest) < 2 {
		return argValue{problem: ArgUnsupported}
	}
	word := rest[2:]
	v := args[name]
	switch rest[:2] {
	case ":-":
		if v.problem == ArgUnsupported || v.value != "" {
			return v
		}
		return args.expand(word)
	case ":+":
		if v.problem == ArgUnsupported {
			return v
		}
		if v.value == "" {
			return argValue{}
		}
		w := args.expand(word)
		if v.problem > w.problem {
			w.problem = v.problem
		}
		return w
	default:
		return argValue{problem: ArgUnsupported}
	}
}

// plain is a reference to name with no modifier.
func (args buildArgs) plain(name string) argValue {
	v := args[name]
	if v.value == "" && v.problem == ArgNone {
		v.problem = ArgUnset
	}
	return v
}

// matchingBrace returns the index of the '}' closing the '{' at open,
// counting the ${ that open inside it, or -1 when there is none.
func matchingBrace(s string, open int) int {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
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
