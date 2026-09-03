package main

import (
	"context"
	"fmt"
	"strings"
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
	SeedPUUID          string
	Refresh            bool
	ClearNonRanked     bool
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
	// If ClearNonRanked is enabled, remove non-ranked matches and orphaned summoners before queue initialization.
	if cfg.ClearNonRanked {
		dataset.ClearNonRanked()
	}

	crawler := &MatchCrawler{
		client:        client,
		dataset:       dataset,
		store:         store,
		cfg:           cfg,
		puuidQueue:    make([]string, 0),
		queuedPUUIDs:  make(map[string]bool),
		visitedPUUIDs: make(map[string]bool),
	}

	// If Refresh is enabled, mark all summoners in the dataset as uncrawled so they are re-queued.
	if cfg.Refresh {
		for _, s := range dataset.Summoners {
			if s != nil && s.Crawled {
				s.Crawled = false
				dataset.Saved = false
			}
		}
	}

	// 1. If seed PUUID is provided, always enqueue it first at startup (even if already crawled)
	// to check for any new matches in the specified time window.
	if cfg.SeedPUUID != "" {
		crawler.queuedPUUIDs[cfg.SeedPUUID] = true
		crawler.puuidQueue = append(crawler.puuidQueue, cfg.SeedPUUID)
	}

	// 2. Pre-populate queue from existing dataset summoners that have not yet been crawled.
	for _, s := range dataset.Summoners {
		if s == nil || s.PUUID == "" {
			continue
		}
		if s.PUUID == cfg.SeedPUUID {
			continue
		}
		if s.Crawled {
			crawler.visitedPUUIDs[s.PUUID] = true
		} else {
			crawler.queuedPUUIDs[s.PUUID] = true
			crawler.puuidQueue = append(crawler.puuidQueue, s.PUUID)
		}
	}

	return crawler
}

// EnqueuePUUID adds a PUUID to the crawl queue if not already queued or visited,
// and not already marked as crawled in the dataset.
func (c *MatchCrawler) EnqueuePUUID(puuid string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if puuid == "" || c.queuedPUUIDs[puuid] || c.visitedPUUIDs[puuid] {
		return
	}
	if s := c.dataset.GetSummoner(puuid); s != nil && s.Crawled {
		c.visitedPUUIDs[puuid] = true
		return
	}
	c.queuedPUUIDs[puuid] = true
	c.puuidQueue = append(c.puuidQueue, puuid)
}

