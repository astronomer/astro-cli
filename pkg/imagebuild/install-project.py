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
#
# Nothing here fails the build over the lock: a lock it cannot use, because it
# is out of date with pyproject.toml or cannot be read, is reported and the
# dependencies install unpinned, as they do without one.

import argparse
import os
import platform
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
    from packaging.requirements import Requirement
    from packaging.specifiers import SpecifierSet
    from packaging.tags import sys_tags
except ImportError:
    try:
        from pip._vendor.packaging.markers import Marker
        from pip._vendor.packaging.requirements import Requirement
        from pip._vendor.packaging.specifiers import SpecifierSet
        from pip._vendor.packaging.tags import sys_tags
    except ImportError:
        Marker = Requirement = SpecifierSet = sys_tags = None

# Set by the CLI's tests to exercise the paths a Python without packaging takes.
if os.environ.get("ASTRO_INSTALL_PROJECT_NO_PACKAGING"):
    Marker = Requirement = SpecifierSet = sys_tags = None

REQUIREMENTS = "requirements.txt"
PACKAGES = "packages.txt"
LOCK_CONSTRAINTS = "uv-lock-constraints.txt"
RUNTIME_CONSTRAINTS = "/etc/pip-constraints.txt"
# Pages a caller that has no pyproject.toml sources for them asks the install to
# search, one per line, passed through as --find-links as they are. The CLI
# writes it only for a build request that names no project.
FIND_LINKS = "install-find-links.txt"

# The distributions that state the Airflow version. The runtime image is
# Airflow, so they are never installed over it.
AIRFLOW = "apache-airflow"
AIRFLOW_CORE = "apache-airflow-core"
TASK_SDK = "apache-airflow-task-sdk"
AIRFLOW_DISTS = (AIRFLOW, AIRFLOW_CORE, TASK_SDK)

GIT_PACKAGE = "git"


def warn(msg):
    print("warning: " + msg, file=sys.stderr)


def norm(name):
    return re.sub(r"[-_.]+", "-", name.lower())


def dist_name(req):
    """The normalized distribution name a PEP 508 requirement starts with."""
    s = req.strip()
    m = re.search(r"[\[ \t<>=!~;@(]", s)
    if m:
        s = s[: m.start()]
    return norm(s)


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


class Manifest:
    """What pyproject.toml asks to install.

    stated: [project] dependencies as written.
    deps: the same as a requirements file has to state them: without Airflow,
      a git or URL source as a direct reference.
    index_sources: {normalized name: (name as written, index URL)} for each
      dependency a [tool.uv.sources] entry routes to a [[tool.uv.index]].
    packages: [tool.astro] packages."""

    def __init__(self, pyproject):
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
        self.index_sources, urls = {}, {}
        for pkg, source in (uv.get("sources") or {}).items():
            if not isinstance(source, dict):
                continue
            name = source.get("index")
            if isinstance(name, str) and name in indexes:
                self.index_sources[norm(pkg)] = (pkg, indexes[name])
            address = direct_url(source)
            if address is not None:
                urls[norm(pkg)] = address

        self.stated = strings(project.get("dependencies"))
        deps = [direct_reference(d, urls) for d in self.stated] if urls else list(self.stated)
        self.deps = without_airflow(deps)
        self.packages = strings(astro.get("packages"))

    def find_links(self, skip=()):
        """The index page of each dependency the install reads from an index:
        the ones it installs (Airflow comes with the image), less skip."""
        installed = {dist_name(d) for d in self.deps}
        pages = []
        for name in sorted(self.index_sources):
            pkg, url = self.index_sources[name]
            if name in installed and name not in skip:
                pages.append(url.rstrip("/") + "/" + pkg + "/")
        return pages


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


# --- markers -----------------------------------------------------------------

# The tokens of a marker, tried in order, so `and`, `or`, `in` and `not in`
# are read before a variable name could take them.
_MARKER_TOKENS = [
    ("open", re.compile(r"\(")),
    ("close", re.compile(r"\)")),
    ("bool", re.compile(r"(and|or)\b")),
    ("op", re.compile(r"(===|==|!=|<=|>=|<|>|~=|not\s+in\b|in\b)")),
    ("var", re.compile(r"[A-Za-z_][A-Za-z0-9_.]*")),
    ("str", re.compile(r"'[^']*'|\"[^\"]*\"")),
]


