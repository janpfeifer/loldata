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

func TestDatasetStoreCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	datasetPath := filepath.Join(tmpDir, "matches.json")
	lockPath := datasetPath + ".lock"

	store := NewDatasetStore(datasetPath, 1)
	ds := data.NewDataset()

	if err := store.Save(ds); err != nil {
		t.Fatalf("save failed: %v", err)
	}

	// Lock file should exist after Save
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Fatalf("expected lock file %s to exist after save", lockPath)
	}

	// Cleanup should remove the lock file
	store.Cleanup()

	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Errorf("expected lock file %s to be removed after Cleanup", lockPath)
	}

	// Calling Cleanup again should be a safe no-op
	store.Cleanup()
}

func TestDatasetStoreGobGz(t *testing.T) {
	tmpDir := t.TempDir()
	datasetPath := filepath.Join(tmpDir, "matches.gob.gz")

	store := NewDatasetStore(datasetPath, 2)
	ds := data.NewDataset()
	ds.GetOrCreateSummoner("faker_puuid", "Faker")

	m := &data.MatchV5{
		Metadata: data.MetadataDto{MatchID: "MATCH_GOB_1"},
		Info: data.InfoDto{
			GameDuration: 1800,
			Participants: []*data.ParticipantDto{
				{PUUID: "faker_puuid", SummonerName: "Faker"},
			},
		},
	}
	ds.AddMatch(m)

	if err := store.Save(ds); err != nil {
		t.Fatalf("store.Save(.gob.gz) failed: %v", err)
	}

	// Verify the file was written and starts with gzip magic bytes (0x1f, 0x8b)
	content, err := os.ReadFile(datasetPath)
	if err != nil {
		t.Fatalf("failed to read saved .gob.gz file: %v", err)
	}
	if len(content) < 2 || content[0] != 0x1f || content[1] != 0x8b {
		t.Fatalf("expected gzip magic bytes 0x1f 0x8b, got %x %x", content[0], content[1])
	}

	// Verify loading
	loadedDS := data.NewDataset()
	if err := store.Load(loadedDS); err != nil {
		t.Fatalf("store.Load(.gob.gz) failed: %v", err)
	}
	if len(loadedDS.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(loadedDS.Matches))
	}
	if loadedDS.GetSummoner("faker_puuid") == nil {
		t.Fatalf("summoner faker_puuid not found in loaded dataset")
	}
}


