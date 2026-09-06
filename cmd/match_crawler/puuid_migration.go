package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/janpfeifer/loldata/data"
)

// matchSummonersFromRankDB scans all summoners in the dataset missing a complete profile
// and updates them with cached rank data if present in RankDatabase.
func (c *MatchCrawler) matchSummonersFromRankDB() int {
	if c.rankDB == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	matched := 0
	for _, s := range c.dataset.Summoners {
		if s == nil || s.PUUID == "" || strings.HasPrefix(s.PUUID, "oe:") {
			continue
		}
		if !s.HasProfile() {
			if entry := c.rankDB.Lookup(s.ID, s.PUUID); entry != nil {
				s.RankTier = entry.RankTier
				s.LeaguePoints = entry.LeaguePoints
				s.RankWins = entry.Wins
				s.RankLosses = entry.Losses
				s.RankFetched = true
				if s.ID == "" && entry.SummonerID != "" {
					s.ID = entry.SummonerID
				}
				c.dataset.Saved = false
				matched++
			}
		}
	}
	return matched
}

// migrateSummonerPUUID attempts to resolve a summoner's new encrypted PUUID using their Riot ID (gameName#tagLine).
// If found, it updates the summoner and re-indexes the dataset.
// If the Riot ID is unknown or the account cannot be found (HTTP 404), it marks ProfileUnavailable = true.
func (c *MatchCrawler) migrateSummonerPUUID(ctx context.Context, s *data.SummonerV4) (*data.SummonerV4, error) {
	if s == nil {
		return nil, nil
	}
	defaultTag := strings.ToUpper(c.client.Platform())
	gameName, tagLine := s.RiotID(defaultTag)
	if gameName == "" {
		c.mu.Lock()
		s.ProfileUnavailable = true
		s.PUUIDInvalid = false
		c.dataset.Saved = false
		c.mu.Unlock()
		return nil, nil
	}

	acc, err := c.client.GetAccountByRiotID(ctx, gameName, tagLine)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			c.mu.Lock()
			s.ProfileUnavailable = true
			s.PUUIDInvalid = false
			c.dataset.Saved = false
			c.mu.Unlock()
			return nil, nil
		}
		c.mu.Lock()
		s.PUUIDInvalid = true
		c.dataset.Saved = false
		c.mu.Unlock()
		return nil, err
	}

	if acc != nil && acc.PUUID != "" {
		c.mu.Lock()
		updated := c.dataset.UpdateSummonerPUUID(s, acc.PUUID)
		c.dataset.Saved = false
		c.mu.Unlock()
		return updated, nil
	}
	return nil, nil
}

