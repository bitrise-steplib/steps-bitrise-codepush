// Ported from github.com/bitrise-io/bitrise-plugins-codepush-cli @ 4b586c72b61af87818445db251a60ee097b3f5bd
// (internal/codepush/push.go, the ResolveDeployment function specifically — the rest of that
// file, the upload/poll orchestration, lands with the Client interface additions it needs in a
// later PR in the stack). The interactive output.Writer dependency was replaced with the
// package-local Logger interface.
package codepush

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Logger is the minimal logging surface this package needs. Satisfied by
// github.com/bitrise-io/go-utils/v2/log.Logger.
type Logger interface {
	Infof(format string, args ...interface{})
	Debugf(format string, args ...interface{})
}

// deploymentLister is the subset of Client needed by ResolveDeployment.
type deploymentLister interface {
	ListDeployments(ctx context.Context, appID string) ([]Deployment, error)
}

// ResolveDeployment resolves a deployment name or UUID to a deployment ID.
// If the input is already a valid UUID, it is returned as-is.
// Otherwise, it lists all deployments and finds the one matching by name.
func ResolveDeployment(ctx context.Context, client deploymentLister, appID, deploymentNameOrID string, logger Logger) (string, error) {
	if _, err := uuid.Parse(deploymentNameOrID); err == nil {
		return deploymentNameOrID, nil
	}

	logger.Infof("Resolving deployment %q", deploymentNameOrID)
	deployments, err := client.ListDeployments(ctx, appID)
	if err != nil {
		return "", fmt.Errorf("listing deployments: %w", err)
	}

	for _, d := range deployments {
		if d.Name == deploymentNameOrID {
			logger.Debugf("Resolved to %s", d.ID)
			return d.ID, nil
		}
	}

	return "", fmt.Errorf("deployment %q not found: check the deployment name or use a deployment UUID", deploymentNameOrID)
}
