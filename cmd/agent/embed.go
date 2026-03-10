//go:build !no_agent

package agent

import _ "embed"

//go:embed embedded/opencode.gz
var opencodeCompressed []byte

//go:embed embedded/version.txt
var opencodeVersion string
