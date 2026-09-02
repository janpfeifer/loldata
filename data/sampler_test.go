package data_test

import (
	"testing"
	"time"

	"github.com/gomlx/compute/dtypes"
	"github.com/gomlx/gomlx/core/tensors"
	"github.com/gomlx/gomlx/ml/train"
	"github.com/janpfeifer/loldata/data"
)

// createSyntheticDataset creates a dataset spanning `days` days with `matchesPerDay` matches per day.
func createSyntheticDataset(days int, matchesPerDay int) *data.Dataset {
	ds := data.NewDataset()
	baseTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	// Create 10 dummy summoners
	for i := 1; i <= 10; i++ {
		ds.GetOrCreateSummoner(fmtSummonerPUUID(i), fmtSummonerName(i))
	}

	matchCount := 0
	for d := 0; d < days; d++ {
		for m := 0; m < matchesPerDay; m++ {
			matchCount++
			mTime := baseTime.Add(time.Duration(d)*24*time.Hour + time.Duration(m)*time.Hour)
			matchID := fmtMatchID(matchCount)

			participants := make([]*data.ParticipantDto, 10)
			for p := 0; p < 10; p++ {
				pos := data.PositionTop
				switch p % 5 {
				case 0:
					pos = data.PositionTop
				case 1:
					pos = data.PositionJungle
				case 2:
					pos = data.PositionMid
				case 3:
					pos = data.PositionBot
				case 4:
					pos = data.PositionSupport
				}

				teamID := 100
				if p >= 5 {
					teamID = 200
				}

				participants[p] = &data.ParticipantDto{
					PUUID:                      fmtSummonerPUUID(p + 1),
					SummonerName:               fmtSummonerName(p + 1),
					ParticipantID:              p + 1,
					TeamID:                     teamID,
					ChampionID:                 100 + p,
					Position:                   pos,
					Kills:                      p + (m % 3),
					Deaths:                     (p + 1) % 4,
					Assists:                    p * 2,
					GoldEarned:                 10000 + p*500,
					TotalDamageDealtToChampions: 15000 + p*1000,
					VisionScore:                20 + p*2,
					Win:                        teamID == 100, // Blue wins
				}
			}

			match := &data.MatchV5{
				Metadata: data.MetadataDto{
					MatchID: matchID,
				},
				Info: data.InfoDto{
					GameStartTimestamp: mTime.UnixMilli(),
					GameDuration:       1800,
					QueueID:            420,
					MapID:              11,
					GameVersion:        "15.1.1",
					Participants:       participants,
					Teams: []*data.TeamDto{
						{TeamID: 100, Win: true},
						{TeamID: 200, Win: false},
					},
				},
			}

			ds.AddMatch(match)
		}
	}

	return ds
}

func fmtSummonerPUUID(i int) string {
	return "puuid_player_" + string(rune('0'+i))
}

func fmtSummonerName(i int) string {
	return "Player_" + string(rune('0'+i))
}

func fmtMatchID(i int) string {
	return "MATCH_" + string(rune('0'+(i/100))) + string(rune('0'+((i/10)%10))) + string(rune('0'+(i%10)))
}

func TestSampler_ChronologicalOneEpoch(t *testing.T) {
	ds := createSyntheticDataset(7, 10) // 7 days, 10 matches/day = 70 matches

	opts := data.SamplerOptions{
		Name:           "val-sampler",
		BatchSize:      16,
		RandomSampling: false,
	}

	var sampler train.Dataset = ds.Sampler(opts)
	if sampler.Name() != "val-sampler" {
		t.Errorf("expected name 'val-sampler', got %q", sampler.Name())
	}

	totalBatches := 0
	totalSamples := 0

	for batch, err := range sampler.Iter() {
		if err != nil {
			t.Fatalf("unexpected error during Iter: %v", err)
		}
		if len(batch.Inputs) != 6 {
			t.Fatalf("expected 6 input tensors, got %d", len(batch.Inputs))
		}
		if len(batch.Labels) != 4 {
			t.Fatalf("expected 4 label tensors, got %d", len(batch.Labels))
		}

		batchSize := batch.Inputs[0].Shape().Dimensions[0]
		totalSamples += batchSize
		totalBatches++
	}

	if totalSamples != 70 {
		t.Errorf("expected 70 total samples across one epoch, got %d", totalSamples)
	}
	// 70 / 16 = 5 batches (16 + 16 + 16 + 16 + 6)
	if totalBatches != 5 {
		t.Errorf("expected 5 batches, got %d", totalBatches)
	}

	// Verify that Iter() can be called a second time (resettable)
	totalSamples2 := 0
	for batch, err := range sampler.Iter() {
		if err != nil {
			t.Fatalf("unexpected error during second Iter: %v", err)
		}
		totalSamples2 += batch.Inputs[0].Shape().Dimensions[0]
	}
	if totalSamples2 != 70 {
		t.Errorf("expected 70 total samples on second epoch, got %d", totalSamples2)
	}
}