// ReenqueueSeed adds the seed PUUID to the front of the crawl queue,
// even if it was previously visited or marked as crawled.
func (c *MatchCrawler) ReenqueueSeed(puuid string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if puuid == "" {
		return
	}
	delete(c.visitedPUUIDs, puuid)

	newQueue := make([]string, 0, len(c.puuidQueue)+1)
	newQueue = append(newQueue, puuid)
	for _, q := range c.puuidQueue {
		if q != puuid {
			newQueue = append(newQueue, q)
		}
	}
	c.puuidQueue = newQueue
	c.queuedPUUIDs[puuid] = true
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

func applySummonerProfile(target, src *data.SummonerV4) bool {
	if target == nil || src == nil {
		return false
	}
	changed := false
	if src.Name != "" {
		if target.Name == "" || (strings.Contains(src.Name, "#") && !strings.Contains(target.Name, "#")) {
			target.Name = src.Name
			changed = true
		}
	}
	if src.SummonerLevel != 0 && target.SummonerLevel != src.SummonerLevel {
		target.SummonerLevel = src.SummonerLevel
		changed = true
	}
	if src.AccountID != "" && target.AccountID != src.AccountID {
		target.AccountID = src.AccountID
		changed = true
	}
	if src.ID != "" && target.ID != src.ID {
		target.ID = src.ID
		changed = true
	}
	if src.ProfileIconID != 0 && target.ProfileIconID != src.ProfileIconID {
		target.ProfileIconID = src.ProfileIconID
		changed = true
	}
	if src.RevisionDate != 0 && target.RevisionDate != src.RevisionDate {
		target.RevisionDate = src.RevisionDate
		changed = true
	}
	return changed
}

func (c *MatchCrawler) saveCheckpoint(reason string) {
	if c.store == nil || c.store.FilePath() == "" {
		return
	}
	c.mu.Lock()
	if c.dataset.Saved {
		c.mu.Unlock()
		return
	}
	matches := len(c.dataset.Matches)
	totalSummoners := len(c.dataset.Summoners)
	crawledSummoners := c.dataset.NumCrawledSummoners()
	profileSummoners := c.dataset.NumSummonersWithProfile()
	err := c.store.Save(c.dataset)
	c.mu.Unlock()

	tag := "[Checkpoint]"
	if reason != "" {
		tag = fmt.Sprintf("[Checkpoint - %s]", reason)
	}
	fmt.Printf("\n%s Saving dataset (%d matches, %d players, %d crawled, %d with profile) to %s ...\n",
		tag, matches, totalSummoners, crawledSummoners, profileSummoners, c.store.FilePath())
	if err != nil {
		fmt.Printf("[Checkpoint Error] Failed to save dataset: %v\n", err)
	} else {
		fmt.Printf("%s Saved successfully.\n", tag)
	}
}

// fetchMissingSummonerProfiles downloads Summoner-V4 profile info (revisionDate, summonerLevel, etc.)
// for all summoners in the dataset that do not have profile information yet.
func (c *MatchCrawler) fetchMissingSummonerProfiles(ctx context.Context) error {
	c.mu.Lock()
	var missing []*data.SummonerV4
	for _, s := range c.dataset.Summoners {
		if s != nil && s.PUUID != "" && !strings.HasPrefix(s.PUUID, "oe:") && !s.HasProfile() {
			missing = append(missing, s)
		}
	}
	c.mu.Unlock()

	if len(missing) == 0 {
		return nil
	}

	fmt.Printf("Fetching Summoner-V4 profile info for %d summoners without profile data...\n", len(missing))
	updatedCount := 0
	errorCount := 0
	startTime := time.Now()

	for i, s := range missing {
		select {
		case <-ctx.Done():
			fmt.Println()
			return ctx.Err()
		default:
		}

		profile, err := c.client.GetSummonerByPUUID(ctx, s.PUUID)
		if err != nil {
			if ctx.Err() != nil {
				fmt.Println()
				return ctx.Err()
			}
			errorCount++
			if c.cfg.Verbose {
				fmt.Printf("\n  [Warning] Failed fetching Summoner-V4 for %s (%s): %v\n", s.PUUID, s.Name, err)
			}
		} else if profile != nil {
			c.mu.Lock()
			if applySummonerProfile(s, profile) {
				c.dataset.Saved = false
			}
			c.mu.Unlock()
			updatedCount++
		}

		elapsed := time.Since(startTime)
		rate := float64(i+1) / elapsed.Seconds()
		fmt.Printf("\r  [SummonerV4 Init] %d/%d (%.1f%%) in %v (%.1f req/s) | %d updated, %d errors",
			i+1, len(missing), float64(i+1)*100.0/float64(len(missing)), elapsed.Round(time.Second), rate, updatedCount, errorCount)
	}

	fmt.Println()
	fmt.Printf("Completed fetching Summoner-V4 profiles: %d updated, %d failed / skipped in %v.\n",
		updatedCount, errorCount, time.Since(startTime).Round(time.Second))

	// Save checkpoint immediately after completing startup profile downloads if any were updated
	if updatedCount > 0 {
		c.saveCheckpoint("SummonerV4 Init")
	}

	return nil
}

func (c *MatchCrawler) printCrawlProgress() {
	c.mu.Lock()
	matches := len(c.dataset.Matches)
	profiles := c.dataset.NumSummonersWithProfile()
	crawledSummoners := c.dataset.NumCrawledSummoners()
	totalSummoners := len(c.dataset.Summoners)
	queueSize := len(c.puuidQueue)
	c.mu.Unlock()

	fmt.Printf("\r[Crawl] Matches: %d | Profiles: %d | Summoners Crawled: %d (Total: %d, Queue: %d)",
		matches, profiles, crawledSummoners, totalSummoners, queueSize)
}

// fetchMissingParticipantProfiles retrieves Summoner-V4 info for any participants in the match
// for which the dataset doesn't have profile data yet.
func (c *MatchCrawler) fetchMissingParticipantProfiles(ctx context.Context, match *data.MatchV5) error {
	if match == nil || match.Info.Participants == nil {
		return nil
	}

	for _, p := range match.Info.Participants {
		if p == nil || p.PUUID == "" || strings.HasPrefix(p.PUUID, "oe:") {
			continue
		}

		c.mu.Lock()
		s := c.dataset.GetSummoner(p.PUUID)
		hasProfile := s != nil && s.HasProfile()
		c.mu.Unlock()

		if hasProfile {
			continue
		}

		select {
		case <-ctx.Done():
			fmt.Println()
			return ctx.Err()
		default:
		}

		profile, err := c.client.GetSummonerByPUUID(ctx, p.PUUID)
		if err != nil {
			if ctx.Err() != nil {
				fmt.Println()
				return ctx.Err()
			}
			if c.cfg.Verbose {
				fmt.Printf("\n  [Warning] Failed fetching Summoner-V4 for participant %s (%s): %v\n", p.PUUID, p.SummonerName, err)
			}
			continue
		}

		if profile != nil {
			c.mu.Lock()
			name := p.SummonerName
			if p.RiotIDGameName != "" {
				if p.RiotIDTagline != "" {
					name = p.RiotIDGameName + "#" + p.RiotIDTagline
				} else if name == "" {
					name = p.RiotIDGameName
				}
			}
			s := c.dataset.GetOrCreateSummoner(p.PUUID, name)
			if applySummonerProfile(s, profile) {
				c.dataset.Saved = false
			}
			p.Summoner = s
			c.mu.Unlock()
			c.printCrawlProgress()
		}
	}

	return nil
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
					c.saveCheckpoint("Periodic")
				}
			}
		}()
	}

	c.mu.Lock()
	initMatches := len(c.dataset.Matches)
	initSummoners := len(c.dataset.Summoners)
	initCrawled := c.dataset.NumCrawledSummoners()
	initProfiles := c.dataset.NumSummonersWithProfile()
	c.mu.Unlock()

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
	fmt.Printf("  Initial Dataset: %d matches, %d players (%d crawled, %d with profile)\n", initMatches, initSummoners, initCrawled, initProfiles)
	fmt.Println("----------------------------------------------------------")

	// 1. Fetch Summoner-V4 profile info for all summoners in dataset missing profile data
	if err := c.fetchMissingSummonerProfiles(ctx); err != nil {
		return err
	}

	c.printCrawlProgress()

	for {
		select {
		case <-ctx.Done():
			fmt.Println()
			return ctx.Err()
		default:
		}

		if c.cfg.MaxMatches > 0 && c.CrawledMatchesCount() >= c.cfg.MaxMatches {
			fmt.Printf("\nReached target max matches (%d). Stopping crawl.\n", c.cfg.MaxMatches)
			break
		}

		c.mu.Lock()
		if len(c.puuidQueue) == 0 {
			c.mu.Unlock()
			fmt.Println("\nQueue is empty. Crawl completed.")
			break
		}
		puuid := c.puuidQueue[0]
		c.puuidQueue = c.puuidQueue[1:]
		c.visitedPUUIDs[puuid] = true
		c.mu.Unlock()

		// Fetch summoner profile if not fully populated
		c.mu.Lock()
		summoner := c.dataset.GetSummoner(puuid)
		needProfile := (summoner == nil || !summoner.HasProfile())
		c.mu.Unlock()

		var summonerProfile *data.SummonerV4
		if needProfile {
			if s, err := c.client.GetSummonerByPUUID(ctx, puuid); err == nil && s != nil {
				summonerProfile = s
			} else if ctx.Err() != nil {
				fmt.Println()
				return ctx.Err()
			}
		}

		// Fetch match IDs for this summoner (always with count=100)
		matchIDs, err := c.client.GetMatchIDsByPUUID(ctx, puuid, c.cfg.StartTime, c.cfg.EndTime, 0)
		if err != nil {
			if ctx.Err() != nil {
				fmt.Println()
				return ctx.Err()
			}
			if c.cfg.Verbose {
				fmt.Printf("\n[Error] Failed fetching match IDs for summoner %s: %v\n", puuid, err)
			}
			continue
		}

		if len(matchIDs) == 0 {
			c.mu.Lock()
			s := c.dataset.GetOrCreateSummoner(puuid, "")
			if summonerProfile != nil {
				if applySummonerProfile(s, summonerProfile) {
					c.dataset.Saved = false
				}
			}
			if !s.Crawled {
				s.Crawled = true
				c.dataset.Saved = false
			}
			c.mu.Unlock()
			c.printCrawlProgress()
			continue
		}

		if c.cfg.Verbose {
			c.mu.Lock()
			displayName := puuid
			if s := c.dataset.GetSummoner(puuid); s != nil && s.Name != "" {
				displayName = fmt.Sprintf("%s (%s)", s.Name, puuid)
			}
			c.mu.Unlock()
			fmt.Printf("\n[Summoner] %s found %d matches in window. Downloading matches...\n", displayName, len(matchIDs))
		}

		// Prioritize fully crawling all matches of this summoner before atomically adding to dataset
		var fetchedMatches []*data.MatchV5
		var alreadyPresentMatches []*data.MatchV5
		fetchFailed := false

		for _, matchID := range matchIDs {
			select {
			case <-ctx.Done():
				fmt.Println()
				return ctx.Err()
			default:
			}

			// Check if match is already loaded in dataset
			c.mu.Lock()
			existingMatch := c.dataset.GetMatch(matchID)
			c.mu.Unlock()

			if existingMatch != nil {
				alreadyPresentMatches = append(alreadyPresentMatches, existingMatch)
				continue
			}

			match, err := c.client.GetMatch(ctx, matchID)
			if err != nil {
				if ctx.Err() != nil {
					fmt.Println()
					return ctx.Err()
				}
				if c.cfg.Verbose {
					fmt.Printf("\n[Error] Failed fetching match %s for summoner %s: %v\n", matchID, puuid, err)
				}
				fetchFailed = true
				break
			}
			if match == nil || !match.IsRanked() {
				continue
			}

			// Immediately retrieve Summoner-V4 profile info of all participants of the match missing that info
			if err := c.fetchMissingParticipantProfiles(ctx, match); err != nil {
				return err
			}

			fetchedMatches = append(fetchedMatches, match)
			c.printCrawlProgress()
		}

		if fetchFailed {
			// If we failed to crawl all matches for this summoner, do not add partial matches
			// and do not mark as crawled, so the summoner can be crawled cleanly on retry/restart.
			continue
		}

		// Atomically add matches to dataset, mark summoner as crawled, and enqueue new participants
		c.mu.Lock()
		s := c.dataset.GetOrCreateSummoner(puuid, "")
		if summonerProfile != nil {
			if applySummonerProfile(s, summonerProfile) {
				c.dataset.Saved = false
			}
		}

		for _, match := range fetchedMatches {
			c.dataset.AddMatch(match)
		}
		if !s.Crawled {
			s.Crawled = true
			c.dataset.Saved = false
		}
		c.crawledMatchesCount += len(fetchedMatches)

		// Enqueue other participants from all matches of this summoner (both new and already present)
		// that have not been enqueued or crawled yet.
		allMatches := append(alreadyPresentMatches, fetchedMatches...)
		for _, m := range allMatches {
			for _, p := range m.Info.Participants {
				if p == nil || p.PUUID == "" {
					continue
				}
				pPUUID := p.PUUID
				if c.visitedPUUIDs[pPUUID] || c.queuedPUUIDs[pPUUID] {
					continue
				}
				pSummoner := c.dataset.GetSummoner(pPUUID)
				if pSummoner != nil && pSummoner.Crawled {
					c.visitedPUUIDs[pPUUID] = true
					continue
				}
				c.queuedPUUIDs[pPUUID] = true
				c.puuidQueue = append(c.puuidQueue, pPUUID)
			}
		}

		c.mu.Unlock()
		c.printCrawlProgress()
	}

	return nil
}
