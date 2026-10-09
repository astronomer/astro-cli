package project

import (
	"errors"
	"strings"
)

// Project1xUnderAPC is the one account of a 1.x project under an Astro
// Private Cloud context, where is the directory holding it as a sentence
// begins with it. APC's deploy still builds the 1.x layout, so astro init
// refuses to convert one there, and every hint that would otherwise say to
// run it says this instead: astro init's refusal, the errors of a command run
// in such a project, and the astro dev stub.
func Project1xUnderAPC(where string) string {
	return where + " holds a project made by Astro CLI 1.x (Dockerfile and .astro/), and the current context is " +
		"Astro Private Cloud, whose astro deploy still builds that layout. Leave the project as it is for now: " +
		"astro deploy keeps working with it on Astro Private Cloud, and converting it will be available once " +
		"Astro Private Cloud deploys pyproject.toml projects. To convert it anyway, for Astro or for local " +
		"development only, switch to an Astro context first (astro context switch astronomer.io, or astro login " +
		"to sign in to Astro) and run " + initCommand + " again"
}

// AdviseUnderAPC returns err with a 1.x project's NotFoundError or
// NoAstroSectionError in it marked as met under an Astro Private Cloud
// context, so its message says what Project1xUnderAPC says. A wrapping
// fmt.Errorf has already rendered the old message into its own, so that text
// is replaced in the outer message too; errors.Is and errors.As still see
// everything err wraps.
func AdviseUnderAPC(err error) error {
	var before, after string
	var nf *NotFoundError
	var ns *NoAstroSectionError
	switch {
	case errors.As(err, &nf) && nf.Project1xDir != "" && !nf.UnderAPC:
		before = nf.Error()
		nf.UnderAPC = true
		after = nf.Error()
	case errors.As(err, &ns) && ns.Has1xProject && !ns.UnderAPC:
		before = ns.Error()
		ns.UnderAPC = true
		after = ns.Error()
	default:
		return err
	}
	return &advisedError{err: err, msg: strings.Replace(err.Error(), before, after, 1)}
}

// advisedError is err with its message rewritten by AdviseUnderAPC.
type advisedError struct {
	err error
	msg string
}

func (e *advisedError) Error() string { return e.msg }
func (e *advisedError) Unwrap() error { return e.err }
