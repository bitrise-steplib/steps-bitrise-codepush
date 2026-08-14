// Ported from github.com/bitrise-io/bitrise-plugins-codepush-cli @ 4b586c72b61af87818445db251a60ee097b3f5bd
// (internal/bundler/detect.go), unchanged apart from this notice.
package bundler

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// ProjectType represents the detected project type.
type ProjectType int

const (
	// ProjectTypeUnknown indicates the project type could not be detected.
	ProjectTypeUnknown ProjectType = iota
	// ProjectTypeReactNative indicates a bare React Native project.
	ProjectTypeReactNative
	// ProjectTypeExpo indicates an Expo-managed project.
	ProjectTypeExpo
)

// String returns the display name of the project type.
func (p ProjectType) String() string {
	switch p {
	case ProjectTypeReactNative:
		return "react-native"
	case ProjectTypeExpo:
		return "expo"
	default:
		return "unknown"
	}
}

// Platform represents the target mobile platform.
type Platform string

const (
	// PlatformIOS targets iOS devices.
	PlatformIOS Platform = "ios"
	// PlatformAndroid targets Android devices.
	PlatformAndroid Platform = "android"
)

// HermesMode represents the Hermes override setting.
type HermesMode string

const (
	// HermesModeAuto detects Hermes configuration from the project.
	HermesModeAuto HermesMode = "auto"
	// HermesModeOn forces Hermes compilation.
	HermesModeOn HermesMode = "on"
	// HermesModeOff disables Hermes compilation.
	HermesModeOff HermesMode = "off"
)

// ProjectConfig holds the auto-detected project configuration.
type ProjectConfig struct {
	ProjectDir    string
	ProjectType   ProjectType
	Platform      Platform
	EntryFile     string
	MetroConfig   string
	HermesEnabled bool
	HermescPath   string
	BundleName    string // expected filename the SDK will search for (Expo only)
}

