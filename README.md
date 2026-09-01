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

## Command-Line Tool: `cmd/stats`

`cmd/stats` is a CLI tool to load Oracle's Elixir CSV datasets and compute dataset-wide statistics, player activity distributions, and match summaries.

### Running the Tool

You can run `cmd/stats` by passing CSV file paths via the `-oe` flag or as positional arguments (glob patterns and comma-separated lists are supported):

```bash
# Using the -oe flag
go run ./cmd/stats -oe ~/work/lol/2025_LoL_esports_match_data_from_OraclesElixir.csv

# Passing multiple files (comma-separated or multiple arguments)
go run ./cmd/stats -oe file1.csv,file2.csv
go run ./cmd/stats ~/work/lol/*.csv
```

### Statistics Provided

- **Total Matches and Players**: Aggregated count of unique games and summoners.
- **Matches per Player Distribution & Quantiles**: Min, 10th percentile, 25th percentile (Q1), Median (50th percentile), 75th percentile (Q3), 90th percentile, 95th percentile, 99th percentile, Max, and Mean.
- **Top 10 Most Active Players**: Players with the highest number of recorded matches and their PUUIDs.
- **Match Metadata & Durations**: Date range of matches, duration statistics (Min, Average, Median, Max), and Blue vs. Red side win rates.
- **Top Leagues**: Distribution of matches across competitive leagues (LPL, LCK, LEC, LCS, etc.).

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
