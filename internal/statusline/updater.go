package statusline

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/mail"
	"github.com/steveyegge/gastown/internal/terminal"
)

// Updater collects status line data for all known identities.
type Updater struct {
	townRoot string
	backend  terminal.Backend
}

// NewUpdater creates a new status line data updater.
func NewUpdater(townRoot string) *Updater {
	return &Updater{
		townRoot: townRoot,
		backend:  terminal.NewCoopBackend(terminal.CoopConfig{}),
	}
}

// Update populates the cache with fresh data.
// This is the function that gets called periodically by the CacheManager.
func (u *Updater) Update(cache *Cache) error {
	// Reset cache
	if cache.Identities == nil {
		cache.Identities = make(map[string]*IdentityData)
	}
	if cache.Rigs == nil {
		cache.Rigs = make(map[string]*RigStatus)
	}
	if cache.AgentHealth == nil {
		cache.AgentHealth = make(map[string]*AgentHealth)
	}

	// Get registered rigs
	rigs := u.getRegisteredRigs()

	// Update town-level identities (Mayor, Deacon)
	u.updateTownIdentities(cache)

	// Update rig-level identities (Witnesses, Refineries, Crew, Polecats)
	for rigName := range rigs {
		u.updateRigIdentities(cache, rigName)
	}

	// Update tmux session health (for Mayor status line)
	u.updateSessionHealth(cache, rigs)

	return nil
}

// getRegisteredRigs returns a map of registered rig names.
func (u *Updater) getRegisteredRigs() map[string]bool {
	rigsPath := filepath.Join(u.townRoot, "mayor", "rigs.json")
	rigsConfig, err := config.LoadRigsConfig(rigsPath)
	if err != nil {
		return nil
	}

	rigs := make(map[string]bool)
	for name := range rigsConfig.Rigs {
		rigs[name] = true
	}
	return rigs
}

// updateTownIdentities updates Mayor and Deacon status data.
// Uses canonical identity format with trailing slash (per AddressToIdentity).
func (u *Updater) updateTownIdentities(cache *Cache) {
	// Mayor identity - use "mayor/" canonical format for consistent cache keys
	mayorData := &IdentityData{}
	u.populateHookedWork(mayorData, "mayor/", u.townRoot)
	u.populateCurrentWork(mayorData, "mayor/", u.townRoot)
	u.populateMail(mayorData, "mayor/", u.townRoot)
	cache.SetIdentity("mayor/", mayorData)

	// Deacon identity - use "deacon/" canonical format for consistent cache keys
	deaconData := &IdentityData{}
	u.populateHookedWork(deaconData, "deacon/", u.townRoot)
	u.populateCurrentWork(deaconData, "deacon/", u.townRoot)
	u.populateMail(deaconData, "deacon/", u.townRoot)
	cache.SetIdentity("deacon/", deaconData)
}

// updateRigIdentities updates identities for a specific rig.
func (u *Updater) updateRigIdentities(cache *Cache, rigName string) {
	rigPath := filepath.Join(u.townRoot, rigName)
	rigBeadsDir := filepath.Join(rigPath, "mayor", "rig")

	// Witness identity
	witnessIdentity := fmt.Sprintf("%s/witness", rigName)
	witnessData := &IdentityData{}
	u.populateHookedWork(witnessData, witnessIdentity, rigBeadsDir)
	u.populateCurrentWork(witnessData, witnessIdentity, rigBeadsDir)
	u.populateMail(witnessData, witnessIdentity, u.townRoot)
	cache.SetIdentity(witnessIdentity, witnessData)

	// Refinery identity
	refineryIdentity := fmt.Sprintf("%s/refinery", rigName)
	refineryData := &IdentityData{}
	u.populateHookedWork(refineryData, refineryIdentity, rigBeadsDir)
	u.populateCurrentWork(refineryData, refineryIdentity, rigBeadsDir)
	u.populateMail(refineryData, refineryIdentity, u.townRoot)
	cache.SetIdentity(refineryIdentity, refineryData)

	// Polecats - scan the polecats directory
	polecatsDir := filepath.Join(rigPath, "polecats")
	u.updatePolecatIdentities(cache, rigName, polecatsDir, rigBeadsDir)

	// Crew members - scan crew tmux sessions
	u.updateCrewIdentities(cache, rigName, rigBeadsDir)
}

// updatePolecatIdentities updates identities for all polecats in a rig.
func (u *Updater) updatePolecatIdentities(cache *Cache, rigName, polecatsDir, rigBeadsDir string) {
	entries, err := os.ReadDir(polecatsDir)
	if err != nil {
		return // No polecats directory
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		polecatName := entry.Name()
		identity := fmt.Sprintf("%s/%s", rigName, polecatName)

		data := &IdentityData{}
		u.populateHookedWork(data, identity, rigBeadsDir)
		u.populateCurrentWork(data, identity, rigBeadsDir)
		u.populateMail(data, identity, u.townRoot)
		cache.SetIdentity(identity, data)
	}
}

