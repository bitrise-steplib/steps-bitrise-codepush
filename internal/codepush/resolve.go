package codepush

import (
	"context"
	"fmt"

	"github.com/google/uuid"
)

// Logger is satisfied by github.com/bitrise-io/go-utils/v2/log.Logger.
type Logger interface {
	Infof(format string, args ...interface{})
	Debugf(format string, args ...interface{})
}

type deploymentLister interface {
	ListDeployments(ctx context.Context, appID string) ([]Deployment, error)
}

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
