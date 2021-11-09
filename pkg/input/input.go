package input

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/crypto/ssh/terminal"
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

// Password requests a users passord, does not print out what they entered, and returns it
func Password(promptText string) (string, error) {
	fmt.Print(promptText)
	bytePassword, err := terminal.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}
	fmt.Print("\n")
	return string(bytePassword), nil
}

// Confirm requests a user to confirm their input
func Confirm(promptText string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s (y/n) ", promptText)

	text, _ := reader.ReadString('\n')
	return strings.Trim(text, "\r\n") == "y", nil
}
