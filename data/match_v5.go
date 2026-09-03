package data

import (
	"time"
)

// MatchV5 represents a League of Legends match based on Riot Match-V5 API (GET /lol/match/v5/matches/{matchId})
// extended with support for esports metrics and Oracle's Elixir data.
// See: https://developer.riotgames.com/apis#match-v5/GET_getMatch
type MatchV5 struct {
	// Metadata contains match metadata such as data version, match ID, and participant PUUIDs.
	Metadata MetadataDto `json:"metadata"`

	// Info contains detailed match information including participants, teams, game mode, duration, etc.
	Info InfoDto `json:"info"`

	// Esports contains additional esports-specific metadata and Oracle's Elixir metrics (if available).
	Esports *EsportsMatchData `json:"esports,omitempty"`
}

// Time returns the start or creation time of the match.
// If Info.GameStartTimestamp or Info.GameCreation is set (in epoch ms), it converts that;
// otherwise if Esports.Date is available, it returns that.
func (m *MatchV5) Time() time.Time {
	if m.Info.GameStartTimestamp > 0 {
		return time.UnixMilli(m.Info.GameStartTimestamp)
	}
	if m.Info.GameCreation > 0 {
		return time.UnixMilli(m.Info.GameCreation)
	}
	if m.Esports != nil && !m.Esports.Date.IsZero() {
		return m.Esports.Date
	}
	return time.Time{}
}

// Duration returns the match duration as a time.Duration.
func (m *MatchV5) Duration() time.Duration {
	return time.Duration(m.Info.GameDuration) * time.Second
}

// IsRanked returns true if the match is a ranked game (e.g. Ranked Solo/Duo, Ranked Flex, or Esports match).
func (m *MatchV5) IsRanked() bool {
	if m == nil {
		return false
	}
	if m.Esports != nil {
		return true
	}
	switch m.Info.QueueID {
	case 420, 440, 410, 470, 4, 6, 9, 42, 52:
		return true
	default:
		return false
	}
}

// GetParticipantByPUUID returns the ParticipantDto corresponding to the given PUUID, or nil if not found.
func (m *MatchV5) GetParticipantByPUUID(puuid string) *ParticipantDto {
	for _, p := range m.Info.Participants {
		if p != nil && p.PUUID == puuid {
			return p
		}
	}
	return nil
}

// GetParticipantByID returns the ParticipantDto corresponding to participantId (1-10), or nil if not found.
func (m *MatchV5) GetParticipantByID(participantID int) *ParticipantDto {
	for _, p := range m.Info.Participants {
		if p != nil && p.ParticipantID == participantID {
			return p
		}
	}
	return nil
}

// GetTeam returns the TeamDto for teamId (100 for Blue, 200 for Red), or nil if not found.
func (m *MatchV5) GetTeam(teamID int) *TeamDto {
	for _, t := range m.Info.Teams {
		if t != nil && t.TeamID == teamID {
			return t
		}
	}
	return nil
}

// GetTeamBySide returns the TeamDto for the specified Side (SideBlue or SideRed).
func (m *MatchV5) GetTeamBySide(side Side) *TeamDto {
	return m.GetTeam(side.TeamID())
}

// MetadataDto contains match metadata.
type MetadataDto struct {
	// DataVersion is the match data version format.
	DataVersion string `json:"dataVersion"`

	// MatchID is the unique match identifier (e.g., "NA1_1234567890" or OE game ID).
	MatchID string `json:"matchId"`

	// Participants is a list of participant PUUIDs.
	Participants []string `json:"participants"`
}

// InfoDto contains general game details and statistics.
type InfoDto struct {
	// EndOfGameResult indicates how the game ended (e.g., "GameComplete", "EarlySurrender", etc.).
	EndOfGameResult string `json:"endOfGameResult,omitempty"`

	// GameCreation is the Unix timestamp in milliseconds when the game was created on the game server.
	GameCreation int64 `json:"gameCreation"`

	// GameDuration is the duration of the game in seconds (for patch 11.20 and later).
	GameDuration int64 `json:"gameDuration"`

	// GameEndTimestamp is the Unix timestamp in milliseconds when the match finished.
	GameEndTimestamp int64 `json:"gameEndTimestamp,omitempty"`

	// GameID is the numerical game ID.
	GameID int64 `json:"gameId"`

	// GameMode is the game mode (e.g., "CLASSIC", "ARAM", "URF", "CHERRY").
	GameMode string `json:"gameMode"`

	// GameName is the custom game name.
	GameName string `json:"gameName,omitempty"`

	// GameStartTimestamp is the Unix timestamp in milliseconds when the match actually started.
	GameStartTimestamp int64 `json:"gameStartTimestamp"`

	// GameType is the type of game (e.g., "CUSTOM_GAME", "MATCHED_GAME", "TUTORIAL_GAME").
	GameType string `json:"gameType"`

	// GameVersion is the patch / version string (e.g., "14.1.1").
	GameVersion string `json:"gameVersion"`

	// MapID is the ID of the map (e.g., 11 for Summoner's Rift, 12 for Howling Abyss).
	MapID int `json:"mapId"`

	// Participants contains participant statistics for each player in the match.
	Participants []*ParticipantDto `json:"participants"`

	// PlatformID is the platform ID where the match was played (e.g., "NA1", "EUW1", "KR").
	PlatformID string `json:"platformId"`

	// QueueID is the queue ID (e.g., 420 for Ranked Solo/Duo, 440 for Ranked Flex).
	QueueID int `json:"queueId"`

	// Teams contains aggregated statistics and objective data for each team.
	Teams []*TeamDto `json:"teams"`

	// TournamentCode is the tournament code used to generate the match (if esports / custom tournament).
	TournamentCode string `json:"tournamentCode,omitempty"`
}

