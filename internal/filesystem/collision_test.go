package filesystem_test

import (
	"testing"

	"github.com/d31ma/sluice/internal/filesystem"
)

func TestPortableCollisionKeyFindsCaseAndNormalizationAliases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		left  string
		right string
	}{
		{name: "ASCII case", left: "Documents/Report.txt", right: "documents/report.TXT"},
		{name: "Unicode normalization", left: "Caf\u00e9/menu.txt", right: "Cafe\u0301/menu.txt"},
		{name: "Unicode case fold", left: "Stra\u00dfe/map.txt", right: "STRASSE/map.txt"},
		{name: "Greek final sigma", left: "\u039f\u0394\u039f\u03a3/map.txt", right: "\u03bf\u03b4\u03bf\u03c2/map.txt"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			left := parseLogicalPath(t, test.left)
			right := parseLogicalPath(t, test.right)
			leftKey := filesystem.PortableCollisionKey(left)
			rightKey := filesystem.PortableCollisionKey(right)
			if leftKey != rightKey {
				t.Fatalf("collision keys differ: %q != %q", leftKey, rightKey)
			}
			if left.String() == right.String() {
				t.Fatal("fixture does not preserve two distinct user names")
			}
		})
	}
}

func TestPortableCollisionKeyKeepsDistinctPathsDistinct(t *testing.T) {
	t.Parallel()

	left := parseLogicalPath(t, "photos/one.jpg")
	right := parseLogicalPath(t, "photos/two.jpg")
	if filesystem.PortableCollisionKey(left) == filesystem.PortableCollisionKey(right) {
		t.Fatal("distinct paths share a collision key")
	}
}

func parseLogicalPath(t *testing.T, value string) filesystem.LogicalPath {
	t.Helper()
	path, err := filesystem.ParseLogicalPath(value)
	if err != nil {
		t.Fatalf("ParseLogicalPath(%q) error = %v", value, err)
	}
	return path
}
