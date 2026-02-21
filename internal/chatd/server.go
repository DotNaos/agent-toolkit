package chatd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"
)

type Server struct {
	store        Store
	listenAddr   string
	dbPath       string
	startedAt    string
	pollInterval time.Duration
	httpServer   *http.Server
	lockFile     *os.File
}

type ServerConfig struct {
	ListenAddr   string
	DBPath       string
	PollInterval time.Duration
	Store        Store
}

func NewServer(cfg ServerConfig) (*Server, error) {
	if cfg.Store == nil {
		return nil, fmt.Errorf("store is required")
	}
	if strings.TrimSpace(cfg.ListenAddr) == "" {
		cfg.ListenAddr = DefaultListenAddr
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = DefaultPollInterval
	}

	lockFile, err := acquireDBLock(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	s := &Server{
		store:        cfg.Store,
		listenAddr:   cfg.ListenAddr,
		dbPath:       cfg.DBPath,
		startedAt:    nowTimestamp(),
		pollInterval: cfg.PollInterval,
		lockFile:     lockFile,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/health", s.handleHealth)
	mux.HandleFunc("/v1/messages/send", s.handleSend)
	mux.HandleFunc("/v1/messages/wait", s.handleWait)
	mux.HandleFunc("/v1/messages/ack", s.handleAck)

	s.httpServer = &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return s, nil
}

func (s *Server) Run() error {
	err := s.httpServer.ListenAndServe()
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func (s *Server) Close() error {
	if s.httpServer != nil {
		_ = s.httpServer.Close()
	}
	if s.store != nil {
		_ = s.store.Close()
	}
	if s.lockFile != nil {
		_ = syscall.Flock(int(s.lockFile.Fd()), syscall.LOCK_UN)
		_ = s.lockFile.Close()
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeMethodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, HealthResponse{
		Status:    "success",
		Action:    "health",
		Listen:    s.listenAddr,
		DBPath:    s.dbPath,
		StartedAt: s.startedAt,
	})
}

func (s *Server) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var req SendRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	msg, err := s.store.EnqueueMessage(EnqueueParams{
		ToAgent:   strings.TrimSpace(req.ToAgent),
		FromAgent: strings.TrimSpace(req.FromAgent),
		ThreadID:  strings.TrimSpace(req.ThreadID),
		Body:      req.Body,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	writeJSON(w, http.StatusOK, SendResponse{
		Status:    "success",
		Action:    "send",
		MessageID: msg.ID,
		ToAgent:   msg.ToAgent,
		ThreadID:  msg.ThreadID,
		CreatedAt: msg.CreatedAt,
	})
}

func (s *Server) handleWait(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var req WaitRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	agent := strings.TrimSpace(req.Agent)
	if agent == "" {
		writeError(w, http.StatusBadRequest, fmt.Errorf("agent is required"))
		return
	}

	timeout, err := parseWaitTimeout(req.Timeout)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	deadline := time.Now().Add(timeout)
	for {
		msg, err := s.store.LeaseNextMessage(LeaseParams{
			Agent:    agent,
			ThreadID: strings.TrimSpace(req.ThreadID),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if msg != nil {
			writeJSON(w, http.StatusOK, WaitResponse{
				Status:         "success",
				Action:         "wait",
				Message:        &msg.Message,
				LeaseToken:     msg.LeaseToken,
				LeaseExpiresAt: msg.LeaseExpiresAt,
			})
			return
		}

		remaining := time.Until(deadline)
		if remaining <= 0 {
			writeJSON(w, http.StatusOK, WaitResponse{
				Status:  "timeout",
				Action:  "wait",
				Agent:   agent,
				Timeout: timeout.String(),
			})
			return
		}

		sleepFor := s.pollInterval
		if remaining < sleepFor {
			sleepFor = remaining
		}

		timer := time.NewTimer(sleepFor)
		select {
		case <-r.Context().Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (s *Server) handleAck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}

	var req AckRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	ackedAt, err := s.store.AckMessage(AckParams{
		Agent:      strings.TrimSpace(req.Agent),
		MessageID:  strings.TrimSpace(req.MessageID),
		LeaseToken: strings.TrimSpace(req.LeaseToken),
	})
	if err != nil {
		statusCode := http.StatusBadRequest
		switch {
		case errors.Is(err, ErrMessageNotFound):
			statusCode = http.StatusNotFound
		case errors.Is(err, ErrLeaseConflict):
			statusCode = http.StatusConflict
		}
		writeError(w, statusCode, err)
		return
	}

	writeJSON(w, http.StatusOK, AckResponse{
		Status:  "success",
		Action:  "ack",
		ID:      req.MessageID,
		AckedAt: ackedAt.UTC().Format(timestampFormat),
	})
}

func parseWaitTimeout(raw string) (time.Duration, error) {
	if strings.TrimSpace(raw) == "" {
		return DefaultClientTimeout, nil
	}

	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid timeout %q: %w", raw, err)
	}
	if d < MinWaitTimeout || d > MaxWaitTimeout {
		return 0, fmt.Errorf("timeout must be between %s and %s", MinWaitTimeout, MaxWaitTimeout)
	}
	return d, nil
}

func decodeJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("invalid request body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, statusCode int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}

func writeError(w http.ResponseWriter, statusCode int, err error) {
	writeJSON(w, statusCode, map[string]any{
		"status":  "error",
		"message": err.Error(),
	})
}

func writeMethodNotAllowed(w http.ResponseWriter) {
	writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("method not allowed"))
}

func acquireDBLock(dbPath string) (*os.File, error) {
	lockPath := dbPath + ".lock"
	if err := os.MkdirAll(filepathDir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to prepare lock dir: %w", err)
	}

	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open db lock file: %w", err)
	}

	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lockFile.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("daemon already running for db %s", dbPath)
		}
		return nil, fmt.Errorf("failed to lock db file: %w", err)
	}

	return lockFile, nil
}

func filepathDir(path string) string {
	idx := strings.LastIndex(path, "/")
	if idx == -1 {
		return "."
	}
	return path[:idx]
}
