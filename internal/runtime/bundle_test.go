package runtimeexec_test

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	runtimeexec "github.com/d31ma/sluice/internal/runtime"
)

func TestOpenMacOSBundleResolvesEveryPackagedRuntimeFromDesktopExecutable(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Relocated Sluice.app")
	paths := map[string]os.FileMode{
		"Contents/MacOS/sluice-desktop":                                 0o755,
		"Contents/Helpers/sluice-agent":                                 0o755,
		"Contents/Helpers/sluice-yon":                                   0o755,
		"Contents/Resources/runtime/bin/ty":                             0o755,
		"Contents/Resources/runtime/bin/fylo":                           0o755,
		"Contents/Resources/tachyon/server/routes/v1/health/yon.sluice": 0o600,
		"Contents/Info.plist":                                           0o600,
	}
	for relative, mode := range paths {
		writeBundleFixture(t, root, relative, mode)
	}
	if err := os.MkdirAll(filepath.Join(root, "Contents/Resources/licenses"), 0o700); err != nil {
		t.Fatalf("create license fixture: %v", err)
	}

	bundle, err := runtimeexec.OpenMacOSBundle(filepath.Join(root, "Contents/MacOS/sluice-desktop"))
	if err != nil {
		t.Fatalf("OpenMacOSBundle() error = %v", err)
	}
	canonicalRoot := filepath.Clean(root)
	if runtime.GOOS != "windows" {
		canonicalRoot, err = filepath.EvalSymlinks(root)
		if err != nil {
			t.Fatalf("canonicalize fixture: %v", err)
		}
	}
	want := runtimeexec.MacOSBundle{
		Root:             canonicalRoot,
		DesktopBinary:    filepath.Join(canonicalRoot, "Contents/MacOS/sluice-desktop"),
		AgentBinary:      filepath.Join(canonicalRoot, "Contents/Helpers/sluice-agent"),
		YonBinary:        filepath.Join(canonicalRoot, "Contents/Helpers/sluice-yon"),
		TachyonBinary:    filepath.Join(canonicalRoot, "Contents/Resources/runtime/bin/ty"),
		FyloBinary:       filepath.Join(canonicalRoot, "Contents/Resources/runtime/bin/fylo"),
		TachyonProject:   filepath.Join(canonicalRoot, "Contents/Resources/tachyon"),
		HealthRoute:      filepath.Join(canonicalRoot, "Contents/Resources/tachyon/server/routes/v1/health/yon.sluice"),
		InfoPlist:        filepath.Join(canonicalRoot, "Contents/Info.plist"),
		LicenseDirectory: filepath.Join(canonicalRoot, "Contents/Resources/licenses"),
	}
	if bundle != want {
		t.Fatalf("bundle = %#v, want %#v", bundle, want)
	}
}

func TestOpenMacOSBundleRejectsDevelopmentAndIncompleteLayouts(t *testing.T) {
	if _, err := runtimeexec.OpenMacOSBundle(filepath.Join(t.TempDir(), "sluice-desktop")); !errors.Is(err, runtimeexec.ErrNotPackaged) {
		t.Fatalf("development path error = %v, want ErrNotPackaged", err)
	}

	root := filepath.Join(t.TempDir(), "Sluice.app")
	desktop := "Contents/MacOS/sluice-desktop"
	writeBundleFixture(t, root, desktop, 0o755)
	if _, err := runtimeexec.OpenMacOSBundle(filepath.Join(root, desktop)); err == nil || errors.Is(err, runtimeexec.ErrNotPackaged) {
		t.Fatalf("incomplete bundle error = %v, want packaged-layout failure", err)
	}
}

