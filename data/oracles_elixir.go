package data

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// dateFormats lists the possible date/time layouts found in Oracle's Elixir datasets.
var dateFormats = []string{
	"2006-01-02 15:04:05",
	time.RFC3339,
	"2006-01-02T15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
	"01/02/2006 15:04:05",
	"01/02/2006",
}

// parseDate tries parsing a date string across supported layouts.
func parseDate(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range dateFormats {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

func parseInt(s string) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	// Support values like "1.0" or "0"
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int(f)
	}
	return 0
}

func parseInt64(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f)
	}
	return 0
}

func parseFloat(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0.0
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return 0.0
}

func parseBool(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "1", "true", "t", "yes", "y":
		return true
	default:
		return false
	}
}

// LoadOraclesElixir reads a CSV file from Oracle's Elixir and adds all matches and summoners to the Dataset.
func (d *Dataset) LoadOraclesElixir(csvFilePath string) error {
	file, err := os.Open(csvFilePath)
	if err != nil {
		return fmt.Errorf("failed to open CSV file %q: %w", csvFilePath, err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	// Some CSV files may have irregular column counts or quotes
	reader.ReuseRecord = true
	reader.FieldsPerRecord = -1

	// Read header
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("failed to read CSV header from %q: %w", csvFilePath, err)
	}

	colIdx := make(map[string]int, len(header))
	for i, col := range header {
		colIdx[strings.TrimSpace(col)] = i
	}

	get := func(record []string, col string) string {
		idx, ok := colIdx[col]
		if !ok || idx >= len(record) {
			return ""
		}
		return record[idx]
	}

	// Group rows by game ID
	// Because games in OE CSVs are contiguous chunks, we process game by game.
	var currentMatch *MatchV5
	var currentGameID string

	flushCurrentMatch := func() {
		if currentMatch != nil {
			d.AddMatch(currentMatch)
			currentMatch = nil
		}
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading CSV row: %w", err)
		}

		gameID := get(record, "gameid")
		if gameID == "" {
			continue
		}

		if gameID != currentGameID {
			flushCurrentMatch()
			currentGameID = gameID

			// Check if match already exists in dataset
			if existing := d.GetMatch(gameID); existing != nil {
				currentMatch = existing
			} else {
				currentMatch = initMatchFromOERow(record, get)
			}
		}

		// Process row (team or player)
		positionStr := get(record, "position")
		pos := ParsePosition(positionStr)
		participantID := parseInt(get(record, "participantid"))

		if pos == PositionTeam || participantID == 100 || participantID == 200 {
			// Team row
			populateTeamFromOERow(currentMatch, record, get)
		} else {
			// Player participant row
			populateParticipantFromOERow(currentMatch, record, get)
		}
	}

	flushCurrentMatch()
	return nil
}

// initMatchFromOERow creates a new MatchV5 with basic metadata populated from an OE CSV row.
func initMatchFromOERow(record []string, get func([]string, string) string) *MatchV5 {
	gameID := get(record, "gameid")
	gameLength := parseInt64(get(record, "gamelength"))
	dateStr := get(record, "date")
	matchDate := parseDate(dateStr)
	var timestampMs int64
	if !matchDate.IsZero() {
		timestampMs = matchDate.UnixMilli()
	}

	m := &MatchV5{
		Metadata: MetadataDto{
			DataVersion:  "2",
			MatchID:      gameID,
			Participants: make([]string, 0, 10),
		},
		Info: InfoDto{
			GameCreation:       timestampMs,
			GameStartTimestamp: timestampMs,
			GameDuration:       gameLength,
			GameMode:           "CLASSIC",
			GameType:           "MATCHED_GAME",
			GameVersion:        get(record, "patch"),
			MapID:              11,
			Participants:       make([]*ParticipantDto, 0, 10),
			Teams:              make([]*TeamDto, 0, 2),
		},
		Esports: &EsportsMatchData{
			League:           get(record, "league"),
			Year:             parseInt(get(record, "year")),
			Split:            get(record, "split"),
			Playoffs:         parseBool(get(record, "playoffs")),
			Date:             matchDate,
			GameNumber:       parseInt(get(record, "game")),
			Patch:            get(record, "patch"),
			DataCompleteness: ParseDataCompleteness(get(record, "datacompleteness")),
			URL:              get(record, "url"),
			TeamKPM:          parseFloat(get(record, "team kpm")),
			CKPM:             parseFloat(get(record, "ckpm")),
		},
	}

	// Pre-create Blue (100) and Red (200) teams
	m.Info.Teams = append(m.Info.Teams, &TeamDto{
		TeamID: 100,
		Bans:   make([]BanDto, 0, 5),
		Esports: &EsportsTeamData{
			Side:          SideBlue,
			Bans:          make([]string, 0, 5),
			Picks:         make([]string, 0, 5),
			IntervalStats: make(map[int]*IntervalStats),
		},
	})
	m.Info.Teams = append(m.Info.Teams, &TeamDto{
		TeamID: 200,
		Bans:   make([]BanDto, 0, 5),
		Esports: &EsportsTeamData{
			Side:          SideRed,
			Bans:          make([]string, 0, 5),
			Picks:         make([]string, 0, 5),
			IntervalStats: make(map[int]*IntervalStats),
		},
	})

	return m
}

