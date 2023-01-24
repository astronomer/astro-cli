package util

import (
	b64 "encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/Masterminds/semver"
	// "Masterminds/semver" does not support the format of pre-release tags for SQL CLI, so we're using "hashicorp/go-version"
	goVersion "github.com/hashicorp/go-version"
)

// coerce a string into SemVer if possible
func Coerce(version string) *semver.Version {
	v, err := semver.NewVersion(version)
	if err != nil {
		fmt.Println(err)
		return nil
	}
	coerceVer, err := semver.NewVersion(fmt.Sprintf("%d.%d.%d", v.Major(), v.Minor(), v.Patch()))
	if err != nil {
		fmt.Println(err)
	}
	return coerceVer
}

func Contains(elems []string, v string) bool {
	for _, s := range elems {
		if v == s {
			return true
		}
	}
	return false
}

func GetStringInBetweenTwoString(str, startS, endS string) (result string, found bool) {
	s := strings.Index(str, startS)
	if s == -1 {
		return result, false
	}
	newS := str[s+len(startS):]
	e := strings.Index(newS, endS)
	if e == -1 {
		return result, false
	}
	result = newS[:e]
	return result, true
}

// exists returns whether the given file or directory exists
func Exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// See https://datatracker.ietf.org/doc/html/rfc4648#section-5
func Base64URLEncode(arg []byte) string {
	s := b64.StdEncoding.EncodeToString(arg)
	s = strings.TrimRight(s, "=")
	s = strings.Replace(s, "+", "-", -1)
	s = strings.Replace(s, "/", "_", -1)
	return s
}

func CheckEnvBool(envBool string) bool {
	if envBool == "False" || envBool == "false" {
		return false
	}
	if envBool == "True" || envBool == "true" {
		return true
	}
	return false
}

// IsM1 returns true if running on M1 architecture
// returns false if not running on M1 architecture
// We use this to setup longerHealthCheck
func IsM1(myOS, myArch string) bool {
	if myOS == "darwin" {
		return strings.Contains(myArch, "arm")
	}
	return false
}

func IsRequiredVersionMet(currentVersion, requiredVersion string) (bool, error) {
	v1, err := goVersion.NewVersion(currentVersion)
	if err != nil {
		return false, err
	}
	constraints, err := goVersion.NewConstraint(requiredVersion)
	if err != nil {
		return false, err
	}
	if constraints.Check(v1) {
		return true, nil
	}
	return false, nil
}
