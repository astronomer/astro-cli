# Written by the Astro CLI. Do not edit: `astro init` replaces this file.
#
# Turns the project's pyproject.toml, and its uv.lock when there is one, into
# the files the Astro Runtime image's install steps read: requirements.txt,
# packages.txt and, from the lock, a constraints file holding every package at
# the version the lock resolved. The image then installs what `uv sync`
# installs on this machine, and `docker build .` needs nothing but the
# project. It runs inside the build, on the runtime image's own Python, and
# uses nothing outside the standard library and pip's vendored copies.
#
#   python .astro/install-project.py [--project DIR] [--out DIR]

import argparse
import os
import re
import sys

try:
    import tomllib
except ImportError:  # Python before 3.11
    try:
        import tomli as tomllib
    except ImportError:
        from pip._vendor import tomli as tomllib

try:
    from packaging.markers import Marker
except ImportError:
    try:
        from pip._vendor.packaging.markers import Marker
    except ImportError:
        Marker = None

REQUIREMENTS = "requirements.txt"
PACKAGES = "packages.txt"
LOCK_CONSTRAINTS = "uv-lock-constraints.txt"
RUNTIME_CONSTRAINTS = "/etc/pip-constraints.txt"

# The distributions that state the Airflow version. The runtime image is
# Airflow, so they are never installed over it.
AIRFLOW = "apache-airflow"
AIRFLOW_CORE = "apache-airflow-core"
TASK_SDK = "apache-airflow-task-sdk"

GIT_PACKAGE = "git"


def dist_name(req):
    """The normalized distribution name a PEP 508 requirement starts with."""
    s = req.strip()
    m = re.search(r"[\[ \t<>=!~;@(]", s)
    if m:
        s = s[: m.start()]
    return re.sub(r"[-_.]+", "-", s.lower())


def states_version(spec):
    head = spec.split(";", 1)[0]
    if "]" in head:
        head = head[head.index("]") + 1 :]
    return any(c in head for c in "<>=!~@")


def without_airflow(deps):
    """Drop the requirements that state the Airflow version, and a bare Task
    SDK, as the CLI's manifest.WithoutAirflow does."""
    out = []
    for d in deps:
        name = dist_name(d)
        if name in (AIRFLOW, AIRFLOW_CORE):
            continue
        if name == TASK_SDK and not states_version(d):
            continue
        out.append(d)
    return out


def direct_url(source):
    """The PEP 508 URL a [tool.uv.sources] entry stands for, when it names a
    git repository or a URL and applies to every install."""
    if not isinstance(source, dict):
        return None
    if any(k in source for k in ("marker", "extra", "group")):
        return None
    sub = source.get("subdirectory")
    sub = sub if isinstance(sub, str) else ""
    repo = source.get("git")
    if isinstance(repo, str) and repo:
        address = repo if repo.startswith("git+") else "git+" + repo
        for key in ("rev", "tag", "branch"):
            ref = source.get(key)
            if isinstance(ref, str) and ref:
                address += "@" + ref
                break
        return address + ("#subdirectory=" + sub if sub else "")
    url = source.get("url")
    if isinstance(url, str) and url:
        return url + ("#subdirectory=" + sub if sub else "")
    return None


def direct_reference(dep, urls):
    """dep as `name[extras] @ url ; marker` when its source is a URL."""
    address = urls.get(dist_name(dep))
    if address is None:
        return dep
    head, sep, marker = dep.partition(";")
    if "@" in head:
        return dep
    head = head.strip()
    m = re.search(r"[ \t\[<>=!~(]", head)
    end = m.start() if m else len(head)
    name, rest = head[:end], head[end:].strip()
    if rest.startswith("[") and "]" in rest:
        name += rest[: rest.index("]") + 1]
    out = name + " @ " + address
    if sep:
        out += " ; " + marker.strip()
    return out


def strings(v):
    return [s for s in v if isinstance(s, str)] if isinstance(v, list) else []


def read_manifest(pyproject):
    """The dependencies (as a requirements file has to state them), the
    find-links pages and the OS packages a pyproject.toml asks for."""
    project = pyproject.get("project") or {}
    tool = pyproject.get("tool") or {}
    uv = tool.get("uv") or {}
    astro = tool.get("astro") or {}

    indexes = {}
    for index in uv.get("index") or []:
        if isinstance(index, dict):
            name, url = index.get("name"), index.get("url")
            if isinstance(name, str) and name and isinstance(url, str) and url:
                indexes[name] = url
    index_sources, urls = {}, {}
    sources = uv.get("sources") or {}
    for pkg, source in sources.items():
        if not isinstance(source, dict):
            continue
        name = source.get("index")
        if isinstance(name, str) and name in indexes:
            index_sources[pkg] = indexes[name]
        address = direct_url(source)
        if address is not None:
            urls[dist_name(pkg)] = address

    deps = strings(project.get("dependencies"))
    if urls:
        deps = [direct_reference(d, urls) for d in deps]
    deps = without_airflow(deps)
    find_links = [index_sources[p].rstrip("/") + "/" + p + "/" for p in sorted(index_sources)]
    packages = strings(astro.get("packages"))
    return deps, find_links, packages


