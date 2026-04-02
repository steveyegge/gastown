// Package slackdm sends Slack DMs for mentions and assignments by resolving
// faultline account names to Slack user IDs via the slack_user_mappings table.
package slackdm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/outdoorsea/faultline/internal/db"
	"github.com/outdoorsea/faultline/internal/integrations"
	slackint "github.com/outdoorsea/faultline/internal/integrations/slack"
)

// Sender sends Slack DMs for mentions and assignments.
type Sender struct {
	DB  *db.DB
	Log *slog.Logger
}

// NotifyMentions sends Slack DMs to all @mentioned users in a comment who have
// linked Slack accounts. It also creates in-app notifications for all mentioned
// users (regardless of Slack linking).
func (s *Sender) NotifyMentions(ctx context.Context, projectID int64, groupID string, comment *db.Comment, actorID int64, actorName string) {
	if len(comment.Mentions) == 0 {
		return
	}

	// Resolve mention names to accounts.
	accounts, err := s.DB.AccountsByNames(ctx, comment.Mentions)
	if err != nil {
		s.Log.Error("resolve mention accounts", "err", err)
		return
	}

	// Create in-app notifications for each mentioned account.
	for _, acct := range accounts {
		if acct.ID == actorID {
			continue // don't notify yourself
		}
		title := fmt.Sprintf("%s mentioned you in a comment", actorName)
		if _, err := s.DB.CreateNotification(ctx, acct.ID, "mention", groupID, &comment.ID, actorID, actorName, title); err != nil {
			s.Log.Error("create mention notification", "account_id", acct.ID, "err", err)
		}
	}

	// Try to get a Slack bot for this project.
	bot := s.slackBotForProject(ctx, projectID)
	if bot == nil {
		return
	}

	for _, acct := range accounts {
		if acct.ID == actorID {
			continue
		}
		slackUserID, err := s.DB.SlackUserIDForAccount(ctx, acct.ID)
		if err != nil {
			s.Log.Error("lookup slack user", "account_id", acct.ID, "err", err)
			continue
		}
		if slackUserID == "" {
			continue // no Slack mapping
		}

		text := fmt.Sprintf("*%s* mentioned you in a comment on issue `%s`:\n> %s", actorName, groupID, truncate(comment.Body, 200))
		if err := bot.SendDM(ctx, slackUserID, text); err != nil {
			s.Log.Error("send mention DM", "slack_user", slackUserID, "err", err)
		}
	}
}

// NotifyAssignment sends a Slack DM and in-app notification to the assigned
// user when an issue is assigned to them.
func (s *Sender) NotifyAssignment(ctx context.Context, projectID int64, groupID, assignedTo, assignedBy string) {
	// Resolve the assignee name to an account.
	accounts, err := s.DB.AccountsByNames(ctx, []string{assignedTo})
	if err != nil || len(accounts) == 0 {
		if err != nil {
			s.Log.Error("resolve assignee account", "name", assignedTo, "err", err)
		}
		return
	}
	assignee := accounts[0]

	// Resolve actor for notification metadata.
	var actorID int64
	actorName := assignedBy
	actors, _ := s.DB.AccountsByNames(ctx, []string{assignedBy})
	if len(actors) > 0 {
		actorID = actors[0].ID
		actorName = actors[0].Name
	}

	// Don't notify if assigning to self.
	if actorID != 0 && actorID == assignee.ID {
		return
	}

	// Create in-app notification.
	title := fmt.Sprintf("%s assigned you to an issue", actorName)
	if _, err := s.DB.CreateNotification(ctx, assignee.ID, "assignment", groupID, nil, actorID, actorName, title); err != nil {
		s.Log.Error("create assignment notification", "account_id", assignee.ID, "err", err)
	}

	// Send Slack DM if mapping exists.
	bot := s.slackBotForProject(ctx, projectID)
	if bot == nil {
		return
	}

	slackUserID, err := s.DB.SlackUserIDForAccount(ctx, assignee.ID)
	if err != nil {
		s.Log.Error("lookup slack user for assignee", "account_id", assignee.ID, "err", err)
		return
	}
	if slackUserID == "" {
		return
	}

	text := fmt.Sprintf("*%s* assigned you to issue `%s`", actorName, groupID)
	if err := bot.SendDM(ctx, slackUserID, text); err != nil {
		s.Log.Error("send assignment DM", "slack_user", slackUserID, "err", err)
	}
}

// slackBotForProject loads the enabled Slack bot integration for a project.
// Returns nil if no Slack bot is configured/enabled.
func (s *Sender) slackBotForProject(ctx context.Context, projectID int64) *slackint.Bot {
	configs, err := s.DB.ListEnabledIntegrations(ctx, projectID)
	if err != nil {
		s.Log.Error("list integrations for slack DM", "project_id", projectID, "err", err)
		return nil
	}
	for _, cfg := range configs {
		if cfg.IntegrationType != string(integrations.TypeSlackBot) {
			continue
		}
		intg, err := integrations.New(integrations.TypeSlackBot, json.RawMessage(cfg.Config))
		if err != nil {
			s.Log.Warn("create slack bot for DM", "err", err)
			continue
		}
		if bot, ok := intg.(*slackint.Bot); ok {
			return bot
		}
	}
	return nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
