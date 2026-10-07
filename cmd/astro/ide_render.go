package astro

// How `astro ide project list`, `import` and `export` publish what they did,
// in text and in json. internal/platform/astro/ide returns it; this file is
// the only place deciding how it looks.

import (
	"bufio"
	"fmt"
	"io"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/astro/ide"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

// emitIDEProjects publishes the project list; in text, the NAME and ID table
// it always printed, its header alone when there are none.
func emitIDEProjects(r cliout.Renderer, list *ide.ProjectList) error {
	return r.Emit(list, func(w io.Writer) error {
		tab := &printutil.Table{
			Padding:        []int{44, 50},
			DynamicPadding: true,
			Header:         []string{"NAME", "ID"},
			ColorRowCode:   [2]string{"\033[1;32m", "\033[0m"},
		}
		for i := range list.Projects {
			p := &list.Projects[i]
			tab.AddRow([]string{p.Name, p.ID}, false)
		}
		return tab.Print(w)
	})
}

// ideProjectLabel is how text names a project: by name, or by ID when the
// name could not be read back.
func ideProjectLabel(id, name string) string {
	if name == "" {
		return id
	}
	return name
}

// emitIDEImport publishes an import. Its text line has always said
// "exported", for the export the IDE makes of the project.
func emitIDEImport(r cliout.Renderer, res *ide.Import) error {
	return r.Emit(res, cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "Successfully exported project from %s\n", ideProjectLabel(res.ProjectID, res.ProjectName))
	}))
}

// emitIDEExport publishes an export.
func emitIDEExport(r cliout.Renderer, res *ide.Export) error {
	return r.Emit(res, cliout.Text(func(b *bufio.Writer) {
		fmt.Fprintf(b, "Successfully exported project to %s\n", ideProjectLabel(res.ProjectID, res.ProjectName))
	}))
}
