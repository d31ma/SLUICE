package filesystem_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/d31ma/sluice/internal/filesystem"
)

func TestParseLogicalPathAcceptsCanonicalPortablePaths(t *testing.T) {
	t.Parallel()

	tests := []string{
		"file.txt",
		"folder/file.txt",
		"photos/2026/ete.png",
		"unicode/café.txt",
	}

	for _, input := range tests {
		input := input
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			path, err := filesystem.ParseLogicalPath(input)
			if err != nil {
				t.Fatalf("ParseLogicalPath(%q) error = %v", input, err)
			}
			if got := path.String(); got != input {
				t.Fatalf("String() = %q, want %q", got, input)
			}
		})
	}
}

func TestParseLogicalPathRejectsAmbiguousOrUnsafePaths(t *testing.T) {
	t.Parallel()

	tests := map[string]filesystem.ErrorCode{
		"":                             filesystem.CodeEmptyPath,
		"/etc/passwd":                  filesystem.CodeAbsolutePath,
		`\server\share`:                filesystem.CodeAbsolutePath,
		"C:/Windows/system32":          filesystem.CodeAbsolutePath,
		"C:drive-relative":             filesystem.CodeAbsolutePath,
		"a\\b":                         filesystem.CodeInvalidSeparator,
		"a//b":                         filesystem.CodeEmptySegment,
		"a/./b":                        filesystem.CodeDotSegment,
		"a/../b":                       filesystem.CodeDotSegment,
		"a/line\nbreak":                filesystem.CodeControlCharacter,
		string([]byte{'a', '/', 0xff}): filesystem.CodeInvalidUTF8,
		"a/name:stream":                filesystem.CodeForbiddenCharacter,
		"a/name?.txt":                  filesystem.CodeForbiddenCharacter,
		"a/trailing.":                  filesystem.CodeTrailingDotOrSpace,
		"a/trailing" + " ":             filesystem.CodeTrailingDotOrSpace,
		"CON":                          filesystem.CodeReservedName,
		"con.txt":                      filesystem.CodeReservedName,
		"logs/LPT9.log":                filesystem.CodeReservedName,
		"devices/AuX":                  filesystem.CodeReservedName,
		"devices/COM\u00b9.txt":        filesystem.CodeReservedName,
	}

	for input, wantCode := range tests {
		input, wantCode := input, wantCode
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			_, err := filesystem.ParseLogicalPath(input)
			if err == nil {
				t.Fatalf("ParseLogicalPath(%q) succeeded", input)
			}
			if got := filesystem.CodeOf(err); got != wantCode {
				t.Fatalf("CodeOf(error) = %q, want %q (error: %v)", got, wantCode, err)
			}
		})
	}
}

func TestCanonicalizeRootProducesAnAbsoluteDirectory(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	root, err := filesystem.CanonicalizeRoot(directory)
	if err != nil {
		t.Fatalf("CanonicalizeRoot() error = %v", err)
	}
	if !filepath.IsAbs(root.Path()) {
		t.Fatalf("root path %q is not absolute", root.Path())
	}

	logical, err := filesystem.ParseLogicalPath("folder/file.txt")
	if err != nil {
		t.Fatalf("ParseLogicalPath() error = %v", err)
	}
	want := filepath.Join(root.Path(), "folder", "file.txt")
	if got := root.Resolve(logical); got != want {
		t.Fatalf("Resolve() = %q, want %q", got, want)
	}
}

func TestCanonicalizeRootAcceptsRelativeConfigurationPaths(t *testing.T) {
	t.Parallel()

	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	directory, err := os.MkdirTemp(workingDirectory, ".sluice-relative-root-test-")
	if err != nil {
		t.Fatalf("MkdirTemp() error = %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	relative, err := filepath.Rel(workingDirectory, directory)
	if err != nil {
		t.Fatalf("Rel() error = %v", err)
	}

	root, err := filesystem.CanonicalizeRoot(relative)
	if err != nil {
		t.Fatalf("CanonicalizeRoot() error = %v", err)
	}
	want, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	if root.Path() != want {
		t.Fatalf("root path = %q, want %q", root.Path(), want)
	}
}

func TestCanonicalizeRootResolvesTheConfiguredRootOnce(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	link := filepath.Join(t.TempDir(), "root-link")
	if err := os.Symlink(directory, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	root, err := filesystem.CanonicalizeRoot(link)
	if err != nil {
		t.Fatalf("CanonicalizeRoot() error = %v", err)
	}
	want, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatalf("EvalSymlinks() error = %v", err)
	}
	if root.Path() != want {
		t.Fatalf("root path = %q, want %q", root.Path(), want)
	}
}

func TestCanonicalizeRootRejectsMissingAndNonDirectoryRoots(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	file := filepath.Join(directory, "file")
	if err := os.WriteFile(file, []byte("content"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	for _, input := range []string{"", filepath.Join(directory, "missing"), file} {
		_, err := filesystem.CanonicalizeRoot(input)
		if !errors.Is(err, filesystem.ErrInvalidRoot) {
			t.Fatalf("CanonicalizeRoot(%q) error = %v, want ErrInvalidRoot", input, err)
		}
	}
}
