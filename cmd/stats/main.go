package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/janpfeifer/loldata/data"
)

var (
	oeFilesFlag   = flag.String("oe", "", "Comma-separated list of Oracle's Elixir CSV file paths or glob patterns")
	jsonFilesFlag = flag.String("json", "", "Comma-separated list of dataset JSON file paths or glob patterns")
	saveJSONFlag  = flag.String("save-json", "", "Save the loaded dataset to this JSON file")
	summonerFlag  = flag.String("summoner", "", "Summoner / player name or PUUID to output statistics for")
)

type loadTask struct {
	path   string
	isJSON bool
}

func main() {
	flag.Parse()

	var tasks []loadTask

	expandPaths := func(pattern string) []string {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			return nil
		}
		if strings.HasPrefix(pattern, "~/") {
			home, err := os.UserHomeDir()
			if err == nil {
				pattern = filepath.Join(home, pattern[2:])
			}
		}
		matches, err := filepath.Glob(pattern)
		if err == nil && len(matches) > 0 {
			return matches
		}
		return []string{pattern}
	}

	// Process -oe flag
	if *oeFilesFlag != "" {
		for _, part := range strings.Split(*oeFilesFlag, ",") {
			for _, p := range expandPaths(part) {
				tasks = append(tasks, loadTask{path: p, isJSON: false})
			}
		}
	}

	// Process -json flag
	if *jsonFilesFlag != "" {
		for _, part := range strings.Split(*jsonFilesFlag, ",") {
			for _, p := range expandPaths(part) {
				tasks = append(tasks, loadTask{path: p, isJSON: true})
			}
		}
	}

	// Also support positional arguments
	for _, arg := range flag.Args() {
		for _, p := range expandPaths(arg) {
			isJSON := strings.HasSuffix(strings.ToLower(p), ".json")
			tasks = append(tasks, loadTask{path: p, isJSON: isJSON})
		}
	}

	if len(tasks) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s [-oe <oracles_elixir.csv>] [-json <dataset.json>] [-summoner <name|puuid>] [-save-json <out.json>] [files...]\n\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	dataset := data.NewDataset()
	startTime := time.Now()

	for _, task := range tasks {
		if task.isJSON {
			fmt.Printf("Loading Dataset JSON: %s ...\n", task.path)
			if err := dataset.LoadFromJSON(task.path); err != nil {
				fmt.Fprintf(os.Stderr, "Error loading %q: %v\n", task.path, err)
				os.Exit(1)
			}
		} else {
			fmt.Printf("Loading Oracle's Elixir CSV: %s ...\n", task.path)
			if err := dataset.LoadOraclesElixir(task.path); err != nil {
				fmt.Fprintf(os.Stderr, "Error loading %q: %v\n", task.path, err)
				os.Exit(1)
			}
		}
	}

	loadDuration := time.Since(startTime)
	fmt.Printf("Loaded in %v\n\n", loadDuration)

	if *saveJSONFlag != "" {
		savePath := *saveJSONFlag
		if strings.HasPrefix(savePath, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				savePath = filepath.Join(home, savePath[2:])
			}
		}
		fmt.Printf("Saving dataset to JSON: %s ...\n", savePath)
		saveStart := time.Now()
		if err := dataset.SaveToJSON(savePath); err != nil {
			fmt.Fprintf(os.Stderr, "Error saving JSON to %q: %v\n", savePath, err)
			os.Exit(1)
		}
		fmt.Printf("Saved in %v\n\n", time.Since(saveStart))
	}

	if *summonerFlag != "" {
		summoner, candidates := dataset.FindSummoner(*summonerFlag)
		if summoner == nil {
			if len(candidates) > 1 {
				fmt.Fprintf(os.Stderr, "Error: Multiple summoners match %q:\n", *summonerFlag)
				limit := 10
				if len(candidates) < limit {
					limit = len(candidates)
				}
				for i := 0; i < limit; i++ {
					c := candidates[i]
					displayName := c.Name
					if displayName == "" {
						displayName = "<no name>"
					}
					fmt.Fprintf(os.Stderr, "  - %-24s (%d matches, PUUID: %s)\n", displayName, len(c.Matches), c.PUUID)
				}
				if len(candidates) > limit {
					fmt.Fprintf(os.Stderr, "  ... and %d more\n", len(candidates)-limit)
				}
				fmt.Fprintln(os.Stderr, "\nPlease specify the full summoner name (e.g. Name#TAG) or exact PUUID.")
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "Error: Summoner %q not found in dataset.\n", *summonerFlag)
			os.Exit(1)
		}

		stats := summoner.ComputeStats()
		ranks := computeCohortRanks(dataset, summoner, stats)
		printSummonerStats(stats, ranks)
		return
	}

	printStats(dataset)
}

