package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/janpfeifer/loldata/data"
)

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newMockRiotClient(handler roundTripFunc) *RiotClient {
	throttler := NewThrottler(LimitRule{
		MaxRequests: 1000,
		Window:      time.Second,
		Name:        "test-limit",
	})
	client := NewRiotClient("test_key", "na1", throttler, false)
	client.httpClient = &http.Client{
		Transport: handler,
		Timeout:   5 * time.Second,
	}
	return client
}

func jsonResponse(statusCode int, body interface{}) (*http.Response, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return &http.Response{
		StatusCode: statusCode,
		Body:       io.NopCloser(bytes.NewReader(b)),
		Header:     make(http.Header),
	}, nil
}

func TestMatchCrawler_SeedReenqueueAndUncrawledSummoners(t *testing.T) {
	ds := data.NewDataset()

	// Seed summoner: already crawled
	sSeed := ds.GetOrCreateSummoner("seed_player", "SeedPlayer")
	sSeed.Crawled = true

	// Another summoner: already crawled
	sCrawled := ds.GetOrCreateSummoner("crawled_player", "CrawledPlayer")
	sCrawled.Crawled = true

	// Uncrawled summoner
	sUncrawled := ds.GetOrCreateSummoner("uncrawled_player", "UncrawledPlayer")
	sUncrawled.Crawled = false

	client := newMockRiotClient(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, nil)
	})

	cfg := CrawlerConfig{
		SeedPUUID: "seed_player",
	}

	crawler := NewMatchCrawler(client, ds, nil, cfg)

	if crawler.QueueSize() != 2 {
		t.Fatalf("expected queue size 2, got %d", crawler.QueueSize())
	}

	// Seed must always be first in queue
	if crawler.puuidQueue[0] != "seed_player" {
		t.Errorf("expected first queued puuid to be 'seed_player', got %q", crawler.puuidQueue[0])
	}
	if crawler.puuidQueue[1] != "uncrawled_player" {
		t.Errorf("expected second queued puuid to be 'uncrawled_player', got %q", crawler.puuidQueue[1])
	}

	// Crawled player must be marked visited and not queued
	if !crawler.visitedPUUIDs["crawled_player"] {
		t.Errorf("expected 'crawled_player' to be marked visited")
	}
	if crawler.queuedPUUIDs["crawled_player"] {
		t.Errorf("expected 'crawled_player' to NOT be marked queued")
	}
}

func TestMatchCrawler_NoSeedOnlyUncrawled(t *testing.T) {
	ds := data.NewDataset()

	s1 := ds.GetOrCreateSummoner("p1", "Player1")
	s1.Crawled = true

	s2 := ds.GetOrCreateSummoner("p2", "Player2")
	s2.Crawled = false

	client := newMockRiotClient(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, nil)
	})

	cfg := CrawlerConfig{
		SeedPUUID: "",
	}

	crawler := NewMatchCrawler(client, ds, nil, cfg)

	if crawler.QueueSize() != 1 {
		t.Fatalf("expected queue size 1, got %d", crawler.QueueSize())
	}
	if crawler.puuidQueue[0] != "p2" {
		t.Errorf("expected queued PUUID to be 'p2', got %q", crawler.puuidQueue[0])
	}
}

