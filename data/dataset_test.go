package data_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/janpfeifer/loldata/data"
)

func TestEnums(t *testing.T) {
	// Side tests
	if data.SideBlue.TeamID() != 100 {
		t.Errorf("expected SideBlue.TeamID() to be 100, got %d", data.SideBlue.TeamID())
	}
	if data.SideRed.TeamID() != 200 {
		t.Errorf("expected SideRed.TeamID() to be 200, got %d", data.SideRed.TeamID())
	}
	if data.SideFromTeamID(100) != data.SideBlue {
		t.Errorf("expected SideFromTeamID(100) to be SideBlue, got %v", data.SideFromTeamID(100))
	}
	if data.ParseSide("Blue") != data.SideBlue {
		t.Errorf("expected ParseSide('Blue') to be SideBlue, got %v", data.ParseSide("Blue"))
	}
	if data.SideBlue.String() != "Blue" {
		t.Errorf("expected SideBlue.String() to be 'Blue', got %q", data.SideBlue.String())
	}

	// Position tests
	if data.ParsePosition("top") != data.PositionTop {
		t.Errorf("expected PositionTop, got %v", data.ParsePosition("top"))
	}
	if data.ParsePosition("jng") != data.PositionJungle {
		t.Errorf("expected PositionJungle, got %v", data.ParsePosition("jng"))
	}
	if data.ParsePosition("team") != data.PositionTeam {
		t.Errorf("expected PositionTeam, got %v", data.ParsePosition("team"))
	}

	// DataCompleteness tests
	if data.ParseDataCompleteness("complete") != data.DataCompletenessComplete {
		t.Errorf("expected DataCompletenessComplete, got %v", data.ParseDataCompleteness("complete"))
	}
}

func TestDatasetBasic(t *testing.T) {
	ds := data.NewDataset()

	// Create match 1
	m1 := &data.MatchV5{
		Metadata: data.MetadataDto{
			MatchID: "MATCH_001",
		},
		Info: data.InfoDto{
			GameStartTimestamp: time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC).UnixMilli(),
			GameDuration:       1800,
			Participants: []*data.ParticipantDto{
				{
					PUUID:         "player_1",
					SummonerName:  "Faker",
					ParticipantID: 1,
					TeamID:        100,
				},
				{
					PUUID:         "player_2",
					SummonerName:  "Chovy",
					ParticipantID: 2,
					TeamID:        200,
				},
			},
		},
	}

	// Create match 2 (earlier time)
	m2 := &data.MatchV5{
		Metadata: data.MetadataDto{
			MatchID: "MATCH_002",
		},
		Info: data.InfoDto{
			GameStartTimestamp: time.Date(2025, 1, 10, 10, 0, 0, 0, time.UTC).UnixMilli(),
			GameDuration:       2000,
			Participants: []*data.ParticipantDto{
				{
					PUUID:         "player_1",
					SummonerName:  "Faker",
					ParticipantID: 1,
					TeamID:        100,
				},
			},
		},
	}

	ds.AddMatch(m1)
	ds.AddMatch(m2)

	if len(ds.Matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(ds.Matches))
	}
	if len(ds.Summoners) != 2 {
		t.Fatalf("expected 2 summoners, got %d", len(ds.Summoners))
	}

	faker := ds.GetSummoner("player_1")
	if faker == nil {
		t.Fatalf("summoner player_1 not found")
	}
	if faker.Name != "Faker" {
		t.Errorf("expected name Faker, got %s", faker.Name)
	}
	if len(faker.Matches) != 2 {
		t.Fatalf("expected Faker to have 2 matches, got %d", len(faker.Matches))
	}

	// Ensure chronological order in summoner matches (m2 before m1)
	if faker.Matches[0].Metadata.MatchID != "MATCH_002" {
		t.Errorf("expected first match to be MATCH_002, got %s", faker.Matches[0].Metadata.MatchID)
	}
	if faker.Matches[1].Metadata.MatchID != "MATCH_001" {
		t.Errorf("expected second match to be MATCH_001, got %s", faker.Matches[1].Metadata.MatchID)
	}

	// Check JSON serialization
	b, err := json.Marshal(m1)
	if err != nil {
		t.Fatalf("failed to marshal MatchV5: %v", err)
	}
	var unmarshaled data.MatchV5
	if err := json.Unmarshal(b, &unmarshaled); err != nil {
		t.Fatalf("failed to unmarshal MatchV5: %v", err)
	}
	if unmarshaled.Metadata.MatchID != "MATCH_001" {
		t.Errorf("expected MatchID MATCH_001, got %s", unmarshaled.Metadata.MatchID)
	}
}

