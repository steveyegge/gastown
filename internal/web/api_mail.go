package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// MailMessage represents a mail message for the API.
type MailMessage struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to"`
	Subject   string `json:"subject"`
	Body      string `json:"body,omitempty"`
	Timestamp string `json:"timestamp"`
	Read      bool   `json:"read"`
	Priority  string `json:"priority,omitempty"`
}

// MailInboxResponse is the response for /api/mail/inbox.
type MailInboxResponse struct {
	Messages    []MailMessage `json:"messages"`
	UnreadCount int           `json:"unread_count"`
	Total       int           `json:"total"`
}

// handleMailInbox returns the user's inbox.
func (h *APIHandler) handleMailInbox(w http.ResponseWriter, r *http.Request) {
	output, err := h.runGtCommand(r.Context(), 10*time.Second, []string{"mail", "inbox", "--json"})
	if err != nil {
		// Try without --json flag
		output, err = h.runGtCommand(r.Context(), 10*time.Second, []string{"mail", "inbox"})
		if err != nil {
			h.sendError(w, "Failed to fetch inbox: "+err.Error(), http.StatusInternalServerError)
			return
		}
		// Parse text output
		messages := parseMailInboxText(output)
		unread := 0
		for _, m := range messages {
			if !m.Read {
				unread++
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(MailInboxResponse{
			Messages:    messages,
			UnreadCount: unread,
			Total:       len(messages),
		})
		return
	}

	// Parse JSON output
	var messages []MailMessage
	if err := json.Unmarshal([]byte(output), &messages); err != nil {
		h.sendError(w, "Failed to parse inbox: "+err.Error(), http.StatusInternalServerError)
		return
	}

	unread := 0
	for _, m := range messages {
		if !m.Read {
			unread++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(MailInboxResponse{
		Messages:    messages,
		UnreadCount: unread,
		Total:       len(messages),
	})
}

// handleMailRead reads a specific message by ID.
func (h *APIHandler) handleMailRead(w http.ResponseWriter, r *http.Request) {
	msgID := r.URL.Query().Get("id")
	if msgID == "" {
		h.sendError(w, "Missing message ID", http.StatusBadRequest)
		return
	}
	if !isValidID(msgID) {
		h.sendError(w, "Invalid message ID format", http.StatusBadRequest)
		return
	}

	output, err := h.runGtCommand(r.Context(), 10*time.Second, []string{"mail", "read", msgID})
	if err != nil {
		h.sendError(w, "Failed to read message: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Parse the message output
	msg := parseMailReadOutput(output, msgID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(msg)
}

// MailSendRequest is the request body for /api/mail/send.
type MailSendRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
	ReplyTo string `json:"reply_to,omitempty"`
}

// handleMailSend sends a new message.
func (h *APIHandler) handleMailSend(w http.ResponseWriter, r *http.Request) {
	var req MailSendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.sendError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.To == "" || req.Subject == "" {
		h.sendError(w, "Missing required fields (to, subject)", http.StatusBadRequest)
		return
	}
	if !isValidMailAddress(req.To) {
		h.sendError(w, "Invalid recipient format", http.StatusBadRequest)
		return
	}
	if req.ReplyTo != "" && !isValidID(req.ReplyTo) {
		h.sendError(w, "Invalid reply-to ID format", http.StatusBadRequest)
		return
	}

	// Enforce length limits (consistent with handleIssueCreate)
	const maxSubjectLen = 500
	const maxBodyLen = 100_000
	if len(req.Subject) > maxSubjectLen {
		h.sendError(w, fmt.Sprintf("Subject too long (max %d bytes)", maxSubjectLen), http.StatusBadRequest)
		return
	}
	if len(req.Body) > maxBodyLen {
		h.sendError(w, fmt.Sprintf("Body too long (max %d bytes)", maxBodyLen), http.StatusBadRequest)
		return
	}
	if strings.Contains(req.Subject, "\x00") || strings.Contains(req.Body, "\x00") {
		h.sendError(w, "Subject and body cannot contain null bytes", http.StatusBadRequest)
		return
	}

	// Build mail send command. Flags go first, then -- to end flag parsing,
	// then the positional recipient (consistent with handleIssueCreate/handleInstall).
	args := []string{"mail", "send"}
	args = append(args, "-s", req.Subject)
	if req.Body != "" {
		args = append(args, "-m", req.Body)
	}
	if req.ReplyTo != "" {
		args = append(args, "--reply-to", req.ReplyTo)
	}
	args = append(args, "--", req.To)

	output, err := h.runGtCommand(r.Context(), 30*time.Second, args)
	if err != nil {
		h.sendError(w, "Failed to send message: "+err.Error()+"\n"+output, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Message sent",
		"output":  output,
	})
}

// parseMailInboxText parses text output from "gt mail inbox".
func parseMailInboxText(output string) []MailMessage {
	var messages []MailMessage
	lines := strings.Split(output, "\n")

	// Format: "  1. ● subject" or "  1. subject" (● = unread)
	// followed by "      id from sender"
	// followed by "      timestamp"
	var current *MailMessage
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "\U0001f4ec") || strings.HasPrefix(trimmed, "(no messages)") {
			continue
		}

		// Check for numbered message line
		if len(trimmed) > 2 && trimmed[0] >= '1' && trimmed[0] <= '9' && trimmed[1] == '.' {
			// Save previous message
			if current != nil {
				messages = append(messages, *current)
			}
			current = &MailMessage{}
			// Parse "1. ● subject" or "1. subject"
			rest := strings.TrimSpace(trimmed[2:])
			if strings.HasPrefix(rest, "\u25cf") {
				current.Read = false
				current.Subject = strings.TrimSpace(strings.TrimPrefix(rest, "\u25cf"))
			} else {
				current.Read = true
				current.Subject = rest
			}
		} else if current != nil && current.ID == "" && strings.Contains(trimmed, " from ") {
			// Parse "id from sender"
			parts := strings.SplitN(trimmed, " from ", 2)
			if len(parts) == 2 {
				current.ID = strings.TrimSpace(parts[0])
				current.From = strings.TrimSpace(parts[1])
			}
		} else if current != nil && current.Timestamp == "" && (strings.Contains(trimmed, "-") || strings.Contains(trimmed, ":")) {
			current.Timestamp = trimmed
		}
	}
	// Don't forget the last one
	if current != nil && current.ID != "" {
		messages = append(messages, *current)
	}

	return messages
}

// parseMailReadOutput parses the output from "gt mail read <id>".
func parseMailReadOutput(output string, msgID string) MailMessage {
	msg := MailMessage{ID: msgID}
	lines := strings.Split(output, "\n")

	inBody := false
	var bodyLines []string

	for _, line := range lines {
		if strings.HasPrefix(line, "\U0001f4ec ") || strings.HasPrefix(line, "Subject: ") {
			msg.Subject = strings.TrimPrefix(strings.TrimPrefix(line, "\U0001f4ec "), "Subject: ")
			msg.Subject = strings.TrimSpace(msg.Subject)
		} else if strings.HasPrefix(line, "From: ") {
			msg.From = strings.TrimPrefix(line, "From: ")
		} else if strings.HasPrefix(line, "To: ") {
			msg.To = strings.TrimPrefix(line, "To: ")
		} else if strings.HasPrefix(line, "ID: ") {
			msg.ID = strings.TrimPrefix(line, "ID: ")
		} else if line == "" && msg.From != "" && !inBody {
			inBody = true
		} else if inBody {
			bodyLines = append(bodyLines, line)
		}
	}

	msg.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
	return msg
}
