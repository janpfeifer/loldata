package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/janpfeifer/loldata/data"
)

// RankEntry represents a summoner's cached rank and LP details.
type RankEntry struct {
	SummonerID   string        `json:"summonerId,omitempty"`
	PUUID        string        `json:"puuid,omitempty"`
	RankTier     data.RankTier `json:"rankTier"`
	LeaguePoints int           `json:"leaguePoints"`
	Wins         int           `json:"wins"`
	Losses       int           `json:"losses"`
}

// RankDatabase stores a lookup table of summoners and their rank tier/LP.
type RankDatabase struct {
	mu                sync.RWMutex
	filePath          string
	BySummonerID      map[string]*RankEntry `json:"bySummonerId"`
	ByPUUID           map[string]*RankEntry `json:"byPuuid"`
	CompletedBrackets map[string]bool       `json:"completedBrackets,omitempty"`
}

// NewRankDatabase creates an initialized RankDatabase.
func NewRankDatabase(filePath string) *RankDatabase {
	return &RankDatabase{
		filePath:          filePath,
		BySummonerID:      make(map[string]*RankEntry),
		ByPUUID:           make(map[string]*RankEntry),
		CompletedBrackets: make(map[string]bool),
	}
}

// TotalBracketsCount is the total number of competitive rank brackets (3 Apex + 28 Standard).
const TotalBracketsCount = 31

// IsComplete returns true if all 31 rank brackets have been fully built.
func (db *RankDatabase) IsComplete() bool {
	if db == nil {
		return false
	}
	db.mu.RLock()
	defer db.mu.RUnlock()
	return len(db.CompletedBrackets) >= TotalBracketsCount
}

// IsBracketComplete returns true if a specific bracket key has finished building.
func (db *RankDatabase) IsBracketComplete(bracketKey string) bool {
	if db == nil {
		return false
	}
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.CompletedBrackets[bracketKey]
}

// MarkBracketComplete records that a bracket has completed building.
func (db *RankDatabase) MarkBracketComplete(bracketKey string) {
	if db == nil {
		return
	}
	db.mu.Lock()
	defer db.mu.Unlock()
	if db.CompletedBrackets == nil {
		db.CompletedBrackets = make(map[string]bool)
	}
	db.CompletedBrackets[bracketKey] = true
}

// FilePath returns the file path of the database.
func (db *RankDatabase) FilePath() string {
	return db.filePath
}

// Size returns the total number of unique summoner entries stored.
func (db *RankDatabase) Size() int {
	if db == nil {
		return 0
	}
	db.mu.RLock()
	defer db.mu.RUnlock()

	seen := make(map[string]struct{}, len(db.BySummonerID)+len(db.ByPUUID))
	for _, e := range db.BySummonerID {
		if e == nil {
			continue
		}
		key := e.SummonerID
		if key == "" {
			key = e.PUUID
		}
		if key != "" {
			seen[key] = struct{}{}
		}
	}
	for _, e := range db.ByPUUID {
		if e == nil {
			continue
		}
		key := e.SummonerID
		if key == "" {
			key = e.PUUID
		}
		if key != "" {
			seen[key] = struct{}{}
		}
	}
	return len(seen)
}

// Lookup finds a rank entry by encrypted summoner ID or PUUID.
func (db *RankDatabase) Lookup(summonerID, puuid string) *RankEntry {
	if db == nil {
		return nil
	}
	db.mu.RLock()
	defer db.mu.RUnlock()

	if summonerID != "" {
		if entry, ok := db.BySummonerID[summonerID]; ok {
			return entry
		}
	}
	if puuid != "" {
		if entry, ok := db.ByPUUID[puuid]; ok {
			return entry
		}
	}
	return nil
}

// Add inserts or updates a rank entry in the database.
func (db *RankDatabase) Add(entry *RankEntry) {
	if db == nil || entry == nil {
		return
	}
	db.mu.Lock()
	defer db.mu.Unlock()

	if entry.SummonerID != "" {
		db.BySummonerID[entry.SummonerID] = entry
	}
	if entry.PUUID != "" {
		db.ByPUUID[entry.PUUID] = entry
	}
}

