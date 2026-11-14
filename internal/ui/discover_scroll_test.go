package ui

import (
	"fmt"
	"testing"

	"github.com/egoist/mygo/ui"
)

// The category rail and channel grid keep separate bounded scroll areas.
func TestDiscoverScrollableRegions(t *testing.T) {
	shell := NewShell()
	d := shell.Discover()
	categories := make([]DiscoverCategory, 0, 40)
	for i := range 40 {
		categories = append(categories, DiscoverCategory{Name: fmt.Sprintf("Category %d", i), Count: 1})
	}
	d.SetFacets(categories)
	d.SetState(DiscoverState{
		Kind:     DiscoverReady,
		Region:   "ID",
		Channels: discoverTestChannels(40),
		Total:    40,
	})

	tt := ui.NewTester(shell.View, 1280, 720)
	tt.Frame()

	if d.categoryScroll.MaxY <= 0 {
		t.Fatalf("category rail MaxY = %v, want a bounded scroll area", d.categoryScroll.MaxY)
	}
	if d.scroll.MaxY <= 0 {
		t.Fatalf("channel grid MaxY = %v, want a bounded scroll area", d.scroll.MaxY)
	}

	categoriesText, ok := tt.Find("CATEGORIES")
	if !ok {
		t.Fatalf("texts %q, want the category rail", tt.Texts())
	}
	tt.Scroll(categoriesText.X+40, categoriesText.Y+80, 0, 300)
	tt.Frame()
	if d.categoryScroll.Y <= 0 {
		t.Fatalf("category rail Y = %v after scrolling, want it moved", d.categoryScroll.Y)
	}

	first, ok := tt.Find("Channel 0")
	if !ok {
		t.Fatalf("texts %q, want the first channel card", tt.Texts())
	}
	tt.Scroll(first.X+40, first.Y+40, 0, 300)
	tt.Frame()
	if d.scroll.Y <= 0 {
		t.Fatalf("channel grid Y = %v after scrolling, want it moved", d.scroll.Y)
	}
}
