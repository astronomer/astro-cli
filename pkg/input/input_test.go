package input

import (
	"os"
	"testing"
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
