# Plan: Add GENERIC Provider Support for Deploy Git Metadata

## Context

We're adding git metadata to all CLI deploys (not just bundle deploys) for better traceability. This allows users to see which commit was deployed, who authored it, and link back to the source.

### Current State

**Backend (PR #35907):**
- v1beta1 `CreateDeploy` endpoint now accepts git metadata via the `Git` field
- The `Provider` field validation only accepts `GITHUB`:
  ```go
  Provider string `json:"provider" binding:"required,oneof=GITHUB"`
  ```

**CLI (PR #1990, branch `feat/deploy-git-metadata`):**
- Extracts git metadata from local repository (commit SHA, branch, author, remote URL)
- Generates commit URL from remote URL
- Hardcodes `Provider: GITHUB` when sending to API

### Problem

Users with non-GitHub repositories (GitLab, Bitbucket, self-hosted, etc.) cannot benefit from git metadata on deploys because:
1. The API rejects any provider value other than `GITHUB`
2. The CLI hardcodes `GITHUB` as the provider

## Proposed Solution

Add `GENERIC` as a fallback provider for non-GitHub repositories.

### Why GENERIC over individual providers?

| Approach | Pros | Cons |
|----------|------|------|
| Individual providers (GitLab, Bitbucket, etc.) | Specific UI display, potential provider-specific features | Must enumerate all providers, self-hosted instances have custom domains, more maintenance |
| GENERIC fallback | Works with any git host, less maintenance, flexible | Less specific UI display |

**Recommendation: GENERIC fallback** because:
1. **Commit URL is already correct** - CLI generates it from the actual remote URL, so links work regardless of provider
2. **Self-hosted instances** - GitLab/Bitbucket can be self-hosted with custom domains (e.g., `git.company.com`), making host detection unreliable
3. **The metadata is informational** - We're not using provider to clone or authenticate, just for traceability/audit
4. **GENERIC already exists** - It's defined in `apps/core/utils/constants/github.go` and used in Polaris

### Provider Detection Logic

```
github.com     -> GITHUB
*              -> GENERIC
```

## Implementation Plan

### Backend Changes (astro repo)

**1. Update v1beta1 deploy model validation**

File: `apps/core/models/controllers/v1beta1/deploy.go`

```go
// Before
Provider string `json:"provider" binding:"required,oneof=GITHUB"`

// After
Provider string `json:"provider" binding:"required,oneof=GITHUB GENERIC"`
```

**2. Verify service layer handles GENERIC**

File: `apps/core/services/platform/shared/deployments/create_deploy.go`

The service layer just copies the git metadata to the database model, so no changes should be needed. However, verify that:
- No switch statements on provider that would fail on GENERIC
- No provider-specific logic that assumes GITHUB

**3. Verify API response/UI handling**

Ensure any UI or API responses that display provider info handle GENERIC gracefully (e.g., don't show "GitHub" icon for GENERIC deploys).

### CLI Changes (astro-cli repo)

**1. Add provider detection function**

File: `pkg/git/git.go`

```go
// GetProviderFromHost returns the git provider based on the remote URL host
func GetProviderFromHost(remoteURL *url.URL) string {
    if remoteURL == nil {
        return ""
    }
    host := strings.ToLower(remoteURL.Host)
    if host == "github.com" || strings.HasSuffix(host, ".github.com") {
        return "GITHUB"
    }
    return "GENERIC"
}
```

**2. Update deploy.go to use detected provider**

File: `cloud/deploy/deploy.go`

```go
// Before
if deployGit != nil {
    createDeployRequest.Git = &astroplatformcore.CreateDeployGitRequest{
        Provider:   astroplatformcore.GITHUB,  // hardcoded
        // ...
    }
}

// After
if deployGit != nil {
    createDeployRequest.Git = &astroplatformcore.CreateDeployGitRequest{
        Provider:   astroplatformcore.CreateDeployGitRequestProvider(deployGit.Provider),
        // ...
    }
}
```

**3. Update bundle.go similarly**

File: `cloud/deploy/bundle.go`

Same pattern - use detected provider instead of hardcoding GITHUB.

**4. Update git metadata retrieval to include provider**

File: `cloud/deploy/deploy.go` (in `retrieveLocalGitMetadata`)

```go
// Add provider detection when parsing remote URL
remoteURL, err := git.GetRemoteRepository(path, "origin")
if err != nil {
    // ...
}
provider := git.GetProviderFromHost(remoteURL)
```

**5. Regenerate platform-core client**

After backend changes are merged, run `make generate` to get the new `GENERIC` enum value.

## Migration / Backwards Compatibility

- **No migration needed** - This is additive (new enum value)
- **Backwards compatible** - Existing GITHUB deploys continue to work
- **CLI version compatibility** - Older CLI versions will continue to send GITHUB only; newer CLI versions will send GITHUB or GENERIC based on remote

## Testing

### Backend
- Unit test: CreateDeploy with `Provider: "GENERIC"` succeeds
- Unit test: CreateDeploy with `Provider: "INVALID"` fails validation
- Integration test: Deploy with GENERIC provider appears correctly in API responses

### CLI
- Unit test: `GetProviderFromHost` returns correct provider for various hosts
- Unit test: Deploy from GitHub repo sends `GITHUB` provider
- Unit test: Deploy from non-GitHub repo sends `GENERIC` provider
- Manual test: Deploy from GitLab repo, verify git metadata appears in UI

## Rollout Order

1. **Backend first** - Merge PR adding GENERIC to allowed providers
2. **CLI second** - After backend is deployed, merge CLI changes that send GENERIC for non-GitHub repos

This order ensures the CLI doesn't send GENERIC before the backend can accept it.

## Open Questions

1. Should we track specific providers (GitLab, Bitbucket) for analytics purposes, even if we treat them the same as GENERIC functionally?
2. Are there any UI changes needed to display GENERIC provider deploys appropriately?
3. Should we add a `providerHost` field to store the actual host (e.g., `gitlab.com`) for GENERIC providers?
