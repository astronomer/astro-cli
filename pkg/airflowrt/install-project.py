# Written by the Astro CLI. Do not edit: `astro init` replaces this file.
#
# Runs inside the image build, on the runtime image's own Python. Reads the
# project's pyproject.toml, and its uv.lock when there is one, and writes what
# the runtime's install steps read: requirements.txt, packages.txt and
# uv-lock-constraints.txt, the lock's versions. A lock it cannot use (out of
# date, unreadable) is reported and the dependencies install unpinned; nothing
# here fails the build over the lock.
#
#   python .astro/install-project.py [--project DIR] [--out DIR]

import argparse
import os
import re
import sys
from urllib.parse import urlsplit, urlunsplit

try:
    import tomllib
except ImportError:  # Python before 3.11
    try:
        import tomli as tomllib
    except ImportError:
        from pip._vendor import tomli as tomllib

try:
    from packaging.markers import Marker
    from packaging.requirements import Requirement
    from packaging.specifiers import SpecifierSet
    from packaging.tags import sys_tags
except ImportError:  # every runtime image's pip vendors it
    from pip._vendor.packaging.markers import Marker
    from pip._vendor.packaging.requirements import Requirement
    from pip._vendor.packaging.specifiers import SpecifierSet
    from pip._vendor.packaging.tags import sys_tags

REQUIREMENTS = "requirements.txt"
PACKAGES = "packages.txt"
LOCK_CONSTRAINTS = "uv-lock-constraints.txt"
RUNTIME_CONSTRAINTS = "/etc/pip-constraints.txt"
# Extra index pages, one per line, for a CLI build that names no project (so
# has no [tool.uv.sources] to derive them from). A project's build never has it.
FIND_LINKS = "install-find-links.txt"

# The distributions that state the Airflow version: the image is Airflow.
AIRFLOW = "apache-airflow"
AIRFLOW_CORE = "apache-airflow-core"
TASK_SDK = "apache-airflow-task-sdk"
AIRFLOW_DISTS = (AIRFLOW, AIRFLOW_CORE, TASK_SDK)
GIT_PACKAGE = "git"


def warn(msg):
    print("warning: " + msg, file=sys.stderr)


def norm(name):
    return re.sub(r"[-_.]+", "-", name.lower())


def strings(v):
    return [s for s in v if isinstance(s, str)] if isinstance(v, list) else []


# --- requirements, as the CLI's pkg/manifest states them -----------------------
# Plain string handling on purpose: the output must match manifest.Requirements
# byte for byte (a test holds the two together).


def dist_name(req):
    s = req.strip()
    m = re.search(r"[\[ \t<>=!~;@(]", s)
    return norm(s[: m.start()] if m else s)


def states_version(spec):
    head = spec.split(";", 1)[0]
    if "]" in head:
        head = head[head.index("]") + 1 :]
    return any(c in head for c in "<>=!~@")


def without_airflow(deps):
    """Drop the Airflow requirements and a bare Task SDK (manifest.WithoutAirflow)."""
    out = []
    for d in deps:
        name = dist_name(d)
        if name in (AIRFLOW, AIRFLOW_CORE) or (name == TASK_SDK and not states_version(d)):
            continue
        out.append(d)
    return out


def direct_url(source):
    """The PEP 508 URL of a [tool.uv.sources] git or URL entry that applies to every install."""
    if not isinstance(source, dict) or any(k in source for k in ("marker", "extra", "group")):
        return None
    sub = source.get("subdirectory")
    sub = "#subdirectory=" + sub if isinstance(sub, str) and sub else ""
    repo = source.get("git")
    if isinstance(repo, str) and repo:
        address = repo if repo.startswith("git+") else "git+" + repo
        ref = next((source[k] for k in ("rev", "tag", "branch") if isinstance(source.get(k), str) and source[k]), None)
        return address + ("@" + ref if ref else "") + sub
    url = source.get("url")
    return url + sub if isinstance(url, str) and url else None


def direct_reference(dep, urls):
    """dep as `name[extras] @ url ; marker` when its source is a URL."""
    address = urls.get(dist_name(dep))
    head, sep, marker = dep.partition(";")
    if address is None or "@" in head:
        return dep
    head = head.strip()
    m = re.search(r"[ \t\[<>=!~(]", head)
    name, rest = head[: m.start() if m else len(head)], head[m.start() if m else len(head) :].strip()
    if rest.startswith("[") and "]" in rest:
        name += rest[: rest.index("]") + 1]
    return name + " @ " + address + (" ; " + marker.strip() if sep else "")


# --- credentials ---------------------------------------------------------------

