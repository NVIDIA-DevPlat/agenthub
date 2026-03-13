package api

import (
	"crypto/rand"
	"fmt"
	"net/http"

	"github.com/NVIDIA-DevPlat/agenthub/src/internal/auth"
	"github.com/NVIDIA-DevPlat/agenthub/src/internal/store"
)

func (s *Server) handleSetupGet(w http.ResponseWriter, r *http.Request) {
	if !s.setupMode {
		http.Redirect(w, r, "/admin/", http.StatusSeeOther)
		return
	}
	s.render(w, "setup.html", pageData{Title: "Setup"})
}

func (s *Server) handleSetupPost(w http.ResponseWriter, r *http.Request) {
	if !s.setupMode {
		http.Redirect(w, r, "/admin/", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil {
		s.render(w, "setup.html", pageData{Title: "Setup", Error: "invalid form submission"})
		return
	}

	password := r.FormValue("password")
	confirm := r.FormValue("confirm_password")

	if password == "" {
		s.render(w, "setup.html", pageData{Title: "Setup", Error: "Password must not be empty."})
		return
	}
	if password != confirm {
		s.render(w, "setup.html", pageData{Title: "Setup", Error: "Passwords do not match."})
		return
	}

	// Open (create) the store with the chosen password.
	st, err := store.Open(s.storePath, password)
	if err != nil {
		s.render(w, "setup.html", pageData{Title: "Setup", Error: "Failed to create secrets store: " + err.Error()})
		return
	}

	// Hash the admin password.
	hash, err := auth.HashPassword(password)
	if err != nil {
		s.render(w, "setup.html", pageData{Title: "Setup", Error: "Failed to hash password."})
		return
	}
	if err := st.Set("admin_password_hash", hash); err != nil {
		s.render(w, "setup.html", pageData{Title: "Setup", Error: "Failed to save password hash."})
		return
	}

	// Generate a random session secret.
	sessionSecret, err := generateRandHex(32)
	if err != nil {
		s.render(w, "setup.html", pageData{Title: "Setup", Error: "Failed to generate session secret."})
		return
	}
	if err := st.Set("session_secret", sessionSecret); err != nil {
		s.render(w, "setup.html", pageData{Title: "Setup", Error: "Failed to save session secret."})
		return
	}

	// Generate a registration token for bot auto-registration.
	regToken, err := generateRandHex(16)
	if err != nil {
		s.render(w, "setup.html", pageData{Title: "Setup", Error: "Failed to generate registration token."})
		return
	}
	if err := st.Set("registration_token", regToken); err != nil {
		s.render(w, "setup.html", pageData{Title: "Setup", Error: "Failed to save registration token."})
		return
	}

	// Optionally save tokens if provided.
	for _, kv := range []struct{ form, key string }{
		{"openai_api_key", "openai_api_key"},
		{"slack_bot_token", "slack_bot_token"},
		{"slack_app_token", "slack_app_token"},
	} {
		if v := r.FormValue(kv.form); v != "" {
			_ = st.Set(kv.key, v)
		}
	}

	s.render(w, "setup.html", pageData{
		Title:   "Setup",
		Success: fmt.Sprintf("Setup complete! Registration token: %s — Restart agenthub with your password to begin.", regToken),
	})
}

// generateRandHex returns n random bytes encoded as a hex string.
func generateRandHex(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", buf), nil
}
