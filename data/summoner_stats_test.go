package data_test

import (
	"testing"
	"time"

	"github.com/janpfeifer/loldata/data"
)

func TestFindSummoner(t *testing.T) {
	ds := data.NewDataset()

	s1 := ds.GetOrCreateSummoner("puuid_111", "LuckyShott#1114")
	s2 := ds.GetOrCreateSummoner("puuid_222", "LuckyCat#EUW")
	s3 := ds.GetOrCreateSummoner("puuid_333", "Faker")

	// 1. Direct PUUID lookup
	s, cands := ds.FindSummoner("puuid_111")
	if s != s1 || cands != nil {
		t.Errorf("expected to find s1 by PUUID, got s=%v, cands=%v", s, cands)
	}

	// 2. Exact full name match (case insensitive)
	s, cands = ds.FindSummoner("luckyshott#1114")
	if s != s1 || cands != nil {
		t.Errorf("expected to find s1 by full name, got s=%v, cands=%v", s, cands)
	}

	// 3. Exact game name match (without tag)
	s, cands = ds.FindSummoner("LuckyShott")
	if s != s1 || cands != nil {
		t.Errorf("expected to find s1 by game name, got s=%v, cands=%v", s, cands)
	}

	// 4. Exact game name match for single name
	s, cands = ds.FindSummoner("faker")
	if s != s3 || cands != nil {
		t.Errorf("expected to find s3 Faker, got s=%v, cands=%v", s, cands)
	}

	// 5. Ambiguous prefix match
	s, cands = ds.FindSummoner("Lucky")
	if s != nil || len(cands) != 2 {
		t.Errorf("expected ambiguous match with 2 candidates, got s=%v, len(cands)=%d", s, len(cands))
	}

	// 6. Non-existent summoner
	s, cands = ds.FindSummoner("NonExistent")
	if s != nil || cands != nil {
		t.Errorf("expected nil for non-existent summoner, got s=%v, cands=%v", s, cands)
	}
}

