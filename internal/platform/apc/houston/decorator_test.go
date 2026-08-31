package houston

import (
	"errors"
)

var errMockHouston = errors.New("mock houston error")

func (s *Suite) Test_isCalledFromUnitTestFile() {
	if got := isCalledFromUnitTestFile(); got != true {
		s.Fail("isCalledFromUnitTestFile() = %v, want %v", got, true)
	}
}

func (s *Suite) TestSanitiseVersionString() {
	type args struct {
		v string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "basic case",
			args: args{v: "0.29.0"},
			want: "v0.29.0",
		},
		{
			name: "version with pre-release",
			args: args{v: "0.29.0-rc2"},
			want: "v0.29.0",
		},
		{
			name: "version with v as prefix",
			args: args{v: "v0.29.2"},
			want: "v0.29.2",
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			if got := sanitiseVersionString(tt.args.v); got != tt.want {
				s.Fail("sanitiseVersionString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func funcToTest(_ interface{}) (interface{}, error) { return nil, nil }

func (s *Suite) TestGetFunctionName() {
	if got := getFunctionName(funcToTest); got != "funcToTest" {
		s.Fail("GetFunctionName() = %v, want %v", got, "funcToTest")
	}
}

func (s *Suite) TestVerifyVersionMatch() {
	type args struct {
		version         string
		funcRestriction VersionRestrictions
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "basic case",
			args: args{version: "0.30.0", funcRestriction: VersionRestrictions{GTE: "0.29.0"}},
			want: true,
		},
		{
			name: "equal case",
			args: args{version: "0.30.0", funcRestriction: VersionRestrictions{EQ: []string{"0.29.0", "0.30.0"}}},
			want: true,
		},
		{
			name: "equal case without version",
			args: args{version: "0.31.0", funcRestriction: VersionRestrictions{EQ: []string{"0.29.0", "0.30.0"}}},
			want: false,
		},
		{
			name: "equal case with v prefix",
			args: args{version: "v0.30.0", funcRestriction: VersionRestrictions{EQ: []string{"0.29.0", "0.30.0"}}},
			want: true,
		},
		{
			name: "case with gte and lt",
			args: args{version: "0.30.0", funcRestriction: VersionRestrictions{GTE: "0.29.0", LT: "0.31.0"}},
			want: true,
		},
		{
			name: "case with version outside gte and lt",
			args: args{version: "0.28.0", funcRestriction: VersionRestrictions{GTE: "0.29.0", LT: "0.31.0"}},
			want: false,
		},
		{
			name: "case with version outside gte and lt",
			args: args{version: "0.31.0", funcRestriction: VersionRestrictions{GTE: "0.29.0", LT: "0.31.0"}},
			want: false,
		},
		{
			name: "case with lt",
			args: args{version: "0.30.0", funcRestriction: VersionRestrictions{LT: "0.31.0"}},
			want: true,
		},
		{
			name: "case with no restriction",
			args: args{version: "0.30.0", funcRestriction: VersionRestrictions{}},
			want: true,
		},
		{
			name: "empty version",
			args: args{version: "", funcRestriction: VersionRestrictions{GTE: "0.31.0"}},
			want: true,
		},
		{
			name: "invalid version",
			args: args{version: "invalid", funcRestriction: VersionRestrictions{GTE: "0.31.0"}},
			want: true,
		},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			if got := VerifyVersionMatch(tt.args.version, tt.args.funcRestriction); got != tt.want {
				s.Fail("VerifyVersionMatch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func (s *Suite) TestCall() {
	s.Run("basic case", func() {
		resp, err := Call(funcToTest)(nil)
		s.NoError(err)
		s.Nil(resp)
	})

	s.Run("basic case bypassing test file check", func() {
		ApplyDecoratorForTests = true
		defer func() { ApplyDecoratorForTests = false }()
		resp, err := Call(funcToTest)(nil)
		s.NoError(err)
		s.Nil(resp)
	})

	s.Run("basic case with method restriction", func() {
		ApplyDecoratorForTests = true
		defer func() { ApplyDecoratorForTests = false }()
		version = "0.30.0"
		houstonAPIAvailabilityByVersion["funcToTest"] = VersionRestrictions{GTE: "0.30.0"}
		resp, err := Call(funcToTest)(nil)
		s.NoError(err)
		s.Nil(resp)
	})

	s.Run("case when version is empty string", func() {
		ApplyDecoratorForTests = true
		defer func() { ApplyDecoratorForTests = false }()
		version = "invalid"
		versionErr = errMockHouston
		houstonAPIAvailabilityByVersion["funcToTest"] = VersionRestrictions{GTE: "0.30.0"}
		resp, err := Call(funcToTest)(nil)
		s.NoError(err)
		s.Nil(resp)
	})

	s.Run("negative case with method restriction", func() {
		ApplyDecoratorForTests = true
		defer func() { ApplyDecoratorForTests = false }()
		version = "0.29.0"
		houstonAPIAvailabilityByVersion["funcToTest"] = VersionRestrictions{GTE: "0.30.0"}
		resp, err := Call(funcToTest)(nil)
		s.ErrorIs(err, ErrAPINotImplemented{APIName: "funcToTest"})
		s.Nil(resp)
	})
}

func (s *Suite) TestGetVersion() {
	s.Run("when version is already present", func() {
		version = "0.30.0"
		resp := getVersion()
		s.Equal(version, resp)
	})

	s.Run("when version error is already present", func() {
		versionErr = errMockHouston
		version = ""
		resp := getVersion()
		s.Equal(version, resp)
	})
}
