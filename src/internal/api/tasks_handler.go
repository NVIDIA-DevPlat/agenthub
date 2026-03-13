package api

import (
	"encoding/json"
	"net/http"
)

var validStatuses = map[string]bool{
	"open":        true,
	"in_progress": true,
	"blocked":     true,
	"done":        true,
	"backlog":     true,
	"ready":       true,
	"review":      true,
}

type taskStatusRequest struct {
	Status string `json:"status"`
	Note   string `json:"note"`
}

func (s *Server) handleTaskStatusUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.validateRegistrationToken(r) {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	actor := r.Header.Get("X-Bot-Name")
	if actor == "" {
		actor = "bot"
	}

	var req taskStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}
	if !validStatuses[req.Status] {
		http.Error(w, `{"error":"invalid status"}`, http.StatusBadRequest)
		return
	}

	issueID := r.PathValue("id")

	if s.taskManager == nil {
		http.Error(w, `{"error":"task management not configured"}`, http.StatusServiceUnavailable)
		return
	}

	if err := s.taskManager.UpdateStatus(r.Context(), issueID, req.Status, req.Note, actor); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