// migratePUUIDsViaMatchGreedyCover updates PUUIDs in bulk by re-fetching historical matches from Match-V5.
// Riot's Match-V5 endpoint dynamically encrypts all 10 participants' PUUIDs for the requesting API key.
// Using a greedy set cover, matches containing the most unmigrated players are fetched first,
// updating up to 10 player PUUIDs per request instead of 1 request per player via Account-V1.
func (c *MatchCrawler) migratePUUIDsViaMatchGreedyCover(ctx context.Context) error {
	c.mu.Lock()
	unmigratedPUUIDs := make(map[string]*data.SummonerV4)
	for _, s := range c.dataset.Summoners {
		if s != nil && s.PUUID != "" && !strings.HasPrefix(s.PUUID, "oe:") && s.PUUIDInvalid {
			unmigratedPUUIDs[s.PUUID] = s
		}
	}

	if len(unmigratedPUUIDs) == 0 {
		c.mu.Unlock()
		return nil
	}

	countUnmigrated := func(m *data.MatchV5) int {
		if m == nil || m.Info.Participants == nil {
			return 0
		}
		count := 0
		for _, p := range m.Info.Participants {
			if p != nil && unmigratedPUUIDs[p.PUUID] != nil {
				count++
			}
		}
		return count
	}

	// Bucket matches by the number of unmigrated participants they contain (1..10)
	var buckets [11][]*data.MatchV5
	for _, m := range c.dataset.Matches {
		if m != nil && m.Metadata.MatchID != "" && !strings.HasPrefix(m.Metadata.MatchID, "oe:") {
			cnt := countUnmigrated(m)
			if cnt > 0 && cnt <= 10 {
				buckets[cnt] = append(buckets[cnt], m)
			}
		}
	}
	c.mu.Unlock()

	fmt.Println("==========================================================")
	fmt.Printf("Starting Match-V5 Greedy Cover PUUID Migration (%d summoners to migrate)\n", len(unmigratedPUUIDs))
	fmt.Printf("  Targeting up to 10 players per Match-V5 API request\n")
	fmt.Println("==========================================================")

	matchesFetched := 0
	summonersMigrated := 0
	rankMatchesCount := 0
	startTime := time.Now()
	lastCheckpoint := time.Now()

	for k := 10; k >= 1; k-- {
		for len(buckets[k]) > 0 {
			select {
			case <-ctx.Done():
				fmt.Println("\nCanceling Match-V5 migration. Saving progress...")
				c.saveCheckpoint("Match-V5 Migration Canceled")
				return ctx.Err()
			default:
			}

			// Pop the last match from bucket k
			c.mu.Lock()
			m := buckets[k][len(buckets[k])-1]
			buckets[k] = buckets[k][:len(buckets[k])-1]

			currCount := countUnmigrated(m)
			c.mu.Unlock()

			if currCount == 0 {
				continue
			}
			if currCount < k {
				buckets[currCount] = append(buckets[currCount], m)
				continue
			}

			// Fetch match from Match-V5 API with current API key
			apiMatch, err := c.client.GetMatch(ctx, m.Metadata.MatchID)
			matchesFetched++
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				if errors.Is(err, ErrUnauthorized) {
					fmt.Printf("\n[FATAL] Riot API key is unauthorized or expired (HTTP 401/403): %v\n", err)
					return err
				}
				if errors.Is(err, ErrNotFound) {
					// Match is too old (> 2 years) or not found on Riot
					continue
				}
				if c.cfg.Verbose {
					fmt.Printf("\n  [Warning] Failed fetching match %s: %v\n", m.Metadata.MatchID, err)
				}
				continue
			}

			// Update participants using the newly encrypted PUUIDs
			c.mu.Lock()
			for idx := 0; idx < len(apiMatch.Info.Participants) && idx < len(m.Info.Participants); idx++ {
				pOld := m.Info.Participants[idx]
				pNew := apiMatch.Info.Participants[idx]
				if pOld == nil || pNew == nil || pNew.PUUID == "" {
					continue
				}

				// Verify slot correspondence by Riot ID / summoner name
				if pOld.RiotIDGameName != "" && pNew.RiotIDGameName != "" &&
					!strings.EqualFold(pOld.RiotIDGameName, pNew.RiotIDGameName) {
					foundSlot := false
					for _, candidate := range apiMatch.Info.Participants {
						if candidate != nil && strings.EqualFold(candidate.RiotIDGameName, pOld.RiotIDGameName) {
							pNew = candidate
							foundSlot = true
							break
						}
					}
					if !foundSlot {
						continue
					}
				}

				oldPUUID := pOld.PUUID
				oldSummoner := unmigratedPUUIDs[oldPUUID]
				if oldSummoner != nil && oldPUUID != pNew.PUUID {
					migrated := c.dataset.UpdateSummonerPUUID(oldSummoner, pNew.PUUID)
					delete(unmigratedPUUIDs, oldPUUID)
					summonersMigrated++

					// Check rank_db immediately with new PUUID
					if c.rankDB != nil && !migrated.HasProfile() {
						if entry := c.rankDB.Lookup(migrated.ID, migrated.PUUID); entry != nil {
							migrated.RankTier = entry.RankTier
							migrated.LeaguePoints = entry.LeaguePoints
							migrated.RankWins = entry.Wins
							migrated.RankLosses = entry.Losses
							migrated.RankFetched = true
							if migrated.ID == "" && entry.SummonerID != "" {
								migrated.ID = entry.SummonerID
							}
							rankMatchesCount++
						}
					}
				}
			}
			c.dataset.Saved = false
			c.mu.Unlock()

			elapsed := time.Since(startTime)
			rate := float64(matchesFetched) / elapsed.Seconds()
			fmt.Printf("\r  [Match-V5 Migration] %d matches fetched (%.1f req/s) | %d summoners migrated (%d remain) | %d matched in rank_db",
				matchesFetched, rate, summonersMigrated, len(unmigratedPUUIDs), rankMatchesCount)

			// Periodically save checkpoint
			if (c.cfg.CheckpointDuration > 0 && time.Since(lastCheckpoint) >= c.cfg.CheckpointDuration) || matchesFetched%500 == 0 {
				c.saveCheckpoint("Match-V5 Migration")
				lastCheckpoint = time.Now()
			}

			if len(unmigratedPUUIDs) == 0 {
				break
			}
		}
		if len(unmigratedPUUIDs) == 0 {
			break
		}
	}

	fmt.Println()
	fmt.Printf("Completed Match-V5 migration: %d matches fetched, %d summoners migrated, %d rank_db matches in %v.\n",
		matchesFetched, summonersMigrated, rankMatchesCount, time.Since(startTime).Round(time.Second))

	if summonersMigrated > 0 {
		c.saveCheckpoint("Match-V5 Migration Complete")
	}
	return nil
}

