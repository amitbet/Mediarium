package usenetclient

import "testing"

func TestParseETA(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"0:12:34", 12*60 + 34},
		{"1:00:00", 3600},
		{"2:03:04:05", 2*86400 + 3*3600 + 4*60 + 5},
		{"12:34", 12*60 + 34},
		{"", 0},
		{"soon", 0},
		{"0:0:x", 0},
	}
	for _, tc := range cases {
		if got := parseETA(tc.in); got != tc.want {
			t.Errorf("parseETA(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestBaseURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"http://192.168.1.10:8080", "http://192.168.1.10:8080"},
		{"http://192.168.1.10:8080/", "http://192.168.1.10:8080"},
		{" http://host:8080/api ", "http://host:8080"},
		{"http://host:6789/jsonrpc", "http://host:6789"},
		{"http://host/sabnzbd/", "http://host"},
		{"https://host/proxy/sab", "https://host/proxy/sab"},
		{"http://host:8080/?apikey=secret", "http://host:8080"},
	}
	for _, tc := range cases {
		if got := baseURL(tc.in); got != tc.want {
			t.Errorf("baseURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSafeFileName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"The.Movie.2020", "The.Movie.2020"},
		{`a/b\c:d`, "a_b_c_d"},
		{"  ", "release"},
	}
	for _, tc := range cases {
		if got := safeFileName(tc.in); got != tc.want {
			t.Errorf("safeFileName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