func TestMatchCrawler_AtomicSummonerCrawling(t *testing.T) {
	ds := data.NewDataset()

	match1 := &data.MatchV5{
		Metadata: data.MetadataDto{
			MatchID: "NA1_1001",
		},
		Info: data.InfoDto{
			GameStartTimestamp: time.Now().UnixMilli(),
			GameDuration:       1800,
			Participants: []*data.ParticipantDto{
				{PUUID: "seed_puuid", SummonerName: "Seed", TeamID: 100},
				{PUUID: "partner_puuid", SummonerName: "Partner", TeamID: 100},
			},
		},
	}

	match2 := &data.MatchV5{
		Metadata: data.MetadataDto{
			MatchID: "NA1_1002",
		},
		Info: data.InfoDto{
			GameStartTimestamp: time.Now().Add(-1 * time.Hour).UnixMilli(),
			GameDuration:       1900,
			Participants: []*data.ParticipantDto{
				{PUUID: "seed_puuid", SummonerName: "Seed", TeamID: 100},
				{PUUID: "enemy_puuid", SummonerName: "Enemy", TeamID: 200},
			},
		},
	}

	mockHandler := func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Path == "/lol/summoner/v4/summoners/by-puuid/seed_puuid":
			return jsonResponse(http.StatusOK, map[string]interface{}{
				"puuid":         "seed_puuid",
				"name":          "Seed",
				"summonerLevel": 120,
			})
		case req.URL.Path == "/lol/match/v5/matches/by-puuid/seed_puuid/ids":
			return jsonResponse(http.StatusOK, []string{"NA1_1001", "NA1_1002"})
		case req.URL.Path == "/lol/match/v5/matches/NA1_1001":
			return jsonResponse(http.StatusOK, match1)
		case req.URL.Path == "/lol/match/v5/matches/NA1_1002":
			return jsonResponse(http.StatusOK, match2)
		case req.URL.Path == "/lol/match/v5/matches/by-puuid/partner_puuid/ids":
			return jsonResponse(http.StatusOK, []string{})
		case req.URL.Path == "/lol/match/v5/matches/by-puuid/enemy_puuid/ids":
			return jsonResponse(http.StatusOK, []string{})
		default:
			return jsonResponse(http.StatusOK, []string{})
		}
	}

	client := newMockRiotClient(mockHandler)
	cfg := CrawlerConfig{
		SeedPUUID:  "seed_puuid",
		MaxMatches: 2,
	}

	crawler := NewMatchCrawler(client, ds, nil, cfg)
	err := crawler.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Verify dataset
	if len(ds.Matches) != 2 {
		t.Fatalf("expected 2 matches in dataset, got %d", len(ds.Matches))
	}

	seed := ds.GetSummoner("seed_puuid")
	if seed == nil {
		t.Fatalf("seed_puuid not found in dataset")
	}
	if !seed.Crawled {
		t.Errorf("expected seed_puuid.Crawled to be true")
	}
	if seed.SummonerLevel != 120 {
		t.Errorf("expected summonerLevel 120, got %d", seed.SummonerLevel)
	}
	if len(seed.Matches) != 2 {
		t.Errorf("expected 2 matches for seed, got %d", len(seed.Matches))
	}

	// Verify newly discovered participants were enqueued in the crawl queue and NOT yet crawled
	partner := ds.GetSummoner("partner_puuid")
	if partner == nil {
		t.Fatalf("partner_puuid not found in dataset")
	}
	if partner.Crawled {
		t.Errorf("expected partner_puuid.Crawled to be false before it is crawled")
	}

	enemy := ds.GetSummoner("enemy_puuid")
	if enemy == nil {
		t.Fatalf("enemy_puuid not found in dataset")
	}
	if enemy.Crawled {
		t.Errorf("expected enemy_puuid.Crawled to be false before it is crawled")
	}

	if crawler.QueueSize() != 2 {
		t.Errorf("expected 2 summoners pending in queue, got %d", crawler.QueueSize())
	}

	if crawler.CrawledMatchesCount() != 2 {
		t.Errorf("expected crawled matches count 2, got %d", crawler.CrawledMatchesCount())
	}

	// Now run crawler to completion with unlimited matches
	crawler.cfg.MaxMatches = 0
	if err := crawler.Run(context.Background()); err != nil {
		t.Fatalf("resumed Run failed: %v", err)
	}

	if !partner.Crawled {
		t.Errorf("expected partner_puuid.Crawled to be true after resumed crawl")
	}
	if !enemy.Crawled {
		t.Errorf("expected enemy_puuid.Crawled to be true after resumed crawl")
	}
	if crawler.QueueSize() != 0 {
		t.Errorf("expected queue to be empty after completion, got %d", crawler.QueueSize())
	}
}

