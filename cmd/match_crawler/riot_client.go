package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/janpfeifer/loldata/data"
)

var (
	// ErrNotFound indicates the requested resource was not found (HTTP 404).
	ErrNotFound = errors.New("resource not found")
	// ErrUnauthorized indicates an invalid or expired Riot API key (HTTP 401/403).
	ErrUnauthorized = errors.New("unauthorized / invalid Riot API key")
	// ErrDecryptionMismatch indicates a PUUID/ID was encrypted for a different API key (HTTP 400 Decryption Exception).
	ErrDecryptionMismatch = errors.New("identifier encrypted with different API key")
)

// Platform aliases map common platform/region names (e.g. "euw", "eune", "na") to canonical Riot platform IDs.
var platformAliases = map[string]string{
	"euw":      "euw1",
	"euw1":     "euw1",
	"eun":      "eun1",
	"eune":     "eun1",
	"eun1":     "eun1",
	"na":       "na1",
	"na1":      "na1",
	"br":       "br1",
	"br1":      "br1",
	"kr":       "kr",
	"jp":       "jp1",
	"jp1":      "jp1",
	"lan":      "la1",
	"la1":      "la1",
	"las":      "la2",
	"la2":      "la2",
	"oce":      "oc1",
	"oc":       "oc1",
	"oc1":      "oc1",
	"tr":       "tr1",
	"tr1":      "tr1",
	"ru":       "ru",
	"me":       "me1",
	"me1":      "me1",
	"sg":       "sg2",
	"sg2":      "sg2",
	"ph":       "ph2",
	"ph2":      "ph2",
	"th":       "th2",
	"th2":      "th2",
	"tw":       "tw2",
	"tw2":      "tw2",
	"vn":       "vn2",
	"vn2":      "vn2",
	// Regional mappings:
	"americas": "na1",
	"europe":   "euw1",
	"asia":     "kr",
	"sea":      "sg2",
}

// Platform to Regional routing mapping.
var platformToRegional = map[string]string{
	"na1":  "americas",
	"br1":  "americas",
	"la1":  "americas",
	"la2":  "americas",
	"euw1": "europe",
	"eun1": "europe",
	"tr1":  "europe",
	"ru":   "europe",
	"me1":  "europe",
	"kr":   "asia",
	"jp1":  "asia",
	"oc1":  "sea",
	"ph2":  "sea",
	"sg2":  "sea",
	"th2":  "sea",
	"tw2":  "sea",
	"vn2":  "sea",
}

// RegionToPlatform maps regional routing values to default platforms.
var regionToPlatform = map[string]string{
	"americas": "na1",
	"europe":   "euw1",
	"asia":     "kr",
	"sea":      "sg2",
}

// NormalizePlatform maps any user-supplied platform or region string to (canonicalPlatform, regionalRoute).
func NormalizePlatform(input string) (platform string, regional string) {
	p := strings.ToLower(strings.TrimSpace(input))
	if p == "" {
		p = "na1"
	}

	if canonical, ok := platformAliases[p]; ok {
		platform = canonical
	} else {
		platform = p
	}

	if reg, ok := platformToRegional[platform]; ok {
		regional = reg
	} else if p == "americas" || p == "europe" || p == "asia" || p == "sea" {
		regional = p
		platform = regionToPlatform[p]
	} else {
		regional = "americas"
	}

	return platform, regional
}

// RiotClient interacts with the Riot Games API (Account-V1, Summoner-V4, Match-V5)
// adhering strictly to rate limits using a shared Throttler.
type RiotClient struct {
	apiKey     string
	platform   string
	regional   string
	throttler  *Throttler
	httpClient *http.Client
	verbose    bool
}

// NewRiotClient creates a configured RiotClient.
func NewRiotClient(apiKey, platform string, throttler *Throttler, verbose bool) *RiotClient {
	canonPlatform, regional := NormalizePlatform(platform)

	return &RiotClient{
		apiKey:    apiKey,
		platform:  canonPlatform,
		regional:  regional,
		throttler: throttler,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		verbose: verbose,
	}
}

// Platform returns the canonical platform routing code (e.g. "euw1", "na1").
func (c *RiotClient) Platform() string {
	return c.platform
}

// Regional returns the regional routing value (e.g. "europe", "americas").
func (c *RiotClient) Regional() string {
	return c.regional
}