// TeamDto contains aggregated team statistics and objective accomplishments.
type TeamDto struct {
	// TeamID is the team ID (100 for Blue side, 200 for Red side).
	TeamID int `json:"teamId"`

	// Win indicates whether this team won the match.
	Win bool `json:"win"`

	// Bans contains the list of champion bans made by this team.
	Bans []BanDto `json:"bans"`

	// Objectives contains objective statistics (towers, dragons, barons, etc.).
	Objectives ObjectivesDto `json:"objectives"`

	// Esports contains additional esports-specific team data and Oracle's Elixir metrics.
	Esports *EsportsTeamData `json:"esports,omitempty"`
}

// BanDto represents a champion ban in champion select.
type BanDto struct {
	// ChampionID is the ID of the banned champion.
	ChampionID int `json:"championId"`

	// PickTurn is the turn order in which the ban was made.
	PickTurn int `json:"pickTurn"`

	// ChampionName is the name of the banned champion (available in esports / OE data).
	ChampionName string `json:"championName,omitempty"`
}

// ObjectivesDto contains objective counters for a team.
type ObjectivesDto struct {
	// Baron contains Baron Nashor statistics.
	Baron ObjectiveDto `json:"baron"`

	// Champion contains kill counters on champions.
	Champion ObjectiveDto `json:"champion"`

	// Dragon contains Dragon kill statistics.
	Dragon ObjectiveDto `json:"dragon"`

	// Horde contains Voidgrub kill statistics (introduced in Season 2024).
	Horde ObjectiveDto `json:"horde"`

	// Inhibitor contains Inhibitor takedown statistics.
	Inhibitor ObjectiveDto `json:"inhibitor"`

	// RiftHerald contains Rift Herald kill statistics.
	RiftHerald ObjectiveDto `json:"riftHerald"`

	// Tower contains Turret takedown statistics.
	Tower ObjectiveDto `json:"tower"`

	// Atakhan contains Atakhan kill statistics (introduced in Season 2025).
	Atakhan ObjectiveDto `json:"atakhan,omitempty"`
}

// ObjectiveDto contains statistics for a single objective type.
type ObjectiveDto struct {
	// First indicates whether the team was the first to secure this objective in the match.
	First bool `json:"first"`

	// Kills is the total count of this objective taken by the team.
	Kills int `json:"kills"`
}

