package graph

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

// fakeCred is a TokenCredential that returns a static token without any network
// or auth interaction, so the HTTP client can be exercised in isolation.
type fakeCred struct{ calls int32 }

func (f *fakeCred) GetToken(_ context.Context, _ policy.TokenRequestOptions) (azcore.AccessToken, error) {
	atomic.AddInt32(&f.calls, 1)
	return azcore.AccessToken{Token: "fake-token", ExpiresOn: time.Now().Add(time.Hour)}, nil
}

// roundTripFunc adapts a function to an http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func resp(code int, body string, header http.Header) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     header,
	}
}

// testClient builds a Client wired to a fake credential and a custom transport.
func testClient(rt http.RoundTripper) (*Client, *fakeCred) {
	cred := &fakeCred{}
	return &Client{cred: cred, scopes: []string{"scope"}, http: &http.Client{Transport: rt}}, cred
}

func TestGetAuthorizesAndReturnsBody(t *testing.T) {
	var gotAuth, gotURL string
	c, cred := testClient(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		gotAuth = r.Header.Get("Authorization")
		gotURL = r.URL.String()
		return resp(http.StatusOK, `{"id":"abc"}`, nil), nil
	}))

	data, err := c.Get(context.Background(), "beta", "/me", url.Values{"$select": {"id"}})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(data) != `{"id":"abc"}` {
		t.Errorf("body = %s", data)
	}
	if gotAuth != "Bearer fake-token" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if !strings.HasPrefix(gotURL, baseBeta+"/me?") || !strings.Contains(gotURL, "%24select=id") {
		t.Errorf("URL = %q", gotURL)
	}
	if atomic.LoadInt32(&cred.calls) != 1 {
		t.Errorf("token fetched %d times, want 1", cred.calls)
	}
}

func TestListAllFollowsNextLink(t *testing.T) {
	var requests int32
	c, _ := testClient(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&requests, 1)
		if n == 1 {
			// First page points to an absolute next link.
			return resp(http.StatusOK, `{"value":[{"id":"1"},{"id":"2"}],"@odata.nextLink":"`+baseBeta+`/things?page=2"}`, nil), nil
		}
		// Second page has no next link -> pagination stops.
		return resp(http.StatusOK, `{"value":[{"id":"3"}]}`, nil), nil
	}))

	items, err := c.ListAll(context.Background(), "beta", "/things", nil)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3 (pagination did not concatenate pages)", len(items))
	}
	if atomic.LoadInt32(&requests) != 2 {
		t.Errorf("made %d requests, want 2", requests)
	}
}

func TestDoRetriesOn429(t *testing.T) {
	var attempts int32
	c, _ := testClient(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		n := atomic.AddInt32(&attempts, 1)
		if n == 1 {
			// Retry-After 0 keeps the test fast while still exercising the retry.
			return resp(http.StatusTooManyRequests, "", http.Header{"Retry-After": {"0"}}), nil
		}
		return resp(http.StatusOK, `{"ok":true}`, nil), nil
	}))

	data, err := c.Get(context.Background(), "v1.0", "/me", nil)
	if err != nil {
		t.Fatalf("Get after 429 retry: %v", err)
	}
	if string(data) != `{"ok":true}` {
		t.Errorf("body = %s", data)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("attempts = %d, want 2 (one 429 + one success)", attempts)
	}
}

func TestDoReturnsErrorOnHTTPError(t *testing.T) {
	c, _ := testClient(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return resp(http.StatusBadRequest, `{"error":{"message":"bad"}}`, nil), nil
	}))

	data, err := c.Get(context.Background(), "beta", "/me", nil)
	if err == nil {
		t.Fatal("expected an error for a 400 response")
	}
	if !strings.Contains(err.Error(), "400") {
		t.Errorf("error should include the status code: %v", err)
	}
	if !strings.Contains(err.Error(), "bad") {
		t.Errorf("error should include the response body: %v", err)
	}
	// The raw body is still returned alongside the error for callers that want it.
	if !strings.Contains(string(data), "bad") {
		t.Errorf("body = %s", data)
	}
}

func TestPostSendsBodyAndContentType(t *testing.T) {
	var gotBody, gotCT, gotMethod string
	c, _ := testClient(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		gotCT = r.Header.Get("Content-Type")
		gotMethod = r.Method
		return resp(http.StatusCreated, `{"id":"new"}`, nil), nil
	}))

	data, err := c.Post(context.Background(), "beta", "/things", []byte(`{"name":"x"}`))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	if string(data) != `{"id":"new"}` {
		t.Errorf("body = %s", data)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s", gotMethod)
	}
	if gotBody != `{"name":"x"}` {
		t.Errorf("sent body = %s", gotBody)
	}
	if gotCT != "application/json" {
		t.Errorf("Content-Type = %q", gotCT)
	}
}

func TestListAllReturnsErrorOnBadJSON(t *testing.T) {
	c, _ := testClient(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return resp(http.StatusOK, `not json`, nil), nil
	}))
	if _, err := c.ListAll(context.Background(), "beta", "/things", nil); err == nil {
		t.Fatal("expected a decode error for an invalid collection body")
	}
}

func TestDoRetriesOn503(t *testing.T) {
	var attempts int32
	c, _ := testClient(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			return resp(http.StatusServiceUnavailable, "", http.Header{"Retry-After": {"0"}}), nil
		}
		return resp(http.StatusOK, `{"ok":true}`, nil), nil
	}))

	data, code, err := c.do(context.Background(), http.MethodGet, baseV1+"/me", nil)
	if err != nil {
		t.Fatalf("do after 503 retry: %v", err)
	}
	if code != http.StatusOK || string(data) != `{"ok":true}` {
		t.Errorf("code=%d body=%q", code, data)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
}

func TestDoRetries429OnPost(t *testing.T) {
	// A 429 means Graph rejected the request without applying it, so retrying a
	// POST is safe (unlike a transport error, which is never retried for POST).
	var attempts int32
	c, _ := testClient(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if atomic.AddInt32(&attempts, 1) == 1 {
			return resp(http.StatusTooManyRequests, "", http.Header{"Retry-After": {"0"}}), nil
		}
		return resp(http.StatusCreated, `{"id":"new"}`, nil), nil
	}))

	data, code, err := c.do(context.Background(), http.MethodPost, baseV1+"/things", []byte(`{}`))
	if err != nil {
		t.Fatalf("do after 429 retry: %v", err)
	}
	if code != http.StatusCreated || string(data) != `{"id":"new"}` {
		t.Errorf("code=%d body=%q", code, data)
	}
	if atomic.LoadInt32(&attempts) != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
}

func TestDoGivesUpAfterMaxRetries(t *testing.T) {
	var attempts int32
	c, _ := testClient(roundTripFunc(func(r *http.Request) (*http.Response, error) {
		atomic.AddInt32(&attempts, 1)
		return resp(http.StatusServiceUnavailable, "", http.Header{"Retry-After": {"0"}}), nil
	}))

	_, code, err := c.do(context.Background(), http.MethodGet, baseV1+"/me", nil)
	if err == nil {
		t.Fatal("want error after persistent 503")
	}
	if code != http.StatusServiceUnavailable {
		t.Errorf("code = %d, want 503", code)
	}
	if n := atomic.LoadInt32(&attempts); n != 6 { // initial attempt + 5 retries
		t.Errorf("attempts = %d, want 6", n)
	}
}
