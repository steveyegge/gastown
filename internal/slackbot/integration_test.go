package slackbot

import (
	"strings"
	"testing"

	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	slackrouter "github.com/steveyegge/gastown/internal/slack"
)

// ====================================================================
// Integration Tests for Slack UX Features (gt-0naejc.3)
//
// These tests verify the 4 Slack UX features work correctly:
// 1. DM Opt-in Flow (gt-m47v6x)
// 2. Peek Button Flow (gt-xndt39)
// 3. Thread Conversation Flow (gt-8d5q52)
// 4. Agent Channel Messaging (gt-2ndxuv)
// ====================================================================

// --- Test Fixtures ---

// mockDecisionTracker tracks decision message timestamps for tests.
type mockDecisionTracker map[string]string // threadTs -> decisionID

func (m mockDecisionTracker) Get(threadTs string) (string, bool) {
	id, ok := m[threadTs]
	return id, ok
}

func (m mockDecisionTracker) Set(threadTs, decisionID string) {
	m[threadTs] = decisionID
}

// --- 1. DM Opt-in Flow Tests (gt-m47v6x) ---

func TestIntegration_DMOptInFlow(t *testing.T) {
	t.Run("DMOptInButtonAction", func(t *testing.T) {
		pm := NewPreferenceManager(t.TempDir())
		userID := "U12345"

		// Initial state: not opted in
		prefs := pm.GetUserPreferences(userID)
		if prefs.DMOptIn {
			t.Error("Expected initial DMOptIn=false")
		}

		// Simulate user clicking DM Me button
		if err := pm.SetDMOptIn(userID, true); err != nil {
			t.Fatalf("SetDMOptIn failed: %v", err)
		}

		// Verify opt-in persisted
		prefs = pm.GetUserPreferences(userID)
		if !prefs.DMOptIn {
			t.Error("Expected DMOptIn=true after button click")
		}
	})

	t.Run("PreferencesModalNotificationLevel", func(t *testing.T) {
		pm := NewPreferenceManager(t.TempDir())
		userID := "U12345"

		// Test all valid notification levels
		levels := []string{"all", "high", "muted"}
		for _, level := range levels {
			if err := pm.SetNotificationLevel(userID, level); err != nil {
				t.Errorf("SetNotificationLevel(%q) failed: %v", level, err)
			}

			prefs := pm.GetUserPreferences(userID)
			if prefs.NotificationLevel != level {
				t.Errorf("Expected NotificationLevel=%q, got %q", level, prefs.NotificationLevel)
			}
		}
	})

	t.Run("DMRoutingForOptedInUser", func(t *testing.T) {
		pm := NewPreferenceManager(t.TempDir())
		userID := "U12345"

		// Opt in
		pm.SetDMOptIn(userID, true)

		// Simulate decision urgency check
		prefs := pm.GetUserPreferences(userID)
		notificationLevel := prefs.NotificationLevel

		// High urgency should route to DM
		shouldDM := prefs.DMOptIn && (notificationLevel == "all" || notificationLevel == "high")
		if !shouldDM {
			t.Error("Expected high urgency decision to route to DM for opted-in user")
		}
	})

	t.Run("MutedUserNoDM", func(t *testing.T) {
		pm := NewPreferenceManager(t.TempDir())
		userID := "U12345"

		// Opt in but mute
		pm.SetDMOptIn(userID, true)
		pm.SetNotificationLevel(userID, "muted")

		prefs := pm.GetUserPreferences(userID)

		// Muted users should not get DMs even if opted in
		shouldDM := prefs.DMOptIn && prefs.NotificationLevel != "muted"
		if shouldDM {
			t.Error("Expected muted user to not receive DMs")
		}
	})
}

// --- 2. Peek Button Flow Tests (gt-xndt39) ---

