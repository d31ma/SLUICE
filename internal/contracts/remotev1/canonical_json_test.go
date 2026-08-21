package remotev1_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/d31ma/sluice/internal/contracts/remotev1"
)

func TestCanonicalizeJSON(t *testing.T) {
	t.Parallel()

	input := []byte(" { \"z\": [3, true, null], \"a\": \"line\\nvalue\", \"n\": -42 } \n")
	want := []byte(`{"a":"line\nvalue","n":-42,"z":[3,true,null]}`)

	got, err := remotev1.CanonicalizeJSON(input)
	if err != nil {
		t.Fatalf("canonicalize JSON: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestValidRemoteFixturesAreCanonicalJSON(t *testing.T) {
	t.Parallel()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	pattern := filepath.Join(filepath.Dir(filename), "..", "..", "..", "api", "remote", "v1", "fixtures", "valid", "*.json")
	fixtures, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("glob valid remote fixtures: %v", err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no valid remote fixtures found")
	}

	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(filepath.Base(fixture), func(t *testing.T) {
			t.Parallel()
			// #nosec G304 -- glob results are confined to the repository fixture directory.
			document, err := os.ReadFile(fixture)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			payload := bytes.TrimSuffix(document, []byte{'\n'})
			canonical, err := remotev1.CanonicalizeJSON(payload)
			if err != nil {
				t.Fatalf("canonicalize fixture: %v", err)
			}
			if !bytes.Equal(payload, canonical) {
				t.Fatalf("fixture is not canonical; canonical form is %s", canonical)
			}
		})
	}
}

func TestCanonicalizeJSONUsesUTF16PropertyOrder(t *testing.T) {
	t.Parallel()

	// RFC 8785 sorts member names by UTF-16 code units. A surrogate pair for
	// U+1F600 sorts before U+E000 even though its Unicode code point is larger.
	input := []byte("{\"\\ue000\":1,\"\U0001f600\":2}")
	want := []byte("{\"\U0001f600\":2,\"\ue000\":1}")

	got, err := remotev1.CanonicalizeJSON(input)
	if err != nil {
		t.Fatalf("canonicalize JSON: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestCanonicalizeJSONEscapesEveryRequiredStringCharacter(t *testing.T) {
	t.Parallel()

	input := []byte("{\"value\":\"quote: \\\" slash: \\\\ controls: \\b\\t\\n\\f\\r\\u0001 emoji: \\ud83d\\ude00\"}")
	want := []byte("{\"value\":\"quote: \\\" slash: \\\\ controls: \\b\\t\\n\\f\\r\\u0001 emoji: \U0001f600\"}")

	got, err := remotev1.CanonicalizeJSON(input)
	if err != nil {
		t.Fatalf("canonicalize JSON: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestCanonicalizeJSONRejectsNonContractJSON(t *testing.T) {
	t.Parallel()

	inputs := [][]byte{
		[]byte(`{"duplicate":1,"duplicate":2}`),
		[]byte(`{"float":1.5}`),
		[]byte(`{"unsafeInteger":9007199254740992}`),
		[]byte(`{"unpairedSurrogate":"\ud800"}`),
		[]byte(`{"unpairedSurrogate":"\udc00"}`),
		[]byte(`{"valid":1} trailing`),
	}
	for _, input := range inputs {
		input := input
		t.Run(string(input), func(t *testing.T) {
			t.Parallel()
			if _, err := remotev1.CanonicalizeJSON(input); err == nil {
				t.Fatal("invalid contract JSON accepted")
			}
		})
	}
}

func TestEquivalentLogicalPathsHaveOneHash(t *testing.T) {
	t.Parallel()

	left, err := remotev1.LogicalPathHash("Cafe\u0301/menu.txt")
	if err != nil {
		t.Fatalf("hash decomposed path: %v", err)
	}
	right, err := remotev1.LogicalPathHash("Caf\u00e9/menu.txt")
	if err != nil {
		t.Fatalf("hash composed path: %v", err)
	}
	if left != right {
		t.Fatalf("equivalent paths produced %q and %q", left, right)
	}
}
