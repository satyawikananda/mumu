package libmpv

import "testing"

func TestRenderingEnabled(t *testing.T) {
	tests := []struct {
		name   string
		config Config
		want   bool
	}{
		{name: "headless default", config: DefaultConfig(), want: false},
		{name: "zero value", config: Config{}, want: false},
		{name: "null video output", config: Config{VideoOutput: "null"}, want: false},
		{name: "libmpv video output", config: Config{VideoOutput: "libmpv"}, want: true},
		{name: "case insensitive", config: Config{VideoOutput: " LIBMPV "}, want: true},
		{name: "other video output", config: Config{VideoOutput: "gpu"}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := renderingEnabled(test.config); got != test.want {
				t.Fatalf("renderingEnabled(%+v) = %v, want %v", test.config, got, test.want)
			}
		})
	}
}
