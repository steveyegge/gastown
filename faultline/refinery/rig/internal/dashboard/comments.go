package dashboard

import (
	"fmt"
	"net/http"
	"strings"
)

// createComment handles POST /dashboard/projects/{project_id}/issues/{issue_id}/comments.
// It creates a comment on the issue, generates notifications for @mentions,
// and sends Slack DMs to mentioned users who have linked Slack accounts.
func (h *Handler) createComment(w http.ResponseWriter, r *http.Request) {
	projectID := pathInt64(r, "project_id")
	issueID := r.PathValue("issue_id")
	accountID := h.currentAccountID(r)
	account := h.currentAccount(r)

	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}

	body := strings.TrimSpace(r.FormValue("body"))
	if body == "" {
		http.Redirect(w, r, fmt.Sprintf("/dashboard/projects/%d/issues/%s", projectID, issueID), http.StatusSeeOther)
		return
	}

	ctx := r.Context()
	comment, err := h.DB.CreateComment(ctx, projectID, issueID, accountID, body)
	if err != nil {
		h.Log.Error("create comment", "project_id", projectID, "issue_id", issueID, "err", err)
		http.Error(w, "failed to create comment", http.StatusInternalServerError)
		return
	}

	// Send Slack DMs and in-app notifications for @mentions.
	actorName := "someone"
	if account != nil {
		actorName = account.Name
	}
	if h.SlackDMs != nil {
		h.SlackDMs.NotifyMentions(ctx, projectID, issueID, comment, accountID, actorName)
	}

	http.Redirect(w, r, fmt.Sprintf("/dashboard/projects/%d/issues/%s", projectID, issueID), http.StatusSeeOther)
}
