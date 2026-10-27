# Written by the Astro CLI. Do not edit: `astro init` replaces this file.
# astro-install-helper: 9
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
# The index pages come from pyproject.toml itself ([tool.uv.sources] entries
# naming a [[tool.uv.index]], for the packages the image installs), on every
# run, so `docker build .` reads the same ones the CLI does. This file only adds
# to them: pages for a build the CLI generates from a request that names no
# project, and so has no pyproject.toml sources, one per line, passed through
# as they are, beside pyproject.toml in the inputs the stage copies. No
# project's build ever has it, and a project's own copy is never read.
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


# --- credentials -------------------------------------------------------------

# Set when a URL the helper would have written carried a user or password.
_saw_credentials = [False]


def without_credentials(url):
    """url without the user and password an http(s) URL can carry
    (https://user:token@host/...), with or without a VCS prefix (git+https).
    What the helper writes lands in an image layer and the build log, so a
    credential never goes in it; the build authenticates through the netrc
    secret instead. An ssh URL's user (git+ssh://git@host) is not a
    credential and is kept."""
    prefix, rest = "", url
    m = re.match(r"[A-Za-z][A-Za-z0-9.-]*\+(?=[A-Za-z][A-Za-z0-9+.-]*://)", url)
    if m:
        prefix, rest = m.group(0), url[m.end():]
    try:
        parts = urlsplit(rest)
    except ValueError:
        return url
    if parts.scheme.lower() not in ("http", "https") or "@" not in parts.netloc:
        return url
    _saw_credentials[0] = True
    return prefix + urlunsplit(parts._replace(netloc=parts.netloc.rsplit("@", 1)[1]))


# --- sources -----------------------------------------------------------------

# A source the stale check cannot compare with the lock's record: limited by a
# marker, an extra or a group, a list of sources, a workspace member, or an
# index pyproject.toml does not define.
UNKNOWN = object()


def url_key(url):
    url = without_credentials(url)
    if url.startswith("git+"):
        url = url[len("git+") :]
    parts = urlsplit(url)
    return urlunsplit((parts.scheme.lower(), parts.netloc.lower(), parts.path.rstrip("/"), parts.query, ""))


def source_key(source, indexes):
    """What a [tool.uv.sources] entry routes its package to, in the shape
    lock_source_key reads the lock's record of it; None for no entry."""
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
    """What the lock's record of a dependency (its root package's
    requires-dist) routes it to, as source_key states it; None for nothing."""
    if isinstance(entry.get("git"), str):
        address, _, _ = entry["git"].partition("#")
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
    """What pyproject.toml asks to install.

    stated: [project] dependencies as written.
    deps: the same as a requirements file has to state them: without Airflow,
      a git or URL source as a direct reference.
    index_sources: {normalized name: (name as written, index URL)} for each
      dependency a [tool.uv.sources] entry routes to a [[tool.uv.index]].
    sources: {normalized name: source_key} for each [tool.uv.sources] entry.
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
        self.index_sources, self.sources, urls = {}, {}, {}
        for pkg, source in (uv.get("sources") or {}).items():
            self.sources[norm(pkg)] = source_key(source, indexes)
            if not isinstance(source, dict):
                continue
            name = source.get("index")
            if isinstance(name, str) and name in indexes:
                self.index_sources[norm(pkg)] = (pkg, without_credentials(indexes[name]))
            address = direct_url(source)
            if address is not None:
                urls[norm(pkg)] = without_credentials(address)

        self.stated = strings(project.get("dependencies"))
        # Every distribution the project names itself, in its dependencies,
        # its optional dependencies or a dependency group: the ones the lock
        # may move off the base image's versions.
        named = list(self.stated)
        for extra in (project.get("optional-dependencies") or {}).values():
            named += strings(extra)
        for entries in (pyproject.get("dependency-groups") or {}).values():
            named += strings(entries)
        self.named = {dist_name(d) for d in named}
        self.constraint_dependencies = strings(uv.get("constraint-dependencies"))
        self.override_dependencies = strings(uv.get("override-dependencies"))
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


def requirement_key(name, spec):
    """A constraint or override's name and specifiers, as a comparable value."""
    if SpecifierSet is not None:
        try:
            spec = ",".join(sorted(str(s) for s in SpecifierSet(spec)))
        except Exception:
            pass
    else:
        spec = ",".join(sorted(s.strip() for s in spec.split(",") if s.strip()))
    return (norm(name), spec.replace(" ", ""))


