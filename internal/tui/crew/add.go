package crew

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/steveyegge/gastown/internal/beads"
	"github.com/steveyegge/gastown/internal/config"
	"github.com/steveyegge/gastown/internal/crew"
	"github.com/steveyegge/gastown/internal/git"
	"github.com/steveyegge/gastown/internal/rig"
)

// WizardStep represents the current step in the wizard
type WizardStep int

const (
	StepRig WizardStep = iota
	StepNewRig // Sub-step for creating a new rig
	StepName
	StepOptions
	StepCreating
	StepSuccess
	StepError
)

// Special index for "Add new rig" option
const addNewRigIndex = -1

// AddModel is the bubbletea model for the crew add wizard
type AddModel struct {
	// Wizard state
	step WizardStep
	done bool // Set when wizard should close (user acknowledged success/error)

	// Input fields
	nameInput    textinput.Model
	selectedRig  int
	rigs         []string
	rigPaths     map[string]string
	createBranch bool

	// New rig creation fields
	newRigNameInput textinput.Model
	newRigURLInput  textinput.Model
	newRigFocused   int // 0 = name, 1 = url

	// Creation progress
	creationSteps []creationStep
	currentStep   int
	spinner       spinner.Model

	// Result
	result    *crew.CrewWorker
	agentBead string
	err       error

	// Context
	townRoot   string
	currentRig string

	// UI dimensions
	width  int
	height int
}

// creationStep tracks progress during workspace creation
type creationStep struct {
	name   string
	done   bool
	active bool
}

// NewAddModel creates a new crew add wizard model
func NewAddModel(townRoot, currentRig string) *AddModel {
	ti := textinput.New()
	ti.Placeholder = "crew_name"
	// Don't focus yet - we start on rig selection
	ti.CharLimit = 64
	ti.Width = 40

	// New rig name input
	rigNameInput := textinput.New()
	rigNameInput.Placeholder = "rig_name"
	rigNameInput.CharLimit = 32
	rigNameInput.Width = 40

	// New rig URL input
	rigURLInput := textinput.New()
	rigURLInput.Placeholder = "https://github.com/org/repo.git"
	rigURLInput.CharLimit = 256
	rigURLInput.Width = 60

	s := spinner.New()
	s.Spinner = spinner.Dot

	return &AddModel{
		step:            StepRig, // Start with rig selection
		nameInput:       ti,
		newRigNameInput: rigNameInput,
		newRigURLInput:  rigURLInput,
		spinner:         s,
		townRoot:        townRoot,
		currentRig:      currentRig,
		rigPaths:        make(map[string]string),
		creationSteps: []creationStep{
			{name: "Cloning repository"},
			{name: "Setting up mail directory"},
			{name: "Configuring shared beads"},
			{name: "Setting up Claude hooks"},
			{name: "Creating agent bead"},
		},
	}
}

// SetSize sets the terminal dimensions for the wizard
func (m *AddModel) SetSize(width, height int) {
	m.width = width
	m.height = height
	m.nameInput.Width = min(40, width-10)
	m.newRigNameInput.Width = min(40, width-10)
	m.newRigURLInput.Width = min(60, width-10)
}

// Init initializes the model
func (m *AddModel) Init() tea.Cmd {
	// Only load rigs at start - we'll focus the text input when we get to the name step
	return m.loadRigs()
}

// rigsLoadedMsg is sent when rigs are loaded
type rigsLoadedMsg struct {
	rigs     []string
	rigPaths map[string]string
	err      error
}

// crewCreatedMsg is sent when crew creation completes
type crewCreatedMsg struct {
	worker    *crew.CrewWorker
	agentBead string
	err       error
}

// creationProgressMsg updates creation progress
type creationProgressMsg struct {
	step int
}

// rigCreatedMsg is sent when new rig creation completes
type rigCreatedMsg struct {
	rigName string
	err     error
}

