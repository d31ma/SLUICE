// Package runtimeexec starts trusted runtime binaries with explicit argument
// vectors and child-scoped environments. It deliberately has no shell API.
package runtimeexec

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"sort"
	"strings"
)

// Environment is the complete environment supplied to a child process.
// An empty value means the child inherits nothing.
type Environment []string

// Executable is a validated absolute path to a regular program.
type Executable struct {
	path string
}

// OpenExecutable validates an explicit program path. PATH lookup belongs at
// installation/configuration time, outside the privileged runtime boundary.
func OpenExecutable(path string) (Executable, error) {
	if path == "" || strings.ContainsRune(path, '\x00') || !filepath.IsAbs(path) {
		return Executable{}, fmt.Errorf("runtime executable path must be absolute")
	}
	clean, err := canonicalExecutablePath(path)
	if err != nil {
		return Executable{}, fmt.Errorf("resolve runtime executable %q: %w", path, err)
	}
	metadata, err := os.Stat(clean)
	if err != nil {
		return Executable{}, fmt.Errorf("inspect runtime executable %q: %w", clean, err)
	}
	if !metadata.Mode().IsRegular() {
		return Executable{}, fmt.Errorf("runtime executable %q is not a regular file", clean)
	}
	if goruntime.GOOS != "windows" && metadata.Mode().Perm()&0o111 == 0 {
		return Executable{}, fmt.Errorf("runtime executable %q is not executable", clean)
	}
	return Executable{path: clean}, nil
}

// Output runs a short-lived command while retaining at most limit bytes from
// stdout and discarding diagnostic sideband output.
func (executable Executable) Output(
	ctx context.Context,
	environment Environment,
	limit int,
	arguments ...string,
) ([]byte, error) {
	if limit < 1 {
		return nil, fmt.Errorf("runtime output limit must be positive")
	}
	retained := &boundedOutput{limit: limit}
	command := executable.Command(ctx, environment, arguments...)
	command.Stdout = retained
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return nil, err
	}
	if retained.overflow {
		return nil, fmt.Errorf("runtime output exceeds %d bytes", limit)
	}
	return bytes.Clone(retained.data), nil
}

// Path returns the validated absolute program path.
func (executable Executable) Path() string {
	return executable.path
}

// Command constructs a direct child process. Arguments are never parsed by a
// shell and environment is always child-scoped.
func (executable Executable) Command(
	ctx context.Context,
	environment Environment,
	arguments ...string,
) *exec.Cmd {
	// #nosec G204 -- OpenExecutable validated an explicit absolute regular program;
	// arguments remain separate operating-system values and never enter a shell.
	command := exec.CommandContext(ctx, executable.path, arguments...)
	command.Env = make([]string, len(environment))
	copy(command.Env, environment)
	return command
}

// AllowEnvironment copies only named variables from the parent process.
func AllowEnvironment(names ...string) Environment {
	allowed := make(map[string]struct{}, len(names))
	for _, name := range names {
		allowed[canonicalEnvironmentName(name)] = struct{}{}
	}
	environment := make(Environment, 0, len(names))
	for _, entry := range os.Environ() {
		separator := strings.IndexByte(entry, '=')
		if separator < 1 {
			continue
		}
		if _, ok := allowed[canonicalEnvironmentName(entry[:separator])]; ok {
			environment = append(environment, entry)
		}
	}
	sort.Strings(environment)
	return environment
}

func canonicalEnvironmentName(name string) string {
	if goruntime.GOOS == "windows" {
		return strings.ToUpper(name)
	}
	return name
}

type boundedOutput struct {
	data     []byte
	limit    int
	overflow bool
}

func (output *boundedOutput) Write(data []byte) (int, error) {
	remaining := output.limit - len(output.data)
	if remaining > 0 {
		retained := min(remaining, len(data))
		output.data = append(output.data, data[:retained]...)
	}
	output.overflow = output.overflow || len(data) > remaining
	return len(data), nil
}
