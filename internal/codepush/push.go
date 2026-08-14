// Ported from github.com/bitrise-io/bitrise-plugins-codepush-cli @ 4b586c72b61af87818445db251a60ee097b3f5bd
// (internal/codepush/push.go). The interactive output.Writer dependency (step spinners, progress
// bars) was replaced with plain Logger calls. Two behavioral additions on top of the source:
//  1. pollStatus now respects ctx cancellation during its sleep interval instead of an
//     unconditional time.Sleep, so a step timeout can interrupt a stuck poll loop promptly.
//  2. uploadBundle no longer deletes the zip it builds, and PushResult now carries its path
//     (PackagePath). The step needs the built package to survive the push call so it can also be
//     exported under $BITRISE_DEPLOY_DIR (see step/step.go ExportOutputs) — the CLI has no
//     equivalent requirement since it never runs inside a Bitrise build.
package codepush

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	ziputil "github.com/bitrise-steplib/steps-bitrise-codepush/internal/zip"
)

// Logger is the minimal logging surface this package needs. Satisfied by
// github.com/bitrise-io/go-utils/v2/log.Logger.
type Logger interface {
	Infof(format string, args ...interface{})
	Debugf(format string, args ...interface{})
}

// Push executes the full push workflow: zip, upload, and poll for completion.
func Push(ctx context.Context, client Client, opts *PushOptions, logger Logger) (*PushResult, error) {
	return PushWithConfig(ctx, client, opts, DefaultPollConfig, logger)
}

// PushWithConfig executes the push workflow with a configurable poll config.
func PushWithConfig(ctx context.Context, client Client, opts *PushOptions, pollCfg PollConfig, logger Logger) (*PushResult, error) {
	if err := validatePushOptions(opts); err != nil {
		return nil, err
	}

	deploymentID, err := ResolveDeployment(ctx, client, opts.AppID, opts.DeploymentID, logger)
	if err != nil {
		return nil, err
	}

	zipPath, updateID, fileSizeBytes, err := uploadBundle(ctx, client, opts, deploymentID, logger)
	if err != nil {
		return nil, err
	}

	logger.Infof("Processing update...")
	status, err := pollStatus(ctx, client, updateID, pollCfg)
	if err != nil {
		return nil, err
	}

	return &PushResult{
		UpdateID:      updateID,
		AppID:         opts.AppID,
		DeploymentID:  deploymentID,
		AppVersion:    opts.AppVersion,
		Status:        status.Status,
		FileSizeBytes: fileSizeBytes,
		Rollout:       opts.Rollout,
		PackagePath:   zipPath,
	}, nil
}

// uploadBundle zips opts.BundlePath, uploads it, and returns the zip path alongside the update ID
// and file size. Unlike the ported CLI version, the zip is intentionally NOT removed afterward —
// see the package-level doc comment.
func uploadBundle(ctx context.Context, client Client, opts *PushOptions, deploymentID string, logger Logger) (string, string, int64, error) {
	logger.Infof("Packaging bundle: %s", opts.BundlePath)
	zipPath, err := ziputil.Directory(opts.BundlePath)
	if err != nil {
		return "", "", 0, fmt.Errorf("packaging bundle: %w", err)
	}

	zipInfo, err := os.Stat(zipPath)
	if err != nil {
		return "", "", 0, fmt.Errorf("reading zip file info: %w", err)
	}
	logger.Debugf("Update size: %d bytes", zipInfo.Size())

	updateID := uuid.New().String()

	logger.Infof("Requesting upload URL")
	uploadResp, err := client.GetUploadURL(ctx, deploymentID, updateID, UploadURLRequest{
		AppVersion:    opts.AppVersion,
		FileName:      filepath.Base(zipPath),
		FileSizeBytes: zipInfo.Size(),
		Description:   opts.Description,
		Mandatory:     opts.Mandatory,
		Disabled:      opts.Disabled,
		Rollout:       &opts.Rollout,
	})
	if err != nil {
		return "", "", 0, fmt.Errorf("requesting upload URL: %w", err)
	}

	zipFile, err := os.Open(zipPath)
	if err != nil {
		return "", "", 0, fmt.Errorf("opening zip for upload: %w", err)
	}
	defer func() { _ = zipFile.Close() }()

	logger.Infof("Uploading update (%d bytes)", zipInfo.Size())
	uploadErr := client.UploadFile(ctx, UploadFileRequest{
		URL:           uploadResp.URL,
		Method:        uploadResp.Method,
		Headers:       uploadResp.Headers,
		Body:          zipFile,
		ContentLength: zipInfo.Size(),
	})
	if uploadErr != nil {
		return "", "", 0, fmt.Errorf("uploading update: %w", uploadErr)
	}

	return zipPath, updateID, zipInfo.Size(), nil
}

func validatePushOptions(opts *PushOptions) error {
	if opts.AppID == "" {
		return errors.New("app ID is required")
	}
	if opts.Token == "" {
		return errors.New("API token is required")
	}
	if opts.DeploymentID == "" {
		return errors.New("deployment is required")
	}
	if opts.AppVersion == "" {
		return errors.New("app version is required")
	}
	if opts.BundlePath == "" {
		return errors.New("bundle path is required")
	}
	if opts.Rollout < 0 || opts.Rollout > 100 {
		return fmt.Errorf("rollout must be between 0 and 100, got %g", opts.Rollout)
	}

	info, err := os.Stat(opts.BundlePath)
	if err != nil {
		return fmt.Errorf("bundle path does not exist: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("bundle path is not a directory: %s", opts.BundlePath)
	}

	return nil
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

// statusChecker is the subset of Client needed by pollStatus.
type statusChecker interface {
	GetUpdateStatus(ctx context.Context, updateID string) (*UpdateStatus, error)
}

func pollStatus(ctx context.Context, client statusChecker, updateID string, cfg PollConfig) (*UpdateStatus, error) {
	for attempt := range cfg.MaxAttempts {
		status, err := client.GetUpdateStatus(ctx, updateID)
		if err != nil {
			return nil, fmt.Errorf("checking update status: %w", err)
		}

		switch status.Status {
		case StatusProcessedValid:
			return status, nil
		case StatusProcessedError:
			return nil, fmt.Errorf("update processing failed: %s", status.StatusReason)
		}

		if attempt < cfg.MaxAttempts-1 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(cfg.Interval):
			}
		}
	}

	totalWait := time.Duration(cfg.MaxAttempts) * cfg.Interval
	return nil, fmt.Errorf("update processing timed out after %s", totalWait)
}
