package indexers

import (
	"net/netip"
	"net/url"
	"strings"
)

// siteAddressMax is the longest site address that is saved.
const siteAddressMax = 200

// IsLocalDefinition reports whether every address a definition lists is on the
// person's own machine or network: localhost, a name without a dot, a private,
// loopback or link-local IP address. Such a site (a BitMagnet, a Prowlarr)
// runs wherever its owner put it, so the address in the definition is only an
// example and may be replaced. A definition with a public address in its list,
// or with no address at all, is not local.
func IsLocalDefinition(links []string) bool {
	if len(links) == 0 {
		return false
	}
	for _, l := range links {
		u, err := url.Parse(strings.TrimSpace(l))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
			return false
		}
		if !localHost(u.Hostname()) {
			return false
		}
	}
	return true
}

// Address problems, as codes the caller turns into a sentence.
const (
	AddrOK       = ""
	AddrEmpty    = "empty"
	AddrTooLong  = "long"
	AddrScheme   = "scheme"
	AddrUserInfo = "userinfo"
	AddrQuery    = "query"
	AddrBadChar  = "char"
)

// CheckLocalSiteAddress judges an address typed for a site that runs on the
// person's own network. It must be an http or https address with a host, no
// user name or password, no query or fragment, and fit in 200 characters. The
// port is optional. It returns the problem code (AddrOK when fine).
func CheckLocalSiteAddress(raw string) string {
	raw = strings.TrimSpace(raw)
	switch {
	case raw == "":
		return AddrEmpty
	case len(raw) > siteAddressMax:
		return AddrTooLong
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f || r == ' ' {
			return AddrBadChar
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return AddrScheme
	}
	if u.User != nil {
		return AddrUserInfo
	}
	if u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(raw, "?#") {
		return AddrQuery
	}
	return AddrOK
}

// LoopbackSite reports whether a site address points at the machine Mediarium
// runs on (127.x.x.x, ::1 or localhost): inside Docker that is the container.
func LoopbackSite(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	h := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return true
	}
	a, err := netip.ParseAddr(h)
	return err == nil && a.IsLoopback()
}
