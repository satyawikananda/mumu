package app

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestOpenCatalogWithoutStore(t *testing.T) {
	a := New()

	if _, err := a.Catalog(); !errors.Is(err, ErrCatalogNotReady) {
		t.Fatalf("Catalog error %v, want ErrCatalogNotReady", err)
	}
}

func TestOpenCatalogWithStore(t *testing.T) {
	a := New()
	a.openStoreAt(filepath.Join(t.TempDir(), "mumu.db"))
	t.Cleanup(a.closeStore)

	a.openCatalog()

	service, err := a.Catalog()
	if err != nil {
		t.Fatalf("Catalog: %v", err)
	}
	if service == nil {
		t.Fatal("Catalog returned a nil service")
	}
}
