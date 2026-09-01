package data

import (
	"sort"
)

// SummonerV4 represents a League of Legends summoner profile based on the Riot Summoner-V4 API.
// See: https://developer.riotgames.com/apis#summoner-v4/GET_getByPUUID
type SummonerV4 struct {
	// AccountID is the encrypted account ID (max length 56 characters).
	AccountID string `json:"accountId,omitempty"`

	// ProfileIconID is the ID of the summoner icon associated with the summoner.
	ProfileIconID int `json:"profileIconId,omitempty"`

	// RevisionDate is the date the summoner was last modified specified as epoch milliseconds.
	RevisionDate int64 `json:"revisionDate,omitempty"`

	// ID is the encrypted summoner ID (max length 63 characters).
	ID string `json:"id,omitempty"`

	// PUUID is the encrypted Player Universally Unique Identifier (exact length 78 characters in Riot API, or OE player ID).
	PUUID string `json:"puuid"`

	// SummonerLevel is the summoner level associated with the summoner.
	SummonerLevel int64 `json:"summonerLevel,omitempty"`

	// Name is the player / summoner name (if available).
	Name string `json:"name,omitempty"`

	// Crawled indicates whether this summoner's match history has been fully crawled.
	Crawled bool `json:"crawled,omitempty"`

	// Matches contains references to all matches this summoner participated in,
	// maintained in chronological order by match start/creation time.
	Matches []*MatchV5 `json:"-"`
}

// AddMatch adds a match to the summoner's match history in chronological order.
// If the match is already present (identified by MatchID), it is not re-added.
func (s *SummonerV4) AddMatch(m *MatchV5) {
	if m == nil {
		return
	}
	matchID := m.Metadata.MatchID
	mTime := m.Time()

	// Check if match already exists
	for _, existing := range s.Matches {
		if existing.Metadata.MatchID == matchID && matchID != "" {
			return
		}
	}

	// Insert keeping sorted order by match time
	idx := sort.Search(len(s.Matches), func(i int) bool {
		return !s.Matches[i].Time().Before(mTime)
	})

	s.Matches = append(s.Matches, nil)
	copy(s.Matches[idx+1:], s.Matches[idx:])
	s.Matches[idx] = m
}
