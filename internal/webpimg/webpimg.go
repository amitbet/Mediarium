// Package webpimg turns WebP pictures into JPEG. Cover art from some sources
// (book files, cover sites, comic pages) comes as WebP, which the rest of the
// program does not keep: covers are stored and sent as JPEG or PNG. Importing
// this package also lets image.Decode read WebP. Only decoding exists in Go's
// WebP package, so a picture never goes back to WebP.
package webpimg

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"

	"golang.org/x/image/webp"
)

const (
	// maxPixels bounds a picture that gets decoded: far more than any cover.
	maxPixels = 25_000_000
	// jpegQuality is the quality the converted picture is written with.
	jpegQuality = 85
)

// ErrTooBig is returned for a picture with more pixels than a cover needs.
var ErrTooBig = errors.New("the picture is too large to convert")

// ToJPEG decodes a WebP picture and writes it as a JPEG. Transparent parts
// are put on white, because JPEG has no transparency.
func ToJPEG(data []byte) ([]byte, error) {
	cfg, err := webp.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("read WebP size: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxPixels || cfg.Height > maxPixels || cfg.Width*cfg.Height > maxPixels {
		return nil, fmt.Errorf("%w (%dx%d)", ErrTooBig, cfg.Width, cfg.Height)
	}
	img, err := webp.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("decode WebP: %w", err)
	}
	b := img.Bounds()
	flat := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(flat, flat.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	draw.Draw(flat, flat.Bounds(), img, b.Min, draw.Over)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, flat, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, fmt.Errorf("write JPEG: %w", err)
	}
	return buf.Bytes(), nil
}
