# Sending requests through a Unix socket

Set `ASTRO_HTTP_UNIX_SOCKET` to the path of a Unix domain socket and the CLI sends its requests to Astronomer's hosts there, instead of over the network. Whatever listens on the socket receives each request and decides what to do with it. The usual reason to run one is to keep credentials out of the CLI's process: a local broker holds the real token, applies its own rules to each request (allow it, ask a person, refuse it), and adds the token on the way upstream. A process driving the CLI, such as a coding agent or a script, then never holds a token it could use elsewhere.

This is the same contract as `gh`'s `http_unix_socket` setting, so a listener written for one works for the other.

## What goes over the socket

Requests reach the socket as plain HTTP/1.1. The CLI does no TLS on the socket: the request line keeps the real path and query, the `Host` header keeps the real host, and the URL keeps its `https` scheme. TLS to the real host is the listener's job. Keep-alives are off, so each request arrives on a connection of its own.

Only requests for these hosts go to the socket:

| host | what it is |
| --- | --- |
| `api.astronomer.io`, `api.astronomer-dev.io`, `api.astronomer-stage.io`, `api.astronomer-perf.io` | the Astro API |
| `*.api.astronomer-dev.io` | PR preview APIs |
| `*.astronomer.run`, `*.astronomer-dev.run`, `*.astronomer-stage.run` | the Airflow API of Astro Deployments |

Everything else leaves the way it always did: local Airflow, Docker, the login server (`auth.*`), the CDN, the runtime catalog, package indexes, and other clouds' APIs. `HTTPS_PROXY` and friends still apply to those requests and never to the socket. Telemetry is posted to `api.astronomer.io`, so it reaches the socket like any other API request; `ASTRO_TELEMETRY_DISABLED=1` stops it.

To choose the hosts yourself, set `ASTRO_HTTP_UNIX_SOCKET_HOSTS` to a comma-separated list. Each entry is a host name, or `*.suffix` for every host below `suffix` (not `suffix` itself). The list replaces the default rather than adding to it. An empty value means the default. An entry that is neither form is an error, reported before any command runs.

A relative path is resolved once, against the directory the command started in. A socket nobody is listening on is an error that names `ASTRO_HTTP_UNIX_SOCKET`. The CLI never falls back to the network for a host on the list.

The first time a command's request reaches the socket, the CLI says so on stderr, once:

```console
$ astro deployment list
Note: requests to Astro go through /Users/you/.local/run/astro.sock (ASTRO_HTTP_UNIX_SOCKET)
 NAME       ...
```

A command that never talks to a listed host (`astro version`, `astro local start`, an Airflow on `localhost`) prints nothing, and stdout is untouched, so `-o json` output stays parseable. The note cannot be turned off: its job is to make a socket set without your knowledge visible.

## Trusting the listener

Whatever listens on the socket is trusted completely. It receives every request in plaintext, including whatever `Authorization` header the CLI sends, which is your real token unless the listener gave you a stand-in. There is no TLS on the socket, so the CLI cannot tell the listener from the real host, and it acts on whatever the listener answers.

So put the socket in a directory only you can write to, such as one under your home directory, rather than a shared one. On Linux and macOS the CLI checks the socket's directory before sending anything:

| the socket's directory | the CLI |
| --- | --- |
| writable only by its owner (`~/.run`, a directory mounted into a container) | accepts the socket, whoever owns it |
| writable by any user, with the sticky bit (`/tmp`) | accepts it only if it is yours or root's, since another user could have created it first |
| writable by its group, not by any user | accepts it only if it is yours or root's, since another member of the group could have created it. A directory you made under a `002` umask is one: its group is yours alone, and your own socket in it is accepted |
| writable by any user, without the sticky bit | refuses it, since another user could replace it |

A socket's owner only matters in a shared directory because a socket mounted into a container or a sandbox keeps the host's numeric owner, so inside it often belongs to another user or to `nobody`. Symlinks are followed, and the socket they lead to and its directory are what is checked. On Windows, access is governed by the directory's ACL and the CLI checks nothing.

## Credentials

The CLI still sends an `Authorization` header, so the listener has something to replace. What the CLI needs to start is a token it will accept:

- An `ASTRO_API_TOKEN` is read for its claims (`permissions`, `exp`) and its signature is never checked. A listener can give the CLI a stand-in token that carries the real token's claims, signed with any key, and swap in the real token upstream. The claims are IDs (organization, workspace, Deployment) and an expiry, not secrets.
- A saved `astro login` renews itself against the login server, which is not on the socket. Use an API token instead.

The Airflow of an Astro Deployment takes the same Astro token, so `astro af` and `astro api airflow -d` need nothing more. An Airflow addressed with `--url` reads `ASTRO_AIRFLOW_TOKEN` or a username and password instead (see [instances.md](instances.md#link-kinds-and-authentication)). MWAA, Composer and self-hosted Airflow authenticate with credentials the CLI reads from your machine, so a listener cannot stand in for them by swapping a header.

## Where it lives

`internal/httpsocket` replaces `http.DefaultTransport` at the start of `main`, before any command runs. Every client in the CLI that names no transport of its own reads `http.DefaultTransport` on each request, which covers the Astro API clients, `astro api` and the Airflow clients. The Software (APC) client builds its own transport and does not use the socket.
