package input

import (
	"io"
	"os"
	"strings"
	"testing"

	"github.com/manifoldco/promptui"
)

func TestText(t *testing.T) {
	type args struct {
		promptText string
	}
	tests := []struct {
		name        string
		args        args
		inputString string
		want        string
	}{
		{
			name:        "basic case",
			inputString: "testing",
			args:        args{promptText: "enter text input"},
			want:        "testing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// mock os.Stdin
			input := []byte(tt.inputString)
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			_, err = w.Write(input)
			if err != nil {
				t.Error(err)
			}
			w.Close()
			stdin := os.Stdin
			os.Stdin = r

			if got := Text(tt.args.promptText); got != tt.want {
				t.Errorf("Text() = %v, want %v", got, tt.want)
			}

			// Restore stdin right after the test.
			os.Stdin = stdin
		})
	}
}

func TestConfirm(t *testing.T) {
	type args struct {
		promptText string
	}
	tests := []struct {
		name        string
		inputString string
		args        args
		want        bool
		wantErr     bool
	}{
		{
			name:        "no case",
			inputString: "n",
			args:        args{promptText: "enter y or n"},
			want:        false,
			wantErr:     false,
		},
		{
			name:        "yes case",
			inputString: "y",
			args:        args{promptText: "enter y or n"},
			want:        true,
			wantErr:     false,
		},
		{
			name:        "no input",
			inputString: "",
			args:        args{promptText: "enter y or n"},
			want:        false,
			wantErr:     false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// mock os.Stdin
			input := []byte(tt.inputString)
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			_, err = w.Write(input)
			if err != nil {
				t.Error(err)
			}
			w.Close()
			stdin := os.Stdin
			os.Stdin = r

			got, err := Confirm(tt.args.promptText)
			if (err != nil) != tt.wantErr {
				t.Errorf("Confirm() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Confirm() = %v, want %v", got, tt.want)
			}

			// Restore stdin right after the test.
			os.Stdin = stdin
		})
	}
}

func TestPassword(t *testing.T) {
	type args struct {
		promptText string
	}
	tests := []struct {
		name        string
		inputString string
		args        args
		want        string
		wantErr     bool
	}{
		{
			name:        "unsupported error",
			inputString: "",
			args:        args{"enter pass"},
			want:        "",
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// mock os.Stdin
			input := []byte(tt.inputString)
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			_, err = w.Write(input)
			if err != nil {
				t.Error(err)
			}
			w.Close()
			stdin := os.Stdin
			os.Stdin = r

			got, err := Password(tt.args.promptText)
			if (err != nil) != tt.wantErr {
				t.Errorf("Password() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Password() = %v, want %v", got, tt.want)
			}

			// Restore stdin right after the test.
			os.Stdin = stdin
		})
	}
}

func TestPromptGetConfirmation(t *testing.T) {
	runner := GetYesNoSelector(PromptContent{Label: "test label, enter y/n"})
	runner.Keys = &promptui.SelectKeys{Next: promptui.Key{Code: rune('S')}, Prev: promptui.Key{Code: rune('W')}, PageUp: promptui.Key{Code: rune('D')}, PageDown: promptui.Key{Code: rune('A')}}
	tests := []struct {
		name        string
		inputString string
		want        bool
		wantErr     bool
	}{
		{
			name:        "basic yes case",
			inputString: "\n",
			want:        true,
			wantErr:     false,
		},
		{
			name:        "basic no case",
			inputString: "S\n",
			want:        false,
			wantErr:     false,
		},
		{
			name:        "no input case",
			inputString: "",
			want:        false,
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runner.Stdin = io.NopCloser(strings.NewReader(tt.inputString))
			got, err := PromptGetConfirmation(runner)
			if (err != nil) != tt.wantErr {
				t.Errorf("PromptGetConfirmation() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("PromptGetConfirmation() = %v, want %v", got, tt.want)
			}
		})
	}
}