// ResolveRegionalForMatch determines the regional routing URL for a given match ID.
func (c *RiotClient) ResolveRegionalForMatch(matchID string) string {
	parts := strings.SplitN(matchID, "_", 2)
	if len(parts) > 1 {
		prefix := strings.ToLower(parts[0])
		if reg, exists := platformToRegional[prefix]; exists {
			return reg
		}
	}
	return c.regional
}

// executeRequest performs an HTTP GET request with global rate throttling and retry handling.
func (c *RiotClient) executeRequest(ctx context.Context, reqURL string) ([]byte, error) {
	maxRetries := 4

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Acquire token from global throttler
		if err := c.throttler.Acquire(ctx); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP request: %w", err)
		}

		req.Header.Set("X-Riot-Token", c.apiKey)
		req.Header.Set("Accept", "application/json")

		if c.verbose {
			fmt.Printf("HTTP GET %s (Attempt %d)\n", reqURL, attempt+1)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}
			// Transient network failure, retry with backoff
			backoff := time.Duration(1<<attempt) * 500 * time.Millisecond
			time.Sleep(backoff)
			continue
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to read response body: %w", err)
		}

		switch resp.StatusCode {
		case http.StatusOK:
			return body, nil

		case http.StatusNotFound:
			return nil, ErrNotFound

		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, fmt.Errorf("%w: HTTP status %d for %s", ErrUnauthorized, resp.StatusCode, reqURL)

		case http.StatusTooManyRequests:
			// Rate limit exceeded (HTTP 429)
			retryAfterSec := 5
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if s, err := strconv.Atoi(ra); err == nil && s > 0 {
					retryAfterSec = s
				}
			}
			retryDur := time.Duration(retryAfterSec) * time.Second
			if c.verbose {
				fmt.Printf("[429 Rate Limit] Backing off for %v on %s\n", retryDur, reqURL)
			}
			c.throttler.ForceBackoff(retryDur)

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDur + 100*time.Millisecond):
				continue
			}

		case http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			// Server error, back off and retry
			backoff := time.Duration(1<<attempt) * time.Second
			if c.verbose {
				fmt.Printf("[%d Server Error] Retrying after %v...\n", resp.StatusCode, backoff)
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
				continue
			}

		case http.StatusBadRequest:
			bodyStr := strings.TrimSpace(string(body))
			if strings.Contains(bodyStr, "Exception decrypting") {
				return nil, fmt.Errorf("%w: %s", ErrDecryptionMismatch, bodyStr)
			}
			return nil, fmt.Errorf("unexpected HTTP status 400: %s", bodyStr)

		default:
			return nil, fmt.Errorf("unexpected HTTP status %d: %s", resp.StatusCode, string(body))
		}
	}

	return nil, fmt.Errorf("max retries exceeded for %s", reqURL)
}

// AccountDto represents Riot Account-V1 response.
type AccountDto struct {
	PUUID    string `json:"puuid"`
	GameName string `json:"gameName"`
	TagLine  string `json:"tagLine"`
}

// GetAccountByRiotID retrieves account details (including new encrypted PUUID) using Riot ID gameName and tagLine.
func (c *RiotClient) GetAccountByRiotID(ctx context.Context, gameName, tagLine string) (*AccountDto, error) {
	reqURL := fmt.Sprintf("https://%s.api.riotgames.com/riot/account/v1/accounts/by-riot-id/%s/%s",
		c.regional, url.PathEscape(gameName), url.PathEscape(tagLine))
	body, err := c.executeRequest(ctx, reqURL)
	if err != nil {
		return nil, err
	}
	var acc AccountDto
	if err := json.Unmarshal(body, &acc); err != nil {
		return nil, fmt.Errorf("failed to parse account response: %w", err)
	}
	return &acc, nil
}

