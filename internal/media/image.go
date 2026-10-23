package media

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color/palette"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"math"
	"os"
	"regexp"
	"strconv"

	"golang.org/x/image/draw"
	"golang.org/x/image/tiff"
	_ "golang.org/x/image/webp" // registers the webp decoder
)

// Info is what sharp's metadata() gives the upload plugin.
type Info struct {
	// Format is sharp's format name: jpeg, png, webp, tiff, gif, svg or avif.
	Format string
	Width  int
	Height int
	// PageHeight is the height of one frame of an animated image.
	PageHeight int
}

// ErrNotImage is returned for files sharp cannot read.
var ErrNotImage = errors.New("not a supported image")

var (
	svgTag  = regexp.MustCompile(`(?is)<svg\b[^>]*>`)
	svgAttr = regexp.MustCompile(`(?is)\b(width|height|viewBox)\s*=\s*["']([^"']*)["']`)
)

// Probe reads an image's format and dimensions.
func Probe(path string) (Info, error) {
	f, err := os.Open(path) //nolint:gosec // a temporary upload file
	if err != nil {
		return Info{}, err
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 512)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return Info{}, err
	}
	if isAVIF(head) {
		return Info{Format: "avif"}, nil
	}
	cfg, format, err := image.DecodeConfig(bufio.NewReader(f))
	if err != nil {
		return probeSVG(path)
	}
	info := Info{Format: format, Width: cfg.Width, Height: cfg.Height}
	if format == "gif" {
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return Info{}, err
		}
		if all, err := gif.DecodeAll(f); err == nil && len(all.Image) > 1 {
			info.PageHeight = all.Config.Height
			info.Height = all.Config.Height * len(all.Image)
		}
	}
	return info, nil
}

func isAVIF(head []byte) bool {
	return len(head) >= 12 && string(head[4:8]) == "ftyp" && (string(head[8:12]) == "avif" || string(head[8:12]) == "avis")
}

func probeSVG(path string) (Info, error) {
	b, err := os.ReadFile(path) //nolint:gosec // a temporary upload file
	if err != nil {
		return Info{}, err
	}
	tag := svgTag.Find(b)
	if tag == nil {
		return Info{}, ErrNotImage
	}
	info := Info{Format: "svg"}
	var viewBox []float64
	for _, m := range svgAttr.FindAllSubmatch(tag, -1) {
		switch string(m[1]) {
		case "width":
			info.Width = svgLength(string(m[2]))
		case "height":
			info.Height = svgLength(string(m[2]))
		case "viewBox":
			for _, p := range regexp.MustCompile(`[\s,]+`).Split(string(bytes.TrimSpace(m[2])), -1) {
				if v, err := strconv.ParseFloat(p, 64); err == nil {
					viewBox = append(viewBox, v)
				}
			}
		}
	}
	if (info.Width == 0 || info.Height == 0) && len(viewBox) == 4 {
		info.Width, info.Height = int(math.Round(viewBox[2])), int(math.Round(viewBox[3]))
	}
	return info, nil
}

