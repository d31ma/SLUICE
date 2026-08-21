package runtimeexec_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	runtimeexec "github.com/d31ma/sluice/internal/runtime"
)

func TestMain(m *testing.M) {
	if os.Getenv("SLUICE_RUNTIME_HELPER") == "1" {
		helperMain()
		return
	}
	os.Exit(m.Run())
}

func helperMain() {
	if os.Getenv("SLUICE_RUNTIME_HELPER_EXIT") == "1" {
		os.Exit(7)
	}
	separator := -1
	for index, argument := range os.Args {
		if argument == "--" {
			separator = index
			break
		}
	}
	if separator < 0 {
		os.Exit(2)
	}
	if err := json.NewEncoder(os.Stdout).Encode(os.Args[separator+1:]); err != nil {
		os.Exit(3)
	}
}

func TestExecutableOutputIsBoundedAndReportsProcessFailure(t *testing.T) {
	t.Parallel()

	path, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	executable, err := runtimeexec.OpenExecutable(path)
	if err != nil {
		t.Fatalf("open executable: %v", err)
	}
	arguments := []string{"-test.run=^$", "--", "bounded"}

	output, err := executable.Output(
		context.Background(),
		runtimeexec.Environment{"SLUICE_RUNTIME_HELPER=1"},
		1024,
		arguments...,
	)
	if err != nil {
		t.Fatalf("Output() error = %v", err)
	}
	if string(output) != "[\"bounded\"]\n" {
		t.Fatalf("Output() = %q, want bounded helper JSON", output)
	}

	if _, err := executable.Output(context.Background(), nil, 0); err == nil {
		t.Fatal("Output() accepted a zero-byte limit")
	}
	if _, err := executable.Output(
		context.Background(),
		runtimeexec.Environment{"SLUICE_RUNTIME_HELPER=1"},
		1,
		arguments...,
	); err == nil {
		t.Fatal("Output() accepted output larger than its limit")
	}
	if _, err := executable.Output(
		context.Background(),
		runtimeexec.Environment{"SLUICE_RUNTIME_HELPER=1", "SLUICE_RUNTIME_HELPER_EXIT=1"},
		1024,
		arguments...,
	); err == nil {
		t.Fatal("Output() ignored a failed child process")
	}
}

func TestExecutablePassesArgumentsWithoutShellInterpretation(t *testing.T) {
	t.Parallel()

	path, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test executable: %v", err)
	}
	executable, err := runtimeexec.OpenExecutable(path)
	if err != nil {
		t.Fatalf("open executable: %v", err)
	}

	marker := filepath.Join(t.TempDir(), "must-not-exist")
	arguments := []string{"literal;touch " + marker, "$(touch " + marker + ")", "space kept"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := executable.Command(
		ctx,
		runtimeexec.Environment{"SLUICE_RUNTIME_HELPER=1"},
		append([]string{"-test.run=^$", "--"}, arguments...)...,
	)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("run helper: %v", err)
	}
	var received []string
	if err := json.Unmarshal(output, &received); err != nil {
		t.Fatalf("decode helper output %q: %v", output, err)
	}
	if !reflect.DeepEqual(received, arguments) {
		t.Fatalf("arguments changed: got %#v, want %#v", received, arguments)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("shell-like argument was interpreted; marker stat error: %v", err)
	}
}

func TestExecutableRequiresAnAbsoluteRegularProgram(t *testing.T) {
	t.Parallel()

	for _, path := range []string{"", "fylo", "."} {
		path := path
		t.Run(strings.ReplaceAll(fmt.Sprintf("%q", path), "/", "_"), func(t *testing.T) {
			t.Parallel()
			if _, err := runtimeexec.OpenExecutable(path); err == nil {
				t.Fatalf("OpenExecutable(%q) succeeded", path)
			}
		})
	}
}

func TestAllowlistedEnvironmentDoesNotInheritUnlistedValues(t *testing.T) {
	t.Setenv("SLUICE_RUNTIME_SECRET", "must-not-be-inherited")
	environment := runtimeexec.AllowEnvironment("PATH", "SLUICE_RUNTIME_SECRET_DOES_NOT_EXIST")
	for _, entry := range environment {
		if strings.HasPrefix(entry, "SLUICE_RUNTIME_SECRET=") {
			t.Fatalf("unlisted environment value leaked: %q", entry)
		}
	}
}
