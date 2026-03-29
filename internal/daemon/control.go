package daemon

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/hatsunemiku3939/jobsd/internal/domain"
)

const jobsTokenHeader = "X-Jobs-Token"

type PingResponse struct {
	Instance  string                 `json:"instance"`
	Status    domain.SchedulerStatus `json:"status"`
	PID       int                    `json:"pid"`
	Port      int                    `json:"port"`
	StartedAt string                 `json:"started_at"`
	Version   string                 `json:"version"`
}

type SchedulerResponse struct {
	Instance  string                 `json:"instance"`
	Status    domain.SchedulerStatus `json:"status"`
	PID       int                    `json:"pid"`
	Port      int                    `json:"port"`
	DBPath    string                 `json:"db_path"`
	StartedAt string                 `json:"started_at"`
	Version   string                 `json:"version"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func generateToken() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate scheduler token: %w", err)
	}

	return hex.EncodeToString(buf), nil
}

func listenControlListener(port int) (net.Listener, int, error) {
	address := "127.0.0.1:0"
	if port > 0 {
		address = fmt.Sprintf("127.0.0.1:%d", port)
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return nil, 0, fmt.Errorf("listen control api: %w", err)
	}

	addr, ok := listener.Addr().(*net.TCPAddr)
	if !ok {
		_ = listener.Close()
		return nil, 0, fmt.Errorf("resolve control api port: unexpected listener address %T", listener.Addr())
	}

	return listener, addr.Port, nil
}

func startControlServer(
	listener net.Listener,
	state domain.SchedulerState,
	requestShutdown func(),
) (*http.Server, <-chan error, error) {
	if listener == nil {
		return nil, nil, fmt.Errorf("control api listener is required")
	}

	mux := http.NewServeMux()
	mux.Handle("/v1/ping", withTokenAuth(state.Token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w)
			return
		}

		writeJSON(w, http.StatusOK, PingResponse{
			Instance:  state.Instance,
			Status:    domain.SchedulerStatusRunning,
			PID:       state.PID,
			Port:      state.Port,
			StartedAt: state.StartedAt.UTC().Format(time.RFC3339),
			Version:   state.Version,
		})
	})))
	mux.Handle("/v1/scheduler", withTokenAuth(state.Token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeMethodNotAllowed(w)
			return
		}

		writeJSON(w, http.StatusOK, SchedulerResponse{
			Instance:  state.Instance,
			Status:    domain.SchedulerStatusRunning,
			PID:       state.PID,
			Port:      state.Port,
			DBPath:    state.DBPath,
			StartedAt: state.StartedAt.UTC().Format(time.RFC3339),
			Version:   state.Version,
		})
	})))
	mux.Handle("/v1/scheduler/shutdown", withTokenAuth(state.Token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeMethodNotAllowed(w)
			return
		}

		w.WriteHeader(http.StatusNoContent)
		requestShutdown()
	})))

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		err := server.Serve(listener)
		if err == nil || err == http.ErrServerClosed {
			errCh <- nil
			return
		}
		errCh <- fmt.Errorf("serve control api: %w", err)
	}()

	return server, errCh, nil
}

func withTokenAuth(token string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(jobsTokenHeader) != token {
			writeJSON(w, http.StatusUnauthorized, errorResponse{
				Error: "missing or invalid X-Jobs-Token",
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, errorResponse{
		Error: "method not allowed",
	})
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func shutdownControlServer(ctx context.Context, server *http.Server) error {
	if server == nil {
		return nil
	}

	if err := server.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown control api: %w", err)
	}

	return nil
}
