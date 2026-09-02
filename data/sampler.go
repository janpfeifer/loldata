package data

import (
	"fmt"
	"iter"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gomlx/gomlx/core/tensors"
	"github.com/gomlx/gomlx/ml/train"
)

// DecayAlpha is the default exponential decay coefficient used for temporal weighting
// when TimeWeight < 1.0. An alpha of 4.0 gives an ~55x higher probability density to the
// earliest match compared to the latest match when TimeWeight == 0.0.
const DecayAlpha = 4.0

// NumPrevMatchFeatures is the total number of features in the previous match data vector
// for each summoner slot.
const NumPrevMatchFeatures = 36

// Feature indices for Previous Match Data vector:
const (
	// Previous match outcome & labels
	PrevFeatMatchWon      = 0  // 1.0 if player won match t-1, 0.0 if lost
	PrevFeatBlueWon       = 1  // 1.0 if Blue won match t-1, 0.0 if Red won
	PrevFeatTeamGoldDiff  = 2  // Player team gold - opponent team gold in match t-1
	PrevFeatTeamKillDiff  = 3  // Player team kills - opponent team kills in match t-1
	PrevFeatTotalKills    = 4  // Total kills by both teams in match t-1
	PrevFeatGameDuration  = 5  // Game duration of match t-1 in seconds

	// Participant performance in match t-1
	PrevFeatChampionID    = 6  // Champion ID played in match t-1
	PrevFeatPosition      = 7  // Position / Role (1=Top, 2=Jungle, 3=Mid, 4=Bot, 5=Support)
	PrevFeatKills         = 8  // Kills scored in match t-1
	PrevFeatDeaths        = 9  // Deaths in match t-1
	PrevFeatAssists       = 10 // Assists in match t-1
	PrevFeatKDARatio      = 11 // (K + A) / max(1, D)
	PrevFeatKillPart      = 12 // (K + A) / TeamKills (0.0 to 1.0)
	PrevFeatDmgToChamps   = 13 // Total damage dealt to champions in match t-1
	PrevFeatDmgShare      = 14 // Damage share of team total (0.0 to 1.0)
	PrevFeatDmgTaken      = 15 // Total damage taken in match t-1
	PrevFeatDmgMitigated  = 16 // Self-mitigated damage in match t-1
	PrevFeatDmgToTurrets  = 17 // Damage dealt to turrets / buildings in match t-1
	PrevFeatTotalCS       = 18 // Total CS (minions + monsters) in match t-1
	PrevFeatCSPM          = 19 // CS per minute in match t-1
	PrevFeatGoldEarned    = 20 // Total gold earned in match t-1
	PrevFeatGPM           = 21 // Gold per minute in match t-1
	PrevFeatGoldShare     = 22 // Gold share of team total (0.0 to 1.0)
	PrevFeatVisionScore   = 23 // Vision score in match t-1
	PrevFeatVSPM          = 24 // Vision score per minute in match t-1
	PrevFeatWardsPlaced   = 25 // Wards placed in match t-1
	PrevFeatWardsKilled   = 26 // Enemy wards destroyed in match t-1
	PrevFeatControlWards  = 27 // Control wards placed/bought in match t-1
	PrevFeatFirstBlood    = 28 // 1.0 if got first blood kill, 0.0 otherwise
	PrevFeatFirstBloodAst = 29 // 1.0 if got first blood assist, 0.0 otherwise
	PrevFeatFirstTower    = 30 // 1.0 if got first tower kill/assist, 0.0 otherwise

	// Temporal & career context
	PrevFeatTimeDeltaHours = 31 // Hours elapsed between match t-1 and current match t
	PrevFeatPatchDelta     = 32 // Patch difference between match t-1 and current match t
	PrevFeatCareerMatches  = 33 // Total matches played by summoner prior to current match t
	PrevFeatCareerWinRate  = 34 // Cumulative win rate across prior matches (0.0 to 1.0)
	PrevFeatIsColdStart    = 35 // 1.0 if summoner has 0 prior recorded matches, 0.0 otherwise
)

// NumGlobalFeatures is the number of match-level global features.
const NumGlobalFeatures = 4

