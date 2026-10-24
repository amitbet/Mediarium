package webpimg

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"strings"
	"testing"
)

// Tiny pictures made with libwebp. Go can read WebP but not write it, so
// they are kept as bytes.
const (
	// 16x24, lossy (VP8), solid red.
	lossyRed = "UklGRkIAAABXRUJQVlA4IDYAAAAQAwCdASoQABgAPrVInkmnJCKhMAgA4BaJQBOl0AAQNgAA/upl/xC72H3/84l/JH6BdrxuMAA="
	// 8x8, lossless (VP8L), fully transparent.
	losslessClear = "UklGRhoAAABXRUJQVlA4TA0AAAAvB8ABEAcQERGIiP4HAA=="
)

func fixture(t *testing.T, b64 string) []byte {
	t.Helper()
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestToJPEG(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		w, h    int
		wantRGB [3]uint8 // roughly; JPEG is lossy
	}{
		{"a lossy picture", fixture(t, lossyRed), 16, 24, [3]uint8{220, 30, 30}},
		{"a transparent picture lands on white", fixture(t, losslessClear), 8, 8, [3]uint8{255, 255, 255}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := ToJPEG(tc.data)
			if err != nil {
				t.Fatal(err)
			}
			img, format, err := image.Decode(bytes.NewReader(out))
			if err != nil || format != "jpeg" {
				t.Fatalf("result is %q, %v", format, err)
			}
			if b := img.Bounds(); b.Dx() != tc.w || b.Dy() != tc.h {
				t.Errorf("size = %dx%d, want %dx%d", b.Dx(), b.Dy(), tc.w, tc.h)
			}
			r, g, b, _ := img.At(tc.w/2, tc.h/2).RGBA()
			got := [3]int{int(r >> 8), int(g >> 8), int(b >> 8)}
			for i, want := range tc.wantRGB {
				if d := got[i] - int(want); d < -25 || d > 25 {
					t.Errorf("middle pixel = %v, want about %v", got, tc.wantRGB)
					break
				}
			}
		})
	}
}

// image.Decode reads WebP once this package is imported.
func TestImageDecodeReadsWebP(t *testing.T) {
	img, format, err := image.Decode(bytes.NewReader(fixture(t, lossyRed)))
	if err != nil || format != "webp" {
		t.Fatalf("format %q, error %v", format, err)
	}
	if b := img.Bounds(); b.Dx() != 16 || b.Dy() != 24 {
		t.Errorf("size = %v", b)
	}
}

func TestToJPEGRefusesWhatIsNotAReadableWebP(t *testing.T) {
	good := fixture(t, lossyRed)
	tests := []struct {
		name string
		data []byte
	}{
		{"nothing", nil},
		{"text", []byte("hello")},
		{"a header only", append([]byte("RIFF\x24\x00\x00\x00WEBPVP8 "), make([]byte, 40)...)},
		{"a cut-off picture", good[:len(good)/2]},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if out, err := ToJPEG(tc.data); err == nil {
				t.Fatalf("expected an error, got %d bytes", len(out))
			}
		})
	}
}

// A picture with a huge size in its header is refused before it is decoded.
func TestToJPEGRefusesAHugePicture(t *testing.T) {
	// A VP8L header that says 16384 x 16384 (268 million pixels).
	data := fixture(t, losslessClear)
	// the 14 bits after the signature byte hold width-1, then height-1
	data[21], data[22], data[23], data[24] = 0xFF, 0xFF, 0xFF, 0x0F
	_, err := ToJPEG(data)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !errors.Is(err, ErrTooBig) && !strings.Contains(err.Error(), "WebP") {
		t.Errorf("unexpected error: %v", err)
	}
}
