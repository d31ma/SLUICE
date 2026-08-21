package s3store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	contractsv1 "github.com/d31ma/sluice/contracts/v1"
	"github.com/d31ma/sluice/internal/domain"
)

func TestStoreImplementsObjectStoreContract(t *testing.T) {
	var _ contractsv1.ObjectStore = (*Store)(nil)
}

func TestNewRejectsUnencryptedRemoteEndpoint(t *testing.T) {
	_, err := New(context.Background(), ProviderConfig{
		Endpoint:        "http://s3.example.test",
		Region:          "us-east-1",
		Bucket:          "bucket",
		AccessKeyID:     "access-key",
		SecretAccessKey: "secret-key",
	}, Options{})
	assertProviderError(t, err, domain.CodeInvalidArgument, false, false)
}

func TestValidateEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		endpoint string
		wantErr  bool
	}{
		{name: "default provider", endpoint: ""},
		{name: "HTTPS provider", endpoint: "https://s3.example.test"},
		{name: "loopback HTTP", endpoint: "http://127.0.0.1:9000"},
		{name: "loopback IPv6 HTTP", endpoint: "http://[::1]:9000"},
		{name: "localhost HTTP", endpoint: "http://localhost:9000"},
		{name: "malformed", endpoint: "https://%41", wantErr: true},
		{name: "relative", endpoint: "s3.example.test", wantErr: true},
		{name: "unsupported scheme", endpoint: "ftp://s3.example.test", wantErr: true},
		{name: "credentials", endpoint: "https://user@s3.example.test", wantErr: true},
		{name: "query", endpoint: "https://s3.example.test?region=one", wantErr: true},
		{name: "fragment", endpoint: "https://s3.example.test#fragment", wantErr: true},
		{name: "remote HTTP", endpoint: "http://s3.example.test", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := validateEndpoint(test.endpoint); (err != nil) != test.wantErr {
				t.Fatalf("validateEndpoint(%q) error = %v, wantErr %t", test.endpoint, err, test.wantErr)
			}
		})
	}
}

func TestDomainClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		apiCode   string
		err       error
		code      domain.Code
		status    int
		retryable bool
	}{
		{name: "unauthorized", status: http.StatusUnauthorized, code: domain.CodeUnauthenticated},
		{name: "forbidden", status: http.StatusForbidden, code: domain.CodeForbidden},
		{name: "missing status", status: http.StatusNotFound, code: domain.CodeNotFound},
		{name: "missing key code", status: http.StatusBadRequest, apiCode: "NoSuchKey", code: domain.CodeNotFound},
		{name: "missing bucket code", apiCode: "NoSuchBucket", code: domain.CodeNotFound},
		{name: "conflict", status: http.StatusConflict, code: domain.CodeConflict},
		{name: "precondition", status: http.StatusPreconditionFailed, code: domain.CodeConflict},
		{name: "bad request", status: http.StatusBadRequest, code: domain.CodeInvalidArgument},
		{name: "timeout", status: http.StatusRequestTimeout, code: domain.CodeObjectStoreUnavailable, retryable: true},
		{name: "rate limited", status: http.StatusTooManyRequests, code: domain.CodeObjectStoreUnavailable, retryable: true},
		{name: "server error", status: http.StatusBadGateway, code: domain.CodeObjectStoreUnavailable, retryable: true},
		{name: "canceled", err: context.Canceled, code: domain.CodeObjectStoreUnavailable},
		{name: "deadline", err: context.DeadlineExceeded, code: domain.CodeObjectStoreUnavailable, retryable: true},
		{name: "network", err: &net.DNSError{Err: "temporary", IsTemporary: true}, code: domain.CodeObjectStoreUnavailable, retryable: true},
		{name: "internal", err: errors.New("provider failed"), code: domain.CodeInternal},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			code, retryable := domainClassification(test.status, test.apiCode, test.err)
			if code != test.code || retryable != test.retryable {
				t.Fatalf("domainClassification() = (%q, %t), want (%q, %t)", code, retryable, test.code, test.retryable)
			}
		})
	}
}