// loadRigs loads available rigs from config
func (m *AddModel) loadRigs() tea.Cmd {
	return func() tea.Msg {
		rigsConfigPath := filepath.Join(m.townRoot, "mayor", "rigs.json")
		rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
		if err != nil {
			// Return just the current rig if we can't load config
			rigPaths := map[string]string{m.currentRig: filepath.Join(m.townRoot, m.currentRig)}
			return rigsLoadedMsg{
				rigs:     []string{m.currentRig},
				rigPaths: rigPaths,
			}
		}

		var rigs []string
		rigPaths := make(map[string]string)

		// Put current rig first
		if _, ok := rigsConfig.Rigs[m.currentRig]; ok {
			rigs = append(rigs, m.currentRig)
			rigPaths[m.currentRig] = filepath.Join(m.townRoot, m.currentRig)
		}

		// Add other rigs
		for name := range rigsConfig.Rigs {
			if name != m.currentRig {
				rigs = append(rigs, name)
				rigPaths[name] = filepath.Join(m.townRoot, name)
			}
		}

		return rigsLoadedMsg{rigs: rigs, rigPaths: rigPaths}
	}
}

// createNewRig creates a new rig
func (m *AddModel) createNewRig() tea.Cmd {
	return func() tea.Msg {
		rigName := m.newRigNameInput.Value()
		gitURL := m.newRigURLInput.Value()

		// Load rigs config
		rigsConfigPath := filepath.Join(m.townRoot, "mayor", "rigs.json")
		rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
		if err != nil {
			rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
		}

		// Create rig manager
		g := git.NewGit(m.townRoot)
		rigMgr := rig.NewManager(m.townRoot, rigsConfig, g)

		// Add the rig
		opts := rig.AddRigOptions{
			Name:   rigName,
			GitURL: gitURL,
		}
		_, err = rigMgr.AddRig(opts)
		if err != nil {
			return rigCreatedMsg{err: err}
		}

		return rigCreatedMsg{rigName: rigName}
	}
}

// createCrew creates the crew workspace
func (m *AddModel) createCrew() tea.Cmd {
	return func() tea.Msg {
		rigName := m.rigs[m.selectedRig]
		rigPath := m.rigPaths[rigName]
		crewName := m.nameInput.Value()

		// Load rig config
		rigsConfigPath := filepath.Join(m.townRoot, "mayor", "rigs.json")
		rigsConfig, err := config.LoadRigsConfig(rigsConfigPath)
		if err != nil {
			rigsConfig = &config.RigsConfig{Rigs: make(map[string]config.RigEntry)}
		}

		// Get rig
		g := git.NewGit(m.townRoot)
		rigMgr := rig.NewManager(m.townRoot, rigsConfig, g)
		r, err := rigMgr.GetRig(rigName)
		if err != nil {
			return crewCreatedMsg{err: fmt.Errorf("rig '%s' not found: %w", rigName, err)}
		}

		// Create crew manager
		crewGit := git.NewGit(r.Path)
		crewMgr := crew.NewManager(r, crewGit)

		// Create workspace
		worker, err := crewMgr.Add(crewName, m.createBranch)
		if err != nil {
			return crewCreatedMsg{err: err}
		}

		// Create agent bead
		var agentBead string
		bd := beads.New(beads.ResolveBeadsDir(rigPath))
		prefix := beads.GetPrefixForRig(m.townRoot, rigName)
		crewID := beads.CrewBeadIDWithPrefix(prefix, rigName, crewName)
		if _, err := bd.Show(crewID); err != nil {
			// Agent bead doesn't exist, create it
			fields := &beads.AgentFields{
				RoleType:   "crew",
				Rig:        rigName,
				AgentState: "idle",
			}
			desc := fmt.Sprintf("Crew worker %s in %s - human-managed persistent workspace.", crewName, rigName)
			if _, err := bd.CreateAgentBead(crewID, desc, fields); err == nil {
				agentBead = crewID
			}
		}

		return crewCreatedMsg{worker: worker, agentBead: agentBead}
	}
}

