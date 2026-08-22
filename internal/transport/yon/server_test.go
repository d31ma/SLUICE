package yon

import (
	"errors"
	"testing"
	"time"
)

func TestServerDoneAndErrRetainTheWaitResult(t *testing.T) {
	want := errors.New("exit status 23")
	server := newServer(nil, func() error { return want })

	select {
	case <-server.Done():
	case <-time.After(time.Second):
		t.Fatal("Done() did not report process exit")
	}
	if first, second := server.Err(), server.Err(); !errors.Is(first, want) || !errors.Is(second, want) {
		t.Fatalf("Err() = %v then %v, want retained %v", first, second, want)
	}
}

func TestServerErrIsNilBeforeWaitCompletes(t *testing.T) {
	release := make(chan struct{})
	server := newServer(nil, func() error {
		<-release
		return nil
	})
	if err := server.Err(); err != nil {
		t.Fatalf("Err() before exit = %v, want nil", err)
	}
	close(release)
	<-server.Done()
	if err := server.Err(); err != nil {
		t.Fatalf("Err() after successful exit = %v, want nil", err)
	}
}