// StatRank represents a player's rank and percentile within the dataset cohort.
type StatRank struct {
	Rank       int     // 1-based rank (1 is highest/best)
	TotalCount int     // Total number of players in the cohort
	Percentile float64 // Percentile rank (0.0 to 100.0)
}

// SummonerCohortRanks contains rank and percentile information across key metrics.
type SummonerCohortRanks struct {
	CohortSize   int
	TotalMatches StatRank
	WinRate      StatRank
	KDARatio     StatRank
	AvgCS        StatRank
	AvgCSPM      StatRank
	AvgGold      StatRank
	AvgGPM       StatRank
	AvgDamage    StatRank
	AvgDPM       StatRank
	AvgVision    StatRank
	AvgVSPM      StatRank
}

// calculateRank computes 1-based rank and percentile position of targetVal within vals.
func calculateRank(vals []float64, targetVal float64) StatRank {
	n := len(vals)
	if n == 0 {
		return StatRank{Rank: 1, TotalCount: 1, Percentile: 100.0}
	}

	strictlyHigher := 0
	strictlyLower := 0
	equalCount := 0

	for _, v := range vals {
		if v > targetVal {
			strictlyHigher++
		} else if v < targetVal {
			strictlyLower++
		} else {
			equalCount++
		}
	}

	rank := 1 + strictlyHigher

	var pct float64
	if n == 1 {
		pct = 100.0
	} else {
		pos := float64(strictlyLower) + float64(equalCount-1)/2.0
		pct = (pos / float64(n-1)) * 100.0
	}

	return StatRank{
		Rank:       rank,
		TotalCount: n,
		Percentile: pct,
	}
}

