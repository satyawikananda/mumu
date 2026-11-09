package storage

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestProviderID(t *testing.T) {
	explicit, err := ProviderID("tvg-1", "https://stream.example/live.m3u8")
	if err != nil {
		t.Fatalf("ProviderID: %v", err)
	}
	if explicit != "tvg-1" {
		t.Fatalf("explicit ID %q, want tvg-1", explicit)
	}

	first, err := ProviderID("", "https://stream.example/live.m3u8")
	if err != nil {
		t.Fatalf("ProviderID: %v", err)
	}
	second, err := ProviderID("", "https://stream.example/live.m3u8#fragment")
	if err != nil {
		t.Fatalf("ProviderID: %v", err)
	}
	if first != second {
		t.Fatalf("fragment changed provider ID: %q vs %q", first, second)
	}

	other, err := ProviderID("", "https://stream.example/other.m3u8")
	if err != nil {
		t.Fatalf("ProviderID: %v", err)
	}
	if other == first {
		t.Fatal("different URLs produced the same provider ID")
	}
}

func TestProviderIDRejectsCredentials(t *testing.T) {
	_, err := ProviderID("", "https://user:pass@stream.example/live.m3u8")
	if !errors.Is(err, ErrCredentialsUnsupported) {
		t.Fatalf("error %v, want ErrCredentialsUnsupported", err)
	}
}

func TestNewSourceLocalPath(t *testing.T) {
	first, err := NewSource(SourceLocalPlaylist, "list.m3u", "Local")
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	absolute, err := filepath.Abs("list.m3u")
	if err != nil {
		t.Fatalf("Abs: %v", err)
	}
	if first.Location != absolute {
		t.Fatalf("location %q, want %q", first.Location, absolute)
	}

	second, err := NewSource(SourceLocalPlaylist, "./list.m3u", "Local")
	if err != nil {
		t.Fatalf("NewSource: %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("local path ID changed: %s vs %s", first.ID, second.ID)
	}
}

func TestNewSourceRejectsInvalidKind(t *testing.T) {
	_, err := NewSource(SourceKind("unknown"), "https://example.com/list.m3u", "")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v, want ErrInvalid", err)
	}
}

func TestNewSourceRejectsUnsupportedScheme(t *testing.T) {
	_, err := NewSource(SourceCustomStream, "rtmp://example.com/live", "")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v, want ErrInvalid", err)
	}
}
