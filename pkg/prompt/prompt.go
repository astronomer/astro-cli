package prompt

import (
	"fmt"
	"os"
	"strings"

	"github.com/manifoldco/promptui"
)

type Content struct {
	ErrorMsg string
	Label    string
}

func GetInput(c Content, isConfirm bool, validateFn func(input string) error) (string, error) {
	bold := promptui.Styler(promptui.FGBold)
	templates := &promptui.PromptTemplates{
		Invalid: fmt.Sprintf("%s {{ . | faint }}%s ", bold(promptui.IconBad), bold(":")),
		Success: fmt.Sprintf("%s {{ . | faint }}%s ", bold(promptui.IconGood), bold(":")),
	}

	if isConfirm {
		templates.Invalid = fmt.Sprintf("%s {{ . | faint }}%s n", bold(promptui.IconBad), bold(":"))
	}

	prompt := promptui.Prompt{
		Label:     c.Label,
		Templates: templates,
		Validate:  validateFn,
		IsConfirm: isConfirm,
	}

	result, err := prompt.Run()
	if err == promptui.ErrInterrupt {
		fmt.Printf("Exiting prompt %v\n", err)
		os.Exit(1)
	}

	return result, err
}

func GetSelect(c Content, items []string) (string, error) {
	bold := promptui.Styler(promptui.FGBold)
	index := -1
	var result string
	var err error

	templates := &promptui.SelectTemplates{
		Selected: fmt.Sprintf(`%s {{ "%s" | faint }}%s {{ . }}`, promptui.IconGood, c.Label, bold(":")),
	}

	for index < 0 {
		prompt := promptui.Select{
			Label:     c.Label,
			Items:     items,
			Templates: templates,
		}

		index, result, err = prompt.Run()

		if index == -1 {
			items = append(items, result)
		}
	}
	if err == promptui.ErrInterrupt {
		fmt.Printf("Exiting prompt %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%s %s", c.Label, result)
	return result, err
}

func GetConfirm(c Content) (bool, error) {
	result, err := GetInput(c, true, nil)
	confirm := strings.ToLower(result) == "y"
	return confirm, err
}
