package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/astronomer/astro-cli/airflow/proxy"
	"github.com/astronomer/astro-cli/config"
	"github.com/astronomer/astro-cli/pkg/ansi"
	pkgproxy "github.com/astronomer/astro-cli/pkg/proxy"
)

func newProxyRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "proxy",
		Short: "Manage the local development reverse proxy",
		Long:  "Manage the reverse proxy that routes <project>.localhost to the correct local Airflow instance.",
		// No runtime needed for proxy commands
		PersistentPreRunE: SetupLogging,
	}
	cmd.AddCommand(
		newProxyStatusCmd(),
		newProxyStopCmd(),
		newProxyServeCmd(),
	)
	return cmd
}

func newProxyStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show proxy status and active routes",
		Long:  "Display the proxy daemon status and a table of all active project routes.",
		RunE:  proxyStatus,
	}
}

func newProxyStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the proxy daemon",
		Long:  "Force-stop the proxy daemon. It will restart automatically the next time local Airflow starts.",
		RunE:  proxyStop,
	}
}

func newProxyServeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "serve",
		Short:  "Run the proxy server (used internally)",
		Hidden: true,
		RunE:   proxyServe,
	}
	cmd.Flags().StringVar(&proxyPortFlag, "port", pkgproxy.DefaultPort, "Port to listen on")
	return cmd
}

func proxyStatus(_ *cobra.Command, _ []string) error {
	port := config.CFG.ProxyPort.GetString()
	if port == "" {
		port = pkgproxy.DefaultPort
	}

	routes, err := proxy.Routes().ListRoutes()
	if err != nil {
		return fmt.Errorf("error reading routes: %w", err)
	}

	pid, alive := proxy.IsRunning()
	switch {
	case alive:
		// Prefer the port the daemon reported at bind time over the config value.
		if bound := proxy.BoundPort(); bound != "" {
			port = bound
		}
		fmt.Printf("%s Proxy is running (PID %d) on port %s\n", ansi.Green("\u2714"), pid, port)
	case len(routes) > 0:
		// Routes exist but daemon is dead — auto-restart
		fmt.Println("Proxy is not running. Restarting…")
		if boundPort, ensureErr := proxy.EnsureRunning(port); ensureErr != nil {
			fmt.Printf("Warning: could not restart proxy: %s\n", ensureErr.Error())
		} else {
			port = boundPort
			pid, alive = proxy.IsRunning()
			if alive {
				fmt.Printf("%s Proxy restarted (PID %d) on port %s\n", ansi.Green("\u2714"), pid, port)
			}
		}
	default:
		fmt.Println("Proxy is not running.")
	}

	if len(routes) == 0 {
		fmt.Println("\nNo active routes.")
		return nil
	}

	fmt.Println("\nActive routes:")
	tw := tabwriter.NewWriter(os.Stdout, 0, 8, 2, '\t', 0) //nolint:mnd // the value is clear from context
	fmt.Fprintln(tw, "URL\tBackend Port\tPostgres Port\tProject Dir\tPID")
	for _, r := range routes {
		pgPort := "-"
		if p, ok := r.Services["postgres"]; ok {
			pgPort = p
		}
		url := fmt.Sprintf("http://%s:%s", r.Hostname, port)
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%d\n", url, r.Port, pgPort, r.ProjectDir, r.PID)
	}
	return tw.Flush()
}

func proxyStop(_ *cobra.Command, _ []string) error {
	_, wasRunning := proxy.IsRunning()

	if err := proxy.StopDaemon(); err != nil {
		return fmt.Errorf("error stopping proxy: %w", err)
	}

	if wasRunning {
		fmt.Println("Proxy stopped.")
	} else {
		fmt.Println("Proxy is not running.")
	}
	return nil
}

func proxyServe(_ *cobra.Command, _ []string) error {
	return proxy.Serve(proxyPortFlag)
}
