package media

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestFitInside(t *testing.T) {
	// Dimensions Strapi 5.59 (sharp) produced on the compat bench.
	cases := []struct{ w, h, maxW, maxH, wantW, wantH int }{
		{400, 400, 245, 156, 156, 156},
		{2000, 1500, 245, 156, 208, 156},
		{1280, 720, 245, 156, 245, 138},
		{1200, 800, 1000, 1000, 1000, 667},
		{600, 600, 500, 500, 500, 500},
		{900, 600, 750, 750, 750, 500},
		{1200, 800, 245, 156, 234, 156},
		{1280, 720, 1000, 1000, 1000, 563},
		{1280, 720, 500, 500, 500, 281},
	}
	for _, c := range cases {
		w, h := FitInside(c.w, c.h, c.maxW, c.maxH)
		if w != c.wantW || h != c.wantH {
			t.Errorf("FitInside(%dx%d in %dx%d) = %dx%d, want %dx%d", c.w, c.h, c.maxW, c.maxH, w, h, c.wantW, c.wantH)
		}
	}
}

func TestGenerateFileName(t *testing.T) {
	re := regexp.MustCompile(`^Cafe_creme_[0-9a-f]{10}$`)
	if got := GenerateFileName("Café crème"); !re.MatchString(got) {
		t.Fatalf("GenerateFileName = %q", got)
	}
	if BytesToKbytes(42234) != 42.23 || BytesToKbytes(44315) != 44.32 {
		t.Fatal("BytesToKbytes")
	}
}

func TestProbeAndResize(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.png")
	f, err := os.Create(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, image.NewRGBA(image.Rect(0, 0, 1280, 720))); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	info, err := Probe(src)
	if err != nil || info.Format != "png" || info.Width != 1280 || info.Height != 720 {
		t.Fatalf("Probe = %+v, %v", info, err)
	}
	res, err := Resize(src, filepath.Join(dir, "t.png"), info, 245, 156)
	if err != nil || res.Width != 245 || res.Height != 138 || res.Size == 0 {
		t.Fatalf("Resize = %+v, %v", res, err)
	}
	if _, err := Probe(filepath.Join(dir, "missing")); err == nil {
		t.Fatal("want an error")
	}
	txt := filepath.Join(dir, "a.txt")
	_ = os.WriteFile(txt, []byte("hello"), 0o600)
	if _, err := Probe(txt); err != ErrNotImage {
		t.Fatalf("Probe(text) = %v", err)
	}
}