func TestProbeDiscoversCapabilitiesAndCleansUp(t *testing.T) {
	digest := sha256.Sum256([]byte(probePayload))
	providerChecksum := base64.StdEncoding.EncodeToString(digest[:])
	var conditionalWrites atomic.Int32
	var cleanedVersions atomic.Int32

	server := newS3Server(t, func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		switch {
		case request.Method == http.MethodHead && request.URL.Path == "/bucket":
			writer.WriteHeader(http.StatusOK)
		case request.Method == http.MethodGet && query.Has("versioning"):
			writeXML(writer, http.StatusOK, `<VersioningConfiguration xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Status>Enabled</Status></VersioningConfiguration>`)
		case request.Method == http.MethodPut:
			_, _ = io.Copy(io.Discard, request.Body)
			if conditionalWrites.Add(1) > 1 {
				writeS3Error(writer, http.StatusPreconditionFailed, "PreconditionFailed")
				return
			}
			writer.Header().Set("ETag", `"probe-etag"`)
			writer.Header().Set("x-amz-version-id", "probe-version")
			writer.WriteHeader(http.StatusOK)
		case request.Method == http.MethodGet:
			if request.Header.Get("Range") != "bytes=0-0" {
				t.Fatalf("range = %q, want bytes=0-0", request.Header.Get("Range"))
			}
			writer.Header().Set("Content-Length", "1")
			writer.Header().Set("Content-Range", fmt.Sprintf("bytes 0-0/%d", len(probePayload)))
			writer.WriteHeader(http.StatusPartialContent)
			_, _ = io.WriteString(writer, probePayload[:1])
		case request.Method == http.MethodHead:
			writer.Header().Set("Content-Length", fmt.Sprint(len(probePayload)))
			writer.Header().Set("x-amz-checksum-sha256", providerChecksum)
			writer.WriteHeader(http.StatusOK)
		case request.Method == http.MethodPost && query.Has("uploads"):
			writeXML(writer, http.StatusOK, `<InitiateMultipartUploadResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Bucket>bucket</Bucket><Key>probe</Key><UploadId>probe-upload</UploadId></InitiateMultipartUploadResult>`)
		case request.Method == http.MethodDelete && query.Get("uploadId") == "probe-upload":
			writer.WriteHeader(http.StatusNoContent)
		case request.Method == http.MethodDelete:
			if query.Get("versionId") != "probe-version" {
				t.Fatalf("cleanup versionId = %q, want probe-version", query.Get("versionId"))
			}
			cleanedVersions.Add(1)
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected probe request: %s %s", request.Method, request.URL.String())
		}
	})

	store := newTestStore(t, server.URL, Options{MaxConcurrentRequests: 1})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	capabilities, err := store.Probe(ctx)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if capabilities != (contractsv1.Capabilities{
		ConditionalWrites: true,
		RangeDownloads:    true,
		MultipartUploads:  true,
		ObjectVersioning:  true,
		ChecksumSHA256:    true,
	}) {
		t.Fatalf("capabilities = %+v", capabilities)
	}
	if cleanedVersions.Load() != 1 {
		t.Fatalf("cleaned versions = %d, want 1", cleanedVersions.Load())
	}
}

