package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
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