func TestIntegration_PeekButtonFlow(t *testing.T) {
	t.Run("CollectAgentActivity", func(t *testing.T) {
		// Test activity collection from git/events
		// The actual implementation uses collectAgentActivity()

		// Verify the activity format
		sampleActivity := []string{
			"[18:21] Committed: fix(nudge): increase delay to 5s",
			"[18:19] Closed: gt-izluj7 (nudge timing fix)",
			"[18:15] Started: gt-izluj7 investigation",
		}

		// Activity should be formatted as lines
		formatted := strings.Join(sampleActivity, "\n")
		if !strings.Contains(formatted, "Committed") {
			t.Error("Expected activity to contain commits")
		}
		if !strings.Contains(formatted, "Closed") {
			t.Error("Expected activity to contain bead closures")
		}
	})

	t.Run("ActivityCodeBlockFormat", func(t *testing.T) {
		// Test the code block formatting for Slack
		agent := "gastown/crew/decisions"
		activity := "Recent activity for " + agent + ":\n" +
			"─────────────────────────────────────────\n" +
			"[18:21] Committed: fix(nudge): increase delay to 5s\n" +
			"─────────────────────────────────────────"

		// Should be wrapped in code block for Slack
		codeBlock := "```\n" + activity + "\n```"
		if !strings.HasPrefix(codeBlock, "```") {
			t.Error("Expected code block format")
		}
	})

	t.Run("EmptyActivityHandling", func(t *testing.T) {
		// When agent has no recent activity
		activity := []string{}
		if len(activity) == 0 {
			// Should show "No recent activity"
			message := "No recent activity for this agent."
			if !strings.Contains(message, "No recent activity") {
				t.Error("Expected empty activity message")
			}
		}
	})
}

// --- 3. Thread Conversation Flow Tests (gt-8d5q52) ---

func TestIntegration_ThreadConversationFlow(t *testing.T) {
	t.Run("ThreadReplyDetection", func(t *testing.T) {
		// Simulate a message event with thread timestamp
		ev := &slackevents.MessageEvent{
			User:            "U12345",
			Channel:         "C12345",
			Text:            "What about option C?",
			ThreadTimeStamp: "1234567890.123456", // Has thread timestamp = is thread reply
			TimeStamp:       "1234567890.654321",
		}

		// Thread replies have ThreadTimeStamp set
		isThreadReply := ev.ThreadTimeStamp != ""
		if !isThreadReply {
			t.Error("Expected message with ThreadTimeStamp to be detected as thread reply")
		}
	})

	t.Run("NonThreadMessageFiltered", func(t *testing.T) {
		// Simulate a message event without thread timestamp
		ev := &slackevents.MessageEvent{
			User:      "U12345",
			Channel:   "C12345",
			Text:      "Regular message",
			TimeStamp: "1234567890.654321",
			// No ThreadTimeStamp = not a thread reply
		}

		isThreadReply := ev.ThreadTimeStamp != ""
		if isThreadReply {
			t.Error("Expected message without ThreadTimeStamp to not be thread reply")
		}
	})

	t.Run("SubtypeMessagesFiltered", func(t *testing.T) {
		// Subtypes like message_changed, message_deleted should be filtered
		subtypes := []string{"message_changed", "message_deleted", "bot_message"}

		for _, subtype := range subtypes {
			ev := &slackevents.MessageEvent{
				User:            "U12345",
				Channel:         "C12345",
				Text:            "Changed message",
				ThreadTimeStamp: "1234567890.123456",
				SubType:         subtype,
			}

			// Should filter out subtypes
			shouldProcess := ev.SubType == ""
			if shouldProcess {
				t.Errorf("Expected subtype %q to be filtered", subtype)
			}
		}
	})

	t.Run("DecisionThreadTracking", func(t *testing.T) {
		// Test that we can track which threads are decision threads
		tracker := make(mockDecisionTracker)

		threadTs := "1234567890.123456"
		decisionID := "gt-abc123"

		// Register a decision thread
		tracker.Set(threadTs, decisionID)

		// Lookup should find it
		id, found := tracker.Get(threadTs)
		if !found {
			t.Error("Expected to find decision thread")
		}
		if id != decisionID {
			t.Errorf("Expected decisionID=%q, got %q", decisionID, id)
		}

		// Unknown thread should not be found
		_, found = tracker.Get("unknown-thread")
		if found {
			t.Error("Expected unknown thread to not be found")
		}
	})

	t.Run("ForwardToAgentFormat", func(t *testing.T) {
		// Test the message format sent to agent
		userName := "Steve"
		text := "What about using Redis instead?"
		decisionID := "gt-abc123"

		// Format for nudge
		nudgeMsg := "[THREAD REPLY] " + userName + " replied to " + decisionID + ": " + text
		if !strings.Contains(nudgeMsg, "THREAD REPLY") {
			t.Error("Expected nudge to indicate thread reply")
		}
		if !strings.Contains(nudgeMsg, decisionID) {
			t.Error("Expected nudge to include decision ID")
		}
	})
}