func TestProbeTreatsUnsupportedConditionalCreateAsUnavailable(t *testing.T) {
	var objectRequests atomic.Int32
	server := newS3Server(t, func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodHead && request.URL.Path == "/bucket":
			writer.WriteHeader(http.StatusOK)
		case request.Method == http.MethodGet && request.URL.Query().Has("versioning"):
			writeS3Error(writer, http.StatusNotImplemented, "NotImplemented")
		case request.Method == http.MethodPut:
			objectRequests.Add(1)
			writeS3Error(writer, http.StatusNotImplemented, "NotImplemented")
		default:
			objectRequests.Add(1)
			t.Fatalf("unexpected request after unsupported conditional create: %s %s", request.Method, request.URL.String())
		}
	})

	store := newTestStore(t, server.URL, Options{MaxConcurrentRequests: 1})
	capabilities, err := store.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if capabilities != (contractsv1.Capabilities{}) {
		t.Fatalf("capabilities = %+v, want none", capabilities)
	}
	if objectRequests.Load() != 1 {
		t.Fatalf("object requests = %d, want 1", objectRequests.Load())
	}
}

func TestListPagesAndBindsCursorToPrefix(t *testing.T) {
	var requests atomic.Int32
	server := newS3Server(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("list-type") != "2" {
			t.Fatalf("list-type = %q, want 2", request.URL.Query().Get("list-type"))
		}
		if request.URL.Query().Get("prefix") != "root/" {
			t.Fatalf("prefix = %q, want root/", request.URL.Query().Get("prefix"))
		}

		requestNumber := requests.Add(1)
		if requestNumber == 1 {
			if request.URL.Query().Get("continuation-token") != "" {
				t.Fatalf("first continuation token = %q, want empty", request.URL.Query().Get("continuation-token"))
			}
			writeXML(writer, http.StatusOK, `<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><IsTruncated>true</IsTruncated><NextContinuationToken>provider-token</NextContinuationToken><Contents><Key>root/a</Key><ETag>&quot;etag-a&quot;</ETag><Size>3</Size></Contents></ListBucketResult>`)
			return
		}

		if request.URL.Query().Get("continuation-token") != "provider-token" {
			t.Fatalf("continuation token = %q, want provider-token", request.URL.Query().Get("continuation-token"))
		}
		writeXML(writer, http.StatusOK, `<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><IsTruncated>false</IsTruncated><Contents><Key>root/b</Key><ETag>&quot;etag-b&quot;</ETag><Size>5</Size></Contents></ListBucketResult>`)
	})

	store := newTestStore(t, server.URL, Options{})
	first, err := store.List(context.Background(), "root/", "", 1)
	if err != nil {
		t.Fatalf("List(first): %v", err)
	}
	if len(first.Objects) != 1 || first.Objects[0].Key != "root/a" || first.Objects[0].ETag != `"etag-a"` {
		t.Fatalf("first objects = %#v", first.Objects)
	}
	if first.NextCursor == "" {
		t.Fatal("first NextCursor is empty")
	}

	second, err := store.List(context.Background(), "root/", first.NextCursor, 1)
	if err != nil {
		t.Fatalf("List(second): %v", err)
	}
	if len(second.Objects) != 1 || second.Objects[0].Key != "root/b" || second.NextCursor != "" {
		t.Fatalf("second page = %#v", second)
	}

	_, err = store.List(context.Background(), "other/", first.NextCursor, 1)
	assertProviderError(t, err, domain.CodeInvalidArgument, false, false)
}

