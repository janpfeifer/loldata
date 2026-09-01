package data

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// FindSummoner searches for a summoner in the dataset matching the query string.
// It searches PUUIDs, exact names, Riot ID names (before '#'), prefix matches, and substrings.
// Returns:
// - (summoner, nil) if exactly one clear match is found.
// - (nil, candidates) if multiple summoners match the same priority tier.
// - (nil, nil) if no matching summoner is found.
func (d *Dataset) FindSummoner(query string) (*SummonerV4, []*SummonerV4) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	// 1. Direct PUUID lookup
	if s, exists := d.PUUIDToSummoner[query]; exists {
		return s, nil
	}

	qLower := strings.ToLower(query)

	var tier1PUUIDExact []*SummonerV4
	var tier2NameExact []*SummonerV4
	var tier3GameNameExact []*SummonerV4
	var tier4NamePrefix []*SummonerV4
	var tier5NameContains []*SummonerV4

	for _, s := range d.Summoners {
		if s == nil {
			continue
		}

		sPUUIDLower := strings.ToLower(s.PUUID)
		sName := strings.TrimSpace(s.Name)
		sNameLower := strings.ToLower(sName)

		// Tier 1: Case-insensitive PUUID exact match
		if sPUUIDLower == qLower {
			tier1PUUIDExact = append(tier1PUUIDExact, s)
			continue
		}

		// Tier 2: Case-insensitive Full Name exact match
		if sNameLower == qLower {
			tier2NameExact = append(tier2NameExact, s)
			continue
		}

		// Tier 3: Case-insensitive GameName exact match (before '#')
		if strings.Contains(sName, "#") {
			parts := strings.Split(sName, "#")
			if strings.EqualFold(strings.TrimSpace(parts[0]), query) {
				tier3GameNameExact = append(tier3GameNameExact, s)
				continue
			}
		}

		// Tier 4: Prefix match on name or gameName
		if strings.HasPrefix(sNameLower, qLower) {
			tier4NamePrefix = append(tier4NamePrefix, s)
			continue
		}

		// Tier 5: Substring match on name or PUUID
		if strings.Contains(sNameLower, qLower) || (len(query) >= 4 && strings.Contains(sPUUIDLower, qLower)) {
			tier5NameContains = append(tier5NameContains, s)
			continue
		}
	}

	if len(tier1PUUIDExact) == 1 {
		return tier1PUUIDExact[0], nil
	} else if len(tier1PUUIDExact) > 1 {
		return nil, tier1PUUIDExact
	}

	if len(tier2NameExact) == 1 {
		return tier2NameExact[0], nil
	} else if len(tier2NameExact) > 1 {
		return nil, tier2NameExact
	}

	if len(tier3GameNameExact) == 1 {
		return tier3GameNameExact[0], nil
	} else if len(tier3GameNameExact) > 1 {
		return nil, tier3GameNameExact
	}

	if len(tier4NamePrefix) == 1 {
		return tier4NamePrefix[0], nil
	} else if len(tier4NamePrefix) > 1 {
		return nil, tier4NamePrefix
	}

	if len(tier5NameContains) == 1 {
		return tier5NameContains[0], nil
	} else if len(tier5NameContains) > 1 {
		return nil, tier5NameContains
	}

	return nil, nil
}

