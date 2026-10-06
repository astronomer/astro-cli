// Package apitoken is what the three API token families (`astro deployment
// token`, `astro workspace token`, `astro organization token`) report: one
// token shape, the lists and removals built from it, and the picker contract
// through which each family asks a person which token they mean.
//
// The families live in packages that import each other (deployment imports
// workspace-token and organization), so the shape they share lives here, below
// all three. Nothing in this package prints, prompts or reads config.
package apitoken

import (
	"fmt"
	"time"

	"github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1"
)

// Token is an API token as a token command reports it. Role is the role the
// token holds on the object the command is about: the Deployment for the
// deployment family, the Workspace for the workspace family, the Organization
// for the organization family.
type Token struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Scope       string `json:"scope"`
	Role        string `json:"role,omitempty"`
	// CreatedBy is the creator's full name, or the name of the API token that
	// created it.
	CreatedAt time.Time  `json:"created_at"`
	CreatedBy string     `json:"created_by,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	// Token is the secret, set only by a create or a rotate: the one time the
	// API returns it, so the one time a caller can keep it.
	Token string `json:"token,omitempty"`
	// ExpiryPeriodInDays is the lifetime the token was created with, which
	// the Organization token picker shows. It is not published: expires_at
	// already says when the token stops working.
	ExpiryPeriodInDays *int `json:"-"`
}

// List is what a token list publishes. Tokens is empty, never nil, when there
// are none, and no token in it carries its secret.
type List struct {
	Tokens []Token `json:"tokens"`
}

// Action is what a delete or a remove did to a token.
type Action string

const (
	// Deleted: the token no longer exists. A family deletes only a token of
	// its own scope.
	Deleted Action = "deleted"
	// Removed: the token exists, without its role on the object the command
	// is about.
	Removed Action = "removed"
)

// DeploymentRemoval names the token a deployment-token delete or remove acted
// on, and what it did.
type DeploymentRemoval struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Scope        string `json:"scope"`
	DeploymentID string `json:"deployment_id"`
	Action       Action `json:"action"`
}

// WorkspaceRemoval names the token a workspace-token delete or remove acted
// on, and what it did.
type WorkspaceRemoval struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Scope       string `json:"scope"`
	WorkspaceID string `json:"workspace_id"`
	Action      Action `json:"action"`
}

// OrganizationRemoval names the token an organization-token delete acted on,
// and what it did, which is always Deleted: the Organization family acts only
// on Organization tokens.
type OrganizationRemoval struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Scope          string `json:"scope"`
	OrganizationID string `json:"organization_id"`
	Action         Action `json:"action"`
}

// Update is the token an update left, and the name it had before, which is
// the name the text confirmation reports.
type Update struct {
	Token        Token
	PreviousName string
}

// Role is one role a token holds, on one object.
type Role struct {
	EntityType string `json:"entity_type"`
	EntityID   string `json:"entity_id"`
	Role       string `json:"role"`
}

// RoleList is what `astro organization token roles` publishes: every role the
// token holds, empty and never nil when it holds none.
type RoleList struct {
	Roles []Role `json:"roles"`
}

// Picker asks a person to choose one of tokens and returns its index. A family
// calls it only when the command was given neither a token ID nor a name, with
// heading "", or when the name given is shared, with heading saying so. The
// picker asks the plain choice in its own words, because only the command
// knows what the token is wanted for.
type Picker func(heading string, tokens []Token) (int, error)

// Roles flattens a token's roles pointer to a usable slice.
func Roles(t *astrov1.ApiToken) []astrov1.ApiTokenRole {
	if t.Roles == nil {
		return nil
	}
	return *t.Roles
}

// RoleOn returns the token's role on the object of entityType with entityID,
// or "".
func RoleOn(t *astrov1.ApiToken, entityType astrov1.ApiTokenRoleEntityType, entityID string) string {
	for _, r := range Roles(t) {
		if r.EntityType == entityType && r.EntityId == entityID {
			return r.Role
		}
	}
	return ""
}

// WithRole returns roles with the entry for (entityType, entityID) set to
// role, added when missing. An empty role removes the entry instead. The
// other entries keep their order; the one set goes last.
func WithRole(roles []astrov1.ApiTokenRole, entityType astrov1.ApiTokenRoleEntityType, entityID, role string) []astrov1.ApiTokenRole {
	out := []astrov1.ApiTokenRole{}
	for _, r := range roles {
		if r.EntityType == entityType && r.EntityId == entityID {
			continue
		}
		out = append(out, r)
	}
	if role != "" {
		out = append(out, astrov1.ApiTokenRole{EntityType: entityType, EntityId: entityID, Role: role})
	}
	return out
}

// FromAPI reports t with role as its role, and no secret.
func FromAPI(t *astrov1.ApiToken, role string) Token {
	tok := Token{
		ID:                 t.Id,
		Name:               t.Name,
		Description:        t.Description,
		Scope:              string(t.Scope),
		Role:               role,
		CreatedAt:          t.CreatedAt,
		ExpiresAt:          t.EndAt,
		ExpiryPeriodInDays: t.ExpiryPeriodInDays,
	}
	if t.CreatedBy != nil {
		switch {
		case t.CreatedBy.FullName != nil:
			tok.CreatedBy = *t.CreatedBy.FullName
		case t.CreatedBy.ApiTokenName != nil:
			tok.CreatedBy = *t.CreatedBy.ApiTokenName
		}
	}
	return tok
}

// AllFromAPI reports tokens, each with the role roleOf reads from it. It is
// empty, never nil, when there are none.
func AllFromAPI(tokens []astrov1.ApiToken, roleOf func(*astrov1.ApiToken) string) []Token {
	out := make([]Token, 0, len(tokens))
	for i := range tokens {
		out = append(out, FromAPI(&tokens[i], roleOf(&tokens[i])))
	}
	return out
}

// WithSecret adds the secret the API returned with t, if it returned one.
func WithSecret(tok Token, t *astrov1.ApiToken) Token { //nolint:gocritic // a value in, a value out
	if t.Token != nil {
		tok.Token = *t.Token
	}
	return tok
}

// Pick has pick choose among tokens, offering each with the role roleOf
// reads from it.
func Pick(pick Picker, heading string, tokens []astrov1.ApiToken, roleOf func(*astrov1.ApiToken) string) (astrov1.ApiToken, error) {
	i, err := pick(heading, AllFromAPI(tokens, roleOf))
	if err != nil {
		return astrov1.ApiToken{}, err
	}
	if i < 0 || i >= len(tokens) {
		return astrov1.ApiToken{}, fmt.Errorf("token picker returned %d of %d tokens", i, len(tokens))
	}
	return tokens[i], nil
}

// TimeAgo says how long before now date was, the way the token tables show
// when a token was created.
func TimeAgo(date time.Time) string {
	duration := time.Since(date)
	days := int(duration.Hours() / 24) //nolint:mnd // the value is clear from context
	hours := int(duration.Hours())
	minutes := int(duration.Minutes())

	switch {
	case days > 0:
		return fmt.Sprintf("%d days ago", days)
	case hours > 0:
		return fmt.Sprintf("%d hours ago", hours)
	case minutes > 0:
		return fmt.Sprintf("%d minutes ago", minutes)
	default:
		return "Just now"
	}
}