// Update handles messages
func (m *AddModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.nameInput.Width = min(40, msg.Width-10)

	case tea.KeyMsg:
		switch m.step {
		case StepName:
			return m.handleNameStep(msg)
		case StepRig:
			return m.handleRigStep(msg)
		case StepNewRig:
			return m.handleNewRigStep(msg)
		case StepOptions:
			return m.handleOptionsStep(msg)
		case StepSuccess, StepError:
			// Any key signals done - parent should check IsDone()
			if msg.Type == tea.KeyEnter || msg.Type == tea.KeyEsc || msg.String() == "q" {
				m.done = true
				return m, nil
			}
		}

	case rigsLoadedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = StepError
		} else {
			m.rigs = msg.rigs
			m.rigPaths = msg.rigPaths
		}

	case crewCreatedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = StepError
		} else {
			m.result = msg.worker
			m.agentBead = msg.agentBead
			m.step = StepSuccess
		}

	case rigCreatedMsg:
		if msg.err != nil {
			m.err = msg.err
			m.step = StepError
		} else {
			// Rig created successfully - reload rigs and select the new one
			m.currentRig = msg.rigName // New rig becomes current
			m.step = StepRig
			// Clear the new rig inputs
			m.newRigNameInput.Reset()
			m.newRigURLInput.Reset()
			return m, m.loadRigs()
		}

	case spinner.TickMsg:
		if m.step == StepCreating {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

// handleNameStep handles input on the name step
func (m *AddModel) handleNameStep(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.step = StepRig // Go back to rig selection
		m.nameInput.Blur()
		return m, nil
	case tea.KeyEnter:
		// Validate name
		name := m.nameInput.Value()
		if err := m.validateName(name); err != nil {
			// Don't advance, error is shown in view
			return m, nil
		}
		m.step = StepOptions
		m.nameInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.nameInput, cmd = m.nameInput.Update(msg)
	return m, cmd
}

// handleRigStep handles input on the rig selection step
func (m *AddModel) handleRigStep(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Total options = rigs + "Add new rig"
	totalOptions := len(m.rigs) + 1
	addNewRigIdx := len(m.rigs) // Last option is "Add new rig"

	switch msg.Type {
	case tea.KeyEsc:
		m.done = true // Cancel wizard - this is the first step
		return m, nil
	case tea.KeyUp, tea.KeyShiftTab:
		if m.selectedRig > 0 {
			m.selectedRig--
		}
	case tea.KeyDown, tea.KeyTab:
		if m.selectedRig < totalOptions-1 {
			m.selectedRig++
		}
	case tea.KeyEnter:
		if m.selectedRig == addNewRigIdx {
			// Go to new rig creation step
			m.step = StepNewRig
			m.newRigNameInput.Focus()
			return m, textinput.Blink
		}
		m.step = StepName
		m.nameInput.Focus()
		return m, textinput.Blink
	}

	// Handle j/k for vim-style navigation
	switch msg.String() {
	case "j":
		if m.selectedRig < totalOptions-1 {
			m.selectedRig++
		}
	case "k":
		if m.selectedRig > 0 {
			m.selectedRig--
		}
	}

	return m, nil
}

// handleNewRigStep handles input on the new rig creation step
func (m *AddModel) handleNewRigStep(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		// Go back to rig selection
		m.step = StepRig
		m.newRigNameInput.Blur()
		m.newRigURLInput.Blur()
		return m, nil
	case tea.KeyTab, tea.KeyShiftTab:
		// Toggle between name and URL inputs
		if m.newRigFocused == 0 {
			m.newRigFocused = 1
			m.newRigNameInput.Blur()
			m.newRigURLInput.Focus()
		} else {
			m.newRigFocused = 0
			m.newRigURLInput.Blur()
			m.newRigNameInput.Focus()
		}
		return m, textinput.Blink
	case tea.KeyEnter:
		// Validate and create
		name := m.newRigNameInput.Value()
		url := m.newRigURLInput.Value()
		if name == "" || url == "" {
			return m, nil // Don't proceed if fields are empty
		}
		// Validate name (same rules as crew name)
		if err := m.validateRigName(name); err != nil {
			return m, nil
		}
		// Start rig creation
		m.step = StepCreating
		return m, tea.Batch(m.spinner.Tick, m.createNewRig())
	}

	// Forward input to focused field
	var cmd tea.Cmd
	if m.newRigFocused == 0 {
		m.newRigNameInput, cmd = m.newRigNameInput.Update(msg)
	} else {
		m.newRigURLInput, cmd = m.newRigURLInput.Update(msg)
	}
	return m, cmd
}

