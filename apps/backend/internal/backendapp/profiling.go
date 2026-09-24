package backendapp

import (
	"context"
	"expvar"
	"fmt"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

// isProfilingEnabled returns true if KANDEV_PROFILE is set to "1" or "true".
func isProfilingEnabled() bool {
	val := os.Getenv("KANDEV_PROFILE")
	return val == "1" || strings.EqualFold(val, "true")
}

// startProfilingIfEnabled starts the profiling server when KANDEV_PROFILE is
// enabled, registers its cleanup, and writes the endpoint file when
// KANDEV_PROFILE_PPROF_FILE is set.
func startProfilingIfEnabled(log *logger.Logger, cleanups *[]func() error) {
	if !isProfilingEnabled() {
		return
	}
	stopProfiling, pprofEndpoint, pprofErr := startProfilingServer(log)
	if pprofErr != nil {
		log.Warn("failed to start profiling pprof server", zap.Error(pprofErr))
		return
	}
	*cleanups = append(*cleanups, stopProfiling)
	pprofFile := os.Getenv("KANDEV_PROFILE_PPROF_FILE")
	if pprofFile == "" {
		return
	}
	if writeErr := os.WriteFile(pprofFile, []byte(pprofEndpoint+"\n"), 0o600); writeErr != nil {
		log.Warn("failed to write profiling endpoint file", zap.String("file", pprofFile), zap.Error(writeErr))
	}
}

// startProfilingServer starts a dedicated loopback HTTP server on an ephemeral port
// serving standard pprof and expvar endpoints when KANDEV_PROFILE=1 is enabled.
// It returns a cleanup function to gracefully shut down the server and the bound endpoint URL.
func startProfilingServer(log *logger.Logger) (func() error, string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, "", fmt.Errorf("bind profiling pprof listener: %w", err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	for _, name := range []string{"allocs", "block", "goroutine", "heap", "mutex", "threadcreate"} {
		mux.Handle("/debug/pprof/"+name, pprof.Handler(name))
	}
	mux.Handle("/debug/vars", expvar.Handler())

	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 120 * time.Second, // Allow CPU profiles up to standard durations
	}

	endpoint := fmt.Sprintf("http://127.0.0.1:%d", ln.Addr().(*net.TCPAddr).Port)

	go func() {
		if serveErr := server.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			if log != nil {
				log.Warn("Profiling pprof server stopped with error", zap.Error(serveErr))
			}
		}
	}()

	if log != nil {
		log.Info("pprof profiling server listening", zap.String("endpoint", endpoint))
	}
	fmt.Fprintf(os.Stderr, "[pprof] profiling server listening at %s/debug/pprof/\n", endpoint)

	cleanup := func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	}

	return cleanup, endpoint, nil
}
