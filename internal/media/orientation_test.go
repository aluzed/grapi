package media

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"
)

// jpegWithOrientation encodes a 32 x 16 JPEG, red on the left half and blue
// on the right, with an EXIF APP1 segment carrying orientation o.
func jpegWithOrientation(t *testing.T, o uint16) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 32, 16))
	for y := 0; y < 16; y++ {
		for x := 0; x < 32; x++ {
			c := color.RGBA{R: 255, A: 255}
			if x >= 16 {
				c = color.RGBA{B: 255, A: 255}
			}
			img.Set(x, y, c)
		}
	}
	var enc bytes.Buffer
	if err := jpeg.Encode(&enc, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	var tiff bytes.Buffer
	tiff.WriteString("II*\x00")
	_ = binary.Write(&tiff, binary.LittleEndian, uint32(8))
	_ = binary.Write(&tiff, binary.LittleEndian, uint16(1))
	_ = binary.Write(&tiff, binary.LittleEndian, []uint16{0x0112, 3})
	_ = binary.Write(&tiff, binary.LittleEndian, uint32(1))
	_ = binary.Write(&tiff, binary.LittleEndian, []uint16{o, 0})
	_ = binary.Write(&tiff, binary.LittleEndian, uint32(0))
	app1 := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	var out bytes.Buffer
	out.Write(enc.Bytes()[:2])
	out.Write([]byte{0xFF, 0xE1})
	_ = binary.Write(&out, binary.BigEndian, uint16(len(app1)+2))
	out.Write(app1)
	out.Write(enc.Bytes()[2:])
	p := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(p, out.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAutoOrientation(t *testing.T) {
	src := jpegWithOrientation(t, 6)
	if o := exifOrientation(src); o != 6 {
		t.Fatalf("orientation = %d", o)
	}
	info, err := Probe(src)
	if err != nil || info.Width != 32 || info.Height != 16 {
		t.Fatalf("probe: %+v %v", info, err)
	}
	dst := filepath.Join(t.TempDir(), "out.jpg")
	res, ok, err := Optimize(src, dst, info, 100, true)
	if err != nil || !ok || res.Width != 16 || res.Height != 32 {
		t.Fatalf("orientation 6 turns the image upright, 16 x 32: %+v %v %v", res, ok, err)
	}
	f, _ := os.Open(dst) //nolint:gosec // the test's own file
	defer func() { _ = f.Close() }()
	img, err := jpeg.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	// 90 degrees clockwise: the left half (red) becomes the top.
	if r, _, b, _ := img.At(8, 4).RGBA(); r < b {
		t.Fatalf("top should be red: %v", img.At(8, 4))
	}
	if r, _, b, _ := img.At(8, 28).RGBA(); b < r {
		t.Fatalf("bottom should be blue: %v", img.At(8, 28))
	}
	if _, ok, _ := Optimize(src, dst, info, 100, false); ok {
		if out, _ := Probe(dst); out.Width != 32 {
			t.Fatalf("without autoOrientation the image stays as stored: %+v", out)
		}
	}
}