// SummonerStats holds aggregated performance statistics and distribution metrics for a summoner.
type SummonerStats struct {
	Summoner *SummonerV4

	// Overall Match Record
	TotalMatches int
	Wins         int
	Losses       int
	WinRate      float64 // Percentage (0.0 - 100.0)

	// Side Win Rates
	BlueGames   int
	BlueWins    int
	BlueWinRate float64
	RedGames    int
	RedWins     int
	RedWinRate  float64

	// Dates & Durations
	EarliestMatch time.Time
	LatestMatch   time.Time
	TotalDuration time.Duration
	AvgDuration   time.Duration

	// Combat & KDA
	TotalKills           int
	TotalDeaths          int
	TotalAssists         int
	AvgKills             float64
	AvgDeaths            float64
	AvgAssists           float64
	KDARatio             float64
	PerfectKDA           bool
	AvgKillParticipation float64

	// Multikills
	DoubleKills int
	TripleKills int
	QuadraKills int
	PentaKills  int

	// First Objectives
	FirstBloodKills   int
	FirstBloodAssists int
	FirstBloodVictims int
	FirstTowerKills   int
	FirstTowerAssists int

	// Economy & Farming
	TotalCS int
	AvgCS   float64
	AvgCSPM float64

	TotalGold int
	AvgGold   float64
	AvgGPM    float64

	// Damage
	TotalDamageToChampions int
	AvgDamageToChampions   float64
	AvgDPM                 float64
	AvgDamageShare         float64
	TotalDamageTaken       int
	AvgDamageTaken         float64
	TotalDamageMitigated   int
	AvgDamageMitigated     float64
	TotalDamageToTurrets   int
	AvgDamageToTurrets     float64

	// Vision
	TotalVisionScore  int
	AvgVisionScore    float64
	AvgVSPM           float64
	TotalWardsPlaced  int
	AvgWardsPlaced    float64
	TotalWardsKilled  int
	AvgWardsKilled    float64
	TotalControlWards int
	AvgControlWards   float64

	// Distributions
	Champions []*ChampionStat
	Positions []*PositionStat
	Teammates []*PlayerRelationStat
	Opponents []*PlayerRelationStat
	Queues    []*QueueStat
	Recent    []*RecentMatchSummary
}

// ChampionStat holds aggregated performance metrics for a specific champion played by a summoner.
type ChampionStat struct {
	ChampionName   string
	ChampionID     int
	Games          int
	PercentOfTotal float64
	Wins           int
	Losses         int
	WinRate        float64
	Kills          int
	Deaths         int
	Assists        int
	AvgKills       float64
	AvgDeaths      float64
	AvgAssists     float64
	KDARatio       float64
	TotalCS        int
	AvgCSPM        float64
	TotalDamage    int
	AvgDPM         float64
	TotalVision    int
	AvgVision      float64
	DoubleKills    int
	TripleKills    int
	QuadraKills    int
	PentaKills     int

	totalDurationMinutes float64
}

// PositionStat holds aggregated performance metrics for a lane/position.
type PositionStat struct {
	Position       Position
	PositionName   string
	Games          int
	PercentOfTotal float64
	Wins           int
	Losses         int
	WinRate        float64
	Kills          int
	Deaths         int
	Assists        int
	AvgKills       float64
	AvgDeaths      float64
	AvgAssists     float64
	KDARatio       float64
	TotalCS        int
	AvgCSPM        float64
	TotalDamage    int
	AvgDPM         float64

	totalDurationMinutes float64
}

// PlayerRelationStat holds statistics for other players played with (as teammates or opponents).
type PlayerRelationStat struct {
	PUUID          string
	Name           string
	Games          int
	PercentOfTotal float64
	Wins           int
	Losses         int
	WinRate        float64
}

// QueueStat holds statistics per game queue/mode.
type QueueStat struct {
	QueueID        int
	Description    string
	Games          int
	PercentOfTotal float64
	Wins           int
	Losses         int
	WinRate        float64
}

// RecentMatchSummary holds a summary of a single recent match.
type RecentMatchSummary struct {
	MatchID      string
	Time         time.Time
	ChampionName string
	Position     Position
	Win          bool
	Kills        int
	Deaths       int
	Assists      int
	Duration     time.Duration
	CS           int
	Damage       int
}

