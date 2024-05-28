package util

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestUtils(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestCoerce() {
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
		s.Run(tt.name, func() {
			got := Coerce(tt.args.version)
			if tt.want != "" && tt.want != got.String() {
				s.Fail("Coerce() = %v, want %v", got, tt.want)
			} else if tt.want == "" && got != nil {
				s.Fail("Coerce() = %v, want nil", got)
			}
		})
	}
}

func (s *Suite) TestContains() {
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
		s.Run(tt.name, func() {
			if got := Contains(tt.args.elems, tt.args.v); got != tt.want {
				s.Fail("Contains() = %v, want %v", got, tt.want)
			}
		})
	}
}

func (s *Suite) TestGetStringInBetweenTwoString() {
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
		s.Run(tt.name, func() {
			gotResult, gotFound := GetStringInBetweenTwoString(tt.args.str, tt.args.startS, tt.args.endS)
			if gotResult != tt.wantResult {
				s.Fail("GetStringInBetweenTwoString() gotResult = %v, want %v", gotResult, tt.wantResult)
			}
			if gotFound != tt.wantFound {
				s.Fail("GetStringInBetweenTwoString() gotFound = %v, want %v", gotFound, tt.wantFound)
			}
		})
	}
}

func (s *Suite) TestExists() {
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
		s.Run(tt.name, func() {
			got, err := Exists(tt.args.path)
			if !tt.errAssertion(s.T(), err) {
				return
			}

			s.Equal(tt.want, got)
		})
	}
}

func (s *Suite) TestBase64URLEncode() {
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
		s.Run(tt.name, func() {
			s.Equal(tt.want, Base64URLEncode(tt.args.arg))
		})
	}
}

func (s *Suite) TestCheckEnvBool() {
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
		s.Run(tt.name, func() {
			s.Equal(tt.want, CheckEnvBool(tt.args.arg))
		})
	}
}

func (s *Suite) TestParseAPIToken() {
	s.Run("throw error is token is invalid", func() {
		token := "invalid-token"
		_, err := ParseAPIToken(token)
		s.NotNil(err)
		s.Contains(err.Error(), "token is invalid or malformed")
	})

	s.Run("returns token claims if token is valid", func() {
		// dummy token
		token := "eyJhbGciOiAibm9uZSIsICJ0eXAiOiAiSldUIn0K.eyJ1c2VybmFtZSI6ImFkbWluaW5pc3RyYXRvciIsImlzX2FkbWluIjp0cnVlLCJpYXQiOjE1MTYyMzkwMjIsImV4cCI6MTUxNjI0MjYyMn0."
		claims, err := ParseAPIToken(token)
		s.NotNil(claims)
		s.NoError(err)
	})
}

func (s *Suite) TestIsM1() {
	s.Run("returns true if running on arm architecture", func() {
		s.True(IsM1("darwin", "arm64"))
	})
	s.Run("returns false if not running on arm architecture", func() {
		s.False(IsM1("darwin", "x86_64"))
	})
	s.Run("returns false if running on windows", func() {
		s.False(IsM1("windows", "amd64"))
	})
}

func (s *Suite) TestGetbuildSecretString() {
	s.Run("returns empty string if buildSecret is empty", func() {
		s.Equal("", GetbuildSecretString([]string{}))
	})

	s.Run("returns the only secret if buildSecret has only one element", func() {
		s.Equal("secret1", GetbuildSecretString([]string{"secret1"}))
	})

	s.Run("returns comma-separated string for multiple secrets in buildSecret", func() {
		s.Equal("secret1,secret2,secret3", GetbuildSecretString([]string{"secret1", "secret2", "secret3"}))
	})

	s.Run("overrides buildSecretString with BUILD_SECRET_INPUT if set", func() {
		// Save the original value of BUILD_SECRET_INPUT
		originalBuildSecretInput := os.Getenv("BUILD_SECRET_INPUT")
		defer func() {
			// Reset BUILD_SECRET_INPUT to its original value after the test
			os.Setenv("BUILD_SECRET_INPUT", originalBuildSecretInput)
		}()

		// Set BUILD_SECRET_INPUT to a different value
		os.Setenv("BUILD_SECRET_INPUT", "override_secret")

		// Test with a non-empty buildSecret
		s.Equal("secret1,secret2", GetbuildSecretString([]string{"secret1", "secret2"}))

		// Test with an empty buildSecret
		s.Equal("override_secret", GetbuildSecretString([]string{}))
	})
}

func (s *Suite) TestStripOutKeysFromJSONByteArray() {
	s.Run("valid JSON, strip out keys", func() {
		jsonData := []byte(`{"a": 1, "b": 2, "c": 3}`)
		keys := []string{"a", "c"}
		expectedResult := []byte(`{"b":2}`)
		result, err := StripOutKeysFromJSONByteArray(jsonData, keys)
		s.NoError(err)
		s.Equal(result, expectedResult)
	})

	s.Run("invalid JSON, return as is - case 1", func() {
		jsonData := []byte(`{invalid: json}`)
		keys := []string{"a", "c"}
		expectedResult := jsonData
		result, err := StripOutKeysFromJSONByteArray(jsonData, keys)
		s.NoError(err)
		s.Equal(result, expectedResult)
	})

	s.Run("invalid JSON, return as is - case 2", func() {
		jsonData := []byte(``)
		keys := []string{"a", "c"}
		expectedResult := jsonData
		result, err := StripOutKeysFromJSONByteArray(jsonData, keys)
		s.NoError(err)
		s.Equal(result, expectedResult)
	})
}

func (s *Suite) TestFilter() {
	s.Run("strings", func() {
		expectedResult := []string{"a"}
		result := Filter([]string{"a", "b", "c"}, func(s string) bool { return s == "a" })
		s.Equal(result, expectedResult)
	})
	s.Run("ints", func() {
		expectedResult := []int{2}
		result := Filter([]int{1, 2, 3}, func(s int) bool { return s%2 == 0 })
		s.Equal(result, expectedResult)
	})
}
