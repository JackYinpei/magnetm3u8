package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

const (
	defaultTurnTTLSeconds = 3600
)

// IceServer describes a TURN/STUN server entry returned to clients.
type IceServer struct {
	URLs       []string `json:"urls"`
	Username   string   `json:"username,omitempty"`
	Credential string   `json:"credential,omitempty"`
}

// cloudflareIceResponse mirrors the response from Cloudflare's TURN API.
type cloudflareIceResponse struct {
	IceServers []IceServer `json:"iceServers"`
}

// IceServerProvider manages retrieval and caching of Cloudflare TURN credentials.
type IceServerProvider struct {
	apiToken  string
	accountID string
	cacheTTL  time.Duration
	client    *http.Client

	mu        sync.RWMutex
	cache     []IceServer
	expiresAt time.Time
}

// NewIceServerProviderFromEnv constructs a provider based on environment variables.
func NewIceServerProviderFromEnv() *IceServerProvider {
	apiToken := os.Getenv("CLOUDFLARE_TURN_API_TOKEN")
	accountID := os.Getenv("CLOUDFLARE_ACCOUNT_ID")

	ttlSeconds := defaultTurnTTLSeconds
	if raw := os.Getenv("CLOUDFLARE_TURN_TTL"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			ttlSeconds = parsed
		}
	}

	return NewIceServerProvider(apiToken, accountID, time.Duration(ttlSeconds)*time.Second)
}

// NewIceServerProvider creates a provider with the given credentials and cache TTL.
func NewIceServerProvider(apiToken, accountID string, ttl time.Duration) *IceServerProvider {
	if ttl <= 0 {
		ttl = time.Duration(defaultTurnTTLSeconds) * time.Second
	}

	return &IceServerProvider{
		apiToken:  apiToken,
		accountID: accountID,
		cacheTTL:  ttl,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Enabled indicates whether the provider has sufficient configuration to operate.
func (p *IceServerProvider) Enabled() bool {
	return p != nil && p.apiToken != "" && p.accountID != ""
}

// Get returns cached ICE servers or fetches fresh credentials when necessary.
func (p *IceServerProvider) Get() ([]IceServer, time.Duration, error) {
	if !p.Enabled() {
		return nil, 0, errors.New("Cloudflare TURN not configured")
	}

	p.mu.RLock()
	if len(p.cache) > 0 && time.Now().Before(p.expiresAt) {
		ttl := time.Until(p.expiresAt)
		cacheCopy := make([]IceServer, len(p.cache))
		copy(cacheCopy, p.cache)
		p.mu.RUnlock()
		return cacheCopy, ttl, nil
	}
	p.mu.RUnlock()

	servers, err := p.fetch()
	if err != nil {
		return nil, 0, err
	}

	p.mu.Lock()
	p.cache = make([]IceServer, len(servers))
	copy(p.cache, servers)
	p.expiresAt = time.Now().Add(p.cacheTTL)
	cacheCopy := make([]IceServer, len(p.cache))
	copy(cacheCopy, p.cache)
	p.mu.Unlock()

	return cacheCopy, p.cacheTTL, nil
}

func (p *IceServerProvider) fetch() ([]IceServer, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	endpoint := fmt.Sprintf("https://rtc.live.cloudflare.com/v1/turn/keys/%s/credentials/generate-ice-servers", p.accountID)

	requestBody := map[string]interface{}{
		"ttl": int(p.cacheTTL.Seconds()),
	}

	payload, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", p.apiToken))
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request Cloudflare TURN credentials: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read Cloudflare response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("Cloudflare TURN API returned %s: %s", resp.Status, string(body))
	}

	var parsed cloudflareIceResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse Cloudflare response: %w", err)
	}

	if len(parsed.IceServers) == 0 {
		return nil, errors.New("Cloudflare TURN API returned no iceServers")
	}

	return parsed.IceServers, nil
}