func svgLength(s string) int {
	m := regexp.MustCompile(`^\s*([\d.]+)`).FindStringSubmatch(s)
	if m == nil {
		return 0
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	return int(math.Round(v))
}

// FitInside is sharp's resize({width, height, fit: 'inside'}) target size.
func FitInside(w, h, maxW, maxH int) (int, int) {
	xf := float64(w) / float64(maxW)
	yf := float64(h) / float64(maxH)
	if xf > yf {
		return maxW, int(math.Round(float64(h) / xf))
	}
	return int(math.Round(float64(w) / yf)), maxH
}

// Resized is a format written by Resize.
type Resized struct {
	Width, Height int
	Size          int64
	// Image is the decoded result of Optimize, for the formats.
	Image image.Image
}

var resizable = map[string]bool{"jpeg": true, "png": true, "tiff": true, "gif": true}

// CanResize reports whether the pure Go processor writes this format;
// sharp also writes webp, which needs libvips.
func CanResize(format string) bool { return resizable[format] }

// Resize writes src resized to fit inside maxW x maxH, in src's format.
func Resize(src, dst string, info Info, maxW, maxH int) (Resized, error) {
	f, err := os.Open(src) //nolint:gosec // a temporary upload file
	if err != nil {
		return Resized{}, err
	}
	defer func() { _ = f.Close() }()
	pageH := info.Height
	if info.PageHeight > 0 {
		pageH = info.PageHeight
	}
	w, h := FitInside(info.Width, pageH, maxW, maxH)
	out, err := os.Create(dst) //nolint:gosec // inside the request's working directory
	if err != nil {
		return Resized{}, err
	}
	defer func() { _ = out.Close() }()
	switch info.Format {
	case "gif":
		all, err := gif.DecodeAll(f)
		if err != nil {
			return Resized{}, err
		}
		frames := make([]*image.Paletted, len(all.Image))
		canvas := image.NewRGBA(image.Rect(0, 0, all.Config.Width, all.Config.Height))
		for i, frame := range all.Image {
			draw.Draw(canvas, frame.Bounds(), frame, frame.Bounds().Min, draw.Over)
			p := image.NewPaletted(image.Rect(0, 0, w, h), palette.Plan9)
			draw.CatmullRom.Scale(p, p.Bounds(), canvas, canvas.Bounds(), draw.Src, nil)
			frames[i] = p
		}
		all.Image = frames
		all.Config.Width, all.Config.Height = w, h
		if err := gif.EncodeAll(out, all); err != nil {
			return Resized{}, err
		}
	default:
		img, _, err := image.Decode(f)
		if err != nil {
			return Resized{}, err
		}
		if err := scaleTo(out, img, info.Format, w, h); err != nil {
			return Resized{}, err
		}
	}
	if err := out.Close(); err != nil {
		return Resized{}, err
	}
	st, err := os.Stat(dst)
	if err != nil {
		return Resized{}, err
	}
	return Resized{Width: w, Height: h, Size: st.Size()}, nil
}

func encode(w io.Writer, img image.Image, format string, quality int) error {
	switch format {
	case "jpeg":
		return jpeg.Encode(w, img, &jpeg.Options{Quality: quality})
	case "png":
		return (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(w, img)
	case "tiff":
		return tiff.Encode(w, img, &tiff.Options{Compression: tiff.Deflate})
	}
	return fmt.Errorf("cannot encode %s", format)
}

var optimizable = map[string]bool{"jpeg": true, "png": true, "tiff": true}

// Optimize re-encodes src at quality (80 for sizeOptimization, 100 for
// orientation only), turned upright by its EXIF orientation when autoOrient
// is set. ok is false when the format is not handled or the result is
// bigger than the original, which Strapi then keeps.
func Optimize(src, dst string, info Info, quality int, autoOrient bool) (Resized, bool, error) {
	if !optimizable[info.Format] {
		return Resized{}, false, nil
	}
	f, err := os.Open(src) //nolint:gosec // a temporary upload file
	if err != nil {
		return Resized{}, false, err
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	if err != nil {
		return Resized{}, false, err
	}
	turned := false
	if autoOrient && info.Format == "jpeg" {
		if o := exifOrientation(src); o > 1 {
			img, turned = orient(img, o), true
		}
	}
	out, err := os.Create(dst) //nolint:gosec // inside the request's working directory
	if err != nil {
		return Resized{}, false, err
	}
	if err := encode(out, img, info.Format, quality); err != nil {
		_ = out.Close()
		return Resized{}, false, err
	}
	if err := out.Close(); err != nil {
		return Resized{}, false, err
	}
	before, err := os.Stat(src)
	if err != nil {
		return Resized{}, false, err
	}
	after, err := os.Stat(dst)
	if err != nil {
		return Resized{}, false, err
	}
	// A turned image is kept even when bigger: the original is sideways.
	if after.Size() > before.Size() && !turned {
		return Resized{Image: img}, false, nil
	}
	b := img.Bounds()
	return Resized{Width: b.Dx(), Height: b.Dy(), Size: after.Size(), Image: img}, true, nil
}

func scaleTo(out io.Writer, img image.Image, format string, w, h int) error {
	dstImg := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.CatmullRom.Scale(dstImg, dstImg.Bounds(), img, img.Bounds(), draw.Over, nil)
	return encode(out, dstImg, format, 80)
}

// Decode reads a still image once, for several ResizeFrom calls: decoding a
// large photo for every format multiplies the memory it takes.
func Decode(src string) (image.Image, error) {
	f, err := os.Open(src) //nolint:gosec // a temporary upload file
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	img, _, err := image.Decode(f)
	return img, err
}

// ResizeFrom is Resize for an image already decoded (not an animated GIF).
func ResizeFrom(img image.Image, dst string, info Info, maxW, maxH int) (Resized, error) {
	w, h := FitInside(info.Width, info.Height, maxW, maxH)
	out, err := os.Create(dst) //nolint:gosec // inside the request's working directory
	if err != nil {
		return Resized{}, err
	}
	if err := scaleTo(out, img, info.Format, w, h); err != nil {
		_ = out.Close()
		return Resized{}, err
	}
	if err := out.Close(); err != nil {
		return Resized{}, err
	}
	st, err := os.Stat(dst)
	if err != nil {
		return Resized{}, err
	}
	return Resized{Width: w, Height: h, Size: st.Size()}, nil
}
