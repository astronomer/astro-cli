package ide

import (
	"time"

	astrov1alpha1 "github.com/astronomer/astro-cli/internal/platform/astro/clients/astrov1alpha1"
)

// Project is one Astro IDE project, as `astro ide project list` publishes it.
type Project struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description,omitempty"`
	WorkspaceID    string    `json:"workspace_id"`
	OrganizationID string    `json:"organization_id"`
	URL            string    `json:"url,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// ProjectList is the Workspace's Astro IDE projects, by name.
type ProjectList struct {
	Projects []Project `json:"projects"`
}

// Import is what `astro ide project import` did: which project, from which
// session, into which directory, and how much it wrote there.
type Import struct {
	ProjectID string `json:"project_id"`
	// ProjectName is "" when the project could not be read back after the
	// import; text names it by ID then.
	ProjectName string `json:"project_name,omitempty"`
	SessionID   string `json:"session_id"`
	// Directory is the absolute path imported into.
	Directory string `json:"directory"`
	// Files and Bytes count the regular files written.
	Files  int    `json:"files"`
	Bytes  int64  `json:"bytes"`
	Action string `json:"action"`
}

// Export is what `astro ide project export` did: which project, whether the
// run created it, from which directory, and how much it uploaded.
type Export struct {
	ProjectID string `json:"project_id"`
	// ProjectName is "" when the project could not be read back after the
	// export; text names it by ID then.
	ProjectName    string `json:"project_name,omitempty"`
	ProjectCreated bool   `json:"project_created"`
	// URL opens the project in the Astro IDE, when the API gave one.
	URL string `json:"url,omitempty"`
	// Directory is the absolute path exported from.
	Directory string `json:"directory"`
	// Files and Bytes count the regular files uploaded, after .gitignore.
	Files  int    `json:"files"`
	Bytes  int64  `json:"bytes"`
	Action string `json:"action"`
}

const (
	ActionImported = "imported"
	ActionExported = "exported"
)

func projectOf(p *astrov1alpha1.AstroIdeProject) Project {
	out := Project{
		ID:             p.Id,
		Name:           p.Name,
		WorkspaceID:    p.WorkspaceId,
		OrganizationID: p.OrganizationId,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
	if p.Description != nil {
		out.Description = *p.Description
	}
	if p.Url != nil {
		out.URL = *p.Url
	}
	return out
}

// archiveStats counts the regular files an archive carried and their bytes.
type archiveStats struct {
	files int
	bytes int64
}
