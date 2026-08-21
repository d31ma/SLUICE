package s3store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"strconv"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	transfertypes "github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	contractsv1 "github.com/d31ma/sluice/contracts/v1"
	"github.com/d31ma/sluice/internal/domain"
)

const (
	sha256HexLength      = sha256.Size * 2
	checksumMetadataName = "sluice-sha256"
)

func (store *Store) List(ctx context.Context, prefix, cursor string, limit int) (contractsv1.ObjectPage, error) {
	if err := validatePrefix(prefix); err != nil {
		return contractsv1.ObjectPage{}, invalid("list", err)
	}
	if limit < 1 || limit > 1000 {
		return contractsv1.ObjectPage{}, invalid("list", errors.New("list limit must be between 1 and 1000"))
	}
	continuationToken, err := decodeCursor(prefix, cursor)
	if err != nil {
		return contractsv1.ObjectPage{}, invalid("list", err)
	}
	release, err := store.acquire(ctx, "list")
	if err != nil {
		return contractsv1.ObjectPage{}, err
	}
	defer release()

	output, err := store.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:            store.bucket,
		ContinuationToken: optionalString(continuationToken),
		MaxKeys:           aws.Int32(int32(limit)),
		Prefix:            aws.String(prefix),
	})
	if err != nil {
		return contractsv1.ObjectPage{}, classify("list", err, false)
	}
	return objectPage(prefix, output)
}

func objectPage(prefix string, output *s3.ListObjectsV2Output) (contractsv1.ObjectPage, error) {
	objects := make([]contractsv1.RemoteObject, 0, len(output.Contents))
	for _, object := range output.Contents {
		objects = append(objects, contractsv1.RemoteObject{
			Key:  aws.ToString(object.Key),
			ETag: aws.ToString(object.ETag),
			Size: aws.ToInt64(object.Size),
		})
	}
	page := contractsv1.ObjectPage{Objects: objects}
	if !aws.ToBool(output.IsTruncated) {
		return page, nil
	}
	token := aws.ToString(output.NextContinuationToken)
	if token == "" {
		return contractsv1.ObjectPage{}, newProviderError("list", domain.CodeProtocolMismatch, false, false, errors.New("truncated list omitted continuation token"))
	}
	cursor, err := encodeCursor(prefix, token)
	if err != nil {
		return contractsv1.ObjectPage{}, newProviderError("list", domain.CodeInternal, false, false, err)
	}
	page.NextCursor = cursor
	return page, nil
}

func (store *Store) Head(ctx context.Context, key string) (contractsv1.RemoteObject, error) {
	if err := validateKey(key); err != nil {
		return contractsv1.RemoteObject{}, invalid("head", err)
	}
	release, err := store.acquire(ctx, "head")
	if err != nil {
		return contractsv1.RemoteObject{}, err
	}
	defer release()
	return store.head(ctx, key, "")
}

func (store *Store) head(ctx context.Context, key, versionID string) (contractsv1.RemoteObject, error) {
	output, err := store.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket:       store.bucket,
		ChecksumMode: s3types.ChecksumModeEnabled,
		Key:          aws.String(key),
		VersionId:    optionalString(versionID),
	})
	if err != nil {
		return contractsv1.RemoteObject{}, classify("head", err, false)
	}
	digest, err := remoteSHA256(output.Metadata, output.ChecksumSHA256)
	if err != nil {
		return contractsv1.RemoteObject{}, integrity("head", err)
	}
	return contractsv1.RemoteObject{
		Key:       key,
		ETag:      aws.ToString(output.ETag),
		VersionID: aws.ToString(output.VersionId),
		SHA256:    digest,
		Size:      aws.ToInt64(output.ContentLength),
	}, nil
}