// populateTeamFromOERow populates team statistics and objectives from a team CSV row.
func populateTeamFromOERow(m *MatchV5, record []string, get func([]string, string) string) {
	side := ParseSide(get(record, "side"))
	team := m.GetTeamBySide(side)
	if team == nil {
		return
	}

	team.Win = parseBool(get(record, "result"))

	// Bans
	bans := make([]string, 0, 5)
	for i := 1; i <= 5; i++ {
		ban := get(record, fmt.Sprintf("ban%d", i))
		if ban != "" {
			bans = append(bans, ban)
			team.Bans = append(team.Bans, BanDto{
				PickTurn:     i,
				ChampionName: ban,
			})
		}
	}

	// Picks
	picks := make([]string, 0, 5)
	for i := 1; i <= 5; i++ {
		pick := get(record, fmt.Sprintf("pick%d", i))
		if pick != "" {
			picks = append(picks, pick)
		}
	}

	// Objectives
	team.Objectives = ObjectivesDto{
		Baron: ObjectiveDto{
			First: parseBool(get(record, "firstbaron")),
			Kills: parseInt(get(record, "barons")),
		},
		Champion: ObjectiveDto{
			First: parseBool(get(record, "firstblood")),
			Kills: parseInt(get(record, "teamkills")),
		},
		Dragon: ObjectiveDto{
			First: parseBool(get(record, "firstdragon")),
			Kills: parseInt(get(record, "dragons")),
		},
		Horde: ObjectiveDto{
			First: false,
			Kills: parseInt(get(record, "void_grubs")),
		},
		Inhibitor: ObjectiveDto{
			First: false,
			Kills: parseInt(get(record, "inhibitors")),
		},
		RiftHerald: ObjectiveDto{
			First: parseBool(get(record, "firstherald")),
			Kills: parseInt(get(record, "heralds")),
		},
		Tower: ObjectiveDto{
			First: parseBool(get(record, "firsttower")),
			Kills: parseInt(get(record, "towers")),
		},
		Atakhan: ObjectiveDto{
			First: false,
			Kills: parseInt(get(record, "atakhans")),
		},
	}

	// Esports Team Data
	et := team.Esports
	if et == nil {
		et = &EsportsTeamData{
			Side:          side,
			IntervalStats: make(map[int]*IntervalStats),
		}
		team.Esports = et
	}

	et.TeamName = get(record, "teamname")
	et.TeamID = get(record, "teamid")
	et.FirstPick = parseBool(get(record, "firstPick"))
	et.Bans = bans
	et.Picks = picks
	et.TeamKills = parseInt(get(record, "teamkills"))
	et.TeamDeaths = parseInt(get(record, "teamdeaths"))
	et.TeamKPM = parseFloat(get(record, "team kpm"))
	et.CKPM = parseFloat(get(record, "ckpm"))
	et.FirstDragon = parseBool(get(record, "firstdragon"))
	et.Dragons = parseInt(get(record, "dragons"))
	et.OppDragons = parseInt(get(record, "opp_dragons"))
	et.ElementalDrakes = parseInt(get(record, "elementaldrakes"))
	et.OppElementalDrakes = parseInt(get(record, "opp_elementaldrakes"))
	et.Infernals = parseInt(get(record, "infernals"))
	et.Mountains = parseInt(get(record, "mountains"))
	et.Clouds = parseInt(get(record, "clouds"))
	et.Oceans = parseInt(get(record, "oceans"))
	et.Chemtechs = parseInt(get(record, "chemtechs"))
	et.Hextechs = parseInt(get(record, "hextechs"))
	et.DragonsTypeUnknown = parseInt(get(record, "dragons (type unknown)"))
	et.Elders = parseInt(get(record, "elders"))
	et.OppElders = parseInt(get(record, "opp_elders"))
	et.FirstHerald = parseBool(get(record, "firstherald"))
	et.Heralds = parseInt(get(record, "heralds"))
	et.OppHeralds = parseInt(get(record, "opp_heralds"))
	et.VoidGrubs = parseInt(get(record, "void_grubs"))
	et.OppVoidGrubs = parseInt(get(record, "opp_void_grubs"))
	et.FirstBaron = parseBool(get(record, "firstbaron"))
	et.Barons = parseInt(get(record, "barons"))
	et.OppBarons = parseInt(get(record, "opp_barons"))
	et.Atakhans = parseInt(get(record, "atakhans"))
	et.OppAtakhans = parseInt(get(record, "opp_atakhans"))
	et.FirstTower = parseBool(get(record, "firsttower"))
	et.Towers = parseInt(get(record, "towers"))
	et.OppTowers = parseInt(get(record, "opp_towers"))
	et.FirstMidTower = parseBool(get(record, "firstmidtower"))
	et.FirstToThreeTowers = parseBool(get(record, "firsttothreetowers"))
	et.TurretPlates = parseInt(get(record, "turretplates"))
	et.OppTurretPlates = parseInt(get(record, "opp_turretplates"))
	et.Inhibitors = parseInt(get(record, "inhibitors"))
	et.OppInhibitors = parseInt(get(record, "opp_inhibitors"))
	et.DamageToChampions = parseInt(get(record, "damagetochampions"))
	et.DPM = parseFloat(get(record, "dpm"))
	et.DamageShare = parseFloat(get(record, "damageshare"))
	et.DamageTakenPerMinute = parseFloat(get(record, "damagetakenperminute"))
	et.DamageMitigatedPerMinute = parseFloat(get(record, "damagemitigatedperminute"))
	et.DamageToTowers = parseFloat(get(record, "damagetotowers"))
	et.WardsPlaced = parseInt(get(record, "wardsplaced"))
	et.WPM = parseFloat(get(record, "wpm"))
	et.WardsKilled = parseInt(get(record, "wardskilled"))
	et.WCPM = parseFloat(get(record, "wcpm"))
	et.ControlWardsBought = parseInt(get(record, "controlwardsbought"))
	et.VisionScore = parseFloat(get(record, "visionscore"))
	et.VSPM = parseFloat(get(record, "vspm"))
	et.TotalGold = parseInt(get(record, "totalgold"))
	et.EarnedGold = parseInt(get(record, "earnedgold"))
	et.EarnedGPM = parseFloat(get(record, "earned gpm"))
	et.EarnedGoldShare = parseFloat(get(record, "earnedgoldshare"))
	et.GoldSpent = parseInt(get(record, "goldspent"))
	et.GSPD = parseFloat(get(record, "gspd"))
	et.GPR = parseFloat(get(record, "gpr"))
	et.TotalCS = parseInt(get(record, "total cs"))
	et.MinionKills = parseInt(get(record, "minionkills"))
	et.MonsterKills = parseInt(get(record, "monsterkills"))
	et.MonsterKillsOwnJungle = parseInt(get(record, "monsterkillsownjungle"))
	et.MonsterKillsEnemyJungle = parseInt(get(record, "monsterkillsenemyjungle"))
	et.CSPM = parseFloat(get(record, "cspm"))

	// Populate interval stats at 10, 15, 20, 25
	for _, minute := range []int{10, 15, 20, 25} {
		minStr := strconv.Itoa(minute)
		goldAt := get(record, "goldat"+minStr)
		if goldAt != "" {
			et.IntervalStats[minute] = parseIntervalStats(record, get, minStr, minute)
		}
	}
}

