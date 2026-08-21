# Sluice Remote Protocol v1

Remote protocol version 1 occupies this namespace beneath a profile's
canonical optional prefix:

```text
<prefix>/.sluice/v1/
  roots/<root-id>/descriptor.json
  manifests/<root-id>/<logical-path-sha256>.json
  objects/sha256/<first-two>/<content-sha256>
  tombstones/<root-id>/<logical-path-sha256>.json
```

The version is independent of the Sluice application version. Readers must
reject unknown `formatVersion` values without modifying local files or remote
objects.

## Canonical logical paths

Before hashing, a logical path is normalized to Unicode NFC and uses `/` as
its only separator; backslashes are rejected rather than rewritten. It is
root-relative, non-empty, and no longer than 4096 UTF-8
bytes. Every component is non-empty, at most 255 UTF-8 bytes, and cannot be
`.` or `..`. Control characters, Windows-reserved characters and device names,
and components ending in a space or period are rejected. These deliberately
portable rules prevent a remote writer from creating paths another desktop
cannot install safely.

`logicalPathHash` is lowercase hexadecimal SHA-256 over the exact UTF-8 bytes
of the canonical logical path. Manifest and tombstone readers must recompute
the hash and compare it in constant time with both the document field and the
object-key filename. JSON Schema validates the field shape; this semantic
comparison is mandatory application behavior.

Profile prefixes are either empty or already canonical: no leading or trailing
slash, empty segment, `.`/`..` segment, backslash, or control character.
Root/device/revision IDs use lowercase ASCII letters, digits, `_`, and `-`, are
at most 128 bytes, and begin and end with a letter or digit.

## Canonical documents and conditional writes

Documents use the JSON Canonicalization Scheme (RFC 8785). Producers emit UTF-8
without a BOM or insignificant whitespace; object member names are sorted by
the RFC 8785 rule; strings and numbers use its required serialization. Contract
fields containing paths are NFC before serialization. Duplicate member names,
non-integer schema numbers, unknown properties, invalid UTF-8, and non-canonical
documents are rejected. SHA-256 values are lowercase hexadecimal.

Content objects are immutable. A writer may treat an existing content object
as success only after its length and SHA-256 digest are verified. Manifests and
tombstones are mutable only via compare-and-swap using the exact last observed
ETag or provider version ID. Creation uses a provider-qualified create-only
precondition. An ambiguous write is read back and reconciled before retry.

Revision IDs are opaque. `parentRevisionIds` records causal ancestry; an empty
array is valid only for a root's first record. Listing discovers records but is
never the sole conflict clock. Tombstones carry the replaced manifest revision
and a retention deadline. Garbage collection must not delete content while any
retained manifest or tombstone can reference it.

The Go helpers in `internal/contracts/remotev1` are the executable object-key
and logical-path contract.
