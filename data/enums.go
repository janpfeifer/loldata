package data

import (
	"strings"
)

// Side represents the map side (team color) in a match.
//
//go:generate go tool enumer -type=Side -json -text -yaml -values -trimprefix=Side -output=gen_side_enumer.go
type Side int

const (
	// SideUnknown indicates an unspecified or unknown side.
	SideUnknown Side = iota
	// SideBlue represents the Blue side (Team 100 in Riot API).
	SideBlue
	// SideRed represents the Red side (Team 200 in Riot API).
	SideRed
)

// TeamID returns the Riot API team ID (100 for Blue, 200 for Red, 0 for unknown).
func (s Side) TeamID() int {
	switch s {
	case SideBlue:
		return 100
	case SideRed:
		return 200
	default:
		return 0
	}
}

// SideFromTeamID converts a Riot API team ID (100/200) to a Side enum.
func SideFromTeamID(teamID int) Side {
	switch teamID {
	case 100:
		return SideBlue
	case 200:
		return SideRed
	default:
		return SideUnknown
	}
}

// ParseSide parses a string into a Side enum, case-insensitively.
func ParseSide(s string) Side {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "blue", "100":
		return SideBlue
	case "red", "200":
		return SideRed
	default:
		return SideUnknown
	}
}

// Position represents a player's lane role or position in a match.
//
//go:generate go tool enumer -type=Position -json -text -yaml -values -trimprefix=Position -output=gen_position_enumer.go
type Position int

const (
	// PositionUnknown represents an unknown or unassigned position.
	PositionUnknown Position = iota
	// PositionTop represents the Top lane position.
	PositionTop
	// PositionJungle represents the Jungle position.
	PositionJungle
	// PositionMid represents the Middle lane position.
	PositionMid
	// PositionBot represents the Bottom lane (ADC/Carry) position.
	PositionBot
	// PositionSupport represents the Support/Utility position.
	PositionSupport
	// PositionTeam represents a team-aggregated row (in Oracle's Elixir data).
	PositionTeam
)

// ParsePosition parses a position string from Riot API or Oracle's Elixir into a Position enum.
func ParsePosition(p string) Position {
	p = strings.TrimSpace(strings.ToLower(p))
	switch p {
	case "top":
		return PositionTop
	case "jng", "jungle":
		return PositionJungle
	case "mid", "middle":
		return PositionMid
	case "bot", "bottom", "carry", "adc":
		return PositionBot
	case "sup", "support", "utility":
		return PositionSupport
	case "team":
		return PositionTeam
	default:
		return PositionUnknown
	}
}

// DataCompleteness indicates whether match data has complete or partial statistical coverage.
//
//go:generate go tool enumer -type=DataCompleteness -json -text -yaml -values -trimprefix=DataCompleteness -output=gen_datacompleteness_enumer.go
type DataCompleteness int

const (
	// DataCompletenessUnknown indicates unspecified data completeness.
	DataCompletenessUnknown DataCompleteness = iota
	// DataCompletenessComplete indicates full statistical reporting is available.
	DataCompletenessComplete
	// DataCompletenessPartial indicates only partial statistical reporting is available.
	DataCompletenessPartial
)

// ParseDataCompleteness parses a completeness string.
func ParseDataCompleteness(s string) DataCompleteness {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "complete":
		return DataCompletenessComplete
	case "partial":
		return DataCompletenessPartial
	default:
		return DataCompletenessUnknown
	}
}

// DragonType represents the type of elemental or elder dragon in League of Legends.
//
//go:generate go tool enumer -type=DragonType -json -text -yaml -values -trimprefix=DragonType -output=gen_dragontype_enumer.go
type DragonType int

