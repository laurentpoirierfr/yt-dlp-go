// Package networking provides the HTTP client used across yt-dlp-go:
// default headers, cookie jar (with Netscape file support), proxy, retries.
package networking

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"time"
)

// Default timeout, as in yt-dlp's DEFAULT_TIMEOUT.
const DefaultTimeout = 20 * time.Second

// UserAgents is the Chrome UA rotation used by yt-dlp's std_headers.
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36",
}

// HTTPError is a non-2xx response.
type HTTPError struct {
	StatusCode int
	Status     string
	URL        string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP Error %d: %s", e.StatusCode, e.Status)
}

// TransportError is a lower-level network failure.
type TransportError struct {
	Err error
}

func (e *TransportError) Error() string { return e.Err.Error() }
func (e *TransportError) Unwrap() error { return e.Err }

// Client is the request director equivalent of yt-dlp.
type Client struct {
	http        *http.Client
	Jar         http.CookieJar
	Headers     map[string]string // handler-level (default) headers
	Timeout     time.Duration
	NoCheckCert bool
	UserAgent   string

	// OnResponse, if set, is called with every response (for debugging).
	OnResponse func(*http.Response)

	cookieStore *CookieStore
}

// NewClient builds a client with yt-dlp defaults.
func NewClient() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		Jar:         jar,
		Headers:     stdHeaders(),
		Timeout:     DefaultTimeout,
		cookieStore: NewCookieStore(),
	}
}

func stdHeaders() map[string]string {
	return map[string]string{
		"User-Agent":      userAgents[time.Now().UnixNano()%int64(len(userAgents))],
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Language": "en-us,en;q=0.5",
		"Sec-Fetch-Mode":  "navigate",
	}
}

// ApplyOptions configures the client from runtime options.
func (c *Client) ApplyOptions(proxy string, timeout time.Duration, noCheckCert bool, headers map[string]string, userAgent string) error {
	if timeout > 0 {
		c.Timeout = timeout
	}
	c.NoCheckCert = noCheckCert
	for k, v := range headers {
		c.Headers[k] = v
	}
	if userAgent != "" {
		c.Headers["User-Agent"] = userAgent
		c.UserAgent = userAgent
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
	}
	if proxy != "" && proxy != "__noproxy__" {
		pu, err := url.Parse(normalizeProxy(proxy))
		if err != nil {
			return fmt.Errorf("invalid proxy %q: %w", proxy, err)
		}
		transport.Proxy = http.ProxyURL(pu)
	} else if proxy == "__noproxy__" {
		transport.Proxy = nil
	}
	if noCheckCert {
		// #nosec G402 -- explicitly requested by --no-check-certificates
		transport.TLSClientConfig = insecureTLSConfig()
	}
	c.http = &http.Client{
		Transport: transport,
		Timeout:   c.Timeout,
		Jar:       c.Jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return errors.New("stopped after 10 redirects")
			}
			// yt-dlp drops the Cookie header on redirects; the jar re-adds
			// the right cookies for the new host.
			req.Header.Del("Cookie")
			return nil
		},
	}
	return nil
}

func normalizeProxy(p string) string {
	if !strings.Contains(p, "://") {
		return "http://" + p
	}
	return p
}

// Request describes one HTTP call.
type Request struct {
	URL     string
	Method  string
	Data    []byte
	Headers map[string]string
	Timeout time.Duration
}

// Response is a simplified response handle.
type Response struct {
	URL        string
	StatusCode int
	Status     string
	Header     http.Header
	body       io.ReadCloser
	client     *Client
}

// Header returns the first value of a header (case-insensitive).
func (r *Response) HeaderGet(name string) string { return r.Header.Get(name) }

// ReadAll drains the body.
func (r *Response) ReadAll() ([]byte, error) {
	defer r.body.Close()
	return io.ReadAll(r.body)
}

// Body exposes the streaming body (caller must close).
func (r *Response) Body() io.ReadCloser { return r.body }

// Close closes the body if not already drained.
func (r *Response) Close() error { return r.body.Close() }

