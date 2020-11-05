import os
import re
import sys
from github import Github
from packaging.version import parse as semver

branch = os.environ["CIRCLE_BRANCH"]
org = os.environ["GIT_ORG"]
repository = os.environ["CIRCLE_PROJECT_REPONAME"]
github = Github()
repo = github.get_repo(f"{ org }/{ repository }")

release_regex = re.compile("release-(\d*\.\d*)")
major_minor_version = release_regex.findall(branch)
most_recent_tag = None

if not len(major_minor_version):
    raise Exception(
        "ERROR: we are not on a release branch. Branch should be named release-X.Y where X and Y are positive integers"
    )
major_minor_version = major_minor_version[0]
print(
    f"We are on a release branch: {branch}, detected major.minor version {major_minor_version}",
    file=sys.stderr,
)
print(
    f"We will find the most recent patch version of {major_minor_version} and return it incremented by one",
    file=sys.stderr,
)
for release in repo.get_releases():
    version = semver(release.tag_name).release
    if not version:
        continue
    this_major_minor = f"{version[0]}.{version[1]}"
    if this_major_minor == major_minor_version:
        most_recent_tag = release.tag_name
        print(
            f"The most recent tag matching this release is {most_recent_tag}",
            file=sys.stderr,
        )
        break

if most_recent_tag:
    base_version = semver(most_recent_tag).base_version
    print(f"Parsed base version as {base_version}", file=sys.stderr)
    base_version = semver(base_version)
    major, minor, patch = base_version.release
    patch += 1
    new_version = ".".join(str(i) for i in [major, minor, patch])
else:
    print(
        "Did not detect a most recent version. Setting patch number to zero.",
        file=sys.stderr,
    )
    new_version = major_minor_version + ".0"

new_tag_regex = re.compile("\d*\.\d*\.\d*")
assert len(
    new_tag_regex.findall(new_version)
), f"Error, did not produce a new tag in the form {new_tag_regex.pattern}"
print(f"Calculated new version as {new_version}", file=sys.stderr)
sys.stdout.write(new_version)