// computeCohortRanks computes rankings and percentiles for a summoner relative to the cohort in the dataset.
func computeCohortRanks(ds *data.Dataset, targetSummoner *data.SummonerV4, targetStats *data.SummonerStats) *SummonerCohortRanks {
	if ds == nil || targetSummoner == nil || targetStats == nil {
		return nil
	}

	var cohort []*data.SummonerV4
	for _, s := range ds.Summoners {
		if s != nil && s.Crawled && len(s.Matches) > 0 {
			cohort = append(cohort, s)
		}
	}
	if len(cohort) == 0 {
		for _, s := range ds.Summoners {
			if s != nil && len(s.Matches) > 0 {
				cohort = append(cohort, s)
			}
		}
	}

	hasTarget := false
	for _, s := range cohort {
		if s == targetSummoner || (s.PUUID != "" && s.PUUID == targetSummoner.PUUID) {
			hasTarget = true
			break
		}
	}
	if !hasTarget && len(targetSummoner.Matches) > 0 {
		cohort = append(cohort, targetSummoner)
	}

	if len(cohort) == 0 {
		return nil
	}

	cohortStats := make([]*data.SummonerStats, 0, len(cohort))
	for _, s := range cohort {
		if s == targetSummoner || (s.PUUID != "" && s.PUUID == targetSummoner.PUUID) {
			cohortStats = append(cohortStats, targetStats)
		} else {
			cs := s.ComputeStats()
			if cs != nil && cs.TotalMatches > 0 {
				cohortStats = append(cohortStats, cs)
			}
		}
	}

	if len(cohortStats) == 0 {
		return nil
	}

	n := len(cohortStats)
	matchesVals := make([]float64, n)
	winRateVals := make([]float64, n)
	kdaVals := make([]float64, n)
	csVals := make([]float64, n)
	cspmVals := make([]float64, n)
	goldVals := make([]float64, n)
	gpmVals := make([]float64, n)
	dmgVals := make([]float64, n)
	dpmVals := make([]float64, n)
	visionVals := make([]float64, n)
	vspmVals := make([]float64, n)

	for i, cs := range cohortStats {
		matchesVals[i] = float64(cs.TotalMatches)
		winRateVals[i] = cs.WinRate
		kdaVals[i] = cs.KDARatio
		csVals[i] = cs.AvgCS
		cspmVals[i] = cs.AvgCSPM
		goldVals[i] = cs.AvgGold
		gpmVals[i] = cs.AvgGPM
		dmgVals[i] = cs.AvgDamageToChampions
		dpmVals[i] = cs.AvgDPM
		visionVals[i] = cs.AvgVisionScore
		vspmVals[i] = cs.AvgVSPM
	}

	return &SummonerCohortRanks{
		CohortSize:   n,
		TotalMatches: calculateRank(matchesVals, float64(targetStats.TotalMatches)),
		WinRate:      calculateRank(winRateVals, targetStats.WinRate),
		KDARatio:     calculateRank(kdaVals, targetStats.KDARatio),
		AvgCS:        calculateRank(csVals, targetStats.AvgCS),
		AvgCSPM:      calculateRank(cspmVals, targetStats.AvgCSPM),
		AvgGold:      calculateRank(goldVals, targetStats.AvgGold),
		AvgGPM:       calculateRank(gpmVals, targetStats.AvgGPM),
		AvgDamage:    calculateRank(dmgVals, targetStats.AvgDamageToChampions),
		AvgDPM:       calculateRank(dpmVals, targetStats.AvgDPM),
		AvgVision:    calculateRank(visionVals, targetStats.AvgVisionScore),
		AvgVSPM:      calculateRank(vspmVals, targetStats.AvgVSPM),
	}
}