func TestMatchCrawler_FailureDoesNotCommitPartialSummoner(t *testing.T) {
	ds := data.NewDataset()

	match1 := &data.MatchV5{
		Metadata: data.MetadataDto{
			MatchID: "NA1_1001",
		},
		Info: data.InfoDto{
			GameStartTimestamp: time.Now().UnixMilli(),
			GameDuration:       1800,
			Participants: []*data.ParticipantDto{
				{PUUID: "seed_puuid", SummonerName: "Seed", TeamID: 100},
				{PUUID: "partner_puuid", SummonerName: "Partner", TeamID: 100},
			},
		},
	}

	mockHandler := func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/lol/summoner/v4/summoners/by-puuid/seed_puuid":
			return jsonResponse(http.StatusOK, map[string]interface{}{
				"puuid":         "seed_puuid",
				"name":          "Seed",
				"summonerLevel": 120,
			})
		case "/lol/match/v5/matches/by-puuid/seed_puuid/ids":
			return jsonResponse(http.StatusOK, []string{"NA1_1001", "NA1_FAIL"})
		case "/lol/match/v5/matches/NA1_1001":
			return jsonResponse(http.StatusOK, match1)
		case "/lol/match/v5/matches/NA1_FAIL":
			return jsonResponse(http.StatusBadRequest, "bad request")
		default:
			return jsonResponse(http.StatusNotFound, nil)
		}
	}

	client := newMockRiotClient(mockHandler)
	cfg := CrawlerConfig{
		SeedPUUID: "seed_puuid",
	}

	crawler := NewMatchCrawler(client, ds, nil, cfg)
	err := crawler.Run(context.Background())
	if err != nil {
		t.Fatalf("unexpected Run error: %v", err)
	}

	// Since one match failed, NO matches should be committed for seed_puuid
	if len(ds.Matches) != 0 {
		t.Errorf("expected 0 matches in dataset, got %d", len(ds.Matches))
	}

	seed := ds.GetSummoner("seed_puuid")
	if seed != nil && seed.Crawled {
		t.Errorf("expected seed.Crawled to be false on failed crawl")
	}

	// partner_puuid should not be in dataset
	if ds.GetSummoner("partner_puuid") != nil {
		t.Errorf("expected partner_puuid to NOT be in dataset")
	}
}

func TestMatchCrawler_AlreadyCrawledParticipantNotReenqueued(t *testing.T) {
	ds := data.NewDataset()

	// Existing already-crawled summoner
	sExisting := ds.GetOrCreateSummoner("crawled_partner", "CrawledPartner")
	sExisting.Crawled = true

	match1 := &data.MatchV5{
		Metadata: data.MetadataDto{
			MatchID: "NA1_2001",
		},
		Info: data.InfoDto{
			GameStartTimestamp: time.Now().UnixMilli(),
			GameDuration:       1800,
			Participants: []*data.ParticipantDto{
				{PUUID: "seed_puuid", SummonerName: "Seed", TeamID: 100},
				{PUUID: "crawled_partner", SummonerName: "CrawledPartner", TeamID: 100},
				{PUUID: "new_player", SummonerName: "NewPlayer", TeamID: 200},
			},
		},
	}

	mockHandler := func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/lol/summoner/v4/summoners/by-puuid/seed_puuid":
			return jsonResponse(http.StatusOK, map[string]interface{}{"puuid": "seed_puuid", "name": "Seed"})
		case "/lol/match/v5/matches/by-puuid/seed_puuid/ids":
			return jsonResponse(http.StatusOK, []string{"NA1_2001"})
		case "/lol/match/v5/matches/NA1_2001":
			return jsonResponse(http.StatusOK, match1)
		case "/lol/match/v5/matches/by-puuid/new_player/ids":
			return jsonResponse(http.StatusOK, []string{})
		default:
			return jsonResponse(http.StatusOK, []string{})
		}
	}

	client := newMockRiotClient(mockHandler)
	cfg := CrawlerConfig{
		SeedPUUID: "seed_puuid",
	}

	crawler := NewMatchCrawler(client, ds, nil, cfg)
	err := crawler.Run(context.Background())
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// Verify crawled_partner was NOT re-queued/re-crawled
	if !crawler.visitedPUUIDs["crawled_partner"] {
		t.Errorf("expected crawled_partner to be visited")
	}

	// new_player was enqueued and then crawled
	newP := ds.GetSummoner("new_player")
	if newP == nil || !newP.Crawled {
		t.Errorf("expected new_player to be crawled")
	}
}