func TestCompareAndSwapUsesCreateAndReplacePreconditions(t *testing.T) {
	var mu sync.Mutex
	currentETag := ""
	server := newS3Server(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", request.Method)
		}
		_, _ = io.Copy(io.Discard, request.Body)

		mu.Lock()
		defer mu.Unlock()
		if request.Header.Get("If-None-Match") == "*" {
			if currentETag != "" {
				writeS3Error(writer, http.StatusPreconditionFailed, "PreconditionFailed")
				return
			}
			currentETag = `"etag-1"`
			writer.Header().Set("ETag", currentETag)
			writer.WriteHeader(http.StatusOK)
			return
		}

		if request.Header.Get("If-Match") != currentETag {
			writeS3Error(writer, http.StatusConflict, "ConditionalRequestConflict")
			return
		}
		currentETag = `"etag-2"`
		writer.Header().Set("ETag", currentETag)
		writer.WriteHeader(http.StatusOK)
	})

	store := newTestStore(t, server.URL, Options{})
	created, err := store.CompareAndSwap(context.Background(), contractsv1.CASRequest{
		Body:           strings.NewReader("first"),
		Key:            "root/manifest.json",
		ContentType:    "application/json",
		IdempotencyKey: "create-1",
		Size:           5,
		ExpectedAbsent: true,
	})
	if err != nil {
		t.Fatalf("CompareAndSwap(create): %v", err)
	}
	if !created.Applied || created.Object.ETag != `"etag-1"` {
		t.Fatalf("created = %#v", created)
	}

	duplicate, err := store.CompareAndSwap(context.Background(), contractsv1.CASRequest{
		Body:           strings.NewReader("first"),
		Key:            "root/manifest.json",
		ContentType:    "application/json",
		IdempotencyKey: "create-2",
		Size:           5,
		ExpectedAbsent: true,
	})
	if err != nil || duplicate.Applied {
		t.Fatalf("duplicate = %#v, err = %v", duplicate, err)
	}

	replaced, err := store.CompareAndSwap(context.Background(), contractsv1.CASRequest{
		Body:           strings.NewReader("second"),
		Key:            "root/manifest.json",
		ContentType:    "application/json",
		ExpectedETag:   `"etag-1"`,
		IdempotencyKey: "replace-1",
		Size:           6,
	})
	if err != nil {
		t.Fatalf("CompareAndSwap(replace): %v", err)
	}
	if !replaced.Applied || replaced.Object.ETag != `"etag-2"` {
		t.Fatalf("replaced = %#v", replaced)
	}

	stale, err := store.CompareAndSwap(context.Background(), contractsv1.CASRequest{
		Body:           strings.NewReader("stale"),
		Key:            "root/manifest.json",
		ContentType:    "application/json",
		ExpectedETag:   `"etag-1"`,
		IdempotencyKey: "replace-2",
		Size:           5,
	})
	if err != nil || stale.Applied {
		t.Fatalf("stale = %#v, err = %v", stale, err)
	}
}

func TestGetSupportsRangesAndRejectsCorruptFullDownloads(t *testing.T) {
	content := []byte("abcdefgh")
	digest := sha256.Sum256(content)
	server := newS3Server(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", request.Method)
		}
		writer.Header().Set("ETag", `"etag"`)
		writer.Header().Set("x-amz-meta-sluice-sha256", hex.EncodeToString(digest[:]))
		if request.Header.Get("Range") == "bytes=2-4" {
			writer.Header().Set("Content-Range", "bytes 2-4/8")
			writer.Header().Set("Content-Length", "3")
			writer.WriteHeader(http.StatusPartialContent)
			_, _ = writer.Write(content[2:5])
			return
		}
		writer.Header().Set("Content-Length", "8")
		writer.WriteHeader(http.StatusOK)
		_, _ = writer.Write([]byte("corrupt!"))
	})

	store := newTestStore(t, server.URL, Options{})
	var ranged bytes.Buffer
	result, err := store.Get(context.Background(), contractsv1.DownloadRequest{
		Writer: &ranged,
		Range:  &contractsv1.ByteRange{Offset: 2, Length: 3},
		Key:    "root/data",
	})
	if err != nil {
		t.Fatalf("Get(range): %v", err)
	}
	if ranged.String() != "cde" || result.BytesWritten != 3 || result.Object.Size != 8 {
		t.Fatalf("range body = %q, result = %#v", ranged.String(), result)
	}

	var full bytes.Buffer
	_, err = store.Get(context.Background(), contractsv1.DownloadRequest{Writer: &full, Key: "root/data"})
	assertProviderError(t, err, domain.CodeIntegrityFailure, false, false)
}

