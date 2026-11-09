package main

import (
	"errors"
	"testing"
)

func TestTransfer(t *testing.T) {
	for _, tc := range []struct {
		name           string
		from, to, want presentation
		fail, wantErr  bool
		calls          int
	}{
		{name: "enter", from: mainPresentation, to: pipPresentation, want: pipPresentation, calls: 1},
		{name: "return", from: pipPresentation, to: mainPresentation, want: mainPresentation, calls: 1},
		{name: "rollback", from: mainPresentation, to: pipPresentation, want: mainPresentation, fail: true, wantErr: true, calls: 1},
		{name: "duplicate", from: pipPresentation, to: pipPresentation, want: pipPresentation},
		{name: "stopped", from: stopped, to: pipPresentation, want: stopped, wantErr: true},
		{name: "busy", from: transitioning, to: mainPresentation, want: transitioning, wantErr: true},
		{name: "invalid", from: mainPresentation, to: stopped, want: mainPresentation, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			current := tc.from
			calls := 0
			err := transfer(&current, tc.to, func() error {
				calls++
				if current != transitioning {
					t.Fatal("attachment outside transition")
				}
				if tc.fail {
					return errors.New("native failure")
				}
				return nil
			})
			if (err != nil) != tc.wantErr || current != tc.want || calls != tc.calls {
				t.Fatalf("state=%s calls=%d err=%v", current, calls, err)
			}
		})
	}
}