def is_git_reference(dep):
    _, at, ref = dep.partition("@")
    return bool(at) and ref.strip().lower().startswith("git+")


def is_git_package(pkg):
    name = pkg.strip().split("=", 1)[0].split(":", 1)[0]
    return name == GIT_PACKAGE


def with_git_for_vcs_deps(deps, packages):
    """Add git to the OS packages when a dependency is fetched with git, which
    the runtime image does not ship."""
    if any(is_git_package(p) for p in packages) or not any(is_git_reference(d) for d in deps):
        return list(packages)
    return list(packages) + [GIT_PACKAGE]


def applies(markers):
    """Whether a uv.lock entry belongs to this interpreter's environment: it
    names no resolution markers (it is in every fork), or one of them holds
    here. Without packaging to evaluate them, every entry applies, and a
    package left at two versions goes unpinned."""
    if not markers or Marker is None:
        return True
    for m in markers:
        try:
            if Marker(m).evaluate():
                return True
        except Exception:
            return True
    return False


def runtime_constrained(path):
    """The distributions the runtime image's own constraints file pins."""
    try:
        with open(path) as f:
            lines = f.read().splitlines()
    except OSError:
        return set()
    out = set()
    for line in lines:
        line = line.split("#", 1)[0].strip()
        if line and not line.startswith("-"):
            out.add(dist_name(line))
    return out


def lock_pins(lock, deps, runtime):
    """The `name==version` lines that hold every package uv.lock resolved from
    an index at that version, and why the lock was not used when it was not.

    A lock forked by environment (a requires-python spanning two Pythons
    resolves pandas once for each) holds a package at more than one version,
    each entry naming the forks it belongs to; the one whose markers this
    interpreter satisfies, the image's own, is the one pinned.

    Left out: the project itself, Airflow, packages the lock resolves to
    another source (a git or URL requirement names its own), a package still
    at more than one version for this interpreter, and packages the runtime's
    constraints file pins, which the runtime decides."""
    packages = lock.get("package") or []
    root = None
    for p in packages:
        source = p.get("source") or {}
        if source.get("virtual") == "." or source.get("editable") == ".":
            root = p
            break
    if root is not None:
        locked = {dist_name(r.get("name", "")) for r in (root.get("metadata") or {}).get("requires-dist") or []}
        stated = {dist_name(d) for d in deps}
        if not stated <= locked:
            return None, "uv.lock is out of date with pyproject.toml (it does not resolve %s); run `uv lock`" % ", ".join(sorted(stated - locked))
    versions = {}
    for p in packages:
        if p is root or not applies(p.get("resolution-markers")):
            continue
        name = dist_name(p.get("name", ""))
        if "registry" not in (p.get("source") or {}):
            versions.setdefault(name, set()).add(None)
            continue
        versions.setdefault(name, set()).add(p.get("version"))
    pins, forked = [], []
    for name in sorted(versions):
        vs = versions[name]
        if name in (AIRFLOW, AIRFLOW_CORE, TASK_SDK) or name in runtime or None in vs:
            continue
        if len(vs) != 1:
            forked.append(name)
            continue
        pins.append("%s==%s" % (name, next(iter(vs))))
    lines = ["# Generated from uv.lock by .astro/install-project.py."]
    if forked:
        lines.append("# Not pinned, uv.lock holds more than one version: " + ", ".join(forked))
    return lines + pins, None


def write(path, lines):
    with open(path, "w") as f:
        f.write("\n".join(lines) + "\n" if lines else "")


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--project", default=".")
    ap.add_argument("--out", default=".")
    ap.add_argument("--runtime-constraints", default=RUNTIME_CONSTRAINTS)
    args = ap.parse_args(argv)

    with open(os.path.join(args.project, "pyproject.toml"), "rb") as f:
        deps, find_links, packages = read_manifest(tomllib.load(f))

    os.makedirs(args.out, exist_ok=True)
    requirements = ["--find-links " + page for page in find_links]
    # Written either way, empty without a lock, so the step that copies it
    # into the install always finds it.
    pins = []
    lock_path = os.path.join(args.project, "uv.lock")
    if os.path.exists(lock_path):
        with open(lock_path, "rb") as f:
            pins, stale = lock_pins(tomllib.load(f), deps, runtime_constrained(args.runtime_constraints))
        if stale:
            print("warning: " + stale + "; installing pyproject.toml's dependencies unpinned", file=sys.stderr)
            pins = []
        else:
            requirements.append("-c " + LOCK_CONSTRAINTS)
    write(os.path.join(args.out, LOCK_CONSTRAINTS), pins)
    with open(os.path.join(args.out, REQUIREMENTS), "w") as f:
        f.write("\n".join(requirements + deps) + "\n")
    write(os.path.join(args.out, PACKAGES), with_git_for_vcs_deps(deps, packages))


if __name__ == "__main__":
    main()
