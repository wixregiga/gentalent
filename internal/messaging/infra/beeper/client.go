package beeper

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	sdk "github.com/beeper/desktop-api-go/v6"
	"github.com/beeper/desktop-api-go/v6/option"

	"gentalent/internal/messaging/domain"
)

type Client struct {
	api             sdk.Client
	group           domain.Group
	mu              sync.RWMutex
	observedVersion string
}

const requestTimeout = 15 * time.Second
const uploadTimeout = 30 * time.Second

func New(baseURL, token string, group domain.Group) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.ForceQuery ||
		u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("use a local loopback platform URL without credentials or a path")
	}
	host := u.Hostname()
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return nil, fmt.Errorf("use a local loopback platform URL")
	}
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("token was empty")
	}
	client := &Client{group: group}
	dialer := &net.Dialer{Timeout: requestTimeout}
	client.api = sdk.NewClient(
		option.WithBaseURL(u.String()),
		option.WithAccessToken(token),
		option.WithHTTPClient(httpClient(dialer, requestTimeout)),
		option.WithMaxRetries(0),
		option.WithMiddleware(platformHTTP(token, client.recordObservedVersion)),
	)
	return client, nil
}

func (c *Client) recordObservedVersion(version string) {
	c.mu.Lock()
	c.observedVersion = version
	c.mu.Unlock()
}

func (c *Client) LastObservedVersion() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.observedVersion
}

func httpClient(dialer *net.Dialer, timeout time.Duration) *http.Client {
	return &http.Client{
		Transport:     &http.Transport{DialContext: dialer.DialContext, DisableKeepAlives: true},
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

type responseError struct {
	status int
}

func (e responseError) Error() string {
	return fmt.Sprintf("API returned HTTP %d; no automatic write retry was made", e.status)
}

func platformHTTP(token string, onVersion func(string)) option.Middleware {
	return func(req *http.Request, next option.MiddlewareNext) (*http.Response, error) {
		contentType := req.Header.Get("Content-Type")
		req.Header = http.Header{}
		req.Header.Set("User-Agent", "GenTalent/0.1")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		response, err := next(req)
		if err != nil {
			return nil, err
		}
		if onVersion != nil {
			if version := strings.TrimSpace(response.Header.Get("X-Beeper-Desktop-Version")); version != "" {
				onVersion(version)
			}
		}
		if response.StatusCode < 200 || response.StatusCode >= 300 {
			response.Body.Close()
			return nil, responseError{status: response.StatusCode}
		}
		return response, nil
	}
}

func apiError(err error) error {
	if _, ok := err.(responseError); ok {
		return err
	}
	return fmt.Errorf("API connection or JSON response failed; a write outcome may be uncertain")
}

func pathID(id string) string {
	switch id {
	case ".":
		return "%2E"
	case "..":
		return "%2E%2E"
	default:
		return url.PathEscape(id)
	}
}