// populateParticipantFromOERow populates an individual player's statistics from a player CSV row.
func populateParticipantFromOERow(m *MatchV5, record []string, get func([]string, string) string) {
	participantID := parseInt(get(record, "participantid"))
	playerID := get(record, "playerid")
	playerName := get(record, "playername")

	// Use playerid as PUUID if available, otherwise fall back to playername
	puuid := playerID
	if puuid == "" {
		puuid = playerName
	}

	side := ParseSide(get(record, "side"))
	position := ParsePosition(get(record, "position"))

	p := &ParticipantDto{
		PUUID:                       puuid,
		SummonerName:                playerName,
		RiotIDGameName:              playerName,
		ParticipantID:               participantID,
		TeamID:                      side.TeamID(),
		ChampionName:                get(record, "champion"),
		Position:                    position,
		IndividualPosition:          get(record, "position"),
		TeamPosition:                get(record, "position"),
		Kills:                       parseInt(get(record, "kills")),
		Deaths:                      parseInt(get(record, "deaths")),
		Assists:                     parseInt(get(record, "assists")),
		DoubleKills:                 parseInt(get(record, "doublekills")),
		TripleKills:                 parseInt(get(record, "triplekills")),
		QuadraKills:                 parseInt(get(record, "quadrakills")),
		PentaKills:                  parseInt(get(record, "pentakills")),
		FirstBloodKill:              parseBool(get(record, "firstbloodkill")),
		FirstBloodAssist:            parseBool(get(record, "firstbloodassist")),
		FirstBloodVictim:            parseBool(get(record, "firstbloodvictim")),
		FirstTowerKill:              parseBool(get(record, "firsttower")),
		Win:                         parseBool(get(record, "result")),
		TimePlayed:                  parseInt(get(record, "gamelength")),
		TotalDamageDealtToChampions: parseInt(get(record, "damagetochampions")),
		DamageDealtToTurrets:        parseInt(get(record, "damagetotowers")),
		DamageSelfMitigated:         parseInt(get(record, "damagemitigatedperminute")),
		TotalMinionsKilled:          parseInt(get(record, "minionkills")),
		NeutralMinionsKilled:        parseInt(get(record, "monsterkills")),
		GoldEarned:                  parseInt(get(record, "totalgold")),
		GoldSpent:                   parseInt(get(record, "goldspent")),
		VisionScore:                 parseInt(get(record, "visionscore")),
		WardsPlaced:                 parseInt(get(record, "wardsplaced")),
		WardsKilled:                 parseInt(get(record, "wardskilled")),
		DetectorWardsPlaced:         parseInt(get(record, "controlwardsbought")),
		VisionWardsBoughtInGame:     parseInt(get(record, "controlwardsbought")),
		TurretKills:                 parseInt(get(record, "towers")),
		InhibitorKills:              parseInt(get(record, "inhibitors")),
		Esports: &EsportsParticipantData{
			PlayerName:               playerName,
			PlayerID:                 playerID,
			TeamName:                 get(record, "teamname"),
			TeamID:                   get(record, "teamid"),
			Position:                 position,
			Side:                     side,
			FirstPick:                parseBool(get(record, "firstPick")),
			TeamKills:                parseInt(get(record, "teamkills")),
			TeamDeaths:               parseInt(get(record, "teamdeaths")),
			FirstBlood:               parseBool(get(record, "firstblood")),
			FirstBloodKill:           parseBool(get(record, "firstbloodkill")),
			FirstBloodAssist:         parseBool(get(record, "firstbloodassist")),
			FirstBloodVictim:         parseBool(get(record, "firstbloodvictim")),
			TeamKPM:                  parseFloat(get(record, "team kpm")),
			CKPM:                     parseFloat(get(record, "ckpm")),
			DamageToChampions:        parseInt(get(record, "damagetochampions")),
			DPM:                      parseFloat(get(record, "dpm")),
			DamageShare:              parseFloat(get(record, "damageshare")),
			DamageTakenPerMinute:     parseFloat(get(record, "damagetakenperminute")),
			DamageMitigatedPerMinute: parseFloat(get(record, "damagemitigatedperminute")),
			DamageToTowers:           parseFloat(get(record, "damagetotowers")),
			WardsPlaced:              parseInt(get(record, "wardsplaced")),
			WPM:                      parseFloat(get(record, "wpm")),
			WardsKilled:              parseInt(get(record, "wardskilled")),
			WCPM:                     parseFloat(get(record, "wcpm")),
			ControlWardsBought:       parseInt(get(record, "controlwardsbought")),
			VisionScore:              parseFloat(get(record, "visionscore")),
			VSPM:                     parseFloat(get(record, "vspm")),
			TotalGold:                parseInt(get(record, "totalgold")),
			EarnedGold:               parseInt(get(record, "earnedgold")),
			EarnedGPM:                parseFloat(get(record, "earned gpm")),
			EarnedGoldShare:          parseFloat(get(record, "earnedgoldshare")),
			GoldSpent:                parseInt(get(record, "goldspent")),
			GSPD:                     parseFloat(get(record, "gspd")),
			GPR:                      parseFloat(get(record, "gpr")),
			TotalCS:                  parseInt(get(record, "total cs")),
			MinionKills:              parseInt(get(record, "minionkills")),
			MonsterKills:             parseInt(get(record, "monsterkills")),
			MonsterKillsOwnJungle:    parseInt(get(record, "monsterkillsownjungle")),
			MonsterKillsEnemyJungle:  parseInt(get(record, "monsterkillsenemyjungle")),
			CSPM:                     parseFloat(get(record, "cspm")),
			IntervalStats:            make(map[int]*IntervalStats),
		},
	}

	// Interval stats at 10, 15, 20, 25
	for _, minute := range []int{10, 15, 20, 25} {
		minStr := strconv.Itoa(minute)
		goldAt := get(record, "goldat"+minStr)
		if goldAt != "" {
			p.Esports.IntervalStats[minute] = parseIntervalStats(record, get, minStr, minute)
		}
	}

	m.Info.Participants = append(m.Info.Participants, p)
	if puuid != "" {
		m.Metadata.Participants = append(m.Metadata.Participants, puuid)
	}
}