// ParticipantDto contains comprehensive performance statistics for an individual player in a match.
type ParticipantDto struct {
	// PUUID is the Player Universally Unique Identifier.
	PUUID string `json:"puuid"`

	// SummonerID is the encrypted summoner ID.
	SummonerID string `json:"summonerId,omitempty"`

	// SummonerName is the summoner name.
	SummonerName string `json:"summonerName,omitempty"`

	// RiotIDGameName is the Riot ID game name (in-game display name).
	RiotIDGameName string `json:"riotIdGameName,omitempty"`

	// RiotIDTagline is the Riot ID tagline.
	RiotIDTagline string `json:"riotIdTagline,omitempty"`

	// ParticipantID is the participant ID within the match (1-10).
	ParticipantID int `json:"participantId"`

	// TeamID is the team ID (100 for Blue side, 200 for Red side).
	TeamID int `json:"teamId"`

	// ChampionID is the ID of the played champion.
	ChampionID int `json:"championId"`

	// ChampionName is the name of the played champion.
	ChampionName string `json:"championName"`

	// ChampLevel is the champion level at game end.
	ChampLevel int `json:"champLevel"`

	// ChampExperience is the total champion experience earned.
	ChampExperience int `json:"champExperience"`

	// ChampionTransform represents champion transform state (e.g. Kayn 1=Assassin, 2=Slayer).
	ChampionTransform int `json:"championTransform"`

	// Position is the parsed position enum (Top, Jungle, Mid, Bot, Support).
	Position Position `json:"position"`

	// IndividualPosition is the individual position assigned by the match engine.
	IndividualPosition string `json:"individualPosition,omitempty"`

	// TeamPosition is the position within the team.
	TeamPosition string `json:"teamPosition,omitempty"`

	// Role is the player's role (e.g., "SOLO", "DUO_CARRY", "DUO_SUPPORT", "NONE").
	Role string `json:"role,omitempty"`

	// Lane is the player's lane (e.g., "TOP", "JUNGLE", "MIDDLE", "BOTTOM").
	Lane string `json:"lane,omitempty"`

	// Kills is the number of champion kills by this participant.
	Kills int `json:"kills"`

	// Deaths is the number of times this participant died.
	Deaths int `json:"deaths"`

	// Assists is the number of assists by this participant.
	Assists int `json:"assists"`

	// DoubleKills is the count of double kills scored.
	DoubleKills int `json:"doubleKills"`

	// TripleKills is the count of triple kills scored.
	TripleKills int `json:"tripleKills"`

	// QuadraKills is the count of quadra kills scored.
	QuadraKills int `json:"quadraKills"`

	// PentaKills is the count of penta kills scored.
	PentaKills int `json:"pentaKills"`

	// UnrealKills is the count of unreal kills (6+ in quick succession) scored.
	UnrealKills int `json:"unrealKills"`

	// LargestMultiKill is the largest multikill achieved.
	LargestMultiKill int `json:"largestMultiKill"`

	// LargestKillingSpree is the largest killing spree achieved.
	LargestKillingSpree int `json:"largestKillingSpree"`

	// KillingSprees is the number of killing sprees started.
	KillingSprees int `json:"killingSprees"`

	// TotalDamageDealt is the total damage dealt (magic, physical, true) including to minions/monsters.
	TotalDamageDealt int `json:"totalDamageDealt"`

	// TotalDamageDealtToChampions is the total damage dealt to enemy champions.
	TotalDamageDealtToChampions int `json:"totalDamageDealtToChampions"`

	// PhysicalDamageDealt is the physical damage dealt to all targets.
	PhysicalDamageDealt int `json:"physicalDamageDealt"`

	// PhysicalDamageDealtToChampions is the physical damage dealt to enemy champions.
	PhysicalDamageDealtToChampions int `json:"physicalDamageDealtToChampions"`

	// MagicDamageDealt is the magic damage dealt to all targets.
	MagicDamageDealt int `json:"magicDamageDealt"`

	// MagicDamageDealtToChampions is the magic damage dealt to enemy champions.
	MagicDamageDealtToChampions int `json:"magicDamageDealtToChampions"`

	// TrueDamageDealt is the true damage dealt to all targets.
	TrueDamageDealt int `json:"trueDamageDealt"`

	// TrueDamageDealtToChampions is the true damage dealt to enemy champions.
	TrueDamageDealtToChampions int `json:"trueDamageDealtToChampions"`

	// TotalDamageTaken is the total damage taken from all sources.
	TotalDamageTaken int `json:"totalDamageTaken"`

	// PhysicalDamageTaken is the physical damage taken.
	PhysicalDamageTaken int `json:"physicalDamageTaken"`

	// MagicDamageTaken is the magic damage taken.
	MagicDamageTaken int `json:"magicDamageTaken"`

	// TrueDamageTaken is the true damage taken.
	TrueDamageTaken int `json:"trueDamageTaken"`

	// DamageSelfMitigated is the total damage mitigated by shields, armor, and magic resist.
	DamageSelfMitigated int `json:"damageSelfMitigated"`

	// DamageDealtToBuildings is the damage dealt to towers and inhibitors.
	DamageDealtToBuildings int `json:"damageDealtToBuildings"`

	// DamageDealtToObjectives is the damage dealt to neutral monsters and structures.
	DamageDealtToObjectives int `json:"damageDealtToObjectives"`

	// DamageDealtToTurrets is the damage dealt specifically to turrets.
	DamageDealtToTurrets int `json:"damageDealtToTurrets"`

	// TotalHeal is the total health restored to self and allies.
	TotalHeal int `json:"totalHeal"`

	// TotalHealsOnTeammates is the total health restored to teammates.
	TotalHealsOnTeammates int `json:"totalHealsOnTeammates"`

	// TotalDamageShieldedOnTeammates is the total shield amount applied to teammates.
	TotalDamageShieldedOnTeammates int `json:"totalDamageShieldedOnTeammates"`

	// TotalUnitsHealed is the count of distinct units healed.
	TotalUnitsHealed int `json:"totalUnitsHealed"`

	// TotalMinionsKilled is the number of lane minions killed (CS).
	TotalMinionsKilled int `json:"totalMinionsKilled"`

	// NeutralMinionsKilled is the total number of neutral jungle monsters killed.
	NeutralMinionsKilled int `json:"neutralMinionsKilled"`

	// GoldEarned is the total gold earned during the match.
	GoldEarned int `json:"goldEarned"`

	// GoldSpent is the total gold spent on items.
	GoldSpent int `json:"goldSpent"`

	// ItemsPurchased is the count of items purchased.
	ItemsPurchased int `json:"itemsPurchased"`

	// ConsumablesPurchased is the count of consumable items purchased.
	ConsumablesPurchased int `json:"consumablesPurchased"`

	// Item0 is the item ID in inventory slot 0.
	Item0 int `json:"item0"`
	// Item1 is the item ID in inventory slot 1.
	Item1 int `json:"item1"`
	// Item2 is the item ID in inventory slot 2.
	Item2 int `json:"item2"`
	// Item3 is the item ID in inventory slot 3.
	Item3 int `json:"item3"`
	// Item4 is the item ID in inventory slot 4.
	Item4 int `json:"item4"`
	// Item5 is the item ID in inventory slot 5.
	Item5 int `json:"item5"`
	// Item6 is the item ID in inventory slot 6 (trinket slot).
	Item6 int `json:"item6"`

	// VisionScore is the calculated vision score.
	VisionScore int `json:"visionScore"`

	// WardsPlaced is the number of wards placed.
	WardsPlaced int `json:"wardsPlaced"`

	// WardsKilled is the number of enemy wards destroyed.
	WardsKilled int `json:"wardsKilled"`

	// DetectorWardsPlaced is the number of control wards placed.
	DetectorWardsPlaced int `json:"detectorWardsPlaced"`

	// VisionWardsBoughtInGame is the number of control wards bought.
	VisionWardsBoughtInGame int `json:"visionWardsBoughtInGame"`

	// SightWardsBoughtInGame is the number of stealth/sight wards bought.
	SightWardsBoughtInGame int `json:"sightWardsBoughtInGame"`

	// TurretKills is the number of direct turret last-hits.
	TurretKills int `json:"turretKills"`

	// TurretTakedowns is the number of turret kills and assists.
	TurretTakedowns int `json:"turretTakedowns"`

	// TurretsLost is the number of allied turrets destroyed while alive.
	TurretsLost int `json:"turretsLost"`

	// InhibitorKills is the number of direct inhibitor last-hits.
	InhibitorKills int `json:"inhibitorKills"`

	// InhibitorTakedowns is the number of inhibitor kills and assists.
	InhibitorTakedowns int `json:"inhibitorTakedowns"`

	// InhibitorsLost is the number of allied inhibitors destroyed.
	InhibitorsLost int `json:"inhibitorsLost"`

	// NexusKills is the number of direct nexus last-hits.
	NexusKills int `json:"nexusKills"`

	// NexusTakedowns is the number of nexus kills and assists.
	NexusTakedowns int `json:"nexusTakedowns"`

	// NexusLost is the count of nexus losses.
	NexusLost int `json:"nexusLost"`

	// FirstBloodKill indicates whether the player got first blood kill.
	FirstBloodKill bool `json:"firstBloodKill"`

	// FirstBloodAssist indicates whether the player got first blood assist.
	FirstBloodAssist bool `json:"firstBloodAssist"`

	// FirstBloodVictim indicates whether the player was the victim of first blood (OE data).
	FirstBloodVictim bool `json:"firstBloodVictim,omitempty"`

	// FirstTowerKill indicates whether the player got first turret kill.
	FirstTowerKill bool `json:"firstTowerKill"`

	// FirstTowerAssist indicates whether the player got first turret assist.
	FirstTowerAssist bool `json:"firstTowerAssist"`

	// Win indicates whether the player won the match.
	Win bool `json:"win"`

	// GameEndedInSurrender indicates whether the match ended via standard surrender.
	GameEndedInSurrender bool `json:"gameEndedInSurrender"`

	// GameEndedInEarlySurrender indicates whether the match ended via early / remake surrender.
	GameEndedInEarlySurrender bool `json:"gameEndedInEarlySurrender"`

	// TeamEarlySurrendered indicates whether the player's team voted for early surrender.
	TeamEarlySurrendered bool `json:"teamEarlySurrendered"`

	// TimePlayed is the total time played in seconds.
	TimePlayed int `json:"timePlayed"`

	// TimeCCingOthers is the total crowd control duration applied to enemies in seconds.
	TimeCCingOthers int `json:"timeCCingOthers"`

	// TotalTimeSpentDead is the total duration spent dead in seconds.
	TotalTimeSpentDead int `json:"totalTimeSpentDead"`

	// TotalTimeCCDealt is the total crowd control duration dealt.
	TotalTimeCCDealt int `json:"totalTimeCCDealt"`

	// Summoner1ID is the spell ID of the first summoner spell (D).
	Summoner1ID int `json:"summoner1Id"`

	// Summoner1Casts is the number of times summoner spell 1 was cast.
	Summoner1Casts int `json:"summoner1Casts"`

	// Summoner2ID is the spell ID of the second summoner spell (F).
	Summoner2ID int `json:"summoner2Id"`

	// Summoner2Casts is the number of times summoner spell 2 was cast.
	Summoner2Casts int `json:"summoner2Casts"`

	// Spell1Casts is the number of Q spell casts.
	Spell1Casts int `json:"spell1Casts"`

	// Spell2Casts is the number of W spell casts.
	Spell2Casts int `json:"spell2Casts"`

	// Spell3Casts is the number of E spell casts.
	Spell3Casts int `json:"spell3Casts"`

	// Spell4Casts is the number of R spell casts.
	Spell4Casts int `json:"spell4Casts"`

	// Perks contains rune / perk tree selections.
	Perks *PerksDto `json:"perks,omitempty"`

	// Challenges contains challenge progression stats (Riot API).
	Challenges map[string]interface{} `json:"challenges,omitempty"`

	// Summoner is a direct reference to the corresponding SummonerV4 struct in the dataset.
	Summoner *SummonerV4 `json:"-"`

	// Esports contains additional esports-specific performance metrics and Oracle's Elixir stats.
	Esports *EsportsParticipantData `json:"esports,omitempty"`
}