func TestLoadOraclesElixirSample(t *testing.T) {
	// Path to sample file
	samplePath := filepath.Join(os.Getenv("HOME"), "work/lol/2025_LoL_esports_match_data_from_OraclesElixir.csv")
	if _, err := os.Stat(samplePath); os.IsNotExist(err) {
		t.Skipf("sample file %s does not exist, skipping full dataset test", samplePath)
	}

	ds := data.NewDataset()
	err := ds.LoadOraclesElixir(samplePath)
	if err != nil {
		t.Fatalf("LoadOraclesElixir failed: %v", err)
	}

	t.Logf("Loaded %d matches and %d summoners", len(ds.Matches), len(ds.Summoners))

	if len(ds.Matches) != 10041 {
		t.Errorf("expected 10041 matches, got %d", len(ds.Matches))
	}
	// 2636 players have an explicit playerid + 186 players have empty playerid and fallback to playername = 2822 summoners
	if len(ds.Summoners) != 2822 {
		t.Errorf("expected 2822 summoners, got %d", len(ds.Summoners))
	}

	// Verify sample match details
	m := ds.GetMatch("LOLTMNT03_179647")
	if m == nil {
		t.Fatalf("match LOLTMNT03_179647 not found")
	}
	if len(m.Info.Participants) != 10 {
		t.Errorf("expected 10 participants, got %d", len(m.Info.Participants))
	}
	if len(m.Info.Teams) != 2 {
		t.Errorf("expected 2 teams, got %d", len(m.Info.Teams))
	}

	blueTeam := m.GetTeamBySide(data.SideBlue)
	if blueTeam == nil {
		t.Fatalf("blue team not found")
	}
	if blueTeam.Win {
		t.Errorf("expected blue team to lose, got win")
	}
	if blueTeam.Esports.TeamName != "IziDream" {
		t.Errorf("expected blue team name 'IziDream', got %q", blueTeam.Esports.TeamName)
	}

	redTeam := m.GetTeamBySide(data.SideRed)
	if redTeam == nil {
		t.Fatalf("red team not found")
	}
	if !redTeam.Win {
		t.Errorf("expected red team to win, got loss")
	}
	if redTeam.Esports.TeamName != "Team Valiant" {
		t.Errorf("expected red team name 'Team Valiant', got %q", redTeam.Esports.TeamName)
	}

	// Verify summoner chronological ordering
	for _, s := range ds.Summoners {
		for i := 1; i < len(s.Matches); i++ {
			tPrev := s.Matches[i-1].Time()
			tCurr := s.Matches[i].Time()
			if tPrev.After(tCurr) {
				t.Errorf("summoner %s (%s) matches not in chronological order: match %d time %v after match %d time %v",
					s.PUUID, s.Name, i-1, tPrev, i, tCurr)
				break
			}
		}
	}
}

func TestDatasetSaveAndLoadJSON(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "dataset.json")

	ds := data.NewDataset()

	// Register a standalone summoner (without match yet)
	sStandalone := ds.GetOrCreateSummoner("solo_player", "SoloPlayer")
	sStandalone.SummonerLevel = 42

	// Create matches
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
				},
				{
					PUUID:         "p2",
					SummonerName:  "PlayerTwo",
					ParticipantID: 2,
					TeamID:        200,
					Kills:         2,
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
				},
			},
			Teams: []*data.TeamDto{
				{TeamID: 100, Win: true},
			},
		},
	}

	ds.AddMatch(m1)
	ds.AddMatch(m2)

	// Save to JSON
	if err := ds.SaveToJSON(jsonPath); err != nil {
		t.Fatalf("SaveToJSON failed: %v", err)
	}

	// Verify file was written and is not empty
	stat, err := os.Stat(jsonPath)
	if err != nil {
		t.Fatalf("failed to stat written JSON file: %v", err)
	}
	if stat.Size() == 0 {
		t.Fatalf("saved JSON file is empty")
	}

	// Load into a new Dataset
	loaded := data.NewDataset()
	if err := loaded.LoadFromJSON(jsonPath); err != nil {
		t.Fatalf("LoadFromJSON failed: %v", err)
	}

	// Verify counts
	if len(loaded.Matches) != 2 {
		t.Fatalf("expected 2 matches in loaded dataset, got %d", len(loaded.Matches))
	}
	if len(loaded.Summoners) != 3 { // p1, p2, solo_player
		t.Fatalf("expected 3 summoners in loaded dataset, got %d", len(loaded.Summoners))
	}

	// Verify lookup maps
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
}