_saw_credentials = [False]


def without_credentials(url):
    """url without an http(s) user:password, which would land in an image layer
    and the build log (the build authenticates through the netrc secret). An
    ssh URL's user is not a credential and is kept."""
    prefix, rest = "", url
    m = re.match(r"[A-Za-z][A-Za-z0-9.-]*\+(?=[A-Za-z][A-Za-z0-9+.-]*://)", url)
    if m:
        prefix, rest = m.group(0), url[m.end() :]
    try:
        parts = urlsplit(rest)
    except ValueError:
        return url
    if parts.scheme.lower() not in ("http", "https") or "@" not in parts.netloc:
        return url
    _saw_credentials[0] = True
    return prefix + urlunsplit(parts._replace(netloc=parts.netloc.rsplit("@", 1)[1]))


# --- sources: pyproject.toml's routing vs the lock's record of it --------------

# A source the stale check cannot compare: marker/extra/group-limited, a list,
# a workspace member, or an index pyproject.toml does not define.
UNKNOWN = object()


def url_key(url):
    url = without_credentials(url)
    if url.startswith("git+"):
        url = url[len("git+") :]
    parts = urlsplit(url)
    return urlunsplit((parts.scheme.lower(), parts.netloc.lower(), parts.path.rstrip("/"), parts.query, ""))


def source_key(source, indexes):
    if source is None:
        return None
    if not isinstance(source, dict) or any(k in source for k in ("marker", "extra", "group", "workspace")):
        return UNKNOWN
    sub = source.get("subdirectory") or ""
    if isinstance(source.get("git"), str):
        ref = next(((k, source[k]) for k in ("rev", "tag", "branch") if isinstance(source.get(k), str) and source[k]), None)
        return ("git", url_key(source["git"]), ref, sub)
    if isinstance(source.get("url"), str):
        return ("url", url_key(source["url"]), sub)
    if isinstance(source.get("path"), str):
        return ("path", os.path.normpath(source["path"]))
    name = source.get("index")
    if isinstance(name, str) and name in indexes:
        return ("index", url_key(indexes[name]))
    return UNKNOWN


def lock_source_key(entry):
    """The lock's record of a dependency (its root's requires-dist), as source_key states it."""
    if isinstance(entry.get("git"), str):
        address = entry["git"].partition("#")[0]
        base, _, query = address.partition("?")
        params = dict(p.partition("=")[::2] for p in query.split("&") if p)
        ref = next(((k, params[k]) for k in ("rev", "tag", "branch") if params.get(k)), None)
        return ("git", url_key(base), ref, entry.get("subdirectory") or params.get("subdirectory", ""))
    if isinstance(entry.get("url"), str):
        address, _, fragment = entry["url"].partition("#")
        sub = entry.get("subdirectory") or (fragment.split("=", 1)[1] if fragment.startswith("subdirectory=") else "")
        return ("url", url_key(address), sub)
    for key in ("path", "directory", "editable"):
        if isinstance(entry.get(key), str):
            return ("path", os.path.normpath(entry[key]))
    if isinstance(entry.get("index"), str):
        return ("index", url_key(entry["index"]))
    return None


class Manifest:
    """What pyproject.toml asks to install."""

    def __init__(self, pyproject):
        project = pyproject.get("project") or {}
        tool = pyproject.get("tool") or {}
        uv, astro = tool.get("uv") or {}, tool.get("astro") or {}

        indexes = {}
        for index in uv.get("index") or []:
            if isinstance(index, dict) and isinstance(index.get("name"), str) and isinstance(index.get("url"), str):
                if index["name"] and index["url"]:
                    indexes[index["name"]] = index["url"]
        # index_sources: {name: (name as written, index URL)}; sources: {name: source_key}.
        self.index_sources, self.sources, urls = {}, {}, {}
        for pkg, source in (uv.get("sources") or {}).items():
            self.sources[norm(pkg)] = source_key(source, indexes)
            if not isinstance(source, dict):
                continue
            if isinstance(source.get("index"), str) and source["index"] in indexes:
                self.index_sources[norm(pkg)] = (pkg, without_credentials(indexes[source["index"]]))
            address = direct_url(source)
            if address is not None:
                urls[norm(pkg)] = without_credentials(address)

        self.stated = strings(project.get("dependencies"))
        # Everything the project names itself: the packages the lock may move
        # off the base image's versions.
        named = list(self.stated)
        for extra in (project.get("optional-dependencies") or {}).values():
            named += strings(extra)
        for group in (pyproject.get("dependency-groups") or {}).values():
            named += strings(group)
        self.named = {dist_name(d) for d in named}
        self.constraint_dependencies = strings(uv.get("constraint-dependencies"))
        self.override_dependencies = strings(uv.get("override-dependencies"))
        self.deps = without_airflow([direct_reference(d, urls) for d in self.stated] if urls else list(self.stated))
        self.packages = strings(astro.get("packages"))

    def find_links(self, skip=()):
        """The index page of each dependency the image installs from its own index."""
        installed = {dist_name(d) for d in self.deps}
        return [
            url.rstrip("/") + "/" + pkg + "/"
            for name, (pkg, url) in sorted(self.index_sources.items())
            if name in installed and name not in skip
        ]


