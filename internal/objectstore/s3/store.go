package s3store

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	defaultMaxConcurrentRequests = 8
	defaultTransferConcurrency   = 4
	defaultMultipartThreshold    = 16 * 1024 * 1024
	defaultPartSize              = 8 * 1024 * 1024
	defaultRetryMaxAttempts      = 3
)

// ProviderConfig identifies one bucket. Credential values must come from a
// native secret adapter and are never persisted or logged by this package.
type ProviderConfig struct {
	Endpoint        string
	Region          string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	ForcePathStyle  bool
}

// Options bounds provider retries, multipart workers, and aggregate calls.
type Options struct {
	MaxConcurrentRequests int
	TransferConcurrency   int
	RetryMaxAttempts      int
	MultipartThreshold    int64
	PartSize              int64
}

// Store is safe for concurrent use.
type Store struct {
	client   *s3.Client
	uploader *transfermanager.Client
	bucket   *string
	limiter  chan struct{}
}

// New constructs an S3-compatible store using an isolated AWS SDK config.
func New(ctx context.Context, provider ProviderConfig, options Options) (*Store, error) {
	if err := validateProviderConfig(provider); err != nil {
		return nil, invalid("configure", err)
	}
	resolved, err := resolveOptions(options)
	if err != nil {
		return nil, invalid("configure", err)
	}

	loadOptions := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(provider.Region),
		awsconfig.WithRetryMaxAttempts(resolved.RetryMaxAttempts),
	}
	if provider.AccessKeyID != "" {
		loadOptions = append(loadOptions, awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			provider.AccessKeyID,
			provider.SecretAccessKey,
			provider.SessionToken,
		)))
	}

	awsConfig, err := awsconfig.LoadDefaultConfig(ctx, loadOptions...)
	if err != nil {
		return nil, classify("configure", err, false)
	}
	client := s3.NewFromConfig(awsConfig, func(s3Options *s3.Options) {
		s3Options.UsePathStyle = provider.ForcePathStyle
		if provider.Endpoint != "" {
			s3Options.BaseEndpoint = aws.String(provider.Endpoint)
		}
	})
	return newStore(client, provider.Bucket, resolved), nil
}

func newStore(client *s3.Client, bucket string, options Options) *Store {
	uploader := transfermanager.New(client, func(transferOptions *transfermanager.Options) {
		transferOptions.Concurrency = options.TransferConcurrency
		transferOptions.MultipartUploadThreshold = options.MultipartThreshold
		transferOptions.PartSizeBytes = options.PartSize
	})
	return &Store{
		client:   client,
		uploader: uploader,
		bucket:   aws.String(bucket),
		limiter:  make(chan struct{}, options.MaxConcurrentRequests),
	}
}

func validateProviderConfig(provider ProviderConfig) error {
	if strings.TrimSpace(provider.Region) == "" {
		return errors.New("region is required")
	}
	if strings.TrimSpace(provider.Bucket) == "" {
		return errors.New("bucket is required")
	}
	if (provider.AccessKeyID == "") != (provider.SecretAccessKey == "") {
		return errors.New("access key ID and secret access key must be supplied together")
	}
	return validateEndpoint(provider.Endpoint)
}

func validateEndpoint(endpoint string) error {
	if endpoint == "" {
		return nil
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("parse endpoint: %w", err)
	}
	return validateParsedEndpoint(parsed)
}

var endpointValidators = []func(*url.URL) error{
	validateEndpointLocation,
	validateEndpointContents,
	validateEndpointEncryption,
}

var endpointSchemes = map[string]struct{}{
	"http":  {},
	"https": {},
}

func validateParsedEndpoint(endpoint *url.URL) error {
	for _, validate := range endpointValidators {
		if err := validate(endpoint); err != nil {
			return err
		}
	}
	return nil
}

func validateEndpointLocation(endpoint *url.URL) error {
	_, supportedScheme := endpointSchemes[endpoint.Scheme]
	if !supportedScheme || endpoint.Host == "" {
		return errors.New("endpoint must be an absolute HTTP or HTTPS URL")
	}
	return nil
}

func validateEndpointContents(endpoint *url.URL) error {
	if endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return errors.New("endpoint cannot contain credentials, a query, or a fragment")
	}
	return nil
}

func validateEndpointEncryption(endpoint *url.URL) error {
	if endpoint.Scheme == "http" && !isLoopbackHost(endpoint.Hostname()) {
		return errors.New("unencrypted endpoints are allowed only on loopback")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && address.IsLoopback()
}

func resolveOptions(options Options) (Options, error) {
	if options.MaxConcurrentRequests == 0 {
		options.MaxConcurrentRequests = defaultMaxConcurrentRequests
	}
	if options.TransferConcurrency == 0 {
		options.TransferConcurrency = defaultTransferConcurrency
	}
	if options.RetryMaxAttempts == 0 {
		options.RetryMaxAttempts = defaultRetryMaxAttempts
	}
	if options.MultipartThreshold == 0 {
		options.MultipartThreshold = defaultMultipartThreshold
	}
	if options.PartSize == 0 {
		options.PartSize = defaultPartSize
	}
	if options.MaxConcurrentRequests < 1 || options.TransferConcurrency < 1 || options.RetryMaxAttempts < 1 {
		return Options{}, errors.New("concurrency and retry limits must be positive")
	}
	if options.PartSize < 5*1024*1024 {
		return Options{}, errors.New("multipart part size must be at least 5 MiB")
	}
	if options.MultipartThreshold < options.PartSize {
		return Options{}, errors.New("multipart threshold cannot be smaller than part size")
	}
	return options, nil
}

func (store *Store) acquire(ctx context.Context, operation string) (func(), error) {
	select {
	case store.limiter <- struct{}{}:
		return func() { <-store.limiter }, nil
	case <-ctx.Done():
		return nil, classify(operation, ctx.Err(), false)
	}
}