// validateRigName checks if a rig name is valid
func (m *AddModel) validateRigName(name string) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if strings.ContainsAny(name, "-. ") {
		sanitized := strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(name)
		sanitized = strings.ToLower(sanitized)
		return fmt.Errorf("invalid characters (-, ., space). Try %q instead", sanitized)
	}
	return nil
}

// handleOptionsStep handles input on the options step
func (m *AddModel) handleOptionsStep(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.step = StepName
		m.nameInput.Focus()
		return m, textinput.Blink
	case tea.KeySpace:
		m.createBranch = !m.createBranch
	case tea.KeyEnter:
		m.step = StepCreating
		return m, tea.Batch(m.spinner.Tick, m.createCrew())
	}
	return m, nil
}

// validateName checks if a crew name is valid
func (m *AddModel) validateName(name string) error {
	if name == "" {
		return fmt.Errorf("name cannot be empty")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("%q is not allowed", name)
	}
	if strings.ContainsAny(name, "/\\") {
		return fmt.Errorf("name cannot contain path separators")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("name cannot contain path traversal sequence")
	}
	if strings.ContainsAny(name, "-. ") {
		sanitized := strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(name)
		sanitized = strings.ToLower(sanitized)
		return fmt.Errorf("invalid characters (-, ., space). Try %q instead", sanitized)
	}
	return nil
}

// suggestName returns a sanitized version of an invalid name
func (m *AddModel) suggestName(name string) string {
	if name == "" {
		return ""
	}
	sanitized := strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(name)
	return strings.ToLower(sanitized)
}

// View renders the wizard
func (m *AddModel) View() string {
	if m.width < 40 || m.height < 10 {
		return "Terminal too small. Please resize."
	}

	var b strings.Builder

	// Title
	b.WriteString(titleStyle.Render("Create Crew Member"))
	b.WriteString("\n")

	// Step indicator
	b.WriteString(m.renderStepIndicator())
	b.WriteString("\n\n")

	// Current step content
	switch m.step {
	case StepName:
		b.WriteString(m.renderNameStep())
	case StepRig:
		b.WriteString(m.renderRigStep())
	case StepNewRig:
		b.WriteString(m.renderNewRigStep())
	case StepOptions:
		b.WriteString(m.renderOptionsStep())
	case StepCreating:
		b.WriteString(m.renderCreatingStep())
	case StepSuccess:
		b.WriteString(m.renderSuccessStep())
	case StepError:
		b.WriteString(m.renderErrorStep())
	}

	// Help text
	b.WriteString("\n\n")
	b.WriteString(m.renderHelp())

	return b.String()
}

// renderStepIndicator renders the step progress indicator
func (m *AddModel) renderStepIndicator() string {
	// Map display steps to actual step constants
	// StepNewRig is a sub-step of Rig, so it shows as Rig being active
	displaySteps := []struct {
		name string
		step WizardStep
	}{
		{"Rig", StepRig},
		{"Name", StepName},
		{"Options", StepOptions},
		{"Create", StepCreating},
	}

	// Normalize current step for display purposes
	// StepNewRig counts as StepRig for the indicator
	currentStep := m.step
	if currentStep == StepNewRig {
		currentStep = StepRig
	}

	var parts []string
	for _, ds := range displaySteps {
		var style lipgloss.Style

		if ds.step < currentStep || m.step == StepSuccess {
			style = stepCompleteStyle
			parts = append(parts, style.Render("("+ds.name+")"))
		} else if ds.step == currentStep {
			style = stepActiveStyle
			parts = append(parts, style.Render("["+ds.name+"]"))
		} else {
			style = stepInactiveStyle
			parts = append(parts, style.Render(" "+ds.name+" "))
		}
	}

	return strings.Join(parts, " > ")
}