def with_git_for_vcs_deps(deps, packages):
    """Add git to the OS packages when a dependency is fetched with git (the image ships none)."""
    has_git = any(p.strip().split("=", 1)[0].split(":", 1)[0] == GIT_PACKAGE for p in packages)
    needs_git = any(d.partition("@")[1] and d.partition("@")[2].strip().lower().startswith("git+") for d in deps)
    return list(packages) + ([GIT_PACKAGE] if needs_git and not has_git else [])


# --- markers and requirements --------------------------------------------------


def applies(markers):
    """Whether a uv.lock entry is in this interpreter's fork. One it cannot
    evaluate counts as applying, so a forked package goes unpinned, not mispinned."""
    if not markers:
        return True
    for m in markers:
        try:
            if Marker(m).evaluate():
                return True
        except Exception:
            return True
    return False


# A single comparison: `var op 'value'` (either side may be the string).
_COMPARISON = re.compile(r"""\s*([A-Za-z_][\w.]*|'[^']*'|"[^"]*")\s*(===|==|!=|<=|>=|<|>|~=|not\s+in|in)\s*([A-Za-z_][\w.]*|'[^']*'|"[^"]*")\s*""")

# A marker canonical_marker cannot spell reliably: it matches any other.
ANY_MARKER = object()


def canonical_marker(marker):
    """One spelling of a single-comparison marker, so the one pyproject.toml
    states and the one uv records compare equal (python_version == 'X.Y' is
    uv's python_full_version == 'X.Y.*'). Compound or unreadable: ANY_MARKER,
    since uv may regroup it and that must not make a lock stale."""
    if not marker or not marker.strip():
        return ""
    m = _COMPARISON.fullmatch(marker)
    if not m:
        return ANY_MARKER
    lhs, op, rhs = (re.sub(r"\s+", " ", g) for g in m.groups())
    lhs, rhs = ("'%s'" % s[1:-1] if s[0] in "'\"" else s for s in (lhs, rhs))
    if lhs == "python_version" and op in ("==", "!=") and re.fullmatch(r"'\d+\.\d+'", rhs):
        lhs, rhs = "python_full_version", "'%s.*'" % rhs[1:-1]
    return " ".join((lhs, op, rhs))


def parse_requirement(text):
    """(name, specifier, extras, url) of a PEP 508 requirement."""
    r = Requirement(text)
    return norm(r.name), str(r.specifier), sorted(norm(e) for e in r.extras), r.url


def requirement_key(name, spec):
    try:
        spec = ",".join(sorted(str(s) for s in SpecifierSet(spec)))
    except Exception:
        pass
    return (norm(name), spec.replace(" ", ""))


def same_requirements(stated, recorded):
    """Whether constraint- or override-dependencies as written match the lock's
    [manifest] record: the same names and specifiers, each as many times, with
    markers that mean the same."""

    def grouped(entries):
        groups = {}
        for name, spec, marker in entries:
            groups.setdefault(requirement_key(name, spec), []).append(canonical_marker(marker))
        return groups

    mine = []
    for text in stated:
        name, spec, _, url = parse_requirement(text)
        mine.append((name, url or spec, text.split(";", 1)[1].strip() if ";" in text else ""))
    theirs = [(r.get("name", ""), r.get("url") or r.get("specifier", ""), r.get("marker", "")) for r in recorded or []]
    mine, theirs = grouped(mine), grouped(theirs)
    if mine.keys() != theirs.keys():
        return False
    for key, markers in mine.items():
        others = theirs[key]
        if len(markers) != len(others):
            return False
        if ANY_MARKER not in markers and ANY_MARKER not in others and sorted(markers) != sorted(others):
            return False
    return True