// SamplerOptions configures match filtering, sampling distribution, and batch generation
// for GoMLX ML training and evaluation pipelines.
type SamplerOptions struct {
	// Name identifies the dataset in GoMLX (returned by train.Dataset.Name()).
	// Default is "loldata-sampler".
	Name string

	// BatchSize is the number of matches per batch.
	// Default is 32.
	BatchSize int

	// StartTime filters matches to those with Time() >= StartTime.
	// If zero, no start time filter is applied.
	StartTime time.Time

	// EndTime filters matches to those with Time() < EndTime.
	// If zero, no end time filter is applied.
	EndTime time.Time

	// RandomSampling enables random sampling with replacement instead of chronological iteration.
	// If false, matches are iterated in chronological order for exactly one epoch.
	RandomSampling bool

	// TimeWeight controls temporal bias during random sampling (from 0.0 to 1.0):
	// - 0.0: Much more weight given to earlier in time matches.
	// - 1.0: Uniformly samples matches across the time window.
	// Values outside [0.0, 1.0] will be clamped.
	TimeWeight float64

	// Infinite controls whether random sampling loops indefinitely.
	// When RandomSampling is true, Infinite defaults to true.
	// When RandomSampling is false (sequential), Infinite is false (yields one epoch).
	Infinite *bool

	// RandSeed sets the random seed for deterministic sampling.
	// If 0, uses a non-deterministic time-seeded random generator.
	RandSeed int64

	// Filter is an optional custom predicate to filter matches.
	Filter func(m *MatchV5) bool

	// BatchBuilder is an optional custom function to convert a batch of []*MatchV5 into inputs and labels tensors.
	// If nil, Dataset.DefaultBatchBuilder is used.
	BatchBuilder func(matches []*MatchV5) (inputs []*tensors.Tensor, labels []*tensors.Tensor, err error)

	// Spec is the optional task/batch spec passed to train.Batch.Spec.
	Spec any
}

// MatchSampler implements train.Dataset for sampling matches from a Dataset.
type MatchSampler struct {
	name        string
	dataset     *Dataset
	opts        SamplerOptions
	matches     []*MatchV5
	cumWeights  []float64
	totalWeight float64
	rng         *rand.Rand
	mu          sync.Mutex

	// Cached indices
	puuidToIndex map[string]int32
	matchToIndex map[string]int32
}

var _ train.Dataset = (*MatchSampler)(nil)

// Sampler creates and returns a train.Dataset for training or evaluation with GoMLX.
func (d *Dataset) Sampler(opts SamplerOptions) train.Dataset {
	s, _ := d.NewSampler(opts)
	return s
}

// NewSampler creates a fully initialized *MatchSampler from the dataset using the given options.
func (d *Dataset) NewSampler(opts SamplerOptions) (*MatchSampler, error) {
	if opts.Name == "" {
		opts.Name = "loldata-sampler"
	}
	if opts.BatchSize <= 0 {
		opts.BatchSize = 32
	}

	// Filter and sort matches chronologically
	filtered := d.FilterMatches(opts)

	var rng *rand.Rand
	if opts.RandSeed != 0 {
		rng = rand.New(rand.NewSource(opts.RandSeed))
	} else {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}

	sampler := &MatchSampler{
		name:    opts.Name,
		dataset: d,
		opts:    opts,
		matches: filtered,
		rng:     rng,
	}

	sampler.buildLookups()
	sampler.buildWeights()

	return sampler, nil
}

// Name returns the identifier of the dataset (satisfies train.Dataset).
func (s *MatchSampler) Name() string {
	return s.name
}

// Options returns the SamplerOptions configured for this sampler.
func (s *MatchSampler) Options() SamplerOptions {
	return s.opts
}

// FilteredMatches returns the slice of matches matching the filter, sorted chronologically.
func (s *MatchSampler) FilteredMatches() []*MatchV5 {
	return s.matches
}

// NumMatches returns the number of filtered matches in the sampler.
func (s *MatchSampler) NumMatches() int {
	return len(s.matches)
}

// buildLookups caches PUUID and MatchID integer indices for fast tensor feature extraction.
func (s *MatchSampler) buildLookups() {
	if s.dataset == nil {
		return
	}
	s.puuidToIndex = make(map[string]int32, len(s.dataset.Summoners))
	for idx, summoner := range s.dataset.Summoners {
		if summoner != nil && summoner.PUUID != "" {
			s.puuidToIndex[summoner.PUUID] = int32(idx)
		}
	}

	s.matchToIndex = make(map[string]int32, len(s.dataset.Matches))
	for idx, m := range s.dataset.Matches {
		if m != nil && m.Metadata.MatchID != "" {
			s.matchToIndex[m.Metadata.MatchID] = int32(idx)
		}
	}
}