// renderNameStep renders the name input step
func (m *AddModel) renderNameStep() string {
	var b strings.Builder

	b.WriteString(inputLabelStyle.Render("Crew member name:"))
	b.WriteString("\n\n")
	b.WriteString(m.nameInput.View())

	// Show validation error or hint
	name := m.nameInput.Value()
	if err := m.validateName(name); err != nil && name != "" {
		b.WriteString("\n")
		b.WriteString(inputErrorStyle.Render("  " + err.Error()))
	} else if name == "" {
		b.WriteString("\n")
		b.WriteString(inputHintStyle.Render("  Use lowercase letters and underscores (e.g., feature_dev)"))
	} else {
		b.WriteString("\n")
		b.WriteString(successStyle.Render("  Valid name"))
	}

	return b.String()
}

// renderRigStep renders the rig selection step
func (m *AddModel) renderRigStep() string {
	var b strings.Builder

	b.WriteString(inputLabelStyle.Render("Select target rig:"))
	b.WriteString("\n\n")

	// Render existing rigs
	for i, rigName := range m.rigs {
		cursor := "  "
		if i == m.selectedRig {
			cursor = "> "
		}

		var line string
		if i == m.selectedRig {
			line = radioSelectedStyle.Render(cursor + "(*) " + rigName)
		} else {
			line = radioUnselectedStyle.Render(cursor + "( ) " + rigName)
		}

		if rigName == m.currentRig {
			line += helpStyle.Render(" (current)")
		}

		b.WriteString(line)
		b.WriteString("\n")
	}

	// Render "Add new rig" option
	addNewRigIdx := len(m.rigs)
	cursor := "  "
	if m.selectedRig == addNewRigIdx {
		cursor = "> "
	}
	var addLine string
	if m.selectedRig == addNewRigIdx {
		addLine = radioSelectedStyle.Render(cursor + "[+] Add new rig")
	} else {
		addLine = radioUnselectedStyle.Render(cursor + "[+] Add new rig")
	}
	b.WriteString("\n") // Extra spacing before add option
	b.WriteString(addLine)
	b.WriteString("\n")

	return b.String()
}

// renderNewRigStep renders the new rig creation step
func (m *AddModel) renderNewRigStep() string {
	var b strings.Builder

	b.WriteString(inputLabelStyle.Render("Create new rig:"))
	b.WriteString("\n\n")

	// Rig name input
	nameLabel := "Rig name:"
	if m.newRigFocused == 0 {
		nameLabel = "> " + nameLabel
	} else {
		nameLabel = "  " + nameLabel
	}
	b.WriteString(inputLabelStyle.Render(nameLabel))
	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(m.newRigNameInput.View())
	b.WriteString("\n")

	// Validate rig name
	name := m.newRigNameInput.Value()
	if name != "" {
		if err := m.validateRigName(name); err != nil {
			b.WriteString(inputErrorStyle.Render("  " + err.Error()))
		} else {
			b.WriteString(successStyle.Render("  Valid name"))
		}
	} else {
		b.WriteString(inputHintStyle.Render("  Use lowercase letters and underscores"))
	}
	b.WriteString("\n\n")

	// Git URL input
	urlLabel := "Git URL:"
	if m.newRigFocused == 1 {
		urlLabel = "> " + urlLabel
	} else {
		urlLabel = "  " + urlLabel
	}
	b.WriteString(inputLabelStyle.Render(urlLabel))
	b.WriteString("\n")
	b.WriteString("  ")
	b.WriteString(m.newRigURLInput.View())
	b.WriteString("\n")

	url := m.newRigURLInput.Value()
	if url == "" {
		b.WriteString(inputHintStyle.Render("  e.g., https://github.com/org/repo.git"))
	}

	return b.String()
}

