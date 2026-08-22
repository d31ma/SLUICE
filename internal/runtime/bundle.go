package runtimeexec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ErrNotPackaged means an executable is not running from a supported native
// package location. Development binaries use this signal to avoid starting
// production helpers accidentally.
var ErrNotPackaged = errors.New("Sluice is not running from a native application package") //nolint:staticcheck // Product name begins the sentinel message.

// Bundle contains absolute paths derived from the desktop executable's
// location. No PATH lookup or current-working-directory assumption is used.
type Bundle struct {
	Root             string
	DesktopBinary    string
	AgentBinary      string
	YonBinary        string
	TachyonBinary    string
	FyloBinary       string
	TachyonProject   string
	HealthRoute      string
	InfoPlist        string
	DesktopEntry     string
	PackageManifest  string
	LicenseDirectory string
}

// MacOSBundle is retained as a source-compatible name for the original native
// package contract. Lifecycle code uses Bundle because Linux AppDir packages
// have the same runtime boundary.
type MacOSBundle = Bundle

// OpenMacOSBundle resolves and validates the relocatable Sluice.app payload.
func OpenMacOSBundle(desktopExecutable string) (Bundle, error) {
	if _, ok := macOSBundleRoot(filepath.Clean(desktopExecutable)); !ok {
		return Bundle{}, ErrNotPackaged
	}
	desktop, err := OpenExecutable(desktopExecutable)
	if err != nil {
		return Bundle{}, fmt.Errorf("open Sluice desktop executable: %w", err)
	}
	root, ok := macOSBundleRoot(desktop.Path())
	if !ok {
		return Bundle{}, ErrNotPackaged
	}
	bundle := macOSBundlePaths(root)
	if err := validateBundle(bundle, "Info.plist", bundle.InfoPlist); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

// OpenLinuxBundle resolves and validates a relocatable Sluice.AppDir payload.
func OpenLinuxBundle(desktopExecutable string) (Bundle, error) {
	if _, ok := linuxBundleRoot(filepath.Clean(desktopExecutable)); !ok {
		return Bundle{}, ErrNotPackaged
	}
	desktop, err := OpenExecutable(desktopExecutable)
	if err != nil {
		return Bundle{}, fmt.Errorf("open Sluice desktop executable: %w", err)
	}
	root, ok := linuxBundleRoot(desktop.Path())
	if !ok {
		return Bundle{}, ErrNotPackaged
	}
	bundle := linuxBundlePaths(root)
	if err := validateBundle(bundle, "desktop entry", bundle.DesktopEntry); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

// OpenWindowsBundle resolves and validates a relocatable Sluice directory.
func OpenWindowsBundle(desktopExecutable string) (Bundle, error) {
	if _, ok := windowsBundleRoot(filepath.Clean(desktopExecutable)); !ok {
		return Bundle{}, ErrNotPackaged
	}
	desktop, err := OpenExecutable(desktopExecutable)
	if err != nil {
		return Bundle{}, fmt.Errorf("open Sluice desktop executable: %w", err)
	}
	root, ok := windowsBundleRoot(desktop.Path())
	if !ok {
		return Bundle{}, ErrNotPackaged
	}
	bundle := windowsBundlePaths(root)
	if err := validateBundle(bundle, "package manifest", bundle.PackageManifest); err != nil {
		return Bundle{}, err
	}
	return bundle, nil
}

func macOSBundleRoot(desktop string) (string, bool) {
	macOSDirectory := filepath.Dir(desktop)
	contentsDirectory := filepath.Dir(macOSDirectory)
	root := filepath.Dir(contentsDirectory)
	if filepath.Base(desktop) != "sluice-desktop" ||
		filepath.Base(macOSDirectory) != "MacOS" ||
		filepath.Base(contentsDirectory) != "Contents" ||
		filepath.Ext(root) != ".app" {
		return "", false
	}
	return root, true
}

func macOSBundlePaths(root string) Bundle {
	contents := filepath.Join(root, "Contents")
	resources := filepath.Join(contents, "Resources")
	project := filepath.Join(resources, "tachyon")
	return Bundle{
		Root:             root,
		DesktopBinary:    filepath.Join(contents, "MacOS", "sluice-desktop"),
		AgentBinary:      filepath.Join(contents, "Helpers", "sluice-agent"),
		YonBinary:        filepath.Join(contents, "Helpers", "sluice-yon"),
		TachyonBinary:    filepath.Join(resources, "runtime", "bin", "ty"),
		FyloBinary:       filepath.Join(resources, "runtime", "bin", "fylo"),
		TachyonProject:   project,
		HealthRoute:      filepath.Join(project, "server", "routes", "v1", "health", "yon.sluice"),
		InfoPlist:        filepath.Join(contents, "Info.plist"),
		LicenseDirectory: filepath.Join(resources, "licenses"),
	}
}

func linuxBundleRoot(desktop string) (string, bool) {
	root := filepath.Dir(desktop)
	if filepath.Base(desktop) != "AppRun" || filepath.Base(root) != "Sluice.AppDir" {
		return "", false
	}
	return root, true
}

func linuxBundlePaths(root string) Bundle {
	resources := filepath.Join(root, "usr", "share", "sluice")
	project := filepath.Join(resources, "tachyon")
	return Bundle{
		Root:             root,
		DesktopBinary:    filepath.Join(root, "AppRun"),
		AgentBinary:      filepath.Join(root, "usr", "libexec", "sluice", "sluice-agent"),
		YonBinary:        filepath.Join(root, "usr", "libexec", "sluice", "sluice-yon"),
		TachyonBinary:    filepath.Join(root, "usr", "lib", "sluice", "runtime", "bin", "ty"),
		FyloBinary:       filepath.Join(root, "usr", "lib", "sluice", "runtime", "bin", "fylo"),
		TachyonProject:   project,
		HealthRoute:      filepath.Join(project, "server", "routes", "v1", "health", "yon.sluice"),
		DesktopEntry:     filepath.Join(root, "sluice.desktop"),
		LicenseDirectory: filepath.Join(root, "usr", "share", "doc", "sluice", "licenses"),
	}
}

func windowsBundleRoot(desktop string) (string, bool) {
	root := filepath.Dir(desktop)
	if !strings.EqualFold(filepath.Base(desktop), "sluice-desktop.exe") ||
		!strings.EqualFold(filepath.Base(root), "Sluice") {
		return "", false
	}
	return root, true
}

func windowsBundlePaths(root string) Bundle {
	resources := filepath.Join(root, "resources")
	project := filepath.Join(resources, "tachyon")
	return Bundle{
		Root:             root,
		DesktopBinary:    filepath.Join(root, "sluice-desktop.exe"),
		AgentBinary:      filepath.Join(root, "libexec", "sluice-agent.exe"),
		YonBinary:        filepath.Join(root, "libexec", "sluice-yon.exe"),
		TachyonBinary:    filepath.Join(root, "runtime", "bin", "ty.exe"),
		FyloBinary:       filepath.Join(root, "runtime", "bin", "fylo.exe"),
		TachyonProject:   project,
		HealthRoute:      filepath.Join(project, "server", "routes", "v1", "health", "yon.sluice"),
		PackageManifest:  filepath.Join(root, "app-manifest.json"),
		LicenseDirectory: filepath.Join(root, "licenses"),
	}
}

func validateBundle(bundle Bundle, metadataName, metadataPath string) error {
	programs := []struct {
		name string
		path string
	}{
		{name: "agent", path: bundle.AgentBinary},
		{name: "Yon handler", path: bundle.YonBinary},
		{name: "Tachyon", path: bundle.TachyonBinary},
		{name: "Fylo", path: bundle.FyloBinary},
	}
	for _, program := range programs {
		if _, err := OpenExecutable(program.path); err != nil {
			return fmt.Errorf("open bundled %s: %w", program.name, err)
		}
	}
	for _, directory := range []struct {
		name string
		path string
	}{
		{name: "Tachyon project", path: bundle.TachyonProject},
		{name: "license directory", path: bundle.LicenseDirectory},
	} {
		if err := requireRealDirectory(directory.path); err != nil {
			return fmt.Errorf("open bundled %s: %w", directory.name, err)
		}
	}
	for _, file := range []struct {
		name string
		path string
	}{
		{name: "health route", path: bundle.HealthRoute},
		{name: metadataName, path: metadataPath},
	} {
		if err := requireRegularFile(file.path); err != nil {
			return fmt.Errorf("open bundled %s: %w", file.name, err)
		}
	}
	return nil
}

func requireRealDirectory(path string) error {
	metadata, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !metadata.IsDir() || metadata.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%q is not a real directory", path)
	}
	return nil
}

func requireRegularFile(path string) error {
	metadata, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !metadata.Mode().IsRegular() || metadata.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%q is not a regular file", path)
	}
	return nil
}
