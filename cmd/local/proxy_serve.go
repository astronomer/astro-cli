package local

import (
	"github.com/spf13/cobra"

	proxydaemon "github.com/astronomer/astro-cli/airflow/proxy"
	"github.com/astronomer/astro-cli/pkg/proxy"
)

// newProxyServeCmd builds the hidden `astro __proxy-serve` subcommand: the
// reverse-proxy server loop the daemon re-execs into (airflow/proxy's
// StartDaemon spawns `<astro> __proxy-serve --port <port>`). Never typed by
// users. It mirrors the standalone engine's `__supervise` convention, and
// re-homes the server that v2's dev-tree removal took `dev proxy serve` with.
func newProxyServeCmd(_ Deps) *cobra.Command {
	var port string
	cmd := &cobra.Command{
		Use:           proxydaemon.ServeSubcommand,
		Short:         "Run the local reverse proxy server (internal)",
		Hidden:        true,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return proxydaemon.Serve(port)
		},
	}
	cmd.Flags().StringVar(&port, "port", proxy.DefaultPort, "Port to listen on")
	markSkipPreRun(cmd)
	return cmd
}
