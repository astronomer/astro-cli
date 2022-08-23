package houston

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_isCalledFromUnitTestFile(t *testing.T) {
	if got := isCalledFromUnitTestFile(); got != true {
		t.Errorf("isCalledFromUnitTestFile() = %v, want %v", got, true)
	}
}

func TestSetVersion(t *testing.T) {
	type args struct {
		v string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "set version",
			args: args{v: "0.29.0"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetVersion(tt.args.v)
			assert.Equal(t, version, tt.args.v)
		})
	}
}

func TestSanitiseVersionString(t *testing.T) {
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
		t.Run(tt.name, func(t *testing.T) {
			if got := sanitiseVersionString(tt.args.v); got != tt.want {
				t.Errorf("sanitiseVersionString() = %v, want %v", got, tt.want)
			}
		})
	}
}

func funcToTest(_ interface{}) (interface{}, error) { return nil, nil }

func TestGetFunctionName(t *testing.T) {
	if got := getFunctionName(funcToTest); got != "funcToTest" {
		t.Errorf("GetFunctionName() = %v, want %v", got, "funcToTest")
	}

}

func TestVerifyVersionMatch(t *testing.T) {
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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VerifyVersionMatch(tt.args.version, tt.args.funcRestriction); got != tt.want {
				t.Errorf("VerifyVersionMatch() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCall(t *testing.T) {
	t.Run("basic case", func(t *testing.T) {
		resp, err := Call(funcToTest, nil)
		assert.NoError(t, err)
		assert.Nil(t, resp)
	})

	t.Run("basic case bypassing test file check", func(t *testing.T) {
		ApplyDecoratorForTests = true
		defer func() { ApplyDecoratorForTests = false }()
		version = "0.29.0"
		resp, err := Call(funcToTest, nil)
		assert.NoError(t, err)
		assert.Nil(t, resp)
	})

	t.Run("basic case with method restriction", func(t *testing.T) {
		ApplyDecoratorForTests = true
		defer func() { ApplyDecoratorForTests = false }()
		version = "0.29.0"
		houstonMethodAvailabilityByVersion["funcToTest"] = VersionRestrictions{GTE: "0.29.0"}
		resp, err := Call(funcToTest, nil)
		assert.NoError(t, err)
		assert.Nil(t, resp)
	})

	t.Run("negative case with method restriction", func(t *testing.T) {
		ApplyDecoratorForTests = true
		defer func() { ApplyDecoratorForTests = false }()
		version = "0.28.0"
		houstonMethodAvailabilityByVersion["funcToTest"] = VersionRestrictions{GTE: "0.29.0"}
		resp, err := Call(funcToTest, nil)
		assert.ErrorIs(t, err, ErrMethodNotImplemented{MethodName: "funcToTest"})
		assert.Nil(t, resp)
	})
}
