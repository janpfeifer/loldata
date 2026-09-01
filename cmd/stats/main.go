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
	oeFilesFlag = flag.String("oe", "", "Comma-separated list of Oracle's Elixir CSV file paths or glob patterns")
)

func main() {
	flag.Parse()

	var filePaths []string

	// Process -oe flag
	if *oeFilesFlag != "" {
		for _, part := range strings.Split(*oeFilesFlag, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			// Handle ~ expansion if needed
			if strings.HasPrefix(part, "~/") {
				home, err := os.UserHomeDir()
				if err == nil {
					part = filepath.Join(home, part[2:])
				}
			}
			matches, err := filepath.Glob(part)
			if err == nil && len(matches) > 0 {
				filePaths = append(filePaths, matches...)
			} else {
				filePaths = append(filePaths, part)
			}
		}
	}

	// Also support positional arguments
	for _, arg := range flag.Args() {
		arg = strings.TrimSpace(arg)
		if arg == "" {
			continue
		}
		if strings.HasPrefix(arg, "~/") {
			home, err := os.UserHomeDir()
			if err == nil {
				arg = filepath.Join(home, arg[2:])
			}
		}
		matches, err := filepath.Glob(arg)
		if err == nil && len(matches) > 0 {
			filePaths = append(filePaths, matches...)
		} else {
			filePaths = append(filePaths, arg)
		}
	}

	if len(filePaths) == 0 {
		fmt.Fprintf(os.Stderr, "Usage: %s -oe <oracles_elixir.csv[,file2.csv,...]>\n\n", os.Args[0])
		flag.PrintDefaults()
		os.Exit(1)
	}

	dataset := data.NewDataset()
	startTime := time.Now()

	for _, path := range filePaths {
		fmt.Printf("Loading Oracle's Elixir CSV: %s ...\n", path)
		if err := dataset.LoadOraclesElixir(path); err != nil {
			fmt.Fprintf(os.Stderr, "Error loading %q: %v\n", path, err)
			os.Exit(1)
		}
	}

	loadDuration := time.Since(startTime)
	fmt.Printf("Loaded in %v\n\n", loadDuration)

	printStats(dataset)
}

func printStats(ds *data.Dataset) {
	numMatches := len(ds.Matches)
	numSummoners := len(ds.Summoners)

	fmt.Println("==========================================================")
	fmt.Println("             LEAGUE OF LEGENDS DATASET STATS              ")
	fmt.Println("==========================================================")
	fmt.Printf("Total Matches:   %d\n", numMatches)
	fmt.Printf("Total Players:   %d\n\n", numSummoners)

	if numMatches == 0 || numSummoners == 0 {
		return
	}

	// 1. Matches per Player Quantiles & Distribution
	matchCounts := make([]int, len(ds.Summoners))
	totalPlayerMatches := 0
	type playerActivity struct {
		name    string
		puuid   string
		matches int
	}
	topPlayers := make([]playerActivity, len(ds.Summoners))

	for i, s := range ds.Summoners {
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

	meanMatches := float64(totalPlayerMatches) / float64(numSummoners)

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
