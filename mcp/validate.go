package mcp

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// validateURL checks that a URL is safe to fetch. It rejects private/internal
// addresses (RFC 1918), loopback, link-local, multicast, and non-HTTP schemes
// to prevent SSRF attacks from LLM-generated URLs.
func validateURL(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Only allow http and https schemes
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("unsupported scheme %q: only http and https are allowed", parsed.Scheme)
	}

	// Must have a host
	host := parsed.Hostname()
	if host == "" {
		return fmt.Errorf("URL must have a host")
	}

	// Resolve hostname to IP addresses and check each one
	ips, err := net.LookupIP(host)
	if err != nil {
		// If resolution fails, check if it's an IP literal
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("cannot resolve host %q: %w", host, err)
		}
		ips = []net.IP{ip}
	}

	for _, ip := range ips {
		if ip.IsLoopback() {
			return fmt.Errorf("URL resolves to loopback address %s", ip)
		}
		if ip.IsPrivate() {
			return fmt.Errorf("URL resolves to private address %s", ip)
		}
		if ip.IsLinkLocalUnicast() {
			return fmt.Errorf("URL resolves to link-local address %s", ip)
		}
		if ip.IsLinkLocalMulticast() {
			return fmt.Errorf("URL resolves to link-local multicast address %s", ip)
		}
		if ip.IsMulticast() {
			return fmt.Errorf("URL resolves to multicast address %s", ip)
		}
		if ip.IsUnspecified() {
			return fmt.Errorf("URL resolves to unspecified address %s", ip)
		}
	}

	return nil
}
