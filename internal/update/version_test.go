package update

import "testing"

func TestParseVersion(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{name: "plain", raw: "0.1.0", want: "0.1.0"},
		{name: "leading v", raw: "v1.2.3", want: "1.2.3"},
		{name: "leading V", raw: "V1.2.3", want: "1.2.3"},
		{name: "spaces", raw: "  v1.2.3  ", want: "1.2.3"},
		{name: "prerelease", raw: "v1.2.3-beta.1", want: "1.2.3-beta.1"},
		{name: "build", raw: "v1.2.3+ci.5", want: "1.2.3+ci.5"},
		{name: "prerelease and build", raw: "v1.2.3-rc.2+sha.abc", want: "1.2.3-rc.2+sha.abc"},
		{name: "hyphenated prerelease", raw: "1.2.3-beta-2", want: "1.2.3-beta-2"},
		{name: "leading zero build", raw: "1.2.3+build.01", want: "1.2.3+build.01"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			version, err := ParseVersion(tc.raw)
			if err != nil {
				t.Fatalf("ParseVersion(%q): %v", tc.raw, err)
			}
			if got := version.String(); got != tc.want {
				t.Fatalf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseVersionRejectsInvalid(t *testing.T) {
	for _, raw := range []string{
		"",
		"v",
		"dev",
		"1",
		"1.2",
		"1.2.3.4",
		"1.2.x",
		"01.2.3",
		"1.02.3",
		"1.2.03",
		"1.2.3-",
		"1.2.3+",
		"1.2.3-01",
		"1.2.3-alpha..1",
		"1.2.3-alpha.",
		"1.2.3+build..1",
		"1.2.3-alpha_1",
		"1.2.3-beta+one+two",
		"1.2.3-βeta",
		"999999999999999999999.0.0",
	} {
		if version, err := ParseVersion(raw); err == nil {
			t.Fatalf("ParseVersion(%q) = %v, want an error", raw, version)
		}
	}
}

func TestVersionCompare(t *testing.T) {
	for _, tc := range []struct {
		name  string
		left  string
		right string
		want  int
	}{
		{name: "equal", left: "1.2.3", right: "v1.2.3", want: 0},
		{name: "build ignored", left: "1.2.3+one", right: "1.2.3+two", want: 0},
		{name: "major", left: "2.0.0", right: "1.9.9", want: 1},
		{name: "minor", left: "1.10.0", right: "1.9.0", want: 1},
		{name: "patch", left: "1.0.1", right: "1.0.0", want: 1},
		{name: "stable over prerelease", left: "1.0.0", right: "1.0.0-rc.1", want: 1},
		{name: "shorter prerelease", left: "1.0.0-alpha", right: "1.0.0-alpha.1", want: -1},
		{name: "numeric before alphanumeric", left: "1.0.0-1", right: "1.0.0-alpha", want: -1},
		{name: "numeric order", left: "1.0.0-alpha.2", right: "1.0.0-alpha.11", want: -1},
		{name: "alphanumeric order", left: "1.0.0-alpha.beta", right: "1.0.0-beta", want: -1},
		{name: "rc after beta", left: "1.0.0-rc.1", right: "1.0.0-beta.11", want: 1},
		{name: "semver example chain", left: "1.0.0-beta", right: "1.0.0-alpha.1", want: 1},
		{name: "uppercase lexical", left: "1.0.0-Beta", right: "1.0.0-alpha", want: -1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			left, err := ParseVersion(tc.left)
			if err != nil {
				t.Fatalf("ParseVersion(%q): %v", tc.left, err)
			}
			right, err := ParseVersion(tc.right)
			if err != nil {
				t.Fatalf("ParseVersion(%q): %v", tc.right, err)
			}

			if got := left.Compare(right); got != tc.want {
				t.Fatalf("%s.Compare(%s) = %d, want %d", tc.left, tc.right, got, tc.want)
			}
			if got := right.Compare(left); got != -tc.want {
				t.Fatalf("%s.Compare(%s) = %d, want %d", tc.right, tc.left, got, -tc.want)
			}
		})
	}
}

func TestVersionChannel(t *testing.T) {
	stable, err := ParseVersion("v0.1.0")
	if err != nil {
		t.Fatalf("ParseVersion stable: %v", err)
	}
	if stable.IsPrerelease() || stable.Channel() != "" {
		t.Fatalf("stable = %+v, want no prerelease channel", stable)
	}

	beta, err := ParseVersion("v0.2.0-beta.3")
	if err != nil {
		t.Fatalf("ParseVersion beta: %v", err)
	}
	if !beta.IsPrerelease() || beta.Channel() != "beta" {
		t.Fatalf("beta = %+v, want channel beta", beta)
	}
}