# Not a marker canonical_marker could read: it matches any other.
ANY_MARKER = object()


def canonical_marker(marker):
    """marker in one spelling of what it means, so the one pyproject.toml states
    and the one uv records of it compare equal: its tokens with single spaces
    and single quotes, and python_version == 'X.Y' (or !=) written as
    python_full_version == 'X.Y.*', as uv records it. "" for none. ANY_MARKER
    for one it cannot spell reliably, which then never makes a lock stale: one
    the tokenizer cannot read, and any but a single comparison, since uv may
    reorder or regroup a compound marker in ways a token-level spelling would
    call a change."""
    if not marker or not marker.strip():
        return ""
    try:
        tokens = _marker_tokens(marker)
    except ValueError:
        return ANY_MARKER
    if len(tokens) != 3 or tokens[1][0] != "op":
        return ANY_MARKER
    out, i = [], 0
    while i < len(tokens):
        kind, text = tokens[i]
        if (
            kind == "var"
            and text == "python_version"
            and i + 2 < len(tokens)
            and tokens[i + 1] in (("op", "=="), ("op", "!="))
            and tokens[i + 2][0] == "str"
            and re.fullmatch(r"\d+\.\d+", tokens[i + 2][1][1:-1])
        ):
            out += ["python_full_version", tokens[i + 1][1], "'%s.*'" % tokens[i + 2][1][1:-1]]
            i += 3
            continue
        out.append("'%s'" % text[1:-1] if kind == "str" else text)
        i += 1
    return " ".join(out)


def same_requirements(stated, recorded):
    """Whether [tool.uv] constraint-dependencies (or override-dependencies) as
    written match what the lock's [manifest] recorded of them: the same names
    and specifiers, each as many times, with markers that mean the same once
    canonical_marker has spelled them alike. A marker it cannot read matches
    any marker, so a spelling this does not know never calls a fresh lock
    stale."""

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
        if ANY_MARKER in markers or ANY_MARKER in others:
            continue
        if sorted(markers) != sorted(others):
            return False
    return True


def stale_reason(lock, root, manifest, locked_versions):
    """Why uv.lock does not resolve pyproject.toml's dependencies as they now
    stand, or None. The lock has to have a record of the project. Each
    dependency has to be in it, routed to the same [tool.uv.sources] source,
    with the same extras and, unless that source is a git repository, a URL or
    a path (for which uv records no specifier), the same version specifier; and
    the version the lock resolved has to satisfy it. And [tool.uv]
    constraint-dependencies and override-dependencies have to be what the
    lock's [manifest] recorded."""
    if root is None:
        return "it has no record of the project"
    recorded_manifest = lock.get("manifest") or {}
    if not same_requirements(manifest.constraint_dependencies, recorded_manifest.get("constraints")):
        return "it was resolved under other [tool.uv] constraint-dependencies"
    if not same_requirements(manifest.override_dependencies, recorded_manifest.get("overrides")):
        return "it was resolved under other [tool.uv] override-dependencies"
    stated, sources = manifest.stated, manifest.sources
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
        source = sources.get(name)
        if source is not UNKNOWN:
            entries = [e for e in entries if lock_source_key(e) == source]
            if not entries:
                return "it resolves %s from another source than pyproject.toml's" % name
        if not any(sorted(norm(x) for x in e.get("extras") or []) == extras for e in entries):
            return "it resolves %s with other extras than %r" % (name, dep)
        if isinstance(source, tuple) and source[0] in ("git", "url", "path"):
            continue
        if not any(same_specifier(spec, e.get("specifier", "")) for e in entries):
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
    url = without_credentials(url)
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


# The index the install reads for everything not routed elsewhere: the runtime
# image's installer reads PyPI and nothing else.
PYPI = url_key("https://pypi.org/simple")


