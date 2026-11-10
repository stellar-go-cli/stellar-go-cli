package mcp

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
)

// carrierGradeNAT covers the 100.64.0.0/10 shared address space (RFC 6598),
// which net.IP.IsPrivate does not classify as private.
var carrierGradeNAT = &net.IPNet{IP: net.ParseIP("100.64.0.0"), Mask: net.CIDRMask(10, 32)}

// validateOutboundURL ensures a caller-supplied URL is http(s) and does not
// resolve to loopback, private, link-local, or otherwise reserved addresses —
// unless the caller explicitly opted into private targets. This bounds the
// SSRF surface of tools that make outbound HTTP requests.
//
// Note: hostname resolution happens at check time; the subsequent fetch
// re-resolves. Full pinning would require plumbing the resolved address into
// the HTTP client — the primary boundary remains transport authentication.
func validateOutboundURL(ctx context.Context, rawURL string, allowPrivate bool) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return fmt.Errorf("invalid resource URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("resource URL must use http or https scheme")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("resource URL has no host")
	}
	if allowPrivate {
		return nil
	}

	var ips []net.IP
	if ip := net.ParseIP(host); ip != nil {
		ips = []net.IP{ip}
	} else {
		addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return fmt.Errorf("cannot resolve resource host %q: %w", host, err)
		}
		for _, a := range addrs {
			ips = append(ips, a.IP)
		}
	}
	if len(ips) == 0 {
		return fmt.Errorf("resource host %q resolved to no addresses", host)
	}
	for _, ip := range ips {
		if isRestrictedIP(ip) {
			return fmt.Errorf("resource host %q resolves to a private or local address (%s) — set allow_private_resource=true to override", host, ip)
		}
	}
	return nil
}

func isRestrictedIP(ip net.IP) bool {
	return ip.IsUnspecified() ||
		ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		carrierGradeNAT.Contains(ip)
}