// PerksDto contains runes/perks tree and stat shard choices.
type PerksDto struct {
	// StatPerks contains stat shard selections (offense, flex, defense).
	StatPerks PerkStatsDto `json:"statPerks"`

	// Styles contains primary and secondary rune trees.
	Styles []PerkStyleDto `json:"styles"`
}

// PerkStatsDto contains stat shard perks.
type PerkStatsDto struct {
	// Defense is the defensive stat perk ID.
	Defense int `json:"defense"`
	// Flex is the flex stat perk ID.
	Flex int `json:"flex"`
	// Offense is the offensive stat perk ID.
	Offense int `json:"offense"`
}

// PerkStyleDto contains rune style tree details and selected perks.
type PerkStyleDto struct {
	// Description describes the tree (e.g. "primaryStyle", "subStyle").
	Description string `json:"description"`
	// Selections contains individual perk choices.
	Selections []PerkStyleSelectionDto `json:"selections"`
	// Style is the rune tree style ID (e.g., 8000 for Precision, 8100 for Domination, etc.).
	Style int `json:"style"`
}

// PerkStyleSelectionDto represents a selected rune perk.
type PerkStyleSelectionDto struct {
	// Perk is the perk / rune ID.
	Perk int `json:"perk"`
	// Var1 is the first tracking variable for the rune.
	Var1 int `json:"var1"`
	// Var2 is the second tracking variable for the rune.
	Var2 int `json:"var2"`
	// Var3 is the third tracking variable for the rune.
	Var3 int `json:"var3"`
}

