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
