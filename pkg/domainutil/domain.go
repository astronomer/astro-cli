package domainutil

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	DefaultDomain = "astronomer.io"
	LocalDomain   = "localhost"
)

var PRPreviewDomainRegex = regexp.MustCompile(`^(pr\d{4,6}).astronomer-dev\.io$`)

func FormatDomain(domain string) string {
	if strings.Contains(domain, "cloud") {
		domain = strings.Replace(domain, "cloud.", "", 1)
		domain = strings.Replace(domain, "https://", "", 1)
		domain = strings.TrimRight(domain, "/") // removes trailing / if present
	} else if domain == "" {
		domain = DefaultDomain
	}

	return domain
}

func isPrPreviewDomain(domain string) bool {
	return PRPreviewDomainRegex.MatchString(domain)
}

func GetPRSubDomain(domain string) (prSubDomain, restOfDomain string) {
	if isPrPreviewDomain(domain) {
		prSubDomain, domain, _ = strings.Cut(domain, ".")
	}
	return prSubDomain, domain
}

func GetURLToEndpoint(protocol, domain, endpoint string) string {
	var addr, prSubDomain string

	switch domain {
	case LocalDomain:
		addr = fmt.Sprintf("%s://%s:8871/%s", "http", domain, endpoint)
		return addr
	default:
		if isPrPreviewDomain(domain) {
			prSubDomain, domain = GetPRSubDomain(domain)
			addr = fmt.Sprintf("%s://%s.api.%s/hub/%s", protocol, prSubDomain, domain, endpoint)
			return addr
		}
		addr = fmt.Sprintf("%s://api.%s/hub/%s", protocol, domain, endpoint)
	}
	return addr
}

func TransformToCoreAPIEndpoint(addr string) string {
	if strings.Contains(addr, "v1alpha1") || strings.Contains(addr, "v1beta1") {
		addr = strings.Replace(addr, "/hub", "", 1)
		addr = strings.Replace(addr, "localhost:8871", "localhost:8888", 1)
	}
	return addr
}
