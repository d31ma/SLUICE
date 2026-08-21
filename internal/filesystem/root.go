package filesystem

import (
	"errors"
	"os"
	"path/filepath"
)

// Root is an existing directory resolved to an absolute physical path.
type Root struct {
	path string
}

// CanonicalizeRoot resolves a configured root once. Logical paths beneath the
// resulting root are still checked component-by-component before mutation.
func CanonicalizeRoot(input string) (Root, error) {
	if input == "" {
		return Root{}, failure(CodeInvalidRoot, ErrInvalidRoot)
	}
	absolute, err := filepath.Abs(input)
	if err != nil {
		return Root{}, failure(CodeInvalidRoot, errorsJoin(ErrInvalidRoot, err))
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return Root{}, failure(CodeInvalidRoot, errorsJoin(ErrInvalidRoot, err))
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return Root{}, failure(CodeInvalidRoot, errorsJoin(ErrInvalidRoot, err))
	}
	return Root{path: filepath.Clean(resolved)}, nil
}

// Path returns the canonical physical root. Callers must treat it as sensitive.
func (root Root) Path() string {
	return root.path
}

// Resolve converts a validated logical path to its host representation.
func (root Root) Resolve(path LogicalPath) string {
	return filepath.Join(root.path, filepath.FromSlash(path.value))
}

func errorsJoin(primary, cause error) error {
	if cause == nil {
		return primary
	}
	return errors.Join(primary, cause)
}
