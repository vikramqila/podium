// Package proxy forwards matched requests to configured upstream services.
package proxy

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"gatewaykit/internal/config"
)

var ErrUnsupportedUpstream = errors.New("unsupported upstream")

type Forwarder struct {
	client *http.Client
}

func NewForwarder(client *http.Client) *Forwarder {
	if client == nil {
		client = http.DefaultClient
	}
	return &Forwarder{client: client}
}

func (f *Forwarder) ServeHTTP(w http.ResponseWriter, r *http.Request, route config.Route) error {
	if route.Upstream.URL == "" {
		return ErrUnsupportedUpstream
	}

	targetURL, err := buildTargetURL(route.Upstream.URL, r.URL)
	if err != nil {
		return fmt.Errorf("build upstream request: %w", err)
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		return fmt.Errorf("create upstream request: %w", err)
	}
	copyRequestHeaders(upstreamReq.Header, r.Header)
	upstreamReq.Host = upstreamReq.URL.Host
	appendForwardedFor(upstreamReq.Header, r.RemoteAddr)

	resp, err := f.client.Do(upstreamReq)
	if err != nil {
		return fmt.Errorf("send upstream request: %w", err)
	}
	defer resp.Body.Close()

	copyResponseHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if _, err := io.Copy(w, resp.Body); err != nil {
		return fmt.Errorf("copy upstream response: %w", err)
	}

	return nil
}

func buildTargetURL(upstream string, requestURL *url.URL) (string, error) {
	base, err := url.Parse(upstream)
	if err != nil {
		return "", err
	}

	out := *base
	out.Path = singleJoiningSlash(base.Path, requestURL.Path)
	out.RawQuery = requestURL.RawQuery
	out.Fragment = ""
	return out.String(), nil
}

func singleJoiningSlash(left string, right string) string {
	leftSlash := strings.HasSuffix(left, "/")
	rightSlash := strings.HasPrefix(right, "/")
	switch {
	case leftSlash && rightSlash:
		return left + right[1:]
	case !leftSlash && !rightSlash:
		return left + "/" + right
	default:
		return left + right
	}
}

func copyRequestHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if isHopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func copyResponseHeaders(dst http.Header, src http.Header) {
	for key, values := range src {
		if isHopByHopHeader(key) {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func appendForwardedFor(header http.Header, remoteAddr string) {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return
	}

	prior := header.Get("X-Forwarded-For")
	if prior == "" {
		header.Set("X-Forwarded-For", host)
		return
	}
	header.Set("X-Forwarded-For", prior+", "+host)
}

func isHopByHopHeader(header string) bool {
	switch strings.ToLower(header) {
	case "connection",
		"keep-alive",
		"proxy-authenticate",
		"proxy-authorization",
		"te",
		"trailer",
		"transfer-encoding",
		"upgrade":
		return true
	default:
		return false
	}
}