// packageJSON represents the relevant fields of a package.json file.
type packageJSON struct {
	Main            string            `json:"main"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
}

// DetectProject inspects the project directory and returns a ProjectConfig.
// opts may be nil; when provided, GradleFile and PodFile override the default
// paths used for Hermes auto-detection.
func DetectProject(projectDir string, platform Platform, hermesMode HermesMode, opts *BundleOptions) (*ProjectConfig, error) {
	absDir, err := filepath.Abs(projectDir)
	if err != nil {
		return nil, fmt.Errorf("resolving project directory: %w", err)
	}

	if _, err := os.Stat(absDir); err != nil {
		return nil, fmt.Errorf("project directory does not exist: %w", err)
	}

	projectType, err := detectProjectType(absDir)
	if err != nil {
		return nil, err
	}

	entryFile, err := detectEntryFile(absDir, platform)
	if err != nil {
		return nil, err
	}

	var gradleFile, podFile string
	if opts != nil {
		gradleFile = opts.GradleFile
		podFile = opts.PodFile
	}

	var hermesEnabled bool
	hermescPath := ""

	switch hermesMode {
	case HermesModeOn:
		hermesEnabled = true
	case HermesModeOff:
		hermesEnabled = false
	default:
		hermesEnabled = detectHermes(absDir, platform, gradleFile, podFile)
	}

	if hermesEnabled {
		hermescPath, _ = findHermesc(absDir)
	}

	metroConfig := detectMetroConfig(absDir)

	bundleName := ""
	if projectType == ProjectTypeExpo {
		bundleName = detectBundleName(absDir, platform, opts)
	}

	return &ProjectConfig{
		ProjectDir:    absDir,
		ProjectType:   projectType,
		Platform:      platform,
		EntryFile:     entryFile,
		MetroConfig:   metroConfig,
		HermesEnabled: hermesEnabled,
		HermescPath:   hermescPath,
		BundleName:    bundleName,
	}, nil
}

// detectProjectType reads package.json and determines the project type.
func detectProjectType(projectDir string) (ProjectType, error) {
	pkgPath := filepath.Join(projectDir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return ProjectTypeUnknown, fmt.Errorf("no package.json found in %s: is this a React Native or Expo project?", projectDir)
	}

	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return ProjectTypeUnknown, fmt.Errorf("parsing package.json: %w", err)
	}

	// Check for Expo first (Expo projects also have react-native as a dependency)
	if _, ok := pkg.Dependencies["expo"]; ok {
		return ProjectTypeExpo, nil
	}
	if _, ok := pkg.DevDependencies["expo"]; ok {
		return ProjectTypeExpo, nil
	}

	if _, ok := pkg.Dependencies["react-native"]; ok {
		return ProjectTypeReactNative, nil
	}
	if _, ok := pkg.DevDependencies["react-native"]; ok {
		return ProjectTypeReactNative, nil
	}

	return ProjectTypeUnknown, errors.New("could not detect project type: package.json does not list react-native or expo as a dependency")
}

// detectEntryFile searches for the JS entry file.
// Priority: index.<platform>.js, then index.js, then package.json "main" field.
func detectEntryFile(projectDir string, platform Platform) (string, error) {
	platformSpecific := fmt.Sprintf("index.%s.js", platform)
	candidates := []string{platformSpecific, "index.js"}

	for _, candidate := range candidates {
		path := filepath.Join(projectDir, candidate)
		if _, err := os.Stat(path); err == nil {
			return candidate, nil
		}
	}

	// Fall back to package.json "main" field
	pkgPath := filepath.Join(projectDir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err == nil {
		var pkg packageJSON
		if err := json.Unmarshal(data, &pkg); err == nil && pkg.Main != "" {
			mainPath := filepath.Join(projectDir, pkg.Main)
			if _, err := os.Stat(mainPath); err == nil {
				return pkg.Main, nil
			}
		}
	}

	return "", fmt.Errorf("entry file not found: tried %s and index.js in %s", platformSpecific, projectDir)
}

// hermesDetection represents the result of scanning build files for Hermes config.
type hermesDetection int

const (
	hermesNotFound hermesDetection = iota // no explicit config found
	hermesEnabled                         // explicitly enabled
	hermesDisabled                        // explicitly disabled
)

// detectHermes checks the project for Hermes configuration.
// If no explicit config is found, defaults to true for React Native >= 0.70
// (Hermes became the default engine in that version).
// gradleFile and podFile override the default build file paths for detection.
func detectHermes(projectDir string, platform Platform, gradleFile, podFile string) bool {
	var detection hermesDetection

	switch platform {
	case PlatformAndroid:
		detection = detectHermesAndroid(projectDir, gradleFile)
	case PlatformIOS:
		detection = detectHermesIOS(projectDir, podFile)
	default:
		return false
	}

	switch detection {
	case hermesEnabled:
		return true
	case hermesDisabled:
		return false
	default:
		// No explicit config found: check if RN >= 0.70 where Hermes is the default
		return isHermesDefaultVersion(projectDir)
	}
}

// isHermesDefaultVersion checks if the react-native version in package.json
// is >= 0.70, where Hermes became the default JS engine.
func isHermesDefaultVersion(projectDir string) bool {
	pkgPath := filepath.Join(projectDir, "package.json")
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return false
	}

	var pkg packageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return false
	}

	rnVersion := pkg.Dependencies["react-native"]
	if rnVersion == "" {
		rnVersion = pkg.DevDependencies["react-native"]
	}
	if rnVersion == "" {
		return false
	}

	return parseRNMajorMinor(rnVersion) >= 70
}

// parseRNMajorMinor extracts the minor version number from a React Native
// version string. Returns the minor version (e.g., 72 for "0.72.0") or 0
// if parsing fails. Handles semver ranges like "^0.72.0", "~0.72.0", ">=0.72.0".
func parseRNMajorMinor(version string) int {
	// Strip common semver prefixes
	v := strings.TrimLeft(version, "^~>=<! ")

	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return 0
	}

	// React Native uses 0.XX.Y format; the meaningful version is the minor
	major := 0
	minor := 0
	if _, err := fmt.Sscanf(parts[0], "%d", &major); err != nil {
		return 0
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &minor); err != nil {
		return 0
	}

	// For RN 0.x, the minor is the "effective major"
	if major == 0 {
		return minor
	}

	// Future-proofing: if RN ever goes to 1.x+, treat as >= 0.70
	return 100
}

// detectHermesAndroid checks android/app/build.gradle for Hermes configuration.
// If gradleFile is non-empty it is used instead of the default paths.
func detectHermesAndroid(projectDir string, gradleFile string) hermesDetection {
	var gradlePaths []string
	if gradleFile != "" {
		if !filepath.IsAbs(gradleFile) {
			gradleFile = filepath.Join(projectDir, gradleFile)
		}
		gradlePaths = []string{gradleFile}
	} else {
		gradlePaths = []string{
			filepath.Join(projectDir, "android", "app", "build.gradle"),
			filepath.Join(projectDir, "android", "app", "build.gradle.kts"),
		}
	}

	for _, gradlePath := range gradlePaths {
		data, err := os.ReadFile(gradlePath)
		if err != nil {
			continue
		}

		content := string(data)
		// Check for various Hermes configuration patterns across RN versions
		enablePatterns := []string{
			"hermesEnabled = true",
			"hermesEnabled.set(true)",
			"enableHermes: true",
			"enableHermes = true",
		}
		for _, pattern := range enablePatterns {
			if strings.Contains(content, pattern) {
				return hermesEnabled
			}
		}

		// Check for explicit disable
		disablePatterns := []string{
			"hermesEnabled = false",
			"hermesEnabled.set(false)",
			"enableHermes: false",
			"enableHermes = false",
		}
		for _, pattern := range disablePatterns {
			if strings.Contains(content, pattern) {
				return hermesDisabled
			}
		}
	}

	return hermesNotFound
}

// detectHermesIOS checks ios/Podfile for Hermes configuration.
// If podFile is non-empty it is used instead of the default path.
func detectHermesIOS(projectDir string, podFile string) hermesDetection {
	podfilePath := podFile
	if podfilePath == "" {
		podfilePath = filepath.Join(projectDir, "ios", "Podfile")
	} else if !filepath.IsAbs(podfilePath) {
		podfilePath = filepath.Join(projectDir, podfilePath)
	}
	data, err := os.ReadFile(podfilePath)
	if err != nil {
		return hermesNotFound
	}

	content := string(data)
	enablePatterns := []string{
		":hermes_enabled => true",
		"hermes_enabled: true",
	}
	for _, pattern := range enablePatterns {
		if strings.Contains(content, pattern) {
			return hermesEnabled
		}
	}

	disablePatterns := []string{
		":hermes_enabled => false",
		"hermes_enabled: false",
	}
	for _, pattern := range disablePatterns {
		if strings.Contains(content, pattern) {
			return hermesDisabled
		}
	}

	return hermesNotFound
}

// findHermesc locates the hermesc binary in node_modules.
func findHermesc(projectDir string) (string, error) {
	osName := runtime.GOOS
	archName := runtime.GOARCH

	// Map Go OS/arch names to hermesc directory and binary name conventions.
	var osTriplet string
	binaryName := "hermesc"
	switch {
	case osName == "darwin":
		osTriplet = "osx-bin"
	case osName == "linux" && archName == "amd64":
		osTriplet = "linux64-bin"
	case osName == "windows":
		osTriplet = "windows-bin"
		binaryName = "hermesc.exe"
	default:
		osTriplet = osName + "-bin"
	}

	// Check known hermesc locations in order of preference.
	candidates := []string{
		filepath.Join(projectDir, "node_modules", "hermes-engine", osTriplet, binaryName),
		filepath.Join(projectDir, "node_modules", "react-native", "sdks", "hermesc", osTriplet, binaryName),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", errors.New("hermesc binary not found in node_modules")
}

// detectMetroConfig searches for metro.config.js or metro.config.ts.
func detectMetroConfig(projectDir string) string {
	candidates := []string{
		filepath.Join(projectDir, "metro.config.js"),
		filepath.Join(projectDir, "metro.config.ts"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}

	return ""
}

// Bundle name detection regexes. Compiled once at package init to avoid repeated allocation.
var (
	reBundleAssetName      = regexp.MustCompile(`bundleAssetName\s*=\s*"([^"]+)"`)
	reBundleURLForResObjC  = regexp.MustCompile(`bundleURLForResource:\s*@"([^"]+)"`)
	reBundleURLForResSwift = regexp.MustCompile(`bundleURL\(forResource:\s*"([^"]+)"`)
)