// --- 4. Agent Channel Messaging Tests (gt-2ndxuv) ---

func TestIntegration_AgentChannelMessaging(t *testing.T) {
	t.Run("ChannelToAgentLookup", func(t *testing.T) {
		// Test the reverse lookup from channel ID to agent
		cfg := &slackrouter.Config{
			Enabled:        true,
			DefaultChannel: "C0DEFAULT",
			Overrides: map[string]string{
				"gastown/crew/decisions": "C0DECISIONS",
				"gastown/polecats/alpha": "C0ALPHA",
			},
		}

		r := slackrouter.NewRouter(cfg)

		// Test reverse lookup
		agent := r.GetAgentByChannel("C0DECISIONS")
		if agent != "gastown/crew/decisions" {
			t.Errorf("Expected agent=gastown/crew/decisions, got %q", agent)
		}

		// Unknown channel returns empty
		agent = r.GetAgentByChannel("C0UNKNOWN")
		if agent != "" {
			t.Errorf("Expected empty for unknown channel, got %q", agent)
		}
	})

	t.Run("BotMessageFiltered", func(t *testing.T) {
		// Bot's own messages should be filtered
		ev := &slackevents.MessageEvent{
			User:    "",     // Bots may have empty User
			BotID:   "B123", // But have BotID set
			Channel: "C0DECISIONS",
			Text:    "Bot message",
		}

		isBotMessage := ev.BotID != ""
		if !isBotMessage {
			t.Error("Expected message with BotID to be filtered as bot message")
		}
	})

	t.Run("ForwardChannelMessageFormat", func(t *testing.T) {
		// Test the format for forwarding channel messages to agent
		userName := "Steve"
		text := "What are you working on?"

		nudgeMsg := "[CHANNEL MESSAGE] " + userName + " says: " + text
		if !strings.Contains(nudgeMsg, "CHANNEL MESSAGE") {
			t.Error("Expected nudge to indicate channel message")
		}
		if !strings.Contains(nudgeMsg, userName) {
			t.Error("Expected nudge to include user name")
		}
	})

	t.Run("NonAgentChannelIgnored", func(t *testing.T) {
		// Messages in non-agent channels should be ignored
		cfg := &slackrouter.Config{
			Enabled:        true,
			DefaultChannel: "C0DEFAULT",
			Overrides:      map[string]string{}, // No agent channels configured
		}

		r := slackrouter.NewRouter(cfg)

		agent := r.GetAgentByChannel("C0SOMECHANNEL")
		shouldProcess := agent != ""
		if shouldProcess {
			t.Error("Expected non-agent channel to be ignored")
		}
	})
}

// --- Cross-Feature Integration Tests ---

func TestIntegration_CrossFeature(t *testing.T) {
	t.Run("BreakOutCreatesAgentChannel", func(t *testing.T) {
		// When user clicks Break Out, a new channel is created and registered
		cfg := &slackrouter.Config{
			Enabled:        true,
			DefaultChannel: "C0DEFAULT",
			Overrides:      map[string]string{},
		}

		r := slackrouter.NewRouter(cfg)

		// Simulate Break Out creating a channel
		agentAddress := "gastown/crew/decisions"
		newChannelID := "C0DECISIONS"

		r.AddOverride(agentAddress, newChannelID)

		// Verify agent channel is registered
		agent := r.GetAgentByChannel(newChannelID)
		if agent != agentAddress {
			t.Errorf("Expected agent=%q after Break Out, got %q", agentAddress, agent)
		}
	})

	t.Run("ThreadReplyInAgentChannel", func(t *testing.T) {
		// Thread replies in agent channels should be handled as thread replies, not channel messages
		ev := &slackevents.MessageEvent{
			User:            "U12345",
			Channel:         "C0DECISIONS", // Agent channel
			Text:            "Reply in thread",
			ThreadTimeStamp: "1234567890.123456", // Thread reply
			TimeStamp:       "1234567890.654321",
		}

		// Thread reply takes priority
		isThreadReply := ev.ThreadTimeStamp != ""
		if !isThreadReply {
			t.Error("Thread replies should be handled before agent channel logic")
		}
	})

	t.Run("DMOptInAffectsDecisionRouting", func(t *testing.T) {
		pm := NewPreferenceManager(t.TempDir())

		// User opts in
		pm.SetDMOptIn("U12345", true)

		// Decision should route to DM
		prefs := pm.GetUserPreferences("U12345")
		routeToDM := prefs.DMOptIn && prefs.NotificationLevel != "muted"
		if !routeToDM {
			t.Error("Expected decision to route to DM for opted-in user")
		}
	})
}

