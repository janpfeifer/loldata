package data_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/janpfeifer/loldata/data"
)

func createSampleDataset() *data.Dataset {
	ds := data.NewDataset()

	sSolo := ds.GetOrCreateSummoner("solo_player", "SoloPlayer")
	sSolo.SummonerLevel = 42
	sSolo.Crawled = true

	m1 := &data.MatchV5{
		Metadata: data.MetadataDto{
			MatchID: "MATCH_001",
		},
		Info: data.InfoDto{
			GameStartTimestamp: time.Date(2025, 2, 1, 12, 0, 0, 0, time.UTC).UnixMilli(),
			GameDuration:       1900,
			Participants: []*data.ParticipantDto{
				{
					PUUID:         "p1",
					SummonerName:  "PlayerOne",
					ParticipantID: 1,
					TeamID:        100,
					Kills:         5,
					Deaths:        2,
					Assists:       8,
					Position:      data.PositionMid,
				},
				{
					PUUID:         "p2",
					SummonerName:  "PlayerTwo",
					ParticipantID: 2,
					TeamID:        200,
					Kills:         2,
					Deaths:        6,
					Assists:       1,
					Position:      data.PositionTop,
				},
			},
			Teams: []*data.TeamDto{
				{TeamID: 100, Win: true},
				{TeamID: 200, Win: false},
			},
		},
	}

	m2 := &data.MatchV5{
		Metadata: data.MetadataDto{
			MatchID: "MATCH_002",
		},
		Info: data.InfoDto{
			GameStartTimestamp: time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC).UnixMilli(),
			GameDuration:       2100,
			Participants: []*data.ParticipantDto{
				{
					PUUID:         "p1",
					SummonerName:  "PlayerOne",
					ParticipantID: 1,
					TeamID:        100,
					Kills:         10,
					Deaths:        1,
					Assists:       4,
					Position:      data.PositionMid,
				},
			},
			Teams: []*data.TeamDto{
				{TeamID: 100, Win: true},
			},
		},
	}

	ds.AddMatch(m1)
	ds.AddMatch(m2)
	ds.GetSummoner("p1").Crawled = true
	return ds
}

func verifyLoadedDataset(t *testing.T, loaded *data.Dataset) {
	t.Helper()

	if len(loaded.Matches) != 2 {
		t.Fatalf("expected 2 matches in loaded dataset, got %d", len(loaded.Matches))
	}
	if len(loaded.Summoners) != 3 { // p1, p2, solo_player
		t.Fatalf("expected 3 summoners in loaded dataset, got %d", len(loaded.Summoners))
	}

	p1 := loaded.GetSummoner("p1")
	if p1 == nil {
		t.Fatalf("GetSummoner('p1') returned nil")
	}
	if p1.Name != "PlayerOne" {
		t.Errorf("expected summoner name 'PlayerOne', got %q", p1.Name)
	}
	if len(p1.Matches) != 2 {
		t.Fatalf("expected 2 matches for p1, got %d", len(p1.Matches))
	}

	// Chronological order: MATCH_002 is earlier than MATCH_001
	if p1.Matches[0].Metadata.MatchID != "MATCH_002" {
		t.Errorf("expected first match to be MATCH_002, got %s", p1.Matches[0].Metadata.MatchID)
	}
	if p1.Matches[1].Metadata.MatchID != "MATCH_001" {
		t.Errorf("expected second match to be MATCH_001, got %s", p1.Matches[1].Metadata.MatchID)
	}

	// Verify participant bidirectional pointer
	match1 := loaded.GetMatch("MATCH_001")
	if match1 == nil {
		t.Fatalf("GetMatch('MATCH_001') returned nil")
	}
	part1 := match1.GetParticipantByPUUID("p1")
	if part1 == nil {
		t.Fatalf("participant p1 not found in match1")
	}
	if part1.Summoner != p1 {
		t.Errorf("expected participant.Summoner to match p1 pointer")
	}
	if part1.Position != data.PositionMid {
		t.Errorf("expected participant position PositionMid, got %v", part1.Position)
	}

	// Verify standalone summoner
	solo := loaded.GetSummoner("solo_player")
	if solo == nil {
		t.Fatalf("standalone summoner 'solo_player' not found")
	}
	if solo.SummonerLevel != 42 {
		t.Errorf("expected summonerLevel 42, got %d", solo.SummonerLevel)
	}
	if len(solo.Matches) != 0 {
		t.Errorf("expected 0 matches for solo_player, got %d", len(solo.Matches))
	}
	if !solo.Crawled {
		t.Errorf("expected solo_player.Crawled to be true")
	}

	// Verify Crawled status on loaded summoners
	if !p1.Crawled {
		t.Errorf("expected p1.Crawled to be true")
	}
	p2 := loaded.GetSummoner("p2")
	if p2 == nil {
		t.Fatalf("GetSummoner('p2') returned nil")
	}
	if p2.Crawled {
		t.Errorf("expected p2.Crawled to be false")
	}
}

