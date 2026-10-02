package worker

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // register PNG decoder

	"golang.org/x/image/draw"
	"golang.org/x/image/webp"
)

const jpegQuality = 85

type Resizer interface {
	Resize(raw []byte, width, height int) ([]byte, error)
}

type JPEGResizer struct{}

func NewResizer() *JPEGResizer { return &JPEGResizer{} }

func (*JPEGResizer) Resize(raw []byte, width, height int) ([]byte, error) {
	if width <= 0 || height <= 0 {
		return nil, permanent(fmt.Errorf("resize image: invalid size %dx%d", width, height))
	}
	img, err := decodeImage(raw)
	if err != nil {
		return nil, permanent(fmt.Errorf("resize image: %w", err))
	}

	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, permanent(fmt.Errorf("encode thumbnail: %w", err))
	}
	return buf.Bytes(), nil
}

func decodeImage(raw []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err == nil {
		return img, nil
	}
	img, webpErr := webp.Decode(bytes.NewReader(raw))
	if webpErr != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	return img, nil
}
