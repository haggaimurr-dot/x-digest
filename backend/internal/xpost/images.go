package xpost

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/image/draw"
)

// Image limits for downloaded X post images.
const (
	maxImageWidth   = 800
	jpegQuality     = 75
	maxImageBytes   = 10 << 20 // 10MB download cap per image
	// MaxImagesPerPost caps how many images are kept per post.
	MaxImagesPerPost = 4
	imageTimeout    = 30 * time.Second
)

// DownloadedImage is a compressed image ready to store.
type DownloadedImage struct {
	Data   []byte // JPEG bytes
	Width  int
	Height int
}

// DownloadAndCompress fetches an image URL and returns a resized JPEG.
// Images wider than maxImageWidth are scaled down; smaller ones are kept as-is.
func DownloadAndCompress(ctx context.Context, rawURL string) (*DownloadedImage, error) {
	ctx, cancel := context.WithTimeout(ctx, imageTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; x-digest/1.0)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes))
	if err != nil {
		return nil, err
	}
	img, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	 }
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return nil, fmt.Errorf("invalid dimensions %dx%d", w, h)
	}
	// Scale down if wider than the limit.
	if w > maxImageWidth {
		scale := float64(maxImageWidth) / float64(w)
		nw := maxImageWidth
		nh := int(float64(h) * scale)
		if nh < 1 {
			nh = 1
		}
		dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
		draw.CatmullRom.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
		img = dst
		w, h = nw, nh
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, err
	}
	return &DownloadedImage{Data: buf.Bytes(), Width: w, Height: h}, nil
}

// SaveImage writes compressed data to dir as name and returns the full path.
func SaveImage(dir, name string, data []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return "", err
	}
	return p, nil
}
