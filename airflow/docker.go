package airflow

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"os"
	"regexp"
	"strconv"
	"strings"
	"text/tabwriter"

	dockercompose "github.com/docker/libcompose/docker"
	"github.com/docker/libcompose/docker/ctx"
	"github.com/docker/libcompose/project"
	"github.com/docker/libcompose/project/options"
	"github.com/pkg/errors"

	"github.com/astronomerio/astro-cli/airflow/include"
	"github.com/astronomerio/astro-cli/config"
	"github.com/astronomerio/astro-cli/docker"
	"github.com/astronomerio/astro-cli/houston"
	"github.com/astronomerio/astro-cli/pkg/input"
)

const (
	componentName     = "airflow"
	deployTagPrefix   = "cli-"
	dockerStateUp     = "Up"
	dockerStateExited = "Exited"
)

// ComposeConfig is input data to docker compose yaml template
type ComposeConfig struct {
	PostgresUser         string
	PostgresPassword     string
	PostgresHost         string
	PostgresPort         string
	AirflowImage         string
	AirflowHome          string
	AirflowUser          string
	AirflowWebserverPort string
}

// repositoryName creates an airflow repository name
func repositoryName(name string) string {
	return fmt.Sprintf("%s/%s", name, componentName)
}

// imageName creates an airflow image name
func imageName(name, tag string) string {
	return fmt.Sprintf("%s:%s", repositoryName(name), tag)
}

// imageBuild builds the airflow project
func imageBuild(path, imageName string) {
	// Change to location of Dockerfile
	os.Chdir(path)

	// Build image
	docker.Exec("build", "-t", imageName, ".")
}

// generateConfig generates the docker-compose config
func generateConfig(projectName, airflowHome string) string {
	tmpl, err := template.New("yml").Parse(include.Composeyml)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	config := ComposeConfig{
		PostgresUser:         config.CFG.PostgresUser.GetString(),
		PostgresPassword:     config.CFG.PostgresPassword.GetString(),
		PostgresHost:         config.CFG.PostgresHost.GetString(),
		PostgresPort:         config.CFG.PostgresPort.GetString(),
		AirflowImage:         imageName(projectName, "latest"),
		AirflowHome:          airflowHome,
		AirflowUser:          "astro",
		AirflowWebserverPort: "8080",
	}

	buff := new(bytes.Buffer)
	err = tmpl.Execute(buff, config)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	return buff.String()
}

func checkServiceState(serviceState, expectedState string) bool {
	scrubbedState := strings.Split(serviceState, " ")[0]

	if scrubbedState == expectedState {
		return true
	}

	return false
}

// createProject creates project with yaml config as context
func createProject(projectName, airflowHome string) (project.APIProject, error) {
	// Generate the docker-compose yaml
	yaml := generateConfig(projectName, airflowHome)

	// Create the project
	project, err := dockercompose.NewProject(&ctx.Context{
		Context: project.Context{
			ComposeBytes: [][]byte{[]byte(yaml)},
			ProjectName:  projectName,
		},
	}, nil)

	return project, err
}

// Start starts a local airflow development cluster
func Start(airflowHome string) error {
	// Get project name from config
	projectName := config.CFG.ProjectName.GetString()

	// Create a libcompose project
	project, err := createProject(projectName, airflowHome)
	if err != nil {
		return errors.Wrap(err, "Error creating docker-compose project")
	}

	// Fetch project containers
	psInfo, err := project.Ps(context.Background())
	if err != nil {
		return errors.Wrap(err, "Error checking docker-compose status")
	}

	if len(psInfo) > 0 {
		// Ensure project is not already running
		for _, info := range psInfo {
			if checkServiceState(info["State"], dockerStateUp) {
				return errors.New("Project is already running, cannot start")
			}
		}

		err = project.Start(context.Background())
		if err != nil {
			return errors.Wrap(err, "Error building, (re)creating or starting project containers")
		}
	} else {
		// Build this project image
		imageBuild(airflowHome, imageName(projectName, "latest"))

		// Start up our project
		err = project.Up(context.Background(), options.Up{})
		if err != nil {
			return errors.Wrap(err, "Error building, (re)creating or starting project containers")
		}
	}
	fmt.Println("Airflow Webserver: http://localhost:8080/admin/")
	fmt.Println("Postgres Database: localhost:5432/postgres")
	return nil
}

