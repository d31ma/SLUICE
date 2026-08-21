package s3store

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	contractsv1 "github.com/d31ma/sluice/contracts/v1"
	"github.com/d31ma/sluice/internal/domain"
)

const probePayload = "sluice-s3-probe"

// Probe performs reversible provider operations instead of inferring support
// from an endpoint name. The caller should persist the returned capabilities
// with the provider profile.
func (store *Store) Probe(ctx context.Context) (capabilities contractsv1.Capabilities, returnErr error) {
	release, err := store.acquire(ctx, "probe")
	if err != nil {
		return contractsv1.Capabilities{}, err
	}
	defer release()
	return store.runProbe(ctx)
}

func (store *Store) runProbe(ctx context.Context) (capabilities contractsv1.Capabilities, returnErr error) {
	capabilities, probe, err := store.prepareProbe(ctx)
	if err != nil {
		return capabilities, err
	}
	defer probe.cleanup(ctx, &returnErr)
	return probe.collectCapabilities(ctx, capabilities)
}

func (store *Store) prepareProbe(ctx context.Context) (contractsv1.Capabilities, *capabilityProbe, error) {
	capabilities := contractsv1.Capabilities{}
	if _, headErr := store.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: store.bucket}); headErr != nil {
		return capabilities, nil, classify("probe", headErr, false)
	}
	var err error
	capabilities.ObjectVersioning, err = store.probeVersioning(ctx)
	if err != nil {
		return capabilities, nil, err
	}

	key, err := randomProbeKey()
	if err != nil {
		return capabilities, nil, newProviderError("probe", domain.CodeInternal, false, false, err)
	}
	probeStore := *store
	probeStore.limiter = make(chan struct{}, 1)
	return capabilities, &capabilityProbe{
		store:      store,
		probeStore: &probeStore,
		key:        key,
		versions:   make([]string, 0, 2),
	}, nil
}

type capabilityProbe struct {
	store      *Store
	probeStore *Store
	key        string
	versions   []string
}

func (probe *capabilityProbe) collectCapabilities(ctx context.Context, capabilities contractsv1.Capabilities) (contractsv1.Capabilities, error) {
	conditionalWrites, proceed, err := probe.probeConditionalWrites(ctx)
	if err != nil {
		return capabilities, err
	}
	if !proceed {
		return capabilities, nil
	}
	capabilities.ConditionalWrites = conditionalWrites
	return probe.probeObjectCapabilities(ctx, capabilities)
}

func (probe *capabilityProbe) probeConditionalWrites(ctx context.Context) (conditionalWrites, proceed bool, err error) {
	created, err := probe.createObject(ctx)
	if err != nil {
		return false, false, err
	}
	if !created {
		return false, false, nil
	}
	conditionalWrites, err = probe.verifyConditionalWrite(ctx)
	return conditionalWrites, true, err
}

func (probe *capabilityProbe) createObject(ctx context.Context) (bool, error) {
	created, err := probe.probeStore.CompareAndSwap(ctx, probeCASRequest(probe.key))
	if err != nil {
		return false, optionalCapabilityError(err, err)
	}
	probe.versions = append(probe.versions, created.Object.VersionID)
	if !created.Applied {
		return false, newProviderError("probe-conditional-create", domain.CodeProtocolMismatch, false, false, errors.New("fresh probe key was not created"))
	}
	return true, nil
}

func (probe *capabilityProbe) verifyConditionalWrite(ctx context.Context) (bool, error) {
	duplicate, err := probe.probeStore.CompareAndSwap(ctx, probeCASRequest(probe.key))
	if err != nil {
		return false, optionalCapabilityError(err, err)
	}
	if duplicate.Applied {
		probe.versions = append(probe.versions, duplicate.Object.VersionID)
	}
	return !duplicate.Applied, nil
}