def stale_reason(lock, root, manifest, locked_versions):
    """Why uv.lock does not resolve pyproject.toml as it now stands, or None."""
    if root is None:
        return "it has no record of the project"
    recorded_manifest = lock.get("manifest") or {}
    if not same_requirements(manifest.constraint_dependencies, recorded_manifest.get("constraints")):
        return "it was resolved under other [tool.uv] constraint-dependencies"
    if not same_requirements(manifest.override_dependencies, recorded_manifest.get("overrides")):
        return "it was resolved under other [tool.uv] override-dependencies"
    recorded = {}
    for r in (root.get("metadata") or {}).get("requires-dist") or []:
        recorded.setdefault(norm(r.get("name", "")), []).append(r)
    for dep in manifest.stated:
        name, spec, extras, url = parse_requirement(dep)
        entries = recorded.get(name)
        if not entries:
            return "it does not resolve %s" % name
        if url:
            continue
        source = manifest.sources.get(name)
        if source is not UNKNOWN:
            entries = [e for e in entries if lock_source_key(e) == source]
            if not entries:
                return "it resolves %s from another source than pyproject.toml's" % name
        if not any(sorted(norm(x) for x in e.get("extras") or []) == extras for e in entries):
            return "it resolves %s with other extras than %r" % (name, dep)
        # uv records no specifier for a git, URL or path source.
        if isinstance(source, tuple) and source[0] in ("git", "url", "path"):
            continue
        if not any(SpecifierSet(spec) == SpecifierSet(e.get("specifier", "")) for e in entries):
            return "it resolves %s for another requirement than %r" % (name, dep)
        versions = locked_versions.get(name) or set()
        if spec and len(versions) == 1:
            (version,) = versions
            if version is not None and not SpecifierSet(spec).contains(version, prereleases=True):
                return "it holds %s at %s, which %r excludes" % (name, version, dep)
    return None


# --- the runtime and the lock --------------------------------------------------


def runtime_constrained(path, seen=None):
    """The distributions the runtime's own constraints file pins, following -c/-r."""
    seen = seen if seen is not None else set()
    path = os.path.abspath(path)
    if path in seen:
        return set()
    seen.add(path)
    try:
        with open(path) as f:
            lines = f.read().splitlines()
    except OSError:
        return set()
    out = set()
    for line in lines:
        line = line.split("#", 1)[0].strip()
        m = re.match(r"(?:-c|--constraint|-r|--requirement)(?:\s+|=)(\S+)$", line)
        if m:
            out |= runtime_constrained(os.path.join(os.path.dirname(path), m.group(1)), seen)
        elif line and not line.startswith("-"):
            out.add(dist_name(line))
    return out


def lock_root(lock):
    for p in lock.get("package") or []:
        source = p.get("source") or {}
        if source.get("virtual") == "." or source.get("editable") == ".":
            return p
    return None


def file_tags(filename):
    parts = filename[: -len(".whl")].split("-")
    if len(parts) < 5:
        return set()
    return {(i, a, p) for i in parts[-3].split(".") for a in parts[-2].split(".") for p in parts[-1].split(".")}


def with_hash(entry):
    url, digest = entry.get("url"), entry.get("hash")
    if not url:
        return None
    url = without_credentials(url)
    if isinstance(digest, str) and ":" in digest:
        url += "#%s=%s" % tuple(digest.split(":", 1))
    return url


def pick_file(entry):
    """The URL and hash of the locked file this interpreter installs: its best
    wheel by the interpreter's tag order, else its sdist."""
    rank = {}
    for n, t in enumerate(sys_tags()):
        rank.setdefault((t.interpreter, t.abi, t.platform), n)
    best = None
    for w in entry.get("wheels") or []:
        if not isinstance(w, dict) or not w.get("url"):
            continue
        filename = w["url"].split("#", 1)[0].split("?", 1)[0].rsplit("/", 1)[-1]
        fits = [rank[t] for t in file_tags(filename) if t in rank]
        if fits and (best is None or min(fits) < best[0]):
            best = (min(fits), w)
    if best:
        return with_hash(best[1])
    return with_hash(entry["sdist"]) if isinstance(entry.get("sdist"), dict) else None


# The install reads PyPI for everything not routed elsewhere.
PYPI = url_key("https://pypi.org/simple")


