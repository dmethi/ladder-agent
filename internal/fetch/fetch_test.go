package fetch

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"
)

func TestFetchPinsCheckedAddressAndIgnoresProxyEnv(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	var addresses []string
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "public.example" || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Error("unexpected request identity or credentials")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<h1>Visible</h1><script>fetch('/private')</script><style>hidden</style><p>Article &amp; text</p>")
	}))
	defer fixture.Close()
	lookups := 0
	e := engine{lookup: func(context.Context, string) ([]netip.Addr, error) {
		lookups++
		return []netip.Addr{netip.MustParseAddr("1.1.1.1")}, nil
	}, dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		addresses = append(addresses, address)
		return (&net.Dialer{}).DialContext(ctx, "tcp", strings.TrimPrefix(fixture.URL, "http://"))
	}}
	result := e.fetch(context.Background(), Input{URL: "http://public.example", Format: "text"})
	if result.Outcome != "ok" || !strings.Contains(result.Body, "Visible") || strings.Contains(result.Body, "fetch") || strings.Contains(result.Body, "hidden") || !strings.Contains(result.Body, "Article & text") {
		t.Fatalf("unexpected result: %+v", result)
	}
	if lookups != 1 || len(addresses) != 1 || addresses[0] != "1.1.1.1:80" {
		t.Fatalf("did not pin checked IP: %d %v", lookups, addresses)
	}
}

func TestRejectsMixedPublicAndPrivateDNSBeforeAnyDial(t *testing.T) {
	dials := 0
	e := engine{lookup: func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("::ffff:127.0.0.1")}, nil
	}, dial: func(context.Context, string, string) (net.Conn, error) { dials++; return nil, nil }}
	result := e.fetch(context.Background(), Input{URL: "http://mixed.example", Format: "text"})
	if result.Outcome != "blocked" || dials != 0 {
		t.Fatalf("mixed DNS not rejected: %+v, dials %d", result, dials)
	}
}

func TestPrivateRedirectDoesNotConnect(t *testing.T) {
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data", http.StatusFound)
	}))
	defer fixture.Close()
	dials := 0
	e := engine{lookup: func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("1.1.1.1")}, nil
	}, dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		dials++
		return (&net.Dialer{}).DialContext(ctx, "tcp", strings.TrimPrefix(fixture.URL, "http://"))
	}}
	result := e.fetch(context.Background(), Input{URL: "http://redirect.example", Format: "text"})
	if result.Outcome != "blocked" || dials != 1 {
		t.Fatalf("redirect escaped policy: %+v, dials %d", result, dials)
	}
}

func TestDNSRebindingOnRedirectCannotUseOldCheck(t *testing.T) {
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://rebind.example/next", http.StatusFound)
	}))
	defer fixture.Close()
	lookups, dials := 0, 0
	e := engine{lookup: func(context.Context, string) ([]netip.Addr, error) {
		lookups++
		if lookups > 1 {
			return []netip.Addr{netip.MustParseAddr("127.0.0.1")}, nil
		}
		return []netip.Addr{netip.MustParseAddr("1.1.1.1")}, nil
	}, dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		dials++
		return (&net.Dialer{}).DialContext(ctx, "tcp", strings.TrimPrefix(fixture.URL, "http://"))
	}}
	result := e.fetch(context.Background(), Input{URL: "http://rebind.example", Format: "text"})
	if result.Outcome != "blocked" || lookups != 2 || dials != 1 {
		t.Fatalf("rebinding escaped policy: %+v lookups=%d dials=%d", result, lookups, dials)
	}
}

func TestBoundedResponsesAndFormats(t *testing.T) {
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	_, _ = zw.Write(bytes.Repeat([]byte("a"), MaxBytes+1))
	_ = zw.Close()
	tests := []struct{ name, body, contentType, encoding, format, want string }{
		{"raw too large", strings.Repeat("a", MaxBytes+1), "text/plain", "", "text", "too_large"},
		{"gzip bomb", compressed.String(), "text/plain", "gzip", "text", "too_large"},
		{"binary", "binary", "application/octet-stream", "", "text", "unsupported"},
		{"bad JSON", "not json", "application/json", "", "json", "unsupported"},
		{"inert HTML", "<script>alert(1)</script>", "text/html", "", "html", "ok"},
		{"JSON", `{"a":1}`, "application/json", "", "json", "ok"},
		{"brotli rejected", "data", "text/plain", "br", "text", "unsupported"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				if tt.encoding != "" {
					w.Header().Set("Content-Encoding", tt.encoding)
				}
				_, _ = io.WriteString(w, tt.body)
			}))
			defer fixture.Close()
			e := engine{lookup: func(context.Context, string) ([]netip.Addr, error) {
				return []netip.Addr{netip.MustParseAddr("1.1.1.1")}, nil
			}, dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", strings.TrimPrefix(fixture.URL, "http://"))
			}}
			result := e.fetch(context.Background(), Input{URL: "http://public.example", Format: tt.format})
			if result.Outcome != tt.want || result.Trust != "untrusted" {
				t.Fatalf("got %+v", result)
			}
			if tt.want != "ok" && result.Body != "" {
				t.Fatal("partial content returned on failure")
			}
			if tt.format == "html" && result.Body != tt.body {
				t.Fatal("HTML string altered or executed")
			}
		})
	}
}

func TestCanceledFetchReturnsTypedFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := Fetch(ctx, Input{URL: "https://example.com", Format: "text"})
	if result.Outcome != "timeout" {
		t.Fatalf("got %+v", result)
	}
}

func TestSlowUpstreamHonorsParentDeadline(t *testing.T) {
	fixture := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer fixture.Close()
	e := engine{lookup: func(context.Context, string) ([]netip.Addr, error) {
		return []netip.Addr{netip.MustParseAddr("1.1.1.1")}, nil
	}, dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", strings.TrimPrefix(fixture.URL, "http://"))
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	result := e.fetch(ctx, Input{URL: "http://slow.example", Format: "text"})
	if result.Outcome != "timeout" || result.Body != "" {
		t.Fatalf("got %+v", result)
	}
}
