package input

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/manifoldco/promptui"
	"golang.org/x/term"
)

// Text requests a user for input text and returns it
func Text(promptText string) string {
	reader := bufio.NewReader(os.Stdin)
	if promptText != "" {
		fmt.Print(promptText)
	}
	text, _ := reader.ReadString('\n')
	return strings.Trim(text, "\r\n")
}

// Confirm requests a user to confirm their input
func Confirm(promptText string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s (y/n) ", promptText)

	text, _ := reader.ReadString('\n')
	return strings.Trim(text, "\r\n") == "y", nil
}

// Password requests a users passord, does not print out what they entered, and returns it
func Password(promptText string) (string, error) {
	fmt.Print(promptText)
	bytePassword, err := term.ReadPassword(int(syscall.Stdin)) //nolint: unconvert
	if err != nil {
		return "", err
	}
	fmt.Print("\n")
	return string(bytePassword), nil
}

// Structure to hold content required for displayig prompts required for promptui library functions
type PromptContent struct {
	ErrorMsg string
	Label    string
}

// Gets a y/n confirmation from the user for the given prompt content using the promptui library and returns a boolean accordingly
func PromptGetConfirmation(pc PromptContent) (bool, error) {
	prompt := promptui.Select{
		Label: pc.Label,
		Items: []string{"y", "n"},
	}

	_, result, err := prompt.Run()
	if err != nil {
		fmt.Printf("Prompt failed %v\n", pc.ErrorMsg)
		return false, err
	}

	if result == "y" {
		return true, nil
	}
	return false, nil
}