class Lock:
    """What uv.lock holds for this interpreter: pins for the constraints file,
    and the exact file of each dependency read from an index of its own.

    runtime: the distributions the runtime's own constraints file pins.
    base: the distributions the base image already has installed."""

    def __init__(self, lock, manifest, runtime, base):
        packages = lock.get("package") or []
        root = lock_root(lock)
        entries = {}
        for p in packages:
            if p is root or not applies(p.get("resolution-markers")):
                continue
            entries.setdefault(norm(p.get("name", "")), []).append(p)
        versions, registries = {}, {}
        for name, es in entries.items():
            versions[name] = {e.get("version") if "registry" in (e.get("source") or {}) else None for e in es}
            registries[name] = {url_key((e.get("source") or {}).get("registry") or "") for e in es}

        self.stale = stale_reason(lock, root, manifest, versions)
        self.files, self.pins, self.forked, self.elsewhere = {}, [], [], []
        if self.stale:
            return
        direct = {dist_name(d) for d in manifest.deps}
        # A dependency an index of its own serves is installed from the very
        # file the lock resolved, by URL and hash, so the install never asks
        # PyPI for a package of that name.
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
            # What the base image has installed stays as the runtime ships it,
            # its tested versions, unless the project names the package itself
            # (a dependency, an optional one, a dependency group). A lock
            # resolved under the runtime's own versions (the freeze pins astro
            # writes into constraint-dependencies) holds the same versions, so
            # this changes nothing there; one resolved without them would
            # otherwise move Airflow's own dependencies off what it was tested
            # with.
            if name in base and name not in manifest.named:
                continue
            # The install reads PyPI. A pin to a version the lock took from
            # another index (a mirror, a private index) could name a version
            # PyPI does not have, so that package installs unpinned, as it did
            # before the lock.
            if registries[name] != {PYPI}:
                self.elsewhere.append(name)
                continue
            if len(vs) != 1:
                self.forked.append(name)
                continue
            self.pins.append("%s==%s" % (name, next(iter(vs))))

    def constraints(self):
        lines = ["# Generated from uv.lock by .astro/install-project.py."]
        if self.forked:
            lines.append("# Not pinned, uv.lock holds more than one version: " + ", ".join(self.forked))
        if self.elsewhere:
            lines.append("# Not pinned, uv.lock took them from an index other than PyPI: " + ", ".join(self.elsewhere))
        return lines + self.pins


def base_installed():
    """The distributions installed in this image, which is the runtime's base:
    the install stage runs on the same -base image as the install. The CLI's
    tests name them in ASTRO_INSTALL_PROJECT_BASE_INSTALLED instead."""
    if "ASTRO_INSTALL_PROJECT_BASE_INSTALLED" in os.environ:
        return {norm(n) for n in os.environ["ASTRO_INSTALL_PROJECT_BASE_INSTALLED"].split(",") if n.strip()}
    try:
        from importlib import metadata
    except ImportError:
        return set()
    return {norm(d.metadata["Name"]) for d in metadata.distributions() if d.metadata["Name"]}


def base_airflow():
    """The Airflow release this image has installed, or None when it cannot
    tell. The CLI's tests name it in ASTRO_INSTALL_PROJECT_BASE_AIRFLOW; a test
    that names the base's packages without it has no Airflow to compare."""
    if "ASTRO_INSTALL_PROJECT_BASE_AIRFLOW" in os.environ:
        return os.environ["ASTRO_INSTALL_PROJECT_BASE_AIRFLOW"].strip() or None
    if "ASTRO_INSTALL_PROJECT_BASE_INSTALLED" in os.environ:
        return None
    try:
        from importlib import metadata

        return metadata.version(AIRFLOW)
    except Exception:
        return None


def locked_airflow(lock):
    """The Airflow release uv.lock resolved for this interpreter, or None when
    it holds none, or more than one."""
    versions = {
        p.get("version")
        for p in lock.get("package") or []
        if norm(p.get("name", "")) == AIRFLOW and p.get("version") and applies(p.get("resolution-markers"))
    }
    return next(iter(versions)) if len(versions) == 1 else None


