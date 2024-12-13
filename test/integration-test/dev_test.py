import os
import time
import subprocess
import shutil
import tempfile
import docker
import pytest
from datetime import datetime

ASTRO = os.path.abspath("../../astro")
AIRFLOW_CONTAINERS = ["postgres", "webserver", "scheduler", "triggerer"]


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
    return docker.from_env()


def get_container_status(keywords, client):
    running_containers = client.containers.list()
    client.containers

    container_status = {}
    for container in running_containers:
        for keyword in keywords:
            if keyword in container.name:
                container_status[container.name] = container.status

    for keyword in keywords:
        if not any(keyword in name for name in container_status.keys()):
            raise AssertionError(f"Container with keyword '{keyword}' not found.")

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
        [ASTRO, "dev", "start"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )

    assert result.returncode == 0
    time.sleep(5)

    # Validate airflow containers are up and running
    container_status = get_container_status(AIRFLOW_CONTAINERS, docker_client)
    for name, status in container_status.items():
        assert status == "running", f"Container '{name}' is not running as expected."


def test_dev_ps():
    # Run `astro dev ps` command
    result = subprocess.run(
        [ASTRO, "dev", "ps"], stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True
    )

    assert result.returncode == 0

    # Validate airflow conrtainers are listed in output
    output = result.stdout
    for keyword in AIRFLOW_CONTAINERS:
        assert any(
            keyword in line for line in output.splitlines()
        ), f"Container '{keyword}' not listed in output."


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
    # Validate dag has been parsed successfully
    assert "no errors detected in your DAGs" in output


def test_dev_run():
    # Run `astro dev run dags list` command
    result = subprocess.run(
        [ASTRO, "dev", "run", "dags", "list"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        check=True,
    )

    output = result.stdout
    # Validate deployed example dag has been listed in output
    assert "example_astronauts" in output


def test_dev_restart(docker_client):
    # Get initial container start times
    pre_restart_times = {}
    for keyword in AIRFLOW_CONTAINERS:
        matching_containers = [
            container.name
            for container in docker_client.containers.list()
            if keyword in container.name
        ]
        assert matching_containers, f"No containers found with keyword '{keyword}'."
        pre_restart_times[keyword] = [
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
    for keyword in AIRFLOW_CONTAINERS:
        matching_containers = [
            container.name
            for container in docker_client.containers.list()
            if keyword in container.name
        ]
        assert matching_containers, f"No containers found with keyword '{keyword}'."
        post_restart_times[keyword] = [
            get_container_start_time(name, docker_client)
            for name in matching_containers
        ]

    # Compare pre-restart and post-restart container start times
    for keyword in AIRFLOW_CONTAINERS:
        for pre_time, post_time in zip(
            pre_restart_times[keyword], post_restart_times[keyword]
        ):
            assert (
                post_time > pre_time
            ), f"Container with keyword '{keyword}' was not restarted."


def test_dev_kill(docker_client):
    # Run `astro dev stop` command
    result = subprocess.run(
        [ASTRO, "dev", "kill"],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
    )

    assert result.returncode == 0
    time.sleep(5)

    # Validate airflow containers are stopped
    running_containers = docker_client.containers.list()
    assert len(running_containers) == 0