// EsportsMatchData contains esports tournament metadata and series information (from Oracle's Elixir, etc.).
type EsportsMatchData struct {
	// League is the tournament league (e.g., "LCS", "LEC", "LCK", "LPL", "LFL2").
	League string `json:"league"`

	// Year is the competition year (e.g., 2025).
	Year int `json:"year"`

	// Split is the split name (e.g., "Winter", "Spring", "Summer", "Split 1").
	Split string `json:"split"`

	// Playoffs indicates whether this was a playoff / knockout match.
	Playoffs bool `json:"playoffs"`

	// Date is the scheduled or recorded match start time.
	Date time.Time `json:"date"`

	// GameNumber is the game number within a match series (e.g., 1, 2, 3 in a Bo3 or Bo5).
	GameNumber int `json:"gameNumber"`

	// Patch is the game patch version (e.g., "15.01").
	Patch string `json:"patch"`

	// DataCompleteness indicates whether all statistics were recorded.
	DataCompleteness DataCompleteness `json:"dataCompleteness"`

	// URL is the link to the match match history or source.
	URL string `json:"url,omitempty"`

	// TeamKPM is the combined kills per minute for a team in Oracle's Elixir.
	TeamKPM float64 `json:"teamKpm,omitempty"`

	// CKPM is the combined kills per minute for both teams.
	CKPM float64 `json:"ckpm,omitempty"`
}

