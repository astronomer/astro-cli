# Written by the Astro CLI. Do not edit: `astro init` replaces this file.
# astro-install-helper: 12
#
# Runs in the image build's first stage on the runtime image's Python, with uv
# copied in from the uv image. uv does the resolving: `uv lock --check` says
# whether uv.lock matches pyproject.toml, `uv lock` resolves one in the build
# when it is missing or stale, and `uv export --format pylock.toml` lists it.
# This file applies the image's policy to that list -- the runtime's packages
# stay as it ships them; packages the project lists, and those the image lacks,
# get the lock's versions -- and writes what the runtime's install steps read:
# requirements.txt, packages.txt and uv-lock-constraints.txt.
import argparse
import os
import re
import subprocess
import sys
from importlib import metadata
from urllib.parse import urlsplit

try:
    import tomllib
except ImportError:  # Python before 3.11
    from pip._vendor import tomli as tomllib
try:
    from packaging import markers, tags, utils
    from packaging.requirements import Requirement
except ImportError:
    from pip._vendor.packaging import markers, tags, utils
    from pip._vendor.packaging.requirements import Requirement

UV = os.environ.get("ASTRO_UV", "/opt/astro-uv/uv")
UV_CONFIG = "/etc/uv/uv.toml"  # the runtime image's uv settings
AIRFLOW = {"apache-airflow", "apache-airflow-core"}
TASK_SDK = "apache-airflow-task-sdk"
name_of = utils.canonicalize_name


def warn(msg):
    print("warning: " + msg, file=sys.stderr)


def uv(project, *args):
    return subprocess.run([UV, *args, "--project", project], capture_output=True, text=True)


def lock(project):
    """Leaves a uv.lock that matches pyproject.toml, or returns why uv could not."""
    if os.path.exists(os.path.join(project, "uv.lock")):
        # The image's uv settings name another default index, which makes every
        # lock read as stale, so the check runs without them.
        moved = os.path.isfile(UV_CONFIG) and not os.rename(UV_CONFIG, UV_CONFIG + ".astro")
        try:
            if uv(project, "lock", "--check").returncode == 0:
                return None
        finally:
            if moved:
                os.rename(UV_CONFIG + ".astro", UV_CONFIG)
    warn("uv.lock is missing or out of date with pyproject.toml; resolving it in the build. Run uv lock and commit uv.lock")
    r = uv(project, "lock")
    return None if r.returncode == 0 else r.stderr.strip() or "uv lock failed"


def locked(project):
    """The pylock.toml (PEP 751) entries of uv.lock that this interpreter installs."""
    r = uv(project, "export", "--frozen", "--no-dev", "--no-emit-project", "--format", "pylock.toml")
    if r.returncode:
        sys.exit("uv export failed: " + r.stderr.strip())
    env = markers.default_environment()
    return [p for p in tomllib.loads(r.stdout).get("packages", []) if markers.Marker(p.get("marker") or "python_version > '0'").evaluate(env)]


def no_credentials(url):
    parts = urlsplit(url)
    if "@" not in parts.netloc:
        return url
    warn("the image build leaves out a URL's user and password: give the build a netrc secret instead")
    return parts._replace(netloc=parts.netloc.rsplit("@", 1)[1]).geturl()


def direct(pkg, routed):
    """`name @ url` for a package uv takes from git or a URL, or from the index a
    [tool.uv.sources] entry routes it to (which the image's install does not
    read), else None."""
    if "vcs" in pkg:
        v = pkg["vcs"]
        sub = "#subdirectory=" + v["subdirectory"] if v.get("subdirectory") else ""
        return "%s @ %s+%s@%s%s" % (pkg["name"], v["type"], no_credentials(v["url"]), v["commit-id"], sub)
    if "url" in pkg.get("archive", {}):
        return "%s @ %s" % (pkg["name"], no_credentials(pkg["archive"]["url"]))
    if name_of(pkg["name"]) not in routed:
        return None
    rank = {t: i for i, t in enumerate(tags.sys_tags())}  # the wheel pip would pick, else the sdist
    wheels = [(min(rank.get(t, len(rank)) for t in utils.parse_wheel_filename(w["url"].rsplit("/", 1)[-1])[3]), w) for w in pkg.get("wheels", [])]
    f = min([w for w in wheels if w[0] < len(rank)], key=lambda w: w[0], default=(0, pkg.get("sdist")))[1]
    sha = (f or {}).get("hashes", {}).get("sha256")
    return f and "%s @ %s%s" % (pkg["name"], no_credentials(f["url"]), "#sha256=" + sha if sha else "")


def runtime_pins(path):
    """The distributions the runtime's own constraints pin, following -c and -r."""
    if not os.path.exists(path):
        return set()
    names = set()
    for line in open(path, encoding="utf-8"):
        line = line.split("#", 1)[0].strip()
        if line[:2] in ("-c", "-r"):
            names |= runtime_pins(os.path.join(os.path.dirname(path), line[2:].strip()))
        elif line and not line.startswith("-"):
            names.add(name_of(Requirement(line).name))
    return names


