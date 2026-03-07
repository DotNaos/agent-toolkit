package hubapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-toolkit/internal/delegaterun"
	"agent-toolkit/internal/hubstore"
	"agent-toolkit/internal/hubworker"
)

type Server struct {
	listenAddr string
	webDir     string
	store      *hubstore.Store
	broker     *Broker
	worker     *hubworker.Manager
	delegate   *delegaterun.Runner
	httpServer *http.Server
}

type Config struct {
	ListenAddr         string
	DBPath             string
	WebDir             string
	DelegateConfigPath string
}

func NewServer(cfg Config) (*Server, error) {
	if strings.TrimSpace(cfg.ListenAddr) == "" {
		cfg.ListenAddr = "127.0.0.1:46001"
	}
	if strings.TrimSpace(cfg.DBPath) == "" {
		return nil, fmt.Errorf("db path is required")
	}

	store, err := hubstore.New(cfg.DBPath)
	if err != nil {
		return nil, err
	}

	broker := NewBroker()
	delegate := delegaterun.New(cfg.DelegateConfigPath)
	worker := hubworker.NewManager(store, broker, delegate)

	s := &Server{
		listenAddr: cfg.ListenAddr,
		webDir:     cfg.WebDir,
		store:      store,
		broker:     broker,
		worker:     worker,
		delegate:   delegate,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/v1/conversations", s.handleConversations)
	mux.HandleFunc("/v1/messages", s.handleMessages)
	mux.HandleFunc("/v1/events/stream", s.handleEventsStream)
	mux.HandleFunc("/v1/approvals/request", s.handleApprovalRequest)
	mux.HandleFunc("/v1/approvals/pending", s.handleApprovalPending)
	mux.HandleFunc("/v1/approvals/", s.handleApprovalRespond)
	mux.HandleFunc("/v1/agents/", s.handleDispatch)
	mux.HandleFunc("/v1/delegate/adapters", s.handleDelegateAdapters)
	mux.HandleFunc("/", s.handleWeb)

	s.httpServer = &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           s.withCORS(mux),
		ReadHeaderTimeout: 5 * time.Second,
	}

	return s, nil
}

func (s *Server) Run() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Close(ctx context.Context) error {
	if s.httpServer != nil {
		_ = s.httpServer.Shutdown(ctx)
	}
	if s.store != nil {
		return s.store.Close()
	}
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "listen": s.listenAddr})
}

func (s *Server) handleConversations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req CreateConversationRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		participants := make([]hubstore.ParticipantInput, 0, len(req.Participants))
		for _, p := range req.Participants {
			participants = append(participants, hubstore.ParticipantInput{Type: hubstore.ParticipantType(p.Type), ID: p.ID})
		}
		conversation, err := s.store.CreateConversation(hubstore.CreateConversationParams{Name: req.Name, Participants: participants})
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		s.broker.Publish(hubworker.Event{Type: "conversation.created", ConversationID: conversation.ID, Data: map[string]any{"conversation": conversation}})
		writeJSON(w, http.StatusOK, CreateConversationResponse{ConversationID: conversation.ID, CreatedAt: conversation.CreatedAt})
	case http.MethodGet:
		conversations, err := s.store.ListConversations(100)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"conversations": conversations})
	default:
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		var req PostMessageRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		msg, err := s.store.AddMessage(hubstore.AddMessageParams{
			ConversationID: req.ConversationID,
			FromID:         req.FromID,
			ToID:           req.ToID,
			Kind:           hubstore.MessageKind(req.Kind),
			Body:           req.Body,
		})
		if err != nil {
			code := http.StatusBadRequest
			if errors.Is(err, hubstore.ErrNotFound) {
				code = http.StatusNotFound
			}
			writeError(w, code, err)
			return
		}
		s.broker.Publish(hubworker.Event{Type: "message.created", ConversationID: msg.ConversationID, Data: map[string]any{"message": msg}})
		writeJSON(w, http.StatusOK, PostMessageResponse{MessageID: msg.ID, CreatedAt: msg.CreatedAt})
	case http.MethodGet:
		conversationID := strings.TrimSpace(r.URL.Query().Get("conversation_id"))
		cursor := strings.TrimSpace(r.URL.Query().Get("cursor"))
		messages, nextCursor, err := s.store.ListMessages(conversationID, cursor, 100)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, ListMessagesResponse{Messages: messages, NextCursor: nextCursor})
	default:
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (s *Server) handleEventsStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	conversationID := strings.TrimSpace(r.URL.Query().Get("conversation_id"))

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, errors.New("streaming unsupported"))
		return
	}

	subID, ch := s.broker.Subscribe(conversationID)
	defer s.broker.Unsubscribe(subID)

	fmt.Fprintf(w, ": connected\n\n")
	flusher.Flush()

	pingTicker := time.NewTicker(25 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-pingTicker.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		case evt := <-ch:
			data, err := json.Marshal(evt.Data)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\n", evt.Type)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (s *Server) handleApprovalRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}

	var req ApprovalRequestCreate
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	expiresAt, err := time.Parse(time.RFC3339, req.ExpiresAt)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("invalid expires_at: %w", err))
		return
	}
	approval, err := s.store.CreateApprovalRequest(hubstore.CreateApprovalRequestParams{
		ConversationID: req.ConversationID,
		AgentID:        req.AgentID,
		Title:          req.Title,
		Description:    req.Description,
		SchemaJSON:     string(req.Schema),
		RiskLevel:      req.RiskLevel,
		ExpiresAt:      expiresAt,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	s.broker.Publish(hubworker.Event{Type: "approval.requested", ConversationID: approval.ConversationID, Data: map[string]any{"approval": approval}})
	writeJSON(w, http.StatusOK, ApprovalRequestCreateResponse{ApprovalID: approval.ID, Status: string(approval.Status)})
}

func (s *Server) handleApprovalPending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	_, _ = s.store.MarkExpiredApprovals(time.Now().UTC())
	conversationID := strings.TrimSpace(r.URL.Query().Get("conversation_id"))
	items, err := s.store.ListPendingApprovals(conversationID, 100)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"approvals": items})
}

