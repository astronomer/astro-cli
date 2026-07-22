import json
import os
import shutil
import subprocess
import tempfile

import pytest

ASTRO = os.path.abspath("../astro")


@pytest.fixture(scope="module")
def temp_dir():
    # Create a temporary directory
    temp_dir = tempfile.mkdtemp()
    yield temp_dir

    # Remove directory after tests
    shutil.rmtree(temp_dir)


def run_astro(*args, cwd=None):
    return subprocess.run(
        [ASTRO, *args],
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        cwd=cwd,
    )


# The v1 `astro dev` tree is replaced by a removal stub in v2: every
# subcommand fails loudly (so scripts and CI break) and names its `astro
# local` / `astro init` replacement. These cases mirror the mapping the stub
# ships (see internal/scaffold devmap).
@pytest.mark.parametrize(
    "typed,replacement",
    [
        (["start"], "astro local start"),
        (["stop"], "astro local stop"),
        (["restart"], "astro local restart"),
        (["ps"], "astro local status"),
        (["logs"], "astro local logs"),
        (["run"], "astro local run"),
        (["kill"], "astro local stop --clean"),
        (["init"], "astro init"),
    ],
)
def test_dev_subcommand_removed(typed, replacement):
    result = run_astro("dev", *typed)
    assert result.returncode != 0, f"`astro dev {' '.join(typed)}` must fail"
    assert f"Use `{replacement}` instead" in result.stderr
    assert "removed in Astro CLI v2" in result.stderr


def test_dev_bare_removed():
    result = run_astro("dev")
    assert result.returncode != 0
    assert "astro dev was removed in Astro CLI v2" in result.stderr


def test_dev_json_output():
    result = run_astro("dev", "ps", "--output", "json")
    assert result.returncode != 0
    payload = json.loads(result.stdout)
    assert payload["typed_command"] == "astro dev ps"
    assert payload["replacement"] == "astro local status"
    assert payload["mapping"]
    assert payload["doc"]


def test_init_scaffolds_project(temp_dir):
    # `astro init` is the v2 replacement for `astro dev init`: it scaffolds a
    # pyproject.toml-based project.
    result = run_astro("init", "--name", "demo", cwd=temp_dir)
    assert result.returncode == 0, result.stderr

    assert os.path.isfile(os.path.join(temp_dir, "pyproject.toml"))
    for dir_name in ["dags", "include", "plugins"]:
        assert os.path.isdir(os.path.join(temp_dir, dir_name))