// updateCrewIdentities updates identities for crew members by scanning crew directories.
func (u *Updater) updateCrewIdentities(cache *Cache, rigName, rigBeadsDir string) {
	// Scan crew directory for crew members instead of tmux sessions
	crewDir := filepath.Join(u.townRoot, rigName, "crew")
	entries, err := os.ReadDir(crewDir)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		crewName := entry.Name()
		identity := fmt.Sprintf("%s/crew/%s", rigName, crewName)

		data := &IdentityData{}
		u.populateHookedWork(data, identity, rigBeadsDir)
		u.populateCurrentWork(data, identity, rigBeadsDir)
		u.populateMail(data, identity, u.townRoot)
		cache.SetIdentity(identity, data)
	}
}

// populateHookedWork fills in hooked work data for an identity.
func (u *Updater) populateHookedWork(data *IdentityData, identity string, beadsDir string) {
	if beadsDir == "" {
		beadsDir = u.townRoot
	}

	// Check if beads directory exists
	if _, err := os.Stat(filepath.Join(beadsDir, ".beads")); os.IsNotExist(err) {
		// Try the parent directory (for rig beads)
		if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
			return
		}
	}

	b := beads.New(beadsDir)

	// Query for hooked beads assigned to this identity
	hookedBeads, err := b.List(beads.ListOptions{
		Status:   beads.StatusHooked,
		Assignee: identity,
		Priority: -1,
	})
	if err != nil || len(hookedBeads) == 0 {
		return
	}

	// Format the first hooked bead
	bead := hookedBeads[0]
	const maxLen = 40
	display := fmt.Sprintf("%s: %s", bead.ID, bead.Title)
	if len(display) > maxLen {
		display = display[:maxLen-1] + "\u2026"
	}
	data.HookedWork = display
}

// populateCurrentWork fills in current work (in_progress) data for an identity (gt-avr97i.1).
func (u *Updater) populateCurrentWork(data *IdentityData, identity string, beadsDir string) {
	if beadsDir == "" {
		beadsDir = u.townRoot
	}

	// Check if beads directory exists
	if _, err := os.Stat(filepath.Join(beadsDir, ".beads")); os.IsNotExist(err) {
		// Try the parent directory (for rig beads)
		if _, err := os.Stat(beadsDir); os.IsNotExist(err) {
			return
		}
	}

	b := beads.New(beadsDir)

	// Query for in_progress beads assigned to this identity
	inProgressBeads, err := b.List(beads.ListOptions{
		Status:   "in_progress",
		Assignee: identity,
		Priority: -1,
	})
	if err != nil || len(inProgressBeads) == 0 {
		return
	}

	// Format the first in_progress bead
	bead := inProgressBeads[0]
	const maxLen = 40
	display := fmt.Sprintf("%s: %s", bead.ID, bead.Title)
	if len(display) > maxLen {
		display = display[:maxLen-1] + "\u2026"
	}
	data.CurrentWork = display
}

// populateMail fills in mail data for an identity.
func (u *Updater) populateMail(data *IdentityData, identity string, townRoot string) {
	mailbox := mail.NewMailboxFromAddress(identity, townRoot)

	messages, err := mailbox.ListUnread()
	if err != nil || len(messages) == 0 {
		return
	}

	data.MailUnread = len(messages)

	// Get first message subject, truncated
	const maxLen = 45
	subject := messages[0].Subject
	if len(subject) > maxLen {
		subject = subject[:maxLen-1] + "\u2026"
	}
	data.MailSubject = subject
}

// updateSessionHealth updates session health data for the Mayor status line.
func (u *Updater) updateSessionHealth(cache *Cache, rigs map[string]bool) {
	// Track per-rig status
	for rigName := range rigs {
		cache.Rigs[rigName] = &RigStatus{
			OpState: "OPERATIONAL",
		}
	}

	// Track agent health by checking coop sessions
	witnessHealth := &AgentHealth{}
	refineryHealth := &AgentHealth{}

	// Check deacon session
	hasDeacon := false
	if running, _ := u.backend.HasSession("hq-deacon"); running {
		hasDeacon = true
	}

	// Check rig-level agent sessions
	for rigName := range rigs {
		status := cache.Rigs[rigName]

		// Check witness
		witnessSession := fmt.Sprintf("gt-%s-witness", rigName)
		if running, _ := u.backend.HasSession(witnessSession); running {
			status.HasWitness = true
			witnessHealth.Total++
			if u.isSessionWorking(witnessSession) {
				witnessHealth.Working++
			}
		}

		// Check refinery
		refinerySession := fmt.Sprintf("gt-%s-refinery", rigName)
		if running, _ := u.backend.HasSession(refinerySession); running {
			status.HasRefinery = true
			refineryHealth.Total++
			if u.isSessionWorking(refinerySession) {
				refineryHealth.Working++
			}
		}
	}

	cache.HasDeacon = hasDeacon
	cache.AgentHealth["witness"] = witnessHealth
	cache.AgentHealth["refinery"] = refineryHealth
	cache.RigCount = len(rigs)
}

// isSessionWorking checks if a Claude session is actively working.
func (u *Updater) isSessionWorking(session string) bool {
	lines, err := u.backend.CapturePaneLines(session, 10)
	if err != nil || len(lines) == 0 {
		return false
	}

	for _, line := range lines {
		if strings.Contains(line, "\u2736") || strings.Contains(line, "\u273B") {
			return true
		}
	}

	return false
}