def _marker_tokens(text):
    tokens, pos = [], 0
    while True:
        while pos < len(text) and text[pos].isspace():
            pos += 1
        if pos == len(text):
            return tokens
        for kind, pattern in _MARKER_TOKENS:
            m = pattern.match(text, pos)
            if m:
                tokens.append((kind, re.sub(r"\s+", " ", m.group(0))))
                pos = m.end()
                break
        else:
            raise ValueError("cannot read marker %r" % text)


def _marker_env():
    py = platform.python_version()
    return {
        "python_full_version": py,
        "python_version": ".".join(py.split(".")[:2]),
        "sys_platform": sys.platform,
        "platform_machine": platform.machine(),
        "platform_system": platform.system(),
        "os_name": os.name,
        "implementation_name": sys.implementation.name,
        "platform_python_implementation": platform.python_implementation(),
    }


def _version_key(v):
    return tuple(int(x) if x.isdigit() else -1 for x in re.split(r"[.*]", v) if x != "")


def _compare(lhs, op, rhs):
    if op in ("in", "not in"):
        return (lhs in rhs) == (op == "in")
    numeric = re.fullmatch(r"[0-9.]+\*?", lhs) and re.fullmatch(r"[0-9.]+\*?", rhs)
    if numeric:
        if rhs.endswith(".*") and op in ("==", "!="):
            prefix = _version_key(rhs[:-2])
            return (_version_key(lhs)[: len(prefix)] == prefix) == (op == "==")
        lhs, rhs = _version_key(lhs), _version_key(rhs)
        width = max(len(lhs), len(rhs))
        lhs, rhs = lhs + (0,) * (width - len(lhs)), rhs + (0,) * (width - len(rhs))
    if op in ("==", "==="):
        return lhs == rhs
    if op == "!=":
        return lhs != rhs
    if not numeric:
        raise ValueError("ordering a string")
    return {"<": lhs < rhs, "<=": lhs <= rhs, ">": lhs > rhs, ">=": lhs >= rhs}[op]


def evaluate_marker(text, env=None):
    """Evaluate the marker forms uv writes as resolution markers: comparisons
    of python_full_version, python_version, sys_platform, platform_machine and
    the like with a quoted value, joined by and/or and parentheses. For a
    Python whose packaging cannot evaluate one. Raises ValueError on anything
    else."""
    env = env or _marker_env()
    tokens = _marker_tokens(text)
    i = [0]

    def peek():
        return tokens[i[0]] if i[0] < len(tokens) else (None, None)

    def take():
        t = peek()
        i[0] += 1
        return t

    def value(t):
        kind, s = t
        if kind == "var":
            if s not in env:
                raise ValueError("unknown marker variable %r" % s)
            return env[s]
        if kind == "str":
            return s[1:-1]
        raise ValueError("expected a value in %r" % text)

    def factor():
        if peek()[0] == "open":
            take()
            v = expr()
            if take()[0] != "close":
                raise ValueError("unbalanced parentheses in %r" % text)
            return v
        lhs = value(take())
        kind, op = take()
        if kind != "op":
            raise ValueError("expected an operator in %r" % text)
        return _compare(lhs, op, value(take()))

    def term():
        v = factor()
        while peek() == ("bool", "and"):
            take()
            rhs = factor()
            v = v and rhs
        return v

    def expr():
        v = term()
        while peek() == ("bool", "or"):
            take()
            rhs = term()
            v = v or rhs
        return v

    result = expr()
    if i[0] != len(tokens):
        raise ValueError("trailing text in marker %r" % text)
    return result


def marker_holds(text):
    if Marker is not None:
        try:
            return Marker(text).evaluate()
        except Exception:
            pass
    return evaluate_marker(text)