func TestComputeSummonerStats(t *testing.T) {
	ds := data.NewDataset()

	// Create match 1: Blue side win, Yone Mid
	m1 := &data.MatchV5{
		Metadata: data.MetadataDto{
			MatchID: "MATCH_001",
		},
		Info: data.InfoDto{
			GameStartTimestamp: time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC).UnixMilli(),
			GameDuration:       1800, // 30 minutes
			QueueID:            420,  // Ranked Solo/Duo
			Teams: []*data.TeamDto{
				{TeamID: 100, Win: true, Objectives: data.ObjectivesDto{Champion: data.ObjectiveDto{Kills: 20}}},
				{TeamID: 200, Win: false, Objectives: data.ObjectivesDto{Champion: data.ObjectiveDto{Kills: 10}}},
			},
			Participants: []*data.ParticipantDto{
				{
					PUUID:                       "puuid_test",
					SummonerName:                "TestPlayer#EUW",
					TeamID:                      100,
					Position:                    data.PositionMid,
					ChampionName:                "Yone",
					ChampionID:                  777,
					Win:                         true,
					Kills:                       10,
					Deaths:                      2,
					Assists:                     5,
					DoubleKills:                 2,
					TotalMinionsKilled:          200,
					NeutralMinionsKilled:        10,
					GoldEarned:                  15000,
					TotalDamageDealtToChampions: 25000,
					TotalDamageTaken:            12000,
					DamageSelfMitigated:         8000,
					DamageDealtToTurrets:        3500,
					VisionScore:                 25,
					WardsPlaced:                 10,
					WardsKilled:                 3,
					DetectorWardsPlaced:         2,
					FirstBloodKill:              true,
				},
				{
					PUUID:        "puuid_teammate",
					SummonerName: "Teammate#EUW",
					TeamID:       100,
					Win:          true,
					Kills:        10,
				},
				{
					PUUID:        "puuid_enemy",
					SummonerName: "Enemy#EUW",
					TeamID:       200,
					Win:          false,
					Kills:        5,
				},
			},
		},
	}

	// Create match 2: Red side loss, Yasuo Mid
	m2 := &data.MatchV5{
		Metadata: data.MetadataDto{
			MatchID: "MATCH_002",
		},
		Info: data.InfoDto{
			GameStartTimestamp: time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC).UnixMilli(),
			GameDuration:       1200, // 20 minutes
			QueueID:            440,  // Ranked Flex
			Teams: []*data.TeamDto{
				{TeamID: 100, Win: true, Objectives: data.ObjectivesDto{Champion: data.ObjectiveDto{Kills: 15}}},
				{TeamID: 200, Win: false, Objectives: data.ObjectivesDto{Champion: data.ObjectiveDto{Kills: 5}}},
			},
			Participants: []*data.ParticipantDto{
				{
					PUUID:                       "puuid_test",
					SummonerName:                "TestPlayer#EUW",
					TeamID:                      200,
					Position:                    data.PositionMid,
					ChampionName:                "Yasuo",
					ChampionID:                  157,
					Win:                         false,
					Kills:                       2,
					Deaths:                      6,
					Assists:                     3,
					TotalMinionsKilled:          140,
					NeutralMinionsKilled:        0,
					GoldEarned:                  8000,
					TotalDamageDealtToChampions: 11000,
					TotalDamageTaken:            15000,
					DamageSelfMitigated:         6000,
					VisionScore:                 15,
					WardsPlaced:                 5,
					WardsKilled:                 1,
					DetectorWardsPlaced:         1,
				},
				{
					PUUID:        "puuid_teammate",
					SummonerName: "Teammate#EUW",
					TeamID:       200,
					Win:          false,
					Kills:        3,
				},
			},
		},
	}

	ds.AddMatch(m1)
	ds.AddMatch(m2)

	summoner := ds.GetSummoner("puuid_test")
	if summoner == nil {
		t.Fatalf("summoner puuid_test not found")
	}

	stats := summoner.ComputeStats()
	if stats == nil {
		t.Fatalf("expected stats not nil")
	}

	// Verify Overall Stats
	if stats.TotalMatches != 2 {
		t.Errorf("expected 2 total matches, got %d", stats.TotalMatches)
	}
	if stats.Wins != 1 || stats.Losses != 1 {
		t.Errorf("expected 1W - 1L, got %dW - %dL", stats.Wins, stats.Losses)
	}
	if stats.WinRate != 50.0 {
		t.Errorf("expected 50%% win rate, got %.1f%%", stats.WinRate)
	}
	if stats.BlueGames != 1 || stats.BlueWins != 1 || stats.BlueWinRate != 100.0 {
		t.Errorf("expected 1 Blue game with 100%% win rate, got %d games, %d wins, %.1f%%", stats.BlueGames, stats.BlueWins, stats.BlueWinRate)
	}
	if stats.RedGames != 1 || stats.RedWins != 0 || stats.RedWinRate != 0.0 {
		t.Errorf("expected 1 Red game with 0%% win rate, got %d games, %d wins, %.1f%%", stats.RedGames, stats.RedWins, stats.RedWinRate)
	}

	// Verify Combat Stats
	if stats.TotalKills != 12 || stats.TotalDeaths != 8 || stats.TotalAssists != 8 {
		t.Errorf("expected 12/8/8, got %d/%d/%d", stats.TotalKills, stats.TotalDeaths, stats.TotalAssists)
	}
	expectedKDA := float64(12+8) / 8.0 // 2.5
	if stats.KDARatio != expectedKDA {
		t.Errorf("expected KDA ratio %.2f, got %.2f", expectedKDA, stats.KDARatio)
	}
	if stats.DoubleKills != 2 {
		t.Errorf("expected 2 double kills, got %d", stats.DoubleKills)
	}
	if stats.FirstBloodKills != 1 {
		t.Errorf("expected 1 first blood kill, got %d", stats.FirstBloodKills)
	}

	// Verify CS and DPM
	// Total CS: (200+10) + 140 = 350 across 50 minutes total duration
	expectedCSPM := 350.0 / 50.0 // 7.0
	if stats.AvgCSPM != expectedCSPM {
		t.Errorf("expected AvgCSPM %.1f, got %.1f", expectedCSPM, stats.AvgCSPM)
	}
	// Total Damage: 25000 + 11000 = 36000 across 50 mins = 720 DPM
	expectedDPM := 36000.0 / 50.0
	if stats.AvgDPM != expectedDPM {
		t.Errorf("expected AvgDPM %.1f, got %.1f", expectedDPM, stats.AvgDPM)
	}

	// Verify Champions Played Distribution
	if len(stats.Champions) != 2 {
		t.Fatalf("expected 2 champions, got %d", len(stats.Champions))
	}
	// Yone (1W) should come before Yasuo (0W) because of win rate sorting
	if stats.Champions[0].ChampionName != "Yone" || stats.Champions[0].WinRate != 100.0 {
		t.Errorf("expected Yone first with 100%% WR, got %s with %.1f%%", stats.Champions[0].ChampionName, stats.Champions[0].WinRate)
	}
	if stats.Champions[0].PercentOfTotal != 50.0 {
		t.Errorf("expected Yone to be 50%% of total games, got %.1f%%", stats.Champions[0].PercentOfTotal)
	}

	// Verify Position Stats
	if len(stats.Positions) != 1 {
		t.Fatalf("expected 1 position (Mid), got %d", len(stats.Positions))
	}
	if stats.Positions[0].Position != data.PositionMid || stats.Positions[0].Games != 2 {
		t.Errorf("expected Mid with 2 games, got %v (%s) with %d games", stats.Positions[0].Position, stats.Positions[0].PositionName, stats.Positions[0].Games)
	}

	// Verify Teammates Stats
	if len(stats.Teammates) != 1 {
		t.Fatalf("expected 1 teammate, got %d", len(stats.Teammates))
	}
	if stats.Teammates[0].Name != "Teammate#EUW" || stats.Teammates[0].Games != 2 {
		t.Errorf("expected Teammate#EUW with 2 games, got %s with %d games", stats.Teammates[0].Name, stats.Teammates[0].Games)
	}
	if stats.Teammates[0].PercentOfTotal != 100.0 {
		t.Errorf("expected Teammate#EUW to be 100%% of games, got %.1f%%", stats.Teammates[0].PercentOfTotal)
	}

	// Verify Queues Stats
	if len(stats.Queues) != 2 {
		t.Fatalf("expected 2 queues, got %d", len(stats.Queues))
	}
}
