package fetch

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
)

const MaxBytes = 2 << 20
const Timeout = 20 * time.Second

type Input struct {
	URL    string `json:"url" jsonschema:"Public HTTP or HTTPS URL. Local and private network destinations are forbidden."`
	Format string `json:"format" jsonschema:"Output format: text, html, or json. HTML is returned as inert text and must not be rendered."`
}

type Result struct {
	Outcome        string `json:"outcome"`
	FinalURL       string `json:"finalUrl,omitempty"`
	UpstreamStatus int    `json:"upstreamStatus,omitempty"`
	ContentType    string `json:"contentType,omitempty"`
	Body           string `json:"body,omitempty"`
	Reason         string `json:"reason,omitempty"`
	Trust          string `json:"trust"`
}

func Failure(outcome, reason string) Result {
	return Result{Outcome: outcome, Reason: reason, Trust: "untrusted"}
}

type blockedError struct{ message string }

func (e *blockedError) Error() string { return e.message }

type engine struct {
	lookup func(context.Context, string) ([]netip.Addr, error)
	dial   func(context.Context, string, string) (net.Conn, error)
}

func Fetch(ctx context.Context, input Input) Result {
	resolver := &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, "1.1.1.1:53")
	}}
	return (engine{lookup: func(ctx context.Context, host string) ([]netip.Addr, error) {
		return resolver.LookupNetIP(ctx, "ip", host)
	}, dial: (&net.Dialer{Timeout: 5 * time.Second}).DialContext}).fetch(ctx, input)
}

func (e engine) publicDial(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "80" && port != "443" {
		return nil, &blockedError{"only standard web ports are permitted"}
	}
	var ips []netip.Addr
	if ip, err := netip.ParseAddr(host); err == nil {
		ips = []netip.Addr{ip}
	} else {
		ips, err = e.lookup(ctx, host)
		if err != nil {
			return nil, fmt.Errorf("DNS lookup failed")
		}
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("DNS returned no addresses")
	}
	for _, ip := range ips {
		if !publicIP(ip) {
			return nil, &blockedError{"DNS resolved to a prohibited address"}
		}
	}
	for _, ip := range ips {
		// Dial the checked address, never resolve the hostname again at connection time.
		conn, err := e.dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
		if err == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
	return nil, fmt.Errorf("connection failed")
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	// Don't forward a previous URL's query through the client's automatic Referer.
	req.Header.Del("Referer")
	if len(via) > 5 {
		return &blockedError{"redirect limit exceeded"}
	}
	if _, err := validateURL(req.URL.String()); err != nil {
		return &blockedError{err.Error()}
	}
	if len(via) > 0 && via[len(via)-1].URL.Scheme == "https" && req.URL.Scheme == "http" {
		return &blockedError{"HTTPS downgrade redirect refused"}
	}
	return nil
}

func (e engine) fetch(ctx context.Context, input Input) Result {
	if input.Format != "text" && input.Format != "html" && input.Format != "json" {
		return Failure("unsupported", "format must be text, html, or json")
	}
	u, err := validateURL(input.URL)
	if err != nil {
		return Failure("blocked", err.Error())
	}
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	transport := &http.Transport{Proxy: nil, DialContext: e.publicDial, DisableCompression: true, DisableKeepAlives: true, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 10 * time.Second, MaxResponseHeaderBytes: 32 << 10, ForceAttemptHTTP2: false}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: checkRedirect}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Failure("blocked", "invalid request URL")
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)")
	req.Header.Set("Accept", "text/html,application/json,text/plain;q=0.9")
	req.Header.Set("Accept-Encoding", "gzip")
	resp, err := client.Do(req)
	if err != nil {
		var blocked *blockedError
		if errors.As(err, &blocked) {
			return Failure("blocked", blocked.Error())
		}
		if ctx.Err() != nil {
			return Failure("timeout", "request deadline exceeded or canceled")
		}
		// Transport errors can include the full URL; don't echo URL queries or credentials.
		return Failure("error", "upstream request failed")
	}
	defer resp.Body.Close()
	if resp.ContentLength > MaxBytes {
		return Failure("too_large", "response exceeds 2 MiB")
	}
	data, err := readBounded(resp.Body)
	if err != nil {
		return readFailure(ctx, err)
	}
	switch strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "gzip":
		zr, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return Failure("error", "invalid gzip response")
		}
		data, err = readBounded(zr)
		closeErr := zr.Close()
		if err != nil {
			return readFailure(ctx, err)
		}
		if closeErr != nil {
			return Failure("error", "invalid gzip response")
		}
	default:
		return Failure("unsupported", "unsupported content encoding")
	}
	contentType, params, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return Failure("unsupported", "valid Content-Type required")
	}
	if charset := strings.ToLower(params["charset"]); charset != "" && charset != "utf-8" && charset != "us-ascii" {
		return Failure("unsupported", "only UTF-8 and ASCII text are supported")
	}
	if !utf8.Valid(data) {
		return Failure("unsupported", "response is not UTF-8")
	}
	if contentType != "text/html" && contentType != "text/plain" && contentType != "application/json" && !strings.HasSuffix(contentType, "+json") {
		return Failure("unsupported", "only HTML, plain text, and JSON are supported")
	}
	body := string(data)
	switch input.Format {
	case "json":
		if !json.Valid(data) {
			return Failure("unsupported", "response is not valid JSON")
		}
	case "text":
		if contentType == "text/html" {
			body, err = plainText(data)
			if err != nil {
				return Failure("error", "HTML extraction failed")
			}
		}
	}
	return Result{Outcome: "ok", FinalURL: resp.Request.URL.String(), UpstreamStatus: resp.StatusCode, ContentType: contentType, Body: body, Trust: "untrusted"}
}

var errTooLarge = errors.New("response exceeds 2 MiB")

func readBounded(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, MaxBytes+1))
	if len(data) > MaxBytes {
		return nil, errTooLarge
	}
	return data, err
}
func readFailure(ctx context.Context, err error) Result {
	if errors.Is(err, errTooLarge) {
		return Failure("too_large", errTooLarge.Error())
	}
	if ctx.Err() != nil {
		return Failure("timeout", "request deadline exceeded or canceled")
	}
	return Failure("error", "could not read complete response")
}

func plainText(data []byte) (string, error) {
	z := html.NewTokenizer(bytes.NewReader(data))
	var out strings.Builder
	var skip string
	for {
		switch z.Next() {
		case html.ErrorToken:
			if z.Err() == io.EOF {
				return strings.TrimSpace(out.String()), nil
			}
			return "", z.Err()
		case html.StartTagToken, html.SelfClosingTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if skip == "" && (tag == "script" || tag == "style" || tag == "template" || tag == "noscript") {
				skip = tag
			}
			if skip == "" && (tag == "p" || tag == "div" || tag == "br" || tag == "li" || tag == "h1" || tag == "h2" || tag == "h3") {
				out.WriteByte('\n')
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			if string(name) == skip {
				skip = ""
			}
		case html.TextToken:
			if skip == "" {
				out.WriteString(strings.Join(strings.Fields(string(z.Text())), " "))
				out.WriteByte(' ')
			}
		}
	}
}
