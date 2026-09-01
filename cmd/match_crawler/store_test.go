package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/janpfeifer/loldata/data"
)

func TestDatasetStoreAtomicSaveAndBackups(t *testing.T) {
	tmpDir := t.TempDir()
	datasetPath := filepath.Join(tmpDir, "matches.json")

	store := NewDatasetStore(datasetPath, 3)

	ds := data.NewDataset()
	ds.GetOrCreateSummoner("puuid_1", "Player1")

	// Save 1: first creation, no backup created yet
	if err := store.Save(ds); err != nil {
		t.Fatalf("first save failed: %v", err)
	}

	if !store.Exists() {
		t.Fatalf("expected dataset file to exist after save")
	}

	// Verify ~ temp file is gone
	if _, err := os.Stat(datasetPath + "~"); !os.IsNotExist(err) {
		t.Errorf("temp file %s~ still exists", datasetPath)
	}

	// Save 2, 3, 4, 5 with small delays to create backups
	for i := 2; i <= 5; i++ {
		time.Sleep(10 * time.Millisecond)
		ds.GetOrCreateSummoner(strings.ReplaceAll("puuid_X", "X", string(rune('0'+i))), "Player")
		if err := store.Save(ds); err != nil {
			t.Fatalf("save %d failed: %v", i, err)
		}
	}

	// Count backup files
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}

	var backups []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "matches.json.backup-") {
			backups = append(backups, entry.Name())
		}
	}

	if len(backups) != 3 {
		t.Fatalf("expected exactly 3 backups, got %d: %v", len(backups), backups)
	}

	// Verify loading
	loadedDS := data.NewDataset()
	if err := store.Load(loadedDS); err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(loadedDS.Summoners) != 5 {
		t.Errorf("expected 5 summoners, got %d", len(loadedDS.Summoners))
	}
}
