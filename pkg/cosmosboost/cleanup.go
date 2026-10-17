package cosmosboost

import "fmt"

// Cleanup removes the Cosmos Boost artifacts under the given roots (default
// ".").
func Cleanup(roots ...string) error {
	if len(roots) == 0 {
		roots = []string{"."}
	}
	if err := removeArtifacts(roots); err != nil {
		return err
	}
	fmt.Println("Removed the Cosmos Boost artifacts")
	return nil
}
