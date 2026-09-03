package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/janpfeifer/loldata/data"
)

var (
	datasetFlag       = flag.String("dataset", "", "Path to dataset JSON file to read at startup and save during checkpoints / exit")
	checkpointsFlag   = flag.Duration("checkpoints", 10*time.Minute, "Dataset checkpoint save interval (e.g. 10m, 1h)")
	backupsFlag       = flag.Int("backups", 3, "Number of rotating backup files to maintain (<dataset>.backup-YYYYMMDDhhmmss)")
	startTimeFlag     = flag.String("start_time", "1w", "Download matches from this time forward (e.g. '1w', '7d', '2026-08-01', unix timestamp)")
	endTimeFlag       = flag.String("end_time", "", "Download matches up to this time (defaults to program execution time)")
	limitReqsPerSec   = flag.Int("limit_reqs_persec", 19, "Maximum requests allowed per 1 second window")
	limitReqsPer2Min  = flag.Int("limit_reqs_per2min", 99, "Maximum requests allowed per 2 minute window")
	seedFlag          = flag.String("seed", "", "Summoner name or Riot ID (e.g. 'Faker#KR1' or 'SummonerName') to seed the crawler")
	apiKeyFlag        = flag.String("api_key", "", "Riot API Key (defaults to RIOT_API_KEY or RIOT_TOKEN environment variable)")
	platformFlag      = flag.String("platform", "na1", "Riot platform routing (e.g. 'na1', 'euw1', 'kr', 'br1', etc.)")
	maxMatchesFlag    = flag.Int("max_matches", 0, "Maximum number of new matches to crawl (0 for unlimited)")
	refreshFlag        = flag.Bool("refresh", false, "Mark all loaded summoners as uncrawled at startup to re-crawl their match histories")
	clearNonRankedFlag = flag.Bool("clear_non_ranked", false, "Remove non-ranked matches and orphaned summoners with no matches from dataset")
	verboseFlag        = flag.Bool("verbose", false, "Enable verbose debug output")
)

