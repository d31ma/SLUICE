package contractsv1

import (
	"context"
	"io"
	"time"

	"github.com/d31ma/sluice/internal/domain"
)

type StateStore interface {
	Open(ctx context.Context) error
	Close(ctx context.Context) error
	LoadRoot(ctx context.Context, rootID string) (Root, error)
	UpsertEntries(ctx context.Context, entries []Entry) error
	PageEntries(ctx context.Context, query EntryQuery, page Page) (EntryPage, error)
	Enqueue(ctx context.Context, operation Operation) error
	Claim(ctx context.Context, workerID string, limit int, lease time.Duration) ([]Operation, error)
	Complete(ctx context.Context, operationID, receipt string, result OperationResult) error
	Retry(ctx context.Context, operationID, receipt string, failure Failure, at time.Time) error
	RecordConflict(ctx context.Context, conflict Conflict) error
}

type ObjectStore interface {
	Probe(ctx context.Context) (Capabilities, error)
	List(ctx context.Context, prefix, cursor string, limit int) (ObjectPage, error)
	Head(ctx context.Context, key string) (RemoteObject, error)
	Get(ctx context.Context, request DownloadRequest) (DownloadResult, error)
	Put(ctx context.Context, request UploadRequest) (UploadResult, error)
	Delete(ctx context.Context, request DeleteRequest) (DeleteResult, error)
	CompareAndSwap(ctx context.Context, request CASRequest) (CASResult, error)
}

type PlatformAdapter interface {
	CanonicalizeRoot(ctx context.Context, path string) (CanonicalRoot, error)
	Scan(ctx context.Context, root CanonicalRoot, rules IgnoreRules) (<-chan ScanEvent, <-chan error)
	Watch(ctx context.Context, root CanonicalRoot) (Watcher, error)
	Materialize(ctx context.Context, request MaterializeRequest) error
	Evict(ctx context.Context, request EvictRequest) error
	SetStartAtLogin(ctx context.Context, enabled bool) error
}

type SyncEngine interface {
	Reconcile(ctx context.Context, rootID string, reason ReconcileReason) (RunID, error)
	Pause(ctx context.Context, rootID string) error
	Resume(ctx context.Context, rootID string) error
	ResolveConflict(ctx context.Context, conflictID string, resolution Resolution) error
	Status(ctx context.Context, rootID string) (RootStatus, error)
}

type Failure = domain.Failure

type Root struct {
	ID        string
	ProfileID string
	Path      string
	Paused    bool
}

type Entry struct {
	ModifiedAt  time.Time
	RootID      string
	LogicalPath string
	Kind        EntryKind
	ContentHash string
	Revision    string
	Size        int64
}

type EntryKind string

const (
	EntryKindFile      EntryKind = "file"
	EntryKindDirectory EntryKind = "directory"
)

type EntryQuery struct {
	RootID     string
	PathPrefix string
}

type Page struct {
	Cursor string
	Limit  int
}

type EntryPage struct {
	NextCursor string
	Entries    []Entry
}

type Operation struct {
	ID             string
	RootID         string
	Kind           OperationKind
	LogicalPath    string
	IdempotencyKey string
	Attempt        int
}

type OperationKind string

const (
	OperationUpload   OperationKind = "upload"
	OperationDownload OperationKind = "download"
	OperationDelete   OperationKind = "delete"
)

type OperationResult struct {
	Revision  string
	ETag      string
	VersionID string
}

type Conflict struct {
	DetectedAt     time.Time
	ID             string
	RootID         string
	LogicalPath    string
	LocalRevision  string
	RemoteRevision string
}

type Capabilities struct {
	ConditionalWrites bool
	RangeDownloads    bool
	MultipartUploads  bool
	ObjectVersioning  bool
	ChecksumSHA256    bool
}

type ObjectPage struct {
	NextCursor string
	Objects    []RemoteObject
}

type RemoteObject struct {
	Key       string
	ETag      string
	VersionID string
	SHA256    string
	Size      int64
}

type ByteRange struct {
	Offset int64
	Length int64
}

type DownloadRequest struct {
	Writer    io.Writer
	Range     *ByteRange
	Key       string
	VersionID string
}

type DownloadResult struct {
	Object       RemoteObject
	BytesWritten int64
}

type UploadRequest struct {
	Body           io.Reader
	Key            string
	SHA256         string
	ContentType    string
	IdempotencyKey string
	Size           int64
}

type UploadResult struct {
	Object RemoteObject
}

type DeleteRequest struct {
	Key            string
	VersionID      string
	IdempotencyKey string
}

type DeleteResult struct {
	DeletedVersionID string
}

type CASRequest struct {
	Body           io.Reader
	Key            string
	ContentType    string
	ExpectedETag   string
	IdempotencyKey string
	Size           int64
	ExpectedAbsent bool
}

type CASResult struct {
	Object  RemoteObject
	Applied bool
}

type CanonicalRoot struct {
	Path       string
	Filesystem string
	Identity   string
}

type IgnoreRules struct {
	Version  string
	Patterns []string
}

type ScanEvent struct {
	ModifiedAt  time.Time
	LogicalPath string
	Kind        EntryKind
	Size        int64
}

type WatchEvent struct {
	LogicalPath string
	Kind        WatchEventKind
}

type WatchEventKind string

const (
	WatchEventChanged  WatchEventKind = "changed"
	WatchEventRemoved  WatchEventKind = "removed"
	WatchEventOverflow WatchEventKind = "overflow"
)

type Watcher interface {
	Events() <-chan WatchEvent
	Errors() <-chan error
	Close() error
}

type MaterializeRequest struct {
	Body         io.Reader
	Root         CanonicalRoot
	LogicalPath  string
	ExpectedHash string
	ExpectedSize int64
	Mode         uint32
}

type EvictRequest struct {
	Root        CanonicalRoot
	LogicalPath string
}

type ReconcileReason string

const (
	ReconcileManual          ReconcileReason = "manual"
	ReconcileWatcher         ReconcileReason = "watcher"
	ReconcilePeriodic        ReconcileReason = "periodic"
	ReconcileRecovery        ReconcileReason = "recovery"
	ReconcileWatcherOverflow ReconcileReason = "watcher_overflow"
)

type RunID string

type Resolution string

const (
	ResolutionKeepBoth   Resolution = "keep_both"
	ResolutionKeepLocal  Resolution = "keep_local"
	ResolutionKeepRemote Resolution = "keep_remote"
)

type RootStatus struct {
	LastReconciledAt  time.Time
	RootID            string
	State             RootState
	PendingOperations int
	ConflictCount     int
}

type RootState string

const (
	RootStateIdle     RootState = "idle"
	RootStateSyncing  RootState = "syncing"
	RootStatePaused   RootState = "paused"
	RootStateDegraded RootState = "degraded"
)