func printSummonerStats(stats *data.SummonerStats, ranks *SummonerCohortRanks) {
	fmt.Println("================================================================================")
	displayName := stats.Summoner.Name
	if displayName == "" {
		displayName = stats.Summoner.PUUID
	}
	fmt.Printf("             SUMMONER STATS: %s\n", displayName)
	fmt.Println("================================================================================")
	if stats.Summoner.Name != "" && stats.Summoner.Name != stats.Summoner.PUUID {
		fmt.Printf("Summoner:      %s\n", stats.Summoner.Name)
	}
	fmt.Printf("PUUID:         %s\n", stats.Summoner.PUUID)
	if stats.Summoner.SummonerLevel > 0 {
		fmt.Printf("Level:         %d\n", stats.Summoner.SummonerLevel)
	}
	if !stats.EarliestMatch.IsZero() && !stats.LatestMatch.IsZero() {
		fmt.Printf("Date Range:    %s -> %s\n", stats.EarliestMatch.Format("2006-01-02"), stats.LatestMatch.Format("2006-01-02"))
	}
	if stats.AvgDuration > 0 {
		fmt.Printf("Avg Duration:  %s\n", stats.AvgDuration.Round(time.Second))
	}
	if ranks != nil && ranks.CohortSize > 1 {
		fmt.Printf("Cohort Size:   %d players\n", ranks.CohortSize)
	}
	fmt.Println()

	if stats.TotalMatches == 0 {
		fmt.Println("No matches recorded for this summoner in the dataset.")
		fmt.Println("================================================================================")
		return
	}

	// 1. Overall Performance & Combat
	fmt.Println("--------------------------------------------------------------------------------")
	fmt.Println("OVERALL PERFORMANCE & WIN RATIO:")
	if ranks != nil && ranks.CohortSize > 1 {
		fmt.Printf("  Total Matches:     %d (%.1f %%-tile, #%d of %d)\n",
			stats.TotalMatches, ranks.TotalMatches.Percentile, ranks.TotalMatches.Rank, ranks.CohortSize)
		fmt.Printf("  Record:            %dW - %dL (%.1f%% Win Rate, %.1f %%-tile)\n",
			stats.Wins, stats.Losses, stats.WinRate, ranks.WinRate.Percentile)
	} else {
		fmt.Printf("  Total Matches:     %d\n", stats.TotalMatches)
		fmt.Printf("  Record:            %dW - %dL (%.1f%% Win Rate)\n", stats.Wins, stats.Losses, stats.WinRate)
	}
	if stats.BlueGames > 0 || stats.RedGames > 0 {
		fmt.Printf("  Side Win Rates:    Blue: %.1f%% (%dW-%dL in %dG) | Red: %.1f%% (%dW-%dL in %dG)\n",
			stats.BlueWinRate, stats.BlueWins, stats.BlueGames-stats.BlueWins, stats.BlueGames,
			stats.RedWinRate, stats.RedWins, stats.RedGames-stats.RedWins, stats.RedGames)
	}
	fmt.Println()

	fmt.Println("COMBAT & KDA:")
	kdaRatioStr := fmt.Sprintf("%.2f:1 Ratio", stats.KDARatio)
	if stats.PerfectKDA {
		kdaRatioStr = "Perfect KDA"
	}
	if ranks != nil && ranks.CohortSize > 1 {
		kdaRatioStr = fmt.Sprintf("%s, %.1f %%-tile", kdaRatioStr, ranks.KDARatio.Percentile)
	}
	fmt.Printf("  Average KDA:       %.1f / %.1f / %.1f (%s)\n", stats.AvgKills, stats.AvgDeaths, stats.AvgAssists, kdaRatioStr)
	if stats.AvgKillParticipation > 0 {
		fmt.Printf("  Kill Participation: %.1f%%\n", stats.AvgKillParticipation)
	}
	fmt.Printf("  Multikills:        Double: %d | Triple: %d | Quadra: %d | Penta: %d\n",
		stats.DoubleKills, stats.TripleKills, stats.QuadraKills, stats.PentaKills)
	if stats.FirstBloodKills > 0 || stats.FirstBloodAssists > 0 || stats.FirstBloodVictims > 0 {
		fmt.Printf("  First Blood:       Kills: %d (%.1f%%) | Assists: %d (%.1f%%) | Victims: %d (%.1f%%)\n",
			stats.FirstBloodKills, float64(stats.FirstBloodKills)/float64(stats.TotalMatches)*100,
			stats.FirstBloodAssists, float64(stats.FirstBloodAssists)/float64(stats.TotalMatches)*100,
			stats.FirstBloodVictims, float64(stats.FirstBloodVictims)/float64(stats.TotalMatches)*100)
	}
	if stats.FirstTowerKills > 0 || stats.FirstTowerAssists > 0 {
		fmt.Printf("  First Tower:       Kills: %d (%.1f%%) | Assists: %d (%.1f%%)\n",
			stats.FirstTowerKills, float64(stats.FirstTowerKills)/float64(stats.TotalMatches)*100,
			stats.FirstTowerAssists, float64(stats.FirstTowerAssists)/float64(stats.TotalMatches)*100)
	}
	fmt.Println()

	// 2. Farming, Economy, Damage & Vision
	fmt.Println("FARMING, ECONOMY & DAMAGE:")
	if ranks != nil && ranks.CohortSize > 1 {
		fmt.Printf("  Average CS:        %.1f (%.1f %%-tile) (%.1f CS/min, %.1f %%-tile)\n",
			stats.AvgCS, ranks.AvgCS.Percentile, stats.AvgCSPM, ranks.AvgCSPM.Percentile)
		fmt.Printf("  Average Gold:      %s (%.1f %%-tile) (%.1f Gold/min, %.1f %%-tile)\n",
			formatNumber(int(stats.AvgGold)), ranks.AvgGold.Percentile, stats.AvgGPM, ranks.AvgGPM.Percentile)
		fmt.Printf("  Avg Damage (Champs): %s (%.1f %%-tile) (%.1f DPM, %.1f %%-tile)\n",
			formatNumber(int(stats.AvgDamageToChampions)), ranks.AvgDamage.Percentile, stats.AvgDPM, ranks.AvgDPM.Percentile)
	} else {
		fmt.Printf("  Average CS:        %.1f (%.1f CS/min)\n", stats.AvgCS, stats.AvgCSPM)
		fmt.Printf("  Average Gold:      %s (%.1f Gold/min)\n", formatNumber(int(stats.AvgGold)), stats.AvgGPM)
		fmt.Printf("  Avg Damage (Champs): %s (%.1f DPM)\n", formatNumber(int(stats.AvgDamageToChampions)), stats.AvgDPM)
	}
	if stats.AvgDamageShare > 0 {
		fmt.Printf("  Avg Damage Share:  %.1f%%\n", stats.AvgDamageShare*100)
	}
	fmt.Printf("  Avg Damage Taken:  %s\n", formatNumber(int(stats.AvgDamageTaken)))
	if stats.AvgDamageMitigated > 0 {
		fmt.Printf("  Avg Mitigated:     %s\n", formatNumber(int(stats.AvgDamageMitigated)))
	}
	if stats.AvgDamageToTurrets > 0 {
		fmt.Printf("  Avg Turret Damage: %s\n", formatNumber(int(stats.AvgDamageToTurrets)))
	}
	fmt.Println()

	fmt.Println("VISION:")
	if ranks != nil && ranks.CohortSize > 1 {
		fmt.Printf("  Avg Vision Score:  %.1f (%.1f %%-tile) (%.2f VSPM, %.1f %%-tile)\n",
			stats.AvgVisionScore, ranks.AvgVision.Percentile, stats.AvgVSPM, ranks.AvgVSPM.Percentile)
	} else {
		fmt.Printf("  Avg Vision Score:  %.1f (%.2f VSPM)\n", stats.AvgVisionScore, stats.AvgVSPM)
	}
	fmt.Printf("  Avg Wards:         Placed: %.1f | Cleared: %.1f | Control: %.1f\n",
		stats.AvgWardsPlaced, stats.AvgWardsKilled, stats.AvgControlWards)
	fmt.Println()

	// 3. Position / Role Distribution
	if len(stats.Positions) > 0 {
		fmt.Println("--------------------------------------------------------------------------------")
		mostPlayedPos := stats.Positions[0]
		fmt.Printf("POSITION / ROLE DISTRIBUTION (Most Played: %s - %.1f%% of matches):\n",
			mostPlayedPos.PositionName, mostPlayedPos.PercentOfTotal)
		fmt.Println("  Position         Games   Share      Record       Win Rate   Avg KDA           CSPM    DPM")
		fmt.Println("  --------------------------------------------------------------------------------")
		for _, p := range stats.Positions {
			kdaStr := fmt.Sprintf("%.1f/%.1f/%.1f", p.AvgKills, p.AvgDeaths, p.AvgAssists)
			recStr := fmt.Sprintf("%dW - %dL", p.Wins, p.Losses)
			fmt.Printf("  %-16s %4d   %5.1f%%   %10s    %5.1f%%    %-15s %5.1f  %6.1f\n",
				p.PositionName, p.Games, p.PercentOfTotal, recStr, p.WinRate, kdaStr, p.AvgCSPM, p.AvgDPM)
		}
		fmt.Println()
	}

	// 4. Champions Played Distribution
	if len(stats.Champions) > 0 {
		fmt.Println("--------------------------------------------------------------------------------")
		mostPlayedChamp := stats.Champions[0]
		fmt.Printf("CHAMPIONS PLAYED DISTRIBUTION (%d unique champions | Most Played: %s with %.1f%% share):\n",
			len(stats.Champions), mostPlayedChamp.ChampionName, mostPlayedChamp.PercentOfTotal)
		fmt.Println("  Champion         Games   Share      Record       Win Rate   Avg KDA           CSPM    DPM")
		fmt.Println("  --------------------------------------------------------------------------------")
		for _, c := range stats.Champions {
			kdaStr := fmt.Sprintf("%.1f/%.1f/%.1f", c.AvgKills, c.AvgDeaths, c.AvgAssists)
			recStr := fmt.Sprintf("%dW - %dL", c.Wins, c.Losses)
			fmt.Printf("  %-16s %4d   %5.1f%%   %10s    %5.1f%%    %-15s %5.1f  %6.1f\n",
				c.ChampionName, c.Games, c.PercentOfTotal, recStr, c.WinRate, kdaStr, c.AvgCSPM, c.AvgDPM)
		}
		fmt.Println()
	}

	// 5. Most Frequent Teammates / Duo Partners
	if len(stats.Teammates) > 0 {
		fmt.Println("--------------------------------------------------------------------------------")
		mostPlayedTeammate := stats.Teammates[0]
		fmt.Printf("MOST FREQUENT TEAMMATES / DUO PARTNERS (Most Played With: %s - %.1f%% of matches):\n",
			mostPlayedTeammate.Name, mostPlayedTeammate.PercentOfTotal)
		fmt.Println("  Teammate                     Games   Share      Record       Win Rate")
		fmt.Println("  --------------------------------------------------------------------------------")
		limit := 10
		if len(stats.Teammates) < limit {
			limit = len(stats.Teammates)
		}
		for i := 0; i < limit; i++ {
			tm := stats.Teammates[i]
			recStr := fmt.Sprintf("%dW - %dL", tm.Wins, tm.Losses)
			fmt.Printf("  %-28s %4d   %5.1f%%   %10s    %5.1f%%\n",
				tm.Name, tm.Games, tm.PercentOfTotal, recStr, tm.WinRate)
		}
		fmt.Println()
	}

	// 6. Most Frequent Opponents
	if len(stats.Opponents) > 0 {
		fmt.Println("--------------------------------------------------------------------------------")
		fmt.Println("MOST FREQUENT OPPONENTS:")
		fmt.Println("  Opponent                     Games   Share     Record vs     Win Rate vs")
		fmt.Println("  --------------------------------------------------------------------------------")
		limit := 5
		if len(stats.Opponents) < limit {
			limit = len(stats.Opponents)
		}
		for i := 0; i < limit; i++ {
			op := stats.Opponents[i]
			recStr := fmt.Sprintf("%dW - %dL", op.Wins, op.Losses)
			fmt.Printf("  %-28s %4d   %5.1f%%   %10s    %5.1f%%\n",
				op.Name, op.Games, op.PercentOfTotal, recStr, op.WinRate)
		}
		fmt.Println()
	}

	// 7. Game Modes & Queues
	if len(stats.Queues) > 0 {
		fmt.Println("--------------------------------------------------------------------------------")
		fmt.Println("GAME MODES & QUEUES:")
		fmt.Println("  Queue                        Games   Share      Record       Win Rate")
		fmt.Println("  --------------------------------------------------------------------------------")
		for _, q := range stats.Queues {
			recStr := fmt.Sprintf("%dW - %dL", q.Wins, q.Losses)
			fmt.Printf("  %-28s %4d   %5.1f%%   %10s    %5.1f%%\n",
				q.Description, q.Games, q.PercentOfTotal, recStr, q.WinRate)
		}
		fmt.Println()
	}

	// 8. Recent Match History
	if len(stats.Recent) > 0 {
		fmt.Println("--------------------------------------------------------------------------------")
		fmt.Println("RECENT MATCH HISTORY (Newest First):")
		fmt.Println("  Date        Result  Champion        Position   KDA          Duration  Match ID")
		fmt.Println("  --------------------------------------------------------------------------------")
		count := 0
		for i := len(stats.Recent) - 1; i >= 0 && count < 10; i-- {
			r := stats.Recent[i]
			count++
			dateStr := "-"
			if !r.Time.IsZero() {
				dateStr = r.Time.Format("2006-01-02")
			}
			resStr := "LOSS"
			if r.Win {
				resStr = "WIN "
			}
			posStr := r.Position.String()
			if r.Position == data.PositionBot {
				posStr = "Bot"
			}
			kdaStr := fmt.Sprintf("%d/%d/%d", r.Kills, r.Deaths, r.Assists)
			durStr := fmt.Sprintf("%dm%02ds", int(r.Duration.Minutes()), int(r.Duration.Seconds())%60)
			fmt.Printf("  %-10s  %-4s    %-15s %-10s %-12s %-8s  %s\n",
				dateStr, resStr, r.ChampionName, posStr, kdaStr, durStr, r.MatchID)
		}
		fmt.Println()
	}

	fmt.Println("================================================================================")
}