// fetchMissingSummonerProfiles downloads Summoner-V4 profile info (revisionDate, summonerLevel, etc.)
// and rank details for all summoners in the dataset that do not have complete profile information yet.
// If any summoner has PUUIDInvalid (e.g. from an API key migration), it resolves their new PUUID
// via Match-V5 greedy set cover, or falls back to Riot ID.
func (c *MatchCrawler) fetchMissingSummonerProfiles(ctx context.Context) error {
	// 1. Upfront match against RankDatabase for all summoners missing a profile
	matchedRankDB := c.matchSummonersFromRankDB()
	if c.rankDB != nil {
		fmt.Printf("Matched %d summoners without profile from rank_db upfront.\n", matchedRankDB)
	}

	c.mu.Lock()
	hasInvalid := false
	var toUpdate []*data.SummonerV4
	for _, s := range c.dataset.Summoners {
		if s != nil && s.PUUID != "" && !strings.HasPrefix(s.PUUID, "oe:") {
			if s.PUUIDInvalid {
				hasInvalid = true
			}
			if s.PUUIDInvalid || s.NeedsProfile() {
				toUpdate = append(toUpdate, s)
			}
		}
	}
	c.mu.Unlock()

	if len(toUpdate) == 0 {
		return nil
	}

	// 2. Canary check: probe a sample summoner missing level/revisionDate to detect if the API key encryption context changed
	// or if the API key is unauthorized/expired (HTTP 401/403), avoiding thousands of failed requests.
	if !hasInvalid {
		probed := 0
		for _, testSummoner := range toUpdate {
			if probed >= 3 {
				break
			}
			if testSummoner == nil || testSummoner.PUUID == "" || testSummoner.PUUIDInvalid || testSummoner.SummonerLevel > 0 || testSummoner.RevisionDate > 0 {
				continue
			}
			probed++
			_, err := c.client.GetSummonerByPUUID(ctx, testSummoner.PUUID)
			if err != nil {
				if errors.Is(err, ErrUnauthorized) {
					fmt.Printf("\n[FATAL] Riot API key is unauthorized or expired (HTTP 401/403): %v\n", err)
					return err
				}
				if errors.Is(err, ErrDecryptionMismatch) {
					fmt.Printf("\n[Application Key Mismatch Detected]\n")
					fmt.Printf("  The PUUIDs in this dataset were encrypted with a different API key/project than the current one.\n")
					fmt.Printf("  Riot cannot decrypt these %d legacy summoner PUUIDs with your current key.\n", len(toUpdate))
					fmt.Printf("  Marking %d legacy summoners as 'PUUIDInvalid' for Match-V5 bulk migration...\n", len(toUpdate))
					c.mu.Lock()
					for _, s := range toUpdate {
						s.PUUIDInvalid = true
					}
					c.dataset.Saved = false
					hasInvalid = true
					c.mu.Unlock()
					c.saveCheckpoint("Key Mismatch Detected")
					break
				}
				if errors.Is(err, ErrNotFound) {
					continue
				}
			}
			break
		}
	}

	// 3. If invalid PUUIDs are detected, run Greedy Match-V5 re-retrieval (10 players per request)
	if hasInvalid && len(c.dataset.Matches) > 0 {
		if err := c.migratePUUIDsViaMatchGreedyCover(ctx); err != nil {
			return err
		}
		// Re-run upfront match against RankDatabase with newly migrated PUUIDs
		if newMatches := c.matchSummonersFromRankDB(); newMatches > 0 {
			fmt.Printf("Matched %d additional summoners from rank_db after Match-V5 migration.\n", newMatches)
		}
	}

	// 4. Re-collect any remaining summoners that still need profile or fallback PUUID migration
	c.mu.Lock()
	toUpdate = nil
	for _, s := range c.dataset.Summoners {
		if s != nil && s.PUUID != "" && !strings.HasPrefix(s.PUUID, "oe:") && (s.PUUIDInvalid || s.NeedsProfile()) {
			toUpdate = append(toUpdate, s)
		}
	}
	// Prioritize summoners with the most matches in the dataset so the most matches become complete and usable first
	sort.Slice(toUpdate, func(i, j int) bool {
		return len(toUpdate[i].Matches) > len(toUpdate[j].Matches)
	})
	c.mu.Unlock()

	if len(toUpdate) == 0 {
		return nil
	}

	fmt.Printf("Syncing Summoner PUUIDs and profiles for %d summoners...\n", len(toUpdate))
	updatedProfileCount := 0
	migratedPUUIDCount := 0
	errorCount := 0
	startTime := time.Now()

	for i, s := range toUpdate {
		select {
		case <-ctx.Done():
			fmt.Println()
			return ctx.Err()
		default:
		}

		profileUpdated := false

		// Fallback PUUID update if still marked invalid (migrate using Riot ID / Account-V1)
		if s.PUUIDInvalid {
			migrated, err := c.migrateSummonerPUUID(ctx, s)
			if err != nil {
				if ctx.Err() != nil {
					fmt.Println()
					return ctx.Err()
				}
				if errors.Is(err, ErrUnauthorized) {
					fmt.Printf("\n[FATAL] Riot API key is unauthorized or expired (HTTP 401/403): %v\n", err)
					return err
				}
				errorCount++
				if c.cfg.Verbose {
					gameName, tagLine := s.RiotID(strings.ToUpper(c.client.Platform()))
					fmt.Printf("\n  [Warning] Failed resolving Riot ID %s#%s: %v\n", gameName, tagLine, err)
				}
			} else if migrated != nil {
				s = migrated
				migratedPUUIDCount++
				if c.rankDB != nil && !s.HasProfile() {
					if entry := c.rankDB.Lookup(s.ID, s.PUUID); entry != nil {
						c.mu.Lock()
						s.RankTier = entry.RankTier
						s.LeaguePoints = entry.LeaguePoints
						s.RankWins = entry.Wins
						s.RankLosses = entry.Losses
						s.RankFetched = true
						if s.ID == "" && entry.SummonerID != "" {
							s.ID = entry.SummonerID
						}
						c.dataset.Saved = false
						profileUpdated = true
						c.mu.Unlock()
					}
				}
			}
		}

		// Download profile if needed
		if !s.PUUIDInvalid && !s.ProfileUnavailable && s.NeedsProfile() {
			if s.SummonerLevel == 0 && s.RevisionDate == 0 {
				profile, err := c.client.GetSummonerByPUUID(ctx, s.PUUID)
				if err != nil {
					if ctx.Err() != nil {
						fmt.Println()
						return ctx.Err()
					}
					if errors.Is(err, ErrUnauthorized) {
						fmt.Printf("\n[FATAL] Riot API key is unauthorized or expired (HTTP 401/403): %v\n", err)
						return err
					}
					if errors.Is(err, ErrDecryptionMismatch) {
						// Immediately attempt to migrate this summoner via Riot ID
						migrated, mErr := c.migrateSummonerPUUID(ctx, s)
						if mErr != nil {
							if errors.Is(mErr, ErrUnauthorized) {
								fmt.Printf("\n[FATAL] Riot API key is unauthorized or expired (HTTP 401/403): %v\n", mErr)
								return mErr
							}
							errorCount++
						} else if migrated != nil {
							s = migrated
							migratedPUUIDCount++
							// Retry fetching profile with newly migrated PUUID
							if p2, pErr := c.client.GetSummonerByPUUID(ctx, s.PUUID); pErr == nil && p2 != nil {
								c.mu.Lock()
								if applySummonerProfile(s, p2) {
									c.dataset.Saved = false
									profileUpdated = true
								}
								c.mu.Unlock()
							}
						}
					} else if errors.Is(err, ErrNotFound) {
						c.mu.Lock()
						s.ProfileUnavailable = true
						c.dataset.Saved = false
						c.mu.Unlock()
					} else {
						errorCount++
						if c.cfg.Verbose {
							fmt.Printf("\n  [Warning] Failed fetching Summoner-V4 for %s (%s): %v\n", s.PUUID, s.Name, err)
						}
					}
				} else if profile != nil {
					c.mu.Lock()
					if applySummonerProfile(s, profile) {
						c.dataset.Saved = false
						profileUpdated = true
					}
					c.mu.Unlock()
				}
			}

			if !s.RankFetched && !s.PUUIDInvalid && !s.ProfileUnavailable {
				tier, lp, wins, losses, fetched, err := c.resolveSummonerRank(ctx, s.ID, s.PUUID)
				if err != nil {
					if ctx.Err() != nil {
						fmt.Println()
						return ctx.Err()
					}
					if errors.Is(err, ErrUnauthorized) {
						fmt.Printf("\n[FATAL] Riot API key is unauthorized or expired (HTTP 401/403): %v\n", err)
						return err
					}
					if errors.Is(err, ErrDecryptionMismatch) {
						c.mu.Lock()
						s.PUUIDInvalid = true
						c.dataset.Saved = false
						c.mu.Unlock()
					}
					errorCount++
					if c.cfg.Verbose {
						fmt.Printf("\n  [Warning] Failed fetching League-V4 for %s (%s): %v\n", s.PUUID, s.Name, err)
					}
				} else if fetched {
					c.mu.Lock()
					s.RankTier = tier
					s.LeaguePoints = lp
					s.RankWins = wins
					s.RankLosses = losses
					s.RankFetched = true
					c.dataset.Saved = false
					c.mu.Unlock()
					profileUpdated = true
				}
			}
		}

		if profileUpdated {
			updatedProfileCount++
		}

		elapsed := time.Since(startTime)
		rate := float64(i+1) / elapsed.Seconds()
		fmt.Printf("\r  [Summoner Profile Sync] %d/%d (%.1f%%) in %v (%.1f req/s) | %d profiles updated, %d PUUIDs migrated, %d errors",
			i+1, len(toUpdate), float64(i+1)*100.0/float64(len(toUpdate)), elapsed.Round(time.Second), rate, updatedProfileCount, migratedPUUIDCount, errorCount)
	}

	fmt.Println()
	fmt.Printf("Completed Summoner sync: %d profiles updated, %d PUUIDs migrated, %d errors in %v.\n",
		updatedProfileCount, migratedPUUIDCount, errorCount, time.Since(startTime).Round(time.Second))

	if updatedProfileCount > 0 || migratedPUUIDCount > 0 {
		c.saveCheckpoint("Summoner Profile Sync")
	}

	return nil
}