// EsportsTeamData contains team-level esports statistics from Oracle's Elixir.
type EsportsTeamData struct {
	// TeamName is the name of the team.
	TeamName string `json:"teamName"`

	// TeamID is the Oracle's Elixir team ID (e.g., "oe:team:...").
	TeamID string `json:"teamId"`

	// Side is the side of the map (Blue or Red).
	Side Side `json:"side"`

	// FirstPick indicates whether this team had first pick in champion select.
	FirstPick bool `json:"firstPick"`

	// Bans contains the list of champion names banned by this team.
	Bans []string `json:"bans,omitempty"`

	// Picks contains the list of champion names picked by this team.
	Picks []string `json:"picks,omitempty"`

	// TeamKills is the total kills scored by the team.
	TeamKills int `json:"teamKills"`

	// TeamDeaths is the total deaths suffered by the team.
	TeamDeaths int `json:"teamDeaths"`

	// TeamKPM is the kills per minute achieved by the team.
	TeamKPM float64 `json:"teamKpm"`

	// CKPM is the combined kills per minute of the match.
	CKPM float64 `json:"ckpm"`

	// FirstDragon indicates whether the team secured the first dragon.
	FirstDragon bool `json:"firstDragon"`

	// Dragons is the total number of dragons secured by the team.
	Dragons int `json:"dragons"`

	// OppDragons is the total number of dragons secured by the opponent.
	OppDragons int `json:"oppDragons"`

	// ElementalDrakes is the number of elemental dragons secured.
	ElementalDrakes int `json:"elementalDrakes"`

	// OppElementalDrakes is the number of elemental dragons secured by the opponent.
	OppElementalDrakes int `json:"oppElementalDrakes"`

	// Infernals is the count of Infernal Drakes killed.
	Infernals int `json:"infernals"`

	// Mountains is the count of Mountain Drakes killed.
	Mountains int `json:"mountains"`

	// Clouds is the count of Cloud Drakes killed.
	Clouds int `json:"clouds"`

	// Oceans is the count of Ocean Drakes killed.
	Oceans int `json:"oceans"`

	// Chemtechs is the count of Chemtech Drakes killed.
	Chemtechs int `json:"chemtechs"`

	// Hextechs is the count of Hextech Drakes killed.
	Hextechs int `json:"hextechs"`

	// DragonsTypeUnknown is the count of dragons killed whose type was not recorded.
	DragonsTypeUnknown int `json:"dragonsTypeUnknown"`

	// Elders is the count of Elder Dragons killed.
	Elders int `json:"elders"`

	// OppElders is the count of Elder Dragons killed by the opponent.
	OppElders int `json:"oppElders"`

	// FirstHerald indicates whether the team killed the first Rift Herald.
	FirstHerald bool `json:"firstHerald"`

	// Heralds is the number of Rift Heralds killed.
	Heralds int `json:"heralds"`

	// OppHeralds is the number of Rift Heralds killed by opponent.
	OppHeralds int `json:"oppHeralds"`

	// VoidGrubs is the number of Voidgrubs killed.
	VoidGrubs int `json:"voidGrubs"`

	// OppVoidGrubs is the number of Voidgrubs killed by opponent.
	OppVoidGrubs int `json:"oppVoidGrubs"`

	// FirstBaron indicates whether the team killed the first Baron Nashor.
	FirstBaron bool `json:"firstBaron"`

	// Barons is the number of Baron Nashors killed.
	Barons int `json:"barons"`

	// OppBarons is the number of Baron Nashors killed by opponent.
	OppBarons int `json:"oppBarons"`

	// Atakhans is the number of Atakhans killed.
	Atakhans int `json:"atakhans"`

	// OppAtakhans is the number of Atakhans killed by opponent.
	OppAtakhans int `json:"oppAtakhans"`

	// FirstTower indicates whether the team destroyed the first turret.
	FirstTower bool `json:"firstTower"`

	// Towers is the number of enemy turrets destroyed.
	Towers int `json:"towers"`

	// OppTowers is the number of allied turrets lost.
	OppTowers int `json:"oppTowers"`

	// FirstMidTower indicates whether the team destroyed the first mid lane turret.
	FirstMidTower bool `json:"firstMidTower"`

	// FirstToThreeTowers indicates whether the team was the first to destroy 3 turrets.
	FirstToThreeTowers bool `json:"firstToThreeTowers"`

	// TurretPlates is the number of turret plates destroyed.
	TurretPlates int `json:"turretPlates"`

	// OppTurretPlates is the number of turret plates destroyed by opponent.
	OppTurretPlates int `json:"oppTurretPlates"`

	// Inhibitors is the number of enemy inhibitors destroyed.
	Inhibitors int `json:"inhibitors"`

	// OppInhibitors is the number of allied inhibitors destroyed.
	OppInhibitors int `json:"oppInhibitors"`

	// DamageToChampions is the total damage dealt to enemy champions by the team.
	DamageToChampions int `json:"damageToChampions"`

	// DPM is the damage dealt to champions per minute.
	DPM float64 `json:"dpm"`

	// DamageShare is the team's damage share (typically 1.0 for team row).
	DamageShare float64 `json:"damageShare"`

	// DamageTakenPerMinute is the damage taken per minute.
	DamageTakenPerMinute float64 `json:"damageTakenPerMinute"`

	// DamageMitigatedPerMinute is the damage mitigated per minute.
	DamageMitigatedPerMinute float64 `json:"damageMitigatedPerMinute"`

	// DamageToTowers is the damage dealt to enemy turrets.
	DamageToTowers float64 `json:"damageToTowers"`

	// WardsPlaced is the number of wards placed by the team.
	WardsPlaced int `json:"wardsPlaced"`

	// WPM is the wards placed per minute.
	WPM float64 `json:"wpm"`

	// WardsKilled is the number of enemy wards cleared.
	WardsKilled int `json:"wardsKilled"`

	// WCPM is the wards cleared per minute.
	WCPM float64 `json:"wcpm"`

	// ControlWardsBought is the number of control wards purchased.
	ControlWardsBought int `json:"controlWardsBought"`

	// VisionScore is the total vision score of the team.
	VisionScore float64 `json:"visionScore"`

	// VSPM is the vision score per minute.
	VSPM float64 `json:"vspm"`

	// TotalGold is the total gold accumulated by the team.
	TotalGold int `json:"totalGold"`

	// EarnedGold is the total gold earned excluding passive/starting gold.
	EarnedGold int `json:"earnedGold"`

	// EarnedGPM is the earned gold per minute.
	EarnedGPM float64 `json:"earnedGpm"`

	// EarnedGoldShare is the share of earned gold.
	EarnedGoldShare float64 `json:"earnedGoldShare"`

	// GoldSpent is the total gold spent on items.
	GoldSpent int `json:"goldSpent"`

	// GSPD is the gold spent difference vs opponent.
	GSPD float64 `json:"gspd"`

	// GPR is the gold percentage ratio.
	GPR float64 `json:"gpr"`

	// TotalCS is the total creep score (minions + monsters).
	TotalCS int `json:"totalCs"`

	// MinionKills is the total minion kills.
	MinionKills int `json:"minionKills"`

	// MonsterKills is the total jungle monster kills.
	MonsterKills int `json:"monsterKills"`

	// MonsterKillsOwnJungle is the monsters killed in own jungle.
	MonsterKillsOwnJungle int `json:"monsterKillsOwnJungle"`

	// MonsterKillsEnemyJungle is the monsters killed in enemy jungle.
	MonsterKillsEnemyJungle int `json:"monsterKillsEnemyJungle"`

	// CSPM is the creep score per minute.
	CSPM float64 `json:"cspm"`

	// IntervalStats maps minute thresholds (10, 15, 20, 25) to interval metrics.
	IntervalStats map[int]*IntervalStats `json:"intervalStats,omitempty"`
}