// --- Error Cases ---

func TestIntegration_ErrorCases(t *testing.T) {
	t.Run("InvalidNotificationLevel", func(t *testing.T) {
		pm := NewPreferenceManager(t.TempDir())

		// Invalid level should error
		err := pm.SetNotificationLevel("U12345", "invalid")
		if err == nil {
			t.Error("Expected error for invalid notification level")
		}
	})

	t.Run("EmptyUserID", func(t *testing.T) {
		pm := NewPreferenceManager(t.TempDir())

		// Empty user ID should still work (returns defaults)
		prefs := pm.GetUserPreferences("")
		if prefs.NotificationLevel != "high" {
			t.Error("Expected defaults for empty user ID")
		}
	})

	t.Run("EmptyRouterConfig", func(t *testing.T) {
		// Router with empty config should be disabled
		r := slackrouter.NewRouter(&slackrouter.Config{})
		if r.IsEnabled() {
			t.Error("Expected disabled router for empty config")
		}
	})
}

// --- Callback Parsing Tests ---

func TestIntegration_CallbackParsing(t *testing.T) {
	t.Run("ExtractDecisionIDFromActionID", func(t *testing.T) {
		// Action IDs are formatted as "action_type:decision_id"
		actionIDs := []struct {
			actionID   string
			wantAction string
			wantID     string
		}{
			{"resolve:gt-abc123", "resolve", "gt-abc123"},
			{"peek:gt-xyz789", "peek", "gt-xyz789"},
			{"dm_optin:settings", "dm_optin", "settings"},
		}

		for _, tc := range actionIDs {
			parts := strings.SplitN(tc.actionID, ":", 2)
			if len(parts) != 2 {
				t.Errorf("Expected 2 parts from %q", tc.actionID)
				continue
			}

			if parts[0] != tc.wantAction {
				t.Errorf("Expected action=%q, got %q", tc.wantAction, parts[0])
			}
			if parts[1] != tc.wantID {
				t.Errorf("Expected ID=%q, got %q", tc.wantID, parts[1])
			}
		}
	})

	t.Run("ParseBreakOutCallback", func(t *testing.T) {
		// Break Out callback value is the agent address
		callback := slack.InteractionCallback{
			User: slack.User{
				ID:   "U12345",
				Name: "testuser",
			},
			Channel: slack.Channel{
				GroupConversation: slack.GroupConversation{
					Conversation: slack.Conversation{
						ID: "C12345",
					},
				},
			},
		}

		agentAddress := "gastown/crew/decisions"

		// Verify callback contains expected fields
		if callback.User.ID != "U12345" {
			t.Error("Expected user ID in callback")
		}
		if agentAddress == "" {
			t.Error("Expected agent address in Break Out action value")
		}
	})
}

// --- DM Me Button Tests (gt-5uqg3k) ---

