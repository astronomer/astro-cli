// Post-processes astro-client-v1/api.gen.go to fix an oapi-codegen bug
// (present through at least v2.6.0).
//
// For each oneOf variant, oapi-codegen emits From*/Merge* setter helpers that
// assign an untyped string literal to the discriminator field:
//
//	v.Type = "DEDICATED"
//
// When the variant schema doesn't list the discriminator under `required`,
// oapi-codegen generates the field as a pointer (e.g. *CreateDedicatedDeploymentRequestType),
// and the assignment fails to compile.
//
// Four schemas are affected:
//
//	CreateDedicatedDeploymentRequest.Type
//	CreateHybridDeploymentRequest.Type
//	CreateStandardDeploymentRequest.Type
//	UpdateDedicatedClusterRequest.ClusterType
//
// We rewrite each bad assignment to `new(<TypedConstant>)`, preserving the
// setter's discriminator-tagging semantics instead of just dropping the line.
// The Update*DeploymentRequest helpers use a non-pointer Type field (the spec
// marks it required there) and compile as-is, so we leave them alone — which
// is why the patcher needs to know which function it's inside.
package main

import (
	"fmt"
	"io/fs"
	"os"
	"slices"
	"strings"
)

const outputPerm fs.FileMode = 0o600

type rewrite struct {
	setters []string // enclosing function names whose body contains oldLine
	oldLine string
	newLine string
}

var rewrites = []rewrite{{
	setters: []string{"FromCreateDedicatedDeploymentRequest", "MergeCreateDedicatedDeploymentRequest"},
	oldLine: "\tv.Type = \"DEDICATED\"",
	newLine: "\tv.Type = new(CreateDedicatedDeploymentRequestTypeDEDICATED)",
}, {
	setters: []string{"FromCreateHybridDeploymentRequest", "MergeCreateHybridDeploymentRequest"},
	oldLine: "\tv.Type = \"HYBRID\"",
	newLine: "\tv.Type = new(CreateHybridDeploymentRequestTypeHYBRID)",
}, {
	setters: []string{"FromCreateStandardDeploymentRequest", "MergeCreateStandardDeploymentRequest"},
	oldLine: "\tv.Type = \"STANDARD\"",
	newLine: "\tv.Type = new(CreateStandardDeploymentRequestTypeSTANDARD)",
}, {
	setters: []string{"FromUpdateDedicatedClusterRequest", "MergeUpdateDedicatedClusterRequest"},
	oldLine: "\tv.ClusterType = \"DEDICATED\"",
	newLine: "\tv.ClusterType = new(UpdateDedicatedClusterRequestClusterTypeDEDICATED)",
}}

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: patch-v1-gen <path/to/api.gen.go>")
		os.Exit(2)
	}
	path := os.Args[1]

	src, err := os.ReadFile(path) //nolint:gosec // developer-run codegen tool, not user input
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	out, applied := patch(string(src))

	want := 0
	for _, r := range rewrites {
		want += len(r.setters)
	}
	if applied != want {
		fmt.Fprintf(os.Stderr,
			"patch-v1-gen: applied %d rewrites, expected %d — oapi-codegen output shape may have changed\n",
			applied, want)
		os.Exit(1)
	}

	if err := os.WriteFile(path, []byte(out), outputPerm); err != nil { //nolint:gosec // developer-run codegen tool, not user input
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// patch walks src line-by-line, tracks the enclosing function name, and
// applies the rewrite whose setters list contains the current function.
// Returns the rewritten source and the number of substitutions performed.
func patch(src string) (out string, applied int) {
	lines := strings.Split(src, "\n")
	var currentFunc string

	for i, line := range lines {
		if name, ok := funcName(line); ok {
			currentFunc = name
			continue
		}
		for _, r := range rewrites {
			if line == r.oldLine && slices.Contains(r.setters, currentFunc) {
				lines[i] = r.newLine
				applied++
			}
		}
	}
	return strings.Join(lines, "\n"), applied
}

// funcName extracts the method name from a line of the form
// `func (recv) Name(args) result {`. Returns "" if the line isn't a method
// declaration.
func funcName(line string) (string, bool) {
	if !strings.HasPrefix(line, "func (") {
		return "", false
	}
	_, after, ok := strings.Cut(line, ") ")
	if !ok {
		return "", false
	}
	name, _, ok := strings.Cut(after, "(")
	return name, ok
}
