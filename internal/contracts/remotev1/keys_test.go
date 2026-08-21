package remotev1_test

import (
	"testing"

	"github.com/d31ma/sluice/internal/contracts/remotev1"
)

const (
	rootID        = "01982fe2-1f5a-7b80-a1c2-123456789abc"
	contentDigest = "4d9673acb1cb0d6209f8014b33f1f530aa3ab0f22433da675b87765b2e03af31"
	pathDigest    = "64545dc8166d9ca0a725175d4a1f24a4b8d3908a0aa12453052af27681e4ac52"
)

func TestCanonicalLogicalPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "portable path", raw: "Documents/report.txt", want: "Documents/report.txt"},
		{name: "normalizes unicode", raw: "Cafe\u0301/menu.txt", want: "Caf\u00e9/menu.txt"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := remotev1.CanonicalLogicalPath(tc.raw)
			if err != nil {
				t.Fatalf("canonicalize: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestCanonicalLogicalPathRejectsUnsafeInputs(t *testing.T) {
	t.Parallel()

	paths := []string{
		"",
		"/absolute.txt",
		"\\server\\share\\absolute.txt",
		"Documents\\2026\\report.txt",
		"../escape.txt",
		"safe/../../escape.txt",
		"safe//empty.txt",
		"safe/./dot.txt",
		"CON.txt",
		"trailing-dot.",
		"trailing-space ",
		"colon:name.txt",
		"nul\x00byte",
	}
	for _, path := range paths {
		path := path
		t.Run(path, func(t *testing.T) {
			t.Parallel()
			if _, err := remotev1.CanonicalLogicalPath(path); err == nil {
				t.Fatalf("unsafe path %q accepted", path)
			}
		})
	}
}

func TestRemoteObjectKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		get  func() (string, error)
		want string
	}{
		{name: "descriptor", get: func() (string, error) { return remotev1.DescriptorKey("team/archive", rootID) }, want: "team/archive/.sluice/v1/roots/" + rootID + "/descriptor.json"},
		{name: "manifest", get: func() (string, error) { return remotev1.ManifestKey("team/archive", rootID, "Documents/report.txt") }, want: "team/archive/.sluice/v1/manifests/" + rootID + "/" + pathDigest + ".json"},
		{name: "content", get: func() (string, error) { return remotev1.ContentKey("", contentDigest) }, want: ".sluice/v1/objects/sha256/4d/" + contentDigest},
		{name: "tombstone", get: func() (string, error) { return remotev1.TombstoneKey("team/archive", rootID, "Documents/report.txt") }, want: "team/archive/.sluice/v1/tombstones/" + rootID + "/" + pathDigest + ".json"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := tc.get()
			if err != nil {
				t.Fatalf("build key: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRemoteObjectKeysRejectUnsafeComponents(t *testing.T) {
	t.Parallel()

	tests := []struct {
		get  func() (string, error)
		name string
	}{
		{name: "prefix traversal", get: func() (string, error) { return remotev1.ContentKey("../escape", contentDigest) }},
		{name: "uppercase root ID", get: func() (string, error) { return remotev1.DescriptorKey("", "ROOT") }},
		{name: "root ID trailing separator", get: func() (string, error) { return remotev1.DescriptorKey("", "root-") }},
		{name: "non-canonical prefix", get: func() (string, error) { return remotev1.ContentKey("Cafe\u0301", contentDigest) }},
		{name: "short content digest", get: func() (string, error) { return remotev1.ContentKey("", "abcd") }},
		{name: "unsafe logical path", get: func() (string, error) { return remotev1.ManifestKey("", rootID, "../escape") }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if _, err := tc.get(); err == nil {
				t.Fatal("unsafe key component accepted")
			}
		})
	}
}
