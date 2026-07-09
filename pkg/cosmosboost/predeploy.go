package cosmosboost

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/astronomer/astro-cli/pkg/logger"
)

// PreDeploy runs `astro-cosmos-boost pre-deploy <path>`, which discovers
// every dbt project (dbt_project.yml) and dbt manifest under path and writes
// a .astro/dbt_metadata.json sidecar beside each, carrying the content hash
// the parse-time consumer uses as its cache version key.
func PreDeploy(path string) error {
	out, err := exec.Command(BinaryPath(), "pre-deploy", path).CombinedOutput()
	logger.Debugf("astro-cosmos-boost pre-deploy output:\n%s", out)
	if err != nil {
		return fmt.Errorf("running astro-cosmos-boost pre-deploy: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// BestEffortStamp stamps .astro/dbt_metadata.json sidecars under path ahead
// of a deploy. Failures are reported as warnings and never returned: when
// the sidecar is absent the consumer falls back to hashing the project at
// parse time, so an unavailable or failing helper must not block a deploy.
func BestEffortStamp(path string) {
	if err := EnsureBinary(); err != nil {
		fmt.Printf("Warning: skipping the dbt pre-deploy stamping step: %s\n", err)
		return
	}
	if err := PreDeploy(path); err != nil {
		fmt.Printf("Warning: dbt pre-deploy stamping failed, continuing deploy: %s\n", err)
		return
	}
	fmt.Println("Pre-deploy: stamped dbt project metadata (.astro/dbt_metadata.json)")
}
