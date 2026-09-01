# loldata

Go library and tools for processing League of Legends match and player data for statistical analysis and machine learning experimentation.

Purely for research (Graph Neural Networks) and improvement purposes.

## Goals

This is very experimental, but loose goals would be something like:

- Machine learning a match winner (and maybe other variables) prediction model.
- Tool to display for a summoner, how they performed vs how it was predicted.
- What-if tools: provide predicted counterfactuals, to help players make better choices.

## Features

- **Riot Match-V5 & Summoner-V4 Compatible Models**: Complete Go structs (`MatchV5`, `SummonerV4`, `ParticipantDto`, `TeamDto`, etc.) adhering to the official Riot Games API schemas.
- **Esports & Oracle's Elixir Integration**: Support for detailed esports statistics, draft bans/picks, milestone timeline stats (gold, XP, CS, and diffs at 10, 15, 20, 25 minutes), DPM, damage shares, and vision metrics.
- **Bidirectional & Chronological Match History**: `SummonerV4` profiles automatically link to all matches they participated in, kept in chronological order.
- **Fast CSV Ingestion**: High-throughput parsing of Oracle's Elixir match data (aggregates 12 CSV rows per match into unified `MatchV5` records).
- **Type-safe Enumerations**: Uses `enumer` to generate string, JSON, and YAML conversion methods for `Side`, `Position`, `DataCompleteness`, and `DragonType`.

---

## Command-Line Tool: `cmd/match_crawler`

`cmd/match_crawler` is a CLI tool to crawl Riot Match-V5 and Summoner-V4 APIs to collect match records and player profiles into a JSON dataset.

### Usage

```bash
export RIOT_API_KEY="RGAPI-..."

go run ./cmd/match_crawler \
  -seed "Faker#KR1" \
  -platform kr \
  -dataset ./kr_matches.json \
  -checkpoints 10m \
  -backups 3 \
  -start_time "7d" \
  -limit_reqs_persec 19 \
  -limit_reqs_per2min 99
```

### Features & Flags

- **Dataset Persistence (`-dataset`)**: Reads existing dataset on startup, saves progress on periodic checkpoints, and performs final save on exit.
- **Atomic Writes & Backups (`-checkpoints`, `-backups`)**: Saves to temporary `<dataset>~` file, safely rotates backups into `<dataset>.backup-YYYYMMDDhhmmss`, prunes older backups, and atomically renames.
- **Strict Global Rate Limiting (`-limit_reqs_persec`, `-limit_reqs_per2min`)**: Enforces multi-window throttling across all API calls to prevent Riot server blocks.
- **Time Filtering (`-start_time`, `-end_time`)**: Downloads only matches within specified window (supports relative durations like `7d`, `1w`, `24h` or dates/timestamps).
- **Riot ID Resolution (`-seed`)**: Resolves player seeds (`GameName#TagLine`) to PUUIDs and crawls matches and participants graph.

---

## Command-Line Tool: `cmd/stats`

`cmd/stats` is a CLI tool to load Oracle's Elixir CSV or JSON datasets and compute dataset-wide statistics or detailed individual summoner statistics.

### Running the Tool

You can run `cmd/stats` by passing JSON dataset files or CSV file paths via flags or positional arguments:

```bash
# General dataset statistics from JSON or CSV
go run ./cmd/stats -json ./dataset.json
go run ./cmd/stats -oe ~/work/lol/2025_LoL_esports_match_data_from_OraclesElixir.csv

# Individual summoner performance & champion distribution statistics
go run ./cmd/stats -json ./dataset.json -summoner "LuckyShott#1114"
go run ./cmd/stats -json ./dataset.json -summoner "Faker"
```

### Statistics Provided