// EsportsParticipantData contains individual player esports statistics from Oracle's Elixir.
type EsportsParticipantData struct {
	// PlayerName is the competitive handle / IGN of the player.
	PlayerName string `json:"playerName"`

	// PlayerID is the Oracle's Elixir player ID (e.g., "oe:player:...").
	PlayerID string `json:"playerId"`

	// TeamName is the name of the player's team.
	TeamName string `json:"teamName"`

	// TeamID is the Oracle's Elixir team ID (e.g., "oe:team:...").
	TeamID string `json:"teamId"`

	// Position is the player's position (Top, Jungle, Mid, Bot, Support).
	Position Position `json:"position"`

	// Side is the side of the map (Blue or Red).
	Side Side `json:"side"`

	// FirstPick indicates whether this player's team had first pick.
	FirstPick bool `json:"firstPick"`

	// TeamKills is the total kills scored by the player's team.
	TeamKills int `json:"teamKills"`

	// TeamDeaths is the total deaths suffered by the player's team.
	TeamDeaths int `json:"teamDeaths"`

	// FirstBlood indicates whether first blood occurred in the match.
	FirstBlood bool `json:"firstBlood"`

	// FirstBloodKill indicates whether this player secured first blood kill.
	FirstBloodKill bool `json:"firstBloodKill"`

	// FirstBloodAssist indicates whether this player assisted on first blood.
	FirstBloodAssist bool `json:"firstBloodAssist"`

	// FirstBloodVictim indicates whether this player was killed for first blood.
	FirstBloodVictim bool `json:"firstBloodVictim"`

	// TeamKPM is the kills per minute of the player's team.
	TeamKPM float64 `json:"teamKpm"`

	// CKPM is the combined kills per minute of the match.
	CKPM float64 `json:"ckpm"`

	// DamageToChampions is the damage dealt to champions.
	DamageToChampions int `json:"damageToChampions"`

	// DPM is the damage to champions per minute.
	DPM float64 `json:"dpm"`

	// DamageShare is the player's percentage share of their team's damage to champions.
	DamageShare float64 `json:"damageShare"`

	// DamageTakenPerMinute is the damage taken per minute.
	DamageTakenPerMinute float64 `json:"damageTakenPerMinute"`

	// DamageMitigatedPerMinute is the damage mitigated per minute.
	DamageMitigatedPerMinute float64 `json:"damageMitigatedPerMinute"`

	// DamageToTowers is the damage dealt to turrets.
	DamageToTowers float64 `json:"damageToTowers"`

	// WardsPlaced is the count of wards placed.
	WardsPlaced int `json:"wardsPlaced"`

	// WPM is the wards placed per minute.
	WPM float64 `json:"wpm"`

	// WardsKilled is the count of enemy wards destroyed.
	WardsKilled int `json:"wardsKilled"`

	// WCPM is the wards cleared per minute.
	WCPM float64 `json:"wcpm"`

	// ControlWardsBought is the count of control wards bought.
	ControlWardsBought int `json:"controlWardsBought"`

	// VisionScore is the vision score achieved.
	VisionScore float64 `json:"visionScore"`

	// VSPM is the vision score per minute.
	VSPM float64 `json:"vspm"`

	// TotalGold is the total gold earned.
	TotalGold int `json:"totalGold"`

	// EarnedGold is the gold earned excluding base/passive gold.
	EarnedGold int `json:"earnedGold"`

	// EarnedGPM is the earned gold per minute.
	EarnedGPM float64 `json:"earnedGpm"`

	// EarnedGoldShare is the player's percentage share of their team's earned gold.
	EarnedGoldShare float64 `json:"earnedGoldShare"`

	// GoldSpent is the gold spent on items.
	GoldSpent int `json:"goldSpent"`

	// GSPD is the gold spent difference.
	GSPD float64 `json:"gspd"`

	// GPR is the gold percent ratio.
	GPR float64 `json:"gpr"`

	// TotalCS is the total creep score (minions + monsters).
	TotalCS int `json:"totalCs"`

	// MinionKills is the number of lane minions killed.
	MinionKills int `json:"minionKills"`

	// MonsterKills is the number of neutral monsters killed.
	MonsterKills int `json:"monsterKills"`

	// MonsterKillsOwnJungle is the number of monsters killed in own team's jungle.
	MonsterKillsOwnJungle int `json:"monsterKillsOwnJungle"`

	// MonsterKillsEnemyJungle is the number of monsters killed in opponent's jungle.
	MonsterKillsEnemyJungle int `json:"monsterKillsEnemyJungle"`

	// CSPM is the creep score per minute.
	CSPM float64 `json:"cspm"`

	// IntervalStats maps minute intervals (10, 15, 20, 25) to interval snapshot metrics.
	IntervalStats map[int]*IntervalStats `json:"intervalStats,omitempty"`
}

