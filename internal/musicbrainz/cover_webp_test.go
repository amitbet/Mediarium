package musicbrainz

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	_ "image/jpeg"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// 16x24, lossy WebP, solid red (made with libwebp; Go can only read WebP).
const webpFront = "UklGRkIAAABXRUJQVlA4IDYAAAAQAwCdASoQABgAPrVInkmnJCKhMAgA4BaJQBOl0AAQNgAA/upl/xC72H3/84l/JH6BdrxuMAA="

func TestFrontCoverKeepsWebPAsJPEG(t *testing.T) {
	webp, err := base64.StdEncoding.DecodeString(webpFront)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		body       []byte
		wantErr    bool
		wantType   string
		wantFormat string
	}{
		{"a WebP cover", webp, false, "image/jpeg", "jpeg"},
		{"a cut-off WebP cover", webp[:len(webp)/2], true, "", ""},
		{"a web page", []byte("<html>no</html>"), true, "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Write(tc.body)
			}))
			defer srv.Close()
			c := New("1.2.3", WithCoverArtBaseURL(srv.URL), WithHTTPClient(srv.Client()), WithRateLimit(0), WithBackoff(func(int) time.Duration { return 0 }))
			data, ct, err := c.FrontCover(context.Background(), "b1392450-e666-3926-a536-22c65f834433")
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected an error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if ct != tc.wantType {
				t.Errorf("content type = %q, want %q", ct, tc.wantType)
			}
			if _, format, err := image.DecodeConfig(bytes.NewReader(data)); err != nil || format != tc.wantFormat {
				t.Errorf("picture is %q, %v", format, err)
			}
		})
	}
}
