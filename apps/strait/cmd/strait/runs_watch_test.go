package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWatchRunUntilDone_Completed(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n == 1 {
			fmt.Fprint(w, `{"id":"run-1","status":"executing","attempt":1}`)
		} else {
			fmt.Fprint(w, `{"id":"run-1","status":"completed","attempt":1}`)
		}
	}))
	defer srv.Close()

	state := &appState{opts: &rootOptions{serverURL: srv.URL, outputFormat: "json"}}
	err := watchRunUntilDone(context.Background(), state, "run-1", 10*time.Millisecond, 5*time.Second)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestWatchRunUntilDone_Failed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"run-1","status":"failed","attempt":1}`)
	}))
	defer srv.Close()

	state := &appState{opts: &rootOptions{serverURL: srv.URL, outputFormat: "json"}}
	err := watchRunUntilDone(context.Background(), state, "run-1", 10*time.Millisecond, 5*time.Second)
	if err == nil {
		t.Fatal("expected error for failed run")
	}
	if !strings.Contains(err.Error(), "terminal status") {
		t.Fatalf("expected 'terminal status' in error, got: %v", err)
	}
}

func TestWatchRunUntilDone_Timeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"run-1","status":"executing","attempt":1}`)
	}))
	defer srv.Close()

	state := &appState{opts: &rootOptions{serverURL: srv.URL, outputFormat: "json"}}
	err := watchRunUntilDone(context.Background(), state, "run-1", 20*time.Millisecond, 100*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if !strings.Contains(err.Error(), "watch timeout reached") {
		t.Fatalf("expected 'watch timeout reached' in error, got: %v", err)
	}
}

func TestWatchRunUntilDone_ContextCanceled(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"run-1","status":"executing","attempt":1}`)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	state := &appState{opts: &rootOptions{serverURL: srv.URL, outputFormat: "json"}}
	err := watchRunUntilDone(ctx, state, "run-1", 10*time.Millisecond, 5*time.Second)
	if err == nil {
		t.Fatal("expected context canceled error")
	}
}

func TestWatchRunUntilDone_PrintsEachPoll(t *testing.T) {
	t.Parallel()

	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := callCount.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n < 3 {
			fmt.Fprint(w, `{"id":"run-1","status":"executing","attempt":1}`)
		} else {
			fmt.Fprint(w, `{"id":"run-1","status":"completed","attempt":1}`)
		}
	}))
	defer srv.Close()

	state := &appState{opts: &rootOptions{serverURL: srv.URL, outputFormat: "json"}}
	err := watchRunUntilDone(context.Background(), state, "run-1", 10*time.Millisecond, 5*time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount.Load() != 3 {
		t.Fatalf("expected 3 polls, got %d", callCount.Load())
	}
}
