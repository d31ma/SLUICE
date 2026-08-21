package contractsv1_test

import (
	"context"
	"testing"
	"time"

	contractsv1 "github.com/d31ma/sluice/contracts/v1"
)

func TestSectionThreeInterfacesRemainImplementable(t *testing.T) {
	var _ contractsv1.StateStore = stateStoreContract{}
	var _ contractsv1.ObjectStore = objectStoreContract{}
	var _ contractsv1.PlatformAdapter = platformContract{}
	var _ contractsv1.SyncEngine = syncEngineContract{}
}

type stateStoreContract struct{}

func (stateStoreContract) Open(context.Context) error  { return nil }
func (stateStoreContract) Close(context.Context) error { return nil }
func (stateStoreContract) LoadRoot(context.Context, string) (contractsv1.Root, error) {
	return contractsv1.Root{}, nil
}
func (stateStoreContract) UpsertEntries(context.Context, []contractsv1.Entry) error { return nil }
func (stateStoreContract) PageEntries(context.Context, contractsv1.EntryQuery, contractsv1.Page) (contractsv1.EntryPage, error) {
	return contractsv1.EntryPage{}, nil
}
func (stateStoreContract) Enqueue(context.Context, contractsv1.Operation) error { return nil }
func (stateStoreContract) Claim(context.Context, string, int, time.Duration) ([]contractsv1.Operation, error) {
	return nil, nil
}

func (stateStoreContract) Complete(context.Context, string, string, contractsv1.OperationResult) error {
	return nil
}

func (stateStoreContract) Retry(context.Context, string, string, contractsv1.Failure, time.Time) error {
	return nil
}
func (stateStoreContract) RecordConflict(context.Context, contractsv1.Conflict) error { return nil }

type objectStoreContract struct{}

func (objectStoreContract) Probe(context.Context) (contractsv1.Capabilities, error) {
	return contractsv1.Capabilities{}, nil
}

func (objectStoreContract) List(context.Context, string, string, int) (contractsv1.ObjectPage, error) {
	return contractsv1.ObjectPage{}, nil
}

func (objectStoreContract) Head(context.Context, string) (contractsv1.RemoteObject, error) {
	return contractsv1.RemoteObject{}, nil
}

func (objectStoreContract) Get(context.Context, contractsv1.DownloadRequest) (contractsv1.DownloadResult, error) {
	return contractsv1.DownloadResult{}, nil
}

func (objectStoreContract) Put(context.Context, contractsv1.UploadRequest) (contractsv1.UploadResult, error) {
	return contractsv1.UploadResult{}, nil
}

func (objectStoreContract) Delete(context.Context, contractsv1.DeleteRequest) (contractsv1.DeleteResult, error) {
	return contractsv1.DeleteResult{}, nil
}

func (objectStoreContract) CompareAndSwap(context.Context, contractsv1.CASRequest) (contractsv1.CASResult, error) {
	return contractsv1.CASResult{}, nil
}

type platformContract struct{}

func (platformContract) CanonicalizeRoot(context.Context, string) (contractsv1.CanonicalRoot, error) {
	return contractsv1.CanonicalRoot{}, nil
}

func (platformContract) Scan(context.Context, contractsv1.CanonicalRoot, contractsv1.IgnoreRules) (<-chan contractsv1.ScanEvent, <-chan error) {
	events := make(chan contractsv1.ScanEvent)
	errs := make(chan error)
	close(events)
	close(errs)
	return events, errs
}

func (platformContract) Watch(context.Context, contractsv1.CanonicalRoot) (contractsv1.Watcher, error) {
	return nil, nil
}

func (platformContract) Materialize(context.Context, contractsv1.MaterializeRequest) error {
	return nil
}
func (platformContract) Evict(context.Context, contractsv1.EvictRequest) error { return nil }
func (platformContract) SetStartAtLogin(context.Context, bool) error           { return nil }

type syncEngineContract struct{}

func (syncEngineContract) Reconcile(context.Context, string, contractsv1.ReconcileReason) (contractsv1.RunID, error) {
	return "", nil
}
func (syncEngineContract) Pause(context.Context, string) error  { return nil }
func (syncEngineContract) Resume(context.Context, string) error { return nil }
func (syncEngineContract) ResolveConflict(context.Context, string, contractsv1.Resolution) error {
	return nil
}

func (syncEngineContract) Status(context.Context, string) (contractsv1.RootStatus, error) {
	return contractsv1.RootStatus{}, nil
}
