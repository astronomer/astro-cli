package imageinstall

import (
	"fmt"
	"path"
	"strconv"
	"strings"
)

// Which wheel of a routed package an image installs is decided here, on the
// host, for the image rather than for the host: CPython on Linux, at the
// image's Python, for each machine an image is built for. The tags and their
// order are the ones `packaging.tags.sys_tags()` gives such an interpreter,
// which is how pip and uv rank a package's wheels.

// imageMachines are the machines an image is built for, as platform_machine
// and the wheel platform tags spell them: linux/amd64 and linux/arm64.
var imageMachines = []string{"x86_64", "aarch64"}

// manylinuxGlibc is the newest glibc whose manylinux wheels count as
// compatible. The runtime images run 2.31 (runtime 9 and 10, Debian 11), 2.36
// (11 to 13, Debian 12) and 2.41 (3.3, Debian 13); 2.28 is what every one of
// them has, and the most common floor a wheel is built for after 2.17. A
// package that publishes only for a newer glibc installs from its sdist.
const manylinuxGlibc = 28

// manylinux2014Glibc is the glibc minor manylinux2014, the oldest tag an
// aarch64 wheel is built for, names.
const manylinux2014Glibc = 17

// wheelNameParts is how many dash-separated parts a wheel's file name has at
// the least: name, version, Python tag, ABI tag, platform tag.
const wheelNameParts = 5

// imageTags ranks the wheel tags an image's CPython (python, "3.12") on
// machine installs, best first, as rank by tag. With python unknown, only the
// tags any Python 3 takes.
func imageTags(python, machine string) map[string]int {
	minor := -1
	if major, m, ok := strings.Cut(python, "."); ok && major == "3" {
		if n, err := strconv.Atoi(m); err == nil {
			minor = n
		}
	}
	var platforms []string
	for g := manylinuxGlibc; g >= manylinux2014Glibc; g-- {
		platforms = append(platforms, fmt.Sprintf("manylinux_2_%d_%s", g, machine))
		if g == manylinux2014Glibc {
			platforms = append(platforms, "manylinux2014_"+machine)
		}
	}
	if machine == "x86_64" {
		platforms = append(platforms, "manylinux_2_12_x86_64", "manylinux2010_x86_64", "manylinux_2_5_x86_64", "manylinux1_x86_64")
	}
	platforms = append(platforms, "linux_"+machine)

	var tags []string
	add := func(interp, abi string, plats ...string) {
		for _, p := range plats {
			tags = append(tags, interp+"-"+abi+"-"+p)
		}
	}
	// pyVersions are py3Y, py3, py3(Y-1) … py30, as packaging's
	// _py_interpreter_range gives them.
	var pyVersions []string
	if minor >= 0 {
		cp := "cp3" + strconv.Itoa(minor)
		add(cp, cp, platforms...)
		add(cp, "abi3", platforms...)
		add(cp, "none", platforms...)
		for m := minor - 1; m >= 2; m-- {
			add("cp3"+strconv.Itoa(m), "abi3", platforms...)
		}
		pyVersions = append(pyVersions, "py3"+strconv.Itoa(minor), "py3")
		for m := minor - 1; m >= 0; m-- {
			pyVersions = append(pyVersions, "py3"+strconv.Itoa(m))
		}
	} else {
		pyVersions = []string{"py3"}
	}
	for _, v := range pyVersions {
		add(v, "none", platforms...)
	}
	if minor >= 0 {
		add("cp3"+strconv.Itoa(minor), "none", "any")
	}
	for _, v := range pyVersions {
		add(v, "none", "any")
	}

	rank := make(map[string]int, len(tags))
	for i, t := range tags {
		if _, ok := rank[t]; !ok {
			rank[t] = i
		}
	}
	return rank
}

// wheelTags are the tags a wheel's file name states, its compressed tag sets
// expanded (py2.py3-none-any is py2-none-any and py3-none-any), or none for a
// name that is not a wheel's.
func wheelTags(url string) []string {
	name := path.Base(strings.SplitN(url, "#", 2)[0])
	stem, ok := strings.CutSuffix(name, ".whl")
	if !ok {
		return nil
	}
	parts := strings.Split(stem, "-")
	if len(parts) < wheelNameParts {
		return nil
	}
	n := len(parts)
	var tags []string
	for _, py := range strings.Split(parts[n-3], ".") {
		for _, abi := range strings.Split(parts[n-2], ".") {
			for _, plat := range strings.Split(parts[n-1], ".") {
				tags = append(tags, py+"-"+abi+"-"+plat)
			}
		}
	}
	return tags
}
