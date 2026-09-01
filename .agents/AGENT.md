# Agent Guide: `loldata`

This document provides AI agents with a structural and architectural overview of the `loldata` codebase.

---

## 1. Project Purpose & Scope

`loldata` is a Go library and suite of CLI tools for processing, analyzing, and structuring League of Legends matches and player performance data. It combines schemas from:
1. **Riot Games Match-V5 API** (`/lol/match/v5/matches/{matchId}`)
2. **Riot Games Summoner-V4 API** (`/lol/summoner/v4/summoners/by-puuid/{puuid}`)
3. **Oracle's Elixir Esports Datasets** (Draft picks/bans, advanced timeline metrics at 10/15/20/25 mins, DPM, damage shares, vision score rates).

---

## 2. Directory Structure

```
loldata/
├── .agents/
│   └── AGENT.md                   # This guide for AI agents
├── cmd/
│   └── stats/
│       └── main.go                # CLI tool to inspect dataset statistics & quantiles
├── data/
│   ├── dataset.go                 # Dataset struct, match & summoner registration/indexing
│   ├── dataset_test.go            # Unit and regression tests
│   ├── enums.go                   # Side, Position, DataCompleteness, DragonType enums
│   ├── match_v5.go                # MatchV5, InfoDto, ParticipantDto, TeamDto & esports models
│   ├── oracles_elixir.go          # Oracle's Elixir CSV loader and row-aggregation logic
│   ├── summoner_v4.go             # SummonerV4 struct and chronological match linking
├── go.mod                         # Go module definition (Go 1.26+)
├── go.sum
└── README.md                      # Human-facing documentation & reference links
```

---

## 3. Core Data Structures & Files

### `data/match_v5.go`
- **`MatchV5`**: Root match structure containing `Metadata` (`MetadataDto`), `Info` (`InfoDto`), and optional `Esports` (`*EsportsMatchData`).
- **`ParticipantDto`**: Per-player statistics (kills, deaths, damage, CS, vision, gold, items, runes) + `Summoner *SummonerV4` link + `Esports *EsportsParticipantData`.
- **`TeamDto`**: Team-level statistics (bans, objectives, win/loss) + `Esports *EsportsTeamData`.
- **`IntervalStats`**: Snapshot metrics (Gold, XP, CS, diffs, KDA) at minute intervals 10, 15, 20, 25.
- Helper methods:
  - `MatchV5.Time() time.Time`: Returns game start/creation timestamp.
  - `MatchV5.Duration() time.Duration`: Returns match duration.
  - `MatchV5.GetParticipantByPUUID(puuid string) *ParticipantDto`
  - `MatchV5.GetTeamBySide(side Side) *TeamDto`

### `data/summoner_v4.go`
- **`SummonerV4`**: Player profile with `PUUID`, `Name`, `AccountID`, `SummonerLevel`, and `Matches []*MatchV5`.
- **`SummonerV4.AddMatch(m *MatchV5)`**: Inserts a match in chronological order (by match time) and avoids duplicates.

### `data/dataset.go`
- **`Dataset`**: Top-level container:
  - `Matches []*MatchV5`
  - `Summoners []*SummonerV4`
  - `PUUIDToSummoner map[string]*SummonerV4`
  - `MatchIDToMatch map[string]*MatchV5`
- **`Dataset.AddMatch(m *MatchV5)`**: Registers match, resolves or creates participant summoners, and establishes bidirectional links.

### `data/oracles_elixir.go`
- **`Dataset.LoadOraclesElixir(csvFilePath string) error`**:
  - Reads CSV files from Oracle's Elixir.
  - Groups 12 rows sharing the same `gameid` (10 player rows + 2 team rows) into a single `MatchV5` instance.
  - Fallback logic: if `playerid` is empty in the CSV, uses `playername` as the PUUID key.

### `data/enums.go` & Generated Files
- **Enums**: `Side` (Blue/Red), `Position` (Top/Jungle/Mid/Bot/Support/Team), `DataCompleteness` (Complete/Partial), `DragonType`.
- **Code Generation**: Uses `go tool enumer` via `//go:generate` directives.
- **Convention**: All generated files must be prefixed with `gen_` (e.g. `gen_side_enumer.go`).

---

## 4. CLI Tools (`cmd/`)

### `cmd/stats/main.go`
- Accepts `-oe <files>` or positional arguments.
- Loads Oracle's Elixir CSV data into `data.Dataset`.
- Computes and prints:
  - Total match and player counts.
  - Matches-per-player quantiles (Min, p10, p25, p50/Median, p75, p90, p95, p99, Max, Mean).
  - Top 10 most active players.
  - Match duration stats (Min, Avg, Median, Max).
  - Blue vs Red win rates.
  - League distribution.

---

## 5. Important Invariants & Conventions

1. **Never Commit / Push to Git**: Let the user review changes unless explicitly asked to commit.
2. **Generated File Prefix**: All generated code files must start with `gen_`.
3. **Chronological Ordering**: A player's `Matches` slice must always be ordered by `m.Time()`.
4. **Oracle's Elixir 12-Row Invariant**:
   - Each esports match in Oracle's Elixir consists of 10 participant rows (`participantid` 1–10) and 2 team rows (`participantid` 100/200, `position == "team"`).
5. **No External Imports without Verification**: Standard library + `enumer` are used; check latest online documentation if adding new packages.

---

## 6. Common Commands

```bash
# Run tests
go test -v ./...

# Run code generator for enums
go generate ./...

# Run stats CLI tool
go run ./cmd/stats -oe ~/work/lol/2025_LoL_esports_match_data_from_OraclesElixir.csv
```

## 7. Generate Code

All the generated code must be in files starting with `gen_`. Currently, generate code is used only for enumerations names (using the `enumer` tool).
