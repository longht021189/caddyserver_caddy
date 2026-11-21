package tarpit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
)

func provisioned(t *testing.T, tp *Tarpit) {
	t.Helper()
	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	t.Cleanup(cancel)
	if err := tp.Provision(ctx); err != nil {
		t.Fatal(err)
	}
}

func serve(tp *Tarpit, ctx context.Context) <-chan any {
	done := make(chan any, 1)
	go func() {
		defer func() { done <- recover() }()
		req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(ctx)
		_ = tp.ServeHTTP(httptest.NewRecorder(), req, nil)
	}()
	return done
}

func TestHoldsUntilClientGivesUp(t *testing.T) {
	tp := &Tarpit{}
	provisioned(t, tp)
	ctx, cancel := context.WithCancel(context.Background())
	done := serve(tp, ctx)

	select {
	case <-done:
		t.Fatal("tarpit returned before the client gave up")
	case <-time.After(200 * time.Millisecond):
	}
	cancel()
	select {
	case got := <-done:
		if got != http.ErrAbortHandler {
			t.Fatalf("expected abort, got %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("tarpit did not release after client cancel")
	}
}

func TestReleasesAfterDuration(t *testing.T) {
	tp := &Tarpit{Duration: caddy.Duration(50 * time.Millisecond)}
	provisioned(t, tp)
	select {
	case got := <-serve(tp, context.Background()):
		if got != http.ErrAbortHandler {
			t.Fatalf("expected abort, got %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("tarpit ignored duration")
	}
}

func TestAbortsImmediatelyWhenFull(t *testing.T) {
	tp := &Tarpit{MaxConcurrent: 1}
	provisioned(t, tp)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := serve(tp, ctx)
	time.Sleep(50 * time.Millisecond)

	select {
	case got := <-serve(tp, context.Background()):
		if got != http.ErrAbortHandler {
			t.Fatalf("expected abort, got %v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("second request was held although the tarpit was full")
	}
	select {
	case <-first:
		t.Fatal("first request was released early")
	default:
	}
}