func (probe *capabilityProbe) probeObjectCapabilities(ctx context.Context, capabilities contractsv1.Capabilities) (contractsv1.Capabilities, error) {
	probes := []struct {
		target *bool
		probe  func(context.Context) (bool, error)
	}{
		{target: &capabilities.RangeDownloads, probe: func(ctx context.Context) (bool, error) {
			return probe.probeStore.probeRead(ctx, probe.key)
		}},
		{target: &capabilities.ChecksumSHA256, probe: func(ctx context.Context) (bool, error) {
			return probe.store.probeNativeChecksum(ctx, probe.key)
		}},
		{target: &capabilities.MultipartUploads, probe: func(ctx context.Context) (bool, error) {
			return probe.store.probeMultipart(ctx, probe.key+"-multipart")
		}},
	}
	for _, capability := range probes {
		available, err := capability.probe(ctx)
		if err != nil {
			return capabilities, err
		}
		*capability.target = available
	}
	return capabilities, nil
}

func (probe *capabilityProbe) cleanup(ctx context.Context, returnErr *error) {
	cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	for _, versionID := range probe.versions {
		if err := probe.store.deleteVersion(cleanupContext, probe.key, versionID); err != nil {
			setProbeCleanupError(returnErr, err)
		}
	}
}

func setProbeCleanupError(returnErr *error, cleanupErr error) {
	if *returnErr == nil {
		*returnErr = classify("probe-cleanup", cleanupErr, true)
	}
}

func (store *Store) probeVersioning(ctx context.Context) (bool, error) {
	output, err := store.client.GetBucketVersioning(ctx, &s3.GetBucketVersioningInput{
		Bucket: store.bucket,
	})
	if err != nil {
		if unsupportedCapability(err) {
			return false, nil
		}
		return false, classify("probe-versioning", err, false)
	}
	return output.Status == s3types.BucketVersioningStatusEnabled, nil
}

func (store *Store) probeRead(ctx context.Context, key string) (bool, error) {
	var destination bytes.Buffer
	result, err := store.Get(ctx, contractsv1.DownloadRequest{
		Writer: &destination,
		Range:  &contractsv1.ByteRange{Offset: 0, Length: 1},
		Key:    key,
	})
	if err != nil {
		return false, optionalCapabilityError(err, err)
	}
	if destination.String() != probePayload[:1] {
		return false, integrity("probe-range", errors.New("provider returned the wrong range"))
	}
	return result.BytesWritten == 1, nil
}

func (store *Store) probeNativeChecksum(ctx context.Context, key string) (bool, error) {
	output, err := store.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket:       store.bucket,
		ChecksumMode: s3types.ChecksumModeEnabled,
		Key:          aws.String(key),
	})
	if err != nil {
		return false, optionalCapabilityError(err, classify("probe-checksum", err, false))
	}
	providerDigest, err := remoteSHA256(nil, output.ChecksumSHA256)
	if err != nil {
		return false, integrity("probe-checksum", err)
	}
	digest := sha256.Sum256([]byte(probePayload))
	return providerDigest == hex.EncodeToString(digest[:]), nil
}

func (store *Store) probeMultipart(ctx context.Context, key string) (bool, error) {
	created, err := store.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:            store.bucket,
		ChecksumAlgorithm: s3types.ChecksumAlgorithmSha256,
		Key:               aws.String(key),
	})
	if err != nil {
		return false, optionalCapabilityError(err, classify("probe-multipart", err, true))
	}
	_, err = store.client.AbortMultipartUpload(ctx, &s3.AbortMultipartUploadInput{
		Bucket:   store.bucket,
		Key:      aws.String(key),
		UploadId: created.UploadId,
	})
	if err != nil {
		return false, classify("probe-abort-multipart", err, true)
	}
	return true, nil
}

func probeCASRequest(key string) contractsv1.CASRequest {
	return contractsv1.CASRequest{
		Body:           bytes.NewBufferString(probePayload),
		Key:            key,
		ContentType:    "application/octet-stream",
		IdempotencyKey: key,
		Size:           int64(len(probePayload)),
		ExpectedAbsent: true,
	}
}

func randomProbeKey() (string, error) {
	identifier := make([]byte, 16)
	if _, err := rand.Read(identifier); err != nil {
		return "", fmt.Errorf("generate probe identifier: %w", err)
	}
	return ".sluice/v1/probes/" + hex.EncodeToString(identifier), nil
}

func unsupportedCapability(err error) bool {
	status, _ := providerStatus(err)
	return status == http.StatusBadRequest || status == http.StatusMethodNotAllowed || status == http.StatusNotImplemented
}

func optionalCapabilityError(err, fallback error) error {
	if unsupportedCapability(err) {
		return nil
	}
	return fallback
}
