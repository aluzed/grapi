package runner

import (
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aluzed/grapi/compat/corpus"
)

// PerfResult is the outcome of a load run.
type PerfResult struct {
	Requests  int64   `json:"requests"`
	Errors    int64   `json:"errors"`
	Seconds   float64 `json:"seconds"`
	RPS       float64 `json:"rps"`
	P50ms     float64 `json:"p50ms"`
	P95ms     float64 `json:"p95ms"`
	P99ms     float64 `json:"p99ms"`
	Endpoints int     `json:"endpoints"`
}

// PerfURLs are the GET cases of files that need no variable.
func PerfURLs(base string, files []corpus.File) []string {
	var out []string
	for _, f := range files {
		for _, c := range f.Cases {
			if c.Method != "" && c.Method != http.MethodGet || c.Skip != "" || c.Token != "" || c.Bearer != "" {
				continue
			}
			if strings.Contains(c.Path+c.Query, "{{") {
				continue
			}
			u := strings.TrimRight(base, "/") + c.Path
			if c.Query != "" {
				u += "?" + encodeQuery(c.Query)
			}
			out = append(out, u)
		}
	}
	return out
}

// Perf sends the URLs round robin from concurrency workers for d.
func Perf(urls []string, concurrency int, d time.Duration) PerfResult {
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: concurrency * 2}}
	var (
		n, errs atomic.Int64
		mu      sync.Mutex
		lat     []time.Duration
		wg      sync.WaitGroup
	)
	deadline := time.Now().Add(d)
	start := time.Now()
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			var local []time.Duration
			for i := w; time.Now().Before(deadline); i += concurrency {
				u := urls[i%len(urls)]
				t0 := time.Now()
				res, err := client.Get(u)
				if err == nil {
					_, _ = io.Copy(io.Discard, res.Body)
					_ = res.Body.Close()
				}
				local = append(local, time.Since(t0))
				n.Add(1)
				if err != nil || res.StatusCode >= 500 {
					errs.Add(1)
				}
			}
			mu.Lock()
			lat = append(lat, local...)
			mu.Unlock()
		}(w)
	}
	wg.Wait()
	elapsed := time.Since(start).Seconds()
	sort.Slice(lat, func(i, j int) bool { return lat[i] < lat[j] })
	pct := func(p float64) float64 {
		if len(lat) == 0 {
			return 0
		}
		return float64(lat[int(p*float64(len(lat)-1))].Microseconds()) / 1000
	}
	return PerfResult{
		Requests: n.Load(), Errors: errs.Load(), Seconds: elapsed, RPS: float64(n.Load()) / elapsed,
		P50ms: pct(0.50), P95ms: pct(0.95), P99ms: pct(0.99), Endpoints: len(urls),
	}
}
