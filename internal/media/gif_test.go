package media

import (
	"image"
	"image/color"
	"image/color/palette"
	"image/gif"
	"os"
	"path/filepath"
	"testing"
)

func TestAnimatedGIFStaysAnimated(t *testing.T) {
	anim := &gif.GIF{}
	for i, c := range []color.Color{color.RGBA{R: 255, A: 255}, color.RGBA{G: 255, A: 255}, color.RGBA{B: 255, A: 255}} {
		frame := image.NewPaletted(image.Rect(0, 0, 600, 400), palette.Plan9)
		for y := 0; y < 400; y++ {
			for x := 0; x < 600; x++ {
				frame.Set(x, y, c)
			}
		}
		anim.Image = append(anim.Image, frame)
		anim.Delay = append(anim.Delay, 10*(i+1))
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "anim.gif")
	f, err := os.Create(src) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	if err := gif.EncodeAll(f, anim); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	info, err := Probe(src)
	if err != nil || info.Format != "gif" || info.Width != 600 || info.PageHeight != 400 || info.Height != 1200 {
		t.Fatalf("probe: %+v %v", info, err)
	}
	dst := filepath.Join(dir, "thumb.gif")
	res, err := Resize(src, dst, info, 245, 156)
	if err != nil || res.Width != 234 || res.Height != 156 {
		t.Fatalf("thumbnail: %+v %v", res, err)
	}
	out, err := os.Open(dst) //nolint:gosec // the test's own file
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = out.Close() }()
	got, err := gif.DecodeAll(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Image) != 3 || got.Delay[2] != 30 || got.Config.Width != 234 {
		t.Fatalf("the thumbnail keeps the 3 frames and their delays: %d frames, delays %v, width %d", len(got.Image), got.Delay, got.Config.Width)
	}
}
