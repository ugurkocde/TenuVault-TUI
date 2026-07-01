// Package graph is a thin raw-HTTP Microsoft Graph client. It deliberately does
// not use the typed Graph SDK: TenuVault stores verbatim policy JSON so items
// restore cleanly, and a typed SDK would transform or drop fields. This mirrors
// MgGraphCommunity's Invoke-MgGraphCommunityRequest.
package graph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

const (
	baseV1   = "https://graph.microsoft.com/v1.0"
	baseBeta = "https://graph.microsoft.com/beta"
)

// API is the subset of Graph operations the engines use. It lets backup,
// restore, and sync be tested against a fake without network access.
type API interface {
	Get(ctx context.Context, version, path string, query url.Values) (json.RawMessage, error)
	ListAll(ctx context.Context, version, path string, query url.Values) ([]json.RawMessage, error)
	Post(ctx context.Context, version, path string, body json.RawMessage) (json.RawMessage, error)
	Patch(ctx context.Context, version, path string, body json.RawMessage) (json.RawMessage, error)
}

// Client calls Microsoft Graph using a token credential and a fixed scope set.
type Client struct {
	cred   azcore.TokenCredential
	scopes []string
	http   *http.Client
}

// New returns a Graph client backed by the given credential and token scopes.
func New(cred azcore.TokenCredential, scopes []string) *Client {
	return &Client{cred: cred, scopes: scopes, http: &http.Client{Timeout: 60 * time.Second}}
}

func base(version string) string {
	if version == "beta" {
		return baseBeta
	}
	return baseV1
}

func (c *Client) token(ctx context.Context) (string, error) {
	tok, err := c.cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: c.scopes})
	if err != nil {
		return "", err
	}
	return tok.Token, nil
}

// do performs a single request against an absolute or relative Graph URL,
// retrying on HTTP 429/503/504 honoring Retry-After. Transport-level errors are
// retried only for GET: a POST/PATCH may have been applied server-side even
// when the response never arrived, and retrying could duplicate a policy.
func (c *Client) do(ctx context.Context, method, fullURL string, body []byte) ([]byte, int, error) {
	for attempt := 0; ; attempt++ {
		// Acquire the token each attempt so a long Retry-After wait can't leave
		// us holding an expired token (the SDK caches and refreshes as needed).
		tok, err := c.token(ctx)
		if err != nil {
			return nil, 0, fmt.Errorf("acquire token: %w", err)
		}
		var rdr io.Reader
		if body != nil {
			rdr = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, fullURL, rdr)
		if err != nil {
			return nil, 0, err
		}
		req.Header.Set("Authorization", "Bearer "+tok)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, 0, ctx.Err()
			}
			if method == http.MethodGet && attempt < 3 {
				if !sleepCtx(ctx, backoff(attempt)) {
					return nil, 0, ctx.Err()
				}
				continue
			}
			return nil, 0, err
		}
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if retryableStatus(resp.StatusCode) && attempt < 5 {
			wait := backoff(attempt)
			if ra := resp.Header.Get("Retry-After"); ra != "" {
				if secs, err := strconv.Atoi(ra); err == nil {
					wait = time.Duration(secs) * time.Second
				}
			}
			if wait > 2*time.Minute {
				wait = 2 * time.Minute
			}
			if !sleepCtx(ctx, wait) {
				return nil, resp.StatusCode, ctx.Err()
			}
			continue
		}
		if resp.StatusCode >= 400 {
			return data, resp.StatusCode, fmt.Errorf("graph %s %s: %d: %s", method, fullURL, resp.StatusCode, string(data))
		}
		return data, resp.StatusCode, nil
	}
}

// retryableStatus reports whether Graph signalled a transient condition worth
// retrying (throttling or a temporarily unavailable service).
func retryableStatus(code int) bool {
	switch code {
	case http.StatusTooManyRequests, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// backoff returns the exponential fallback wait for the given attempt when no
// Retry-After header is present: 2s, 4s, 8s, 16s, 32s.
func backoff(attempt int) time.Duration {
	return time.Duration(2<<attempt) * time.Second
}

// sleepCtx waits for d or until ctx is cancelled; it reports whether the full
// wait elapsed.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d):
		return true
	}
}

// Get fetches a single resource and returns the raw JSON body.
func (c *Client) Get(ctx context.Context, version, path string, query url.Values) (json.RawMessage, error) {
	u := base(version) + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	data, _, err := c.do(ctx, http.MethodGet, u, nil)
	return data, err
}

// listPage is the minimal OData envelope for collection responses.
type listPage struct {
	Value    []json.RawMessage `json:"value"`
	NextLink string            `json:"@odata.nextLink"`
}

// ListAll fetches every item in a collection, following @odata.nextLink.
func (c *Client) ListAll(ctx context.Context, version, path string, query url.Values) ([]json.RawMessage, error) {
	u := base(version) + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	var out []json.RawMessage
	for u != "" {
		data, _, err := c.do(ctx, http.MethodGet, u, nil)
		if err != nil {
			return out, err
		}
		var page listPage
		if err := json.Unmarshal(data, &page); err != nil {
			return out, fmt.Errorf("decode collection: %w", err)
		}
		out = append(out, page.Value...)
		u = page.NextLink // already an absolute URL
	}
	return out, nil
}

// Post creates a resource and returns the created body.
func (c *Client) Post(ctx context.Context, version, path string, body json.RawMessage) (json.RawMessage, error) {
	data, _, err := c.do(ctx, http.MethodPost, base(version)+path, body)
	return data, err
}

// Patch updates a resource.
func (c *Client) Patch(ctx context.Context, version, path string, body json.RawMessage) (json.RawMessage, error) {
	data, _, err := c.do(ctx, http.MethodPatch, base(version)+path, body)
	return data, err
}
