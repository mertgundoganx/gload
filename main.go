package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/mertgundoganx/gload/internal/logger"
	"github.com/mertgundoganx/gload/internal/metrics"
	"github.com/mertgundoganx/gload/internal/runner"
	"github.com/mertgundoganx/gload/internal/scheduler"
	"github.com/mertgundoganx/gload/internal/server"
	"github.com/mertgundoganx/gload/internal/storage"
	"github.com/mertgundoganx/gload/internal/ui"
	"github.com/mertgundoganx/gload/internal/worker"
	"github.com/mertgundoganx/gload/pkg/config"
)

// Version is the release version. Release builds override it via
// -ldflags "-X main.Version=<tag>"; this is the fallback for plain `go build`.
var Version = "1.1.0"

func main() {
	// Check --version before config.Parse (which calls flag.Parse internally)
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-version" {
			fmt.Printf("gload version %s\n", Version)
			os.Exit(0)
		}
	}

	// Register log flags so config.Parse (flag.Parse) picks them up
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	logJSON := flag.Bool("log-json", false, "Enable JSON log output")
	cfg, err := config.Parse()

	// Stamp the release version onto the outgoing User-Agent (CLI, web, worker).
	runner.UserAgent = fmt.Sprintf("gload/%s", Version)

	// Configure structured logger (flags are now parsed by config.Parse)
	switch strings.ToLower(*logLevel) {
	case "debug":
		logger.SetLevel(logger.DEBUG)
	case "warn":
		logger.SetLevel(logger.WARN)
	case "error":
		logger.SetLevel(logger.ERROR)
	default:
		logger.SetLevel(logger.INFO)
	}
	if *logJSON {
		logger.SetJSON(true)
		// Set static fields for ELK/Loki/Datadog structured log ingestion.
		logger.SetStatic("service", "gload")
		logger.SetStatic("version", Version)
		logger.SetStatic("pid", os.Getpid())
		if hn, err := os.Hostname(); err == nil {
			logger.SetStatic("hostname", hn)
		}
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "\nUsage: gload -u <url> [options]\n")
		fmt.Fprintf(os.Stderr, "  -u  Target URL (required)\n")
		fmt.Fprintf(os.Stderr, "  -m  HTTP method (default: GET)\n")
		fmt.Fprintf(os.Stderr, "  -c  Concurrent workers (default: 10)\n")
		fmt.Fprintf(os.Stderr, "  -d  Duration (default: 10s)\n")
		fmt.Fprintf(os.Stderr, "  -H  Header (repeatable)\n")
		fmt.Fprintf(os.Stderr, "  -b  Request body\n")
		fmt.Fprintf(os.Stderr, "  -t  Timeout (default: 30s)\n")
		fmt.Fprintf(os.Stderr, "  -r  Requests per second limit (default: 0, unlimited)\n")
		fmt.Fprintf(os.Stderr, "  --no-ui  Disable TUI\n")
		fmt.Fprintf(os.Stderr, "  --web    Start web UI server\n")
		fmt.Fprintf(os.Stderr, "  --port   Web server port (default: 8080)\n")
		os.Exit(1)
	}

	if cfg.WorkerMode {
		ws := worker.NewWorkerServer()
		if err := ws.ListenAndServe(cfg.Port); err != nil {
			fmt.Fprintf(os.Stderr, "Worker error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	if cfg.WebMode {
		dbPath, err := storage.DefaultDBPath()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		store, err := storage.New(dbPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
			os.Exit(1)
		}
		defer store.Close()

		srv := server.New(store)
		server.AppVersion = Version

		sched := scheduler.New(store, srv.RunScheduledTest)
		sched.Start()
		defer sched.Stop()

		httpServer, err := srv.CreateHTTPServer(cfg.Port)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		srv.StartRetentionWorker()
		srv.ResumeQueue() // resume any queue persisted from a previous run

		// Set HTTP server timeouts to prevent resource exhaustion.
		httpServer.ReadHeaderTimeout = 10 * time.Second
		httpServer.IdleTimeout = 120 * time.Second

		// Graceful shutdown
		go func() {
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			sig := <-sigCh
			logger.Info("received signal, shutting down gracefully", logger.Fields("signal", sig.String()))

			// Second signal = force exit immediately.
			go func() {
				<-sigCh
				logger.Warn("received second signal, forcing exit")
				os.Exit(1)
			}()

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			// 1. Stop accepting new scheduled tests.
			sched.Stop()
			logger.Info("scheduler stopped")

			// 2. Wait for running tests to complete (or timeout).
			srv.WaitForTests(ctx)

			// 3. Shutdown HTTP server (drains active connections).
			if err := httpServer.Shutdown(ctx); err != nil {
				logger.Error("http server shutdown error", logger.Fields("error", err.Error()))
			} else {
				logger.Info("http server stopped")
			}
		}()

		logger.Info("gload web server started", logger.Fields("port", cfg.Port, "version", Version))

		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		logger.Info("shutdown complete")
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	r := runner.New(cfg)

	go r.Run(ctx)

	if cfg.NoUI {
		// Ctrl+C cancels the run and still prints partial results. (Use Ctrl+C,
		// not Ctrl+Break — Ctrl+Break asks the Go runtime to dump every
		// goroutine's stack, which floods the terminal during a big test.)
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			if _, ok := <-sigCh; ok {
				fmt.Fprintln(os.Stderr, "\nStopping (signal received)...")
				cancel()
			}
		}()

		<-r.Done
		signal.Stop(sigCh)
		close(sigCh)
		printSummary(r, cfg)

		// CI mode: exit 1 if error rate > 10% (basic threshold)
		snap := r.Metrics.Snapshot()
		if snap.TotalReqs > 0 {
			errRate := float64(snap.Errors) / float64(snap.TotalReqs) * 100
			if errRate > 10 {
				fmt.Fprintf(os.Stderr, "\nTest FAILED: error rate %.1f%% exceeds 10%% threshold\n", errRate)
				os.Exit(1)
			}
		}
		return
	}

	m := ui.NewModel(cfg, r, cancel)
	p := tea.NewProgram(m, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func printSummary(r *runner.Runner, cfg *config.Config) {
	snap := r.Metrics.Snapshot()

	fmt.Println()
	fmt.Println("  gload - Load Test Results")
	fmt.Println("  " + strings.Repeat("─", 40))
	fmt.Printf("  Target:       %s %s\n", cfg.Method, cfg.URL)
	fmt.Printf("  Duration:     %s\n", formatDuration(snap.Duration))
	fmt.Printf("  Concurrency:  %d\n", cfg.Concurrency)
	fmt.Println("  " + strings.Repeat("─", 40))
	fmt.Printf("  Requests:     %d\n", snap.TotalReqs)
	fmt.Printf("  RPS:          %.1f\n", snap.RPS)
	fmt.Printf("  Errors:       %d (%.1f%%)\n", snap.Errors, errorRate(snap))
	fmt.Println("  " + strings.Repeat("─", 40))
	fmt.Printf("  Avg Latency:  %s\n", formatLatency(snap.AvgLatency))
	fmt.Printf("  P50 Latency:  %s\n", formatLatency(snap.P50Latency))
	fmt.Printf("  P95 Latency:  %s\n", formatLatency(snap.P95Latency))
	fmt.Printf("  P99 Latency:  %s\n", formatLatency(snap.P99Latency))
	fmt.Printf("  Min Latency:  %s\n", formatLatency(snap.MinLatency))
	fmt.Printf("  Max Latency:  %s\n", formatLatency(snap.MaxLatency))
	fmt.Println("  " + strings.Repeat("─", 40))

	codes := make([]int, 0, len(snap.StatusCodes))
	for k := range snap.StatusCodes {
		codes = append(codes, k)
	}
	sort.Ints(codes)
	for _, code := range codes {
		label := fmt.Sprintf("%d", code)
		if code == 0 {
			label = "ERR"
		}
		fmt.Printf("  HTTP %s:    %d\n", label, snap.StatusCodes[code])
	}

	fmt.Println()
}

func errorRate(snap metrics.Snapshot) float64 {
	if snap.TotalReqs == 0 {
		return 0
	}
	return float64(snap.Errors) / float64(snap.TotalReqs) * 100
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func formatLatency(d time.Duration) string {
	if d == 0 {
		return "-"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dus", d.Microseconds())
	}
	if d < time.Second {
		return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000)
	}
	return fmt.Sprintf("%.2fs", d.Seconds())
}