func TestOpenLinuxBundleResolvesEveryPackagedRuntimeFromAppRun(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Sluice.AppDir")
	paths := map[string]os.FileMode{
		"AppRun":                          0o755,
		"usr/libexec/sluice/sluice-agent": 0o755,
		"usr/libexec/sluice/sluice-yon":   0o755,
		"usr/lib/sluice/runtime/bin/ty":   0o755,
		"usr/lib/sluice/runtime/bin/fylo": 0o755,
		"usr/share/sluice/tachyon/server/routes/v1/health/yon.sluice": 0o600,
		"sluice.desktop": 0o600,
	}
	for relative, mode := range paths {
		writeBundleFixture(t, root, relative, mode)
	}
	if err := os.MkdirAll(filepath.Join(root, "usr/share/doc/sluice/licenses"), 0o700); err != nil {
		t.Fatalf("create license fixture: %v", err)
	}

	bundle, err := runtimeexec.OpenLinuxBundle(filepath.Join(root, "AppRun"))
	if err != nil {
		t.Fatalf("OpenLinuxBundle() error = %v", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("canonicalize fixture: %v", err)
	}
	want := runtimeexec.Bundle{
		Root:             canonicalRoot,
		DesktopBinary:    filepath.Join(canonicalRoot, "AppRun"),
		AgentBinary:      filepath.Join(canonicalRoot, "usr/libexec/sluice/sluice-agent"),
		YonBinary:        filepath.Join(canonicalRoot, "usr/libexec/sluice/sluice-yon"),
		TachyonBinary:    filepath.Join(canonicalRoot, "usr/lib/sluice/runtime/bin/ty"),
		FyloBinary:       filepath.Join(canonicalRoot, "usr/lib/sluice/runtime/bin/fylo"),
		TachyonProject:   filepath.Join(canonicalRoot, "usr/share/sluice/tachyon"),
		HealthRoute:      filepath.Join(canonicalRoot, "usr/share/sluice/tachyon/server/routes/v1/health/yon.sluice"),
		DesktopEntry:     filepath.Join(canonicalRoot, "sluice.desktop"),
		LicenseDirectory: filepath.Join(canonicalRoot, "usr/share/doc/sluice/licenses"),
	}
	if bundle != want {
		t.Fatalf("bundle = %#v, want %#v", bundle, want)
	}
}

func TestOpenLinuxBundleRejectsDevelopmentAndIncompleteLayouts(t *testing.T) {
	if _, err := runtimeexec.OpenLinuxBundle(filepath.Join(t.TempDir(), "AppRun")); !errors.Is(err, runtimeexec.ErrNotPackaged) {
		t.Fatalf("development path error = %v, want ErrNotPackaged", err)
	}

	root := filepath.Join(t.TempDir(), "Sluice.AppDir")
	writeBundleFixture(t, root, "AppRun", 0o755)
	if _, err := runtimeexec.OpenLinuxBundle(filepath.Join(root, "AppRun")); err == nil || errors.Is(err, runtimeexec.ErrNotPackaged) {
		t.Fatalf("incomplete bundle error = %v, want packaged-layout failure", err)
	}
}

func TestOpenWindowsBundleResolvesEveryPackagedRuntime(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Sluice")
	paths := map[string]os.FileMode{
		"sluice-desktop.exe":                                   0o755,
		"libexec/sluice-agent.exe":                             0o755,
		"libexec/sluice-yon.exe":                               0o755,
		"runtime/bin/ty.exe":                                   0o755,
		"runtime/bin/fylo.exe":                                 0o755,
		"resources/tachyon/server/routes/v1/health/yon.sluice": 0o600,
		"app-manifest.json":                                    0o600,
	}
	for relative, mode := range paths {
		writeBundleFixture(t, root, relative, mode)
	}
	if err := os.MkdirAll(filepath.Join(root, "licenses"), 0o700); err != nil {
		t.Fatalf("create license fixture: %v", err)
	}

	bundle, err := runtimeexec.OpenWindowsBundle(filepath.Join(root, "sluice-desktop.exe"))
	if err != nil {
		t.Fatalf("OpenWindowsBundle() error = %v", err)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("canonicalize fixture: %v", err)
	}
	want := runtimeexec.Bundle{
		Root:             canonicalRoot,
		DesktopBinary:    filepath.Join(canonicalRoot, "sluice-desktop.exe"),
		AgentBinary:      filepath.Join(canonicalRoot, "libexec/sluice-agent.exe"),
		YonBinary:        filepath.Join(canonicalRoot, "libexec/sluice-yon.exe"),
		TachyonBinary:    filepath.Join(canonicalRoot, "runtime/bin/ty.exe"),
		FyloBinary:       filepath.Join(canonicalRoot, "runtime/bin/fylo.exe"),
		TachyonProject:   filepath.Join(canonicalRoot, "resources/tachyon"),
		HealthRoute:      filepath.Join(canonicalRoot, "resources/tachyon/server/routes/v1/health/yon.sluice"),
		PackageManifest:  filepath.Join(canonicalRoot, "app-manifest.json"),
		LicenseDirectory: filepath.Join(canonicalRoot, "licenses"),
	}
	if bundle != want {
		t.Fatalf("bundle = %#v, want %#v", bundle, want)
	}
}

func TestOpenWindowsBundleRejectsDevelopmentAndIncompleteLayouts(t *testing.T) {
	if _, err := runtimeexec.OpenWindowsBundle(filepath.Join(t.TempDir(), "sluice-desktop.exe")); !errors.Is(err, runtimeexec.ErrNotPackaged) {
		t.Fatalf("development path error = %v, want ErrNotPackaged", err)
	}

	root := filepath.Join(t.TempDir(), "Sluice")
	writeBundleFixture(t, root, "sluice-desktop.exe", 0o755)
	if _, err := runtimeexec.OpenWindowsBundle(filepath.Join(root, "sluice-desktop.exe")); err == nil || errors.Is(err, runtimeexec.ErrNotPackaged) {
		t.Fatalf("incomplete bundle error = %v, want packaged-layout failure", err)
	}
}

func writeBundleFixture(t *testing.T, root, relative string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte("fixture"), mode); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if runtime.GOOS != "windows" && mode&0o111 != 0 {
		if err := os.Chmod(path, mode); err != nil {
			t.Fatalf("make fixture executable: %v", err)
		}
	}
}
