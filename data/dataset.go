package data

// DefaultSummonerEmbeddingDim is the default dimensionality for summoner dynamic embeddings.
const DefaultSummonerEmbeddingDim = 64

// Dataset represents a collection of matches and summoners with indexing for fast lookups.
type Dataset struct {
	// Matches contains all matches loaded into the dataset.
	Matches []*MatchV5 `json:"matches"`

	// Summoners contains all summoners/players in the dataset.
	Summoners []*SummonerV4 `json:"summoners"`

	// Saved indicates whether the dataset is saved to disk and has had no modifications since.
	Saved bool `json:"-"`

	// PUUIDToSummoner indexes summoners by their PUUID (or OE player ID).
	PUUIDToSummoner map[string]*SummonerV4 `json:"-"`

	// MatchIDToMatch indexes matches by their unique match/game ID.
	MatchIDToMatch map[string]*MatchV5 `json:"-"`

	// SummonerEmbeddingDim is the dimensionality of each summoner embedding vector.
	// Defaults to DefaultSummonerEmbeddingDim (64).
	SummonerEmbeddingDim int `json:"-"`

	// SummonerEmbeddings is a transient (not saved/loaded) table storing the embedding snapshot
	// for each of the 10 participant slots per match, stored as a flat slice of shape
	// [NumMatches, 10, SummonerEmbeddingDim].
	SummonerEmbeddings []float32 `json:"-"`
}

// NewDataset initializes and returns an empty Dataset.
func NewDataset() *Dataset {
	return &Dataset{
		Matches:         make([]*MatchV5, 0),
		Summoners:       make([]*SummonerV4, 0),
		PUUIDToSummoner: make(map[string]*SummonerV4),
		MatchIDToMatch:  make(map[string]*MatchV5),
	}
}

// GetSummoner looks up a summoner by PUUID. Returns nil if not found.
func (d *Dataset) GetSummoner(puuid string) *SummonerV4 {
	if d.PUUIDToSummoner == nil {
		return nil
	}
	return d.PUUIDToSummoner[puuid]
}

// NumCrawledSummoners returns the number of summoners marked as crawled in the dataset.
func (d *Dataset) NumCrawledSummoners() int {
	if d == nil {
		return 0
	}
	count := 0
	for _, s := range d.Summoners {
		if s != nil && s.Crawled {
			count++
		}
	}
	return count
}

// NumSummonersWithProfile returns the number of summoners with Summoner-V4 profile information in the dataset.
func (d *Dataset) NumSummonersWithProfile() int {
	if d == nil {
		return 0
	}
	count := 0
	for _, s := range d.Summoners {
		if s != nil && s.HasProfile() {
			count++
		}
	}
	return count
}

// GetMatch looks up a match by its MatchID. Returns nil if not found.
func (d *Dataset) GetMatch(matchID string) *MatchV5 {
	if d.MatchIDToMatch == nil {
		return nil
	}
	return d.MatchIDToMatch[matchID]
}

// GetOrCreateSummoner retrieves an existing summoner or creates and registers a new one.
func (d *Dataset) GetOrCreateSummoner(puuid, name string) *SummonerV4 {
	if d.PUUIDToSummoner == nil {
		d.PUUIDToSummoner = make(map[string]*SummonerV4)
	}

	if s, exists := d.PUUIDToSummoner[puuid]; exists {
		if s.Name == "" && name != "" {
			s.Name = name
			d.Saved = false
		}
		return s
	}

	s := &SummonerV4{
		PUUID:   puuid,
		Name:    name,
		Matches: make([]*MatchV5, 0),
	}
	d.Summoners = append(d.Summoners, s)
	d.PUUIDToSummoner[puuid] = s
	d.Saved = false
	return s
}

// AddMatch adds a match to the dataset and establishes bidirectional links
// with all participant summoners.
func (d *Dataset) AddMatch(m *MatchV5) {
	if m == nil {
		return
	}
	if d.MatchIDToMatch == nil {
		d.MatchIDToMatch = make(map[string]*MatchV5)
	}
	if d.PUUIDToSummoner == nil {
		d.PUUIDToSummoner = make(map[string]*SummonerV4)
	}

	matchID := m.Metadata.MatchID
	if matchID != "" {
		if _, exists := d.MatchIDToMatch[matchID]; exists {
			// Already added, skip
			return
		}
		d.MatchIDToMatch[matchID] = m
	}

	d.Matches = append(d.Matches, m)
	d.Saved = false

	// Link participants and summoners
	for _, p := range m.Info.Participants {
		if p == nil || p.PUUID == "" {
			continue
		}

		name := p.SummonerName
		if p.RiotIDGameName != "" {
			if p.RiotIDTagline != "" {
				name = p.RiotIDGameName + "#" + p.RiotIDTagline
			} else if name == "" {
				name = p.RiotIDGameName
			}
		}
		if name == "" && p.Esports != nil {
			name = p.Esports.PlayerName
		}

		s := d.GetOrCreateSummoner(p.PUUID, name)
		p.Summoner = s
		s.AddMatch(m)
	}
}

