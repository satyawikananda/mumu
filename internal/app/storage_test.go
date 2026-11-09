package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo"

	"mumu/internal/storage"
)

func TestOpenStoreCreatesDatabase(t *testing.T) {
	a := New()
	path := filepath.Join(t.TempDir(), "mumu.db")

	a.openStoreAt(path)

	store, err := a.Store()
	if err != nil || store == nil {
		t.Fatalf("Store = %v, %v, want open store", store, err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("Stat: %v", err)
	}

	a.closeStore()
	if _, err := a.Store(); !errors.Is(err, ErrStorageNotReady) {
		t.Fatalf("Store after close error %v, want ErrStorageNotReady", err)
	}
}

func TestOpenLocalStoreUsesUserDataPath(t *testing.T) {
	dir := t.TempDir()
	mygo.App.SetPath(mygo.PathUserData, dir)
	t.Cleanup(func() {
		mygo.App.SetPath(mygo.PathUserData, "")
	})

	a := New()
	a.openLocalStore()

	store, err := a.Store()
	if err != nil || store == nil {
		t.Fatalf("Store = %v, %v, want open store", store, err)
	}
	if _, err := os.Stat(filepath.Join(dir, storeFileName)); err != nil {
		t.Fatalf("Stat: %v", err)
	}
	a.closeStore()
}

func TestOpenStoreCorrupt(t *testing.T) {
	a := New()
	path := filepath.Join(t.TempDir(), "broken.db")
	if err := os.WriteFile(path, []byte("not sqlite"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	a.openStoreAt(path)

	store, err := a.Store()
	if store != nil {
		t.Fatal("Store returned a corrupt database")
	}
	if !errors.Is(err, storage.ErrCorrupt) {
		t.Fatalf("Store error %v, want ErrCorrupt", err)
	}
	if got := storeMessage(err); !strings.Contains(got, "damaged") {
		t.Fatalf("message %q, want damaged-library guidance", got)
	}
}

func TestStoreMessage(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{name: "corrupt", err: storage.ErrCorrupt, want: "damaged"},
		{name: "newer", err: storage.ErrSchemaNewer, want: "newer version"},
		{name: "unknown", err: storage.ErrUnknownSchema, want: "does not recognize"},
		{name: "permission", err: storage.ErrPermission, want: "permissions"},
		{name: "other", err: errors.New("boom"), want: "could not open"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := storeMessage(tc.err); !strings.Contains(got, tc.want) {
				t.Fatalf("message %q, want substring %q", got, tc.want)
			}
		})
	}
}

func TestStopClosesStore(t *testing.T) {
	a := New()
	a.openStoreAt(filepath.Join(t.TempDir(), "mumu.db"))

	if _, err := a.Store(); err != nil {
		t.Fatalf("Store: %v", err)
	}

	a.Stop()

	if _, err := a.Store(); !errors.Is(err, ErrStorageNotReady) {
		t.Fatalf("Store after Stop error %v, want ErrStorageNotReady", err)
	}
}

func TestStoreBeforeOpen(t *testing.T) {
	a := New()

	if _, err := a.Store(); !errors.Is(err, ErrStorageNotReady) {
		t.Fatalf("Store error %v, want ErrStorageNotReady", err)
	}
}
