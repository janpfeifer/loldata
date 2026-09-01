package data

// Dataset represents a collection of matches and summoners with indexing for fast lookups.
type Dataset struct {
	// Matches contains all matches loaded into the dataset.
	Matches []*MatchV5 `json:"matches"`

	// Summoners contains all summoners/players in the dataset.
	Summoners []*SummonerV4 `json:"summoners"`

	// PUUIDToSummoner indexes summoners by their PUUID (or OE player ID).
	PUUIDToSummoner map[string]*SummonerV4 `json:"-"`

	// MatchIDToMatch indexes matches by their unique match/game ID.
	MatchIDToMatch map[string]*MatchV5 `json:"-"`
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

	// Link participants and summoners
	for _, p := range m.Info.Participants {
		if p == nil || p.PUUID == "" {
			continue
		}

		name := p.SummonerName
		if name == "" && p.RiotIDGameName != "" {
			name = p.RiotIDGameName
		}
		if name == "" && p.Esports != nil {
			name = p.Esports.PlayerName
		}

		s := d.GetOrCreateSummoner(p.PUUID, name)
		p.Summoner = s
		s.AddMatch(m)
	}
}