func TestSampler_TimeLimitsTrainValTestSplit(t *testing.T) {
	ds := createSyntheticDataset(7, 10) // 7 days, 10 matches/day

	baseTime := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	day5End := baseTime.Add(5 * 24 * time.Hour)
	day6End := baseTime.Add(6 * 24 * time.Hour)
	day7End := baseTime.Add(7 * 24 * time.Hour)

	// Train: First 5 days (Days 0..4) -> 50 matches
	trainSampler := ds.Sampler(data.SamplerOptions{
		Name:           "train",
		StartTime:      baseTime,
		EndTime:        day5End,
		BatchSize:      10,
		RandomSampling: false,
	})
	trainCount := 0
	for batch, _ := range trainSampler.Iter() {
		trainCount += batch.Inputs[0].Shape().Dimensions[0]
	}
	if trainCount != 50 {
		t.Errorf("expected 50 train matches, got %d", trainCount)
	}

	// Val: Day 6 (Day 5 in 0-indexed) -> 10 matches
	valSampler := ds.Sampler(data.SamplerOptions{
		Name:           "val",
		StartTime:      day5End,
		EndTime:        day6End,
		BatchSize:      10,
		RandomSampling: false,
	})
	valCount := 0
	for batch, _ := range valSampler.Iter() {
		valCount += batch.Inputs[0].Shape().Dimensions[0]
	}
	if valCount != 10 {
		t.Errorf("expected 10 val matches, got %d", valCount)
	}

	// Test: Day 7 (Day 6 in 0-indexed) -> 10 matches
	testSampler := ds.Sampler(data.SamplerOptions{
		Name:           "test",
		StartTime:      day6End,
		EndTime:        day7End,
		BatchSize:      10,
		RandomSampling: false,
	})
	testCount := 0
	for batch, _ := range testSampler.Iter() {
		testCount += batch.Inputs[0].Shape().Dimensions[0]
	}
	if testCount != 10 {
		t.Errorf("expected 10 test matches, got %d", testCount)
	}
}

func TestSampler_RandomSamplingInfinite(t *testing.T) {
	ds := createSyntheticDataset(5, 10) // 50 matches

	opts := data.SamplerOptions{
		Name:           "train-random",
		BatchSize:      8,
		RandomSampling: true,
		TimeWeight:     1.0, // Uniform
		RandSeed:       42,
	}

	sampler := ds.Sampler(opts)

	batchesRead := 0
	maxBatches := 20
	for batch, err := range sampler.Iter() {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if batch.Inputs[0].Shape().Dimensions[0] != 8 {
			t.Errorf("expected batch size 8, got %d", batch.Inputs[0].Shape().Dimensions[0])
		}
		batchesRead++
		if batchesRead >= maxBatches {
			break // Break out of infinite loop
		}
	}

	if batchesRead != maxBatches {
		t.Errorf("expected to read %d batches, got %d", maxBatches, batchesRead)
	}
}