func TestHeadAndDeletePreserveVersionIdentity(t *testing.T) {
	payload := []byte("versioned")
	digest := sha256.Sum256(payload)
	digestHex := hex.EncodeToString(digest[:])
	server := newS3Server(t, func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodHead:
			writer.Header().Set("Content-Length", fmt.Sprint(len(payload)))
			writer.Header().Set("ETag", `"version-etag"`)
			writer.Header().Set("x-amz-version-id", "version-42")
			writer.Header().Set("x-amz-meta-sluice-sha256", digestHex)
			writer.WriteHeader(http.StatusOK)
		case http.MethodDelete:
			if request.URL.Query().Get("versionId") != "version-42" {
				t.Fatalf("versionId = %q, want version-42", request.URL.Query().Get("versionId"))
			}
			writer.Header().Set("x-amz-version-id", "version-42")
			writer.WriteHeader(http.StatusNoContent)
		default:
			t.Fatalf("unexpected method %s", request.Method)
		}
	})

	store := newTestStore(t, server.URL, Options{})
	object, err := store.Head(context.Background(), "root/versioned")
	if err != nil {
		t.Fatalf("Head: %v", err)
	}
	if object.VersionID != "version-42" || object.ETag != `"version-etag"` || object.SHA256 != digestHex {
		t.Fatalf("object = %#v", object)
	}
	deleted, err := store.Delete(context.Background(), contractsv1.DeleteRequest{
		Key:            object.Key,
		VersionID:      object.VersionID,
		IdempotencyKey: "delete-version-42",
	})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if deleted.DeletedVersionID != "version-42" {
		t.Fatalf("DeletedVersionID = %q", deleted.DeletedVersionID)
	}
}

func TestPutUsesMultipartAndVerifiesLocalSHA256(t *testing.T) {
	const partSize = 5 * 1024 * 1024
	payload := bytes.Repeat([]byte("m"), partSize+97)
	digest := sha256.Sum256(payload)
	digestHex := hex.EncodeToString(digest[:])

	var multipartCreated atomic.Bool
	var partUploaded atomic.Bool
	server := newS3Server(t, func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		switch {
		case request.Method == http.MethodPost && query.Has("uploads"):
			multipartCreated.Store(true)
			writeXML(writer, http.StatusOK, `<InitiateMultipartUploadResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Bucket>bucket</Bucket><Key>root/large</Key><UploadId>upload-1</UploadId></InitiateMultipartUploadResult>`)
		case request.Method == http.MethodPut && query.Get("partNumber") != "":
			partUploaded.Store(true)
			_, _ = io.Copy(io.Discard, request.Body)
			writer.Header().Set("ETag", fmt.Sprintf(`"part-%s"`, query.Get("partNumber")))
			writer.WriteHeader(http.StatusOK)
		case request.Method == http.MethodPost && query.Get("uploadId") == "upload-1":
			_, _ = io.Copy(io.Discard, request.Body)
			writer.Header().Set("x-amz-version-id", "version-1")
			writeXML(writer, http.StatusOK, `<CompleteMultipartUploadResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/"><Location>http://example.test/bucket/root/large</Location><Bucket>bucket</Bucket><Key>root/large</Key><ETag>&quot;multipart-etag&quot;</ETag></CompleteMultipartUploadResult>`)
		case request.Method == http.MethodHead:
			writer.Header().Set("Content-Length", fmt.Sprint(len(payload)))
			writer.Header().Set("ETag", `"multipart-etag"`)
			writer.Header().Set("x-amz-version-id", "version-1")
			writer.Header().Set("x-amz-meta-sluice-sha256", digestHex)
			writer.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.String())
		}
	})

	store := newTestStore(t, server.URL, Options{
		MultipartThreshold:  partSize,
		PartSize:            partSize,
		TransferConcurrency: 2,
	})
	result, err := store.Put(context.Background(), contractsv1.UploadRequest{
		Body:           bytes.NewReader(payload),
		Key:            "root/large",
		SHA256:         digestHex,
		ContentType:    "application/octet-stream",
		IdempotencyKey: "upload-1",
		Size:           int64(len(payload)),
	})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if !multipartCreated.Load() || !partUploaded.Load() {
		t.Fatalf("multipart created = %t, part uploaded = %t", multipartCreated.Load(), partUploaded.Load())
	}
	if result.Object.SHA256 != digestHex || result.Object.VersionID != "version-1" || result.Object.Size != int64(len(payload)) {
		t.Fatalf("result = %#v", result)
	}
}