// detectBundleName returns the expected bundle filename from native project files.
// Always returns a non-empty string; falls back to DefaultBundleName(platform).
func detectBundleName(projectDir string, platform Platform, opts *BundleOptions) string {
	var name string
	switch platform {
	case PlatformAndroid:
		name = detectBundleNameAndroid(projectDir, opts)
	case PlatformIOS:
		name = detectBundleNameIOS(projectDir)
	}
	if name == "" {
		return DefaultBundleName(platform)
	}
	return name
}

// detectBundleNameAndroid scans android/app/build.gradle (and build.gradle.kts)
// for bundleAssetName. Respects opts.GradleFile override (same as Hermes detection).
func detectBundleNameAndroid(projectDir string, opts *BundleOptions) string {
	var gradlePaths []string
	if opts != nil && opts.GradleFile != "" {
		p := opts.GradleFile
		if !filepath.IsAbs(p) {
			p = filepath.Join(projectDir, p)
		}
		gradlePaths = []string{p}
	} else {
		gradlePaths = []string{
			filepath.Join(projectDir, "android", "app", "build.gradle"),
			filepath.Join(projectDir, "android", "app", "build.gradle.kts"),
		}
	}
	for _, gradlePath := range gradlePaths {
		data, err := os.ReadFile(gradlePath)
		if err != nil {
			continue
		}
		if m := reBundleAssetName.FindStringSubmatch(string(data)); len(m) >= 2 {
			return m[1]
		}
	}
	return ""
}

// detectBundleNameIOS scans ios/AppDelegate.{m,mm,swift} for a custom bundle name.
// Returns "" if no custom name is found; caller falls back to DefaultBundleName.
func detectBundleNameIOS(projectDir string) string {
	candidates := []string{
		filepath.Join(projectDir, "ios", "AppDelegate.m"),
		filepath.Join(projectDir, "ios", "AppDelegate.mm"),
		filepath.Join(projectDir, "ios", "AppDelegate.swift"),
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		for _, re := range []*regexp.Regexp{reBundleURLForResObjC, reBundleURLForResSwift} {
			if m := re.FindStringSubmatch(content); len(m) >= 2 {
				return m[1] + ".jsbundle"
			}
		}
	}
	return ""
}