// InitSummonerEmbeddings allocates or reallocates the transient summoner embeddings table
// with the given dimension.
func (d *Dataset) InitSummonerEmbeddings(dim int) {
	if dim <= 0 {
		dim = DefaultSummonerEmbeddingDim
	}
	d.SummonerEmbeddingDim = dim
	totalElements := len(d.Matches) * 10 * dim
	d.SummonerEmbeddings = make([]float32, totalElements)
}

// EnsureSummonerEmbeddings guarantees that the transient summoner embeddings table is initialized
// with the correct size for the current number of matches.
func (d *Dataset) EnsureSummonerEmbeddings() {
	if d.SummonerEmbeddingDim <= 0 {
		d.SummonerEmbeddingDim = DefaultSummonerEmbeddingDim
	}
	expectedSize := len(d.Matches) * 10 * d.SummonerEmbeddingDim
	if len(d.SummonerEmbeddings) != expectedSize {
		d.SummonerEmbeddings = make([]float32, expectedSize)
	}
}

// GetSummonerEmbedding returns a slice pointing to the embedding of the summoner in the specified
// match index (0..len(Matches)-1) and participant slot (0..9).
// If indices are out of range or table is uninitialized, returns nil.
func (d *Dataset) GetSummonerEmbedding(matchIdx, slot int) []float32 {
	if d == nil || matchIdx < 0 || matchIdx >= len(d.Matches) || slot < 0 || slot >= 10 {
		return nil
	}
	dim := d.SummonerEmbeddingDim
	if dim <= 0 {
		dim = DefaultSummonerEmbeddingDim
	}
	offset := (matchIdx*10 + slot) * dim
	if offset+dim > len(d.SummonerEmbeddings) {
		return nil
	}
	return d.SummonerEmbeddings[offset : offset+dim]
}

// SetSummonerEmbedding writes the embedding values for the specified match index and participant slot.
func (d *Dataset) SetSummonerEmbedding(matchIdx, slot int, emb []float32) {
	if d == nil || matchIdx < 0 || matchIdx >= len(d.Matches) || slot < 0 || slot >= 10 {
		return
	}
	d.EnsureSummonerEmbeddings()
	dim := d.SummonerEmbeddingDim
	offset := (matchIdx*10 + slot) * dim
	copy(d.SummonerEmbeddings[offset:offset+dim], emb)
}

// ClearNonRanked removes all non-ranked matches from the dataset and eliminates any summoners
// that are left with no associated matches. Returns the count of removed matches and summoners.
func (d *Dataset) ClearNonRanked() (int, int) {
	if d == nil {
		return 0, 0
	}

	// 1. Filter dataset matches
	initialMatches := len(d.Matches)
	var keptMatches []*MatchV5
	newMatchIDToMatch := make(map[string]*MatchV5, len(d.Matches))
	for _, m := range d.Matches {
		if m != nil && m.IsRanked() {
			keptMatches = append(keptMatches, m)
			if m.Metadata.MatchID != "" {
				newMatchIDToMatch[m.Metadata.MatchID] = m
			}
		}
	}
	d.Matches = keptMatches
	d.MatchIDToMatch = newMatchIDToMatch
	removedMatches := initialMatches - len(keptMatches)

	// 2. Clean up matches on each summoner
	for _, s := range d.Summoners {
		if s == nil {
			continue
		}
		var keptSummonerMatches []*MatchV5
		for _, m := range s.Matches {
			if m != nil && m.IsRanked() {
				keptSummonerMatches = append(keptSummonerMatches, m)
			}
		}
		s.Matches = keptSummonerMatches
	}

	// 3. Filter summoners without matches
	initialSummoners := len(d.Summoners)
	var keptSummoners []*SummonerV4
	newPUUIDToSummoner := make(map[string]*SummonerV4, len(d.Summoners))
	for _, s := range d.Summoners {
		if s != nil && len(s.Matches) > 0 {
			keptSummoners = append(keptSummoners, s)
			if s.PUUID != "" {
				newPUUIDToSummoner[s.PUUID] = s
			}
		}
	}
	d.Summoners = keptSummoners
	d.PUUIDToSummoner = newPUUIDToSummoner
	removedSummoners := initialSummoners - len(keptSummoners)

	if removedMatches > 0 || removedSummoners > 0 {
		d.Saved = false
		// Invalidate transient embeddings as match indices have shifted
		d.SummonerEmbeddings = nil
	}

	return removedMatches, removedSummoners
}