// Kill stops a local airflow development cluster
func Kill(airflowHome string) error {
	// Get project name from config
	projectName := config.CFG.ProjectName.GetString()

	// Create a libcompose project
	project, err := createProject(projectName, airflowHome)
	if err != nil {
		return errors.Wrap(err, "Error creating docker-compose project")
	}

	// Shut down our project
	err = project.Down(context.Background(), options.Down{RemoveVolume: true, RemoveOrphans: true})
	if err != nil {
		return errors.Wrap(err, "Error stopping and removing containers")
	}

	return nil
}

// Stop a running docker project
func Stop(airflowHome string) error {
	// Get project name from config
	projectName := config.CFG.ProjectName.GetString()

	// Create a libcompose project
	project, err := createProject(projectName, airflowHome)
	if err != nil {
		return errors.Wrap(err, "Error creating docker-compose project")
	}

	// Pause our project
	err = project.Stop(context.Background(), 30)
	if err != nil {
		return errors.Wrap(err, "Error pausing project containers")
	}

	return nil
}

// PS prints the running airflow containers
func PS(airflowHome string) error {
	// Get project name from config
	projectName := config.CFG.ProjectName.GetString()

	// Create a libcompose project
	project, err := createProject(projectName, airflowHome)
	if err != nil {
		return errors.Wrap(err, "Error creating docker-compose project")
	}

	// List project containers
	psInfo, err := project.Ps(context.Background())
	if err != nil {
		return errors.Wrap(err, "Error checking docker-compose status")
	}

	// Columns for table
	infoColumns := []string{"Name", "State", "Ports"}

	// Create a new tabwriter
	tw := new(tabwriter.Writer)
	tw.Init(os.Stdout, 0, 8, 2, '\t', tabwriter.AlignRight)

	// Append data to table
	fmt.Fprintln(tw, strings.Join(infoColumns, "\t"))
	for _, info := range psInfo {
		data := []string{}
		for _, lbl := range infoColumns {
			data = append(data, info[lbl])
		}
		fmt.Fprintln(tw, strings.Join(data, "\t"))
	}

	// Flush to stdout
	return tw.Flush()
}

// Deploy pushes a new docker image
// TODO: Check for uncommitted git changes
func Deploy(path, name string) error {
	if name == "" {
		deployments, err := api.FetchDeployments()
		if err != nil {
			return err
		}

		if len(deployments) == 0 {
			return errors.New("No airflow deployments found")
		}

		deployMap := map[string]houston.Deployment{}
		fmt.Println("Select which airflow deployment you want to deploy to:")
		for i, deployment := range deployments {
			index := i + 1
			deployMap[strconv.Itoa(index)] = deployment
			fmt.Printf("%d) %s (%s)\n", index, deployment.Title, deployment.ReleaseName)
		}

		choice := input.InputText("")
		selected, ok := deployMap[choice]
		if !ok {
			return errors.New("Invalid deployment selection")
		}
		name = selected.ReleaseName
	}
	fmt.Printf("Deploying: %s\n", name)
	fmt.Println(repositoryName(name))

	// Get a list of tags in the repository for this image
	tags, err := docker.ListRepositoryTags(repositoryName(name))
	if err != nil {
		return err
	}

	// Keep track of the highest tag we have listed in the registry
	// Default to 0, if never deployed before
	highestTag := 0

	// Loop through tags in the registry and find the highest one
	re := regexp.MustCompile("^" + deployTagPrefix + "(\\d+)$")
	for _, tag := range tags {
		matches := re.FindStringSubmatch(tag)
		if len(matches) > 0 {
			t, _ := strconv.Atoi(matches[1])
			if t > highestTag {
				highestTag = t
			}
		}
	}

	// Build the image to deploy
	// We use latest and keep this tag around after deployments to keep subsequent deploys quick
	deployImage := imageName(name, "latest")

	// Build our image
	fmt.Println("Building image")
	imageBuild(path, deployImage)

	// Tag our build with remote registry and incremented tag
	tag := fmt.Sprintf("%s%d", deployTagPrefix, highestTag+1)
	remoteImage := fmt.Sprintf("%s/%s",
		config.CFG.RegistryAuthority.GetString(), imageName(name, tag))
	docker.Exec("tag", deployImage, remoteImage)

	// Push image to registry
	fmt.Println("Pushing image to Astronomer registry")
	docker.Exec("push", remoteImage)

	// Delete the image tags we just generated
	docker.Exec("rmi", remoteImage)

	return nil
}
