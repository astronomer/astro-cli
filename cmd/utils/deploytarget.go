package utils

import (
	"errors"
	"io"
	"sync"

	manifestdeploy "github.com/astronomer/astro-cli/internal/deploy"
	"github.com/astronomer/astro-cli/pkg/input"
	"github.com/astronomer/astro-cli/pkg/picker"
)

// OnceEngine memoizes a container engine resolver, so a deploy resolves its
// engine once however many steps ask: the checks before the build, and the
// build and push.
func OnceEngine(resolve func() (string, []string, error)) func() (string, []string, error) {
	type engine struct {
		bin string
		env []string
	}
	once := sync.OnceValues(func() (engine, error) {
		bin, env, err := resolve()
		return engine{bin, env}, err
	})
	return func() (string, []string, error) {
		e, err := once()
		return e.bin, e.env, err
	}
}

// ConfirmDeployTarget asks which deployment a pyproject.toml project's deploy
// ships to, on Astro and on Astro Private Cloud alike. Every interactive
// deploy that did not name its target comes through here — the project's
// default link moves the cursor and never skips the question
// (docs/instances.md, "Deploy always asks").
//
// Each row ends in a marker rather than a column of its own: "linked" on the
// project's links, "linked, default" on the default, nothing on a Deployment
// picking would link. The words are the ones `astro use` and the `astro link`
// pickers mark the default with.
//
// It takes a name or a number, names first: a link may legally be called "2",
// and what the user typed is what they meant. Enter takes the highlighted entry
// when there is one. With nothing highlighted there is no default on Enter: the
// safe answer to "where should I ship this code" is never one the CLI picked by
// itself. It asks on errOut and reads in, so stdout stays the deploy's own
// output.
func ConfirmDeployTarget(choices []manifestdeploy.Choice, defaultLink string, in io.Reader, errOut io.Writer) (string, error) {
	notes := make([]string, len(choices))
	anyNote := false
	for i, choice := range choices {
		switch {
		case choice.Unlinked:
		case defaultLink != "" && choice.Name == defaultLink:
			// Only a link is ever the default.
			notes[i] = "linked, " + manifestdeploy.DefaultMark
		default:
			notes[i] = "linked"
		}
		anyNote = anyNote || notes[i] != ""
	}
	header := []string{"NAME", "WHERE"}
	if anyNote {
		header = append(header, "")
	}
	l := picker.List{
		Title:  "Deploy to which deployment?",
		Header: header,
		Ask:    []input.Option{input.About("the deployment to ship to"), input.AnsweredBy("--deployment")},
		// Asked picker.DefaultAttempts times, and still no pick. The picker has said "Not one
		// of the choices." after every answer, the third included, so that
		// line is the last word and this ends quietly: that is what the
		// sentinel is for. Input that ends instead fails with Ended.
		Invalid: manifestdeploy.ErrAborted,
		ByName:  true,
		Ended:   input.Required(errors.New("a deploy must name the deployment it ships to: astro deploy <name> or --deployment <name>")),
	}
	for i, choice := range choices {
		cells := []string{choice.Name, choice.Where}
		if anyNote {
			cells = append(cells, notes[i])
		}
		chosen := defaultLink != "" && choice.Name == defaultLink
		if chosen {
			l.Default = i + 1
		}
		l.AddRow(chosen, cells...)
	}
	i, err := l.Pick(errOut, in)
	if err != nil {
		return "", err
	}
	return choices[i].Name, nil
}
