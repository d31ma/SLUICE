package contracts_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

type fixtureCase struct {
	name    string
	schema  string
	fixture string
}

func TestContractSchemasAcceptValidFixtures(t *testing.T) {
	t.Parallel()

	for _, tc := range validFixtures() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			validateFixture(t, tc, true)
		})
	}
}

func TestContractSchemasRejectInvalidFixtures(t *testing.T) {
	t.Parallel()

	for _, tc := range invalidFixtures() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			validateFixture(t, tc, false)
		})
	}
}

func TestEveryContractSchemaCompiles(t *testing.T) {
	t.Parallel()

	patterns := []string{
		filepath.Join(repositoryRoot(t), "api", "local", "v1", "*.schema.json"),
		filepath.Join(repositoryRoot(t), "api", "remote", "v1", "*.schema.json"),
	}

	var schemas []string
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob schemas: %v", err)
		}
		schemas = append(schemas, matches...)
	}
	if len(schemas) == 0 {
		t.Fatal("no contract schemas found")
	}

	for _, schemaPath := range schemas {
		schemaPath := schemaPath
		t.Run(filepath.Base(schemaPath), func(t *testing.T) {
			t.Parallel()
			compiler := jsonschema.NewCompiler()
			if _, err := compiler.Compile(schemaPath); err != nil {
				t.Fatalf("compile %s: %v", schemaPath, err)
			}
		})
	}
}

func validateFixture(t *testing.T, tc fixtureCase, wantValid bool) {
	t.Helper()

	root := repositoryRoot(t)
	compiler := jsonschema.NewCompiler()
	schema, err := compiler.Compile(filepath.Join(root, tc.schema))
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}

	fixturePath := filepath.Join(root, tc.fixture)
	// #nosec G304 -- fixture paths are a closed table rooted in this repository.
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var fixture any
	if decodeErr := json.Unmarshal(data, &fixture); decodeErr != nil {
		t.Fatalf("decode fixture: %v", decodeErr)
	}
	err = schema.Validate(fixture)
	if wantValid && err != nil {
		t.Fatalf("valid fixture %s rejected: %v", fixturePath, err)
	}
	if !wantValid && err == nil {
		t.Fatalf("invalid fixture %s accepted", fixturePath)
	}
}

func repositoryRoot(t *testing.T) string {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate contract test source")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func validFixtures() []fixtureCase {
	return []fixtureCase{
		{name: "request metadata", schema: "api/local/v1/request-metadata.schema.json", fixture: "api/local/v1/fixtures/valid/request-metadata.json"},
		{name: "problem", schema: "api/local/v1/problem.schema.json", fixture: "api/local/v1/fixtures/valid/problem.json"},
		{name: "health", schema: "api/local/v1/health.schema.json", fixture: "api/local/v1/fixtures/valid/health.json"},
		{name: "version", schema: "api/local/v1/version.schema.json", fixture: "api/local/v1/fixtures/valid/version.json"},
		{name: "status", schema: "api/local/v1/status.schema.json", fixture: "api/local/v1/fixtures/valid/status.json"},
		{name: "profile create", schema: "api/local/v1/profile-create.schema.json", fixture: "api/local/v1/fixtures/valid/profile-create.json"},
		{name: "profile probe", schema: "api/local/v1/profile-probe.schema.json", fixture: "api/local/v1/fixtures/valid/profile-probe.json"},
		{name: "profiles", schema: "api/local/v1/profiles.schema.json", fixture: "api/local/v1/fixtures/valid/profiles.json"},
		{name: "root create", schema: "api/local/v1/root-create.schema.json", fixture: "api/local/v1/fixtures/valid/root-create.json"},
		{name: "root action", schema: "api/local/v1/root-action.schema.json", fixture: "api/local/v1/fixtures/valid/root-action.json"},
		{name: "roots", schema: "api/local/v1/roots.schema.json", fixture: "api/local/v1/fixtures/valid/roots.json"},
		{name: "operations", schema: "api/local/v1/operations.schema.json", fixture: "api/local/v1/fixtures/valid/operations.json"},
		{name: "conflicts", schema: "api/local/v1/conflicts.schema.json", fixture: "api/local/v1/fixtures/valid/conflicts.json"},
		{name: "conflict resolution", schema: "api/local/v1/conflict-resolution.schema.json", fixture: "api/local/v1/fixtures/valid/conflict-resolution.json"},
		{name: "diagnostic export", schema: "api/local/v1/diagnostic-export.schema.json", fixture: "api/local/v1/fixtures/valid/diagnostic-export.json"},
		{name: "descriptor", schema: "api/remote/v1/descriptor.schema.json", fixture: "api/remote/v1/fixtures/valid/descriptor.json"},
		{name: "manifest", schema: "api/remote/v1/manifest.schema.json", fixture: "api/remote/v1/fixtures/valid/manifest.json"},
		{name: "tombstone", schema: "api/remote/v1/tombstone.schema.json", fixture: "api/remote/v1/fixtures/valid/tombstone.json"},
	}
}

func invalidFixtures() []fixtureCase {
	return []fixtureCase{
		{name: "missing bearer token", schema: "api/local/v1/request-metadata.schema.json", fixture: "api/local/v1/fixtures/invalid/request-metadata-missing-auth.json"},
		{name: "missing idempotency key", schema: "api/local/v1/request-metadata.schema.json", fixture: "api/local/v1/fixtures/invalid/request-metadata-missing-idempotency.json"},
		{name: "unredacted problem detail", schema: "api/local/v1/problem.schema.json", fixture: "api/local/v1/fixtures/invalid/problem-sensitive-detail.json"},
		{name: "unknown local API version", schema: "api/local/v1/version.schema.json", fixture: "api/local/v1/fixtures/invalid/version-unknown-api.json"},
		{name: "inline profile secret", schema: "api/local/v1/profile-create.schema.json", fixture: "api/local/v1/fixtures/invalid/profile-create-inline-secret.json"},
		{name: "absolute root logical path", schema: "api/local/v1/root-create.schema.json", fixture: "api/local/v1/fixtures/invalid/root-create-relative-path.json"},
		{name: "timestamp conflict winner", schema: "api/local/v1/conflict-resolution.schema.json", fixture: "api/local/v1/fixtures/invalid/conflict-resolution-latest.json"},
		{name: "unbounded operations page", schema: "api/local/v1/operations.schema.json", fixture: "api/local/v1/fixtures/invalid/operations-missing-page.json"},
		{name: "unknown descriptor version", schema: "api/remote/v1/descriptor.schema.json", fixture: "api/remote/v1/fixtures/invalid/descriptor-version.json"},
		{name: "invalid manifest path hash", schema: "api/remote/v1/manifest.schema.json", fixture: "api/remote/v1/fixtures/invalid/manifest-path-hash.json"},
		{name: "manifest mutable object key", schema: "api/remote/v1/manifest.schema.json", fixture: "api/remote/v1/fixtures/invalid/manifest-content-hash.json"},
		{name: "missing tombstone retention", schema: "api/remote/v1/tombstone.schema.json", fixture: "api/remote/v1/fixtures/invalid/tombstone-retention.json"},
	}
}
