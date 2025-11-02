package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	webrtcLib "github.com/pion/webrtc/v3"
)

const defaultGatewayTurnTTL = time.Hour

type iceServersResponse struct {
	Success    bool                  `json:"success"`
	IceServers []webrtcLib.ICEServer `json:"iceServers"`
	TTL        int                   `json:"ttl"`
	Error      string                `json:"error"`
	Message    string                `json:"message"`
}

func (w *Worker) ensureWebRTCConfiguration() webrtcLib.Configuration {
	w.iceConfigMu.RLock()
	if len(w.iceTurnServers) > 0 && w.now().Before(w.iceConfigExpiry) {
		cached := make([]webrtcLib.ICEServer, len(w.iceTurnServers))
		copy(cached, w.iceTurnServers)
		w.iceConfigMu.RUnlock()
		return w.composeWebRTCConfiguration(cached)
	}
	w.iceConfigMu.RUnlock()

	turnServers, ttl, err := w.fetchTurnServersFromGateway()
	if err != nil {
		log.Printf("Failed to retrieve TURN servers from gateway: %v", err)
		return w.composeWebRTCConfiguration(nil)
	}

	if ttl <= 0 {
		ttl = defaultGatewayTurnTTL
	}

	w.iceConfigMu.Lock()
	w.iceTurnServers = make([]webrtcLib.ICEServer, len(turnServers))
	copy(w.iceTurnServers, turnServers)
	w.iceConfigExpiry = w.now().Add(ttl)
	cached := make([]webrtcLib.ICEServer, len(w.iceTurnServers))
	copy(cached, w.iceTurnServers)
	w.iceConfigMu.Unlock()

	return w.composeWebRTCConfiguration(cached)
}

func (w *Worker) fetchTurnServersFromGateway() ([]webrtcLib.ICEServer, time.Duration, error) {
	baseURL, err := w.gatewayAPIBase()
	if err != nil {
		return nil, 0, err
	}

	endpoint := fmt.Sprintf("%s/api/webrtc/ice-servers", baseURL)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request gateway: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, 0, fmt.Errorf("gateway returned status %s", resp.Status)
	}

	var payload iceServersResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, 0, fmt.Errorf("decode gateway response: %w", err)
	}

	if !payload.Success {
		message := payload.Error
		if message == "" {
			message = payload.Message
		}
		if message == "" {
			message = "gateway reported failure retrieving ICE servers"
		}
		return nil, 0, fmt.Errorf(message)
	}

	ttl := time.Duration(payload.TTL) * time.Second
	return payload.IceServers, ttl, nil
}

func (w *Worker) composeWebRTCConfiguration(turnServers []webrtcLib.ICEServer) webrtcLib.Configuration {
	var config webrtcLib.Configuration

	for _, entry := range w.config.Network.STUNServers {
		urlValue := strings.TrimSpace(entry)
		if urlValue == "" {
			continue
		}

		prefix := strings.ToLower(urlValue)
		if !strings.HasPrefix(prefix, "stun:") && !strings.HasPrefix(prefix, "turn:") && !strings.HasPrefix(prefix, "turns:") {
			urlValue = "stun:" + urlValue
		}

		config.ICEServers = append(config.ICEServers, webrtcLib.ICEServer{
			URLs: []string{urlValue},
		})
	}

	config.ICEServers = append(config.ICEServers, turnServers...)
	return config
}

func (w *Worker) gatewayAPIBase() (string, error) {
	raw := strings.TrimSpace(w.config.Gateway.URL)
	if raw == "" {
		return "", fmt.Errorf("gateway URL is empty")
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("parse gateway URL: %w", err)
	}

	switch parsed.Scheme {
	case "ws":
		parsed.Scheme = "http"
	case "wss":
		parsed.Scheme = "https"
	case "http", "https":
	default:
		return "", fmt.Errorf("unsupported gateway URL scheme: %s", parsed.Scheme)
	}

	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return strings.TrimRight(parsed.String(), "/"), nil
}
