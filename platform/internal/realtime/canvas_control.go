// Package realtime owns the strict, private Go-to-Hocuspocus lifecycle wire.
package realtime

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	canvasControlFreezeBudget = 2 * time.Second
	maxControlResponseBytes   = 48 * 1024 * 1024
	maxSnapshots              = 50
	maxSnapshotBytes          = 8 * 1024 * 1024
	maxAggregateBytes         = 32 * 1024 * 1024
)

var errCanvasControlRedirect = errors.New("canvas control redirects are refused")

type CanvasControlConfig struct {
	URL, Secret string
	HTTPClient  *http.Client
}
type CanvasControlClient struct {
	baseURL, secret string
	client          *http.Client
}

type FreezeRequest struct {
	SessionID   string   `json:"sessionId"`
	FreezeToken string   `json:"freezeToken"`
	CanvasIDs   []string `json:"canvasIds"`
}
type CanvasSnapshot struct {
	CanvasID string
	State    []byte
	Digest   string
}
type FreezeBundle struct {
	Snapshots []CanvasSnapshot
	Closed    int
}

func NewCanvasControlClient(cfg CanvasControlConfig) (*CanvasControlClient, error) {
	if err := ValidateControlSecret(cfg.Secret); err != nil {
		return nil, err
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
	copyClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return errCanvasControlRedirect }
	return &CanvasControlClient{baseURL: strings.TrimRight(u.String(), "/"), secret: cfg.Secret, client: &copyClient}, nil
}

// ValidateControlSecret deliberately pins the on-wire bearer to 32 random
// bytes encoded as 64 lowercase hexadecimal characters.
func ValidateControlSecret(secret string) error {
	if len(secret) != 64 || strings.ToLower(secret) != secret {
		return errors.New("HOCUSPOCUS_CONTROL_SECRET must be 64 lowercase hexadecimal characters")
	}
	decoded, err := hex.DecodeString(secret)
	if err != nil || len(decoded) != 32 {
		return errors.New("HOCUSPOCUS_CONTROL_SECRET must be 64 lowercase hexadecimal characters")
	}
	return nil
}
func ConstantTimeBearerMatch(header, secret string) bool {
	return subtle.ConstantTimeCompare([]byte(header), []byte("Bearer "+secret)) == 1
}
func ValidateControlURL(raw string) error { _, err := validateControlURL(raw); return err }
func validateControlURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || !u.IsAbs() || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("invalid canvas control origin")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, errors.New("canvas control URL must use HTTP or HTTPS")
	}
	if u.Scheme == "http" {
		ip := net.ParseIP(u.Hostname())
		// IPv4-mapped IPv6 is intentionally not accepted as a back door.
		if ip == nil || (ip.To4() != nil && strings.Contains(u.Hostname(), ":")) || !ip.IsLoopback() {
			return nil, errors.New("plaintext canvas control URL must use canonical numeric loopback")
		}
	}
	return u, nil
}
func transportDisablesTLSVerification(rt http.RoundTripper) bool {
	if rt == nil {
		return false
	}
	t, ok := rt.(*http.Transport)
	return ok && t.TLSClientConfig != nil && t.TLSClientConfig.InsecureSkipVerify
}

var _ = tls.Config{}