// buildWeights computes the cumulative probability distribution for temporal random sampling.
func (s *MatchSampler) buildWeights() {
	n := len(s.matches)
	if n == 0 {
		s.cumWeights = nil
		s.totalWeight = 0
		return
	}

	w := s.opts.TimeWeight
	if w < 0.0 {
		w = 0.0
	} else if w > 1.0 {
		w = 1.0
	}

	s.cumWeights = make([]float64, n)
	t0 := s.matches[0].Time()
	tMax := s.matches[n-1].Time()
	dt := tMax.Sub(t0)

	total := 0.0
	for i, m := range s.matches {
		var tau float64
		if dt > 0 {
			tau = float64(m.Time().Sub(t0)) / float64(dt)
		} else if n > 1 {
			tau = float64(i) / float64(n-1)
		} else {
			tau = 0.0
		}

		// Weight function: W = exp(-DecayAlpha * (1.0 - w) * tau)
		// When w == 1.0 (uniform), W == 1.0 for all matches.
		// When w == 0.0 (early-biased), W decays from 1.0 down to exp(-DecayAlpha).
		weightVal := math.Exp(-DecayAlpha * (1.0 - w) * tau)
		total += weightVal
		s.cumWeights[i] = total
	}
	s.totalWeight = total
}

// SampleMatch draws one match from the filtered matches according to the configured sampling distribution.
func (s *MatchSampler) SampleMatch() *MatchV5 {
	n := len(s.matches)
	if n == 0 {
		return nil
	}
	if n == 1 {
		return s.matches[0]
	}

	s.mu.Lock()
	r := s.rng.Float64() * s.totalWeight
	s.mu.Unlock()

	idx := sort.Search(n, func(i int) bool {
		return s.cumWeights[i] >= r
	})
	if idx >= n {
		idx = n - 1
	}
	return s.matches[idx]
}

// Iter returns an iterator over batches (satisfies train.Dataset).
// If RandomSampling is true, it yields batches sampled with replacement (indefinitely by default).
// If RandomSampling is false, it yields batches in chronological order for exactly one epoch.
func (s *MatchSampler) Iter() iter.Seq2[train.Batch, error] {
	return func(yield func(train.Batch, error) bool) {
		if len(s.matches) == 0 {
			return
		}

		batchSize := s.opts.BatchSize
		if batchSize <= 0 {
			batchSize = 32
		}

		batchBuilder := s.opts.BatchBuilder
		if batchBuilder == nil {
			batchBuilder = s.dataset.DefaultBatchBuilder
		}

		if s.opts.RandomSampling {
			infinite := true
			if s.opts.Infinite != nil {
				infinite = *s.opts.Infinite
			}

			numBatches := (len(s.matches) + batchSize - 1) / batchSize
			batchCount := 0

			for {
				if !infinite && batchCount >= numBatches {
					return
				}

				batchMatches := make([]*MatchV5, batchSize)
				for i := 0; i < batchSize; i++ {
					batchMatches[i] = s.SampleMatch()
				}

				inputs, labels, err := batchBuilder(batchMatches)
				batch := train.Batch{
					Inputs: inputs,
					Labels: labels,
					Spec:   s.opts.Spec,
				}
				if !yield(batch, err) {
					return
				}
				if err != nil {
					return
				}
				batchCount++
			}
		} else {
			// Chronological / Temporal sequential iteration (1 epoch)
			for i := 0; i < len(s.matches); i += batchSize {
				end := i + batchSize
				if end > len(s.matches) {
					end = len(s.matches)
				}
				batchMatches := s.matches[i:end]
				inputs, labels, err := batchBuilder(batchMatches)
				batch := train.Batch{
					Inputs: inputs,
					Labels: labels,
					Spec:   s.opts.Spec,
				}
				if !yield(batch, err) {
					return
				}
				if err != nil {
					return
				}
			}
		}
	}
}

// FilterMatches filters and sorts matches from the dataset according to the given SamplerOptions.
func (d *Dataset) FilterMatches(opts SamplerOptions) []*MatchV5 {
	var filtered []*MatchV5
	for _, m := range d.Matches {
		if m == nil {
			continue
		}
		mTime := m.Time()
		if !opts.StartTime.IsZero() && mTime.Before(opts.StartTime) {
			continue
		}
		if !opts.EndTime.IsZero() && !mTime.Before(opts.EndTime) {
			continue
		}
		if opts.Filter != nil && !opts.Filter(m) {
			continue
		}
		filtered = append(filtered, m)
	}

	// Sort chronologically by match time
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Time().Before(filtered[j].Time())
	})

	return filtered
}

