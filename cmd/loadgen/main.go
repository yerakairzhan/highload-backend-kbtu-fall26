package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"sis2/internal/client"
	"sis2/internal/loadgen"

	log "github.com/sirupsen/logrus"
)

const usage = `loadgen <command> [flags]

commands:
  bench   one (target, scenario) run -> JSON
  sizes   protobuf vs JSON size of real messages -> JSON
  report  Markdown tables from a results directory`

func writeJSON(path string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if path == "-" {
		fmt.Println(string(b))
		return
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Infof("wrote %s", path)
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println(usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd, args := os.Args[1], os.Args[2:]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	url := fs.String("url", "http://blog-platform:8080", "server base URL")
	out := fs.String("out", "-", "output file (- = stdout)")
	password := fs.String("password", "lab-password", "password of the seeded users")

	switch cmd {
	case "bench":
		target := fs.String("target", "grpc", "grpc | json_h1")
		scenario := fs.String("scenario", "get_post", "get_post | list_comments | create_comment")
		rate := fs.Int("rate", 2000, "requests per second (open loop)")
		dur := fs.Duration("duration", 20*time.Second, "measured duration")
		warmup := fs.Duration("warmup", 5*time.Second, "warm-up duration, not measured")
		rep := fs.Int("rep", 1, "repetition number, recorded in the result")
		users := fs.Int("users", 10, "seeded users to log in; requests rotate over their tokens")
		_ = fs.Parse(args)
		r, err := loadgen.Bench(ctx, loadgen.BenchConfig{
			URL: *url, Target: client.Target(*target), Scenario: loadgen.Scenario(*scenario),
			Rate: *rate, Duration: *dur, Warmup: *warmup, Rep: *rep, Users: *users, Password: *password,
		})
		if err != nil {
			log.Fatal(err)
		}
		log.Infof("%s %s: p50=%.3f p99=%.3f ms, failed=%d, sent=%.0f recv=%.0f B/req",
			r.Target, r.Scenario, r.Latency.P50, r.Latency.P99, r.Failed, r.WireSentPerReq, r.WireReceivedPerReq)
		writeJSON(*out, r)
	case "sizes":
		_ = fs.Parse(args)
		r, err := loadgen.Sizes(ctx, *url, *password)
		if err != nil {
			log.Fatal(err)
		}
		writeJSON(*out, r)
	case "report":
		dir := fs.String("dir", "results", "results directory")
		_ = fs.Parse(args)
		if err := loadgen.Report(*dir, os.Stdout); err != nil {
			log.Fatal(err)
		}
	default:
		fmt.Println(usage)
		os.Exit(2)
	}
}