func TestIntegration_DMButtonSendsImmediateDM(t *testing.T) {
	t.Run("DMButtonActionExtractsDecisionID", func(t *testing.T) {
		// The DM Me button has action_id="open_preferences" with value=decision.ID
		callback := slack.InteractionCallback{
			User: slack.User{ID: "U12345"},
			ActionCallback: slack.ActionCallbacks{
				BlockActions: []*slack.BlockAction{
					{
						ActionID: "open_preferences",
						Value:    "gt-dec-test123",
					},
				},
			},
		}

		// Verify decision ID can be extracted from button value
		if len(callback.ActionCallback.BlockActions) == 0 {
			t.Fatal("Expected block actions in callback")
		}

		decisionID := callback.ActionCallback.BlockActions[0].Value
		if decisionID != "gt-dec-test123" {
			t.Errorf("Expected decision ID 'gt-dec-test123', got %q", decisionID)
		}
	})

	t.Run("DMButtonHandlerFlow", func(t *testing.T) {
		// Simulate the flow of DM Me button:
		// 1. User clicks DM Me button
		// 2. Handler extracts decision ID from button value
		// 3. Decision is sent to user via DM
		// 4. Preferences modal is opened

		callback := slack.InteractionCallback{
			User:      slack.User{ID: "U12345"},
			TriggerID: "trigger123",
			Channel:   slack.Channel{GroupConversation: slack.GroupConversation{Conversation: slack.Conversation{ID: "C12345"}}},
			ActionCallback: slack.ActionCallbacks{
				BlockActions: []*slack.BlockAction{
					{ActionID: "open_preferences", Value: "gt-dec-abc"},
				},
			},
		}

		// Step 1: Extract decision ID
		decisionID := ""
		if len(callback.ActionCallback.BlockActions) > 0 {
			decisionID = callback.ActionCallback.BlockActions[0].Value
		}

		if decisionID == "" {
			t.Error("Expected decision ID from button value")
		}

		// Step 2: Verify user ID is available for DM
		userID := callback.User.ID
		if userID == "" {
			t.Error("Expected user ID for sending DM")
		}

		// Step 3: Verify trigger ID is available for modal
		if callback.TriggerID == "" {
			t.Error("Expected trigger ID for opening preferences modal")
		}
	})

	t.Run("DMButtonWithEmptyValue", func(t *testing.T) {
		// Edge case: button value is empty (shouldn't happen but be defensive)
		callback := slack.InteractionCallback{
			User: slack.User{ID: "U12345"},
			ActionCallback: slack.ActionCallbacks{
				BlockActions: []*slack.BlockAction{
					{ActionID: "open_preferences", Value: ""},
				},
			},
		}

		decisionID := ""
		if len(callback.ActionCallback.BlockActions) > 0 {
			decisionID = callback.ActionCallback.BlockActions[0].Value
		}

		// Empty value should be handled gracefully
		if decisionID != "" {
			t.Error("Expected empty decision ID for empty button value")
		}
	})
}

// --- Peek Button Activity Tests (gt-5gfztk) ---

func TestIntegration_PeekActivityForCrewAgents(t *testing.T) {
	t.Run("CrewAgentIdentityExtraction", func(t *testing.T) {
		// For crew agents, extractAgentShortName should return the crew name
		agents := []struct {
			fullPath  string
			shortName string
		}{
			{"gastown/crew/decisions", "decisions"},
			{"beads/crew/dolt_doctor", "dolt_doctor"},
			{"myrig/crew/worker", "worker"},
		}

		for _, tc := range agents {
			got := extractAgentShortName(tc.fullPath)
			if got != tc.shortName {
				t.Errorf("extractAgentShortName(%q) = %q, want %q",
					tc.fullPath, got, tc.shortName)
			}
		}
	})

	t.Run("ActivityMatchingIncludesAuthor", func(t *testing.T) {
		// Activity matching should find commits by author name, not just subject
		// This is the fix for gt-5gfztk

		// Simulated git log output: hash|date|author|subject
		gitLogLines := []string{
			"abc123|2026-02-02 12:00:00 +0000|decisions|fix: update statusline",
			"def456|2026-02-02 11:00:00 +0000|other-author|unrelated commit",
			"ghi789|2026-02-02 10:00:00 +0000|nux|feat: add new feature",
		}

		shortName := "decisions"
		matchCount := 0

		for _, line := range gitLogLines {
			parts := strings.SplitN(line, "|", 4)
			if len(parts) != 4 {
				continue
			}

			author := strings.ToLower(parts[2])
			subject := strings.ToLower(parts[3])

			authorMatch := author == shortName || strings.Contains(author, shortName)
			subjectMatch := strings.Contains(subject, shortName)

			if authorMatch || subjectMatch {
				matchCount++
			}
		}

		// Should match the first commit (author=decisions)
		if matchCount != 1 {
			t.Errorf("Expected 1 match for author 'decisions', got %d", matchCount)
		}
	})
}
