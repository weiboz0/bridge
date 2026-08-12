// Package realtime contains the Go-side client for the private Hocuspocus
// canvas lifecycle listener.  This is deliberately a small, strict protocol:
// it is an auth boundary, not a general purpose HTTP client.
package realtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	canvasControlFreezeBudget = 2 * time.Second
	maxControlResponseBytes   = 48 * 1024 * 1024
)

var errCanvasControlRedirect = errors.New("canvas control redirects are refused")

type CanvasControlConfig struct {
	URL        string
	Secret     string
	HTTPClient *http.Client
}

type CanvasControlClient struct {
	baseURL string
	secret  string
	client  *http.Client
}

type FreezeRequest struct {
	SessionID string   `json:"sessionId"`
	Token     string   `json:"token"`
	CanvasIDs []string `json:"canvasIds"`
}

type CanvasSnapshot struct {
	CanvasID string
	State    []byte
	SHA256   string
}

type FreezeBundle struct {
	Entries []CanvasSnapshot
}

// NewCanvasControlClient refuses clear-text destinations except an IPv4
// numeric loopback address. HTTPS retains the standard Go TLS verifier; no
// insecure TLS escape hatch exists in this client.
func NewCanvasControlClient(cfg CanvasControlConfig) (*CanvasControlClient, error) {
	if strings.TrimSpace(cfg.Secret) == "" {
		return nil, errors.New("canvas control secret is required")
	}
	u, err := validateControlURL(cfg.URL)
	if err != nil {
		return nil, err
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{}
	}
	if u.Scheme == "https" && transportDisablesTLSVerification(client.Transport) {
		return nil, errors.New("canvas control HTTPS requires certificate verification")
	}
	copyClient := *client
	copyClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return errCanvasControlRedirect
	}
	return &CanvasControlClient{baseURL: strings.TrimRight(u.String(), "/"), secret: cfg.Secret, client: &copyClient}, nil
}

func transportDisablesTLSVerification(rt http.RoundTripper) bool {
	if rt == nil {
		return false
	}
	transport, ok := rt.(*http.Transport)
	if !ok || transport.TLSClientConfig == nil {
		return false
	}
	var cfg *tls.Config = transport.TLSClientConfig
	return cfg.InsecureSkipVerify
}

// ValidateControlURL is shared with config startup validation.
func ValidateControlURL(raw string) error {
	_, err := validateControlURL(raw)
	return err
}

func validateControlURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || !u.IsAbs() || u.Host == "" {
		return nil, errors.New("invalid canvas control URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("canvas control URL must be an origin")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("canvas control URL must use HTTP or HTTPS")
	}
	if u.Scheme == "http" {
		host := u.Hostname()
		ip := net.ParseIP(host)
		if ip == nil || ip.To4() == nil || !ip.IsLoopback() {
			return nil, errors.New("plaintext canvas control URL must use numeric IPv4 loopback")
		}
	}
	return u, nil
}

func (c *CanvasControlClient) Freeze(ctx context.Context, request FreezeRequest) (FreezeBundle, error) {
	if err := validateFreezeRequest(request); err != nil {
		return FreezeBundle{}, err
	}
	deadline := time.Now().Add(canvasControlFreezeBudget)
	if parentDeadline, ok := ctx.Deadline(); ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline
	}
	freezeCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()

	var lastErr error
	for {
		bundle, retry, err := c.freezeOnce(freezeCtx, request)
		if err == nil {
			return bundle, nil
		}
		lastErr = err
		if !retry || freezeCtx.Err() != nil || time.Until(deadline) <= 0 {
			return FreezeBundle{}, fmt.Errorf("canvas freeze failed: %w", lastErr)
		}
		// A tiny bounded yield avoids a hot loop while retaining practically all
		// of the two-second lease budget for an immediately cached retry.
		select {
		case <-freezeCtx.Done():
			return FreezeBundle{}, fmt.Errorf("canvas freeze failed: %w", lastErr)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func validateFreezeRequest(request FreezeRequest) error {
	if _, err := uuid.Parse(request.SessionID); err != nil || request.SessionID == "" {
		return errors.New("invalid canvas freeze session ID")
	}
	if _, err := uuid.Parse(request.Token); err != nil || request.Token == "" {
		return errors.New("invalid canvas freeze token")
	}
	seen := make(map[string]struct{}, len(request.CanvasIDs))
	for _, id := range request.CanvasIDs {
		if _, err := uuid.Parse(id); err != nil || id == "" {
			return errors.New("invalid canvas ID")
		}
		if _, exists := seen[id]; exists {
			return errors.New("duplicate canvas ID")
		}
		seen[id] = struct{}{}
	}
	return nil
}

func (c *CanvasControlClient) freezeOnce(ctx context.Context, request FreezeRequest) (FreezeBundle, bool, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return FreezeBundle{}, false, err
	}
	req, err := c.newRequest(ctx, "/freeze", body)
	if err != nil {
		return FreezeBundle{}, false, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, errCanvasControlRedirect) {
			return FreezeBundle{}, false, errCanvasControlRedirect
		}
		return FreezeBundle{}, true, errors.New("control transport unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return FreezeBundle{}, resp.StatusCode >= 500, fmt.Errorf("control returned HTTP %d", resp.StatusCode)
	}
	data, err := readBounded(resp.Body, maxControlResponseBytes)
	if err != nil {
		return FreezeBundle{}, false, err
	}
	bundle, err := validateFreezeBundle(data, request.CanvasIDs)
	if err != nil {
		return FreezeBundle{}, false, err
	}
	return bundle, false, nil
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, errors.New("control response read failed")
	}
	if int64(len(data)) > limit {
		return nil, errors.New("control response exceeds size limit")
	}
	return data, nil
}