func TestSampler_RandomSamplingTimeWeight(t *testing.T) {
	ds := createSyntheticDataset(7, 20) // 140 matches
	allMatches := ds.FilterMatches(data.SamplerOptions{})
	t0 := allMatches[0].Time()
	tEnd := allMatches[len(allMatches)-1].Time()
	midTime := t0.Add(tEnd.Sub(t0) / 2)

	// Sample 1000 items with TimeWeight = 0.0 (heavy early bias)
	sEarly, err := ds.NewSampler(data.SamplerOptions{
		BatchSize:      1,
		RandomSampling: true,
		TimeWeight:     0.0,
		RandSeed:       12345,
	})
	if err != nil {
		t.Fatalf("failed to create early-biased sampler: %v", err)
	}

	earlyCountEarlyHalf := 0
	totalSamples := 1000
	for i := 0; i < totalSamples; i++ {
		m := sEarly.SampleMatch()
		if m.Time().Before(midTime) {
			earlyCountEarlyHalf++
		}
	}

	// Sample 1000 items with TimeWeight = 1.0 (uniform)
	sUniform, err := ds.NewSampler(data.SamplerOptions{
		BatchSize:      1,
		RandomSampling: true,
		TimeWeight:     1.0,
		RandSeed:       12345,
	})
	if err != nil {
		t.Fatalf("failed to create uniform sampler: %v", err)
	}

	uniformCountEarlyHalf := 0
	for i := 0; i < totalSamples; i++ {
		m := sUniform.SampleMatch()
		if m.Time().Before(midTime) {
			uniformCountEarlyHalf++
		}
	}

	t.Logf("TimeWeight=0.0 (Early biased): %d / %d in early half (%.1f%%)",
		earlyCountEarlyHalf, totalSamples, float64(earlyCountEarlyHalf)/float64(totalSamples)*100.0)
	t.Logf("TimeWeight=1.0 (Uniform):      %d / %d in early half (%.1f%%)",
		uniformCountEarlyHalf, totalSamples, float64(uniformCountEarlyHalf)/float64(totalSamples)*100.0)

	// TimeWeight = 0.0 should have significantly more than 50% in early half (e.g. > 70%)
	if earlyCountEarlyHalf <= 700 {
		t.Errorf("expected > 700 early-half samples with TimeWeight=0.0, got %d", earlyCountEarlyHalf)
	}

	// Uniform should be close to 50% (e.g. 400..600)
	if uniformCountEarlyHalf < 400 || uniformCountEarlyHalf > 600 {
		t.Errorf("expected ~500 uniform samples in early half, got %d", uniformCountEarlyHalf)
	}
}

func TestSampler_TensorShapesAndDtypes(t *testing.T) {
	ds := createSyntheticDataset(3, 5) // 15 matches
	ds.InitSummonerEmbeddings(32)

	// Write a test embedding at match 0, slot 0
	testEmb := make([]float32, 32)
	for k := range testEmb {
		testEmb[k] = float32(k + 1)
	}
	ds.SetSummonerEmbedding(0, 0, testEmb)

	sampler := ds.Sampler(data.SamplerOptions{
		BatchSize:      4,
		RandomSampling: false,
	})

	for batch, err := range sampler.Iter() {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(batch.Inputs) != 6 {
			t.Fatalf("expected 6 inputs, got %d", len(batch.Inputs))
		}
		if len(batch.Labels) != 4 {
			t.Fatalf("expected 4 labels, got %d", len(batch.Labels))
		}

		// Inputs[0]: ChampIDs [B, 2, 5] (Int32)
		if shape := batch.Inputs[0].Shape(); shape.DType != dtypes.Int32 || shape.Dimensions[0] != 4 || shape.Dimensions[1] != 2 || shape.Dimensions[2] != 5 {
			t.Errorf("Inputs[0] ChampIDs unexpected shape: %v", shape)
		}

		// Inputs[1]: SummonerIndices [B, 2, 5] (Int32)
		if shape := batch.Inputs[1].Shape(); shape.DType != dtypes.Int32 || shape.Dimensions[0] != 4 || shape.Dimensions[1] != 2 || shape.Dimensions[2] != 5 {
			t.Errorf("Inputs[1] SummonerIndices unexpected shape: %v", shape)
		}

		// Inputs[2]: Prior Summoner Embeddings [B, 10, 32] (Float32)
		if shape := batch.Inputs[2].Shape(); shape.DType != dtypes.Float32 || shape.Dimensions[0] != 4 || shape.Dimensions[1] != 10 || shape.Dimensions[2] != 32 {
			t.Errorf("Inputs[2] PriorEmbeddings unexpected shape: %v", shape)
		}

		// Inputs[3]: Previous Match Data [B, 10, 36] (Float32)
		if shape := batch.Inputs[3].Shape(); shape.DType != dtypes.Float32 || shape.Dimensions[0] != 4 || shape.Dimensions[1] != 10 || shape.Dimensions[2] != 36 {
			t.Errorf("Inputs[3] PrevMatchData unexpected shape: %v", shape)
		}

		// Inputs[4]: GlobalFeatures [B, 4] (Float32)
		if shape := batch.Inputs[4].Shape(); shape.DType != dtypes.Float32 || shape.Dimensions[0] != 4 || shape.Dimensions[1] != 4 {
			t.Errorf("Inputs[4] GlobalFeatures unexpected shape: %v", shape)
		}

		// Inputs[5]: MatchIndices [B] (Int32)
		if shape := batch.Inputs[5].Shape(); shape.DType != dtypes.Int32 || shape.Dimensions[0] != 4 {
			t.Errorf("Inputs[5] MatchIndices unexpected shape: %v", shape)
		}

		// Labels:
		// 0: Win/Loss [B, 1] (Float32)
		if shape := batch.Labels[0].Shape(); shape.DType != dtypes.Float32 || shape.Dimensions[1] != 1 {
			t.Errorf("Labels[0] WinLoss unexpected shape: %v", shape)
		}

		// 1: GoldDiff [B, 1] (Float32)
		if shape := batch.Labels[1].Shape(); shape.DType != dtypes.Float32 || shape.Dimensions[1] != 1 {
			t.Errorf("Labels[1] GoldDiff unexpected shape: %v", shape)
		}

		// 2: KillDiff [B, 1] (Float32)
		if shape := batch.Labels[2].Shape(); shape.DType != dtypes.Float32 || shape.Dimensions[1] != 1 {
			t.Errorf("Labels[2] KillDiff unexpected shape: %v", shape)
		}

		// 3: TotalKills [B, 1] (Float32)
		if shape := batch.Labels[3].Shape(); shape.DType != dtypes.Float32 || shape.Dimensions[1] != 1 {
			t.Errorf("Labels[3] TotalKills unexpected shape: %v", shape)
		}
		break
	}
}