// IntervalStats contains snapshot metrics at specific time milestones (10, 15, 20, 25 minutes).
type IntervalStats struct {
	// Minute is the interval milestone (10, 15, 20, or 25).
	Minute int `json:"minute"`

	// Gold is the total gold at this minute.
	Gold int `json:"gold"`

	// XP is the total experience at this minute.
	XP int `json:"xp"`

	// CS is the total creep score at this minute.
	CS int `json:"cs"`

	// OppGold is the opponent's direct lane counterpart gold at this minute.
	OppGold int `json:"oppGold"`

	// OppXP is the opponent's direct lane counterpart XP at this minute.
	OppXP int `json:"oppXp"`

	// OppCS is the opponent's direct lane counterpart CS at this minute.
	OppCS int `json:"oppCs"`

	// GoldDiff is the gold difference (Gold - OppGold) at this minute.
	GoldDiff int `json:"goldDiff"`

	// XPDiff is the XP difference (XP - OppXP) at this minute.
	XPDiff int `json:"xpDiff"`

	// CSDiff is the CS difference (CS - OppCS) at this minute.
	CSDiff int `json:"csDiff"`

	// Kills is the kills at this minute.
	Kills int `json:"kills"`

	// Assists is the assists at this minute.
	Assists int `json:"assists"`

	// Deaths is the deaths at this minute.
	Deaths int `json:"deaths"`

	// OppKills is the opponent counterpart kills at this minute.
	OppKills int `json:"oppKills"`

	// OppAssists is the opponent counterpart assists at this minute.
	OppAssists int `json:"oppAssists"`

	// OppDeaths is the opponent counterpart deaths at this minute.
	OppDeaths int `json:"oppDeaths"`
}