// StandardizeParticipants returns the 10 participants of a match organized into fixed slots:
// Slots 0..4: Blue Team (Top=0, Jungle=1, Mid=2, Bot=3, Support=4)
// Slots 5..9: Red Team (Top=5, Jungle=6, Mid=7, Bot=8, Support=9)
func StandardizeParticipants(m *MatchV5) [10]*ParticipantDto {
	var slots [10]*ParticipantDto
	if m == nil || m.Info.Participants == nil {
		return slots
	}

	var blueUnassigned []*ParticipantDto
	var redUnassigned []*ParticipantDto

	for _, p := range m.Info.Participants {
		if p == nil {
			continue
		}
		isBlue := p.TeamID == 100 || (p.Esports != nil && p.Esports.Side == SideBlue)
		isRed := p.TeamID == 200 || (p.Esports != nil && p.Esports.Side == SideRed)

		// Parse position
		pos := p.Position
		if pos == PositionUnknown && p.Esports != nil {
			pos = p.Esports.Position
		}
		if pos == PositionUnknown && p.TeamPosition != "" {
			pos = ParsePosition(p.TeamPosition)
		}
		if pos == PositionUnknown && p.IndividualPosition != "" {
			pos = ParsePosition(p.IndividualPosition)
		}

		slotOffset := -1
		switch pos {
		case PositionTop:
			slotOffset = 0
		case PositionJungle:
			slotOffset = 1
		case PositionMid:
			slotOffset = 2
		case PositionBot:
			slotOffset = 3
		case PositionSupport:
			slotOffset = 4
		}

		if isBlue {
			if slotOffset >= 0 && slots[slotOffset] == nil {
				slots[slotOffset] = p
			} else {
				blueUnassigned = append(blueUnassigned, p)
			}
		} else if isRed {
			if slotOffset >= 0 && slots[5+slotOffset] == nil {
				slots[5+slotOffset] = p
			} else {
				redUnassigned = append(redUnassigned, p)
			}
		} else {
			if len(blueUnassigned) < 5 {
				blueUnassigned = append(blueUnassigned, p)
			} else {
				redUnassigned = append(redUnassigned, p)
			}
		}
	}

	// Fill remaining empty blue slots
	for i := 0; i < 5; i++ {
		if slots[i] == nil && len(blueUnassigned) > 0 {
			slots[i] = blueUnassigned[0]
			blueUnassigned = blueUnassigned[1:]
		}
	}

	// Fill remaining empty red slots
	for i := 5; i < 10; i++ {
		if slots[i] == nil && len(redUnassigned) > 0 {
			slots[i] = redUnassigned[0]
			redUnassigned = redUnassigned[1:]
		}
	}

	return slots
}

