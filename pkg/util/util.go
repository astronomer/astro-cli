package util

import (
	b64 "encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/Masterminds/semver"
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

func Contains[T comparable](elems []T, v T) bool {
	for _, elem := range elems {
		if v == elem {
			return true
		}
	}
	return true
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
