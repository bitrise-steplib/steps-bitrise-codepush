// Adapted from github.com/bitrise-io/bitrise-plugins-codepush-cli @ 4b586c72b61af87818445db251a60ee097b3f5bd
// (internal/codepush/testhelpers_test.go). mockClient is trimmed to the one Client method this
// layer of the stack keeps (ListDeployments); the upload-path mock hooks land with
// GetUploadURL/UploadFile/GetUpdateStatus in a later PR. testOut (output.NewTest) was replaced
// with a no-op Logger test double.
package codepush

import "context"

type mockClient struct {
	listDeploymentsFunc func(appID string) ([]Deployment, error)
}

func (m *mockClient) ListDeployments(_ context.Context, appID string) ([]Deployment, error) {
	if m.listDeploymentsFunc != nil {
		return m.listDeploymentsFunc(appID)
	}
	return nil, nil
}

// testLogger is a no-op Logger test double, standing in for the CLI's
// output.NewTest(io.Discard) (this package uses the local Logger interface; see resolve.go).
type testLogger struct{}

func (l *testLogger) Infof(format string, args ...interface{})  {}
func (l *testLogger) Debugf(format string, args ...interface{}) {}

var testOut = &testLogger{}