// DefaultBatchBuilder converts a slice of MatchV5 objects into GoMLX tensors according to
// the architecture defined in docs/model.md.
//
// Inputs:
// - Inputs[0]: Champion IDs [B, 2, 5] (int32: side 0=Blue, side 1=Red; positions Top..Support in order 0..4)
// - Inputs[1]: Summoner Indices [B, 2, 5] (int32: index in Dataset.Summoners or -1)
// - Inputs[2]: Prior Summoner Embeddings (2 matches before, h_u^(t-2)) [B, 10, SummonerEmbeddingDim] (float32)
// - Inputs[3]: Previous Match Data (match t-1 labels & summoner stats) [B, 10, 36] (float32)
// - Inputs[4]: Match Global Features [B, 4] (float32: queue ID, map ID, duration, patch)
// - Inputs[5]: Match Indices in Dataset [B] (int32)
//
// Labels (Multi-Task):
// - Labels[0]: Match Winner [B, 1] (float32: 1.0 if Blue won, 0.0 if Red won)
// - Labels[1]: Gold Difference [B, 1] (float32: Blue total gold - Red total gold)
// - Labels[2]: Kill Difference [B, 1] (float32: Blue total kills - Red total kills)
// - Labels[3]: Total Kills [B, 1] (float32: Blue total kills + Red total kills)
func (d *Dataset) DefaultBatchBuilder(matches []*MatchV5) (inputs []*tensors.Tensor, labels []*tensors.Tensor, err error) {
	b := len(matches)
	if b == 0 {
		return nil, nil, nil
	}

	embDim := DefaultSummonerEmbeddingDim
	if d != nil && d.SummonerEmbeddingDim > 0 {
		embDim = d.SummonerEmbeddingDim
	}

	// Allocate feature slices
	champIDs := make([][][]int32, b)           // [B, 2, 5]
	summonerIndices := make([][][]int32, b)    // [B, 2, 5]
	priorEmbeddings := make([][][]float32, b)  // [B, 10, SummonerEmbeddingDim]
	prevMatchData := make([][][]float32, b)    // [B, 10, 36]
	globalFeatures := make([][]float32, b)     // [B, 4]
	matchIndices := make([]int32, b)           // [B]

	winLoss := make([][]float32, b)
	goldDiff := make([][]float32, b)
	killDiff := make([][]float32, b)
	totalKills := make([][]float32, b)

	for i, m := range matches {
		champIDs[i] = make([][]int32, 2)
		champIDs[i][0] = make([]int32, 5)
		champIDs[i][1] = make([]int32, 5)

		summonerIndices[i] = make([][]int32, 2)
		summonerIndices[i][0] = make([]int32, 5)
		summonerIndices[i][1] = make([]int32, 5)

		priorEmbeddings[i] = make([][]float32, 10)
		prevMatchData[i] = make([][]float32, 10)
		globalFeatures[i] = make([]float32, NumGlobalFeatures)

		for slot := 0; slot < 10; slot++ {
			priorEmbeddings[i][slot] = make([]float32, embDim)
			prevMatchData[i][slot] = make([]float32, NumPrevMatchFeatures)
		}

		winLoss[i] = make([]float32, 1)
		goldDiff[i] = make([]float32, 1)
		killDiff[i] = make([]float32, 1)
		totalKills[i] = make([]float32, 1)

		if m == nil {
			continue
		}

		mTime := m.Time()

		// Match index lookup in d.Matches
		matchIdx := int32(-1)
		if d != nil && m.Metadata.MatchID != "" {
			if _, ok := d.MatchIDToMatch[m.Metadata.MatchID]; ok {
				for mi, matchPtr := range d.Matches {
					if matchPtr == m {
						matchIdx = int32(mi)
						break
					}
				}
			}
		}
		matchIndices[i] = matchIdx

		// Global features: QueueID, MapID, GameDuration, Patch
		globalFeatures[i][0] = float32(m.Info.QueueID)
		globalFeatures[i][1] = float32(m.Info.MapID)
		globalFeatures[i][2] = float32(m.Info.GameDuration)
		curPatch := parsePatchNumber(m.Info.GameVersion, m.Esports)
		globalFeatures[i][3] = curPatch

		// Standardize 10 participant slots
		slots := StandardizeParticipants(m)

		for slot := 0; slot < 10; slot++ {
			p := slots[slot]
			sideIdx := slot / 5 // 0 for Blue, 1 for Red
			posIdx := slot % 5  // 0=Top, 1=Jungle, 2=Mid, 3=Bot, 4=Support

			if p == nil {
				summonerIndices[i][sideIdx][posIdx] = -1
				prevMatchData[i][slot][PrevFeatIsColdStart] = 1.0
				continue
			}

			champIDs[i][sideIdx][posIdx] = int32(p.ChampionID)

			// Find summoner in dataset
			var s *SummonerV4
			if p.Summoner != nil {
				s = p.Summoner
			} else if d != nil && p.PUUID != "" {
				s = d.GetSummoner(p.PUUID)
			}

			if s != nil && d != nil {
				sIdx := int32(-1)
				for si, sumPtr := range d.Summoners {
					if sumPtr == s {
						sIdx = int32(si)
						break
					}
				}
				summonerIndices[i][sideIdx][posIdx] = sIdx
			} else {
				summonerIndices[i][sideIdx][posIdx] = -1
			}

			if s != nil && len(s.Matches) > 0 && !mTime.IsZero() {
				// Search matches played strictly before match m
				priorCount := sort.Search(len(s.Matches), func(idx int) bool {
					return !s.Matches[idx].Time().Before(mTime)
				})

				// 1. Summoner Embedding from 2 matches before (h_u^(t-2))
				if priorCount >= 2 && d != nil {
					mMinus2 := s.Matches[priorCount-2]
					mMinus2Idx := -1
					for mi, matchPtr := range d.Matches {
						if matchPtr == mMinus2 {
							mMinus2Idx = mi
							break
						}
					}
					if mMinus2Idx >= 0 {
						// Find slot of s in mMinus2
						slotsMinus2 := StandardizeParticipants(mMinus2)
						slotMinus2 := -1
						for k := 0; k < 10; k++ {
							if slotsMinus2[k] != nil && (slotsMinus2[k].Summoner == s || slotsMinus2[k].PUUID == s.PUUID) {
								slotMinus2 = k
								break
							}
						}
						if slotMinus2 >= 0 {
							emb := d.GetSummonerEmbedding(mMinus2Idx, slotMinus2)
							if len(emb) == embDim {
								copy(priorEmbeddings[i][slot], emb)
							}
						}
					}
				}

				// 2. Previous Match Data (from match t-1)
				if priorCount >= 1 {
					prevMatchData[i][slot][PrevFeatCareerMatches] = float32(priorCount)
					prevMatchData[i][slot][PrevFeatIsColdStart] = 0.0

					mPrev := s.Matches[priorCount-1]
					dtHours := float32(mTime.Sub(mPrev.Time()).Hours())
					if dtHours < 0 {
						dtHours = 0
					}
					prevMatchData[i][slot][PrevFeatTimeDeltaHours] = dtHours

					prevPatch := parsePatchNumber(mPrev.Info.GameVersion, mPrev.Esports)
					prevMatchData[i][slot][PrevFeatPatchDelta] = curPatch - prevPatch
					prevMatchData[i][slot][PrevFeatGameDuration] = float32(mPrev.Info.GameDuration)

					// Find participant pPrev in mPrev
					var pPrev *ParticipantDto
					for _, part := range mPrev.Info.Participants {
						if part != nil && (part.Summoner == s || (part.PUUID != "" && part.PUUID == s.PUUID)) {
							pPrev = part
							break
						}
					}

					// Previous match outcome & labels
					blueTeamPrev := mPrev.GetTeamBySide(SideBlue)
					redTeamPrev := mPrev.GetTeamBySide(SideRed)

					blueWonPrev := false
					if blueTeamPrev != nil && blueTeamPrev.Win {
						blueWonPrev = true
					}
					if blueWonPrev {
						prevMatchData[i][slot][PrevFeatBlueWon] = 1.0
					}

					// Determine previous match gold and kills
					blueGoldPrev, redGoldPrev := 0, 0
					blueKillsPrev, redKillsPrev := 0, 0
					if blueTeamPrev != nil && blueTeamPrev.Esports != nil {
						blueGoldPrev = blueTeamPrev.Esports.TotalGold
						blueKillsPrev = blueTeamPrev.Esports.TeamKills
					}
					if redTeamPrev != nil && redTeamPrev.Esports != nil {
						redGoldPrev = redTeamPrev.Esports.TotalGold
						redKillsPrev = redTeamPrev.Esports.TeamKills
					}

					if pPrev != nil {
						isBluePrev := pPrev.TeamID == 100 || (pPrev.Esports != nil && pPrev.Esports.Side == SideBlue)
						myWin := pPrev.Win || (mPrev.GetTeam(pPrev.TeamID) != nil && mPrev.GetTeam(pPrev.TeamID).Win)
						if myWin {
							prevMatchData[i][slot][PrevFeatMatchWon] = 1.0
						}

						var myGold, oppGold, myTeamKills, oppTeamKills int
						if isBluePrev {
							myGold = blueGoldPrev
							oppGold = redGoldPrev
							myTeamKills = blueKillsPrev
							oppTeamKills = redKillsPrev
						} else {
							myGold = redGoldPrev
							oppGold = blueGoldPrev
							myTeamKills = redKillsPrev
							oppTeamKills = blueKillsPrev
						}
						prevMatchData[i][slot][PrevFeatTeamGoldDiff] = float32(myGold - oppGold)
						prevMatchData[i][slot][PrevFeatTeamKillDiff] = float32(myTeamKills - oppTeamKills)
						prevMatchData[i][slot][PrevFeatTotalKills] = float32(myTeamKills + oppTeamKills)

						// Participant stats in match t-1
						prevMatchData[i][slot][PrevFeatChampionID] = float32(pPrev.ChampionID)
						pos := pPrev.Position
						if pos == PositionUnknown && pPrev.Esports != nil {
							pos = pPrev.Esports.Position
						}
						prevMatchData[i][slot][PrevFeatPosition] = float32(pos)
						prevMatchData[i][slot][PrevFeatKills] = float32(pPrev.Kills)
						prevMatchData[i][slot][PrevFeatDeaths] = float32(pPrev.Deaths)
						prevMatchData[i][slot][PrevFeatAssists] = float32(pPrev.Assists)

						kdaDenom := pPrev.Deaths
						if kdaDenom <= 0 {
							kdaDenom = 1
						}
						prevMatchData[i][slot][PrevFeatKDARatio] = float32(pPrev.Kills+pPrev.Assists) / float32(kdaDenom)

						if myTeamKills > 0 {
							prevMatchData[i][slot][PrevFeatKillPart] = float32(pPrev.Kills+pPrev.Assists) / float32(myTeamKills)
						}

						dmg := pPrev.TotalDamageDealtToChampions
						if dmg == 0 && pPrev.Esports != nil {
							dmg = pPrev.Esports.DamageToChampions
						}
						prevMatchData[i][slot][PrevFeatDmgToChamps] = float32(dmg)
						if pPrev.Esports != nil && pPrev.Esports.DamageShare > 0 {
							prevMatchData[i][slot][PrevFeatDmgShare] = float32(pPrev.Esports.DamageShare)
						}
						prevMatchData[i][slot][PrevFeatDmgTaken] = float32(pPrev.TotalDamageTaken)
						prevMatchData[i][slot][PrevFeatDmgMitigated] = float32(pPrev.DamageSelfMitigated)

						dmgTurrets := pPrev.DamageDealtToTurrets
						if dmgTurrets == 0 {
							dmgTurrets = pPrev.DamageDealtToBuildings
						}
						prevMatchData[i][slot][PrevFeatDmgToTurrets] = float32(dmgTurrets)

						cs := pPrev.TotalMinionsKilled + pPrev.NeutralMinionsKilled
						if cs == 0 && pPrev.Esports != nil {
							cs = pPrev.Esports.TotalCS
						}
						prevMatchData[i][slot][PrevFeatTotalCS] = float32(cs)
						durMin := float32(mPrev.Info.GameDuration) / 60.0
						if durMin > 0 {
							prevMatchData[i][slot][PrevFeatCSPM] = float32(cs) / durMin
						}

						gold := pPrev.GoldEarned
						if gold == 0 && pPrev.Esports != nil {
							gold = pPrev.Esports.TotalGold
						}
						prevMatchData[i][slot][PrevFeatGoldEarned] = float32(gold)
						if durMin > 0 {
							prevMatchData[i][slot][PrevFeatGPM] = float32(gold) / durMin
						}
						if myGold > 0 {
							prevMatchData[i][slot][PrevFeatGoldShare] = float32(gold) / float32(myGold)
						}

						vision := pPrev.VisionScore
						if vision == 0 && pPrev.Esports != nil {
							vision = int(pPrev.Esports.VisionScore)
						}
						prevMatchData[i][slot][PrevFeatVisionScore] = float32(vision)
						if durMin > 0 {
							prevMatchData[i][slot][PrevFeatVSPM] = float32(vision) / durMin
						}

						wards := pPrev.WardsPlaced
						if wards == 0 && pPrev.Esports != nil {
							wards = pPrev.Esports.WardsPlaced
						}
						prevMatchData[i][slot][PrevFeatWardsPlaced] = float32(wards)

						wardsKilled := pPrev.WardsKilled
						if wardsKilled == 0 && pPrev.Esports != nil {
							wardsKilled = pPrev.Esports.WardsKilled
						}
						prevMatchData[i][slot][PrevFeatWardsKilled] = float32(wardsKilled)

						ctrlWards := pPrev.DetectorWardsPlaced
						if ctrlWards == 0 {
							ctrlWards = pPrev.VisionWardsBoughtInGame
						}
						if ctrlWards == 0 && pPrev.Esports != nil {
							ctrlWards = pPrev.Esports.ControlWardsBought
						}
						prevMatchData[i][slot][PrevFeatControlWards] = float32(ctrlWards)

						if pPrev.FirstBloodKill || (pPrev.Esports != nil && pPrev.Esports.FirstBloodKill) {
							prevMatchData[i][slot][PrevFeatFirstBlood] = 1.0
						}
						if pPrev.FirstBloodAssist || (pPrev.Esports != nil && pPrev.Esports.FirstBloodAssist) {
							prevMatchData[i][slot][PrevFeatFirstBloodAst] = 1.0
						}
						if pPrev.FirstTowerKill || pPrev.FirstTowerAssist {
							prevMatchData[i][slot][PrevFeatFirstTower] = 1.0
						}
					}

					// Career win rate prior to match m
					priorWins := 0
					for j := 0; j < priorCount; j++ {
						mPrior := s.Matches[j]
						for _, part := range mPrior.Info.Participants {
							if part != nil && (part.Summoner == s || (part.PUUID != "" && part.PUUID == s.PUUID)) {
								if part.Win || (mPrior.GetTeam(part.TeamID) != nil && mPrior.GetTeam(part.TeamID).Win) {
									priorWins++
								}
								break
							}
						}
					}
					prevMatchData[i][slot][PrevFeatCareerWinRate] = float32(priorWins) / float32(priorCount)
				} else {
					prevMatchData[i][slot][PrevFeatIsColdStart] = 1.0
				}
			} else {
				prevMatchData[i][slot][PrevFeatIsColdStart] = 1.0
			}
		}

		// Labels extraction
		blueTeam := m.GetTeamBySide(SideBlue)
		redTeam := m.GetTeamBySide(SideRed)

		blueWin := false
		if blueTeam != nil && blueTeam.Win {
			blueWin = true
		} else if redTeam != nil && !redTeam.Win {
			blueWin = true
		}
		if blueWin {
			winLoss[i][0] = 1.0
		} else {
			winLoss[i][0] = 0.0
		}

		// Calculate gold and kills for Blue and Red
		blueGold, redGold := 0, 0
		blueKills, redKills := 0, 0

		if blueTeam != nil && blueTeam.Esports != nil {
			blueGold = blueTeam.Esports.TotalGold
			blueKills = blueTeam.Esports.TeamKills
		}
		if redTeam != nil && redTeam.Esports != nil {
			redGold = redTeam.Esports.TotalGold
			redKills = redTeam.Esports.TeamKills
		}

		// Fallback from participants if team total is zero
		if blueGold == 0 || redGold == 0 || (blueKills == 0 && redKills == 0) {
			pBlueGold, pRedGold := 0, 0
			pBlueKills, pRedKills := 0, 0
			for slot := 0; slot < 5; slot++ {
				if p := slots[slot]; p != nil {
					pBlueGold += p.GoldEarned
					pBlueKills += p.Kills
				}
			}
			for slot := 5; slot < 10; slot++ {
				if p := slots[slot]; p != nil {
					pRedGold += p.GoldEarned
					pRedKills += p.Kills
				}
			}
			if blueGold == 0 {
				blueGold = pBlueGold
			}
			if redGold == 0 {
				redGold = pRedGold
			}
			if blueKills == 0 {
				blueKills = pBlueKills
			}
			if redKills == 0 {
				redKills = pRedKills
			}
		}

		goldDiff[i][0] = float32(blueGold - redGold)
		killDiff[i][0] = float32(blueKills - redKills)
		totalKills[i][0] = float32(blueKills + redKills)
	}

	// Create GoMLX tensors
	tChampIDs := tensors.FromValue(champIDs)                 // [B, 2, 5]
	tSummonerIndices := tensors.FromValue(summonerIndices)   // [B, 2, 5]
	tPriorEmbeddings := tensors.FromValue(priorEmbeddings)   // [B, 10, SummonerEmbeddingDim]
	tPrevMatchData := tensors.FromValue(prevMatchData)       // [B, 10, 36]
	tGlobalFeatures := tensors.FromValue(globalFeatures)     // [B, 4]
	tMatchIndices := tensors.FromValue(matchIndices)         // [B]

	tWinLoss := tensors.FromValue(winLoss)
	tGoldDiff := tensors.FromValue(goldDiff)
	tKillDiff := tensors.FromValue(killDiff)
	tTotalKills := tensors.FromValue(totalKills)

	inputs = []*tensors.Tensor{
		tChampIDs,
		tSummonerIndices,
		tPriorEmbeddings,
		tPrevMatchData,
		tGlobalFeatures,
		tMatchIndices,
	}

	labels = []*tensors.Tensor{
		tWinLoss,
		tGoldDiff,
		tKillDiff,
		tTotalKills,
	}

	return inputs, labels, nil
}

// parsePatchNumber extracts a numeric patch version (e.g. 14.1 -> 14.01) from gameVersion or esports patch.
func parsePatchNumber(version string, esports *EsportsMatchData) float32 {
	patchStr := ""
	if esports != nil && esports.Patch != "" {
		patchStr = esports.Patch
	} else if version != "" {
		parts := strings.Split(version, ".")
		if len(parts) >= 2 {
			patchStr = parts[0] + "." + parts[1]
		}
	}
	if patchStr == "" {
		return 0.0
	}
	val, err := strconv.ParseFloat(patchStr, 32)
	if err != nil {
		parts := strings.Split(patchStr, ".")
		if len(parts) > 0 {
			if v, e := strconv.ParseFloat(parts[0], 32); e == nil {
				return float32(v)
			}
		}
		return 0.0
	}
	return float32(val)
}

// String returns a human-readable summary of the sampler status and configuration.
func (s *MatchSampler) String() string {
	return fmt.Sprintf("MatchSampler(name=%q, matches=%d, batchSize=%d, random=%v, timeWeight=%.2f)",
		s.name, len(s.matches), s.opts.BatchSize, s.opts.RandomSampling, s.opts.TimeWeight)
}
