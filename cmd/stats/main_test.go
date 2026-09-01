package main

import (
	"math"
	"testing"
	"time"

	"github.com/janpfeifer/loldata/data"
)

func TestCalculateRank(t *testing.T) {
	tests := []struct {
		name       string
		vals       []float64
		target     float64
		wantRank   int
		wantTotal  int
		wantPctMin float64
		wantPctMax float64
	}{
		{
			name:       "empty",
			vals:       nil,
			target:     10.0,
			wantRank:   1,
			wantTotal:  1,
			wantPctMin: 100.0,
			wantPctMax: 100.0,
		},
		{
			name:       "single element",
			vals:       []float64{10.0},
			target:     10.0,
			wantRank:   1,
			wantTotal:  1,
			wantPctMin: 100.0,
			wantPctMax: 100.0,
		},
		{
			name:       "two elements - top",
			vals:       []float64{10.0, 20.0},
			target:     20.0,
			wantRank:   1,
			wantTotal:  2,
			wantPctMin: 100.0,
			wantPctMax: 100.0,
		},
		{
			name:       "two elements - bottom",
			vals:       []float64{10.0, 20.0},
			target:     10.0,
			wantRank:   2,
			wantTotal:  2,
			wantPctMin: 0.0,
			wantPctMax: 0.0,
		},
		{
			name:       "three elements - median",
			vals:       []float64{10.0, 20.0, 30.0},
			target:     20.0,
			wantRank:   2,
			wantTotal:  3,
			wantPctMin: 50.0,
			wantPctMax: 50.0,
		},
		{
			name:       "tied elements - all tied",
			vals:       []float64{10.0, 10.0, 10.0},
			target:     10.0,
			wantRank:   1,
			wantTotal:  3,
			wantPctMin: 50.0,
			wantPctMax: 50.0,
		},
		{
			name:       "five elements - tied in middle",
			vals:       []float64{10.0, 20.0, 20.0, 20.0, 30.0},
			target:     20.0,
			wantRank:   2,
			wantTotal:  5,
			wantPctMin: 50.0,
			wantPctMax: 50.0,
		},
		{
			name:       "100 elements - top",
			vals:       func() []float64 { v := make([]float64, 100); for i := range v { v[i] = float64(i + 1) }; return v }(),
			target:     100.0,
			wantRank:   1,
			wantTotal:  100,
			wantPctMin: 100.0,
			wantPctMax: 100.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := calculateRank(tc.vals, tc.target)
			if got.Rank != tc.wantRank {
				t.Errorf("Rank: got %d, want %d", got.Rank, tc.wantRank)
			}
			if got.TotalCount != tc.wantTotal {
				t.Errorf("TotalCount: got %d, want %d", got.TotalCount, tc.wantTotal)
			}
			if got.Percentile < tc.wantPctMin-1e-6 || got.Percentile > tc.wantPctMax+1e-6 {
				t.Errorf("Percentile: got %.2f, want between %.2f and %.2f", got.Percentile, tc.wantPctMin, tc.wantPctMax)
			}
		})
	}
}