#### 1. General Dataset Statistics
- **Total Matches and Players**: Aggregated count of unique games and summoners.
- **Matches per Player Distribution & Quantiles**: Min, 10th percentile, 25th percentile (Q1), Median (50th percentile), 75th percentile (Q3), 90th percentile, 95th percentile, 99th percentile, Max, and Mean.
- **Top 10 Most Active Players**: Players with the highest number of recorded matches and their PUUIDs.
- **Match Metadata & Durations**: Date range of matches, duration statistics (Min, Average, Median, Max), and Blue vs. Red side win rates.
- **Top Leagues**: Distribution of matches across competitive leagues (LPL, LCK, LEC, LCS, etc.).

#### 2. Summoner Statistics (`-summoner <name|puuid>`)
- **Overall Performance & Win Ratio**: Total games, wins, losses, win rate percentage, and Blue vs. Red side win rates.
- **Combat & KDA**: Average kills/deaths/assists, KDA ratio, kill participation (KP%), multikills (doubles, triples, quadras, pentas), First Blood (kills, assists, victims), and First Tower stats.
- **Farming, Economy & Damage**: Average CS, CS per minute (CSPM), average gold, gold per minute (GPM), average champion damage, damage per minute (DPM), damage share, damage taken, and turret damage.
- **Vision Metrics**: Average vision score, vision score per minute (VSPM), wards placed, wards cleared, and control wards.
- **Position / Role Distribution**: Frequency, percentage share of matches, record, win rate, KDA, CSPM, and DPM per position (highlighting most played role).
- **Champions Played Distribution**: Comprehensive champion pool breakdown sorted by games played with percentage share of matches, win rate, KDA, CSPM, and DPM (highlighting most played champion).
- **Most Frequent Teammates / Duo Partners**: Most frequent teammates played with, percentage of matches played together, record, and duo win rates.
- **Most Frequent Opponents**: Frequent rivals and win rates against them.
- **Game Modes & Queues**: Performance breakdown across Ranked Solo/Duo, Ranked Flex, Normal Draft, ARAM, etc.
- **Recent Match History**: Last 10 matches summary (Date, Result, Champion, Role, KDA, Duration, Match ID).

> **Note on CSV Row Count vs. Match Count**:
> In Oracle's Elixir match exports, each game is recorded across **12 rows**:
> - 10 individual player rows (`participantid` 1–10: 5 Blue + 5 Red)
> - 2 team summary rows (`participantid` 100 for Blue + 200 for Red, with `position="team"`)
>
> Therefore, a dataset of ~120,492 data rows maps to exactly **10,041 matches**.

---

## Code Generation

Enumeration methods are generated using `go tool enumer`. All generated files use the `gen_` prefix for easy identification.

To regenerate enum files:

```bash
go generate ./...
```

Generated files in `./data`:
- `gen_side_enumer.go`
- `gen_position_enumer.go`
- `gen_datacompleteness_enumer.go`
- `gen_dragontype_enumer.go`

---

## Running Tests

Run all unit tests:

```bash
go test -v ./...
```

---

## External References & Sources

- **[Oracle's Elixir](https://oracleselixir.com/)**: Comprehensive League of Legends esports statistics database and match downloads.
- **[Oracle's Elixir Definitions](https://oracleselixir.com/definitions)**: Definitions and formulas for all advanced statistics (e.g., DPM, CSPM, Gold/XP/CS diffs at 10/15/20/25, GSPD, GPR, DMG%).
- **[Riot Games Developer Portal](https://developer.riotgames.com/)**: Official developer documentation and API reference.
- **[Riot Match-V5 API Reference](https://developer.riotgames.com/apis#match-v5/GET_getMatch)**: Endpoint specification for `GET /lol/match/v5/matches/{matchId}` (`MatchDto`, `MetadataDto`, `InfoDto`, `ParticipantDto`, `TeamDto`).
- **[Riot Summoner-V4 API Reference](https://developer.riotgames.com/apis#summoner-v4/GET_getByPUUID)**: Endpoint specification for `GET /lol/summoner/v4/summoners/by-puuid/{encryptedPUUID}` (`SummonerDTO`).
