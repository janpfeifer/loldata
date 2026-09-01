package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/janpfeifer/loldata/data"
)

// CrawlerConfig holds configuration options for MatchCrawler.
type CrawlerConfig struct {
	StartTime          time.Time
	EndTime            time.Time
	MaxMatches         int
	CheckpointDuration time.Duration
	Verbose            bool
}

// MatchCrawler coordinates crawling matches and summoners from Riot API.
type MatchCrawler struct {
	client  *RiotClient
	dataset *data.Dataset
	store   *DatasetStore
	cfg     CrawlerConfig

	mu                  sync.Mutex
	puuidQueue          []string
	queuedPUUIDs        map[string]bool
	visitedPUUIDs       map[string]bool
	crawledMatchesCount int
}

// NewMatchCrawler initializes a new MatchCrawler.
func NewMatchCrawler(client *RiotClient, dataset *data.Dataset, store *DatasetStore, cfg CrawlerConfig) *MatchCrawler {
	crawler := &MatchCrawler{
		client:        client,
		dataset:       dataset,
		store:         store,
		cfg:           cfg,
		puuidQueue:    make([]string, 0),
		queuedPUUIDs:  make(map[string]bool),
		visitedPUUIDs: make(map[string]bool),
	}

	// Pre-populate queue from existing dataset summoners
	for _, s := range dataset.Summoners {
		if s != nil && s.PUUID != "" {
			crawler.EnqueuePUUID(s.PUUID)
		}
	}

	return crawler
}

// EnqueuePUUID adds a PUUID to the crawl queue if not already queued or visited.
func (c *MatchCrawler) EnqueuePUUID(puuid string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if puuid == "" || c.queuedPUUIDs[puuid] || c.visitedPUUIDs[puuid] {
		return
	}
	c.queuedPUUIDs[puuid] = true
	c.puuidQueue = append(c.puuidQueue, puuid)
}

// QueueSize returns the number of pending summoners in the queue.
func (c *MatchCrawler) QueueSize() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.puuidQueue)
}

// CrawledMatchesCount returns the count of newly downloaded matches in this run.
func (c *MatchCrawler) CrawledMatchesCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.crawledMatchesCount
}

