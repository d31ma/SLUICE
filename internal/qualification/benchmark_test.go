package qualification_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/d31ma/sluice/internal/contracts/remotev1"
	"github.com/d31ma/sluice/internal/filesystem"
	"github.com/d31ma/sluice/internal/transport/localapi"
)

const benchmarkToken = "0123456789abcdef0123456789abcdef"

func BenchmarkCanonicalLogicalPath(b *testing.B) {
	for index := 0; b.Loop(); index++ {
		path := fmt.Sprintf("Documents/Reports/%08d/Cafe\u0301.txt", index)
		if _, err := remotev1.CanonicalLogicalPath(path); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkAuthenticatedLocalAPIRead(b *testing.B) {
	handler, err := localapi.NewHandler(
		benchmarkToken,
		localapi.NewMemoryIdempotencyStore(),
		http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			response.WriteHeader(http.StatusOK)
		}),
	)
	if err != nil {
		b.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/v1/status", nil)
	request.RemoteAddr = "127.0.0.1:4242"
	request.Header.Set("Authorization", "Bearer "+benchmarkToken)

	for b.Loop() {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request.Clone(context.Background()))
		if response.Code != http.StatusOK {
			b.Fatalf("status = %d", response.Code)
		}
	}
}

func BenchmarkAtomicInstallOneMiB(b *testing.B) {
	root, err := filesystem.CanonicalizeRoot(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	installer, err := filesystem.NewAtomicInstaller(root)
	if err != nil {
		b.Fatal(err)
	}
	content := bytes.Repeat([]byte{0x5a}, 1<<20)
	digest := sha256.Sum256(content)
	b.SetBytes(int64(len(content)))
	b.ResetTimer()

	for index := 0; b.Loop(); index++ {
		path, pathErr := filesystem.ParseLogicalPath(fmt.Sprintf("install-%08d.bin", index))
		if pathErr != nil {
			b.Fatal(pathErr)
		}
		_, installErr := installer.Install(context.Background(), filesystem.InstallRequest{
			Source:         bytes.NewReader(content),
			Path:           path,
			ExpectedSHA256: digest,
			Mode:           0o600,
		})
		if installErr != nil {
			b.Fatal(installErr)
		}
	}
}
