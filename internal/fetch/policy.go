package fetch

import (
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

var forbidden = prefixes(
	"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
	"169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.0.2.0/24",
	"192.88.99.0/24", "192.168.0.0/16", "198.18.0.0/15",
	"198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4",
	"2001::/23", "2001:db8::/32", "2002::/16", "3fff::/20",
)

func prefixes(values ...string) []netip.Prefix {
	out := make([]netip.Prefix, len(values))
	for i, value := range values {
		out[i] = netip.MustParsePrefix(value)
	}
	return out
}

func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsValid() || ip.Zone() != "" || !ip.IsGlobalUnicast() {
		return false
	}
	if ip.Is6() && !netip.MustParsePrefix("2000::/3").Contains(ip) {
		return false
	}
	for _, prefix := range forbidden {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

func validateURL(raw string) (*url.URL, error) {
	if len(raw) == 0 || len(raw) > 4096 || strings.TrimSpace(raw) != raw {
		return nil, fmt.Errorf("invalid URL length or whitespace")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, fmt.Errorf("only HTTP and HTTPS are permitted")
	}
	if u.Opaque != "" || u.User != nil || u.Hostname() == "" || strings.Contains(u.Host, "%") {
		return nil, fmt.Errorf("URL must have a host and no credentials or zone")
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if ip, err := netip.ParseAddr(host); err == nil {
		if !publicIP(ip) {
			return nil, fmt.Errorf("destination is not a public address")
		}
	} else {
		if len(host) > 253 || !strings.Contains(host, ".") {
			return nil, fmt.Errorf("a public hostname is required")
		}
		for _, label := range strings.Split(host, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return nil, fmt.Errorf("invalid hostname")
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
					return nil, fmt.Errorf("use an ASCII or punycode hostname")
				}
			}
		}
	}
	port := u.Port()
	if port != "" && !(u.Scheme == "http" && port == "80" || u.Scheme == "https" && port == "443") {
		return nil, fmt.Errorf("only standard web ports are permitted")
	}
	if strings.HasSuffix(u.Host, ":") {
		return nil, fmt.Errorf("empty port is not permitted")
	}
	u.Fragment = ""
	return u, nil
}