def applies(markers):
    """Whether a uv.lock entry belongs to this interpreter's environment: it
    names no resolution markers (it is in every fork), or one of them holds
    here. A marker neither packaging nor evaluate_marker can read counts as
    holding, so a package forked on it goes unpinned rather than mispinned."""
    if not markers:
        return True
    for m in markers:
        try:
            if marker_holds(m):
                return True
        except Exception:
            return True
    return False


# --- the runtime's constraints -----------------------------------------------


def runtime_constrained(path, seen=None):
    """The distributions the runtime image's own constraints file pins,
    following a -c or -r line in it to the file it names, relative to it."""
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
        if not line:
            continue
        m = re.match(r"(?:-c|--constraint|-r|--requirement)(?:\s+|=)(\S+)$", line)
        if m:
            out |= runtime_constrained(os.path.join(os.path.dirname(path), m.group(1)), seen)
        elif not line.startswith("-"):
            out.add(dist_name(line))
    return out


# --- the lock ----------------------------------------------------------------


def lock_root(lock):
    for p in lock.get("package") or []:
        source = p.get("source") or {}
        if source.get("virtual") == "." or source.get("editable") == ".":
            return p
    return None


def same_specifier(a, b):
    if SpecifierSet is not None:
        return SpecifierSet(a) == SpecifierSet(b)
    return sorted(s.strip() for s in a.split(",") if s.strip()) == sorted(s.strip() for s in b.split(",") if s.strip())


def parse_requirement(text):
    """(name, specifier, extras, url) of a PEP 508 requirement."""
    if Requirement is not None:
        r = Requirement(text)
        return norm(r.name), str(r.specifier), sorted(norm(e) for e in r.extras), r.url
    head = text.split(";", 1)[0]
    name = dist_name(head)
    url = head.split("@", 1)[1].strip() if "@" in head else None
    extras = []
    m = re.search(r"\[([^\]]*)\]", head)
    if m:
        extras = sorted(norm(e.strip()) for e in m.group(1).split(",") if e.strip())
        head = head[: m.start()] + head[m.end() :]
    spec = "" if url else re.sub(r"^\s*[A-Za-z0-9._-]+\s*", "", head).strip().strip("()")
    return name, spec, extras, url


def stale_reason(root, stated, locked_versions):
    """Why uv.lock does not resolve pyproject.toml's dependencies as they now
    stand, or None. Each dependency has to be in the lock's own record of the
    project, with the same version specifier and extras, and the version the
    lock resolved has to satisfy it."""
    if root is None:
        return None
    recorded = {}
    for r in (root.get("metadata") or {}).get("requires-dist") or []:
        recorded.setdefault(norm(r.get("name", "")), []).append(r)
    for dep in stated:
        name, spec, extras, url = parse_requirement(dep)
        entries = recorded.get(name)
        if not entries:
            return "it does not resolve %s" % name
        if url:
            continue
        if not any(same_specifier(spec, e.get("specifier", "")) and sorted(norm(x) for x in e.get("extras") or []) == extras for e in entries):
            return "it resolves %s for another requirement than %r" % (name, dep)
        versions = locked_versions.get(name) or set()
        if spec and SpecifierSet is not None and len(versions) == 1:
            (version,) = versions
            if version is not None and not SpecifierSet(spec).contains(version, prereleases=True):
                return "it holds %s at %s, which %r excludes" % (name, version, dep)
    return None


def file_tags(filename):
    """The (interpreter, abi, platform) tags a wheel's file name states."""
    parts = filename[: -len(".whl")].split("-")
    if len(parts) < 5:
        return set()
    return {(i, a, p) for i in parts[-3].split(".") for a in parts[-2].split(".") for p in parts[-1].split(".")}


def with_hash(entry):
    url, digest = entry.get("url"), entry.get("hash")
    if not url:
        return None
    if isinstance(digest, str) and ":" in digest:
        algo, value = digest.split(":", 1)
        url += "#%s=%s" % (algo, value)
    return url


