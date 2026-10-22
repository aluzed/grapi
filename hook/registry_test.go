package hook

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

func TestSeries(t *testing.T) {
	h := &Series[int]{}
	h.Register(func(_ context.Context, v int) (int, error) { return v + 1, nil })
	off := h.Register(func(_ context.Context, v int) (int, error) { return v + 2, nil })
	out, err := h.Call(context.Background(), 10)
	if err != nil || len(out) != 2 || out[0] != 11 || out[1] != 12 {
		t.Fatalf("%v %v", out, err)
	}
	off()
	h.Register(func(context.Context, int) (int, error) { return 0, errors.New("stop") })
	h.Register(func(context.Context, int) (int, error) { t.Error("runs after an error"); return 0, nil })
	if out, err := h.Call(context.Background(), 1); err == nil || len(out) != 1 {
		t.Fatalf("an error stops: %v %v", out, err)
	}
}

func TestWaterfall(t *testing.T) {
	h := &Waterfall[string]{}
	h.Register(func(_ context.Context, v string) (string, error) { return v + "b", nil })
	h.Register(func(_ context.Context, v string) (string, error) { return v + "c", nil })
	if v, err := h.Call(context.Background(), "a"); err != nil || v != "abc" {
		t.Fatalf("%q %v", v, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := h.Call(ctx, "a"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}

func TestParallel(t *testing.T) {
	h := &Parallel[int]{}
	var n atomic.Int32
	for i := range 5 {
		h.Register(func(_ context.Context, v int) (int, error) {
			n.Add(1)
			if i == 3 {
				return 0, errors.New("third")
			}
			return v * i, nil
		})
	}
	out, err := h.Call(context.Background(), 2)
	if err == nil || err.Error() != "third" || n.Load() != 5 || out[4] != 8 {
		t.Fatalf("all run, the first error is returned: %v %v %d", out, err, n.Load())
	}
}

func TestBail(t *testing.T) {
	h := &Bail[string]{}
	h.Register(func(context.Context, string) (string, error) { return "", nil })
	h.Register(func(_ context.Context, v string) (string, error) { return "found " + v, nil })
	h.Register(func(context.Context, string) (string, error) { t.Error("runs after a value"); return "", nil })
	if v, ok, err := h.Call(context.Background(), "x"); err != nil || !ok || v != "found x" {
		t.Fatalf("%q %v %v", v, ok, err)
	}
	if _, ok, _ := (&Bail[string]{}).Call(context.Background(), "x"); ok {
		t.Fatal("no handler, no value")
	}
}

func TestRegistry(t *testing.T) {
	r := &Registry{}
	r.Register("plugin::demo.seeded", &Series[int]{})
	if _, ok := r.Get("plugin::demo.seeded").(*Series[int]); !ok {
		t.Fatal("get")
	}
	r.Delete("plugin::demo.seeded")
	if r.Get("plugin::demo.seeded") != nil {
		t.Fatal("delete")
	}
}
