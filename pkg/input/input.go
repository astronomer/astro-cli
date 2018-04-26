package input

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"syscall"

	"golang.org/x/crypto/ssh/terminal"
)

// InputText requests a user for input text and returns it
func InputText(promptText string) string {
	reader := bufio.NewReader(os.Stdin)
	if promptText != "" {
		fmt.Print(promptText)
	}
	text, _ := reader.ReadString('\n')
	return strings.Trim(text, "\r\n")
}

// InputPassword requests a users passord, does not print out what they entered, and returns it
func InputPassword(promptText string) (string, error) {
	fmt.Print(promptText)
	bytePassword, err := terminal.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return "", err
	}
	fmt.Print("\n")
	return string(bytePassword), nil
}

// InputConfirm requests a user to confirm their input
func InputConfirm(promptText string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Printf("%s (y/n) ", promptText)

	text, _ := reader.ReadString('\n')
	return strings.Trim(text, "\r\n") == "y", nil
}
