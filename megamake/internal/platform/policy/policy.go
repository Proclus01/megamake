package policy

import (
	"net"
	"strings"

	"github.com/megamake/megamake/internal/platform/errors"
)

// Policy is global run policy (network gating, allowlists, later destructive gates).
type Policy struct {
	NetEnabled   bool
	AllowDomains []string
}

// RequireNetworkAllowed returns a policy error if networking is disabled or the host is not allowed.
// Host may include a port; it will be normalized.
func (p Policy) RequireNetworkAllowed(host string) error {
	if !p.NetEnabled {
		return errors.NewPolicy("network access is disabled (pass --net to enable)")
	}

	hostOnly := normalizeHost(host)
	if hostOnly == "" {
		return errors.NewPolicy("invalid host for network request")
	}

	// If allowlist is empty, allow all domains when net is enabled.
	if len(p.AllowDomains) == 0 {
		return nil
	}

	for _, allowed := range p.AllowDomains {
		a := strings.TrimSpace(strings.ToLower(allowed))
		if a == "" {
			continue
		}
		if strings.EqualFold(hostOnly, a) {
			return nil
		}
		// Allow subdomains of an allowed domain.
		if strings.HasSuffix(hostOnly, "."+a) {
			return nil
		}
	}

	return errors.NewPolicy("network domain not allowed by --allow-domain policy: " + hostOnly)
}

func normalizeHost(host string) string {
	h := strings.TrimSpace(host)
	if h == "" {
		return ""
	}
	h = strings.ToLower(h)

	// net.SplitHostPort requires brackets for IPv6; handle best-effort.
	if strings.Contains(h, ":") {
		if hostOnly, _, err := net.SplitHostPort(h); err == nil && hostOnly != "" {
			return strings.Trim(hostOnly, "[]")
		}
	}

	// If not a host:port form, return as-is (also trims IPv6 brackets).
	return strings.Trim(h, "[]")
}
