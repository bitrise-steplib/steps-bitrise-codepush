// pollStatus respects ctx cancellation during its sleep interval, so a step timeout can interrupt
// a stuck poll loop promptly. uploadBundle does not delete the zip it builds; PushResult carries
// its path (PackagePath) since the step needs it to survive the push call to also export it under
// $BITRISE_DEPLOY_DIR (see step/step.go ExportOutputs).
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

func Push(ctx context.Context, client Client, opts *PushOptions, logger Logger) (*PushResult, error) {
	return PushWithConfig(ctx, client, opts, DefaultPollConfig, logger)
}

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