func TestComputeCohortRanks(t *testing.T) {
	ds := data.NewDataset()

	s1 := ds.GetOrCreateSummoner("puuid_1", "Player1#EUW")
	s1.Crawled = true
	s2 := ds.GetOrCreateSummoner("puuid_2", "Player2#EUW")
	s2.Crawled = true
	s3 := ds.GetOrCreateSummoner("puuid_3", "Player3#EUW")
	s3.Crawled = true

	// Match 1: Player 1 and Player 2
	m1 := &data.MatchV5{
		Metadata: data.MetadataDto{MatchID: "M1"},
		Info: data.InfoDto{
			GameStartTimestamp: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC).UnixMilli(),
			GameDuration:       1800, // 30 min
			Teams: []*data.TeamDto{
				{TeamID: 100, Win: true},
				{TeamID: 200, Win: false},
			},
			Participants: []*data.ParticipantDto{
				{
					PUUID:                       "puuid_1",
					SummonerName:                "Player1#EUW",
					TeamID:                      100,
					Win:                         true,
					Kills:                       10,
					Deaths:                      2,
					Assists:                     5,
					TotalMinionsKilled:          200,
					GoldEarned:                  15000,
					TotalDamageDealtToChampions: 25000,
					VisionScore:                 30,
				},
				{
					PUUID:                       "puuid_2",
					SummonerName:                "Player2#EUW",
					TeamID:                      200,
					Win:                         false,
					Kills:                       2,
					Deaths:                      5,
					Assists:                     2,
					TotalMinionsKilled:          100,
					GoldEarned:                  8000,
					TotalDamageDealtToChampions: 10000,
					VisionScore:                 15,
				},
			},
		},
	}

	// Match 2: Player 1 only
	m2 := &data.MatchV5{
		Metadata: data.MetadataDto{MatchID: "M2"},
		Info: data.InfoDto{
			GameStartTimestamp: time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC).UnixMilli(),
			GameDuration:       1800, // 30 min
			Teams: []*data.TeamDto{
				{TeamID: 100, Win: true},
				{TeamID: 200, Win: false},
			},
			Participants: []*data.ParticipantDto{
				{
					PUUID:                       "puuid_1",
					SummonerName:                "Player1#EUW",
					TeamID:                      100,
					Win:                         true,
					Kills:                       8,
					Deaths:                      1,
					Assists:                     4,
					TotalMinionsKilled:          220,
					GoldEarned:                  16000,
					TotalDamageDealtToChampions: 28000,
					VisionScore:                 35,
				},
			},
		},
	}

	// Match 3: Player 3 only (1 game, middle stats)
	m3 := &data.MatchV5{
		Metadata: data.MetadataDto{MatchID: "M3"},
		Info: data.InfoDto{
			GameStartTimestamp: time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC).UnixMilli(),
			GameDuration:       1800, // 30 min
			Teams: []*data.TeamDto{
				{TeamID: 100, Win: false},
			},
			Participants: []*data.ParticipantDto{
				{
					PUUID:                       "puuid_3",
					SummonerName:                "Player3#EUW",
					TeamID:                      100,
					Win:                         false,
					Kills:                       5,
					Deaths:                      3,
					Assists:                     3,
					TotalMinionsKilled:          150,
					GoldEarned:                  11000,
					TotalDamageDealtToChampions: 18000,
					VisionScore:                 20,
				},
			},
		},
	}

	ds.AddMatch(m1)
	ds.AddMatch(m2)
	ds.AddMatch(m3)

	stats1 := s1.ComputeStats()
	ranks1 := computeCohortRanks(ds, s1, stats1)

	if ranks1 == nil {
		t.Fatalf("expected ranks1 not nil")
	}
	if ranks1.CohortSize != 3 {
		t.Fatalf("expected cohort size 3, got %d", ranks1.CohortSize)
	}

	// Player 1 played 2 matches, others played 1 match
	if ranks1.TotalMatches.Rank != 1 {
		t.Errorf("expected TotalMatches rank 1, got %d", ranks1.TotalMatches.Rank)
	}
	if math.Abs(ranks1.TotalMatches.Percentile-100.0) > 1e-6 {
		t.Errorf("expected TotalMatches percentile 100.0, got %.2f", ranks1.TotalMatches.Percentile)
	}

	// Player 1 has best KDA, CSPM, GPM, AvgCS
	if ranks1.KDARatio.Rank != 1 || ranks1.KDARatio.Percentile != 100.0 {
		t.Errorf("expected KDARatio rank 1 (100.0%%), got rank %d (%.2f%%)", ranks1.KDARatio.Rank, ranks1.KDARatio.Percentile)
	}
	if ranks1.AvgCS.Rank != 1 || ranks1.AvgCS.Percentile != 100.0 {
		t.Errorf("expected AvgCS rank 1 (100.0%%), got rank %d (%.2f%%)", ranks1.AvgCS.Rank, ranks1.AvgCS.Percentile)
	}
	if ranks1.AvgCSPM.Rank != 1 || ranks1.AvgCSPM.Percentile != 100.0 {
		t.Errorf("expected AvgCSPM rank 1 (100.0%%), got rank %d (%.2f%%)", ranks1.AvgCSPM.Rank, ranks1.AvgCSPM.Percentile)
	}
	if ranks1.AvgGPM.Rank != 1 || ranks1.AvgGPM.Percentile != 100.0 {
		t.Errorf("expected AvgGPM rank 1 (100.0%%), got rank %d (%.2f%%)", ranks1.AvgGPM.Rank, ranks1.AvgGPM.Percentile)
	}

	// Player 3 rankings (should be median / rank 2 on most stats)
	stats3 := s3.ComputeStats()
	ranks3 := computeCohortRanks(ds, s3, stats3)
	if ranks3.AvgCS.Rank != 2 || ranks3.AvgCS.Percentile != 50.0 {
		t.Errorf("expected Player 3 AvgCS rank 2 (50.0%%), got rank %d (%.2f%%)", ranks3.AvgCS.Rank, ranks3.AvgCS.Percentile)
	}
	if ranks3.TotalMatches.Rank != 2 || ranks3.TotalMatches.Percentile != 25.0 {
		t.Errorf("expected Player 3 TotalMatches rank 2 (25.0%% tied with Player 2), got rank %d (%.2f%%)", ranks3.TotalMatches.Rank, ranks3.TotalMatches.Percentile)
	}
}
