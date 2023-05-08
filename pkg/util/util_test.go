package util

import (
	"errors"
	"testing"

	"github.com/Masterminds/semver"
	"github.com/stretchr/testify/suite"
)

type Suite struct {
	suite.Suite
}

func TestPkgUtilSuite(t *testing.T) {
	suite.Run(t, new(Suite))
}

func (s *Suite) TestCoerce() {
	type args struct {
		version string
	}
	tests := []struct {
		name string
		args args
		want *semver.Version
	}{
		{
			name: "valid case",
			args: args{version: "2.2.2"},
			want: semver.MustParse("2.2.2"),
		},
		{
			name: "invalid case",
			args: args{version: "test"},
			want: nil,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got := Coerce(tt.args.version)
			s.Equal(got, tt.want)
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
			s.Equal(Contains(tt.args.elems, tt.args.v), tt.want)
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
			s.Equal(gotResult, tt.wantResult)
			s.Equal(gotFound, tt.wantFound)
		})
	}
}

func (s *Suite) TestExists() {
	type args struct {
		path string
	}
	tests := []struct {
		name    string
		args    args
		want    bool
		wantErr bool
	}{
		{
			name:    "valid case",
			args:    args{"./util_test.go"},
			want:    true,
			wantErr: false,
		},
		{
			name:    "invalid case",
			args:    args{"./test.go"},
			want:    false,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			got, err := Exists(tt.args.path)
			if (err != nil) != tt.wantErr {
				s.Error(err)
				return
			}
			s.Equal(got, tt.want)
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
			s.Equal(Base64URLEncode(tt.args.arg), tt.want)
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
			s.Equal(CheckEnvBool(tt.args.arg), tt.want)
		})
	}
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

var (
	errMalformedCurrentVersion = errors.New("Malformed version: invalid current version") //nolint:stylecheck
	errMalformedConstraint     = errors.New("Malformed constraint: invalid constraint")   //nolint:stylecheck
)

func (s *Suite) TestIsRequiredVersionMet() {
	type args struct {
		currentVersion  string
		requiredVersion string
	}
	type result struct {
		valid bool
		err   error
	}

	tests := []struct {
		name string
		args args
		want result
	}{
		{
			name: "first true case",
			args: args{"7.1.0", ">7.0.0, <8.0.0"},
			want: result{true, nil},
		},
		{
			name: "first false case",
			args: args{"7.1.0", ">7.1.0"},
			want: result{false, nil},
		},
		{
			name: "first error case",
			args: args{"invalid current version", ">7.0.0, <8.0.0"},
			want: result{false, errMalformedCurrentVersion},
		},
		{
			name: "second error case",
			args: args{"7.1.0", "invalid constraint"},
			want: result{false, errMalformedConstraint},
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			versionMet, err := IsRequiredVersionMet(tt.args.currentVersion, tt.args.requiredVersion)
			s.Equal(versionMet, tt.want.valid)
			if err != nil && err.Error() != tt.want.err.Error() {
				s.Errorf(err, "IsRequiredVersionMet() = %v; want %v, %v", versionMet, tt.want.valid, tt.want.err)
			}
		})
	}
}