def pick_file(entry):
    """The URL, with its hash, of the file of a locked package this
    interpreter installs: its best wheel by the interpreter's own tag order,
    else its sdist. None when the lock records neither, or no wheel fits and
    there is no sdist."""
    wheels = [w for w in entry.get("wheels") or [] if isinstance(w, dict) and w.get("url")]
    if wheels and sys_tags is not None:
        rank = {}
        for n, t in enumerate(sys_tags()):
            rank.setdefault((t.interpreter, t.abi, t.platform), n)
        best = None
        for w in wheels:
            filename = w["url"].split("#", 1)[0].split("?", 1)[0].rsplit("/", 1)[-1]
            fits = [rank[t] for t in file_tags(filename) if t in rank]
            if fits and (best is None or min(fits) < best[0]):
                best = (min(fits), w)
        if best:
            return with_hash(best[1])
    sdist = entry.get("sdist")
    if isinstance(sdist, dict):
        return with_hash(sdist)
    return None


class Lock:
    """What uv.lock holds for this interpreter: pins for the constraints file,
    and the exact file of each dependency read from an index of its own."""

    def __init__(self, lock, manifest, runtime):
        packages = lock.get("package") or []
        root = lock_root(lock)
        entries = {}
        for p in packages:
            if p is root or not applies(p.get("resolution-markers")):
                continue
            entries.setdefault(norm(p.get("name", "")), []).append(p)
        versions = {}
        for name, es in entries.items():
            versions[name] = {e.get("version") if "registry" in (e.get("source") or {}) else None for e in es}

        self.stale = stale_reason(root, manifest.stated, versions)
        self.files, self.pins, self.forked = {}, [], []
        if self.stale:
            return
        installed = {dist_name(d) for d in manifest.deps}
        # A dependency an index of its own serves is installed from the very
        # file the lock resolved, by URL and hash, so the install never asks
        # PyPI for a package of that name.
        for name in manifest.index_sources:
            es = entries.get(name) or []
            if name in installed and len(es) == 1 and "registry" in (es[0].get("source") or {}):
                url = pick_file(es[0])
                if url:
                    self.files[name] = url
        for name in sorted(versions):
            vs = versions[name]
            if name in AIRFLOW_DISTS or name in runtime or name in self.files or None in vs:
                continue
            if len(vs) != 1:
                self.forked.append(name)
                continue
            self.pins.append("%s==%s" % (name, next(iter(vs))))

    def constraints(self):
        lines = ["# Generated from uv.lock by .astro/install-project.py."]
        if self.forked:
            lines.append("# Not pinned, uv.lock holds more than one version: " + ", ".join(self.forked))
        return lines + self.pins


def write(path, lines):
    with open(path, "w") as f:
        f.write("\n".join(lines) + "\n" if lines else "")


def read_find_links(project):
    for path in (os.path.join(project, FIND_LINKS), os.path.join(project, ".astro", FIND_LINKS)):
        try:
            with open(path) as f:
                return [line.strip() for line in f if line.strip() and not line.startswith("#")]
        except OSError:
            continue
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
                lock = Lock(tomllib.load(f), manifest, runtime_constrained(args.runtime_constraints))
            if lock.stale:
                warn("uv.lock is out of date with pyproject.toml (%s); run `uv lock`; installing pyproject.toml's dependencies unpinned" % lock.stale)
                lock = None
        except Exception as e:  # a lock this cannot read installs as no lock
            warn("uv.lock could not be read (%s); installing pyproject.toml's dependencies unpinned" % e)
            lock = None

    files = lock.files if lock else {}
    pages = manifest.find_links(skip=files) + read_find_links(args.project)
    requirements = ["--find-links " + page for page in pages]
    if lock:
        requirements.append("-c " + LOCK_CONSTRAINTS)
    deps = [direct_reference(d, files) for d in manifest.deps] if files else manifest.deps

    os.makedirs(args.out, exist_ok=True)
    # Written either way, empty without a lock, so the step that copies it
    # into the install always finds it.
    write(os.path.join(args.out, LOCK_CONSTRAINTS), lock.constraints() if lock else [])
    with open(os.path.join(args.out, REQUIREMENTS), "w") as f:
        f.write("\n".join(requirements + deps) + "\n")
    write(os.path.join(args.out, PACKAGES), with_git_for_vcs_deps(deps, manifest.packages))


if __name__ == "__main__":
    main()
