package main

import "testing"

func TestRejectInvalidFixtureOptions(t *testing.T) {
	for _, tc := range []struct{ resolution, port, seconds int }{
		{resolution: 480, port: 18765, seconds: 30},
		{resolution: 720, port: 80, seconds: 30},
		{resolution: 1080, port: 65536, seconds: 30},
		{resolution: 720, port: 18765, seconds: 601},
		{resolution: 720, port: 18765, seconds: 0},
	} {
		if err := run(tc.resolution, tc.port, tc.seconds, false); err == nil {
			t.Fatal("invalid options accepted")
		}
	}
}
