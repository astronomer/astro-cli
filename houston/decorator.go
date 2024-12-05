package houston

import (
	"reflect"
	"runtime"
	"strings"

	"golang.org/x/mod/semver"
)

var (
	ApplyDecoratorForTests bool

	version    string
	versionErr error
)

const (
	DesiredMethodDepth = 2 // 2 because getCallerFunctionName will be called from ValidateAvailability which is called from the function which we are interested in
	MaxHoustonVersion  = "1.0.0"
)

type VersionRestrictions struct {
	GTE string
	LT  string
	EQ  []string
}

// APIs availability based on the version they were added/removed in Houston
var houstonAPIAvailabilityByVersion = map[string]VersionRestrictions{
	"ListPaginatedDeployments":          {GTE: "0.32.0"},
	"WorkspacesPaginatedGetRequest":     {GTE: "0.30.0"},
	"WorkspacePaginatedGetUsersRequest": {GTE: "0.30.0"},

	"UpdateDeploymentImage":       {GTE: "0.29.2"},
	"CreateTeamSystemRoleBinding": {GTE: "0.29.2"},
	"DeleteTeamSystemRoleBinding": {GTE: "0.29.2"},

	"UpdateDeploymentRuntime": {GTE: "0.29.0"},
	"GetRuntimeReleases":      {GTE: "0.29.0"},

	"GetTeam":                    {GTE: "0.28.0"},
	"GetTeamUsers":               {GTE: "0.28.0"},
	"ListTeams":                  {GTE: "0.28.0"},
	"AddWorkspaceTeam":           {GTE: "0.28.0"},
	"DeleteWorkspaceTeam":        {GTE: "0.28.0"},
	"ListWorkspaceTeamsAndRoles": {GTE: "0.28.0"},
	"UpdateWorkspaceTeamRole":    {GTE: "0.28.0"},
	"GetWorkspaceTeamRole":       {GTE: "0.28.0"},
}

func Call[fReq any, fResp any, fType func(any) (any, error)](houstonFunc func(fReq) (fResp, error)) func(fReq) (fResp, error) {
	return func(r fReq) (fResp, error) {
		if !ApplyDecoratorForTests && isCalledFromUnitTestFile() { // bypassing this decorator for unit tests
			return houstonFunc(r)
		}

		// get the current version of the connected platform
		houstonVersion := getVersion()

		// get function name of houstonFunc
		funcName := getFunctionName(houstonFunc)
		// check if any restrictions are defined on that API, else return
		funcRestriction, ok := houstonAPIAvailabilityByVersion[funcName]
		if !ok {
			return houstonFunc(r)
		}

		// validate if the platform version fits within API restrictions
		if VerifyVersionMatch(houstonVersion, funcRestriction) {
			return houstonFunc(r)
		}
		var res fResp
		return res, ErrAPINotImplemented{APIName: funcName}
	}
}

func VerifyVersionMatch(version string, funcRestriction VersionRestrictions) bool {
	orgVersion := version
	version = sanitiseVersionString(version)
	if !semver.IsValid(version) { // fallback to the version higher than the latest
		orgVersion = MaxHoustonVersion
		version = sanitiseVersionString(MaxHoustonVersion)
	}

	if len(funcRestriction.EQ) > 0 {
		for _, versionAvailable := range funcRestriction.EQ {
			if orgVersion == versionAvailable || semver.Compare(version, sanitiseVersionString(versionAvailable)) == 0 {
				return true
			}
		}
		return false
	}

	if funcRestriction.GTE != "" && funcRestriction.LT != "" {
		funcRestriction.GTE = sanitiseVersionString(funcRestriction.GTE)
		funcRestriction.LT = sanitiseVersionString(funcRestriction.LT)

		if semver.Compare(version, funcRestriction.GTE) >= 0 && semver.Compare(version, funcRestriction.LT) < 0 {
			return true
		}
		return false
	}
	if funcRestriction.GTE != "" {
		funcRestriction.GTE = sanitiseVersionString(funcRestriction.GTE)

		return semver.Compare(version, funcRestriction.GTE) >= 0
	}
	if funcRestriction.LT != "" {
		funcRestriction.LT = sanitiseVersionString(funcRestriction.LT)

		return semver.Compare(version, funcRestriction.LT) < 0
	}
	return true
}

func getFunctionName(i interface{}) string {
	runtimeFuncName := runtime.FuncForPC(reflect.ValueOf(i).Pointer()).Name() // naming convention: github.com/astronomer/astro-cli/houston.ClientInterface.GetTeam-fm
	methodName := runtimeFuncName[strings.LastIndex(runtimeFuncName, ".")+1:] // split by . and get the last part of it
	methodName, _, _ = strings.Cut(methodName, "-")                           // get the string before the first instance of hyphen
	return methodName
}

// prepare version string to be consumed by "golang.org/x/mod/semver" package
func sanitiseVersionString(v string) string {
	if !strings.HasPrefix(v, "v") {
		v = "v" + v
	}
	preRelease := semver.Prerelease(v)
	if preRelease != "" {
		v = strings.TrimSuffix(v, preRelease)
	}
	return v
}

func isCalledFromUnitTestFile() bool {
	// depth is usually 4 because unit test method will call the actual testing method
	// which will internally call the decorator function and then `isCalledFromUnitTestFile` will be called
	for fileDepth := 1; fileDepth <= 10; fileDepth++ {
		_, file, _, ok := runtime.Caller(fileDepth)
		if ok && strings.HasSuffix(file, "_test.go") { // function is called from a unit test file
			return true
		}
	}
	return false
}

func getVersion() string {
	if version != "" {
		return version
	}

	// fallback case in which somehow we reach here without getting houston version
	httpClient := NewHTTPClient()
	client := NewClient(httpClient)

	version, versionErr = client.GetPlatformVersion(nil)
	return version
}
