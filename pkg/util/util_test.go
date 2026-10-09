package util

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestUtils(t *testing.T) {
	suite.Run(t, new(Suite))
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
		{name: "False", args: args{"False"}, want: false},
		{name: "false", args: args{"false"}, want: false},
		{name: "True", args: args{"True"}, want: true},
		{name: "true", args: args{"true"}, want: true},
		{name: "empty", args: args{""}, want: false},
		{name: "1", args: args{"1"}, want: true},
		{name: "0", args: args{"0"}, want: false},
		{name: "yes", args: args{"yes"}, want: true},
		{name: "YES", args: args{"YES"}, want: true},
		{name: "no", args: args{"no"}, want: false},
		{name: "y", args: args{"y"}, want: true},
		{name: "n", args: args{"n"}, want: false},
		{name: "on", args: args{"on"}, want: true},
		{name: "off", args: args{"off"}, want: false},
		{name: "garbage", args: args{"banana"}, want: false},
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

func (s *Suite) TestResolveBuildSecrets() {
	s.Run("returns nil if no secrets are given", func() {
		s.Nil(ResolveBuildSecrets([]string{}, ""))
	})

	s.Run("returns flag values as-is", func() {
		s.Equal([]string{"secret1"}, ResolveBuildSecrets([]string{"secret1"}, ""))
		s.Equal(
			[]string{"secret1", "secret2", "secret3"},
			ResolveBuildSecrets([]string{"secret1", "secret2", "secret3"}, ""),
		)
	})

	s.Run("uses fallback when flag is empty", func() {
		s.Equal([]string{"fallback-secret"}, ResolveBuildSecrets([]string{}, "fallback-secret"))
	})

	s.Run("flag takes priority over fallback", func() {
		s.Equal([]string{"flag-secret"}, ResolveBuildSecrets([]string{"flag-secret"}, "fallback-secret"))
	})

	s.Run("earlier fallback takes priority over later ones", func() {
		s.Equal([]string{"first"}, ResolveBuildSecrets([]string{}, "first", "second"))
	})

	s.Run("uses later fallback when earlier ones are empty", func() {
		s.Equal([]string{"second"}, ResolveBuildSecrets([]string{}, "", "second"))
	})

	s.Run("splits newline-delimited fallbacks", func() {
		s.Equal(
			[]string{"id=aws,src=credentials", "id=PLATFORM_PASSWORD,env=PLATFORM_PASSWORD"},
			ResolveBuildSecrets([]string{}, "id=aws,src=credentials\nid=PLATFORM_PASSWORD,env=PLATFORM_PASSWORD"),
		)
	})

	s.Run("drops blank lines and surrounding whitespace", func() {
		s.Equal(
			[]string{"id=one,src=one.txt", "id=two,env=TWO"},
			ResolveBuildSecrets([]string{}, "\n  id=one,src=one.txt\r\n\nid=two,env=TWO\n"),
		)
	})

	s.Run("whitespace-only fallback is skipped in favor of later ones", func() {
		s.Equal([]string{"second"}, ResolveBuildSecrets([]string{}, " \n ", "second"))
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

func (s *Suite) TestIsAstronomerRegistry() {
	type args struct {
		registry string
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "valid production registry",
			args: args{registry: "images.astronomer.cloud"},
			want: true,
		},
		{
			name: "valid dev registry",
			args: args{registry: "images.astronomer-dev.cloud"},
			want: true,
		},
		{
			name: "valid stage registry",
			args: args{registry: "images.astronomer-stage.cloud"},
			want: true,
		},
		{
			name: "registry in dockerfile",
			args: args{registry: "FROM images.astronomer.cloud/baseimages/runtime:latest"},
			want: true,
		},
		{
			name: "registry with path in dockerfile",
			args: args{registry: "FROM images.astronomer-dev.cloud/baseimages/astro-remote-execution-agent:3.0-8"},
			want: true,
		},
		{
			name: "registry as part of URL",
			args: args{registry: "https://images.astronomer-stage.cloud/v2/"},
			want: true,
		},
		{
			name: "non-astronomer registry",
			args: args{registry: "quay.io/astronomer/astro-runtime"},
			want: false,
		},
		{
			name: "docker hub registry",
			args: args{registry: "docker.io/postgres"},
			want: false,
		},
		{
			name: "similar but not exact match",
			args: args{registry: "images.astronomer.com"},
			want: false,
		},
		{
			name: "empty string",
			args: args{registry: ""},
			want: false,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got := IsAstronomerRegistry(tt.args.registry)
			s.Equal(tt.want, got)
		})
	}
}

func (s *Suite) TestIsCUID() {
	tests := []struct {
		name   string
		input  string
		expect bool
	}{
		{"valid CUID", "clh1rai0g000008l50d5hahbc", true},
		{"valid CUID all zeros", "c000000000000000000000000", true},
		{"too short", "clh1rai0g000008l50d5hahb", false},
		{"too long", "clh1rai0g000008l50d5hahbcc", false},
		{"wrong prefix", "xlh1rai0g000008l50d5hahbc", false},
		{"uppercase chars", "cLH1RAI0G000008L50D5HAHBC", false},
		{"org name", "my-organization", false},
		{"empty string", "", false},
		{"name containing cuid substring", "clh1rai0g000008l50d5hahbc-prod", false},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			s.Equal(tt.expect, IsCUID(tt.input))
		})
	}
}

func (s *Suite) TestResolveProjectBuildSecrets() {
	a := []string{"id=netrc,env=NETRC_CONTENT"}

	s.T().Setenv(BuildSecretInputEnv, "")
	s.Equal([]string{"id=netrc,env=NETRC_CONTENT"}, ResolveProjectBuildSecrets(nil, a))
	s.Nil(ResolveProjectBuildSecrets(nil, nil))

	s.T().Setenv(BuildSecretInputEnv, "id=pip,env=PIP_CONF\n")
	s.Equal([]string{"id=pip,env=PIP_CONF"}, ResolveProjectBuildSecrets(nil, a))

	s.Equal([]string{"id=ca,src=/etc/ca.pem"}, ResolveProjectBuildSecrets([]string{"id=ca,src=/etc/ca.pem"}, a))
}

func (s *Suite) TestBuildSecretHint() {
	s.Equal("pass --build-secret id=netrc,env=<VAR> or set BUILD_SECRET_INPUT", BuildSecretHint([]string{"netrc"}))
	s.Equal("pass --build-secret id=netrc,env=<VAR> --build-secret id=pip,env=<VAR> or set BUILD_SECRET_INPUT", BuildSecretHint([]string{"netrc", "pip"}))
}
