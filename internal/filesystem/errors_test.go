package filesystem_test

import (
	"errors"
	"testing"

	"github.com/d31ma/sluice/internal/filesystem"
)

func TestFilesystemErrorsExposeCodesWithoutEchoingInput(t *testing.T) {
	t.Parallel()

	input := "/sensitive/root/private.txt"
	_, err := filesystem.ParseLogicalPath(input)
	if err == nil {
		t.Fatal("ParseLogicalPath() succeeded")
	}
	if got, want := err.Error(), string(filesystem.CodeAbsolutePath); got != want {
		t.Fatalf("error message = %q, want %q", got, want)
	}
	if got := filesystem.CodeOf(err); got != filesystem.CodeAbsolutePath {
		t.Fatalf("CodeOf() = %q, want %q", got, filesystem.CodeAbsolutePath)
	}
	if got := filesystem.CodeOf(errors.New("unclassified")); got != "" {
		t.Fatalf("CodeOf(unclassified) = %q, want empty", got)
	}
}
