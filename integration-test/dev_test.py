import os
import time
import subprocess
import shutil
import tempfile
import docker
import pytest
import yaml
from datetime import datetime

ASTRO = os.path.abspath("../astro")
AIRFLOW_COMPONENT = [
    "postgres",
    "api-server",
    "scheduler",
    "triggerer",
    "dag-processor",
]
VAR_KEY = "foo"
VAR_VALUE = "bar"


@pytest.fixture(scope="module")
def temp_dir():
    # Create a temporary directory
    temp_dir = tempfile.mkdtemp()
    yield temp_dir

    # Remove directory after tests
    shutil.rmtree(temp_dir)


@pytest.fixture(scope="module")
def docker_client():
    # Initialize Docker client
    client = docker.from_env()
    yield client

    # teardown the dev containers after tests
    subprocess.run(
        [ASTRO, "dev", "kill"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )


def poll_for_condition(
    check_method, timeout_seconds=30, poll_interval_seconds=1, *args, **kwargs
):
    """
    Polls a given check_method until it returns True or a timeout is reached.

    :param check_method: A callable that returns True when the desired condition is met.
    :param timeout_seconds: Maximum time to wait for the condition.
    :param poll_interval_seconds: Time to wait between checks.
    :param args: Positional arguments to pass to the check_method.
    :param kwargs: Keyword arguments to pass to the check_method.
    :return: True if the condition was met within the timeout, False otherwise.
    """
    start_time = time.time()
    while time.time() - start_time < timeout_seconds:
        if check_method(*args, **kwargs):
            return True
        time.sleep(poll_interval_seconds)
    return False


def check_astro_containers_stopped(client, components):
    """
    Checks if all Astro-related dev containers are stopped.

    :param client: Docker client instance.
    :param components: List of expected Airflow component names.
    :return: True if no Astro dev containers are running, False otherwise.
    """
    running_containers = client.containers.list()
    astro_dev_containers = [
        c
        for c in running_containers
        if "astro-dev" in c.name or any(comp in c.name for comp in components)
    ]
    return not astro_dev_containers


def check_astro_containers_running(client, components):
    """
    Checks if all specified Astro-related dev containers are running.

    :param client: Docker client instance.
    :param components: List of expected Airflow component names.
    :return: True if all specified components have at least one running container, False otherwise.
    """
    container_statuses = get_container_status(components, client)
    if (
        not container_statuses
    ):  # In case get_container_status returns empty due to no containers found
        return False

    for component in components:
        # Check if any container for this component is running
        component_running = any(
            name
            for name, status in container_statuses.items()
            if component in name and status == "running"
        )
        if not component_running:
            # If any component is not found or not running, return False
            # Check if the component is even listed, if not, it's definitely not running
            if not any(component in name for name in container_statuses.keys()):
                print(f"Component {component} not found among running containers.")
                return False
            # If found but not running
            for name, status in container_statuses.items():
                if component in name and status != "running":
                    print(
                        f"Component {component} container {name} is {status}, not 'running'."
                    )
                    return False
    return True


def get_container_status(components, client):
    running_containers = client.containers.list()
    client.containers

    container_status = {}
    for container in running_containers:
        for component in components:
            if component in container.name:
                container_status[container.name] = container.status

    for component in components:
        if not any(component in name for name in container_status.keys()):
            raise AssertionError(
                f"No containers found for airflow component '{component}'"
            )

    return container_status


def get_container_start_time(container_name, client):
    try:
        container = client.containers.get(container_name)
        start_time = container.attrs["State"]["StartedAt"]
        if "." in start_time:
            # Truncate to microseconds if nanoseconds are present
            start_time = (
                start_time.split(".")[0] + "." + start_time.split(".")[1][:6] + "Z"
            )
        return datetime.strptime(start_time, "%Y-%m-%dT%H:%M:%S.%fZ")
    except docker.errors.NotFound:
        pytest.fail(f"Container '{container_name}' not found.")
    except Exception as e:
        pytest.fail(f"Failed to get start time for '{container_name}': {e}")


def test_dev_init(temp_dir):
    # Change to temp directory
    os.chdir(temp_dir)

    # Run `astro dev init` command
    result = subprocess.run(
        [ASTRO, "dev", "init"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )

    assert result.returncode == 0

    # Validate directories and files
    expected_dirs = ["dags", "include", "plugins"]
    for dir_name in expected_dirs:
        assert os.path.isdir(os.path.join(temp_dir, dir_name))

    expected_files = ["Dockerfile", "requirements.txt"]
    for file_name in expected_files:
        assert os.path.isfile(os.path.join(temp_dir, file_name))


def test_dev_start(docker_client):
    # Run `astro dev start` command
    result = subprocess.run(
        [ASTRO, "dev", "start", "--no-browser"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )

    assert result.returncode == 0

    # Validate airflow containers are up and running using polling
    all_running = poll_for_condition(
        check_method=check_astro_containers_running,
        client=docker_client,
        components=AIRFLOW_COMPONENT,
    )

    assert (
        all_running
    ), f"Timeout reached: Not all Airflow component containers were running within the expected time."


def test_dev_ps():
    # Run `astro dev ps` command
    result = subprocess.run(
        [ASTRO, "dev", "ps"], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True
    )

    assert result.returncode == 0

    # Validate airflow conrtainers are listed in output
    output = result.stdout
    for component in AIRFLOW_COMPONENT:
        assert any(
            component in line for line in output.splitlines()
        ), f"Container '{component}' not listed in output."


def test_dev_logs():
    # Run `astro dev logs scheduler` command
    result = subprocess.run(
        [ASTRO, "dev", "logs", "scheduler"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        check=True,
    )

    output = result.stdout
    assert result.returncode == 0
    # Validate that scheduler logs
    assert "Starting the scheduler" in output


def test_dev_parse():
    # Run `astro dev parse` command
    result = subprocess.run(
        [ASTRO, "dev", "parse"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        check=True,
    )

    output = result.stdout
    assert result.returncode == 0
    # Validate dag has been parsed successfully
    assert "No errors detected in your DAGs" in output


def test_dev_run():
    # Run `astro dev run variables set foo bar` command
    result = subprocess.run(
        [ASTRO, "dev", "run", "variables", "set", VAR_KEY, VAR_VALUE],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        check=True,
    )

    output = result.stdout
    assert result.returncode == 0
    # Validate that variable has been created
    assert f"Variable {VAR_KEY} created" in output


def test_dev_restart(docker_client):
    # Get initial container start times
    pre_restart_times = {}
    for component in AIRFLOW_COMPONENT:
        matching_containers = [
            container.name
            for container in docker_client.containers.list()
            if component in container.name
        ]
        assert (
            matching_containers
        ), f"No containers found for airflow component '{component}'."
        pre_restart_times[component] = [
            get_container_start_time(name, docker_client)
            for name in matching_containers
        ]

    # Run the `astro dev restart` command
    result = subprocess.run(
        [ASTRO, "dev", "restart"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        check=True,
    )
    assert result.returncode == 0

    # Get post-restart container start times
    post_restart_times = {}
    for component in AIRFLOW_COMPONENT:
        matching_containers = [
            container.name
            for container in docker_client.containers.list()
            if component in container.name
        ]
        assert (
            matching_containers
        ), f"No containers found for airflow component '{component}'."
        post_restart_times[component] = [
            get_container_start_time(name, docker_client)
            for name in matching_containers
        ]

    # Compare pre-restart and post-restart container start times
    for component in AIRFLOW_COMPONENT:
        for pre_time, post_time in zip(
            pre_restart_times[component], post_restart_times[component]
        ):
            assert (
                post_time > pre_time
            ), f"Container of airflow component '{component}' was not restarted."


def test_dev_export():
    # Run `astro dev object export -v` command
    result = subprocess.run(
        [ASTRO, "dev", "object", "export", "-v"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        check=True,
    )

    output = result.stdout
    assert result.returncode == 0
    # Validate variables are exported and listed in airflow_setting.yaml file
    assert "successfully exported variables" in output

    with open("airflow_settings.yaml", "r") as file:
        data = yaml.safe_load(file)
    af_data = data.get("airflow", {})
    variables = af_data.get("variables", [])
    target_variable = next(
        (item for item in variables if item.get("variable_name") == VAR_KEY), None
    )
    assert (
        target_variable.get("variable_value") == VAR_VALUE
    ), f"Expected value for `{VAR_KEY}' is '{VAR_VALUE}', but got '{target_variable.get('variable_value')}'."


def test_dev_kill(docker_client):
    # Run `astro dev kill` command
    result = subprocess.run(
        [ASTRO, "dev", "kill"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )

    assert result.returncode == 0

    # Validate airflow containers are stopped by polling
    condition_met = poll_for_condition(
        check_method=check_astro_containers_stopped,
        client=docker_client,
        components=AIRFLOW_COMPONENT,
    )

    assert (
        condition_met
    ), f"Timeout reached: Not all Airflow component containers were stopped within the expected time."
