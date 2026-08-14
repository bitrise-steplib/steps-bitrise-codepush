// Package bundler provides JavaScript bundle generation for React Native and Expo projects.
//
// Ported from github.com/bitrise-io/bitrise-plugins-codepush-cli @ 4b586c72b61af87818445db251a60ee097b3f5bd
// (internal/bundler/*.go). This is a deliberate source copy, not a module dependency: the CLI's
// internal/ package cannot be imported by another module, and the CLI's importable surface pulls in
// an interactive terminal-UI dependency stack (charmbracelet/huh, lipgloss) that has no place in a
// non-interactive CI step. See the project brief for the full rationale:
// https://bitrise.atlassian.net/wiki/spaces/RD/pages/5151653927
//
// Differences from the source: the interactive progress/spinner output (internal/output.Writer) and
// the PTY-backed bundler invocation (only used for TTY progress rendering) were dropped, since a CI
// step always runs non-interactively. Bundle signing (internal/bundler/signing.go) was intentionally
// not ported; it is Phase 2 scope per the project brief.
package bundler

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// Logger is the minimal logging surface bundler needs. Satisfied by
// github.com/bitrise-io/go-utils/v2/log.Logger.
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

// ValidatePlatform checks that the given platform string is valid.
func ValidatePlatform(p Platform) error {
	if p != PlatformIOS && p != PlatformAndroid {
		return fmt.Errorf("platform must be 'ios' or 'android', got %q", p)
	}
	return nil
}

// ValidateHermesMode checks that the given hermes mode string is valid.
func ValidateHermesMode(h HermesMode) error {
	if h != HermesModeAuto && h != HermesModeOn && h != HermesModeOff {
		return fmt.Errorf("hermes mode must be 'auto', 'on', or 'off', got %q", h)
	}
	return nil
}

// BundleOptions holds user-specified options for bundle generation.
type BundleOptions struct {
	Platform         Platform
	EntryFile        string
	OutputDir        string
	BundleName       string
	Dev              bool
	Minify           bool // Expo only: pass --minify to expo export:embed
	ResetCache       bool // pass --reset-cache to the bundler (Metro/expo export:embed)
	Sourcemap        bool
	SourcemapOutput  string // when set, overrides the auto-derived sourcemap path and implies Sourcemap=true
	HermesMode       HermesMode
	ExtraBundlerOpts []string
	ExtraHermesFlags []string
	ProjectDir       string
	MetroConfig      string
	SkipInstall      bool
	GradleFile       string // override path for android/app/build.gradle (Hermes auto-detection)
	PodFile          string // override path for ios/Podfile (Hermes auto-detection)
}

// BundleResult contains the output of a successful bundle operation.
type BundleResult struct {
	BundlePath    string
	AssetsDir     string
	SourcemapPath string
	OutputDir     string
	HermesApplied bool
	ProjectType   ProjectType
	Platform      Platform
}

// Bundler is the interface for building a JS bundle.
type Bundler interface {
	Bundle(config *ProjectConfig, opts *BundleOptions) (*BundleResult, error)
}

// CommandExecutor abstracts subprocess execution for testing.
type CommandExecutor interface {
	Run(dir string, stdout io.Writer, stderr io.Writer, name string, args ...string) error
}

// DefaultExecutor implements CommandExecutor using os/exec.
type DefaultExecutor struct{}

// Run executes a command with the given args in the given directory.
func (e *DefaultExecutor) Run(dir string, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

// NewBundler creates the appropriate Bundler implementation based on project type.
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

// DefaultBundleName returns the platform-specific default bundle filename.
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

// ensureDir creates a directory if it does not exist.
func ensureDir(path string) error {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return fmt.Errorf("creating directory %s: %w", path, err)
	}
	return nil
}