func (c *CanvasControlClient) Freeze(ctx context.Context, request FreezeRequest) (FreezeBundle, error) {
	if err := validateFreezeRequest(request); err != nil {
		return FreezeBundle{}, err
	}
	deadline := time.Now().Add(canvasControlFreezeBudget)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	var last error
	for attempt := 0; ; attempt++ {
		bundle, retry, err := c.freezeOnce(ctx, request)
		if err == nil {
			return bundle, nil
		}
		last = err
		if !retry || ctx.Err() != nil || time.Until(deadline) <= 0 {
			slog.Warn("canvas lifecycle freeze failed", "reason", safeControlError(last))
			return FreezeBundle{}, fmt.Errorf("canvas freeze failed: %w", last)
		}
		// bounded deterministic jitter is enough to avoid immediate retry storms
		wait := time.Duration(5+(attempt%4)*7) * time.Millisecond
		select {
		case <-ctx.Done():
			return FreezeBundle{}, fmt.Errorf("canvas freeze failed: %w", last)
		case <-time.After(wait):
		}
	}
}
func validateFreezeRequest(r FreezeRequest) error {
	if id, err := uuid.Parse(r.SessionID); err != nil || id.String() != r.SessionID {
		return errors.New("invalid canvas freeze session ID")
	}
	if id, err := uuid.Parse(r.FreezeToken); err != nil || id.String() != r.FreezeToken {
		return errors.New("invalid canvas freeze token")
	}
	if len(r.CanvasIDs) > maxSnapshots {
		return errors.New("too many canvas IDs")
	}
	for i, id := range r.CanvasIDs {
		parsed, err := uuid.Parse(id)
		if err != nil || parsed.String() != id {
			return errors.New("invalid canvas ID")
		}
		if i > 0 && r.CanvasIDs[i-1] >= id {
			return errors.New("canvas IDs must be sorted and unique")
		}
	}
	return nil
}
func (c *CanvasControlClient) freezeOnce(ctx context.Context, request FreezeRequest) (FreezeBundle, bool, error) {
	body, _ := json.Marshal(request)
	req, err := c.newRequest(ctx, "/internal/canvas-sessions/freeze", body)
	if err != nil {
		return FreezeBundle{}, false, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		if errors.Is(err, errCanvasControlRedirect) {
			return FreezeBundle{}, false, err
		}
		return FreezeBundle{}, true, errors.New("control transport unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusConflict || resp.StatusCode >= 500 {
		return FreezeBundle{}, true, fmt.Errorf("control returned HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return FreezeBundle{}, false, fmt.Errorf("control returned HTTP %d", resp.StatusCode)
	}
	data, err := readBounded(resp.Body, maxControlResponseBytes)
	if err != nil {
		return FreezeBundle{}, true, err
	}
	bundle, err := validateFreezeBundle(data, request.CanvasIDs)
	return bundle, false, err
}
func readBounded(r io.Reader, limit int64) ([]byte, error) {
	b, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, errors.New("control response read failed")
	}
	if int64(len(b)) > limit {
		return nil, errors.New("control response exceeds size limit")
	}
	return b, nil
}
func validateFreezeBundle(data []byte, requested []string) (FreezeBundle, error) {
	type snapshot struct {
		CanvasID string `json:"canvasId"`
		State    string `json:"stateBase64"`
		Digest   string `json:"digest"`
	}
	type wire struct {
		Snapshots []snapshot `json:"snapshots"`
		Closed    int        `json:"closed"`
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var w wire
	if err := dec.Decode(&w); err != nil || dec.Decode(&struct{}{}) != io.EOF || w.Snapshots == nil || w.Closed < 0 {
		return FreezeBundle{}, errors.New("invalid canvas control bundle")
	}
	if len(w.Snapshots) > maxSnapshots {
		return FreezeBundle{}, errors.New("too many canvas snapshots")
	}
	want := make(map[string]struct{}, len(requested))
	for _, id := range requested {
		want[id] = struct{}{}
	}
	b := FreezeBundle{Snapshots: make([]CanvasSnapshot, 0, len(w.Snapshots)), Closed: w.Closed}
	aggregate := 0
	for i, s := range w.Snapshots {
		if _, ok := want[s.CanvasID]; !ok || (i > 0 && w.Snapshots[i-1].CanvasID >= s.CanvasID) {
			return FreezeBundle{}, errors.New("canvas control bundle has unexpected or unordered canvas")
		}
		state, err := base64.StdEncoding.DecodeString(s.State)
		if err != nil || base64.StdEncoding.EncodeToString(state) != s.State {
			return FreezeBundle{}, errors.New("canvas control bundle has non-canonical state")
		}
		if len(state) > maxSnapshotBytes {
			return FreezeBundle{}, errors.New("canvas snapshot exceeds size limit")
		}
		aggregate += len(state)
		if aggregate > maxAggregateBytes {
			return FreezeBundle{}, errors.New("canvas snapshot aggregate exceeds size limit")
		}
		d := sha256.Sum256(state)
		if len(s.Digest) != 64 || s.Digest != strings.ToLower(s.Digest) || subtle.ConstantTimeCompare([]byte(hex.EncodeToString(d[:])), []byte(s.Digest)) != 1 {
			return FreezeBundle{}, errors.New("canvas control bundle digest mismatch")
		}
		b.Snapshots = append(b.Snapshots, CanvasSnapshot{CanvasID: s.CanvasID, State: state, Digest: s.Digest})
	}
	return b, nil
}
func (c *CanvasControlClient) Complete(ctx context.Context, sid, token string) {
	c.terminal(ctx, "/internal/canvas-sessions/complete", sid, token)
}
func (c *CanvasControlClient) Unfreeze(ctx context.Context, sid, token string) {
	c.terminal(ctx, "/internal/canvas-sessions/unfreeze", sid, token)
}
func (c *CanvasControlClient) terminal(ctx context.Context, sidPath, sid, token string) {
	if validateFreezeRequest(FreezeRequest{SessionID: sid, FreezeToken: token}) != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
	defer cancel()
	body, _ := json.Marshal(map[string]string{"sessionId": sid, "freezeToken": token})
	req, err := c.newRequest(ctx, sidPath, body)
	if err != nil {
		return
	}
	resp, err := c.client.Do(req)
	if err != nil {
		slog.Warn("canvas lifecycle terminal call failed", "operation", sidPath, "reason", safeControlError(err))
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		slog.Warn("canvas lifecycle terminal acknowledgement failed", "operation", sidPath, "status", resp.StatusCode)
	}
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
func safeControlError(err error) string {
	if err == nil {
		return "unknown"
	}
	return err.Error()
}

var _ = sort.Strings
