package runtimeexec

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrNotPackaged means an executable is not running from the supported macOS
// application-bundle location. Development binaries use this signal to avoid
// starting production helpers accidentally.
var ErrNotPackaged = errors.New("Sluice is not running from a macOS application bundle") //nolint:staticcheck // Product name begins the sentinel message.

// MacOSBundle contains absolute paths derived from the desktop executable's
// location. No PATH lookup or current-working-directory assumption is used.
type MacOSBundle struct {
	Root             string
	DesktopBinary    string
	AgentBinary      string
	YonBinary        string
	TachyonBinary    string
	FyloBinary       string
	TachyonProject   string
	HealthRoute      string
	InfoPlist        string
	LicenseDirectory string
}

// OpenMacOSBundle resolves and validates the relocatable Sluice.app payload.
func OpenMacOSBundle(desktopExecutable string) (MacOSBundle, error) {
	if _, ok := macOSBundleRoot(filepath.Clean(desktopExecutable)); !ok {
		return MacOSBundle{}, ErrNotPackaged
	}
	desktop, err := OpenExecutable(desktopExecutable)
	if err != nil {
		return MacOSBundle{}, fmt.Errorf("open Sluice desktop executable: %w", err)
	}
	root, ok := macOSBundleRoot(desktop.Path())
	if !ok {
		return MacOSBundle{}, ErrNotPackaged
	}
	bundle := macOSBundlePaths(root)
	if err := validateMacOSBundle(bundle); err != nil {
		return MacOSBundle{}, err
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

func macOSBundlePaths(root string) MacOSBundle {
	contents := filepath.Join(root, "Contents")
	resources := filepath.Join(contents, "Resources")
	project := filepath.Join(resources, "tachyon")
	return MacOSBundle{
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

func validateMacOSBundle(bundle MacOSBundle) error {
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
		{name: "Info.plist", path: bundle.InfoPlist},
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
