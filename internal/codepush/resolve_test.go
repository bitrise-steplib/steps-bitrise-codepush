// Adapted from github.com/bitrise-io/bitrise-plugins-codepush-cli @ 4b586c72b61af87818445db251a60ee097b3f5bd
// (internal/codepush/push_test.go, TestResolveDeployment specifically).
package codepush

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDeployment(t *testing.T) {
	t.Run("UUID passthrough", func(t *testing.T) {
		client := &mockClient{}
		id, err := ResolveDeployment(context.Background(), client, "app-123", "00000000-0000-0000-0000-000000000001", testOut)
		require.NoError(t, err)
		assert.Equal(t, "00000000-0000-0000-0000-000000000001", id)
	})

	t.Run("name resolution", func(t *testing.T) {
		client := &mockClient{
			listDeploymentsFunc: func(appID string) ([]Deployment, error) {
				return []Deployment{
					{ID: "dep-aaa", Name: "Staging"},
					{ID: "dep-bbb", Name: "Production"},
				}, nil
			},
		}

		id, err := ResolveDeployment(context.Background(), client, "app-123", "Production", testOut)
		require.NoError(t, err)
		assert.Equal(t, "dep-bbb", id)
	})

	t.Run("name not found", func(t *testing.T) {
		client := &mockClient{
			listDeploymentsFunc: func(appID string) ([]Deployment, error) {
				return []Deployment{{ID: "dep-aaa", Name: "Staging"}}, nil
			},
		}

		_, err := ResolveDeployment(context.Background(), client, "app-123", "Production", testOut)
		require.Error(t, err)
		assert.ErrorContains(t, err, "Production")
	})

	t.Run("list deployments error", func(t *testing.T) {
		client := &mockClient{
			listDeploymentsFunc: func(appID string) ([]Deployment, error) {
				return nil, errors.New("network error")
			},
		}

		_, err := ResolveDeployment(context.Background(), client, "app-123", "Production", testOut)
		require.Error(t, err)
	})
}