func (store *Store) Get(ctx context.Context, request contractsv1.DownloadRequest) (contractsv1.DownloadResult, error) {
	if err := validateDownload(request); err != nil {
		return contractsv1.DownloadResult{}, invalid("get", err)
	}
	release, err := store.acquire(ctx, "get")
	if err != nil {
		return contractsv1.DownloadResult{}, err
	}
	defer release()

	output, err := store.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket:       store.bucket,
		ChecksumMode: s3types.ChecksumModeEnabled,
		Key:          aws.String(request.Key),
		Range:        byteRangeHeader(request.Range),
		VersionId:    optionalString(request.VersionID),
	})
	if err != nil {
		return contractsv1.DownloadResult{}, classify("get", err, false)
	}
	digest, err := remoteSHA256(output.Metadata, output.ChecksumSHA256)
	if err != nil {
		_ = output.Body.Close()
		return contractsv1.DownloadResult{}, integrity("get", err)
	}
	written, actualDigest, readErr, writeErr := copyAndHash(request.Writer, output.Body)
	closeErr := output.Body.Close()
	if writeErr != nil {
		return contractsv1.DownloadResult{}, writerFailure("get", writeErr)
	}
	if readErr != nil {
		return contractsv1.DownloadResult{}, classify("get", readErr, false)
	}
	if closeErr != nil {
		return contractsv1.DownloadResult{}, classify("get", closeErr, false)
	}
	if err := verifyDownload(request.Range, written, digest, actualDigest); err != nil {
		return contractsv1.DownloadResult{}, integrity("get", err)
	}
	return downloadResult(request.Key, output, digest, written), nil
}

func (store *Store) Put(ctx context.Context, request contractsv1.UploadRequest) (contractsv1.UploadResult, error) {
	if err := validateUpload(request); err != nil {
		return contractsv1.UploadResult{}, invalid("put", err)
	}
	release, err := store.acquire(ctx, "put")
	if err != nil {
		return contractsv1.UploadResult{}, err
	}
	defer release()

	verifyingBody := newVerifyingReader(request.Body)
	output, err := store.uploader.UploadObject(ctx, &transfermanager.UploadObjectInput{
		Bucket:            store.bucket,
		Body:              verifyingBody,
		ChecksumAlgorithm: transfertypes.ChecksumAlgorithmSha256,
		ContentLength:     aws.Int64(request.Size),
		ContentType:       optionalString(request.ContentType),
		Key:               aws.String(request.Key),
		Metadata:          map[string]string{checksumMetadataName: request.SHA256},
	})
	if err != nil {
		return contractsv1.UploadResult{}, classify("put", err, true)
	}
	if verifyErr := verifyingBody.Verify(request.Size, request.SHA256); verifyErr != nil {
		store.cleanupVersion(ctx, request.Key, aws.ToString(output.VersionID))
		return contractsv1.UploadResult{}, integrity("put", verifyErr)
	}
	object, err := store.head(ctx, request.Key, aws.ToString(output.VersionID))
	if err != nil {
		return contractsv1.UploadResult{}, ambiguousPostcondition("put", err)
	}
	if err := verifyUploadedObject(object, request.Size, request.SHA256); err != nil {
		return contractsv1.UploadResult{}, integrity("put", err)
	}
	return contractsv1.UploadResult{Object: object}, nil
}

func (store *Store) CompareAndSwap(ctx context.Context, request contractsv1.CASRequest) (contractsv1.CASResult, error) {
	if err := validateCAS(request); err != nil {
		return contractsv1.CASResult{}, invalid("compare-and-swap", err)
	}
	payload, digest, err := readCASBody(request.Body, request.Size)
	if err != nil {
		return contractsv1.CASResult{}, integrity("compare-and-swap", err)
	}
	release, err := store.acquire(ctx, "compare-and-swap")
	if err != nil {
		return contractsv1.CASResult{}, err
	}
	defer release()

	uploadInput := &transfermanager.UploadObjectInput{
		Bucket:            store.bucket,
		Body:              bytes.NewReader(payload),
		ChecksumAlgorithm: transfertypes.ChecksumAlgorithmSha256,
		ChecksumSHA256:    optionalString(hexToBase64(digest)),
		ContentLength:     aws.Int64(request.Size),
		ContentType:       optionalString(request.ContentType),
		IfMatch:           optionalString(request.ExpectedETag),
		Key:               aws.String(request.Key),
		Metadata:          map[string]string{checksumMetadataName: digest},
	}
	if request.ExpectedAbsent {
		uploadInput.IfNoneMatch = aws.String("*")
	}
	output, err := store.uploader.UploadObject(ctx, uploadInput)
	if isPreconditionFailure(err) {
		return contractsv1.CASResult{Applied: false}, nil
	}
	if err != nil {
		return contractsv1.CASResult{}, classify("compare-and-swap", err, true)
	}
	object := uploadOutputObject(request.Key, digest, request.Size, output)
	return contractsv1.CASResult{Object: object, Applied: true}, nil
}