func main() {
	flag.Parse()

	executionTime := time.Now().UTC()

	// 1. Resolve Riot API key
	apiKey := strings.TrimSpace(*apiKeyFlag)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("RIOT_API_KEY"))
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("RIOT_TOKEN"))
	}
	if apiKey == "" {
		fmt.Fprintln(os.Stderr, "Error: Riot API Key is required. Set via -api_key flag or RIOT_API_KEY environment variable.")
		fmt.Fprintln(os.Stderr, "Get an API key from: https://developer.riotgames.com/")
		os.Exit(1)
	}

	// 2. Parse time window
	endTime, err := ParseTime(*endTimeFlag, executionTime, executionTime)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing -end_time: %v\n", err)
		os.Exit(1)
	}

	defaultStartTime := endTime.Add(-7 * 24 * time.Hour)
	startTime, err := ParseTime(*startTimeFlag, endTime, defaultStartTime)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing -start_time: %v\n", err)
		os.Exit(1)
	}

	if startTime.After(endTime) {
		fmt.Fprintf(os.Stderr, "Error: -start_time (%v) cannot be after -end_time (%v)\n", startTime, endTime)
		os.Exit(1)
	}

	// Expand dataset path if starting with ~/
	datasetPath := strings.TrimSpace(*datasetFlag)
	if strings.HasPrefix(datasetPath, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			datasetPath = filepath.Join(home, datasetPath[2:])
		}
	}

	// 3. Initialize Throttler with multiple rate limit rules
	throttler := NewThrottler(
		LimitRule{
			MaxRequests: *limitReqsPerSec,
			Window:      1 * time.Second,
			Name:        "1-second limit",
		},
		LimitRule{
			MaxRequests: *limitReqsPer2Min,
			Window:      120 * time.Second,
			Name:        "2-minute limit",
		},
	)

	// 4. Initialize Riot API client
	client := NewRiotClient(apiKey, *platformFlag, throttler, *verboseFlag)

	// 5. Initialize Dataset & Store
	dataset := data.NewDataset()
	var store *DatasetStore
	if datasetPath != "" {
		store = NewDatasetStore(datasetPath, *backupsFlag)
		defer store.Cleanup()
		if store.Exists() {
			fmt.Printf("Loading existing dataset from %s ...\n", datasetPath)
			if err := store.Load(dataset); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: failed to load existing dataset %q: %v\n", datasetPath, err)
			} else {
				fmt.Printf("Loaded %d matches and %d players (%d crawled).\n", len(dataset.Matches), len(dataset.Summoners), dataset.NumCrawledSummoners())
			}
		}
	}

	// 6. Context with signal cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigChan
		fmt.Printf("\nReceived signal (%v). Gracefully shutting down...\n", sig)
		cancel()
		// Second signal forces immediate exit
		sig2 := <-sigChan
		fmt.Printf("\nReceived second signal (%v). Forcefully exiting...\n", sig2)
		if store != nil {
			store.Cleanup()
		}
		os.Exit(1)
	}()

	// 7. Resolve seed if provided
	var seedPUUID string
	if *seedFlag != "" {
		fmt.Printf("Resolving seed summoner %q on platform %s ...\n", *seedFlag, *platformFlag)
		var err error
		seedPUUID, err = client.ResolveSeedToPUUID(ctx, *seedFlag)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error resolving seed %q: %v\n", *seedFlag, err)
			if store != nil {
				store.Cleanup()
			}
			os.Exit(1)
		}
		fmt.Printf("Resolved seed %q -> PUUID: %s\n", *seedFlag, seedPUUID)
	}

	// 8. Setup Crawler
	crawlerCfg := CrawlerConfig{
		StartTime:          startTime,
		EndTime:            endTime,
		MaxMatches:         *maxMatchesFlag,
		CheckpointDuration: *checkpointsFlag,
		Verbose:            *verboseFlag,
		SeedPUUID:          seedPUUID,
		Refresh:            *refreshFlag,
		ClearNonRanked:     *clearNonRankedFlag,
	}
	if *clearNonRankedFlag {
		removedMatches, removedSummoners := dataset.ClearNonRanked()
		fmt.Printf("Cleared %d non-ranked matches and %d orphaned summoners with no matches from dataset (remaining: %d matches, %d players).\n",
			removedMatches, removedSummoners, len(dataset.Matches), len(dataset.Summoners))
	}
	crawler := NewMatchCrawler(client, dataset, store, crawlerCfg)

	if crawler.QueueSize() == 0 && len(dataset.Matches) == 0 && len(dataset.Summoners) == 0 {
		if *clearNonRankedFlag && store != nil && store.FilePath() != "" {
			if !dataset.Saved {
				fmt.Printf("Saving cleared dataset to %s ...\n", store.FilePath())
				if err := store.Save(dataset); err != nil {
					fmt.Fprintf(os.Stderr, "Error saving dataset: %v\n", err)
				}
			}
			return
		}
		fmt.Fprintln(os.Stderr, "Error: no seed provided (-seed) and dataset is empty. Provide at least one seed summoner.")
		flag.PrintDefaults()
		if store != nil {
			store.Cleanup()
		}
		os.Exit(1)
	}

	// 9. Run crawler
	crawlErr := crawler.Run(ctx)
	if crawlErr != nil && crawlErr != context.Canceled {
		fmt.Fprintf(os.Stderr, "Crawler encountered error: %v\n", crawlErr)
	}

	// 10. Save dataset at exit
	if store != nil && store.FilePath() != "" {
		fmt.Println("----------------------------------------------------------")
		if !dataset.Saved {
			fmt.Printf("Saving final dataset (%d matches, %d players (%d crawled, %d with profile)) to %s ...\n",
				len(dataset.Matches), len(dataset.Summoners), dataset.NumCrawledSummoners(), dataset.NumSummonersWithProfile(), store.FilePath())
			if err := store.Save(dataset); err != nil {
				fmt.Fprintf(os.Stderr, "Error saving dataset: %v\n", err)
				store.Cleanup()
				os.Exit(1)
			}
			fmt.Printf("Dataset saved successfully.\n")
		} else {
			fmt.Printf("Dataset is already up to date on disk (%s). No changes to save.\n", store.FilePath())
		}
	}

	fmt.Println("==========================================================")
	fmt.Printf("Crawler finished. Total matches in dataset: %d, players: %d (%d crawled, %d with profile) (new matches downloaded: %d)\n",
		len(dataset.Matches), len(dataset.Summoners), dataset.NumCrawledSummoners(), dataset.NumSummonersWithProfile(), crawler.CrawledMatchesCount())
	fmt.Println("==========================================================")
}
