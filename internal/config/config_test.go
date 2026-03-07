package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"amazon-tracker/internal/config"
)

func tempItemsPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "items.json")
}

func TestLoadItems_NonexistentFile(t *testing.T) {
	store, err := config.LoadItems(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(store.All()) != 0 {
		t.Fatalf("expected 0 items, got %d", len(store.All()))
	}
}

func TestLoadItems_EmptyFile(t *testing.T) {
	path := tempItemsPath(t)
	os.WriteFile(path, []byte(""), 0644)

	store, err := config.LoadItems(path)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
	if len(store.All()) != 0 {
		t.Fatalf("expected 0 items, got %d", len(store.All()))
	}
}

func TestAddItem(t *testing.T) {
	path := tempItemsPath(t)
	store, _ := config.LoadItems(path)

	item, err := store.Add("https://www.amazon.it/dp/TEST1", 100.00)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if item.ID != 1 {
		t.Errorf("expected ID=1, got %d", item.ID)
	}
	if !item.Enabled {
		t.Error("expected new item to be enabled")
	}

	// Second item should get ID 2
	item2, err := store.Add("https://www.amazon.it/dp/TEST2", 200.00)
	if err != nil {
		t.Fatalf("Add failed: %v", err)
	}
	if item2.ID != 2 {
		t.Errorf("expected ID=2, got %d", item2.ID)
	}

	if len(store.All()) != 2 {
		t.Errorf("expected 2 items, got %d", len(store.All()))
	}

	// Verify file was written
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read items file: %v", err)
	}
	if len(data) == 0 {
		t.Error("items file should not be empty")
	}
}

func TestRemoveItem(t *testing.T) {
	path := tempItemsPath(t)
	store, _ := config.LoadItems(path)

	store.Add("https://www.amazon.it/dp/A", 100.00)
	store.Add("https://www.amazon.it/dp/B", 200.00)

	if err := store.Remove(1); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	items := store.All()
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].ID != 2 {
		t.Errorf("expected remaining item ID=2, got %d", items[0].ID)
	}

	// Removing non-existent should error
	if err := store.Remove(99); err == nil {
		t.Error("expected error removing non-existent item")
	}
}

func TestUpdateItem(t *testing.T) {
	path := tempItemsPath(t)
	store, _ := config.LoadItems(path)

	store.Add("https://www.amazon.it/dp/OLD", 100.00)

	// Update URL only (price=0 means no change)
	if err := store.Update(1, "https://www.amazon.it/dp/NEW", 0); err != nil {
		t.Fatalf("Update URL failed: %v", err)
	}
	item := store.Get(1)
	if item.URL != "https://www.amazon.it/dp/NEW" {
		t.Errorf("expected updated URL, got %s", item.URL)
	}
	if item.TargetPrice != 100.00 {
		t.Errorf("expected price unchanged at 100, got %.2f", item.TargetPrice)
	}

	// Update price only (url="" means no change)
	if err := store.Update(1, "", 250.00); err != nil {
		t.Fatalf("Update price failed: %v", err)
	}
	item = store.Get(1)
	if item.TargetPrice != 250.00 {
		t.Errorf("expected price=250, got %.2f", item.TargetPrice)
	}
}

func TestEnableDisable(t *testing.T) {
	path := tempItemsPath(t)
	store, _ := config.LoadItems(path)

	store.Add("https://www.amazon.it/dp/X", 100.00)
	store.Add("https://www.amazon.it/dp/Y", 200.00)

	// Disable item 1
	if err := store.SetEnabled(1, false); err != nil {
		t.Fatalf("SetEnabled failed: %v", err)
	}

	enabled := store.EnabledItems()
	if len(enabled) != 1 {
		t.Fatalf("expected 1 enabled item, got %d", len(enabled))
	}
	if enabled[0].ID != 2 {
		t.Errorf("expected enabled item ID=2, got %d", enabled[0].ID)
	}
}

func TestIDPersistence(t *testing.T) {
	path := tempItemsPath(t)
	store, _ := config.LoadItems(path)

	store.Add("https://www.amazon.it/dp/A", 100.00) // ID=1
	store.Add("https://www.amazon.it/dp/B", 200.00) // ID=2
	store.Add("https://www.amazon.it/dp/C", 300.00) // ID=3

	// Remove ID=2
	store.Remove(2)

	// Add another — should get ID=4, not 3
	item, _ := store.Add("https://www.amazon.it/dp/D", 400.00)
	if item.ID != 4 {
		t.Errorf("expected ID=4 after gap, got %d", item.ID)
	}
}

func TestLoadItems_Roundtrip(t *testing.T) {
	path := tempItemsPath(t)
	store, _ := config.LoadItems(path)

	store.Add("https://www.amazon.it/dp/RT1", 111.11)
	store.Add("https://www.amazon.it/dp/RT2", 222.22)
	store.SetEnabled(1, false)

	// Reload from disk
	store2, err := config.LoadItems(path)
	if err != nil {
		t.Fatalf("reload failed: %v", err)
	}

	items := store2.All()
	if len(items) != 2 {
		t.Fatalf("expected 2 items after reload, got %d", len(items))
	}
	if items[0].Enabled {
		t.Error("item 1 should be disabled after reload")
	}
	if !items[1].Enabled {
		t.Error("item 2 should be enabled after reload")
	}

	// Next ID should be 3
	item, _ := store2.Add("https://www.amazon.it/dp/RT3", 333.33)
	if item.ID != 3 {
		t.Errorf("expected next ID=3 after reload, got %d", item.ID)
	}
}

func TestItemLabel(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://www.amazon.it/dp/B0FHL3385S", "B0FHL3385S"},
		{"https://www.amazon.it/gp/product/B0FHL3385S", "B0FHL3385S"},
		{"https://www.amazon.it/dp/B0FHL3385S?ref=something", "B0FHL3385S"},
		{"https://short.url", "https://short.url"},
	}

	for _, tt := range tests {
		path := tempItemsPath(t)
		store, _ := config.LoadItems(path)
		item, _ := store.Add(tt.url, 100)
		got := item.Label()
		if got != tt.want {
			t.Errorf("Label(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}