func parseIntervalStats(record []string, get func([]string, string) string, minStr string, minute int) *IntervalStats {
	return &IntervalStats{
		Minute:     minute,
		Gold:       parseInt(get(record, "goldat"+minStr)),
		XP:         parseInt(get(record, "xpat"+minStr)),
		CS:         parseInt(get(record, "csat"+minStr)),
		OppGold:    parseInt(get(record, "opp_goldat"+minStr)),
		OppXP:      parseInt(get(record, "opp_xpat"+minStr)),
		OppCS:      parseInt(get(record, "opp_csat"+minStr)),
		GoldDiff:   parseInt(get(record, "golddiffat"+minStr)),
		XPDiff:     parseInt(get(record, "xpdiffat"+minStr)),
		CSDiff:     parseInt(get(record, "csdiffat"+minStr)),
		Kills:      parseInt(get(record, "killsat"+minStr)),
		Assists:    parseInt(get(record, "assistsat"+minStr)),
		Deaths:     parseInt(get(record, "deathsat"+minStr)),
		OppKills:   parseInt(get(record, "opp_killsat"+minStr)),
		OppAssists: parseInt(get(record, "opp_assistsat"+minStr)),
		OppDeaths:  parseInt(get(record, "opp_deathsat"+minStr)),
	}
}
