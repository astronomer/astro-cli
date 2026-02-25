---
name: bundle-files
description: Reference for the `astro bundle` CLI commands that manage deployment DAG bundle files. Use when uploading, downloading, listing, deleting, moving, or duplicating files in an Astro deployment bundle, or syncing a local directory as a bundle archive.
---

# Bundle Files CLI Commands

The `astro bundle` commands provide file-level management of deployment DAG bundles for API-driven deployments. These commands interact with the Core API's BundleFiles endpoints (Core -> Laminar -> Gitea).

## Command Overview

```
astro bundle
  sync         [DEPLOYMENT-ID]      - Tar+gzip a local directory and upload as archive
  file                              - Individual file operations
    list       [DEPLOYMENT-ID]      - List files in bundle
    upload     [DEPLOYMENT-ID]      - Upload a single local file
    download   [DEPLOYMENT-ID]      - Download a single file
    delete     [DEPLOYMENT-ID]      - Delete a file
    move       [DEPLOYMENT-ID]      - Move/rename a file within the bundle
    duplicate  [DEPLOYMENT-ID]      - Duplicate a file within the bundle
  archive                           - Archive operations
    upload     [DEPLOYMENT-ID]      - Upload a pre-built tar.gz archive
    download   [DEPLOYMENT-ID]      - Download entire bundle as tar.gz archive
```

All commands accept `--deployment-id`, `--deployment-name`, and `--workspace-id` flags, or a deployment ID as the first positional argument. If no deployment is specified, an interactive prompt is shown.

## Commands

### bundle sync

Tar and gzip a local directory and upload it as a bundle archive. This is the primary command for pushing local DAG files to a deployment.

```bash
# Sync current directory
astro bundle sync <deployment-id>

# Sync a specific directory
astro bundle sync <deployment-id> --path /path/to/dags

# Sync without overwriting existing files
astro bundle sync <deployment-id> --no-overwrite

# Sync into a subdirectory within the bundle
astro bundle sync <deployment-id> --target-path dags/subdir
```

Flags:
- `--path` (default: `.`) - Local directory to sync
- `--target-path` - Remote target path within the bundle
- `--no-overwrite` - Do not overwrite existing files

### bundle file list (alias: ls)

List files in a deployment bundle.

```bash
astro bundle file list <deployment-id>
astro bundle file ls <deployment-id> --path dags/
```

Flags:
- `--path` - Remote path prefix filter

### bundle file upload (alias: up)

Upload a single local file to the bundle.

```bash
astro bundle file upload <deployment-id> /path/to/local/file.py --remote-path dags/file.py
```

Flags:
- `--remote-path` (required) - Destination path within the bundle

### bundle file download (alias: dl)

Download a single file from the bundle.

```bash
# Download to current directory (uses remote filename)
astro bundle file download <deployment-id> dags/example.py

# Download to specific path
astro bundle file download <deployment-id> dags/example.py --output /tmp/example.py
```

Flags:
- `--output`, `-o` - Local output file path (default: basename of remote path)

### bundle file delete (alias: rm)

Delete a file from the bundle.

```bash
astro bundle file delete <deployment-id> dags/old_dag.py
```

### bundle file move (alias: mv)

Move/rename a file within the bundle.

```bash
astro bundle file move <deployment-id> dags/old.py --destination dags/new.py
```

Flags:
- `--destination` (required) - Destination path within the bundle

### bundle file duplicate (alias: cp)

Duplicate a file within the bundle.

```bash
astro bundle file duplicate <deployment-id> dags/template.py --destination dags/new_dag.py
```

Flags:
- `--destination` (required) - Destination path within the bundle

### bundle archive upload (alias: up)

Upload a pre-built tar.gz archive to the bundle.

```bash
astro bundle archive upload <deployment-id> /path/to/bundle.tar.gz
astro bundle archive upload <deployment-id> /path/to/bundle.tar.gz --target-path dags/ --no-overwrite
```

Flags:
- `--target-path` - Remote target path within the bundle
- `--no-overwrite` - Do not overwrite existing files

### bundle archive download (alias: dl)

Download the entire bundle as a tar.gz archive.

```bash
astro bundle archive download <deployment-id>
astro bundle archive download <deployment-id> --output /tmp/my-bundle.tar.gz
```

Flags:
- `--output`, `-o` (default: `bundle.tar.gz`) - Local output file path

## Architecture

### Code Structure

| File | Purpose |
|---|---|
| `astro-client-core/bundle_files.go` | `BundleFilesStreamClient` interface + streaming HTTP implementation |
| `cloud/bundle/bundle.go` | Business logic for all bundle operations |
| `cmd/cloud/bundle.go` | Cobra command definitions |
| `cmd/cloud/root.go` | Registration of `bundleStreamClient` and `newBundleCmd` |

### Client Strategy

The implementation uses a hybrid approach:
- **Generated client** (via `oapi-codegen`): Used for `list`, `delete`, `move`, and `duplicate` operations (simple request/response with JSON bodies)
- **Manual streaming client** (`BundleFilesStreamClient`): Used for `upload file`, `download file`, `upload archive`, and `download archive` (octet-stream bodies requiring streaming I/O)

The streaming client reuses `CoreRequestEditor` from `astro-client-core/client.go` for authentication and base URL construction.

### API Endpoints

Base: `/organizations/{orgId}/deployments/{deploymentId}/bundle/`

| Endpoint | Method | Operation |
|---|---|---|
| `/files?path=` | GET | List files |
| `/files/download/{path}` | GET | Download file (streaming) |
| `/files/upload/{path}` | POST | Upload file (streaming) |
| `/files/delete/{path}` | POST | Delete file |
| `/files/move/{path}` | POST | Move file (JSON body: `{destination}`) |
| `/files/duplicate/{path}` | POST | Duplicate file (JSON body: `{destination}`) |
| `/archive` | GET | Download archive (streaming) |
| `/archive?overwrite&targetPath` | POST | Upload archive (streaming) |
