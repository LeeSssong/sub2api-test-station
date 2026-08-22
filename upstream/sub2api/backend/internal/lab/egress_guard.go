package lab

import (
	"fmt"
	"net"
	"net/url"
	"strings"
)

// ValidateEgressTarget permits only the explicitly named lab mock services.
func ValidateEgressTarget(raw string, allowedHosts ...string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "http" || u.Hostname() == "" {
		return fmt.Errorf("lab egress rejected: target must be an http URL with a host")
	}
	host := strings.ToLower(u.Hostname())
	for _, allowed := range allowedHosts {
		if host == strings.ToLower(strings.TrimSpace(allowed)) {
			if ip := net.ParseIP(host); ip != nil && !ip.IsPrivate() {
				return fmt.Errorf("lab egress rejected: public IP target")
			}
			return nil
		}
	}
	return fmt.Errorf("lab egress rejected: host is not allowlisted")
}
