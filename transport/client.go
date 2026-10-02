package transport

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Mode says which kind of client this is.
type Mode int

const (
	// PlainHTTP sends requests over http:// with no authentication.
	PlainHTTP Mode = iota + 1
	// MutualTLS sends requests over https:// and presents a client certificate.
	MutualTLS
)

func (m Mode) String() string {
	switch m {
	case PlainHTTP:
		return "plain HTTP"
	case MutualTLS:
		return "mutual TLS"
	}
	return fmt.Sprintf("Mode(%d)", int(m))
}

// DefaultTimeout bounds one request, including reading the response body.
const DefaultTimeout = 10 * time.Second

// maxBody bounds how much of a response body is read.
const maxBody = 10 << 20

// Option adjusts a client when it is created.
type Option func(*http.Client)

// WithTimeout sets the per-request timeout. Zero means no timeout.
func WithTimeout(d time.Duration) Option {
	return func(c *http.Client) { c.Timeout = d }
}

// Client sends JSON requests to one system.
type Client struct {
	mode       Mode
	base       *url.URL
	serverName string
	http       *http.Client
}

// NewPlain returns a plain-HTTP client for baseURL, which must be an http://
// URL. Use it only for systems that have no TLS listener: the orchestrator and
// the profile-ca plain port.
func NewPlain(baseURL string, opts ...Option) (*Client, error) {
	u, err := parseBase(baseURL, "http")
	if err != nil {
		return nil, err
	}
	tr := &http.Transport{Proxy: nil} // no proxy from the environment: every hop is explicit
	return newClient(PlainHTTP, u, "", tr, opts), nil
}

// NewMTLS returns a mutual-TLS client for baseURL, which must be an https://
// URL. serverName is the DNS name the server certificate must carry, for
// example "serviceregistry" even when baseURL is https://localhost:8490.
// creds supplies the client certificate and the roots that verify the server.
func NewMTLS(baseURL, serverName string, creds Credentials, opts ...Option) (*Client, error) {
	u, err := parseBase(baseURL, "https")
	if err != nil {
		return nil, err
	}
	if serverName == "" {
		return nil, errors.New("transport: NewMTLS needs an explicit server name (the DNS name in the server certificate)")
	}
	if len(creds.Certificate.Certificate) == 0 {
		return nil, errors.New("transport: NewMTLS needs a client certificate (see LoadCredentials)")
	}
	if creds.Roots == nil {
		return nil, errors.New("transport: NewMTLS needs CA roots to verify the server")
	}
	tr := &http.Transport{
		Proxy: nil,
		TLSClientConfig: &tls.Config{
			Certificates: []tls.Certificate{creds.Certificate},
			RootCAs:      creds.Roots,
			ServerName:   serverName,
			MinVersion:   tls.VersionTLS12,
		},
	}
	return newClient(MutualTLS, u, serverName, tr, opts), nil
}

func parseBase(raw, scheme string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("transport: base URL %q: %w", raw, err)
	}
	if u.Scheme != scheme || u.Host == "" {
		return nil, fmt.Errorf("transport: base URL %q must be %s://host[:port]", raw, scheme)
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u, nil
}

func newClient(mode Mode, u *url.URL, serverName string, tr *http.Transport, opts []Option) *Client {
	hc := &http.Client{
		Transport: tr,
		Timeout:   DefaultTimeout,
		// Never follow a redirect: a redirected POST would silently change meaning.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	for _, o := range opts {
		o(hc)
	}
	return &Client{mode: mode, base: u, serverName: serverName, http: hc}
}

// Mode reports whether the client is plain HTTP or mutual TLS.
func (c *Client) Mode() Mode { return c.mode }

// BaseURL returns the base URL the client was created with.
func (c *Client) BaseURL() string { return c.base.String() }

// ServerName returns the expected server certificate name ("" for plain HTTP).
func (c *Client) ServerName() string { return c.serverName }

// Request is one call. Path starts with "/" and is appended to the base URL;
// path segments that carry data must already be escaped (url.PathEscape).
// Token, when non-empty, is sent as "Authorization: Bearer <Token>".
// Body, when non-nil, is sent as JSON.
type Request struct {
	Method string
	Path   string
	Token  string
	Body   any
}

// Response is a 2xx answer.
type Response struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

// DecodeJSON decodes the response body into v.
func (r *Response) DecodeJSON(v any) error {
	if err := json.Unmarshal(r.Body, v); err != nil {
		return fmt.Errorf("transport: decode %d response: %w", r.StatusCode, err)
	}
	return nil
}

// Do sends one request. A 2xx answer returns a Response. Any other status
// returns a *Error that carries the status and the raw body. Transport
// failures, including a refused TLS handshake, return the underlying error.
func (c *Client) Do(ctx context.Context, req Request) (*Response, error) {
	if !strings.HasPrefix(req.Path, "/") {
		return nil, fmt.Errorf("transport: path %q must start with /", req.Path)
	}
	var body io.Reader
	if req.Body != nil {
		b, err := json.Marshal(req.Body)
		if err != nil {
			return nil, fmt.Errorf("transport: encode request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	target := c.base.String() + req.Path
	hreq, err := http.NewRequestWithContext(ctx, req.Method, target, body)
	if err != nil {
		return nil, fmt.Errorf("transport: build request: %w", err)
	}
	hreq.Header.Set("Accept", "application/json")
	if req.Body != nil {
		hreq.Header.Set("Content-Type", "application/json")
	}
	if req.Token != "" {
		hreq.Header.Set("Authorization", "Bearer "+req.Token)
	}
	hresp, err := c.http.Do(hreq)
	if err != nil {
		return nil, fmt.Errorf("transport: %s %s: %w", req.Method, target, err)
	}
	defer hresp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(hresp.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("transport: read %s %s response: %w", req.Method, target, err)
	}
	if hresp.StatusCode < 200 || hresp.StatusCode > 299 {
		return nil, newError(req.Method, target, hresp.StatusCode, data)
	}
	return &Response{StatusCode: hresp.StatusCode, Header: hresp.Header, Body: data}, nil
}
