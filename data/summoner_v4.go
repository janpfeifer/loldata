package data

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"sort"
	"strings"
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

	// RankTier is the player's competitive Solo/Duo rank tier and division.
	RankTier RankTier `json:"rankTier,omitempty"`

	// LeaguePoints is the player's current LP in Solo/Duo queue.
	LeaguePoints int `json:"leaguePoints,omitempty"`

	// RankWins is the player's wins in Solo/Duo queue.
	RankWins int `json:"rankWins,omitempty"`

	// RankLosses is the player's losses in Solo/Duo queue.
	RankLosses int `json:"rankLosses,omitempty"`

	// RankFetched indicates whether ranked league entries have been fetched for this summoner.
	RankFetched bool `json:"rankFetched,omitempty"`

	// ProfileUnavailable indicates that this summoner's profile cannot be fetched because the account
	// does not exist or was deleted (HTTP 404).
	ProfileUnavailable bool `json:"profileUnavailable,omitempty"`

	// PUUIDInvalid indicates that this PUUID was encrypted for a different API key/project (HTTP 400).
	// The account exists, but cannot be decrypted with the current API key.
	PUUIDInvalid bool `json:"puuidInvalid,omitempty"`

	// Matches contains references to all matches this summoner participated in,
	// maintained in chronological order by match start/creation time.
	Matches []*MatchV5 `json:"-"`
}

// HasProfile returns true if the summoner has complete Summoner-V4 profile information and rank information.
func (s *SummonerV4) HasProfile() bool {
	if s == nil {
		return false
	}
	return (s.SummonerLevel > 0 || s.RevisionDate > 0) && s.RankFetched
}

// NeedsProfile returns true if the summoner is missing profile information
// and can be fetched (i.e. not marked ProfileUnavailable or PUUIDInvalid).
func (s *SummonerV4) NeedsProfile() bool {
	if s == nil {
		return false
	}
	if s.ProfileUnavailable || s.PUUIDInvalid {
		return false
	}
	return !s.HasProfile()
}

// HasRank returns true if rank information has been fetched for this summoner.
func (s *SummonerV4) HasRank() bool {
	if s == nil {
		return false
	}
	return s.RankFetched
}

// RiotID returns the gameName and tagLine for this summoner, parsing s.Name or checking their matches.
func (s *SummonerV4) RiotID(defaultTag string) (string, string) {
	if s == nil {
		return "", ""
	}
	if strings.Contains(s.Name, "#") {
		parts := strings.SplitN(s.Name, "#", 2)
		return parts[0], parts[1]
	}
	// Check match participants for riotIdGameName and riotIdTagline
	for _, m := range s.Matches {
		if m != nil && m.Info.Participants != nil {
			for _, p := range m.Info.Participants {
				if p != nil && p.PUUID == s.PUUID && p.RiotIDGameName != "" {
					tag := p.RiotIDTagline
					if tag == "" {
						tag = defaultTag
					}
					return p.RiotIDGameName, tag
				}
			}
		}
	}
	if s.Name != "" {
		return s.Name, defaultTag
	}
	return "", ""
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

// summonerV4Gob is the internal struct for Gob serialization of SummonerV4.
type summonerV4Gob struct {
	AccountID          string
	ProfileIconID      int
	RevisionDate       int64
	ID                 string
	PUUID              string
	SummonerLevel      int64
	Name               string
	Crawled            bool
	RankTier           RankTier
	LeaguePoints       int
	RankWins           int
	RankLosses         int
	RankFetched        bool
	ProfileUnavailable bool
	PUUIDInvalid       bool
}

// GobEncode implements gob.GobEncoder to serialize SummonerV4 without cyclical match references.
func (s *SummonerV4) GobEncode() ([]byte, error) {
	var buf bytes.Buffer
	g := summonerV4Gob{
		AccountID:          s.AccountID,
		ProfileIconID:      s.ProfileIconID,
		RevisionDate:       s.RevisionDate,
		ID:                 s.ID,
		PUUID:              s.PUUID,
		SummonerLevel:      s.SummonerLevel,
		Name:               s.Name,
		Crawled:            s.Crawled,
		RankTier:           s.RankTier,
		LeaguePoints:       s.LeaguePoints,
		RankWins:           s.RankWins,
		RankLosses:         s.RankLosses,
		RankFetched:        s.RankFetched,
		ProfileUnavailable: s.ProfileUnavailable,
		PUUIDInvalid:       s.PUUIDInvalid,
	}
	encoder := gob.NewEncoder(&buf)
	if err := encoder.Encode(g); err != nil {
		return nil, fmt.Errorf("failed to encode SummonerV4 to Gob: %w", err)
	}
	return buf.Bytes(), nil
}

// GobDecode implements gob.GobDecoder to deserialize SummonerV4 without cyclical match references.
func (s *SummonerV4) GobDecode(data []byte) error {
	buf := bytes.NewReader(data)
	decoder := gob.NewDecoder(buf)
	var g summonerV4Gob
	if err := decoder.Decode(&g); err != nil {
		return fmt.Errorf("failed to decode SummonerV4 from Gob: %w", err)
	}

	s.AccountID = g.AccountID
	s.ProfileIconID = g.ProfileIconID
	s.RevisionDate = g.RevisionDate
	s.ID = g.ID
	s.PUUID = g.PUUID
	s.SummonerLevel = g.SummonerLevel
	s.Name = g.Name
	s.Crawled = g.Crawled
	s.RankTier = g.RankTier
	s.LeaguePoints = g.LeaguePoints
	s.RankWins = g.RankWins
	s.RankLosses = g.RankLosses
	s.RankFetched = g.RankFetched
	s.ProfileUnavailable = g.ProfileUnavailable
	s.PUUIDInvalid = g.PUUIDInvalid
	if s.Matches == nil {
		s.Matches = make([]*MatchV5, 0)
	}
	return nil
}