func TestMatchCrawler_RefreshFlag(t *testing.T) {
	ds := data.NewDataset()

	s1 := ds.GetOrCreateSummoner("p1", "Player1")
	s1.Crawled = true

	s2 := ds.GetOrCreateSummoner("p2", "Player2")
	s2.Crawled = true

	client := newMockRiotClient(func(req *http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, nil)
	})

	// Without refresh: both are already crawled, so queue is empty
	crawlerNoRefresh := NewMatchCrawler(client, ds, nil, CrawlerConfig{Refresh: false})
	if crawlerNoRefresh.QueueSize() != 0 {
		t.Errorf("expected queue size 0 without refresh, got %d", crawlerNoRefresh.QueueSize())
	}

	// With refresh: marks all as uncrawled, so queue has both
	crawlerRefresh := NewMatchCrawler(client, ds, nil, CrawlerConfig{Refresh: true})
	if crawlerRefresh.QueueSize() != 2 {
		t.Fatalf("expected queue size 2 with refresh, got %d", crawlerRefresh.QueueSize())
	}
	if s1.Crawled || s2.Crawled {
		t.Errorf("expected summoners to be marked Crawled = false after Refresh")
	}
}

func TestMatchCrawler_CheckpointSkipsWhenSaved(t *testing.T) {
	tmpDir := t.TempDir()
	datasetPath := filepath.Join(tmpDir, "checkpoint_test.json")

	store := NewDatasetStore(datasetPath, 3)
	ds := data.NewDataset()
	s := ds.GetOrCreateSummoner("p1", "Player1")
	s.Crawled = true

	// Initial save so ds.Saved becomes true and dataset file exists
	if err := store.Save(ds); err != nil {
		t.Fatalf("initial save failed: %v", err)
	}
	if !ds.Saved {
		t.Fatalf("expected ds.Saved == true after initial save")
	}

	proceedChan := make(chan struct{})
	client := newMockRiotClient(func(req *http.Request) (*http.Response, error) {
		<-proceedChan
		return jsonResponse(http.StatusOK, []string{})
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := CrawlerConfig{
		SeedPUUID:          "p1",
		CheckpointDuration: 20 * time.Millisecond,
	}

	crawler := NewMatchCrawler(client, ds, store, cfg)

	runDone := make(chan struct{})
	go func() {
		_ = crawler.Run(ctx)
		close(runDone)
	}()

	// Wait across multiple checkpoint intervals while ds.Saved is true
	time.Sleep(80 * time.Millisecond)

	// Since ds.Saved was true and no changes occurred, no backup files should have been created
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".backup-") {
			t.Errorf("unexpected backup created while dataset had no changes: %s", entry.Name())
		}
	}

	// Now introduce a change under lock
	crawler.mu.Lock()
	ds.GetOrCreateSummoner("p2", "Player2")
	crawler.mu.Unlock()

	if ds.Saved {
		t.Errorf("expected ds.Saved == false after adding new summoner")
	}

	// Wait for checkpoint to trigger and save
	time.Sleep(80 * time.Millisecond)

	crawler.mu.Lock()
	saved := ds.Saved
	crawler.mu.Unlock()

	if !saved {
		t.Errorf("expected ds.Saved to become true after checkpoint saved changes")
	}

	// Verify a backup was now created as part of the checkpoint save
	entries, err = os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("failed to read temp dir: %v", err)
	}
	hasBackup := false
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".backup-") {
			hasBackup = true
			break
		}
	}
	if !hasBackup {
		t.Errorf("expected backup file to be created after checkpoint saved changes")
	}

	close(proceedChan)
	<-runDone
}

