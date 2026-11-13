package update

import (
	"fmt"
	"strconv"
	"strings"
)

// Version is a parsed semantic version. Build metadata is kept for display and
// ignored when comparing versions.
type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease []string
	Build      []string
}

// ParseVersion parses vMAJOR.MINOR.PATCH with optional prerelease and build
// metadata. A leading "v" or "V" is accepted.
func ParseVersion(raw string) (Version, error) {
	text := strings.TrimSpace(raw)
	if len(text) > 0 && (text[0] == 'v' || text[0] == 'V') {
		text = text[1:]
	}
	if text == "" {
		return Version{}, fmt.Errorf("invalid semantic version %q", raw)
	}

	core, build, hasBuild := strings.Cut(text, "+")
	core, prerelease, hasPrerelease := strings.Cut(core, "-")

	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return Version{}, fmt.Errorf("invalid semantic version %q", raw)
	}

	numbers := make([]int, 3)
	for i, part := range parts {
		number, ok := parseCoreNumber(part)
		if !ok {
			return Version{}, fmt.Errorf("invalid semantic version %q", raw)
		}
		numbers[i] = number
	}

	version := Version{Major: numbers[0], Minor: numbers[1], Patch: numbers[2]}
	if hasPrerelease {
		identifiers, ok := parseIdentifiers(prerelease, false)
		if !ok {
			return Version{}, fmt.Errorf("invalid semantic version %q", raw)
		}
		version.Prerelease = identifiers
	}
	if hasBuild {
		identifiers, ok := parseIdentifiers(build, true)
		if !ok {
			return Version{}, fmt.Errorf("invalid semantic version %q", raw)
		}
		version.Build = identifiers
	}

	return version, nil
}

// String returns the canonical version without a leading "v".
func (v Version) String() string {
	text := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if len(v.Prerelease) > 0 {
		text += "-" + strings.Join(v.Prerelease, ".")
	}
	if len(v.Build) > 0 {
		text += "+" + strings.Join(v.Build, ".")
	}

	return text
}

// IsPrerelease reports whether the version carries a prerelease suffix.
func (v Version) IsPrerelease() bool { return len(v.Prerelease) > 0 }

// Channel returns the first prerelease identifier, which names the release
// channel: "beta" for 1.0.0-beta.2. A stable version has no channel.
func (v Version) Channel() string {
	if len(v.Prerelease) == 0 {
		return ""
	}

	return v.Prerelease[0]
}

// Compare returns -1, 0 or 1 when v sorts before, equal to or after other.
// Build metadata does not affect ordering.
func (v Version) Compare(other Version) int {
	if order := compareNumber(v.Major, other.Major); order != 0 {
		return order
	}
	if order := compareNumber(v.Minor, other.Minor); order != 0 {
		return order
	}
	if order := compareNumber(v.Patch, other.Patch); order != 0 {
		return order
	}

	vPre, otherPre := v.IsPrerelease(), other.IsPrerelease()
	switch {
	case !vPre && !otherPre:
		return 0
	case !vPre:
		return 1
	case !otherPre:
		return -1
	}

	shared := min(len(v.Prerelease), len(other.Prerelease))
	for i := 0; i < shared; i++ {
		if order := compareIdentifier(v.Prerelease[i], other.Prerelease[i]); order != 0 {
			return order
		}
	}

	return compareNumber(len(v.Prerelease), len(other.Prerelease))
}

func parseCoreNumber(text string) (int, bool) {
	if text == "" || (len(text) > 1 && text[0] == '0') {
		return 0, false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return 0, false
		}
	}

	number, err := strconv.Atoi(text)
	if err != nil {
		return 0, false
	}

	return number, true
}

func parseIdentifiers(text string, allowLeadingZeros bool) ([]string, bool) {
	parts := strings.Split(text, ".")
	for _, part := range parts {
		if part == "" || !validIdentifier(part) {
			return nil, false
		}
		if !allowLeadingZeros && isNumeric(part) && len(part) > 1 && part[0] == '0' {
			return nil, false
		}
	}

	return parts, true
}

func validIdentifier(text string) bool {
	for i := 0; i < len(text); i++ {
		character := text[i]
		switch {
		case character >= '0' && character <= '9':
		case character >= 'a' && character <= 'z':
		case character >= 'A' && character <= 'Z':
		case character == '-':
		default:
			return false
		}
	}

	return true
}

func isNumeric(text string) bool {
	if text == "" {
		return false
	}
	for i := 0; i < len(text); i++ {
		if text[i] < '0' || text[i] > '9' {
			return false
		}
	}

	return true
}

func compareIdentifier(a, b string) int {
	aNumeric, bNumeric := isNumeric(a), isNumeric(b)
	switch {
	case aNumeric && bNumeric:
		return compareNumericString(a, b)
	case aNumeric:
		return -1
	case bNumeric:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

func compareNumericString(a, b string) int {
	if len(a) != len(b) {
		return compareNumber(len(a), len(b))
	}

	return strings.Compare(a, b)
}

func compareNumber(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
