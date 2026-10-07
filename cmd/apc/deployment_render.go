package apc

import (
	"bufio"
	"fmt"
	"io"

	"github.com/fatih/camelcase"

	"github.com/astronomer/astro-cli/cmd/cliout"
	"github.com/astronomer/astro-cli/internal/platform/apc/deployment"
	"github.com/astronomer/astro-cli/internal/platform/apc/houston"
	"github.com/astronomer/astro-cli/pkg/printutil"
)

// The shapes the APC deployment family publishes under --output json, and the
// text each command has always printed, drawn from the same values. The
// platform functions return what Houston said; this is the one place that
// decides how it looks.

// deploymentJSON is one APC Deployment.
//
// The fields after release_name are null when Houston gave no value for
// them: either the command's query does not ask for the field (create has no
// image_tag yet, a list on an older Houston no namespace or cluster_id), or
// the Deployment has none (airflow_version on a Runtime Deployment). Houston
// returns no value as no value, never as an empty string, so a field is a
// string with something in it, or null; never "".
type deploymentJSON struct {
	DeploymentID string `json:"deployment_id"`
	Label        string `json:"label"`
	// ReleaseName is the Deployment's Kubernetes release name, the tables'
	// DEPLOYMENT NAME.
	ReleaseName string  `json:"release_name"`
	WorkspaceID *string `json:"workspace_id"`
	ClusterID   *string `json:"cluster_id"`
	Namespace   *string `json:"namespace"`
	// ChartVersion is the Airflow Helm chart version the Deployment runs,
	// the tables' ASTRO column: Houston's Deployment.version (houston-api
	// ). It is null on what create returns, until
	// Houston's worker sets it (src/workers/deployment-upserted-for-create).
	ChartVersion *string `json:"chart_version"`
	// AirflowVersion is set on an Astronomer Certified image,
	// RuntimeVersion on a Runtime one.
	AirflowVersion *string `json:"airflow_version"`
	RuntimeVersion *string `json:"runtime_version"`
	// ImageTag is the tag of the image deployed now, the tables' TAG.
	ImageTag          *string `json:"image_tag"`
	DagDeploymentType *string `json:"dag_deployment_type"`
	// URLs is [] when Houston gave none.
	URLs []deploymentURLJSON `json:"urls"`
}

type deploymentURLJSON struct {
	// Type is what the URL serves: airflow, flower, registry.
	Type string `json:"type"`
	URL  string `json:"url"`
}

// deploymentListJSON is `astro deployment list`.
type deploymentListJSON struct {
	Deployments []deploymentJSON `json:"deployments"`
}

// deploymentRemovalJSON is what `astro deployment delete` and `unadopt`
// removed. Action is "deleted" or "unadopted". Label, release_name and
// workspace_id are null when Houston did not say (it answered with no
// record), as deploymentJSON's are.
type deploymentRemovalJSON struct {
	DeploymentID string  `json:"deployment_id"`
	Label        *string `json:"label"`
	ReleaseName  *string `json:"release_name"`
	WorkspaceID  *string `json:"workspace_id"`
	Action       string  `json:"action"`
}

// versionChangeJSON is what `astro deployment airflow upgrade`, `runtime
// upgrade` and `runtime migrate` did, or what canceling one did. Action is
// "started", "canceled" or "nothing_to_cancel". Desired is null unless one
// started.
type versionChangeJSON struct {
	DeploymentID string            `json:"deployment_id"`
	Label        string            `json:"label"`
	ReleaseName  string            `json:"release_name"`
	Action       string            `json:"action"`
	Current      imageVersionJSON  `json:"current"`
	Desired      *imageVersionJSON `json:"desired"`
}

// imageVersionJSON is an image a Deployment runs. Image is
// "astronomer_certified", whose Version is an Airflow version, or "runtime".
type imageVersionJSON struct {
	Image   string `json:"image"`
	Version string `json:"version"`
}