class Lock:
    """What uv.lock holds for this interpreter: pins for the constraints file,
    and the exact file of each dependency read from an index of its own.
    runtime: what the runtime's constraints pin; base: what the base image has."""

    def __init__(self, lock, manifest, runtime, base):
        root = lock_root(lock)
        entries = {}
        for p in lock.get("package") or []:
            if p is not root and applies(p.get("resolution-markers")):
                entries.setdefault(norm(p.get("name", "")), []).append(p)
        versions = {n: {e.get("version") if "registry" in (e.get("source") or {}) else None for e in es} for n, es in entries.items()}
        registries = {n: {url_key((e.get("source") or {}).get("registry") or "") for e in es} for n, es in entries.items()}

        self.stale = stale_reason(lock, root, manifest, versions)
        self.files, self.pins, self.forked, self.elsewhere = {}, [], [], []
        if self.stale:
            return
        direct = {dist_name(d) for d in manifest.deps}
        # A dependency served by an index of its own installs from the exact
        # file the lock resolved, so PyPI is never asked for that name.
        for name in manifest.index_sources:
            es = entries.get(name) or []
            if name in direct and len(es) == 1 and "registry" in (es[0].get("source") or {}):
                url = pick_file(es[0])
                if url:
                    self.files[name] = url
        for name in sorted(versions):
            vs = versions[name]
            if name in AIRFLOW_DISTS or name in runtime or name in self.files or None in vs:
                continue
            # The base image's packages stay at the runtime's tested versions
            # unless the project names them itself.
            if name in base and name not in manifest.named:
                continue
            # A version from a mirror or private index may not exist on PyPI.
            if registries[name] != {PYPI}:
                self.elsewhere.append(name)
            elif len(vs) != 1:
                self.forked.append(name)
            else:
                self.pins.append("%s==%s" % (name, next(iter(vs))))

    def constraints(self):
        lines = ["# Generated from uv.lock by .astro/install-project.py."]
        if self.forked:
            lines.append("# Not pinned, uv.lock holds more than one version: " + ", ".join(self.forked))
        if self.elsewhere:
            lines.append("# Not pinned, uv.lock took them from an index other than PyPI: " + ", ".join(self.elsewhere))
        return lines + self.pins


def base_installed():
    """The distributions installed in this (the runtime's -base) image. The
    CLI's tests name them in ASTRO_INSTALL_PROJECT_BASE_INSTALLED instead."""
    if "ASTRO_INSTALL_PROJECT_BASE_INSTALLED" in os.environ:
        return {norm(n) for n in os.environ["ASTRO_INSTALL_PROJECT_BASE_INSTALLED"].split(",") if n.strip()}
    from importlib import metadata

    return {norm(d.metadata["Name"]) for d in metadata.distributions() if d.metadata["Name"]}


def write(path, lines):
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(lines) + "\n" if lines else "")


def read_find_links(project):
    try:
        with open(os.path.join(project, FIND_LINKS)) as f:
            return [without_credentials(line.strip()) for line in f if line.strip() and not line.startswith("#")]
    except OSError:
        return []


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--project", default=".")
    ap.add_argument("--out", default=".")
    ap.add_argument("--runtime-constraints", default=RUNTIME_CONSTRAINTS)
    args = ap.parse_args(argv)

    with open(os.path.join(args.project, "pyproject.toml"), "rb") as f:
        manifest = Manifest(tomllib.load(f))

    lock = None
    lock_path = os.path.join(args.project, "uv.lock")
    if os.path.exists(lock_path):
        try:
            with open(lock_path, "rb") as f:
                lock = Lock(tomllib.load(f), manifest, runtime_constrained(args.runtime_constraints), base_installed())
            if lock.elsewhere:
                warn("uv.lock took %s from an index other than PyPI, which the image does not read; installing them unpinned" % ", ".join(lock.elsewhere))
            if lock.stale:
                warn("uv.lock is out of date with pyproject.toml (%s); run `uv lock`; installing pyproject.toml's dependencies unpinned" % lock.stale)
                lock = None
        except Exception as e:  # a lock this cannot read installs as no lock
            warn("uv.lock could not be read (%s); installing pyproject.toml's dependencies unpinned" % e)
            lock = None

    files = lock.files if lock else {}
    requirements = ["--find-links " + page for page in manifest.find_links(skip=files) + read_find_links(args.project)]
    if lock:
        requirements.append("-c " + LOCK_CONSTRAINTS)
    deps = [direct_reference(d, files) for d in manifest.deps] if files else manifest.deps

    os.makedirs(args.out, exist_ok=True)
    # Written even without a lock, so the step that copies it always finds it.
    write(os.path.join(args.out, LOCK_CONSTRAINTS), lock.constraints() if lock else [])
    with open(os.path.join(args.out, REQUIREMENTS), "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(requirements + deps) + "\n")
    write(os.path.join(args.out, PACKAGES), with_git_for_vcs_deps(deps, manifest.packages))
    if _saw_credentials[0]:
        warn("a URL in pyproject.toml or uv.lock carries a user or password, which the image build leaves out: give the build a netrc secret to authenticate instead")


if __name__ == "__main__":
    main()
