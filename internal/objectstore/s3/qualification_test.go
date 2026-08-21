package s3store_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	contractsv1 "github.com/d31ma/sluice/contracts/v1"
	s3store "github.com/d31ma/sluice/internal/objectstore/s3"
)

func TestProviderQualification(t *testing.T) {
	if os.Getenv("SLUICE_S3_QUALIFY") != "1" {
		t.Skip("set SLUICE_S3_QUALIFY=1 and the documented provider variables")
	}
	providerName := requiredEnvironment(t, "SLUICE_S3_PROVIDER")
	store, err := s3store.New(context.Background(), s3store.ProviderConfig{
		Endpoint:        os.Getenv("SLUICE_S3_ENDPOINT"),
		Region:          environmentOr("SLUICE_S3_REGION", "us-east-1"),
		Bucket:          requiredEnvironment(t, "SLUICE_S3_BUCKET"),
		AccessKeyID:     os.Getenv("SLUICE_S3_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("SLUICE_S3_SECRET_ACCESS_KEY"),
		SessionToken:    os.Getenv("SLUICE_S3_SESSION_TOKEN"),
		ForcePathStyle:  environmentBool(t, "SLUICE_S3_FORCE_PATH_STYLE"),
	}, s3store.Options{
		MaxConcurrentRequests: 3,
		TransferConcurrency:   2,
		RetryMaxAttempts:      2,
		MultipartThreshold:    5 * 1024 * 1024,
		PartSize:              5 * 1024 * 1024,
	})
	if err != nil {
		t.Fatalf("construct %s store: %v", providerName, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	capabilities, err := store.Probe(ctx)
	if err != nil {
		t.Fatalf("probe %s: %v", providerName, err)
	}
	t.Logf("provider=%s capabilities=%+v", providerName, capabilities)
	if !capabilities.ConditionalWrites || !capabilities.RangeDownloads || !capabilities.MultipartUploads {
		t.Fatalf("provider %s lacks required core capability: %+v", providerName, capabilities)
	}
	if os.Getenv("SLUICE_S3_REQUIRE_VERSIONING") == "1" && !capabilities.ObjectVersioning {
		t.Fatalf("provider %s bucket does not have required versioning", providerName)
	}

	prefix := qualificationPrefix(t)
	knownObjects := make([]contractsv1.RemoteObject, 0, 5)
	defer func() { cleanupQualificationObjects(t, store, knownObjects) }()

	smallPayload := []byte("sluice-provider-qualification")
	smallDigest := sha256Hex(smallPayload)
	small, err := store.Put(ctx, contractsv1.UploadRequest{
		Body:           bytes.NewReader(smallPayload),
		Key:            prefix + "small.bin",
		SHA256:         smallDigest,
		ContentType:    "application/octet-stream",
		IdempotencyKey: prefix + "put-small",
		Size:           int64(len(smallPayload)),
	})
	if err != nil {
		t.Fatalf("small put: %v", err)
	}
	knownObjects = append(knownObjects, small.Object)
	assertHeadAndDownloads(t, ctx, store, small.Object, smallPayload)

	manifestKey := prefix + "manifest.json"
	manifestOne := []byte(`{"revision":1}`)
	created, err := store.CompareAndSwap(ctx, casRequest(manifestKey, manifestOne, "cas-create", "", true))
	if err != nil || !created.Applied {
		t.Fatalf("conditional create = %+v, err = %v", created, err)
	}
	knownObjects = append(knownObjects, created.Object)
	rejected, err := store.CompareAndSwap(ctx, casRequest(manifestKey, manifestOne, "cas-reject-create", "", true))
	if err != nil || rejected.Applied {
		t.Fatalf("duplicate conditional create = %+v, err = %v", rejected, err)
	}
	manifestTwo := []byte(`{"revision":2}`)
	updated, err := store.CompareAndSwap(ctx, casRequest(manifestKey, manifestTwo, "cas-update", created.Object.ETag, false))
	if err != nil || !updated.Applied {
		t.Fatalf("conditional replace = %+v, err = %v", updated, err)
	}
	knownObjects = append(knownObjects, updated.Object)
	stale, err := store.CompareAndSwap(ctx, casRequest(manifestKey, manifestOne, "cas-stale", created.Object.ETag, false))
	if err != nil || stale.Applied {
		t.Fatalf("stale conditional replace = %+v, err = %v", stale, err)
	}

	largePayload := bytes.Repeat([]byte("L"), 5*1024*1024+257)
	large, err := store.Put(ctx, contractsv1.UploadRequest{
		Body:           bytes.NewReader(largePayload),
		Key:            prefix + "large.bin",
		SHA256:         sha256Hex(largePayload),
		ContentType:    "application/octet-stream",
		IdempotencyKey: prefix + "put-large",
		Size:           int64(len(largePayload)),
	})
	if err != nil {
		t.Fatalf("multipart put: %v", err)
	}
	knownObjects = append(knownObjects, large.Object)
	if large.Object.Size != int64(len(largePayload)) || large.Object.SHA256 != sha256Hex(largePayload) {
		t.Fatalf("multipart result = %+v", large.Object)
	}

	assertPaginatedList(t, ctx, store, prefix, 3)
	for _, object := range knownObjects {
		deleted, deleteErr := store.Delete(ctx, contractsv1.DeleteRequest{
			Key:            object.Key,
			VersionID:      object.VersionID,
			IdempotencyKey: prefix + "delete-" + sha256Hex([]byte(object.Key + object.VersionID))[:16],
		})
		if deleteErr != nil {
			t.Fatalf("delete %q version %q: %v", object.Key, object.VersionID, deleteErr)
		}
		if object.VersionID != "" && deleted.DeletedVersionID != object.VersionID {
			t.Fatalf("deleted version = %q, want %q", deleted.DeletedVersionID, object.VersionID)
		}
	}
	knownObjects = knownObjects[:0]
}

func assertHeadAndDownloads(t *testing.T, ctx context.Context, store *s3store.Store, object contractsv1.RemoteObject, payload []byte) {
	t.Helper()
	head, err := store.Head(ctx, object.Key)
	if err != nil {
		t.Fatalf("head %q: %v", object.Key, err)
	}
	if head.Size != int64(len(payload)) || head.SHA256 != sha256Hex(payload) {
		t.Fatalf("head = %+v", head)
	}
	var full bytes.Buffer
	result, err := store.Get(ctx, contractsv1.DownloadRequest{Writer: &full, Key: object.Key, VersionID: object.VersionID})
	if err != nil {
		t.Fatalf("full get: %v", err)
	}
	if !bytes.Equal(full.Bytes(), payload) || result.BytesWritten != int64(len(payload)) {
		t.Fatalf("full get bytes = %d, result = %+v", full.Len(), result)
	}
	var ranged bytes.Buffer
	result, err = store.Get(ctx, contractsv1.DownloadRequest{
		Writer:    &ranged,
		Range:     &contractsv1.ByteRange{Offset: 2, Length: 5},
		Key:       object.Key,
		VersionID: object.VersionID,
	})
	if err != nil {
		t.Fatalf("range get: %v", err)
	}
	if !bytes.Equal(ranged.Bytes(), payload[2:7]) || result.BytesWritten != 5 {
		t.Fatalf("range get = %q, result = %+v", ranged.Bytes(), result)
	}
}

func assertPaginatedList(t *testing.T, ctx context.Context, store *s3store.Store, prefix string, minimum int) {
	t.Helper()
	cursor := ""
	found := 0
	for {
		page, err := store.List(ctx, prefix, cursor, 1)
		if err != nil {
			t.Fatalf("list page: %v", err)
		}
		found += len(page.Objects)
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if found < minimum {
		t.Fatalf("listed %d objects, want at least %d", found, minimum)
	}
}

func casRequest(key string, payload []byte, idempotencyKey, etag string, absent bool) contractsv1.CASRequest {
	return contractsv1.CASRequest{
		Body:           bytes.NewReader(payload),
		Key:            key,
		ContentType:    "application/json",
		ExpectedETag:   etag,
		IdempotencyKey: idempotencyKey,
		Size:           int64(len(payload)),
		ExpectedAbsent: absent,
	}
}

func cleanupQualificationObjects(t *testing.T, store *s3store.Store, objects []contractsv1.RemoteObject) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for index, object := range objects {
		_, err := store.Delete(ctx, contractsv1.DeleteRequest{
			Key:            object.Key,
			VersionID:      object.VersionID,
			IdempotencyKey: fmt.Sprintf("qualification-cleanup-%d", index),
		})
		if err != nil {
			t.Errorf("cleanup %q version %q: %v", object.Key, object.VersionID, err)
		}
	}
}

func qualificationPrefix(t *testing.T) string {
	t.Helper()
	identifier := make([]byte, 16)
	if _, err := rand.Read(identifier); err != nil {
		t.Fatalf("random qualification prefix: %v", err)
	}
	return ".sluice/v1/qualification/" + hex.EncodeToString(identifier) + "/"
}

func sha256Hex(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func requiredEnvironment(t *testing.T, name string) string {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}

func environmentOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func environmentBool(t *testing.T, name string) bool {
	t.Helper()
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return false
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return parsed
}
