package data

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

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

// SaveToJSON serializes the dataset's matches and summoners and writes them to a JSON file.
func (d *Dataset) SaveToJSON(filePath string) error {
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create JSON file %q: %w", filePath, err)
	}
	defer file.Close()

	w := bufio.NewWriter(file)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(d); err != nil {
		return fmt.Errorf("failed to encode dataset to JSON %q: %w", filePath, err)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("failed to flush data to JSON file %q: %w", filePath, err)
	}
	return nil
}

// LoadFromJSON deserializes matches and summoners from a JSON file into the dataset,
// restoring indexing and bidirectional references.
func (d *Dataset) LoadFromJSON(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open JSON file %q: %w", filePath, err)
	}
	defer file.Close()

	r := bufio.NewReader(file)
	var loaded Dataset
	dec := json.NewDecoder(r)
	if err := dec.Decode(&loaded); err != nil {
		return fmt.Errorf("failed to decode dataset from JSON %q: %w", filePath, err)
	}

	if d.PUUIDToSummoner == nil {
		d.PUUIDToSummoner = make(map[string]*SummonerV4)
	}
	if d.MatchIDToMatch == nil {
		d.MatchIDToMatch = make(map[string]*MatchV5)
	}

	// Register all summoners first
	for _, s := range loaded.Summoners {
		if s == nil || s.PUUID == "" {
			continue
		}
		if s.Matches == nil {
			s.Matches = make([]*MatchV5, 0)
		}
		if existing, exists := d.PUUIDToSummoner[s.PUUID]; exists {
			if existing.Name == "" && s.Name != "" {
				existing.Name = s.Name
			}
			if existing.AccountID == "" && s.AccountID != "" {
				existing.AccountID = s.AccountID
			}
			if existing.ID == "" && s.ID != "" {
				existing.ID = s.ID
			}
			if existing.SummonerLevel == 0 && s.SummonerLevel != 0 {
				existing.SummonerLevel = s.SummonerLevel
			}
			if existing.ProfileIconID == 0 && s.ProfileIconID != 0 {
				existing.ProfileIconID = s.ProfileIconID
			}
			if existing.RevisionDate == 0 && s.RevisionDate != 0 {
				existing.RevisionDate = s.RevisionDate
			}
			if !existing.Crawled && s.Crawled {
				existing.Crawled = s.Crawled
			}
		} else {
			d.Summoners = append(d.Summoners, s)
			d.PUUIDToSummoner[s.PUUID] = s
		}
	}

	// Add matches and establish links
	for _, m := range loaded.Matches {
		if m == nil {
			continue
		}
		d.AddMatch(m)
	}

	return nil
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