func validateFreezeBundle(data []byte, requestedIDs []string) (FreezeBundle, error) {
	type entry struct {
		CanvasID string `json:"canvasId"`
		State    string `json:"state"`
		SHA256   string `json:"sha256"`
	}
	type payload struct {
		Entries []entry `json:"entries"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var response payload
	if err := decoder.Decode(&response); err != nil {
		return FreezeBundle{}, errors.New("invalid canvas control bundle")
	}
	if decoder.Decode(&struct{}{}) != io.EOF {
		return FreezeBundle{}, errors.New("invalid canvas control bundle")
	}
	if response.Entries == nil || len(response.Entries) != len(requestedIDs) {
		return FreezeBundle{}, errors.New("canvas control bundle has wrong entry count")
	}
	expected := make(map[string]struct{}, len(requestedIDs))
	for _, id := range requestedIDs {
		expected[id] = struct{}{}
	}
	bundle := FreezeBundle{Entries: make([]CanvasSnapshot, 0, len(response.Entries))}
	for _, candidate := range response.Entries {
		if _, ok := expected[candidate.CanvasID]; !ok {
			return FreezeBundle{}, errors.New("canvas control bundle has unexpected canvas")
		}
		delete(expected, candidate.CanvasID)
		if _, err := uuid.Parse(candidate.CanvasID); err != nil {
			return FreezeBundle{}, errors.New("canvas control bundle has invalid canvas ID")
		}
		if len(candidate.SHA256) != sha256.Size*2 || strings.ToLower(candidate.SHA256) != candidate.SHA256 {
			return FreezeBundle{}, errors.New("canvas control bundle has non-canonical digest")
		}
		if _, err := hex.DecodeString(candidate.SHA256); err != nil {
			return FreezeBundle{}, errors.New("canvas control bundle has invalid digest")
		}
		state, err := base64.StdEncoding.DecodeString(candidate.State)
		if err != nil || base64.StdEncoding.EncodeToString(state) != candidate.State {
			return FreezeBundle{}, errors.New("canvas control bundle has non-canonical state")
		}
		digest := sha256.Sum256(state)
		if hex.EncodeToString(digest[:]) != candidate.SHA256 {
			return FreezeBundle{}, errors.New("canvas control bundle digest mismatch")
		}
		bundle.Entries = append(bundle.Entries, CanvasSnapshot{CanvasID: candidate.CanvasID, State: state, SHA256: candidate.SHA256})
	}
	if len(expected) != 0 {
		return FreezeBundle{}, errors.New("canvas control bundle is incomplete")
	}
	return bundle, nil
}

// Complete and Unfreeze are terminal best effort calls. A durable database
// outcome is authoritative; cleanup failure must not reverse it.
func (c *CanvasControlClient) Complete(ctx context.Context, sessionID, token string) {
	c.terminal(ctx, "/complete", sessionID, token)
}

func (c *CanvasControlClient) Unfreeze(ctx context.Context, sessionID, token string) {
	c.terminal(ctx, "/unfreeze", sessionID, token)
}

func (c *CanvasControlClient) terminal(ctx context.Context, path, sessionID, token string) {
	if validateFreezeRequest(FreezeRequest{SessionID: sessionID, Token: token}) != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, canvasControlFreezeBudget)
	defer cancel()
	body, err := json.Marshal(map[string]string{"sessionId": sessionID, "token": token})
	if err != nil {
		return
	}
	req, err := c.newRequest(ctx, path, body)
	if err != nil {
		return
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
}

func (c *CanvasControlClient) newRequest(ctx context.Context, path string, body []byte) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.secret)
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}