func formatNumber(n int) string {
	if n < 0 {
		return "-" + formatNumber(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var result []byte
	rem := len(s) % 3
	if rem > 0 {
		result = append(result, s[:rem]...)
	}
	for i := rem; i < len(s); i += 3 {
		if len(result) > 0 {
			result = append(result, ',')
		}
		result = append(result, s[i:i+3]...)
	}
	return string(result)
}

func printStats(ds *data.Dataset) {
	numMatches := len(ds.Matches)
	numTotalSummoners := len(ds.Summoners)

	var crawledSummoners []*data.SummonerV4
	for _, s := range ds.Summoners {
		if s != nil && s.Crawled {
			crawledSummoners = append(crawledSummoners, s)
		}
	}
	numCrawled := len(crawledSummoners)

	fmt.Println("==========================================================")
	fmt.Println("             LEAGUE OF LEGENDS DATASET STATS              ")
	fmt.Println("==========================================================")
	fmt.Printf("Total Matches:   %d\n", numMatches)
	if numCrawled == numTotalSummoners {
		fmt.Printf("Total Players:   %d (all crawled)\n\n", numTotalSummoners)
	} else {
		fmt.Printf("Total Players:   %d (%d crawled, %d uncrawled skipped)\n\n",
			numTotalSummoners, numCrawled, numTotalSummoners-numCrawled)
	}

	if numMatches == 0 || numCrawled == 0 {
		return
	}

	// 1. Matches per Player Quantiles & Distribution (crawled summoners only)
	matchCounts := make([]int, numCrawled)
	totalPlayerMatches := 0
	type playerActivity struct {
		name    string
		puuid   string
		matches int
	}
	topPlayers := make([]playerActivity, numCrawled)

	for i, s := range crawledSummoners {
		count := len(s.Matches)
		matchCounts[i] = count
		totalPlayerMatches += count
		topPlayers[i] = playerActivity{
			name:    s.Name,
			puuid:   s.PUUID,
			matches: count,
		}
	}

	sort.Ints(matchCounts)
	sort.Slice(topPlayers, func(i, j int) bool {
		return topPlayers[i].matches > topPlayers[j].matches
	})

	meanMatches := float64(totalPlayerMatches) / float64(numCrawled)

	fmt.Println("----------------------------------------------------------")
	fmt.Println("Matches per Player Distribution & Quantiles:")
	fmt.Printf("  Min:   %d\n", matchCounts[0])
	fmt.Printf("  p10:   %.1f\n", quantile(matchCounts, 0.10))
	fmt.Printf("  p25:   %.1f\n", quantile(matchCounts, 0.25))
	fmt.Printf("  p50:   %.1f (Median)\n", quantile(matchCounts, 0.50))
	fmt.Printf("  p75:   %.1f\n", quantile(matchCounts, 0.75))
	fmt.Printf("  p90:   %.1f\n", quantile(matchCounts, 0.90))
	fmt.Printf("  p95:   %.1f\n", quantile(matchCounts, 0.95))
	fmt.Printf("  p99:   %.1f\n", quantile(matchCounts, 0.99))
	fmt.Printf("  Max:   %d\n", matchCounts[len(matchCounts)-1])
	fmt.Printf("  Mean:  %.2f\n\n", meanMatches)

	// Top 10 Most Active Players
	fmt.Println("Top 10 Most Active Players:")
	limit := 10
	if len(topPlayers) < limit {
		limit = len(topPlayers)
	}
	for i := 0; i < limit; i++ {
		p := topPlayers[i]
		displayName := p.name
		if displayName == "" {
			displayName = p.puuid
		}
		fmt.Printf("  %2d. %-24s (%s) : %d matches\n", i+1, displayName, p.puuid, p.matches)
	}
	fmt.Println()

	// 2. Match metadata summaries: Date range, leagues, durations, side win rates
	var earliestDate, latestDate time.Time
	var totalDurationSec int64
	durations := make([]float64, 0, numMatches)
	leagueCounts := make(map[string]int)
	blueWins := 0
	redWins := 0

	for _, m := range ds.Matches {
		t := m.Time()
		if !t.IsZero() {
			if earliestDate.IsZero() || t.Before(earliestDate) {
				earliestDate = t
			}
			if latestDate.IsZero() || t.After(latestDate) {
				latestDate = t
			}
		}

		durationSec := m.Info.GameDuration
		if durationSec > 0 {
			totalDurationSec += durationSec
			durations = append(durations, float64(durationSec))
		}

		if m.Esports != nil && m.Esports.League != "" {
			leagueCounts[m.Esports.League]++
		}

		blue := m.GetTeamBySide(data.SideBlue)
		if blue != nil && blue.Win {
			blueWins++
		}
		red := m.GetTeamBySide(data.SideRed)
		if red != nil && red.Win {
			redWins++
		}
	}

	fmt.Println("----------------------------------------------------------")
	fmt.Println("Match Metadata & Durations:")
	if !earliestDate.IsZero() && !latestDate.IsZero() {
		fmt.Printf("  Date Range:      %s -> %s\n", earliestDate.Format("2006-01-02"), latestDate.Format("2006-01-02"))
	}
	if len(durations) > 0 {
		sort.Float64s(durations)
		avgDur := time.Duration(float64(totalDurationSec)/float64(len(durations))) * time.Second
		minDur := time.Duration(durations[0]) * time.Second
		medDur := time.Duration(durations[len(durations)/2]) * time.Second
		maxDur := time.Duration(durations[len(durations)-1]) * time.Second
		fmt.Printf("  Average Duration: %s (Min: %s, Median: %s, Max: %s)\n", avgDur, minDur, medDur, maxDur)
	}

	if blueWins+redWins > 0 {
		totalDecided := blueWins + redWins
		blueWR := float64(blueWins) / float64(totalDecided) * 100.0
		redWR := float64(redWins) / float64(totalDecided) * 100.0
		fmt.Printf("  Side Win Rates:   Blue: %.1f%% (%d wins) | Red: %.1f%% (%d wins)\n", blueWR, blueWins, redWR, redWins)
	}
	fmt.Println()

	// 3. Top Leagues by Matches
	if len(leagueCounts) > 0 {
		type leagueStat struct {
			league  string
			matches int
		}
		leagues := make([]leagueStat, 0, len(leagueCounts))
		for l, c := range leagueCounts {
			leagues = append(leagues, leagueStat{league: l, matches: c})
		}
		sort.Slice(leagues, func(i, j int) bool {
			return leagues[i].matches > leagues[j].matches
		})

		fmt.Println("Top Leagues by Match Count:")
		limitL := 10
		if len(leagues) < limitL {
			limitL = len(leagues)
		}
		for i := 0; i < limitL; i++ {
			fmt.Printf("  %2d. %-15s : %d matches\n", i+1, leagues[i].league, leagues[i].matches)
		}
		fmt.Println()
	}
	fmt.Println("==========================================================")
}

// quantile calculates the linear interpolation quantile value from a sorted slice.
func quantile(sortedVals []int, q float64) float64 {
	if len(sortedVals) == 0 {
		return 0
	}
	if q <= 0 || len(sortedVals) == 1 {
		return float64(sortedVals[0])
	}
	if q >= 1 {
		return float64(sortedVals[len(sortedVals)-1])
	}

	pos := q * float64(len(sortedVals)-1)
	idx := int(math.Floor(pos))
	fraction := pos - float64(idx)

	if idx+1 < len(sortedVals) {
		return float64(sortedVals[idx]) + fraction*float64(sortedVals[idx+1]-sortedVals[idx])
	}
	return float64(sortedVals[idx])
}