func (s *Server) handleApprovalRespond(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	// path: /v1/approvals/{id}/respond
	path := strings.TrimPrefix(r.URL.Path, "/v1/approvals/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[1] != "respond" {
		writeError(w, http.StatusNotFound, errors.New("not found"))
		return
	}
	approvalID := parts[0]

	var req ApprovalRespondRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	payload := req.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	approval, err := s.store.RespondApproval(hubstore.RespondApprovalParams{
		ApprovalID:  approvalID,
		HumanID:     req.HumanID,
		Decision:    req.Decision,
		PayloadJSON: string(payload),
	})
	if err != nil {
		code := http.StatusBadRequest
		switch {
		case errors.Is(err, hubstore.ErrNotFound):
			code = http.StatusNotFound
		case errors.Is(err, hubstore.ErrAlreadyResolved):
			code = http.StatusConflict
		}
		writeError(w, code, err)
		return
	}
	resolvedAt := ""
	if approval.ResolvedAt != nil {
		resolvedAt = *approval.ResolvedAt
	}
	resp := ApprovalRespondResponse{ApprovalID: approval.ID, Status: string(approval.Status), ResolvedAt: resolvedAt}
	s.broker.Publish(hubworker.Event{Type: "approval.resolved", ConversationID: approval.ConversationID, Data: map[string]any{"approval": approval}})
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleDispatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	// path: /v1/agents/{agent_id}/dispatch
	path := strings.TrimPrefix(r.URL.Path, "/v1/agents/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 || parts[1] != "dispatch" {
		writeError(w, http.StatusNotFound, errors.New("not found"))
		return
	}
	agentID := parts[0]

	var req DispatchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	dispatch, err := s.worker.DispatchAgent(req.ConversationID, agentID, req.Prompt, req.Metadata)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, DispatchResponse{DispatchID: dispatch.ID, Status: string(dispatch.Status)})
}

func (s *Server) handleDelegateAdapters(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		return
	}
	adapters, err := s.delegate.ListEnabledAdapters()
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"adapters": adapters})
}

func (s *Server) handleWeb(w http.ResponseWriter, r *http.Request) {
	if strings.TrimSpace(s.webDir) == "" {
		writeError(w, http.StatusNotFound, errors.New("web ui not configured"))
		return
	}
	cleanPath := filepath.Clean(r.URL.Path)
	if cleanPath == "." || cleanPath == "/" {
		http.ServeFile(w, r, filepath.Join(s.webDir, "index.html"))
		return
	}

	candidate := filepath.Join(s.webDir, strings.TrimPrefix(cleanPath, "/"))
	if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
		http.ServeFile(w, r, candidate)
		return
	}
	// SPA fallback
	http.ServeFile(w, r, filepath.Join(s.webDir, "index.html"))
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
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

func writeError(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]any{"status": "error", "message": err.Error()})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}