def base_installed():
    """What the runtime's -base image has; tests name it in ASTRO_INSTALL_PROJECT_BASE_INSTALLED."""
    names = os.environ.get("ASTRO_INSTALL_PROJECT_BASE_INSTALLED")
    if names is not None:
        return {name_of(n) for n in names.split(",") if n.strip()}
    return {name_of(d.metadata["Name"]) for d in metadata.distributions() if d.metadata["Name"]}


def base_airflow():
    """The Airflow release this image has, or None; tests name it in ASTRO_INSTALL_PROJECT_BASE_AIRFLOW."""
    if "ASTRO_INSTALL_PROJECT_BASE_AIRFLOW" in os.environ:
        return os.environ["ASTRO_INSTALL_PROJECT_BASE_AIRFLOW"].strip() or None
    if "ASTRO_INSTALL_PROJECT_BASE_INSTALLED" in os.environ:
        return None
    try:
        return metadata.version("apache-airflow")
    except metadata.PackageNotFoundError:
        return None


def recorded_runtime(text):
    """The build named by the record astro writes in [tool.uv] constraint-dependencies,
    read from that array alone (tomllib drops comments), or None."""
    table, inside = None, False
    for line in text.splitlines():
        code = line.split("#", 1)[0].strip()
        if inside:
            m = re.match(r"\s*#\s*Astro Runtime (\S+) \(Python [\d.]+\) constrains these\.", line)
            if m or code.startswith("]"):
                return m and m.group(1)
        elif code.startswith("["):
            table = code.strip("[] ")
        elif table == "tool.uv" and re.match(r"constraint-dependencies\s*=\s*\[", code):
            inside = not code.endswith("]")
    return None


def airflow_mismatch(pkgs, installed, runtime):
    """A warning when uv.lock was made for another Airflow than this image runs, or None."""
    locked = {p["version"] for p in pkgs if name_of(p["name"]) == "apache-airflow" and p.get("version")}
    release = lambda v: v.strip().split("+", 1)[0].lower()  # 3.3.2+astro.2 is release 3.3.2
    if len(locked) != 1 or not installed or release(next(iter(locked))) == release(installed):
        return None
    return (
        "uv.lock was made for Airflow %s%s, and this image has Airflow %s, so the lock's versions install over another runtime than "
        "the one they were resolved for: build FROM the runtime the lock was made for, or run `astro init` to move the project to this runtime"
        % (next(iter(locked)), " (Astro Runtime %s)" % runtime if runtime else "", installed.strip())
    )


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--project", default=".")
    ap.add_argument("--out", default=".")
    ap.add_argument("--runtime-constraints", default="/etc/pip-constraints.txt")
    args = ap.parse_args()
    with open(os.path.join(args.project, "pyproject.toml"), encoding="utf-8") as f:
        text = f.read()
    pyproject = tomllib.loads(text)
    project = pyproject.get("project", {})
    deps = [Requirement(r) for r in project.get("dependencies", [])]
    own = deps + [Requirement(r) for rs in [*project.get("optional-dependencies", {}).values(), *pyproject.get("dependency-groups", {}).values()] for r in rs if isinstance(r, str)]
    own = {name_of(r.name): r for r in own}
    packages = list(pyproject.get("tool", {}).get("astro", {}).get("packages", []))
    requirements, constraints, git = [], [], False

    failed = lock(args.project)
    if failed:  # nothing to pin from: the dependencies as pyproject.toml states them
        warn("uv could not lock the project, so pyproject.toml's dependencies install unpinned and without [tool.uv.sources]:\n" + failed)
        requirements = [str(r) for r in deps if name_of(r.name) not in AIRFLOW and (name_of(r.name) != TASK_SDK or r.specifier)]
        git = any((r.url or "").lower().startswith("git+") for r in deps)
    else:
        skip, installed = runtime_pins(args.runtime_constraints) | AIRFLOW, base_installed()
        sources = pyproject.get("tool", {}).get("uv", {}).get("sources", {})
        routed = {name_of(n) for n, s in sources.items() if any("index" in e for e in (s if isinstance(s, list) else [s]))}
        pkgs = locked(args.project)
        # Every build checks it, `docker build .` included: the lock is for one
        # runtime's Airflow, and this image may be another's. Its versions install.
        mismatch = airflow_mismatch(pkgs, base_airflow(), recorded_runtime(text))
        if mismatch:
            warn(mismatch)
        for pkg in pkgs:
            name = name_of(pkg["name"])
            if name in skip or (name == TASK_SDK and not (name in own and own[name].specifier)):
                continue
            if name in installed and name not in own:
                continue  # the runtime's, at its tested version
            ref = direct(pkg, routed)
            pin = pkg["name"] + ("==" + pkg["version"] if pkg.get("version") else "")
            if name in own or ref:
                requirements.append(ref or pin)
            elif pkg.get("version"):
                constraints.append(pin)
            git = git or "vcs" in pkg
    if git and not any(p.split("=", 1)[0].strip() == "git" for p in packages):
        packages.append("git")

    os.makedirs(args.out, exist_ok=True)
    for name, lines in (("uv-lock-constraints.txt", constraints), ("requirements.txt", ["-c uv-lock-constraints.txt"] * bool(constraints) + requirements), ("packages.txt", packages)):
        with open(os.path.join(args.out, name), "w", encoding="utf-8", newline="\n") as f:
            f.write("".join(line + "\n" for line in lines))


if __name__ == "__main__":
    main()