func TestMatchCrawler_StartupFetchesMissingSummonerProfiles(t *testing.T) {
	ds := data.NewDataset()

	// Summoner 1: already has profile
	s1 := ds.GetOrCreateSummoner("p1", "Player1")
	s1.SummonerLevel = 50
	s1.RevisionDate = 1690000000000
	s1.Crawled = true

	// Summoner 2: missing profile
	s2 := ds.GetOrCreateSummoner("p2", "Player2")
	s2.Crawled = true // already crawled, but missing profile

	// Summoner 3: missing profile
	s3 := ds.GetOrCreateSummoner("p3", "Player3")
	s3.Crawled = false

	p2Fetched := false
	p3Fetched := false
	p1Fetched := false

	mockHandler := func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/lol/summoner/v4/summoners/by-puuid/p1":
			p1Fetched = true
			return jsonResponse(http.StatusOK, map[string]interface{}{
				"puuid":         "p1",
				"summonerLevel": 50,
				"revisionDate":  1690000000000,
			})
		case "/lol/summoner/v4/summoners/by-puuid/p2":
			p2Fetched = true
			return jsonResponse(http.StatusOK, map[string]interface{}{
				"puuid":         "p2",
				"summonerLevel": 150,
				"revisionDate":  1700000000000,
			})
		case "/lol/summoner/v4/summoners/by-puuid/p3":
			p3Fetched = true
			return jsonResponse(http.StatusOK, map[string]interface{}{
				"puuid":         "p3",
				"summonerLevel": 220,
				"revisionDate":  1710000000000,
			})
		case "/lol/match/v5/matches/by-puuid/p3/ids":
			return jsonResponse(http.StatusOK, []string{})
		default:
			return jsonResponse(http.StatusOK, []string{})
		}
	}

	client := newMockRiotClient(mockHandler)
	crawler := NewMatchCrawler(client, ds, nil, CrawlerConfig{})

	if err := crawler.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// p1 already had profile, so it should not have been fetched
	if p1Fetched {
		t.Errorf("expected p1 to NOT be fetched since it already had profile")
	}

	// p2 and p3 were missing profile, so both should have been fetched
	if !p2Fetched {
		t.Errorf("expected p2 profile to be fetched at startup")
	}
	if !p3Fetched {
		t.Errorf("expected p3 profile to be fetched at startup")
	}

	if s2.SummonerLevel != 150 || s2.RevisionDate != 1700000000000 {
		t.Errorf("expected s2 profile updated, got level=%d, rev=%d", s2.SummonerLevel, s2.RevisionDate)
	}
	if s3.SummonerLevel != 220 || s3.RevisionDate != 1710000000000 {
		t.Errorf("expected s3 profile updated, got level=%d, rev=%d", s3.SummonerLevel, s3.RevisionDate)
	}
}

func TestMatchCrawler_ParticipantProfilesFetchedImmediatelyAfterMatch(t *testing.T) {
	ds := data.NewDataset()

	match1 := &data.MatchV5{
		Metadata: data.MetadataDto{
			MatchID: "NA1_5001",
		},
		Info: data.InfoDto{
			GameStartTimestamp: time.Now().UnixMilli(),
			GameDuration:       1800,
			Participants: []*data.ParticipantDto{
				{PUUID: "seed_puuid", SummonerName: "SeedPlayer", RiotIDGameName: "SeedPlayer", RiotIDTagline: "NA1", TeamID: 100},
				{PUUID: "part1_puuid", SummonerName: "Part1", RiotIDGameName: "Part1", RiotIDTagline: "NA1", TeamID: 100},
				{PUUID: "part2_puuid", SummonerName: "Part2", RiotIDGameName: "Part2", RiotIDTagline: "NA1", TeamID: 200},
			},
		},
	}

	part1Fetched := false
	part2Fetched := false

	mockHandler := func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/lol/summoner/v4/summoners/by-puuid/seed_puuid":
			return jsonResponse(http.StatusOK, map[string]interface{}{
				"puuid":         "seed_puuid",
				"summonerLevel": 300,
				"revisionDate":  1705000000000,
			})
		case "/lol/summoner/v4/summoners/by-puuid/part1_puuid":
			part1Fetched = true
			return jsonResponse(http.StatusOK, map[string]interface{}{
				"puuid":         "part1_puuid",
				"summonerLevel": 85,
				"revisionDate":  1706000000000,
			})
		case "/lol/summoner/v4/summoners/by-puuid/part2_puuid":
			part2Fetched = true
			return jsonResponse(http.StatusOK, map[string]interface{}{
				"puuid":         "part2_puuid",
				"summonerLevel": 410,
				"revisionDate":  1707000000000,
			})
		case "/lol/match/v5/matches/by-puuid/seed_puuid/ids":
			return jsonResponse(http.StatusOK, []string{"NA1_5001"})
		case "/lol/match/v5/matches/NA1_5001":
			return jsonResponse(http.StatusOK, match1)
		default:
			return jsonResponse(http.StatusOK, []string{})
		}
	}

	client := newMockRiotClient(mockHandler)
	cfg := CrawlerConfig{
		SeedPUUID:  "seed_puuid",
		MaxMatches: 1,
	}

	crawler := NewMatchCrawler(client, ds, nil, cfg)
	if err := crawler.Run(context.Background()); err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !part1Fetched {
		t.Errorf("expected part1 profile to be fetched immediately after match retrieval")
	}
	if !part2Fetched {
		t.Errorf("expected part2 profile to be fetched immediately after match retrieval")
	}

	p1 := ds.GetSummoner("part1_puuid")
	if p1 == nil {
		t.Fatalf("part1_puuid not found in dataset")
	}
	if p1.SummonerLevel != 85 || p1.RevisionDate != 1706000000000 {
		t.Errorf("expected p1 level=85, rev=1706000000000, got level=%d, rev=%d", p1.SummonerLevel, p1.RevisionDate)
	}

	p2 := ds.GetSummoner("part2_puuid")
	if p2 == nil {
		t.Fatalf("part2_puuid not found in dataset")
	}
	if p2.SummonerLevel != 410 || p2.RevisionDate != 1707000000000 {
		t.Errorf("expected p2 level=410, rev=1707000000000, got level=%d, rev=%d", p2.SummonerLevel, p2.RevisionDate)
	}
}

