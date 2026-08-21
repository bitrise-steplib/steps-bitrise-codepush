package bundler

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Logger is satisfied by github.com/bitrise-io/go-utils/v2/log.Logger.
type Logger interface {
	Infof(format string, args ...interface{})
	Debugf(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
}

// DefaultOutputDir is the default output directory for bundle generation.
// The name "CodePush" is required when code signing is used: the mobile SDK
// verifies package hashes using the directory name as a path prefix, so any
// other name produces a hash mismatch and the signed update is rejected.
const DefaultOutputDir = "./CodePush"

type BundleOptions struct {
	Platform         Platform
	EntryFile        string
	OutputDir        string
	BundleName       string
	Dev              bool
	Minify           bool // Expo only: pass --minify to expo export:embed
	ResetCache       bool // pass --reset-cache to the bundler (Metro/expo export:embed)
	HermesMode       HermesMode
	ExtraBundlerOpts []string
	ExtraHermesFlags []string
	ProjectDir       string
	MetroConfig      string
	SkipInstall      bool
	GradleFile       string // override path for android/app/build.gradle (Hermes auto-detection)
	PodFile          string // override path for ios/Podfile (Hermes auto-detection)
}

type BundleResult struct {
	BundlePath    string
	AssetsDir     string
	OutputDir     string
	HermesApplied bool
	ProjectType   ProjectType
	Platform      Platform
}

type Bundler interface {
	Bundle(config *ProjectConfig, opts *BundleOptions) (*BundleResult, error)
}

// CommandExecutor abstracts subprocess execution for testing.
type CommandExecutor interface {
	Run(dir string, stdout io.Writer, stderr io.Writer, name string, args ...string) error
}

type DefaultExecutor struct{}

func (e *DefaultExecutor) Run(dir string, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

func NewBundler(projectType ProjectType, executor CommandExecutor, logger Logger) (Bundler, error) {
	switch projectType {
	case ProjectTypeReactNative:
		return &ReactNativeBundler{executor: executor, logger: logger}, nil
	case ProjectTypeExpo:
		return &ExpoBundler{executor: executor, logger: logger}, nil
	default:
		return nil, fmt.Errorf("unsupported project type: %s", projectType)
	}
}

func DefaultBundleName(platform Platform) string {
	switch platform {
	case PlatformIOS:
		return "main.jsbundle"
	case PlatformAndroid:
		return "index.android.bundle"
	default:
		return "index.bundle"
	}
}

func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", path, err)
	}
	return nil
}
