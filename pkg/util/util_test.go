package util

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCoerce(t *testing.T) {
	type args struct {
		version string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "valid case",
			args: args{version: "2.2.2"},
			want: "2.2.2",
		},
		{
			name: "invalid case",
			args: args{version: "test"},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Coerce(tt.args.version)
			if tt.want != "" && tt.want != got.String() {
				t.Errorf("Coerce() = %v, want %v", got, tt.want)
			} else if tt.want == "" && got != nil {
				t.Errorf("Coerce() = %v, want nil", got)
			}
		})
	}
}

func TestContains(t *testing.T) {
	type args struct {
		elems []string
		v     string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "true case",
			args: args{elems: []string{"test1", "test2"}, v: "test1"},
			want: true,
		},
		{
			name: "false case",
			args: args{elems: []string{"test1", "test2"}, v: "test3"},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Contains(tt.args.elems, tt.args.v); got != tt.want {
				t.Errorf("Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetStringInBetweenTwoString(t *testing.T) {
	type args struct {
		str    string
		startS string
		endS   string
	}
	tests := []struct {
		name       string
		args       args
		wantResult string
		wantFound  bool
	}{
		{
			name:       "valid case",
			args:       args{"hello world here", "hello", "here"},
			wantResult: " world ",
			wantFound:  true,
		},
		{
			name:       "invalid case without end",
			args:       args{"hello world here", "hello", "there"},
			wantResult: "",
			wantFound:  false,
		},
		{
			name:       "invalid case without start",
			args:       args{"hello world here", "helloworld", "here"},
			wantResult: "",
			wantFound:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotResult, gotFound := GetStringInBetweenTwoString(tt.args.str, tt.args.startS, tt.args.endS)
			if gotResult != tt.wantResult {
				t.Errorf("GetStringInBetweenTwoString() gotResult = %v, want %v", gotResult, tt.wantResult)
			}
			if gotFound != tt.wantFound {
				t.Errorf("GetStringInBetweenTwoString() gotFound = %v, want %v", gotFound, tt.wantFound)
			}
		})
	}
}

func TestExists(t *testing.T) {
	type args struct {
		path string
	}
	tests := []struct {
		name         string
		args         args
		want         bool
		errAssertion assert.ErrorAssertionFunc
	}{
		{
			name:         "valid case",
			args:         args{"./util_test.go"},
			want:         true,
			errAssertion: assert.NoError,
		},
		{
			name:         "invalid case",
			args:         args{"./test.go"},
			want:         false,
			errAssertion: assert.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Exists(tt.args.path)
			if !tt.errAssertion(t, err) {
				return
			}

			if got != tt.want {
				t.Errorf("Exists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBase64URLEncode(t *testing.T) {
	type args struct {
		arg []byte
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "basic case",
			args: args{[]byte(`testing url encode functionality`)},
			want: "dGVzdGluZyB1cmwgZW5jb2RlIGZ1bmN0aW9uYWxpdHk",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Base64URLEncode(tt.args.arg); got != tt.want {
				t.Errorf("Base64URLEncode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCheckEnvBool(t *testing.T) {
	type args struct {
		arg string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "first false case",
			args: args{"False"},
			want: false,
		},
		{
			name: "second false case",
			args: args{"false"},
			want: false,
		},
		{
			name: "first true case",
			args: args{"True"},
			want: true,
		},
		{
			name: "second true case",
			args: args{"true"},
			want: true,
		},
		{
			name: "third false case",
			args: args{""},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckEnvBool(tt.args.arg); got != tt.want {
				t.Errorf("CheckEnvBool() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsM1(t *testing.T) {
	t.Run("returns true if running on arm architecture", func(t *testing.T) {
		assert.True(t, IsM1("darwin", "arm64"))
	})
	t.Run("returns false if not running on arm architecture", func(t *testing.T) {
		assert.False(t, IsM1("darwin", "x86_64"))
	})
	t.Run("returns false if running on windows", func(t *testing.T) {
		assert.False(t, IsM1("windows", "amd64"))
	})
}

func TestGetbuildSecretString(t *testing.T) {
	t.Run("returns empty string if buildSecret is empty", func(t *testing.T) {
		assert.Equal(t, "", GetbuildSecretString([]string{}))
	})

	t.Run("returns the only secret if buildSecret has only one element", func(t *testing.T) {
		assert.Equal(t, "secret1", GetbuildSecretString([]string{"secret1"}))
	})

	t.Run("returns comma-separated string for multiple secrets in buildSecret", func(t *testing.T) {
		assert.Equal(t, "secret1,secret2,secret3", GetbuildSecretString([]string{"secret1", "secret2", "secret3"}))
	})

	t.Run("overrides buildSecretString with BUILD_SECRET_INPUT if set", func(t *testing.T) {
		// Save the original value of BUILD_SECRET_INPUT
		originalBuildSecretInput := os.Getenv("BUILD_SECRET_INPUT")
		defer func() {
			// Reset BUILD_SECRET_INPUT to its original value after the test
			os.Setenv("BUILD_SECRET_INPUT", originalBuildSecretInput)
		}()

		// Set BUILD_SECRET_INPUT to a different value
		os.Setenv("BUILD_SECRET_INPUT", "override_secret")

		// Test with a non-empty buildSecret
		assert.Equal(t, "secret1,secret2", GetbuildSecretString([]string{"secret1", "secret2"}))

		// Test with an empty buildSecret
		assert.Equal(t, "override_secret", GetbuildSecretString([]string{}))
	})
}
