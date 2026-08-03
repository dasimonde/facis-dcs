package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"digital-contracting-service/internal/base/safehttp"
)

const maxResponseSize = 16 << 20

type Response struct {
	Body        []byte
	ContentType string
}

type RequestOpts struct {
	ContentType string
	Accept      string
}

type Client struct {
	HTTPClient *http.Client
}

// NewClient builds the client used to fetch a status list.
//
// The URI comes out of a credential, so it is caller-influenced, though only by
// a caller whose signature already verified against a trusted issuer — a
// narrower reach than an issuer identifier, which is read before anything is
// verified. It is bounded anyway: no redirects, and none of the addresses that
// answer only because the request originates on this host.
//
// Loopback stays dialable, unlike the issuer resolver's default: every
// deployment here co-locates the status service, and dev and CI serve status
// lists from localhost.
func NewClient() *Client {
	return &Client{HTTPClient: safehttp.Client(10*time.Second, safehttp.Policy{AllowLoopback: true})}
}

func (c *Client) Fetch(ctx context.Context, uri string) (Response, error) {
	return c.FetchWithRequest(ctx, uri, RequestOpts{})
}

func (c *Client) FetchWithContentType(ctx context.Context, uri, contentType string) (Response, error) {
	return c.FetchWithRequest(ctx, uri, RequestOpts{ContentType: contentType})
}

func (c *Client) FetchWithRequest(ctx context.Context, uri string, opts RequestOpts) (Response, error) {
	client := c.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(uri), nil)
	if err != nil {
		return Response{}, fmt.Errorf("create request: %w", err)
	}
	if ct := strings.TrimSpace(opts.ContentType); ct != "" {
		req.Header.Set("Content-Type", ct)
	}
	if accept := strings.TrimSpace(opts.Accept); accept != "" {
		req.Header.Set("Accept", accept)
	}

	resp, err := client.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("GET %s: %w", uri, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return Response{}, fmt.Errorf("status list service returned %d", resp.StatusCode)
	}

	limited := io.LimitReader(resp.Body, maxResponseSize+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return Response{}, fmt.Errorf("read response: %w", err)
	}
	if len(body) > maxResponseSize {
		return Response{}, fmt.Errorf("response exceeds %d bytes", maxResponseSize)
	}

	return Response{
		Body:        body,
		ContentType: resp.Header.Get("Content-Type"),
	}, nil
}