func TestPutClassifiesAmbiguousServerFailure(t *testing.T) {
	server := newS3Server(t, func(writer http.ResponseWriter, request *http.Request) {
		writeS3Error(writer, http.StatusInternalServerError, "InternalError")
	})
	store := newTestStore(t, server.URL, Options{})
	payload := []byte("payload")
	digest := sha256.Sum256(payload)
	_, err := store.Put(context.Background(), contractsv1.UploadRequest{
		Body:           bytes.NewReader(payload),
		Key:            "root/ambiguous",
		SHA256:         hex.EncodeToString(digest[:]),
		ContentType:    "application/octet-stream",
		IdempotencyKey: "ambiguous-1",
		Size:           int64(len(payload)),
	})
	assertProviderError(t, err, domain.CodeObjectStoreUnavailable, true, true)
}

func TestRequestConcurrencyIsBounded(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32
	server := newS3Server(t, func(writer http.ResponseWriter, request *http.Request) {
		current := active.Add(1)
		defer active.Add(-1)
		for {
			observed := maximum.Load()
			if current <= observed || maximum.CompareAndSwap(observed, current) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		writer.Header().Set("Content-Length", "0")
		writer.Header().Set("ETag", `"etag"`)
		writer.WriteHeader(http.StatusOK)
	})
	store := newTestStore(t, server.URL, Options{MaxConcurrentRequests: 2})

	const callers = 8
	var wait sync.WaitGroup
	wait.Add(callers)
	for index := 0; index < callers; index++ {
		go func() {
			defer wait.Done()
			if _, err := store.Head(context.Background(), "root/object"); err != nil {
				t.Errorf("Head: %v", err)
			}
		}()
	}
	wait.Wait()

	if got := maximum.Load(); got > 2 {
		t.Fatalf("maximum concurrent requests = %d, want <= 2", got)
	}
}

func newTestStore(t *testing.T, endpoint string, options Options) *Store {
	t.Helper()
	options.RetryMaxAttempts = 1
	store, err := New(context.Background(), ProviderConfig{
		Endpoint:        endpoint,
		Region:          "us-east-1",
		Bucket:          "bucket",
		AccessKeyID:     "access-key",
		SecretAccessKey: "secret-key",
		ForcePathStyle:  true,
	}, options)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return store
}

func newS3Server(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return server
}

func writeXML(writer http.ResponseWriter, status int, payload string) {
	writer.Header().Set("Content-Type", "application/xml")
	writer.WriteHeader(status)
	_, _ = io.WriteString(writer, payload)
}

func writeS3Error(writer http.ResponseWriter, status int, code string) {
	writeXML(writer, status, `<Error><Code>`+code+`</Code><Message>provider error</Message><RequestId>request-id</RequestId></Error>`)
}

func assertProviderError(t *testing.T, err error, code domain.Code, retryable, ambiguous bool) {
	t.Helper()
	if err == nil {
		t.Fatal("error is nil")
	}
	providerError, ok := AsProviderError(err)
	if !ok {
		t.Fatalf("error type = %T, want ProviderError", err)
	}
	if providerError.Code() != code || providerError.Retryable() != retryable || providerError.Ambiguous() != ambiguous {
		t.Fatalf("error classification = (%q, %t, %t), want (%q, %t, %t)", providerError.Code(), providerError.Retryable(), providerError.Ambiguous(), code, retryable, ambiguous)
	}
}