// Send performs the request. Non-2xx responses are returned as *HTTPError
// unless expectStatus contains the code.
func (c *Client) Send(req *Request, expectStatus ...int) (*Response, error) {
	method := req.Method
	if method == "" {
		method = http.MethodGet
		if req.Data != nil {
			method = http.MethodPost
		}
	}
	var body io.Reader
	if req.Data != nil {
		body = strings.NewReader(string(req.Data))
	}
	httpReq, err := http.NewRequest(method, req.URL, body)
	if err != nil {
		return nil, &TransportError{Err: err}
	}
	// Handler-level headers first, request-level win.
	for k, v := range c.Headers {
		httpReq.Header.Set(k, v)
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if _, ok := httpReq.Header["Accept-Encoding"]; !ok {
		// let Go handle compression transparently
		httpReq.Header.Set("Accept-Encoding", "gzip, deflate")
	}
	if req.Data != nil && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}

	client := c.http
	if client == nil {
		_ = c.ApplyOptions("", 0, false, nil, "")
		client = c.http
	}
	httpClient := *client
	if req.Timeout > 0 {
		httpClient.Timeout = req.Timeout
	} else if c.Timeout > 0 {
		httpClient.Timeout = c.Timeout
	}

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, &TransportError{Err: err}
	}
	if c.OnResponse != nil {
		c.OnResponse(resp)
	}
	if resp.StatusCode >= 400 && !statusExpected(resp.StatusCode, expectStatus) {
		resp.Body.Close()
		return nil, &HTTPError{StatusCode: resp.StatusCode, Status: resp.Status, URL: req.URL}
	}
	return &Response{
		URL:        resp.Request.URL.String(),
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Header:     resp.Header,
		body:       resp.Body,
		client:     c,
	}, nil
}

func statusExpected(code int, expect []int) bool {
	for _, e := range expect {
		if e == code {
			return true
		}
	}
	return false
}

// Get is a convenience GET.
func (c *Client) Get(url string, headers map[string]string, expectStatus ...int) (*Response, error) {
	return c.Send(&Request{URL: url, Headers: headers}, expectStatus...)
}

// DownloadTo fetches a URL and returns its bytes.
func (c *Client) DownloadTo(url string, headers map[string]string, expectStatus ...int) ([]byte, string, error) {
	resp, err := c.Get(url, headers, expectStatus...)
	if err != nil {
		return nil, "", err
	}
	data, err := resp.ReadAll()
	if err != nil {
		return nil, "", &TransportError{Err: err}
	}
	return data, resp.URL, nil
}

// SaveCookies persists the jar into a Netscape cookie file.
func (c *Client) SaveCookies(path string) error {
	jar, ok := c.Jar.(*cookiejar.Jar)
	if !ok || jar == nil {
		return nil
	}
	// cookiejar.Jar does not expose cookies across domains; we keep our own
	// mirror instead (see CookieStore).
	if c.cookieStore == nil {
		return nil
	}
	return c.cookieStore.Save(path)
}

// LoadCookies loads a Netscape cookie file into the jar.
func (c *Client) LoadCookies(path string) error {
	store, err := LoadNetscapeCookies(path)
	if err != nil {
		return err
	}
	if c.cookieStore == nil {
		c.cookieStore = store
	} else {
		c.cookieStore.Merge(store)
	}
	for _, ck := range store.All() {
		host := strings.TrimPrefix(ck.Domain, ".")
		if host == "" {
			continue
		}
		u, err := url.Parse("https://" + host)
		if err != nil {
			continue
		}
		nc := ck.toNetCookie()
		nc.Domain = ck.Domain
		c.Jar.SetCookies(u, []*http.Cookie{nc})
	}
	return nil
}

// SetCookie sets a single cookie for the given URL.
func (c *Client) SetCookie(rawURL, name, value, domain, path string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return err
	}
	if path == "" {
		path = "/"
	}
	c.Jar.SetCookies(u, []*http.Cookie{{Name: name, Value: value, Path: path, Domain: domain}})
	if c.cookieStore != nil {
		c.cookieStore.Set(domain, name, value, path)
	}
	return nil
}

// CookiesFor returns the Cookie header value for a URL.
func (c *Client) CookiesFor(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	var parts []string
	for _, ck := range c.Jar.Cookies(u) {
		parts = append(parts, ck.Name+"="+ck.Value)
	}
	return strings.Join(parts, "; ")
}

// WriteFile writes data to path (helper for callers needing file writes).
func WriteFile(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}

// Context returns a background context (API parity helper).
func Context() context.Context { return context.Background() }