func TestDatasetWriteAndReadJSON(t *testing.T) {
	ds := createSampleDataset()
	var buf bytes.Buffer

	if err := ds.WriteJSON(&buf); err != nil {
		t.Fatalf("WriteJSON failed: %v", err)
	}

	loaded := data.NewDataset()
	if err := loaded.ReadJSON(&buf); err != nil {
		t.Fatalf("ReadJSON failed: %v", err)
	}

	verifyLoadedDataset(t, loaded)
}

func TestDatasetWriteAndReadGob(t *testing.T) {
	ds := createSampleDataset()
	var buf bytes.Buffer

	if err := ds.WriteGob(&buf); err != nil {
		t.Fatalf("WriteGob failed: %v", err)
	}

	loaded := data.NewDataset()
	if err := loaded.ReadGob(&buf); err != nil {
		t.Fatalf("ReadGob failed: %v", err)
	}

	verifyLoadedDataset(t, loaded)
}

func TestDatasetSaveAndLoadJSONGz(t *testing.T) {
	tmpDir := t.TempDir()
	jsonGzPath := filepath.Join(tmpDir, "dataset.json.gz")

	ds := createSampleDataset()
	if err := ds.SaveToJSON(jsonGzPath); err != nil {
		t.Fatalf("SaveToJSON (.json.gz) failed: %v", err)
	}

	stat, err := os.Stat(jsonGzPath)
	if err != nil {
		t.Fatalf("failed to stat .json.gz file: %v", err)
	}
	if stat.Size() == 0 {
		t.Fatalf(".json.gz file is empty")
	}

	loaded := data.NewDataset()
	if err := loaded.LoadFromJSON(jsonGzPath); err != nil {
		t.Fatalf("LoadFromJSON (.json.gz) failed: %v", err)
	}

	verifyLoadedDataset(t, loaded)
}

func TestDatasetSaveAndLoadGob(t *testing.T) {
	tmpDir := t.TempDir()
	gobPath := filepath.Join(tmpDir, "dataset.gob")

	ds := createSampleDataset()
	if err := ds.SaveToGob(gobPath); err != nil {
		t.Fatalf("SaveToGob failed: %v", err)
	}

	stat, err := os.Stat(gobPath)
	if err != nil {
		t.Fatalf("failed to stat .gob file: %v", err)
	}
	if stat.Size() == 0 {
		t.Fatalf(".gob file is empty")
	}

	loaded := data.NewDataset()
	if err := loaded.LoadFromGob(gobPath); err != nil {
		t.Fatalf("LoadFromGob failed: %v", err)
	}

	verifyLoadedDataset(t, loaded)
}

func TestDatasetSaveAndLoadGobGz(t *testing.T) {
	tmpDir := t.TempDir()
	gobGzPath := filepath.Join(tmpDir, "dataset.gob.gz")

	ds := createSampleDataset()
	if err := ds.SaveToGob(gobGzPath); err != nil {
		t.Fatalf("SaveToGob (.gob.gz) failed: %v", err)
	}

	stat, err := os.Stat(gobGzPath)
	if err != nil {
		t.Fatalf("failed to stat .gob.gz file: %v", err)
	}
	if stat.Size() == 0 {
		t.Fatalf(".gob.gz file is empty")
	}

	loaded := data.NewDataset()
	if err := loaded.LoadFromGob(gobGzPath); err != nil {
		t.Fatalf("LoadFromGob (.gob.gz) failed: %v", err)
	}

	verifyLoadedDataset(t, loaded)
}