func (store *Store) Delete(ctx context.Context, request contractsv1.DeleteRequest) (contractsv1.DeleteResult, error) {
	if err := validateDelete(request); err != nil {
		return contractsv1.DeleteResult{}, invalid("delete", err)
	}
	release, err := store.acquire(ctx, "delete")
	if err != nil {
		return contractsv1.DeleteResult{}, err
	}
	defer release()

	output, err := store.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket:    store.bucket,
		Key:       aws.String(request.Key),
		VersionId: optionalString(request.VersionID),
	})
	if err != nil {
		return contractsv1.DeleteResult{}, classify("delete", err, true)
	}
	return contractsv1.DeleteResult{DeletedVersionID: aws.ToString(output.VersionId)}, nil
}

func readCASBody(body io.Reader, expectedSize int64) ([]byte, string, error) {
	payload, err := io.ReadAll(io.LimitReader(body, expectedSize+1))
	if err != nil {
		return nil, "", fmt.Errorf("read body: %w", err)
	}
	if int64(len(payload)) != expectedSize {
		return nil, "", fmt.Errorf("body length = %d, want %d", len(payload), expectedSize)
	}
	digest := sha256.Sum256(payload)
	return payload, hex.EncodeToString(digest[:]), nil
}

func uploadOutputObject(key, digest string, size int64, output *transfermanager.UploadObjectOutput) contractsv1.RemoteObject {
	return contractsv1.RemoteObject{
		Key:       key,
		ETag:      aws.ToString(output.ETag),
		VersionID: aws.ToString(output.VersionID),
		SHA256:    digest,
		Size:      size,
	}
}

func (store *Store) cleanupVersion(ctx context.Context, key, versionID string) {
	_ = store.deleteVersion(ctx, key, versionID)
}

func (store *Store) deleteVersion(ctx context.Context, key, versionID string) error {
	_, err := store.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket:    store.bucket,
		Key:       aws.String(key),
		VersionId: optionalString(versionID),
	})
	return err
}

func ambiguousPostcondition(operation string, err error) error {
	providerError, ok := AsProviderError(err)
	if !ok {
		return newProviderError(operation, domain.CodeObjectStoreUnavailable, true, true, err)
	}
	return newProviderError(operation, providerError.Code(), providerError.Retryable(), true, err)
}

func isPreconditionFailure(err error) bool {
	if err == nil {
		return false
	}
	status, code := providerStatus(err)
	return status == 409 || status == 412 || code == "PreconditionFailed" || code == "ConditionalRequestConflict"
}

func verifyUploadedObject(object contractsv1.RemoteObject, size int64, digest string) error {
	if object.Size != size {
		return fmt.Errorf("remote size = %d, want %d", object.Size, size)
	}
	if object.SHA256 != digest {
		return fmt.Errorf("remote SHA-256 = %q, want %q", object.SHA256, digest)
	}
	return nil
}

type verifyingReader struct {
	reader io.Reader
	hash   hash.Hash
	count  int64
	mutex  sync.Mutex
}

func newVerifyingReader(reader io.Reader) *verifyingReader {
	return &verifyingReader{reader: reader, hash: sha256.New()}
}

