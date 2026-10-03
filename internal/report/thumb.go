package report

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"io/fs"
	"os"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const thumbSize = 480

// thumbnail returns the image as a JPEG data URI that fits thumbSize. It returns "" when the
// file is missing or is not an image Go can decode, so the page shows a placeholder.
func thumbnail(path string) (string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read cover %s: %w", path, err)
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", nil
	}

	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w > thumbSize || h > thumbSize {
		if w >= h {
			h, w = h*thumbSize/w, thumbSize
		} else {
			w, h = w*thumbSize/h, thumbSize
		}
		dst := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
		src = dst
	}

	var out bytes.Buffer
	if err = jpeg.Encode(&out, src, &jpeg.Options{Quality: 85}); err != nil {
		return "", fmt.Errorf("encode thumbnail: %w", err)
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(out.Bytes()), nil
}
