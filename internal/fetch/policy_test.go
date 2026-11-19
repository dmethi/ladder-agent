package fetch

import (
	"net/netip"
	"testing"
)

func TestPublicIP(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.0.0.1", "100.64.0.1", "169.254.169.254", "172.16.0.1", "192.168.1.1", "0.0.0.0", "198.18.0.1", "192.0.2.1", "224.0.0.1", "255.255.255.255", "::1", "::", "fd00::1", "fe80::1", "::ffff:127.0.0.1", "64:ff9b::a00:1", "2002:7f00:1::", "2001:db8::1", "3fff::1", "2001::1"} {
		t.Run(s, func(t *testing.T) {
			if publicIP(netip.MustParseAddr(s)) {
				t.Fatal("accepted non-public address")
			}
		})
	}
	for _, s := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111", "2001:4860:4860::8888"} {
		if !publicIP(netip.MustParseAddr(s)) {
			t.Fatalf("blocked public address %s", s)
		}
	}
}

func TestValidateURL(t *testing.T) {
	for _, s := range []string{"file:///etc/passwd", "gopher://example.com", "http://localhost", "http://127.0.0.1", "http://[::1]", "http://[::ffff:192.168.1.1]", "https://user:password@example.com", "https://example.com:8080", "https://example.com:", "https://example.com:80", "http://example.com:443", "https://example.com\\@127.0.0.1", " https://example.com", "https://example.com\n", "https://[fe80::1%25en0]", "https://-bad.example"} {
		t.Run(s, func(t *testing.T) {
			if _, err := validateURL(s); err == nil {
				t.Fatal("accepted prohibited URL")
			}
		})
	}
	for _, s := range []string{"https://example.com/path?q=value", "http://example.com:80", "https://example.com:443", "https://[2606:4700:4700::1111]/"} {
		if _, err := validateURL(s); err != nil {
			t.Fatalf("blocked valid URL %s: %v", s, err)
		}
	}
}
