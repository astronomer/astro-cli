package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// zshCompletionLong replaces the help of cobra's `completion zsh`. Cobra's
// macOS step writes the script into $(brew --prefix)/share/zsh/site-functions
// and stops there, which works only where that directory is already on zsh's
// fpath. On Apple Silicon the prefix is /opt/homebrew, which zsh's default
// fpath does not include, and only `brew shellenv` adds it — so on a Mac whose
// shell setup skips that, the script is written and never loaded.
//
// The fpath line names /opt/homebrew outright rather than asking brew: the Mac
// it is for is the one that skips `brew shellenv`, where brew is often not on
// PATH yet when ~/.zshrc runs, and where it is, the line would fork brew on
// every new shell to learn a path that does not change.
func zshCompletionLong(name string) string {
	return fmt.Sprintf(`Generate the autocompletion script for the zsh shell.

If shell completion is not already enabled in your environment, you will need to enable it. You can execute the following once:

	echo "autoload -U compinit; compinit" >> ~/.zshrc

To load completions in your current shell session:

	source <(%[1]s completion zsh)

To load completions for every new session, write the script once into a directory on zsh's fpath.

Linux:

	%[1]s completion zsh > "${fpath[1]}/_%[1]s"

macOS:

	%[1]s completion zsh > $(brew --prefix)/share/zsh/site-functions/_%[1]s

On Apple Silicon, zsh does not search Homebrew's site-functions directory unless your shell runs brew shellenv. If it does not, add this line to ~/.zshrc, above compinit (or above the line that sources oh-my-zsh.sh, which runs compinit):

	fpath=(/opt/homebrew/share/zsh/site-functions $fpath)

You will need to start a new shell for this setup to take effect. If completions still do not appear, clear zsh's completion cache and start a new shell:

	rm -f "${ZDOTDIR:-$HOME}"/.zcompdump*; exec zsh
`, name)
}

// commandLong is the Long that help shows for c: c's own, except for the
// cobra-generated commands whose text this CLI replaces.
//
// The replacement is made where help reads it rather than on the command,
// because cobra builds its completion commands inside ExecuteC, after the
// tree is assembled. Building them earlier to edit them reorders the root
// menu: ExecuteC re-appends help on every run, so a completion command that
// already exists ends up listed before it.
func commandLong(c *cobra.Command) string {
	if c.Name() == "zsh" && c.HasParent() && c.Parent().Name() == "completion" && c.Parent().Parent() == c.Root() {
		return zshCompletionLong(c.Root().Name())
	}
	return c.Long
}