// ResolveSeedToPUUID converts a seed string (Riot ID 'Name#Tag', PUUID, or summoner name) into a PUUID.
func (c *RiotClient) ResolveSeedToPUUID(ctx context.Context, seed string) (string, error) {
	seed = strings.TrimSpace(seed)
	if seed == "" {
		return "", errors.New("empty seed provided")
	}

	// 1. If seed contains '#', look up by Riot ID
	if strings.Contains(seed, "#") {
		parts := strings.SplitN(seed, "#", 2)
		acc, err := c.GetAccountByRiotID(ctx, parts[0], parts[1])
		if err != nil {
			return "", fmt.Errorf("failed to lookup Riot ID %q: %w", seed, err)
		}
		return acc.PUUID, nil
	}

	// 2. If seed is a 78-character string, try looking up as PUUID directly
	if len(seed) >= 70 {
		reqURL := fmt.Sprintf("https://%s.api.riotgames.com/riot/account/v1/accounts/by-puuid/%s", c.regional, seed)
		if body, err := c.executeRequest(ctx, reqURL); err == nil {
			var acc AccountDto
			if err := json.Unmarshal(body, &acc); err == nil && acc.PUUID != "" {
				return acc.PUUID, nil
			}
		}
	}

	// 3. Try with platform default tag (e.g. GameName#NA1)
	defaultTag := strings.ToUpper(c.platform)
	reqURL := fmt.Sprintf("https://%s.api.riotgames.com/riot/account/v1/accounts/by-riot-id/%s/%s", c.regional, url.PathEscape(seed), url.PathEscape(defaultTag))
	body, err := c.executeRequest(ctx, reqURL)
	if err == nil {
		var acc AccountDto
		if err := json.Unmarshal(body, &acc); err == nil && acc.PUUID != "" {
			return acc.PUUID, nil
		}
	}

	// 4. Try legacy summoner by-name endpoint
	summonerURL := fmt.Sprintf("https://%s.api.riotgames.com/lol/summoner/v4/summoners/by-name/%s", c.platform, url.PathEscape(seed))
	if body, err := c.executeRequest(ctx, summonerURL); err == nil {
		var s data.SummonerV4
		if err := json.Unmarshal(body, &s); err == nil && s.PUUID != "" {
			return s.PUUID, nil
		}
	}

	return "", fmt.Errorf("unable to resolve seed %q to PUUID. Please specify as 'GameName#TagLine' (e.g. 'Faker#KR1')", seed)
}

// GetSummonerByPUUID retrieves the Summoner profile from Summoner-V4 API.
func (c *RiotClient) GetSummonerByPUUID(ctx context.Context, puuid string) (*data.SummonerV4, error) {
	reqURL := fmt.Sprintf("https://%s.api.riotgames.com/lol/summoner/v4/summoners/by-puuid/%s", c.platform, puuid)
	body, err := c.executeRequest(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var summoner data.SummonerV4
	if err := json.Unmarshal(body, &summoner); err != nil {
		return nil, fmt.Errorf("failed to parse Summoner-V4 response: %w", err)
	}

	return &summoner, nil
}

// MaxMatchIDsCount is the maximum page count allowed by Riot API for match IDs list (count=100).
const MaxMatchIDsCount = 100

// GetMatchIDsByPUUID retrieves a list of match IDs for a player within [startTime, endTime].
// Always requests with type=ranked to filter for ranked matches only, and maximum count (=100).
func (c *RiotClient) GetMatchIDsByPUUID(ctx context.Context, puuid string, startTime, endTime time.Time, maxMatches int) ([]string, error) {
	var matchIDs []string
	startIdx := 0
	pageSize := MaxMatchIDsCount

	for {
		params := url.Values{}
		params.Set("start", strconv.Itoa(startIdx))
		params.Set("count", strconv.Itoa(pageSize))
		params.Set("type", "ranked")

		if !startTime.IsZero() {
			params.Set("startTime", strconv.FormatInt(startTime.Unix(), 10))
		}
		if !endTime.IsZero() {
			params.Set("endTime", strconv.FormatInt(endTime.Unix(), 10))
		}

		reqURL := fmt.Sprintf("https://%s.api.riotgames.com/lol/match/v5/matches/by-puuid/%s/ids?%s", c.regional, puuid, params.Encode())
		body, err := c.executeRequest(ctx, reqURL)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				return matchIDs, nil
			}
			return matchIDs, err
		}

		var batch []string
		if err := json.Unmarshal(body, &batch); err != nil {
			return matchIDs, fmt.Errorf("failed to parse match IDs response: %w", err)
		}

		if len(batch) == 0 {
			break
		}

		matchIDs = append(matchIDs, batch...)
		if maxMatches > 0 && len(matchIDs) >= maxMatches {
			matchIDs = matchIDs[:maxMatches]
			break
		}

		if len(batch) < pageSize {
			break
		}

		startIdx += len(batch)
	}

	return matchIDs, nil
}