func TestSampler_SummonerEmbeddingLookupTMinus2(t *testing.T) {
	ds := createSyntheticDataset(1, 5) // 5 matches on day 1
	ds.InitSummonerEmbeddings(16)

	// Set distinct embedding for Match 0, Slot 0 (Blue Top player)
	expectedEmb := make([]float32, 16)
	for i := range expectedEmb {
		expectedEmb[i] = float32(i + 10)
	}
	ds.SetSummonerEmbedding(0, 0, expectedEmb)

	// In Match 2 (the 3rd match for player 1), prior matches are Match 1 (t-1) and Match 0 (t-2).
	// Therefore, sampling Match 2 should yield expectedEmb for slot 0 in Inputs[2].
	match2 := ds.Matches[2]
	inputs, _, err := ds.DefaultBatchBuilder([]*data.MatchV5{match2})
	if err != nil {
		t.Fatalf("DefaultBatchBuilder failed: %v", err)
	}

	embTensor := inputs[2] // [1, 10, 16]
	embData := embTensor.Value().([][][]float32)
	retrieved := embData[0][0] // batch 0, slot 0

	for i := 0; i < 16; i++ {
		if retrieved[i] != expectedEmb[i] {
			t.Errorf("expected embedding[%d] = %f, got %f", i, expectedEmb[i], retrieved[i])
		}
	}

	// Match 0 (cold start, 0 prior matches) -> embedding should be all zeros
	match0 := ds.Matches[0]
	inputs0, _, err := ds.DefaultBatchBuilder([]*data.MatchV5{match0})
	if err != nil {
		t.Fatalf("DefaultBatchBuilder failed: %v", err)
	}
	emb0Data := inputs0[2].Value().([][][]float32)
	for i := 0; i < 16; i++ {
		if emb0Data[0][0][i] != 0.0 {
			t.Errorf("expected cold-start embedding[%d] = 0.0, got %f", i, emb0Data[0][0][i])
		}
	}
}

func TestSampler_CustomBatchBuilder(t *testing.T) {
	ds := createSyntheticDataset(2, 5)

	customBuilder := func(matches []*data.MatchV5) (inputs []*tensors.Tensor, labels []*tensors.Tensor, err error) {
		b := len(matches)
		customInput := make([]float32, b)
		for i, m := range matches {
			customInput[i] = float32(m.Info.GameDuration)
		}
		tInput := tensors.FromValue(customInput)
		tLabel := tensors.FromValue([]float32{1.0})
		return []*tensors.Tensor{tInput}, []*tensors.Tensor{tLabel}, nil
	}

	sampler := ds.Sampler(data.SamplerOptions{
		BatchSize:      3,
		RandomSampling: false,
		BatchBuilder:   customBuilder,
	})

	for batch, err := range sampler.Iter() {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(batch.Inputs) != 1 {
			t.Fatalf("expected 1 custom input tensor, got %d", len(batch.Inputs))
		}
		break
	}
}