// logEntryJSON is one log record of `astro deployment logs`, one per line.
type logEntryJSON struct {
	// Component is the one asked for: webserver, scheduler, worker,
	// triggerer.
	Component string `json:"component"`
	// Timestamp is the record's, as Houston gives it.
	Timestamp string `json:"timestamp"`
	Message   string `json:"message"`
}

// known is s when Houston gave a value, and nil when it gave none.
func known(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func newDeploymentJSON(d *houston.Deployment) deploymentJSON {
	urls := make([]deploymentURLJSON, 0, len(d.Urls))
	for _, u := range d.Urls {
		urls = append(urls, deploymentURLJSON{Type: u.Type, URL: u.URL})
	}
	return deploymentJSON{
		DeploymentID:      d.ID,
		Label:             d.Label,
		ReleaseName:       d.ReleaseName,
		WorkspaceID:       known(d.Workspace.ID),
		ClusterID:         known(d.ClusterID),
		Namespace:         known(d.Namespace),
		ChartVersion:      known(d.Version),
		AirflowVersion:    known(d.AirflowVersion),
		RuntimeVersion:    known(d.RuntimeVersion),
		ImageTag:          known(d.DeploymentInfo.Current),
		DagDeploymentType: known(d.DagDeployment.Type),
		URLs:              urls,
	}
}

func newRemovalJSON(id string, d *houston.Deployment, action string) deploymentRemovalJSON {
	r := deploymentRemovalJSON{DeploymentID: id, Action: action}
	if d != nil {
		if d.ID != "" {
			r.DeploymentID = d.ID
		}
		r.Label, r.ReleaseName, r.WorkspaceID = known(d.Label), known(d.ReleaseName), known(d.Workspace.ID)
	}
	return r
}

func newVersionChangeJSON(c *deployment.VersionChange) versionChangeJSON {
	out := versionChangeJSON{
		DeploymentID: c.Deployment.ID,
		Label:        c.Deployment.Label,
		ReleaseName:  c.Deployment.ReleaseName,
		Action:       c.Action,
		Current:      imageVersionJSON{Image: c.Current.Image, Version: c.Current.Version},
	}
	if c.Desired != nil {
		out.Desired = &imageVersionJSON{Image: c.Desired.Image, Version: c.Desired.Version}
	}
	return out
}

// image is the image a Deployment runs. preferRuntime says which version
// field decides when a table reads it: the list reads Runtime first, create
// and update read Airflow first, as each always has.
func image(d *houston.Deployment, preferRuntime bool) deployment.ImageVersion {
	runtime := deployment.ImageVersion{Image: deployment.ImageRuntime, Version: d.RuntimeVersion}
	certified := deployment.ImageVersion{Image: deployment.ImageCertified, Version: d.AirflowVersion}
	if preferRuntime {
		if d.RuntimeVersion != "" {
			return runtime
		}
		return certified
	}
	if d.AirflowVersion != "" {
		return certified
	}
	return runtime
}

// deploymentTable is the six-column table create, update and list print.
func deploymentTable() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{30, 30, 10, 50, 10, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "DEPLOYMENT NAME", "ASTRO", "DEPLOYMENT ID", "TAG", "IMAGE VERSION"},
	}
}

// versionTable is the five-column table an upgrade or migration prints.
func versionTable() *printutil.Table {
	return &printutil.Table{
		Padding:        []int{30, 30, 10, 50, 10},
		DynamicPadding: true,
		Header:         []string{"NAME", "DEPLOYMENT NAME", "ASTRO", "DEPLOYMENT ID", "IMAGE VERSION"},
	}
}

// tagOrUnknown is a TAG cell: the current image tag, or "?".
func tagOrUnknown(d *houston.Deployment) string {
	if d.DeploymentInfo.Current == "" {
		return "?"
	}
	return d.DeploymentInfo.Current
}