// GetMatch retrieves the full MatchV5 details for a match ID.
func (c *RiotClient) GetMatch(ctx context.Context, matchID string) (*data.MatchV5, error) {
	reg := c.ResolveRegionalForMatch(matchID)
	reqURL := fmt.Sprintf("https://%s.api.riotgames.com/lol/match/v5/matches/%s", reg, matchID)

	body, err := c.executeRequest(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var match data.MatchV5
	if err := json.Unmarshal(body, &match); err != nil {
		return nil, fmt.Errorf("failed to parse MatchV5 response for %s: %w", matchID, err)
	}

	// Normalize positions for participants
	for _, p := range match.Info.Participants {
		if p == nil {
			continue
		}
		if p.Position == data.PositionUnknown {
			if p.TeamPosition != "" {
				p.Position = data.ParsePosition(p.TeamPosition)
			} else if p.IndividualPosition != "" {
				p.Position = data.ParsePosition(p.IndividualPosition)
			}
		}
	}

	return &match, nil
}

// LeagueEntryDto represents a player's league standing in a ranked queue.
type LeagueEntryDto struct {
	LeagueID     string `json:"leagueId"`
	SummonerID   string `json:"summonerId"`
	PUUID        string `json:"puuid,omitempty"`
	QueueType    string `json:"queueType"`
	Tier         string `json:"tier"`
	Rank         string `json:"rank"`
	LeaguePoints int    `json:"leaguePoints"`
	Wins         int    `json:"wins"`
	Losses       int    `json:"losses"`
	HotStreak    bool   `json:"hotStreak"`
	Veteran      bool   `json:"veteran"`
	FreshBlood   bool   `json:"freshBlood"`
	Inactive     bool   `json:"inactive"`
}

// LeagueItemDto represents an individual player entry in an apex league list.
type LeagueItemDto struct {
	SummonerID   string `json:"summonerId"`
	PUUID        string `json:"puuid,omitempty"`
	LeaguePoints int    `json:"leaguePoints"`
	Rank         string `json:"rank"`
	Wins         int    `json:"wins"`
	Losses       int    `json:"losses"`
	Veteran      bool   `json:"veteran"`
	Inactive     bool   `json:"inactive"`
	FreshBlood   bool   `json:"freshBlood"`
	HotStreak    bool   `json:"hotStreak"`
}

// LeagueListDto represents an apex league (Challenger, Grandmaster, Master).
type LeagueListDto struct {
	LeagueID string          `json:"leagueId"`
	Entries  []LeagueItemDto `json:"entries"`
	Tier     string          `json:"tier"`
	Name     string          `json:"name"`
	Queue    string          `json:"queue"`
}

// GetLeagueEntriesBySummonerID retrieves ranked league entries for a summoner by encrypted summoner ID.
func (c *RiotClient) GetLeagueEntriesBySummonerID(ctx context.Context, summonerID string) ([]LeagueEntryDto, error) {
	reqURL := fmt.Sprintf("https://%s.api.riotgames.com/lol/league/v4/entries/by-summoner/%s", c.platform, summonerID)
	body, err := c.executeRequest(ctx, reqURL)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	var entries []LeagueEntryDto
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse League-V4 entries response: %w", err)
	}

	return entries, nil
}

// GetApexLeague retrieves the full apex league ladder for a tier ("challenger", "grandmaster", "master").
func (c *RiotClient) GetApexLeague(ctx context.Context, tier string, queue string) (*LeagueListDto, error) {
	tierLower := strings.ToLower(strings.TrimSpace(tier))
	reqURL := fmt.Sprintf("https://%s.api.riotgames.com/lol/league/v4/%sleagues/by-queue/%s", c.platform, tierLower, queue)
	body, err := c.executeRequest(ctx, reqURL)
	if err != nil {
		return nil, err
	}

	var list LeagueListDto
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("failed to parse %s league list: %w", tier, err)
	}

	return &list, nil
}

// GetLeagueEntriesPage retrieves a page of ranked league entries for a tier and division (up to 205 entries per page).
func (c *RiotClient) GetLeagueEntriesPage(ctx context.Context, queue, tier, division string, page int) ([]LeagueEntryDto, error) {
	reqURL := fmt.Sprintf("https://%s.api.riotgames.com/lol/league/v4/entries/%s/%s/%s?page=%d",
		c.platform, queue, strings.ToUpper(tier), strings.ToUpper(division), page)
	body, err := c.executeRequest(ctx, reqURL)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}

	var entries []LeagueEntryDto
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("failed to parse league entries page %d for %s %s: %w", page, tier, division, err)
	}

	return entries, nil
}