// Run executes the crawler until the queue is exhausted, target max matches is reached, or ctx is canceled.
func (c *MatchCrawler) Run(ctx context.Context) error {
	// Start periodic checkpoint saver if duration > 0 and store is configured
	if c.cfg.CheckpointDuration > 0 && c.store != nil && c.store.FilePath() != "" {
		ticker := time.NewTicker(c.cfg.CheckpointDuration)
		defer ticker.Stop()

		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					c.mu.Lock()
					matches := len(c.dataset.Matches)
					summoners := len(c.dataset.Summoners)
					c.mu.Unlock()

					fmt.Printf("[Checkpoint] Saving dataset (%d matches, %d summoners) to %s ...\n",
						matches, summoners, c.store.FilePath())
					if err := c.store.Save(c.dataset); err != nil {
						fmt.Printf("[Checkpoint Error] Failed to save dataset: %v\n", err)
					} else {
						fmt.Printf("[Checkpoint] Saved successfully.\n")
					}
				}
			}
		}()
	}

	fmt.Println("----------------------------------------------------------")
	fmt.Printf("Starting Match Crawler\n")
	fmt.Printf("  Platform:        %s (regional: %s)\n", c.client.Platform(), c.client.Regional())
	if c.cfg.StartTime.IsZero() {
		fmt.Printf("  Time Window:     All historical matches -> %s\n", c.cfg.EndTime.Format(time.RFC3339))
	} else {
		fmt.Printf("  Time Window:     %s -> %s\n", c.cfg.StartTime.Format(time.RFC3339), c.cfg.EndTime.Format(time.RFC3339))
	}
	if c.cfg.MaxMatches > 0 {
		fmt.Printf("  Max Matches:     %d\n", c.cfg.MaxMatches)
	} else {
		fmt.Printf("  Max Matches:     Unlimited\n")
	}
	fmt.Printf("  Initial Queue:   %d summoners\n", c.QueueSize())
	fmt.Printf("  Initial Dataset: %d matches, %d summoners\n", len(c.dataset.Matches), len(c.dataset.Summoners))
	fmt.Println("----------------------------------------------------------")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if c.cfg.MaxMatches > 0 && c.CrawledMatchesCount() >= c.cfg.MaxMatches {
			fmt.Printf("Reached target max matches (%d). Stopping crawl.\n", c.cfg.MaxMatches)
			break
		}

		c.mu.Lock()
		if len(c.puuidQueue) == 0 {
			c.mu.Unlock()
			fmt.Println("Queue is empty. Crawl completed.")
			break
		}
		puuid := c.puuidQueue[0]
		c.puuidQueue = c.puuidQueue[1:]
		c.visitedPUUIDs[puuid] = true
		c.mu.Unlock()

		// Fetch summoner profile if not fully populated
		summoner := c.dataset.GetSummoner(puuid)
		if summoner == nil || summoner.Name == "" || summoner.SummonerLevel == 0 {
			if s, err := c.client.GetSummonerByPUUID(ctx, puuid); err == nil && s != nil {
				existing := c.dataset.GetOrCreateSummoner(puuid, s.Name)
				if s.Name != "" {
					existing.Name = s.Name
				}
				if s.SummonerLevel != 0 {
					existing.SummonerLevel = s.SummonerLevel
				}
				if s.AccountID != "" {
					existing.AccountID = s.AccountID
				}
				if s.ID != "" {
					existing.ID = s.ID
				}
				if s.ProfileIconID != 0 {
					existing.ProfileIconID = s.ProfileIconID
				}
			}
		}

		// Fetch match IDs for this summoner
		matchIDs, err := c.client.GetMatchIDsByPUUID(ctx, puuid, c.cfg.StartTime, c.cfg.EndTime, 0)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			fmt.Printf("[Error] Failed fetching match IDs for summoner %s: %v\n", puuid, err)
			continue
		}

		if len(matchIDs) == 0 {
			displayName := puuid
			if s := c.dataset.GetSummoner(puuid); s != nil && s.Name != "" {
				displayName = fmt.Sprintf("%s (%s)", s.Name, puuid)
			}
			fmt.Printf("[Summoner] %s: 0 matches found in time window\n", displayName)
		} else if c.cfg.Verbose {
			displayName := puuid
			if s := c.dataset.GetSummoner(puuid); s != nil && s.Name != "" {
				displayName = fmt.Sprintf("%s (%s)", s.Name, puuid)
			}
			fmt.Printf("[Summoner] %s found %d matches in window\n", displayName, len(matchIDs))
		}

		// Process each match
		for _, matchID := range matchIDs {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if c.cfg.MaxMatches > 0 && c.CrawledMatchesCount() >= c.cfg.MaxMatches {
				break
			}

			// Check if match is already loaded in dataset
			if c.dataset.GetMatch(matchID) != nil {
				continue
			}

			match, err := c.client.GetMatch(ctx, matchID)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				fmt.Printf("[Error] Failed fetching match %s: %v\n", matchID, err)
				continue
			}
			if match == nil {
				continue
			}

			// Add match to dataset (registers participants and bidirectional links)
			c.dataset.AddMatch(match)

			c.mu.Lock()
			c.crawledMatchesCount++
			count := c.crawledMatchesCount
			totalMatches := len(c.dataset.Matches)
			totalSummoners := len(c.dataset.Summoners)
			c.mu.Unlock()

			// Enqueue new participants
			newParticipants := 0
			for _, p := range match.Info.Participants {
				if p != nil && p.PUUID != "" {
					c.mu.Lock()
					if !c.visitedPUUIDs[p.PUUID] && !c.queuedPUUIDs[p.PUUID] {
						c.queuedPUUIDs[p.PUUID] = true
						c.puuidQueue = append(c.puuidQueue, p.PUUID)
						newParticipants++
					}
					c.mu.Unlock()
				}
			}

			matchTime := match.Time()
			duration := match.Duration()
			fmt.Printf("[+Match #%d] %s (%s, %v) | Dataset: %d matches, %d players | Queue: %d (+%d)\n",
				count, matchID, matchTime.Format("2006-01-02 15:04"), duration, totalMatches, totalSummoners, c.QueueSize(), newParticipants)
		}
	}

	return nil
}
