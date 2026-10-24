package indexers

import (
	"strings"
	"testing"
)

func TestIsLocalDefinition(t *testing.T) {
	tests := []struct {
		name  string
		links []string
		want  bool
	}{
		{"loopback", []string{"http://127.0.0.1:3333"}, true},
		{"BitMagnet's two", []string{"http://127.0.0.1:3333/", "http://bitmagnet:3333/"}, true},
		{"a name without a dot", []string{"http://bitmagnet:3333"}, true},
		{"localhost", []string{"http://localhost:9696/"}, true},
		{"192.168", []string{"http://192.168.1.10:3333"}, true},
		{"10.x", []string{"http://10.0.0.5"}, true},
		{"172.16", []string{"http://172.16.0.2"}, true},
		{"172.31", []string{"https://172.31.255.1:8443"}, true},
		{"link-local", []string{"http://169.254.1.1"}, true},
		{"ipv6 loopback", []string{"http://[::1]:3333"}, true},
		{"a .lan name", []string{"http://nas.lan:9117"}, true},
		{"172.32 is public", []string{"http://172.32.0.1"}, false},
		{"a public name", []string{"https://example.org/"}, false},
		{"a public address", []string{"http://8.8.8.8/"}, false},
		{"mixed list", []string{"http://127.0.0.1:3333", "https://example.org"}, false},
		{"no links", nil, false},
		{"not a web address", []string{"ftp://192.168.1.10"}, false},
		{"junk", []string{"%%%"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsLocalDefinition(tt.links); got != tt.want {
				t.Fatalf("IsLocalDefinition(%v) = %v, want %v", tt.links, got, tt.want)
			}
		})
	}
}

func TestCheckLocalSiteAddress(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want string
	}{
		{"with a port", "http://192.168.1.10:3333", AddrOK},
		{"https and a path", "https://nas.lan/bitmagnet/", AddrOK},
		{"no port", "http://nas", AddrOK},
		{"spaces around", "  http://nas:3333  ", AddrOK},
		{"ipv6", "http://[fd00::5]:3333", AddrOK},
		{"empty", "  ", AddrEmpty},
		{"ftp", "ftp://nas", AddrScheme},
		{"no scheme", "nas:3333", AddrScheme},
		{"no host", "http://", AddrScheme},
		{"password", "http://bob:pw@nas:3333", AddrUserInfo},
		{"user only", "http://bob@nas", AddrUserInfo},
		{"query", "http://nas:3333/?x=1", AddrQuery},
		{"fragment", "http://nas:3333/#top", AddrQuery},
		{"a space inside", "http://na s", AddrBadChar},
		{"a control character", "http://nas\x00", AddrBadChar},
		{"201 characters", "http://" + strings.Repeat("a", 194), AddrTooLong},
		{"200 characters", "http://" + strings.Repeat("a", 193), AddrOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CheckLocalSiteAddress(tt.addr); got != tt.want {
				t.Fatalf("CheckLocalSiteAddress(%q) = %q, want %q", tt.addr, got, tt.want)
			}
		})
	}
}

func TestLoopbackSite(t *testing.T) {
	for addr, want := range map[string]bool{
		"http://127.0.0.1:3333": true, "http://localhost:3333": true, "http://[::1]:3333": true, "http://127.5.5.5": true,
		"http://192.168.1.10:3333": false, "http://bitmagnet:3333": false, "": false,
	} {
		if got := LoopbackSite(addr); got != want {
			t.Errorf("LoopbackSite(%q) = %v, want %v", addr, got, want)
		}
	}
}