func (reader *verifyingReader) Read(buffer []byte) (int, error) {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	count, err := reader.reader.Read(buffer)
	if count > 0 {
		_, _ = reader.hash.Write(buffer[:count])
		reader.count += int64(count)
	}
	return count, err
}

func (reader *verifyingReader) Verify(size int64, expectedDigest string) error {
	reader.mutex.Lock()
	defer reader.mutex.Unlock()
	if reader.count != size {
		return fmt.Errorf("body length = %d, want %d", reader.count, size)
	}
	digest := hex.EncodeToString(reader.hash.Sum(nil))
	if digest != expectedDigest {
		return fmt.Errorf("body SHA-256 = %q, want %q", digest, expectedDigest)
	}
	return nil
}

func copyAndHash(writer io.Writer, reader io.Reader) (int64, string, error, error) {
	hasher := sha256.New()
	buffer := make([]byte, 128*1024)
	var total int64
	for {
		count, readErr := reader.Read(buffer)
		if count > 0 {
			written, writeErr := writer.Write(buffer[:count])
			if written > 0 {
				_, _ = hasher.Write(buffer[:written])
				total += int64(written)
			}
			if writeErr != nil {
				return total, "", nil, writeErr
			}
			if written != count {
				return total, "", nil, io.ErrShortWrite
			}
		}
		if readErr != nil {
			digest := hex.EncodeToString(hasher.Sum(nil))
			if errors.Is(readErr, io.EOF) {
				return total, digest, nil, nil
			}
			return total, digest, readErr, nil
		}
	}
}

func verifyDownload(byteRange *contractsv1.ByteRange, written int64, expectedDigest, actualDigest string) error {
	if byteRange != nil {
		if written != byteRange.Length {
			return fmt.Errorf("range length = %d, want %d", written, byteRange.Length)
		}
		return nil
	}
	if expectedDigest != "" && actualDigest != expectedDigest {
		return fmt.Errorf("download SHA-256 = %q, want %q", actualDigest, expectedDigest)
	}
	return nil
}

func downloadResult(key string, output *s3.GetObjectOutput, digest string, written int64) contractsv1.DownloadResult {
	return contractsv1.DownloadResult{
		Object: contractsv1.RemoteObject{
			Key:       key,
			ETag:      aws.ToString(output.ETag),
			VersionID: aws.ToString(output.VersionId),
			SHA256:    digest,
			Size:      totalObjectSize(output.ContentRange, aws.ToInt64(output.ContentLength)),
		},
		BytesWritten: written,
	}
}

func totalObjectSize(contentRange *string, contentLength int64) int64 {
	value := aws.ToString(contentRange)
	separator := strings.LastIndexByte(value, '/')
	if separator < 0 {
		return contentLength
	}
	total, err := strconv.ParseInt(value[separator+1:], 10, 64)
	if err != nil {
		return contentLength
	}
	return total
}

func byteRangeHeader(byteRange *contractsv1.ByteRange) *string {
	if byteRange == nil {
		return nil
	}
	last := byteRange.Offset + byteRange.Length - 1
	return aws.String(fmt.Sprintf("bytes=%d-%d", byteRange.Offset, last))
}

func remoteSHA256(metadata map[string]string, providerChecksum *string) (string, error) {
	if metadataDigest := metadata[checksumMetadataName]; metadataDigest != "" {
		if err := validateSHA256(metadataDigest); err != nil {
			return "", fmt.Errorf("invalid Sluice checksum metadata: %w", err)
		}
		return metadataDigest, nil
	}
	checksum := aws.ToString(providerChecksum)
	if checksum == "" || strings.Contains(checksum, "-") {
		return "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(checksum)
	if err != nil || len(decoded) != sha256.Size {
		return "", errors.New("provider returned an invalid SHA-256 checksum")
	}
	return hex.EncodeToString(decoded), nil
}

func hexToBase64(value string) string {
	decoded, _ := hex.DecodeString(value)
	return base64.StdEncoding.EncodeToString(decoded)
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return aws.String(value)
}
