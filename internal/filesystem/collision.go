package filesystem

import (
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// PortableCollisionKey returns the conservative identity used to detect names
// that cannot coexist across Sluice's desktop targets. It never replaces the
// original LogicalPath: callers retain both names and surface a conflict when
// distinct paths share this key.
func PortableCollisionKey(path LogicalPath) string {
	return cases.Fold().String(norm.NFC.String(path.value))
}