// renderOptionsStep renders the options step
func (m *AddModel) renderOptionsStep() string {
	var b strings.Builder

	b.WriteString(inputLabelStyle.Render("Options:"))
	b.WriteString("\n\n")

	// Create branch checkbox
	var checkbox string
	if m.createBranch {
		checkbox = checkboxCheckedStyle.Render("[x]")
	} else {
		checkbox = checkboxUncheckedStyle.Render("[ ]")
	}
	b.WriteString(fmt.Sprintf("  %s Create feature branch (crew/%s)\n", checkbox, m.nameInput.Value()))
	b.WriteString(helpStyle.Render("      Work on a separate branch instead of main"))

	b.WriteString("\n\n")

	// Summary
	b.WriteString(inputLabelStyle.Render("Summary:"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  Name: %s\n", m.nameInput.Value()))
	b.WriteString(fmt.Sprintf("  Rig:  %s\n", m.rigs[m.selectedRig]))
	if m.createBranch {
		b.WriteString(fmt.Sprintf("  Branch: crew/%s\n", m.nameInput.Value()))
	} else {
		b.WriteString("  Branch: main\n")
	}

	return b.String()
}

// renderCreatingStep renders the creation progress
func (m *AddModel) renderCreatingStep() string {
	var b strings.Builder

	b.WriteString(inputLabelStyle.Render("Creating crew workspace..."))
	b.WriteString("\n\n")

	b.WriteString(m.spinner.View())
	b.WriteString(" Working...\n\n")

	return b.String()
}

// renderSuccessStep renders the success state
func (m *AddModel) renderSuccessStep() string {
	var b strings.Builder

	b.WriteString(successStyle.Render("Crew member created successfully!"))
	b.WriteString("\n\n")

	if m.result != nil {
		b.WriteString(inputLabelStyle.Render("Details:"))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  Name:   %s\n", m.result.Name))
		b.WriteString(fmt.Sprintf("  Rig:    %s\n", m.result.Rig))
		b.WriteString(fmt.Sprintf("  Path:   %s\n", m.result.ClonePath))
		b.WriteString(fmt.Sprintf("  Branch: %s\n", m.result.Branch))
		if m.agentBead != "" {
			b.WriteString(fmt.Sprintf("  Agent:  %s\n", m.agentBead))
		}
	}

	b.WriteString("\n")
	b.WriteString(helpStyle.Render("Start working with:"))
	b.WriteString("\n")
	if m.result != nil {
		b.WriteString(fmt.Sprintf("  cd %s\n", m.result.ClonePath))
		b.WriteString(fmt.Sprintf("  gt crew start %s\n", m.result.Name))
	}

	return b.String()
}

// renderErrorStep renders the error state
func (m *AddModel) renderErrorStep() string {
	var b strings.Builder

	b.WriteString(errorStyle.Render("Failed to create crew member"))
	b.WriteString("\n\n")

	if m.err != nil {
		b.WriteString(errorStyle.Render(m.err.Error()))
	}

	return b.String()
}

// renderHelp renders contextual help
func (m *AddModel) renderHelp() string {
	switch m.step {
	case StepRig:
		return helpStyle.Render("j/k or arrows: select  Enter: continue  Esc: cancel")
	case StepNewRig:
		return helpStyle.Render("Tab: switch field  Enter: create rig  Esc: back")
	case StepName:
		return helpStyle.Render("Enter: continue  Esc: back")
	case StepOptions:
		return helpStyle.Render("Space: toggle  Enter: create  Esc: back")
	case StepCreating:
		return helpStyle.Render("Please wait...")
	case StepSuccess, StepError:
		return helpStyle.Render("Press any key to exit")
	}
	return ""
}

// IsDone returns true if the wizard has completed (user acknowledged success/error)
func (m *AddModel) IsDone() bool {
	return m.done
}
