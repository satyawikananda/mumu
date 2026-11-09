package app

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDispatcher records updates and runs them synchronously.
type fakeDispatcher struct {
	mu      sync.Mutex
	updates int
}

func (f *fakeDispatcher) Update(fn func()) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.updates++
	fn()
}

func (f *fakeDispatcher) Invalidate() {}

// waitUpdates waits for n updates, ordering later reads after their writes.
func (f *fakeDispatcher) waitUpdates(t *testing.T, n int) {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		got := f.updates
		f.mu.Unlock()

		if got >= n {
			return
		}
		time.Sleep(time.Millisecond)
	}

	t.Fatalf("dispatcher received %d updates, want at least %d", f.updates, n)
}

func newTestApp() (*App, *fakeDispatcher) {
	a := New()
	f := &fakeDispatcher{}
	a.dispatcher = f

	return a, f
}

func TestGoReportsError(t *testing.T) {
	a, f := newTestApp()

	a.Go(func(context.Context) error { return errors.New("stream failed") })
	f.waitUpdates(t, 1)

	if got := a.shell.ErrorMessage(); got != "stream failed" {
		t.Fatalf("error message %q, want %q", got, "stream failed")
	}
}

func TestGoRecoversPanic(t *testing.T) {
	a, f := newTestApp()

	a.Go(func(context.Context) error { panic("boom") })
	f.waitUpdates(t, 1)

	if got := a.shell.ErrorMessage(); !strings.Contains(got, "panic") || !strings.Contains(got, "boom") {
		t.Fatalf("error message %q, want a recovered panic", got)
	}
}

func TestStopCancelsJobs(t *testing.T) {
	a, _ := newTestApp()

	finished := make(chan struct{})
	a.Go(func(ctx context.Context) error {
		<-ctx.Done()
		close(finished)

		return ctx.Err()
	})

	a.Stop()

	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("job did not observe cancellation")
	}

	// Canceled jobs are normal shutdown, not user-facing errors.
	if got := a.shell.ErrorMessage(); got != "" {
		t.Fatalf("error message %q after cancellation, want none", got)
	}
}

func TestStopIsIdempotent(_ *testing.T) {
	a, _ := newTestApp()

	a.Stop()
	a.Stop()
}

func TestGoAfterStopDoesNothing(t *testing.T) {
	a, _ := newTestApp()
	a.Stop()

	ran := make(chan struct{}, 1)
	a.Go(func(context.Context) error {
		ran <- struct{}{}

		return nil
	})

	select {
	case <-ran:
		t.Fatal("job ran after Stop")
	case <-time.After(20 * time.Millisecond):
	}
}

func TestFailIgnoresNil(t *testing.T) {
	a, _ := newTestApp()

	a.Fail(nil)

	if got := a.shell.ErrorMessage(); got != "" {
		t.Fatalf("error message %q, want none", got)
	}
}