// emitCreated publishes the Deployment create made. executor is the one
// asked for, which names the executor in the message and decides whether
// there is a Flower URL.
func emitCreated(r cliout.Renderer, d *houston.Deployment, executor string) error {
	return r.Emit(newDeploymentJSON(d), func(w io.Writer) error {
		tab := deploymentTable()
		tab.AddRow([]string{d.Label, d.ReleaseName, d.Version, d.ID, "-", image(d, false).Label()}, false)

		splitted := []string{"Celery", ""}
		if executor != "" {
			// trim executor from console message
			splitted = camelcase.Split(executor)
		}
		var airflowURL, flowerURL string
		for _, url := range d.Urls {
			if url.Type == "airflow" {
				airflowURL = url.URL
			}
			if url.Type == "flower" {
				flowerURL = url.URL
			}
		}
		tab.SuccessMsg = fmt.Sprintf("\n Successfully created deployment with %s executor", splitted[0]) +
			". Deployment can be accessed at the following URLs \n" +
			fmt.Sprintf("\n Airflow Dashboard: %s", airflowURL)
		// The Flower URL is specific to CeleryExecutor only
		if executor == houston.CeleryExecutorType || executor == "" {
			tab.SuccessMsg += fmt.Sprintf("\n Flower Dashboard: %s", flowerURL)
		}
		tab.Print(w) //nolint:errcheck // best-effort render to the terminal, as it always was
		return nil
	})
}

// emitUpdated publishes the Deployment update left.
func emitUpdated(r cliout.Renderer, d *houston.Deployment) error {
	return r.Emit(newDeploymentJSON(d), func(w io.Writer) error {
		tab := deploymentTable()
		tab.AddRow([]string{d.Label, d.ReleaseName, d.Version, d.ID, tagOrUnknown(d), image(d, false).Label()}, false)
		tab.SuccessMsg = "\n Successfully updated deployment"
		tab.Print(w) //nolint:errcheck // best-effort render to the terminal, as it always was
		return nil
	})
}

// emitAdopted publishes the Deployment adopt created.
func emitAdopted(r cliout.Renderer, d *houston.Deployment) error {
	return r.Emit(newDeploymentJSON(d), func(w io.Writer) error {
		tab := &printutil.Table{
			Padding:        []int{30, 30, 30, 40, 40},
			DynamicPadding: true,
			Header:         []string{"NAME", "DEPLOYMENT NAME", "NAMESPACE", "CLUSTER ID", "DEPLOYMENT ID"},
		}
		tab.AddRow([]string{d.Label, d.ReleaseName, d.Namespace, d.ClusterID, d.ID}, false)
		tab.SuccessMsg = "\n Successfully adopted deployment"
		tab.Print(w) //nolint:errcheck // best-effort render to the terminal, as it always was
		return nil
	})
}

// emitDeploymentList publishes the Deployments list found, in its order.
func emitDeploymentList(r cliout.Renderer, ds []houston.Deployment) error {
	list := deploymentListJSON{Deployments: make([]deploymentJSON, 0, len(ds))}
	for i := range ds {
		list.Deployments = append(list.Deployments, newDeploymentJSON(&ds[i]))
	}
	return r.Emit(list, func(w io.Writer) error {
		tab := deploymentTable()
		for i := range ds {
			d := &ds[i]
			tab.AddRow([]string{d.Label, d.ReleaseName, "v" + d.Version, d.ID, tagOrUnknown(d), image(d, true).Label()}, false)
		}
		return tab.Print(w)
	})
}

// versionChangeKind is which command made a VersionChange, which decides its
// words.
type versionChangeKind int

const (
	airflowUpgrade versionChangeKind = iota
	runtimeUpgrade
	runtimeMigrate
)

