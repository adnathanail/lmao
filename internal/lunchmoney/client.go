// Package lunchmoney is a client for the parts of the Lunch Money v2 API
// (https://developers.lunchmoney.app) that lmao uses.
package lunchmoney

import (
	"bytes"
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

	"github.com/adnathanail/lmao/internal/auth"
)

// DefaultBaseURL is the v2 API.
const DefaultBaseURL = "https://api.lunchmoney.dev/v2"

// BaseURLEnv overrides DefaultBaseURL, for example to use the mock server
// at https://mock.lunchmoney.dev/v2.
const BaseURLEnv = "LUNCHMONEY_API_URL"

// maxRetryWait caps how long a rate-limited request waits before its retry.
const maxRetryWait = 2 * time.Minute

// Client calls the API. Its methods are safe to call concurrently.
type Client struct {
	BaseURL string
	Tokens  auth.TokenSource
	HTTP    *http.Client

	// OnRateLimit, if set, is called when a request is rate limited, with
	// how long it waits before retrying.
	OnRateLimit func(wait time.Duration)

	// sleep waits for d or until ctx is done. Tests replace it.
	sleep func(ctx context.Context, d time.Duration) error
}

// New returns a client for baseURL ("" for DefaultBaseURL).
func New(baseURL string, tokens auth.TokenSource) *Client {
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	return &Client{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		Tokens:  tokens,
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		sleep:   sleepCtx,
	}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// APIError is an error response from the API.
type APIError struct {
	Status  int
	Message string
	Details []string // each error's errMsg
}

func (e *APIError) Error() string {
	msg := e.Message
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	// The details usually say more than the message ("Request Validation
	// Failure"), so they come after it.
	if len(e.Details) > 0 {
		msg += ": " + strings.Join(e.Details, "; ")
	}
	return fmt.Sprintf("Lunch Money: %s (HTTP %d)", msg, e.Status)
}

// do sends a request and decodes a JSON response into out (if not nil).
// A 429 response is retried once, after the wait its Retry-After asks for.
func (c *Client) do(ctx context.Context, method, path string, query url.Values, body, out any) error {
	var payload []byte
	if body != nil {
		var err error
		if payload, err = json.Marshal(body); err != nil {
			return err
		}
	}
	for attempt := 0; ; attempt++ {
		resp, err := c.send(ctx, method, path, query, payload)
		if err != nil {
			return err
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt == 0 {
			wait := retryAfter(resp.Header.Get("Retry-After"))
			_ = resp.Body.Close()
			if c.OnRateLimit != nil {
				c.OnRateLimit(wait)
			}
			if err := c.sleep(ctx, wait); err != nil {
				return err
			}
			continue
		}
		return decode(resp, out)
	}
}

func (c *Client) send(ctx context.Context, method, path string, query url.Values, payload []byte) (*http.Response, error) {
	token, err := c.Tokens.Token(ctx)
	if err != nil {
		return nil, err
	}
	u := c.BaseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.HTTP.Do(req)
}

func decode(resp *http.Response, out any) error {
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return parseError(resp.StatusCode, data)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("reading Lunch Money's response: %w", err)
	}
	return nil
}

func parseError(status int, data []byte) error {
	e := &APIError{Status: status}
	var body struct {
		Message string `json:"message"`
		Errors  []struct {
			ErrMsg string `json:"errMsg"`
		} `json:"errors"`
	}
	if json.Unmarshal(data, &body) == nil {
		e.Message = body.Message
		for _, d := range body.Errors {
			if d.ErrMsg != "" && d.ErrMsg != body.Message {
				e.Details = append(e.Details, d.ErrMsg)
			}
		}
	}
	return e
}

// retryAfter reads a Retry-After header (seconds, or an HTTP date), falling
// back to a minute, the length of the rate limit's window.
func retryAfter(h string) time.Duration {
	wait := time.Minute
	if s, err := strconv.Atoi(strings.TrimSpace(h)); err == nil && s >= 0 {
		wait = time.Duration(s) * time.Second
	} else if t, err := http.ParseTime(h); err == nil {
		wait = time.Until(t)
	}
	return max(time.Second, min(wait, maxRetryWait))
}

// IsUnauthorized reports whether err is the API rejecting the token.
func IsUnauthorized(err error) bool {
	var e *APIError
	return errors.As(err, &e) && e.Status == http.StatusUnauthorized
}
