# Narrator: The Chronicle Agent

The Narrator is Gas Town's narrative generation agent. It observes work events
across the town and transforms them into compelling narrative content—turning
the technical drama of autonomous agents into engaging prose.

## Overview

The Narrator watches `.events.jsonl` files across rigs, classifies events by
narrative significance, and generates chapter files in configurable styles.

```
Events (.events.jsonl)
        ↓
    NARRATOR
        ↓
    Chapters (narrative/*.md)
        ↓
    Index (narrative/index.md)
```

## Quick Start

```bash
# Start the narrator
gt narrator start

# Check status
gt narrator status

# Attach to the narrator session
gt narrator attach

# Configure the narrative style
gt narrator config --style=book
```

## CLI Commands

### gt narrator start

Start the Narrator tmux session. Creates a new detached tmux session and
launches Claude with the narrator role.

```bash
gt narrator start
gt narrator start --agent=sonnet  # Use specific agent alias
```

### gt narrator stop

Stop the Narrator session. Attempts graceful shutdown first, then kills the
tmux session.

```bash
gt narrator stop
```

### gt narrator attach

Attach to the running Narrator session. If the session isn't running, it will
be started automatically.

```bash
gt narrator attach
```

Detach with `Ctrl-B D`.

### gt narrator status

Check the Narrator session status. Shows whether it's running, when it started,
current configuration, and narrative count.

```bash
gt narrator status
```

Example output:
```
● Narrator session is running
  Status: detached
  Created: 2025-01-17 10:30:00

Configuration:
  Style: book
  Output: (default: narrative/)
  Narratives generated: 5
```

### gt narrator config

View or update the Narrator's configuration.

```bash
# View current config
gt narrator config

# Set narrative style
gt narrator config --style=book
gt narrator config --style=tv-script
gt narrator config --style=youtube-short

# Set output directory
gt narrator config --output-dir=./my-narrative

# Output as JSON
gt narrator config --json
```

### gt narrator restart

Restart the Narrator session. Stops the current session (if running) and starts
a fresh one.

```bash
gt narrator restart
```

## Narrative Styles

The Narrator supports three distinct output styles:

### book (default)

Novel-style prose chapters inspired by Neal Stephenson, William Gibson, and
Vernor Vinge. Technical precision meets literary ambition.

**Characteristics:**
- Third-person omniscient with occasional deep POV
- Technical terminology used precisely
- Metaphors from engineering and systems theory
- Agents rendered as characters with motivations

**Example:**
> The Witness found the polecat precisely where it shouldn't be—suspended in that
> peculiar limbo between acknowledgment and action. Three nudges sent. Three nudges
> ignored.

### tv-script

Television screenplay format inspired by Halt and Catch Fire and Mr. Robot.
Visual, immediate, and dramatic.

**Characteristics:**
- Present tense, active voice
- Dialogue carries exposition naturally
- Visual descriptions of abstract concepts
- Tension through pacing and interruption

**Example:**
```
                    WITNESS
          Check the nux worktree.

Witness's cursor moves across the screen. A list of files appears.

                    WITNESS (CONT'D)
          Uncommitted changes. Unpushed branch.
              (beat)
          It's been idle for twelve minutes.
```

### youtube-short

Short-form social media content inspired by Fireship. Punchy, informal,
meme-aware.

**Characteristics:**
- Second person or first person plural
- Sentence fragments are acceptable
- Tech slang and memes appropriate to context
- Self-aware about AI writing about AI

**Example:**
> 🔥 **A Polecat Got Stuck. Here's What Happened Next.** 🔥
>
> **Three nudges. Zero response. The Witness had a decision to make.**

## Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `style` | string | `book` | Narrative style: book, tv-script, youtube-short |
| `output_dir` | string | `narrative/` | Output directory for chapters |
| `event_types` | []string | (all) | Filter to specific event types |
| `rig_filter` | []string | (all) | Limit observation to specific rigs |

Configuration is stored in `narrator.json` in the town root.

## Event Classification

Events are classified by narrative significance:

| Significance | Event Types | Narrative Treatment |
|--------------|-------------|---------------------|
| **High** | sling, done, handoff, merged, spawn, kill, boot, halt | Full scene treatment |
| **Medium** | hook, unhook, mail, nudge, escalation, merge_started | Paragraph or mention |
| **Low** | session_start, session_end, patrol, polecat_checked | Atmosphere only |
| **None** | Audit-only events | Ignored |

## Output Structure

The Narrator writes to the `narrative/` directory (configurable):

```
narrative/
├── index.md          ← Table of contents (auto-updated)
├── chapter-001.md    ← First chapter
├── chapter-002.md    ← Second chapter
└── ...
```

Chapters are numbered sequentially with zero-padded three-digit numbers.

## Architecture

```
Town Root
├── narrator/           ← Narrator working directory
│   └── narrator.json   ← Narrator state
├── narrative/          ← Generated chapters
└── <rig>/
    └── .events.jsonl   ← Event sources
```

## API

### Manager

The `narrator.Manager` type handles narrator lifecycle:

```go
mgr := narrator.NewManager(townRoot)

// Start the narrator
mgr.Start(agentOverride)

// Stop the narrator
mgr.Stop()

// Check if running
running, err := mgr.IsRunning()

// Get status
state, err := mgr.Status()

// Get session name
name := mgr.SessionName()
```

### OutputWriter

The `narrator.OutputWriter` handles narrative file generation:

```go
writer := narrator.NewOutputWriter(outputDir)

// Ensure output directory exists
writer.EnsureDir()

// Write a chapter
writer.WriteChapter(1, "# Chapter 1\n\nContent...")

// Write index with chapter metadata
chapters := []narrator.Chapter{
    {Number: 1, Title: "The Beginning", Summary: "..."},
}
writer.WriteIndex(chapters)

// List existing chapters
nums, err := writer.ListChapters()
```

### EventReader

The `narrator.EventReader` reads events with offset tracking:

```go
reader := narrator.NewEventReader(townRoot)

// Read all events
events, err := reader.ReadAll()

// Read new events since last read
newEvents, err := reader.ReadNew()

// Set offset for resuming
reader.SetOffset(lastOffset)
```

### Filtering

Filter functions help process events:

```go
// By rig
events := narrator.FilterByRig(allEvents, "gastown")

// By significance (at least medium)
events := narrator.FilterBySignificance(allEvents, narrator.SignificanceMedium)

// By time range
events := narrator.FilterByTimeRange(allEvents, start, end)

// By event types
events := narrator.FilterByTypes(allEvents, []string{"sling", "done"})

// Exclude event types
events := narrator.ExcludeTypes(allEvents, []string{"patrol_started"})
```

## Integration

The Narrator integrates with Gas Town's agent system:

- **Session management**: Uses the standard tmux session pattern
- **State persistence**: State stored via `agent.StateManager`
- **Mail**: Can receive mail with instructions via `gt mail`
- **Nudge**: Can be nudged with `gt nudge narrator`

## Usage Tips

1. **Style selection**: Choose based on your audience
   - `book` for documentation or internal records
   - `tv-script` for presentations or storytelling
   - `youtube-short` for social sharing

2. **Filtering**: Use `rig_filter` to focus on specific projects

3. **Recovery**: The offset tracking ensures events aren't processed twice
   after restarts

4. **Mail instructions**: Send the Narrator mail to request specific outputs:
   ```bash
   gt mail send narrator -s "Summary request" -m "Generate a summary of today's merges"
   ```
