# Astro CLI Breaking Changes (Nexus Integration)

This document lists all breaking changes introduced by the Nexus integration. Commands marked as "removed" have been replaced by auto-generated API commands powered by Nexus. The new commands use the same noun-verb structure but accept API-level parameters instead of CLI-specific flags.

## Removed Commands

### Deployment

| Old Command | Replacement | Notes |
|---|---|---|
| `deployment create` | `deployment create` (nexus) | Prompts for required fields interactively; see [Removed Features](#--deployment-file-flag) for `--deployment-file` removal |
| `deployment update` | `deployment update` (nexus) | Prompts for confirmation when args auto-filled from config; see [Removed Features](#--deployment-file-flag) for `--deployment-file` removal |
| `deployment delete` | `deployment delete` (nexus) | Prompts for confirmation; use `--force` to skip; no `--deployment-name` lookup |
| `deployment list` | `deployment list` (nexus) | No `--all` flag for cross-workspace listing; output is pretty-printed JSON instead of a formatted table |
| `deployment inspect` | `deployment get` | Output is pretty-printed JSON instead of a custom formatted YAML/JSON view; no `--key` field selector; no `--template` flag |
| `deployment hibernate` | `deployment-hibernation-override update` | No `--for` duration shorthand; no `--remove-override`; no `--deployment-name` lookup |
| `deployment wake-up` | `deployment-hibernation-override update` / `delete` | Same as hibernate; wake-up is now an update with `is_hibernating=false` or a delete of the override |
| `deployment user add` | `user-role update` | Scoped to org level; requires user ID instead of email; no interactive prompts |
| `deployment user list` | `user list` | Lists org-level users; no deployment-scoped filtering |
| `deployment user update` | `user-role update` | Scoped to org level; requires user ID instead of email; no interactive role prompt |
| `deployment user remove` | `user-role update` | Scoped to org level; requires user ID instead of email |
| `deployment team list` | `team list` | Lists org-level teams; no deployment-scoped filtering |
| `deployment team add` | `team-role update` | Scoped to org level; no interactive prompts |
| `deployment team update` | `team-role update` | Scoped to org level; no interactive role prompt |
| `deployment team remove` | `team-role update` | Scoped to org level |
| `deployment token list` | `api-token list` | Lists org-level tokens; no deployment-scoped filtering |
| `deployment token create` | `api-token create` | Creates org-level token with deployment role in body; no interactive prompts |
| `deployment token update` | `api-token update` | No `--name` lookup; requires token ID |
| `deployment token rotate` | `api-token rotate` | Prompts for confirmation; use `--force` to skip; no `--name` lookup |
| `deployment token delete` | `api-token delete` | Prompts for confirmation; use `--force` to skip; no `--name` lookup |
| `deployment token organization-token add` | `api-token-role update` | Different command path |
| `deployment token organization-token update` | `api-token-role update` | Different command path |
| `deployment token organization-token remove` | `api-token-role update` | Different command path |
| `deployment token organization-token list` | `api-token list` | Different command path |
| `deployment token workspace-token add` | `api-token-role update` | Different command path |
| `deployment token workspace-token update` | `api-token-role update` | Different command path |
| `deployment token workspace-token remove` | `api-token-role update` | Different command path |
| `deployment token workspace-token list` | `api-token list` | Different command path |

### Workspace

| Old Command | Replacement | Notes |
|---|---|---|
| `workspace create` | `workspace create` (nexus) | No `--enforce-cicd` shorthand flag |
| `workspace update` | `workspace update` (nexus) | No `--enforce-cicd` shorthand flag |
| `workspace delete` | `workspace delete` (nexus) | Prompts for confirmation; use `--force` to skip |
| `workspace list` | `workspace list` (nexus) | Output is pretty-printed JSON instead of a formatted table |
| `workspace user add` | `user-role update` | Scoped to org level; requires user ID instead of email; no interactive prompts |
| `workspace user list` | `user list` | Lists org-level users; no workspace-scoped filtering |
| `workspace user update` | `user-role update` | Scoped to org level; requires user ID instead of email; no interactive role prompt |
| `workspace user remove` | `user-role update` | Scoped to org level; requires user ID instead of email |
| `workspace team list` | `team list` | Lists org-level teams; no workspace-scoped filtering |
| `workspace team add` | `team-role update` | Scoped to org level; no interactive prompts |
| `workspace team update` | `team-role update` | Scoped to org level; no interactive role prompt |
| `workspace team remove` | `team-role update` | Scoped to org level |
| `workspace token list` | `api-token list` | Lists org-level tokens; no workspace-scoped filtering |
| `workspace token create` | `api-token create` | Creates org-level token with workspace role in body; no interactive prompts |
| `workspace token update` | `api-token update` | No `--name` lookup; requires token ID |
| `workspace token rotate` | `api-token rotate` | Prompts for confirmation; use `--force` to skip; no `--name` lookup |
| `workspace token delete` | `api-token delete` | Prompts for confirmation; use `--force` to skip; no `--name` lookup |
| `workspace token add` | `api-token-role update` | Different command path; was for adding org token to workspace |
| `workspace token organization-token add` | `api-token-role update` | Different command path |
| `workspace token organization-token update` | `api-token-role update` | Different command path |
| `workspace token organization-token remove` | `api-token-role update` | Different command path |
| `workspace token organization-token list` | `api-token list` | Different command path |

### Organization

| Old Command | Replacement | Notes |
|---|---|---|
| `organization list` | `organization list` (nexus) | Output is pretty-printed JSON instead of a formatted table |
| `organization user invite` | `user-invite create` | No interactive email prompt; different command path |
| `organization user list` | `user list` | Different command path (no longer under `organization`) |
| `organization user update` | `user-role update` | No interactive role selection prompt; different command path |
| `organization team create` | `team create` | No interactive role selection prompt; different command path |
| `organization team delete` | `team delete` | No interactive team selection prompt; different command path |
| `organization team list` | `team list` | Different command path |
| `organization team update` | `team update` | No interactive team selection prompt; different command path |
| `organization team user add` | `team-member add` | Different command path |
| `organization team user list` | `team-member list` | Different command path |
| `organization team user remove` | `team-member remove` | Different command path |
| `organization audit-logs export` | `organization-audit-log get` | No `--output-file` flag; no `--include` days parameter; output is not automatically saved to a GZIP file |
| `organization token create` | `api-token create` | No interactive name/role prompts; different command path |
| `organization token delete` | `api-token delete` | Prompts for confirmation; use `--force` to skip; different command path |
| `organization token list` | `api-token list` | Different command path |
| `organization token update` | `api-token update` | Different command path |
| `organization token rotate` | `api-token rotate` | Prompts for confirmation; use `--force` to skip; different command path |
| `organization token roles` | `api-token-role list` | Different command path |
| `organization role list` | `role list` | No `--include-default-roles` flag; different command path |

## Removed Features

### `--deployment-file` flag

The `--deployment-file` flag on `deployment create` and `deployment update` has been removed. This flag allowed creating or updating a deployment from a YAML/JSON specification file that included:

- Deployment configuration (name, executor, runtime version, cloud provider, region, etc.)
- Worker queue definitions with validation against API defaults
- Environment variables
- Alert email addresses
- Hibernation schedules
- Automatic cluster name-to-ID and workspace name-to-ID resolution

**Migration**: Use the nexus `deployment create` / `deployment update` commands with individual flags, or use `astro api` for raw API access with a JSON body.

## Behavioral Changes

### Output format

In a terminal, nexus-generated commands display pretty-printed JSON with 2-space indentation. When output is piped (non-terminal), raw compact JSON is returned for scripting compatibility (e.g., `astro deployment list | jq '.deployments[].id'`). API error responses are formatted as `Error (<status>): <message>` instead of raw JSON.

### Interactive prompts

Nexus-generated commands now prompt interactively for missing required body fields when running in a terminal. Enum fields show a numbered selection menu. When piped, no prompts are shown and the API will return a validation error for missing fields.

### Delete confirmation

All `delete` commands now prompt for confirmation (`Are you sure you want to delete? [y/N]`) in a terminal. Use `--force` / `-f` to skip the prompt. Update commands still prompt only when positional arguments were auto-filled from config defaults.

### Name-based lookups

The old commands supported `--deployment-name` to look up deployments by name. The nexus-generated commands require the resource ID directly. Use `deployment list` to find the ID first.

### Flag naming

Nexus-generated commands use API field names in kebab-case (e.g., `--is-dag-deploy-enabled` instead of `--dag-deploy`, `--is-cicd-enforced` instead of `--enforce-cicd`). Run `<command> --help` to see available flags.