func TestMatchCrawler_StartupCheckpointsWhileFetchingProfiles(t *testing.T) {
	tmpDir := t.TempDir()
	datasetPath := filepath.Join(tmpDir, "startup_checkpoints.json")

	store := NewDatasetStore(datasetPath, 3)
	ds := data.NewDataset()

	// 3 summoners missing profile
	ds.GetOrCreateSummoner("p1", "Player1")
	ds.GetOrCreateSummoner("p2", "Player2")
	ds.GetOrCreateSummoner("p3", "Player3")

	// Initial save
	if err := store.Save(ds); err != nil {
		t.Fatalf("initial save failed: %v", err)
	}

	proceedChan := make(chan struct{})
	mockHandler := func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/lol/summoner/v4/summoners/by-puuid/p1":
			return jsonResponse(http.StatusOK, map[string]interface{}{
				"puuid":         "p1",
				"summonerLevel": 100,
				"revisionDate":  1700000000000,
			})
		case "/lol/summoner/v4/summoners/by-puuid/p2":
			<-proceedChan
			return jsonResponse(http.StatusOK, map[string]interface{}{
				"puuid":         "p2",
				"summonerLevel": 200,
				"revisionDate":  1700000000000,
			})
		case "/lol/summoner/v4/summoners/by-puuid/p3":
			return jsonResponse(http.StatusOK, map[string]interface{}{
				"puuid":         "p3",
				"summonerLevel": 300,
				"revisionDate":  1700000000000,
			})
		default:
			return jsonResponse(http.StatusOK, []string{})
		}
	}

	client := newMockRiotClient(mockHandler)
	cfg := CrawlerConfig{
		CheckpointDuration: 20 * time.Millisecond,
	}

	crawler := NewMatchCrawler(client, ds, store, cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runDone := make(chan struct{})
	go func() {
		_ = crawler.Run(ctx)
		close(runDone)
	}()

	// Wait for p1 to be updated and checkpoint ticker to fire while waiting on p2
	time.Sleep(80 * time.Millisecond)

	// Unblock p2 and let crawl complete
	close(proceedChan)
	<-runDone

	// Reload from store file to verify it was saved on disk
	loadedDs := data.NewDataset()
	if err := store.Load(loadedDs); err != nil {
		t.Fatalf("failed to load dataset: %v", err)
	}

	if loadedDs.NumSummonersWithProfile() != 3 {
		t.Errorf("expected 3 summoners with profile in saved dataset, got %d", loadedDs.NumSummonersWithProfile())
	}
}