const (
	// DragonTypeUnknown represents an unknown or unspecified dragon type.
	DragonTypeUnknown DragonType = iota
	// DragonTypeInfernal represents the Infernal Drake.
	DragonTypeInfernal
	// DragonTypeMountain represents the Mountain Drake.
	DragonTypeMountain
	// DragonTypeOcean represents the Ocean Drake.
	DragonTypeOcean
	// DragonTypeCloud represents the Cloud Drake.
	DragonTypeCloud
	// DragonTypeHextech represents the Hextech Drake.
	DragonTypeHextech
	// DragonTypeChemtech represents the Chemtech Drake.
	DragonTypeChemtech
	// DragonTypeElder represents the Elder Dragon.
	DragonTypeElder
)

// RankTier represents a player's merged competitive rank tier and division in League of Legends.
//
//go:generate go tool enumer -type=RankTier -json -text -yaml -values -output=gen_ranktier_enumer.go
type RankTier int

const (
	// RankUnknown indicates an unspecified rank.
	RankUnknown RankTier = iota
	// RankUnranked indicates the player has no ranked games / unranked.
	RankUnranked
	Iron_4
	Iron_3
	Iron_2
	Iron_1
	Bronze_4
	Bronze_3
	Bronze_2
	Bronze_1
	Silver_4
	Silver_3
	Silver_2
	Silver_1
	Gold_4
	Gold_3
	Gold_2
	Gold_1
	Platinum_4
	Platinum_3
	Platinum_2
	Platinum_1
	Emerald_4
	Emerald_3
	Emerald_2
	Emerald_1
	Diamond_4
	Diamond_3
	Diamond_2
	Diamond_1
	Master
	GrandMaster
	Challenger
)

// ParseRankTier parses a Riot API tier string (e.g. "DIAMOND", "GOLD") and division string (e.g. "I", "II", "III", "IV")
// into a merged RankTier enum.
func ParseRankTier(tier, division string) RankTier {
	tier = strings.TrimSpace(strings.ToUpper(tier))
	division = strings.TrimSpace(strings.ToUpper(division))

	switch tier {
	case "CHALLENGER":
		return Challenger
	case "GRANDMASTER", "GRAND_MASTER":
		return GrandMaster
	case "MASTER":
		return Master
	case "DIAMOND":
		switch division {
		case "I", "1":
			return Diamond_1
		case "II", "2":
			return Diamond_2
		case "III", "3":
			return Diamond_3
		case "IV", "4":
			return Diamond_4
		default:
			return Diamond_4
		}
	case "EMERALD":
		switch division {
		case "I", "1":
			return Emerald_1
		case "II", "2":
			return Emerald_2
		case "III", "3":
			return Emerald_3
		case "IV", "4":
			return Emerald_4
		default:
			return Emerald_4
		}
	case "PLATINUM":
		switch division {
		case "I", "1":
			return Platinum_1
		case "II", "2":
			return Platinum_2
		case "III", "3":
			return Platinum_3
		case "IV", "4":
			return Platinum_4
		default:
			return Platinum_4
		}
	case "GOLD":
		switch division {
		case "I", "1":
			return Gold_1
		case "II", "2":
			return Gold_2
		case "III", "3":
			return Gold_3
		case "IV", "4":
			return Gold_4
		default:
			return Gold_4
		}
	case "SILVER":
		switch division {
		case "I", "1":
			return Silver_1
		case "II", "2":
			return Silver_2
		case "III", "3":
			return Silver_3
		case "IV", "4":
			return Silver_4
		default:
			return Silver_4
		}
	case "BRONZE":
		switch division {
		case "I", "1":
			return Bronze_1
		case "II", "2":
			return Bronze_2
		case "III", "3":
			return Bronze_3
		case "IV", "4":
			return Bronze_4
		default:
			return Bronze_4
		}
	case "IRON":
		switch division {
		case "I", "1":
			return Iron_1
		case "II", "2":
			return Iron_2
		case "III", "3":
			return Iron_3
		case "IV", "4":
			return Iron_4
		default:
			return Iron_4
		}
	case "UNRANKED":
		return RankUnranked
	default:
		return RankUnknown
	}
}

