package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
)


const completionLong = `
Generate autocompletions script for Astro for the specified shell (bash or zsh).

This command can generate shell autocompletions. e.g.

	$ astro completion bash

Can be sourced as such

	$ source <(astro completion bash)

Bash users can as well save it to the file and copy it to:
	/etc/bash_completion.d/
Correct arguments for SHELL are: "bash" and "zsh".
Notes:
1) zsh completions requires zsh 5.2 or newer.
	
2) macOS users have to install bash-completion framework to utilize
completion features. This can be done using homebrew:
	brew install bash-completion
Once installed, you must load bash_completion by adding following
line to your .profile or .bashrc/.zshrc:
	source $(brew --prefix)/etc/bash_completion
`

var (
	completionShells = map[string]func(out io.Writer, cmd *cobra.Command) error{
		"bash": runCompletionBash,
		"zsh":  runCompletionZsh,
	}
)

func runCompletion(out io.Writer, cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return errors.New("shell not specified")
	}
	if len(args) > 1 {
		return errors.New("too many arguments, expected only the shell type")
	}
	run, found := completionShells[args[0]]
	if !found {
		return errors.Errorf("unsupported shell type %q", args[0])
	}

	return run(out, cmd)
}

func runCompletionBash(_ io.Writer, cmd *cobra.Command) error {
	var buf bytes.Buffer
	err := cmd.Root().GenBashCompletion(&buf)
	if err != nil {
		return fmt.Errorf("error while generating bash completion: %v", err)
	}
	// remove the command "completion" from auto-completion
	code := buf.String()
	code = strings.Replace(code, `commands+=("completion")`, "", -1)

	fmt.Print(code)
	return nil
}

