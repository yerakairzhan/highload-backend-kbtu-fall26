package loadgen

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"sis2/internal/client"
)

func readJSON[T any](path string) (T, error) {
	var v T
	b, err := os.ReadFile(path)
	if err != nil {
		return v, err
	}
	return v, json.Unmarshal(b, &v)
}

func median(xs []float64) float64 {
	s := slices.Clone(xs)
	slices.Sort(s)
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

// Report renders the result files in dir as Markdown tables.
func Report(dir string, w io.Writer) error {
	files, _ := filepath.Glob(filepath.Join(dir, "bench-*.json"))
	type key struct {
		s Scenario
		t client.Target
	}
	groups := map[key][]BenchResult{}
	reps := 0
	for _, f := range files {
		r, err := readJSON[BenchResult](f)
		if err != nil {
			return fmt.Errorf("%s: %w", f, err)
		}
		k := key{r.Scenario, r.Target}
		groups[k] = append(groups[k], r)
		reps = max(reps, len(groups[k]))
	}
	if len(groups) > 0 {
		fmt.Fprintf(w, "### Latency and bytes per request (median of %d runs)\n\n", reps)
		fmt.Fprintln(w, "| scenario | target | p50 ms | p90 ms | p99 ms | p99 range ms | p99.9 ms | sent B/req | received B/req | failed |")
		fmt.Fprintln(w, "|---|---|--:|--:|--:|--:|--:|--:|--:|--:|")
		for _, s := range AllScenarios {
			for _, t := range client.AllTargets {
				g := groups[key{s, t}]
				if len(g) == 0 {
					continue
				}
				med := func(f func(BenchResult) float64) float64 {
					xs := make([]float64, len(g))
					for i, r := range g {
						xs[i] = f(r)
					}
					return median(xs)
				}
				p99s := make([]float64, len(g))
				failed := 0
				for i, r := range g {
					p99s[i] = r.Latency.P99
					failed += r.Failed + r.Dropped
				}
				fmt.Fprintf(w, "| %s | %s | %.3f | %.3f | %.3f | %.3f–%.3f | %.3f | %.0f | %.0f | %d |\n",
					s, t,
					med(func(r BenchResult) float64 { return r.Latency.P50 }),
					med(func(r BenchResult) float64 { return r.Latency.P90 }),
					med(func(r BenchResult) float64 { return r.Latency.P99 }),
					slices.Min(p99s), slices.Max(p99s),
					med(func(r BenchResult) float64 { return r.Latency.P999 }),
					med(func(r BenchResult) float64 { return r.WireSentPerReq }),
					med(func(r BenchResult) float64 { return r.WireReceivedPerReq }),
					failed)
			}
		}
		fmt.Fprintln(w)
	}

	if sizes, err := readJSON[[]MessageSize](filepath.Join(dir, "sizes.json")); err == nil {
		sort.SliceStable(sizes, func(i, j int) bool {
			return slices.Index(AllScenarios, sizes[i].Scenario) < slices.Index(AllScenarios, sizes[j].Scenario) ||
				(sizes[i].Scenario == sizes[j].Scenario && sizes[i].Direction > sizes[j].Direction)
		})
		fmt.Fprintln(w, "### Message size: protobuf vs JSON (body only, no headers)")
		fmt.Fprintln(w)
		fmt.Fprintln(w, "| scenario | direction | message | protobuf B | JSON B | JSON / protobuf |")
		fmt.Fprintln(w, "|---|---|---|--:|--:|--:|")
		for _, m := range sizes {
			fmt.Fprintf(w, "| %s | %s | %s | %d | %d | %.2f× |\n",
				m.Scenario, m.Direction, m.Message, m.Proto, m.JSON, m.JSONToProto)
		}
		fmt.Fprintln(w)
	}
	return nil
}