// emitVersionChange publishes what an upgrade or migration, or its cancel,
// did.
func emitVersionChange(r cliout.Renderer, kind versionChangeKind, c *deployment.VersionChange) error {
	return r.Emit(newVersionChangeJSON(c), func(w io.Writer) error {
		d := &c.Deployment
		if c.Action == deployment.VersionChangeStarted {
			tab := versionTable()
			desired := c.Desired.Version
			switch kind {
			case airflowUpgrade:
				tab.AddRow([]string{d.Label, d.ReleaseName, "v" + d.Version, d.ID, c.Desired.Label()}, false)
				tab.SuccessMsg = fmt.Sprintf("\nThe upgrade from Airflow %s to %s has been started. ", c.Current.Version, desired) +
					fmt.Sprintf("To complete this process, add an Airflow %s image to your Dockerfile and deploy to APC.\n", desired) +
					"To cancel, run: \n $ astro deployment airflow upgrade --cancel\n"
			case runtimeUpgrade:
				shown := deployment.ImageVersion{Image: deployment.ImageRuntime, Version: d.DesiredRuntimeVersion}
				tab.AddRow([]string{d.Label, d.ReleaseName, "v" + d.Version, d.ID, shown.Label()}, false)
				tab.SuccessMsg = fmt.Sprintf("\nThe upgrade from Runtime %s to %s has been started. ", c.Current.Version, desired) +
					fmt.Sprintf("To complete this process, add an Runtime %s image to your Dockerfile and deploy to APC.\n", desired) +
					"To cancel, run: \n $ astro deployment runtime upgrade --cancel\n"
			case runtimeMigrate:
				tab.AddRow([]string{d.Label, d.ReleaseName, "v" + d.Version, d.ID, c.Desired.Label()}, false)
				tab.SuccessMsg = fmt.Sprintf("\nThe migration from Airflow %s image to Runtime %s has been started. ", c.Current.Version, desired) +
					fmt.Sprintf("To complete this process, add an Runtime %s image to your Dockerfile and deploy to APC.\n", desired) +
					"To cancel, run: \n $ astro deployment runtime migrate --cancel\n"
			}
			tab.Print(w) //nolint:errcheck // best-effort render to the terminal, as it always was
			return nil
		}
		return cliout.WriteText(w, func(b *bufio.Writer) {
			canceled := c.Action == deployment.VersionChangeCanceled
			// running names the image the Deployment runs, as the cancel
			// found it: "Airflow 2.0.0" on Astronomer Certified, "Runtime
			// 4.2.0" on Runtime.
			running := "Airflow " + c.Current.Version
			if c.Current.Image == deployment.ImageRuntime {
				running = "Runtime " + c.Current.Version
			}
			switch {
			case kind == airflowUpgrade && canceled:
				fmt.Fprintf(b, "\nAirflow upgrade process has been successfully canceled. Your Deployment was not interrupted and you are still running %s.\n", running)
			case kind == airflowUpgrade:
				fmt.Fprintf(b, "\nNothing to cancel. You are currently running %s and you have not indicated that you want to upgrade.", running)
			case kind == runtimeUpgrade && canceled:
				fmt.Fprintf(b, "\nRuntime upgrade process has been successfully canceled. Your Deployment was not interrupted and you are still running %s.\n", running)
			case kind == runtimeUpgrade:
				fmt.Fprintf(b, "\nNothing to cancel. You are currently running %s and you have not indicated that you want to upgrade.", running)
			case canceled:
				fmt.Fprintf(b, "\nRuntime migrate process has been successfully canceled. Your Deployment was not interrupted and you are still running %s.\n", running)
			case c.Current.Image == deployment.ImageRuntime:
				fmt.Fprintf(b, "\nNothing to cancel. You are already running %s and you have either not indicated that you want to migrate or migration has been completed.", running)
			default:
				fmt.Fprintf(b, "\nNothing to cancel. You are running %s and you have not indicated that you want to migrate to Runtime.", running)
			}
		})
	})
}

// emitLogEntry publishes one log record. follow says it came from a
// subscription, whose records have always printed without a line break of
// their own.
func emitLogEntry(r cliout.Renderer, component string, l houston.DeploymentLog, follow bool) error {
	return r.EmitEvent(logEntryJSON{Component: component, Timestamp: l.CreatedAt, Message: l.Log}, func(w io.Writer) error {
		if follow {
			_, err := fmt.Fprint(w, l.Log)
			return err
		}
		_, err := fmt.Fprintln(w, l.Log)
		return err
	})
}