# The comment astro writes in [tool.uv] constraint-dependencies above the
# runtime's own constraints, naming the build the project builds on.
RUNTIME_RECORD = re.compile(r"^\s*#\s*Astro Runtime (\S+) \(Python \d+\.\d+\) constrains these\.")
TABLE_HEADER = re.compile(r"^\s*\[\s*([^\[\]]+?)\s*\]\s*(#.*)?$")
CONSTRAINTS_KEY = re.compile(r"^\s*constraint-dependencies\s*=\s*\[")
QUOTED = re.compile(r"'[^']*'|\"(?:[^\"\\]|\\.)*\"")


def recorded_runtime(pyproject_text):
    """The Astro Runtime build the project builds on, as the record astro
    writes in [tool.uv] constraint-dependencies names it, or None. Comments
    are not in what tomllib reads, so this finds the [tool.uv] table's
    constraint-dependencies array in the text, and reads the comment from that
    array alone: the same words anywhere else in the file are not it."""
    table, inside, depth = None, False, 0
    for line in pyproject_text.splitlines():
        if not inside:
            header = TABLE_HEADER.match(line)
            if header:
                table = header.group(1).replace(" ", "")
                continue
            if table == "tool.uv" and CONSTRAINTS_KEY.match(line):
                inside, depth = True, 0
            else:
                continue
        # Brackets and #s inside a string ('pkg[extra]', a URL's #sha256) are
        # not the array's.
        code = QUOTED.sub("", line).split("#", 1)[0]
        depth += code.count("[") - code.count("]")
        m = RUNTIME_RECORD.match(line)
        if m:
            return m.group(1)
        if depth <= 0:
            return None
    return None


def public_release(version):
    """The release a version names, without its local label: 3.3.2+astro.2 and
    3.3.2 are both 3.3.2. Astronomer's builds of one release differ only in
    that label, and an image's Airflow from PyPI has none, so neither is a
    different Airflow from the lock's."""
    return version.strip().split("+", 1)[0].lower()


def airflow_mismatch(lock, installed, runtime):
    """A warning when uv.lock was made for another Airflow than this image
    runs, or None. The lock's versions still install: a build on another runtime
    than the lock's (a floating FROM, a deploy onto the build a Deployment
    offers) is allowed, and said."""
    locked = locked_airflow(lock)
    if not locked or not installed or public_release(locked) == public_release(installed):
        return None
    made_for = "Airflow %s" % locked
    if runtime:
        made_for += " (Astro Runtime %s)" % runtime
    return (
        "uv.lock was made for %s, and this image has Airflow %s, so the lock's versions install over another runtime than "
        "the one they were resolved for: build FROM the runtime the lock was made for, or run `astro init` and `uv lock` "
        "to move the project to this runtime" % (made_for, installed.strip())
    )


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
        pyproject = f.read()
    manifest = Manifest(tomllib.loads(pyproject.decode("utf-8")))

    lock = None
    lock_path = os.path.join(args.project, "uv.lock")
    if os.path.exists(lock_path):
        try:
            base = base_installed()
            with open(lock_path, "rb") as f:
                lock_data = tomllib.load(f)
            lock = Lock(lock_data, manifest, runtime_constrained(args.runtime_constraints), base)
            if lock.stale:
                warn("uv.lock is out of date with pyproject.toml (%s); run `uv lock`; installing pyproject.toml's dependencies unpinned" % lock.stale)
                lock = None
            else:
                if lock.elsewhere:
                    warn("uv.lock took %s from an index other than PyPI, which the image does not read; installing them unpinned" % ", ".join(lock.elsewhere))
                # Every build checks it, `docker build .` included: the lock is
                # for one runtime's Airflow, and this image may be another's. A
                # lock set aside as out of date installs nothing it says.
                mismatch = airflow_mismatch(lock_data, base_airflow(), recorded_runtime(pyproject.decode("utf-8", "replace")))
                if mismatch:
                    warn(mismatch)
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
    with open(os.path.join(args.out, REQUIREMENTS), "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(requirements + deps) + "\n")
    write(os.path.join(args.out, PACKAGES), with_git_for_vcs_deps(deps, manifest.packages))
    if _saw_credentials[0]:
        warn("a URL in pyproject.toml or uv.lock carries a user or password, which the image build leaves out: give the build a netrc secret to authenticate instead")


if __name__ == "__main__":
    main()
