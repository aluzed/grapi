package media

import (
	"encoding/binary"
	"image"
	"io"
	"os"
)

// exifOrientation reads the EXIF orientation (1 to 8) of a JPEG; 1 when
// there is none.
func exifOrientation(path string) int {
	f, err := os.Open(path) //nolint:gosec // a temporary upload file
	if err != nil {
		return 1
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, 64<<10)
	n, _ := io.ReadFull(f, head)
	b := head[:n]
	if len(b) < 4 || b[0] != 0xFF || b[1] != 0xD8 {
		return 1
	}
	for i := 2; i+4 <= len(b) && b[i] == 0xFF; {
		marker, size := b[i+1], int(binary.BigEndian.Uint16(b[i+2:]))
		seg := b[i+4 : min(len(b), i+2+size)]
		if marker == 0xE1 && len(seg) > 14 && string(seg[:6]) == "Exif\x00\x00" {
			return tiffOrientation(seg[6:])
		}
		if marker == 0xDA {
			break
		}
		i += 2 + size
	}
	return 1
}

func tiffOrientation(t []byte) int {
	var order binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		order = binary.LittleEndian
	case "MM":
		order = binary.BigEndian
	default:
		return 1
	}
	ifd := int(order.Uint32(t[4:]))
	if ifd+2 > len(t) {
		return 1
	}
	count := int(order.Uint16(t[ifd:]))
	for e := 0; e < count; e++ {
		at := ifd + 2 + e*12
		if at+12 > len(t) {
			break
		}
		if order.Uint16(t[at:]) == 0x0112 {
			if o := int(order.Uint16(t[at+8:])); o >= 1 && o <= 8 {
				return o
			}
		}
	}
	return 1
}

// orient applies an EXIF orientation, as sharp's rotate() without angle.
func orient(img image.Image, o int) image.Image {
	if o <= 1 || o > 8 {
		return img
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	src := func(dx, dy int) (int, int) {
		switch o {
		case 2:
			return w - 1 - dx, dy
		case 3:
			return w - 1 - dx, h - 1 - dy
		case 4:
			return dx, h - 1 - dy
		case 5:
			return dy, dx
		case 6:
			return dy, h - 1 - dx
		case 7:
			return w - 1 - dy, h - 1 - dx
		}
		return w - 1 - dy, dx
	}
	out := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			sx, sy := src(x, y)
			out.Set(x, y, img.At(b.Min.X+sx, b.Min.Y+sy))
		}
	}
	return out
}
