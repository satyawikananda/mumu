package video

import (
	"errors"
	"testing"
)

// Transfer commits only a successful attachment and rolls back a failure.
func TestTransfer(t *testing.T) {
	for _, tc := range []struct {
		name          string
		from, to      Presentation
		want          Presentation
		fail          bool
		wantErr       error
		calls         int
		wantDuringRun Presentation
	}{
		{name: "enter", from: PresentationMain, to: PresentationPiP, want: PresentationPiP, calls: 1},
		{name: "return", from: PresentationPiP, to: PresentationMain, want: PresentationMain, calls: 1},
		{
			name:    "rollback",
			from:    PresentationMain,
			to:      PresentationPiP,
			want:    PresentationMain,
			fail:    true,
			wantErr: errors.New("attach"),
			calls:   1,
		},
		{name: "duplicate", from: PresentationPiP, to: PresentationPiP, want: PresentationPiP},
		{name: "stopped", from: PresentationStopped, to: PresentationPiP, want: PresentationStopped, wantErr: ErrPresentationStopped},
		{name: "busy", from: PresentationTransitioning, to: PresentationMain, want: PresentationTransitioning, wantErr: ErrPresentationBusy},
		{name: "invalid", from: PresentationMain, to: PresentationStopped, want: PresentationMain, wantErr: ErrPresentationTarget},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := tc.from
			calls := 0

			err := Transfer(&current, tc.to, func() error {
				calls++
				if current != PresentationTransitioning {
					t.Fatal("attachment ran outside a transition")
				}
				if tc.fail {
					return errors.New("native failure")
				}

				return nil
			})

			if (tc.wantErr != nil) != (err != nil) {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if current != tc.want {
				t.Fatalf("state = %s, want %s", current, tc.want)
			}
			if calls != tc.calls {
				t.Fatalf("attach calls = %d, want %d", calls, tc.calls)
			}
		})
	}
}

// Rapid toggles stay sequential, and the last successful target wins.
func TestTransferRapidToggles(t *testing.T) {
	current := PresentationMain
	calls := 0

	for i := range 20 {
		target := PresentationPiP
		if i%2 == 1 {
			target = PresentationMain
		}
		if err := Transfer(&current, target, func() error {
			calls++

			return nil
		}); err != nil {
			t.Fatalf("toggle %d: %v", i, err)
		}
	}

	if calls != 20 {
		t.Fatalf("attach calls = %d, want 20", calls)
	}
	if current != PresentationMain {
		t.Fatalf("state = %s, want main after an even number of toggles", current)
	}
}

// A failed transfer during rapid toggling keeps the previous presentation.
func TestTransferFailureKeepsPrevious(t *testing.T) {
	current := PresentationPiP

	err := Transfer(&current, PresentationMain, func() error {
		return errors.New("surface unavailable")
	})
	if err == nil {
		t.Fatal("expected an attach failure")
	}
	if current != PresentationPiP {
		t.Fatalf("state = %s, want pip", current)
	}

	if err := Transfer(&current, PresentationMain, func() error { return nil }); err != nil {
		t.Fatalf("recovery transfer: %v", err)
	}
	if current != PresentationMain {
		t.Fatalf("state = %s, want main", current)
	}
}
