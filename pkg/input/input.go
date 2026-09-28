package input

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// Text requests a user for input text and returns it
func Text(promptText string) string {
	reader := bufio.NewReader(os.Stdin)
	if promptText != "" {
		fmt.Print(promptText)
	}
	text, _ := reader.ReadString('\n') //nolint:errcheck // error deliberately ignored in this v1 path
	return strings.Trim(text, "\r\n")
}

// ChoicePrompt is the line that asks for a pick from a numbered list of count
// entries, with the one Enter takes in brackets when preselected is above 0.
func ChoicePrompt(count, preselected int) string {
	numbers := "1"
	if count > 1 {
		numbers = fmt.Sprintf("1-%d", count)
	}
	if preselected > 0 {
		return fmt.Sprintf("Choose %s [%d]: ", numbers, preselected)
	}
	return fmt.Sprintf("Choose %s: ", numbers)
}

// Confirm requests a user to confirm their input
func Confirm(promptText string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s (y/n) ", promptText)

	text, _ := reader.ReadString('\n') //nolint:errcheck // error deliberately ignored in this v1 path
	return strings.Trim(text, "\r\n") == "y", nil
}

// Password requests a users passord, does not print out what they entered, and returns it
func Password(promptText string) (string, error) {
	fmt.Print(promptText)
	bytePassword, err := term.ReadPassword(stdinFD())
	if err != nil {
		return "", err
	}
	fmt.Print("\n")
	return string(bytePassword), nil
}
