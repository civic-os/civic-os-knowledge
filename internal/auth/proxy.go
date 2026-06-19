package auth

import (
	"context"
	"net/http"
	"net/url"

	"golang.org/x/oauth2"
)

// proxyTransport rewrites HTTP requests from an external URL to an internal URL.
// Used in Docker environments where browser-facing and server-facing Keycloak URLs differ.
type proxyTransport struct {
	base     http.RoundTripper
	fromHost string
	toScheme string
	toHost   string
}

func (t *proxyTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Host == t.fromHost {
		req2 := req.Clone(req.Context())
		req2.URL.Scheme = t.toScheme
		req2.URL.Host = t.toHost
		return t.base.RoundTrip(req2)
	}
	return t.base.RoundTrip(req)
}

// contextWithProxyClient returns a context with an HTTP client that rewrites
// requests from externalURL to internalURL. If internalURL is empty, returns
// the original context unchanged.
func contextWithProxyClient(ctx context.Context, externalURL, internalURL string) (context.Context, *http.Client, error) {
	if internalURL == "" || internalURL == externalURL {
		return ctx, nil, nil
	}

	from, err := url.Parse(externalURL)
	if err != nil {
		return ctx, nil, err
	}
	to, err := url.Parse(internalURL)
	if err != nil {
		return ctx, nil, err
	}

	client := &http.Client{
		Transport: &proxyTransport{
			base:     http.DefaultTransport,
			fromHost: from.Host,
			toScheme: to.Scheme,
			toHost:   to.Host,
		},
	}

	return context.WithValue(ctx, oauth2.HTTPClient, client), client, nil
}
