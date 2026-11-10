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

## Credentials

The CLI still sends an `Authorization` header, so the listener has something to replace. What the CLI needs to start is a token it will accept:

- An `ASTRO_API_TOKEN` is read for its claims (`permissions`, `exp`) and its signature is never checked. A listener can give the CLI a stand-in token that carries the real token's claims, signed with any key, and swap in the real token upstream. The claims are IDs (organization, workspace, Deployment) and an expiry, not secrets.
- A saved `astro login` renews itself against the login server, which is not on the socket. Use an API token instead.

The Airflow of an Astro Deployment takes the same Astro token, so `astro af` and `astro api airflow -d` need nothing more. An Airflow addressed with `--url` reads `ASTRO_AIRFLOW_TOKEN` or a username and password instead (see [instances.md](instances.md#link-kinds-and-authentication)). MWAA, Composer and self-hosted Airflow authenticate with credentials the CLI reads from your machine, so a listener cannot stand in for them by swapping a header.

## Where it lives

`internal/httpsocket` replaces `http.DefaultTransport` at the start of `main`, before any command runs. Every client in the CLI that names no transport of its own reads `http.DefaultTransport` on each request, which covers the Astro API clients, `astro api` and the Airflow clients. The Software (APC) client builds its own transport and does not use the socket.