// Exists checks if the database file exists on disk.
func (db *RankDatabase) Exists() bool {
	if db.filePath == "" {
		return false
	}
	info, err := os.Stat(db.filePath)
	return err == nil && !info.IsDir()
}

// Load reads the rank database from disk (supports automatic .gz decompression).
func (db *RankDatabase) Load() error {
	if db.filePath == "" {
		return fmt.Errorf("empty file path for rank database")
	}

	f, err := os.Open(db.filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	var reader io.Reader = f
	if strings.HasSuffix(strings.ToLower(db.filePath), ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("failed to initialize gzip reader: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(db); err != nil {
		return fmt.Errorf("failed to decode rank database JSON: %w", err)
	}

	if db.BySummonerID == nil {
		db.BySummonerID = make(map[string]*RankEntry)
	}
	if db.ByPUUID == nil {
		db.ByPUUID = make(map[string]*RankEntry)
	}
	if db.CompletedBrackets == nil {
		db.CompletedBrackets = make(map[string]bool)
	}

	return nil
}

// Save writes the rank database to disk atomically (supports automatic .gz compression).
func (db *RankDatabase) Save() error {
	if db.filePath == "" {
		return fmt.Errorf("empty file path for rank database")
	}

	dir := filepath.Dir(db.filePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %q: %w", dir, err)
		}
	}

	tmpFile := db.filePath + ".tmp"
	f, err := os.Create(tmpFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file %q: %w", tmpFile, err)
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	var writer io.WriteCloser = f
	var gzWriter *gzip.Writer
	if strings.HasSuffix(strings.ToLower(db.filePath), ".gz") {
		gzWriter = gzip.NewWriter(f)
		writer = gzWriter
	}

	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(db); err != nil {
		_ = writer.Close()
		_ = f.Close()
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to encode rank database JSON: %w", err)
	}

	if gzWriter != nil {
		if err := gzWriter.Close(); err != nil {
			_ = f.Close()
			_ = os.Remove(tmpFile)
			return fmt.Errorf("failed to close gzip writer: %w", err)
		}
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to close file: %w", err)
	}

	if err := os.Rename(tmpFile, db.filePath); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("failed to rename %q to %q: %w", tmpFile, db.filePath, err)
	}

	return nil
}

// Build populates the RankDatabase using bulk League-V4 API endpoints up to maxPerRank per bracket.
func (db *RankDatabase) Build(ctx context.Context, client *RiotClient, maxPerRank int, checkpointCallback func()) error {
	if maxPerRank <= 0 {
		maxPerRank = 200000
	}

	fmt.Println("==========================================================")
	fmt.Printf("Building Rank Database (Target: up to %d summoners per rank bracket)\n", maxPerRank)
	fmt.Printf("  Platform: %s | DB File: %s\n", client.Platform(), db.filePath)
	fmt.Println("==========================================================")

	startTime := time.Now()
	totalAdded := 0

	// 1. Apex Leagues (Challenger, GrandMaster, Master)
	apexTiers := []struct {
		name string
		tier data.RankTier
	}{
		{"challenger", data.Challenger},
		{"grandmaster", data.GrandMaster},
		{"master", data.Master},
	}

	for _, apex := range apexTiers {
		bracketKey := "APEX_" + strings.ToUpper(apex.name)
		if db.IsBracketComplete(bracketKey) {
			fmt.Printf("  [Skip] Apex league %s already completed.\n", strings.ToUpper(apex.name))
			continue
		}

		select {
		case <-ctx.Done():
			fmt.Println("\nCanceling Rank Database build. Saving progress to disk...")
			_ = db.Save()
			return ctx.Err()
		default:
		}

		fmt.Printf("Fetching apex league: %s ...\n", strings.ToUpper(apex.name))
		list, err := client.GetApexLeague(ctx, apex.name, "RANKED_SOLO_5x5")
		if err != nil {
			if ctx.Err() != nil {
				fmt.Println("\nCanceling Rank Database build. Saving progress to disk...")
				_ = db.Save()
				return ctx.Err()
			}
			fmt.Printf("  [Warning] Failed fetching %s league: %v\n", apex.name, err)
			continue
		}

		if list != nil {
			added := 0
			for _, item := range list.Entries {
				entry := &RankEntry{
					SummonerID:   item.SummonerID,
					PUUID:        item.PUUID,
					RankTier:     apex.tier,
					LeaguePoints: item.LeaguePoints,
					Wins:         item.Wins,
					Losses:       item.Losses,
				}
				db.Add(entry)
				added++
				totalAdded++
			}
			db.MarkBracketComplete(bracketKey)
			fmt.Printf("  [Apex] Loaded %d %s summoners (Total DB size: %d)\n", added, strings.ToUpper(apex.name), db.Size())
			if checkpointCallback != nil {
				checkpointCallback()
			}
		}
	}

	// 2. Standard Tiers & Divisions
	standardTiers := []string{"DIAMOND", "EMERALD", "PLATINUM", "GOLD", "SILVER", "BRONZE", "IRON"}
	divisions := []string{"I", "II", "III", "IV"}

	for _, tier := range standardTiers {
		for _, div := range divisions {
			parsedTier := data.ParseRankTier(tier, div)
			bracketKey := fmt.Sprintf("%s_%s", tier, div)
			bracketName := fmt.Sprintf("%s %s (%s)", tier, div, parsedTier)

			if db.IsBracketComplete(bracketKey) {
				fmt.Printf("  [Skip] %s already completed.\n", bracketName)
				continue
			}

			select {
			case <-ctx.Done():
				fmt.Println("\nCanceling Rank Database build. Saving progress to disk...")
				_ = db.Save()
				return ctx.Err()
			default:
			}

			fmt.Printf("Fetching %s (max %d players) ...\n", bracketName, maxPerRank)

			bracketCount := 0
			page := 1

			for bracketCount < maxPerRank {
				select {
				case <-ctx.Done():
					fmt.Println("\nCanceling Rank Database build. Saving progress to disk...")
					_ = db.Save()
					return ctx.Err()
				default:
				}

				entries, err := client.GetLeagueEntriesPage(ctx, "RANKED_SOLO_5x5", tier, div, page)
				if err != nil {
					if ctx.Err() != nil {
						fmt.Println("\nCanceling Rank Database build. Saving progress to disk...")
						_ = db.Save()
						return ctx.Err()
					}
					fmt.Printf("  [Warning] Failed fetching %s page %d: %v\n", bracketName, page, err)
					break
				}

				if len(entries) == 0 {
					// Reached last page of this bracket
					break
				}

				for _, e := range entries {
					entry := &RankEntry{
						SummonerID:   e.SummonerID,
						PUUID:        e.PUUID,
						RankTier:     parsedTier,
						LeaguePoints: e.LeaguePoints,
						Wins:         e.Wins,
						Losses:       e.Losses,
					}
					db.Add(entry)
					bracketCount++
					totalAdded++
				}

				elapsed := time.Since(startTime)
				rate := float64(totalAdded) / elapsed.Seconds()
				fmt.Printf("\r  [%s] Page %d: %d players loaded in this bracket | Total DB: %d players (%.1f/s)",
					bracketName, page, bracketCount, db.Size(), rate)

				page++
			}
			fmt.Println()

			db.MarkBracketComplete(bracketKey)
			if checkpointCallback != nil {
				checkpointCallback()
			}
		}
	}

	fmt.Println("==========================================================")
	fmt.Printf("Rank Database Build Complete: %d total players stored in %v.\n",
		db.Size(), time.Since(startTime).Round(time.Second))
	fmt.Println("==========================================================")

	return db.Save()
}