// ComputeStats calculates comprehensive performance statistics for the summoner across all their matches.
func (s *SummonerV4) ComputeStats() *SummonerStats {
	stats := &SummonerStats{
		Summoner: s,
	}

	if s == nil || len(s.Matches) == 0 {
		return stats
	}

	champMap := make(map[string]*ChampionStat)
	posMap := make(map[Position]*PositionStat)
	tmMap := make(map[string]*PlayerRelationStat)
	opMap := make(map[string]*PlayerRelationStat)
	queueMap := make(map[string]*QueueStat)

	var totalKP float64
	var kpMatches int
	var matchesWithDuration int
	var totalDurationSeconds float64
	var totalDmgShare float64
	var dmgShareCount int

	for _, m := range s.Matches {
		if m == nil {
			continue
		}

		// Find the participant representing this summoner in match m
		var p *ParticipantDto
		for _, part := range m.Info.Participants {
			if part == nil {
				continue
			}
			if part.Summoner == s || (part.PUUID != "" && part.PUUID == s.PUUID) {
				p = part
				break
			}
		}
		if p == nil {
			// Fallback by name matching
			for _, part := range m.Info.Participants {
				if part == nil {
					continue
				}
				if part.SummonerName != "" && strings.EqualFold(part.SummonerName, s.Name) {
					p = part
					break
				}
				if part.RiotIDGameName != "" && strings.EqualFold(part.RiotIDGameName, s.Name) {
					p = part
					break
				}
				if part.Esports != nil && part.Esports.PlayerName != "" && strings.EqualFold(part.Esports.PlayerName, s.Name) {
					p = part
					break
				}
			}
		}
		if p == nil {
			continue
		}

		stats.TotalMatches++

		// Match time & duration
		mTime := m.Time()
		if !mTime.IsZero() {
			if stats.EarliestMatch.IsZero() || mTime.Before(stats.EarliestMatch) {
				stats.EarliestMatch = mTime
			}
			if stats.LatestMatch.IsZero() || mTime.After(stats.LatestMatch) {
				stats.LatestMatch = mTime
			}
		}

		durSec := m.Info.GameDuration
		if durSec <= 0 && p.TimePlayed > 0 {
			durSec = int64(p.TimePlayed)
		}
		if durSec > 0 {
			stats.TotalDuration += time.Duration(durSec) * time.Second
			matchesWithDuration++
			totalDurationSeconds += float64(durSec)
		}
		durMinutes := float64(durSec) / 60.0
		if durMinutes <= 0 {
			durMinutes = 1.0
		}

		// Win / Loss
		win := p.Win
		if !win {
			if team := m.GetTeam(p.TeamID); team != nil && team.Win {
				win = true
			}
		}
		if win {
			stats.Wins++
		} else {
			stats.Losses++
		}

		// Side win rates
		side := SideFromTeamID(p.TeamID)
		if side == SideUnknown && p.Esports != nil {
			side = p.Esports.Side
		}
		if side == SideBlue {
			stats.BlueGames++
			if win {
				stats.BlueWins++
			}
		} else if side == SideRed {
			stats.RedGames++
			if win {
				stats.RedWins++
			}
		}

		// Combat stats
		stats.TotalKills += p.Kills
		stats.TotalDeaths += p.Deaths
		stats.TotalAssists += p.Assists

		stats.DoubleKills += p.DoubleKills
		stats.TripleKills += p.TripleKills
		stats.QuadraKills += p.QuadraKills
		stats.PentaKills += p.PentaKills

		if p.FirstBloodKill || (p.Esports != nil && p.Esports.FirstBloodKill) {
			stats.FirstBloodKills++
		}
		if p.FirstBloodAssist || (p.Esports != nil && p.Esports.FirstBloodAssist) {
			stats.FirstBloodAssists++
		}
		if p.FirstBloodVictim || (p.Esports != nil && p.Esports.FirstBloodVictim) {
			stats.FirstBloodVictims++
		}

		if p.FirstTowerKill {
			stats.FirstTowerKills++
		}
		if p.FirstTowerAssist {
			stats.FirstTowerAssists++
		}

		// Farming, economy, damage, vision
		cs := p.TotalMinionsKilled + p.NeutralMinionsKilled
		if cs == 0 && p.Esports != nil {
			cs = p.Esports.TotalCS
		}
		stats.TotalCS += cs

		gold := p.GoldEarned
		if gold == 0 && p.Esports != nil {
			gold = p.Esports.TotalGold
		}
		stats.TotalGold += gold

		dmg := p.TotalDamageDealtToChampions
		if dmg == 0 && p.Esports != nil {
			dmg = p.Esports.DamageToChampions
		}
		stats.TotalDamageToChampions += dmg

		dmgTaken := p.TotalDamageTaken
		stats.TotalDamageTaken += dmgTaken

		dmgMitigated := p.DamageSelfMitigated
		stats.TotalDamageMitigated += dmgMitigated

		dmgTurrets := p.DamageDealtToTurrets
		if dmgTurrets == 0 {
			dmgTurrets = p.DamageDealtToBuildings
		}
		if dmgTurrets == 0 && p.Esports != nil {
			dmgTurrets = int(p.Esports.DamageToTowers)
		}
		stats.TotalDamageToTurrets += dmgTurrets

		vision := p.VisionScore
		if vision == 0 && p.Esports != nil {
			vision = int(p.Esports.VisionScore)
		}
		stats.TotalVisionScore += vision

		wardsPlaced := p.WardsPlaced
		if wardsPlaced == 0 && p.Esports != nil {
			wardsPlaced = p.Esports.WardsPlaced
		}
		stats.TotalWardsPlaced += wardsPlaced

		wardsKilled := p.WardsKilled
		if wardsKilled == 0 && p.Esports != nil {
			wardsKilled = p.Esports.WardsKilled
		}
		stats.TotalWardsKilled += wardsKilled

		ctrlWards := p.DetectorWardsPlaced
		if ctrlWards == 0 {
			ctrlWards = p.VisionWardsBoughtInGame
		}
		if ctrlWards == 0 && p.Esports != nil {
			ctrlWards = p.Esports.ControlWardsBought
		}
		stats.TotalControlWards += ctrlWards

		if p.Esports != nil && p.Esports.DamageShare > 0 {
			totalDmgShare += p.Esports.DamageShare
			dmgShareCount++
		}

		// Kill participation
		teamKills := 0
		if team := m.GetTeam(p.TeamID); team != nil {
			teamKills = team.Objectives.Champion.Kills
			if teamKills == 0 && team.Esports != nil {
				teamKills = team.Esports.TeamKills
			}
		}
		if teamKills == 0 {
			for _, other := range m.Info.Participants {
				if other != nil && other.TeamID == p.TeamID {
					teamKills += other.Kills
				}
			}
		}
		if teamKills > 0 {
			kp := float64(p.Kills+p.Assists) / float64(teamKills) * 100.0
			totalKP += kp
			kpMatches++
		}

		// Champion aggregation
		champName := p.ChampionName
		if champName == "" && p.ChampionID > 0 {
			champName = fmt.Sprintf("ID:%d", p.ChampionID)
		}
		if champName == "" {
			champName = "Unknown"
		}
		cEntry, exists := champMap[champName]
		if !exists {
			cEntry = &ChampionStat{
				ChampionName: champName,
				ChampionID:   p.ChampionID,
			}
			champMap[champName] = cEntry
		}
		cEntry.Games++
		if win {
			cEntry.Wins++
		} else {
			cEntry.Losses++
		}
		cEntry.Kills += p.Kills
		cEntry.Deaths += p.Deaths
		cEntry.Assists += p.Assists
		cEntry.TotalCS += cs
		cEntry.TotalDamage += dmg
		cEntry.TotalVision += vision
		cEntry.DoubleKills += p.DoubleKills
		cEntry.TripleKills += p.TripleKills
		cEntry.QuadraKills += p.QuadraKills
		cEntry.PentaKills += p.PentaKills
		cEntry.totalDurationMinutes += durMinutes

		// Position aggregation
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
		if pos == PositionTeam {
			pos = PositionUnknown
		}

		posEntry, exists := posMap[pos]
		if !exists {
			posName := pos.String()
			if pos == PositionBot {
				posName = "Bot (ADC)"
			} else if pos == PositionUnknown {
				posName = "Unknown"
			}
			posEntry = &PositionStat{
				Position:     pos,
				PositionName: posName,
			}
			posMap[pos] = posEntry
		}
		posEntry.Games++
		if win {
			posEntry.Wins++
		} else {
			posEntry.Losses++
		}
		posEntry.Kills += p.Kills
		posEntry.Deaths += p.Deaths
		posEntry.Assists += p.Assists
		posEntry.TotalCS += cs
		posEntry.TotalDamage += dmg
		posEntry.totalDurationMinutes += durMinutes

		// Teammates & Opponents
		for _, other := range m.Info.Participants {
			if other == nil || other == p {
				continue
			}
			if other.PUUID != "" && other.PUUID == p.PUUID {
				continue
			}
			if other.Summoner != nil && other.Summoner == s {
				continue
			}

			puuid := other.PUUID
			name := other.SummonerName
			if name == "" && other.RiotIDGameName != "" {
				name = other.RiotIDGameName
				if other.RiotIDTagline != "" {
					name += "#" + other.RiotIDTagline
				}
			}
			if name == "" && other.Esports != nil {
				name = other.Esports.PlayerName
			}
			if name == "" && other.Summoner != nil && other.Summoner.Name != "" {
				name = other.Summoner.Name
			}
			if name == "" {
				name = puuid
			}
			if name == "" {
				name = "Unknown"
			}

			relKey := puuid
			if relKey == "" {
				relKey = name
			}

			if other.TeamID == p.TeamID {
				// Teammate
				tm, exists := tmMap[relKey]
				if !exists {
					tm = &PlayerRelationStat{PUUID: puuid, Name: name}
					tmMap[relKey] = tm
				}
				tm.Games++
				if win {
					tm.Wins++
				} else {
					tm.Losses++
				}
			} else {
				// Opponent
				op, exists := opMap[relKey]
				if !exists {
					op = &PlayerRelationStat{PUUID: puuid, Name: name}
					opMap[relKey] = op
				}
				op.Games++
				if win {
					op.Wins++
				} else {
					op.Losses++
				}
			}
		}

		// Queue breakdown
		qID := m.Info.QueueID
		qDesc := QueueDescription(qID)
		if qID == 0 && m.Esports != nil && m.Esports.League != "" {
			qDesc = fmt.Sprintf("Esports (%s)", m.Esports.League)
		} else if qDesc == fmt.Sprintf("Queue %d", qID) && m.Info.GameMode != "" {
			qDesc = m.Info.GameMode
		}
		qEntry, exists := queueMap[qDesc]
		if !exists {
			qEntry = &QueueStat{
				QueueID:     qID,
				Description: qDesc,
			}
			queueMap[qDesc] = qEntry
		}
		qEntry.Games++
		if win {
			qEntry.Wins++
		} else {
			qEntry.Losses++
		}

		// Recent match summary
		stats.Recent = append(stats.Recent, &RecentMatchSummary{
			MatchID:      m.Metadata.MatchID,
			Time:         mTime,
			ChampionName: champName,
			Position:     pos,
			Win:          win,
			Kills:        p.Kills,
			Deaths:       p.Deaths,
			Assists:      p.Assists,
			Duration:     time.Duration(durSec) * time.Second,
			CS:           cs,
			Damage:       dmg,
		})
	}

	if stats.TotalMatches == 0 {
		return stats
	}

	// Calculate overall averages and percentages
	stats.WinRate = float64(stats.Wins) / float64(stats.TotalMatches) * 100.0
	if stats.BlueGames > 0 {
		stats.BlueWinRate = float64(stats.BlueWins) / float64(stats.BlueGames) * 100.0
	}
	if stats.RedGames > 0 {
		stats.RedWinRate = float64(stats.RedWins) / float64(stats.RedGames) * 100.0
	}

	if matchesWithDuration > 0 {
		stats.AvgDuration = stats.TotalDuration / time.Duration(matchesWithDuration)
	}

	totalGamesF := float64(stats.TotalMatches)
	stats.AvgKills = float64(stats.TotalKills) / totalGamesF
	stats.AvgDeaths = float64(stats.TotalDeaths) / totalGamesF
	stats.AvgAssists = float64(stats.TotalAssists) / totalGamesF

	if stats.TotalDeaths == 0 {
		stats.KDARatio = float64(stats.TotalKills + stats.TotalAssists)
		stats.PerfectKDA = true
	} else {
		stats.KDARatio = float64(stats.TotalKills+stats.TotalAssists) / float64(stats.TotalDeaths)
	}

	if kpMatches > 0 {
		stats.AvgKillParticipation = totalKP / float64(kpMatches)
	}

	stats.AvgCS = float64(stats.TotalCS) / totalGamesF
	stats.AvgGold = float64(stats.TotalGold) / totalGamesF
	stats.AvgDamageToChampions = float64(stats.TotalDamageToChampions) / totalGamesF
	stats.AvgDamageTaken = float64(stats.TotalDamageTaken) / totalGamesF
	stats.AvgDamageMitigated = float64(stats.TotalDamageMitigated) / totalGamesF
	stats.AvgDamageToTurrets = float64(stats.TotalDamageToTurrets) / totalGamesF
	stats.AvgVisionScore = float64(stats.TotalVisionScore) / totalGamesF
	stats.AvgWardsPlaced = float64(stats.TotalWardsPlaced) / totalGamesF
	stats.AvgWardsKilled = float64(stats.TotalWardsKilled) / totalGamesF
	stats.AvgControlWards = float64(stats.TotalControlWards) / totalGamesF

	if totalDurationSeconds > 0 {
		totalDurationMin := totalDurationSeconds / 60.0
		stats.AvgCSPM = float64(stats.TotalCS) / totalDurationMin
		stats.AvgGPM = float64(stats.TotalGold) / totalDurationMin
		stats.AvgDPM = float64(stats.TotalDamageToChampions) / totalDurationMin
		stats.AvgVSPM = float64(stats.TotalVisionScore) / totalDurationMin
	}

	if dmgShareCount > 0 {
		stats.AvgDamageShare = totalDmgShare / float64(dmgShareCount)
	}

	// Finalize Champion stats
	for _, c := range champMap {
		c.PercentOfTotal = float64(c.Games) / totalGamesF * 100.0
		c.WinRate = float64(c.Wins) / float64(c.Games) * 100.0
		c.AvgKills = float64(c.Kills) / float64(c.Games)
		c.AvgDeaths = float64(c.Deaths) / float64(c.Games)
		c.AvgAssists = float64(c.Assists) / float64(c.Games)
		if c.Deaths == 0 {
			c.KDARatio = float64(c.Kills + c.Assists)
		} else {
			c.KDARatio = float64(c.Kills+c.Assists) / float64(c.Deaths)
		}
		if c.totalDurationMinutes > 0 {
			c.AvgCSPM = float64(c.TotalCS) / c.totalDurationMinutes
			c.AvgDPM = float64(c.TotalDamage) / c.totalDurationMinutes
		}
		c.AvgVision = float64(c.TotalVision) / float64(c.Games)
		stats.Champions = append(stats.Champions, c)
	}
	sort.Slice(stats.Champions, func(i, j int) bool {
		if stats.Champions[i].Games != stats.Champions[j].Games {
			return stats.Champions[i].Games > stats.Champions[j].Games
		}
		if stats.Champions[i].WinRate != stats.Champions[j].WinRate {
			return stats.Champions[i].WinRate > stats.Champions[j].WinRate
		}
		return stats.Champions[i].ChampionName < stats.Champions[j].ChampionName
	})

	// Finalize Position stats
	for _, p := range posMap {
		p.PercentOfTotal = float64(p.Games) / totalGamesF * 100.0
		p.WinRate = float64(p.Wins) / float64(p.Games) * 100.0
		p.AvgKills = float64(p.Kills) / float64(p.Games)
		p.AvgDeaths = float64(p.Deaths) / float64(p.Games)
		p.AvgAssists = float64(p.Assists) / float64(p.Games)
		if p.Deaths == 0 {
			p.KDARatio = float64(p.Kills + p.Assists)
		} else {
			p.KDARatio = float64(p.Kills+p.Assists) / float64(p.Deaths)
		}
		if p.totalDurationMinutes > 0 {
			p.AvgCSPM = float64(p.TotalCS) / p.totalDurationMinutes
			p.AvgDPM = float64(p.TotalDamage) / p.totalDurationMinutes
		}
		stats.Positions = append(stats.Positions, p)
	}
	sort.Slice(stats.Positions, func(i, j int) bool {
		if stats.Positions[i].Games != stats.Positions[j].Games {
			return stats.Positions[i].Games > stats.Positions[j].Games
		}
		return stats.Positions[i].WinRate > stats.Positions[j].WinRate
	})

	// Finalize Teammates
	for _, tm := range tmMap {
		tm.PercentOfTotal = float64(tm.Games) / totalGamesF * 100.0
		tm.WinRate = float64(tm.Wins) / float64(tm.Games) * 100.0
		stats.Teammates = append(stats.Teammates, tm)
	}
	sort.Slice(stats.Teammates, func(i, j int) bool {
		if stats.Teammates[i].Games != stats.Teammates[j].Games {
			return stats.Teammates[i].Games > stats.Teammates[j].Games
		}
		return stats.Teammates[i].WinRate > stats.Teammates[j].WinRate
	})

	// Finalize Opponents
	for _, op := range opMap {
		op.PercentOfTotal = float64(op.Games) / totalGamesF * 100.0
		op.WinRate = float64(op.Wins) / float64(op.Games) * 100.0
		stats.Opponents = append(stats.Opponents, op)
	}
	sort.Slice(stats.Opponents, func(i, j int) bool {
		if stats.Opponents[i].Games != stats.Opponents[j].Games {
			return stats.Opponents[i].Games > stats.Opponents[j].Games
		}
		return stats.Opponents[i].WinRate > stats.Opponents[j].WinRate
	})

	// Finalize Queues
	for _, q := range queueMap {
		q.PercentOfTotal = float64(q.Games) / totalGamesF * 100.0
		q.WinRate = float64(q.Wins) / float64(q.Games) * 100.0
		stats.Queues = append(stats.Queues, q)
	}
	sort.Slice(stats.Queues, func(i, j int) bool {
		if stats.Queues[i].Games != stats.Queues[j].Games {
			return stats.Queues[i].Games > stats.Queues[j].Games
		}
		return stats.Queues[i].WinRate > stats.Queues[j].WinRate
	})

	return stats
}

// QueueDescription returns a human-friendly string for standard League of Legends queue IDs.
func QueueDescription(queueID int) string {
	switch queueID {
	case 0:
		return "Custom"
	case 400:
		return "Normal Draft (5v5)"
	case 420:
		return "Ranked Solo/Duo"
	case 430:
		return "Normal Blind (5v5)"
	case 440:
		return "Ranked Flex"
	case 450:
		return "ARAM"
	case 700:
		return "Clash"
	case 720:
		return "ARAM Clash"
	case 830:
		return "Co-op vs AI (Intro)"
	case 840:
		return "Co-op vs AI (Beginner)"
	case 850:
		return "Co-op vs AI (Intermediate)"
	case 900:
		return "URF"
	case 1020:
		return "One For All"
	case 1300:
		return "Nexus Blitz"
	case 1400:
		return "Ultimate Spellbook"
	case 1700:
		return "Arena"
	case 1900:
		return "Pick URF"
	default:
		return fmt.Sprintf("Queue %d", queueID)
	}
}