func runCompletionZsh(_ io.Writer, cmd *cobra.Command) error {
	var buf bytes.Buffer

	// zshHead is the header required to declare zsh completion
	zshHead := "#compdef astro\n"

	// zshInitialization represents initialization code needed to convert bash completion
	// code to zsh completion.
	zshInitialization := `
__astro_bash_source() {
	alias shopt=':'
	alias _expand=_bash_expand
	alias _complete=_bash_comp
	emulate -L sh
	setopt kshglob noshglob braceexpand
	source "$@"
}
__astro_type() {
	# -t is not supported by zsh
	if [ "$1" == "-t" ]; then
		shift
		# fake Bash 4 to disable "complete -o nospace". Instead
		# "compopt +-o nospace" is used in the code to toggle trailing
		# spaces. We don't support that, but leave trailing spaces on
		# all the time
		if [ "$1" = "__astro_compopt" ]; then
			echo builtin
			return 0
		fi
	fi
	type "$@"
}
__astro_compgen() {
	local completions w
	completions=( $(compgen "$@") ) || return $?
	# filter by given word as prefix
	while [[ "$1" = -* && "$1" != -- ]]; do
		shift
		shift
	done
	if [[ "$1" == -- ]]; then
		shift
	fi
	for w in "${completions[@]}"; do
		if [[ "${w}" = "$1"* ]]; then
			echo "${w}"
		fi
	done
}
__astro_compopt() {
	true # don't do anything. Not supported by bashcompinit in zsh
}
__astro_ltrim_colon_completions()
{
	if [[ "$1" == *:* && "$COMP_WORDBREAKS" == *:* ]]; then
		# Remove colon-word prefix from COMPREPLY items
		local colon_word=${1%${1##*:}}
		local i=${#COMPREPLY[*]}
		while [[ $((--i)) -ge 0 ]]; do
			COMPREPLY[$i]=${COMPREPLY[$i]#"$colon_word"}
		done
	fi
}
__astro_get_comp_words_by_ref() {
	cur="${COMP_WORDS[COMP_CWORD]}"
	prev="${COMP_WORDS[${COMP_CWORD}-1]}"
	words=("${COMP_WORDS[@]}")
	cword=("${COMP_CWORD[@]}")
}
__astro_filedir() {
	local RET OLD_IFS w qw
	__debug "_filedir $@ cur=$cur"
	if [[ "$1" = \~* ]]; then
		# somehow does not work. Maybe, zsh does not call this at all
		eval echo "$1"
		return 0
	fi
	OLD_IFS="$IFS"
	IFS=$'\n'
	if [ "$1" = "-d" ]; then
		shift
		RET=( $(compgen -d) )
	else
		RET=( $(compgen -f) )
	fi
	IFS="$OLD_IFS"
	IFS="," __debug "RET=${RET[@]} len=${#RET[@]}"
	for w in ${RET[@]}; do
		if [[ ! "${w}" = "${cur}"* ]]; then
			continue
		fi
		if eval "[[ \"\${w}\" = *.$1 || -d \"\${w}\" ]]"; then
			qw="$(__astro_quote "${w}")"
			if [ -d "${w}" ]; then
				COMPREPLY+=("${qw}/")
			else
				COMPREPLY+=("${qw}")
			fi
		fi
	done
}
__astro_quote() {
    if [[ $1 == \'* || $1 == \"* ]]; then
        # Leave out first character
        printf %q "${1:1}"
    else
    	printf %q "$1"
    fi
}
autoload -U +X bashcompinit && bashcompinit
# use word boundary patterns for BSD or GNU sed
LWORD='[[:<:]]'
RWORD='[[:>:]]'
if sed --help 2>&1 | grep -q GNU; then
	LWORD='\<'
	RWORD='\>'
fi
__astro_convert_bash_to_zsh() {
	sed \
	-e 's/declare -F/whence -w/' \
	-e 's/_get_comp_words_by_ref "\$@"/_get_comp_words_by_ref "\$*"/' \
	-e 's/local \([a-zA-Z0-9_]*\)=/local \1; \1=/' \
	-e 's/flags+=("\(--.*\)=")/flags+=("\1"); two_word_flags+=("\1")/' \
	-e 's/must_have_one_flag+=("\(--.*\)=")/must_have_one_flag+=("\1")/' \
	-e "s/${LWORD}_filedir${RWORD}/__astro_filedir/g" \
	-e "s/${LWORD}_get_comp_words_by_ref${RWORD}/__astro_get_comp_words_by_ref/g" \
	-e "s/${LWORD}__ltrim_colon_completions${RWORD}/__astro_ltrim_colon_completions/g" \
	-e "s/${LWORD}compgen${RWORD}/__astro_compgen/g" \
	-e "s/${LWORD}compopt${RWORD}/__astro_compopt/g" \
	-e "s/${LWORD}declare${RWORD}/builtin declare/g" \
	-e 's/aliashash\["\(.\{1,\}\)"\]/aliashash[\1]/g' \
	-e "s/\\\$(type${RWORD}/\$(__astro_type/g" \
	<<'BASH_COMPLETION_EOF'
`

	// zshFinalize is code going to the end of completion file
	// that calls conversion bash to zsh.
	zshFinalize := `
BASH_COMPLETION_EOF
}

__astro_bash_source <(__astro_convert_bash_to_zsh)
_complete astro 2>/dev/null
`
	_, err := buf.Write([]byte(zshHead))
	if err != nil {
		return fmt.Errorf("error while generating zsh completion: %v", err)
	}

	_, err = buf.Write([]byte(zshInitialization))
	if err != nil {
		return fmt.Errorf("error while generating zsh completion: %v", err)
	}
	err = cmd.GenBashCompletion(&buf)
	if err != nil {
		return fmt.Errorf("error wheil generating zsh completion: %v", err)
	}

	_, err = buf.Write([]byte(zshFinalize))
	if err != nil {
		return fmt.Errorf("error while generating zsh completion: %v", err)
	}

	// remove the command "completion" from auto-completion
	code := buf.String()
	code = strings.Replace(code, `commands+=("completion")`, "", -1)

	fmt.Print(code)
	return nil
}

func init() {
	var shells []string
	for s := range completionShells {
		shells = append(shells, s)
	}

	cmd := &cobra.Command{
		Use:   "completion SHELL",
		Short: "Generate autocompletions script for the specified shell (bash or zsh)",
		Long:  completionLong,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCompletion(os.Stdout, cmd, args)
		},
		ValidArgs: shells,
	}

	RootCmd.AddCommand(cmd)
}
