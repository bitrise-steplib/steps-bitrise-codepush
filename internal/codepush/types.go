// Package codepush ports the CodePush API interactions from bitrise-plugins-codepush-cli.
//
// This layer of the stacked PR sequence only ports what's needed for authentication and
// deployment resolution (ListDeployments, ResolveDeployment). The upload path (GetUploadURL,
// UploadFile, GetUpdateStatus, Push) lands in a later PR in the stack.
//
// This is a deliberate source copy from bitrise-plugins-codepush-cli @
// 4b586c72b61af87818445db251a60ee097b3f5bd (internal/codepush/*.go), not a module dependency; see
// internal/bundler/bundler.go for the full rationale, also documented in the project brief:
// https://bitrise.atlassian.net/wiki/spaces/RD/pages/5151653927
package codepush

import "context"

// Deployment represents a CodePush deployment.
type Deployment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DeploymentListResponse wraps the list deployments API response.
type DeploymentListResponse struct {
	Items []Deployment `json:"items"`
}

// Client defines the CodePush API operations this layer of the step needs.
type Client interface {
	ListDeployments(ctx context.Context, appID string) ([]Deployment, error)
}
