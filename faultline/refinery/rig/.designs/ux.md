# User Experience Analysis

## Summary

Faultline occupies a unique position: it's an error tracker whose primary consumers are coding agents (polecats), with humans as secondary users who need oversight and intervention capabilities. This dual-audience design creates a UX challenge that most error trackers don't face — the system must be API-first for agents while remaining approachable for product managers and developers who need to understand what's happening, intervene when automation fails, and trust the agentic loop.

The core UX insight is that faultline's value proposition lives in the *visibility of autonomous repair*. Users don't just want to see errors — they want to see errors being fixed. The "Fixing — polecat obsidian dispatched 3 min ago" status badge is the hero moment. Every UX decision should optimize for building confidence in the agentic loop: making the automated process legible, its failures visible, and human intervention effortless when needed.

## Analysis

### Key Considerations

- **Two distinct user personas with opposing needs**: PMs want scannable summaries and trend lines; developers want stack traces and raw JSON; agents want structured API responses. The system must serve all three without fragmenting into three separate products.
- **Trust is the primary UX challenge**: Users are being asked to trust that an AI agent is fixing their production errors. Every UI element related to the agentic loop must build confidence through transparency — showing what the polecat is doing, how long it's been working, and what it's tried.
- **The geological metaphor is an asset, not a liability**: "Tremor" vs "Quake" vs "Rupture" communicates severity gradient intuitively. But it must remain flavor, not friction — users should never need to consult a glossary.
- **Self-hosted means self-served**: No onboarding team. No sales engineer. The setup experience (install binary, configure DSN, send first error) IS the product's first impression.
- **SDK compatibility is invisible UX**: Users won't know faultline silently drops `transaction`, `profile`, and `replay` items. This is fine initially but will become a support burden when users expect features that exist in Sentry but not here.

### Options Explored

#### Option 1: CLI-first with minimal dashboard
- **Description**: Lean into the agent-first philosophy. Primary interaction is via CLI (`faultline status`, `faultline issues`, `faultline resolve`). Dashboard is a read-only status board.
- **Pros**: Aligns with the agent-first identity. Minimal frontend engineering. Developers already live in terminals. Single binary deployment stays simple.
- **Cons**: Excludes PMs and non-technical stakeholders entirely. Loses the "Fixing" visual moment. Hard to browse stack traces in a terminal. Limits adoption to teams where everyone is CLI-comfortable.
- **Effort**: Low

#### Option 2: Dashboard-first with API parity (current plan)
- **Description**: Full web dashboard (templ + htmx) with content negotiation — same handlers serve JSON for agents and HTML for humans. Three screens: Seismograph (issue list), Fault Report (issue detail), Core Sample (event detail).
- **Pros**: Serves all three personas. Progressive disclosure (PM sees summary, dev clicks for depth, agent gets JSON). The "Fixing" status with polecat name is visually powerful. templ + htmx keeps the Go single-binary promise. No Node.js build step.
- **Cons**: templ + htmx has a lower ceiling for interactivity than React. Keyboard navigation and filter bar will need careful implementation. Content negotiation adds complexity to every handler. Dashboard becomes the largest engineering surface.
- **Effort**: High

#### Option 3: Hybrid — minimal dashboard + rich notifications
- **Description**: Simple dashboard for browsing issues, but push the "awareness" UX into Slack/email notifications. Users learn about errors and resolutions through channels they already monitor.
- **Pros**: Meets users where they are. Slack threads per issue group provide natural conversation context. Reduces dashboard scope. Polecat progress updates in Slack are highly visible.
- **Cons**: Adds Slack integration dependency. Notification fatigue if not carefully throttled. Users still need somewhere to drill into stack traces. Fragments the experience.
- **Effort**: Medium

### Recommendation

**Option 2 (dashboard-first with API parity)** is the right approach, with specific UX priorities:

**1. Optimize the first-run experience.** The gap between "binary installed" and "first error visible" is the highest-risk UX moment. Current setup requires setting three environment variables and knowing the Sentry SDK DSN format. Recommendations:
- Dashboard first-run wizard: create account, create project, copy DSN — all in-browser
- `faultline init` CLI command that generates a starter config and prints a DSN
- "Waiting for first event..." state with a test button that sends a sample error
- Show SDK snippets for each of the 5 supported platforms directly in the dashboard

**2. Make the agentic loop the centerpiece.** This is faultline's only competitive advantage. Every issue card should make the automation state immediately visible:
- **Active fault** (red pulse) — errors accumulating, no polecat yet
- **Repair crew deployed** (amber pulse) — polecat dispatched, show elapsed time
- **Stabilized** (green, static) — fix merged, quiet period confirmed
- **Aftershock** (red flash) — regression detected, new polecat may be dispatched
- **Manual inspection needed** (yellow, static) — polecat escalated, human required

The transition from "Active" to "Fixing" to "Stabilized" should feel like watching a repair crew respond to an earthquake — the system is handling it, and you can see it happening.

**3. Design for the intervention moment.** When a polecat can't fix an issue and escalates, the human needs:
- What the polecat tried (linked bead with notes)
- The stack trace and sample event (same view the polecat had)
- One-click actions: resolve manually, ignore, reassign to a different rig, increase severity
- Clear indication of what happens next ("No polecat will be dispatched until you take action")

**4. Handle the "silent drop" UX gracefully.** When faultline silently drops `transaction`, `profile`, or `replay` items, the user gets no signal. This is correct for MVP but should be documented:
- SDK setup docs should state what's supported and what's ignored
- A `/api/v1/capabilities` endpoint lets SDKs/tools discover what's processed
- Dashboard "Project Settings" page shows which item types are processed vs dropped
- Avoid the failure mode where a user configures Sentry performance monitoring, sees no data in faultline, and thinks it's broken

**5. Progressive disclosure for the geological metaphor.** The severity scale (Tremor/Quake/Rupture) and magnitude language (Micro/Minor/Moderate/Strong/Major/Great) are genuinely good — they communicate gradient intuitively. But:
- Always show the raw count alongside the magnitude label: "Moderate (34 events)" not just "Moderate"
- First-time users should see a tooltip or legend explaining the scale exactly once
- API responses should include both the geological label and the standard Sentry level (for tool compatibility)
- Keyboard shortcuts and filter syntax should accept both: `level:fatal` and `severity:rupture`

## Constraints Identified

1. **Single-binary deployment constrains frontend complexity.** The templ + htmx choice is correct for keeping faultline a single Go binary, but it means no client-side routing, no complex state management, and limited offline capability. Interactive features (keyboard navigation, live filtering, real-time updates) must work within htmx's model.

2. **Content negotiation adds handler complexity.** Every API handler that serves both JSON and HTML must handle Accept headers, error rendering in both formats, and pagination in both formats. This is a significant ongoing cost.

3. **No source maps in MVP means unreadable JS/mobile stack traces.** For browser and React Native SDKs, stack traces will show minified code. This severely limits the dashboard's value for JS-heavy teams until source map support lands (P6). The UX should acknowledge this: "Minified stack trace — source map upload coming soon."

4. **Rate limiting UX must match Sentry SDK expectations.** SDKs expect `429 Too Many Requests` with `Retry-After` header. The SDK handles backoff automatically and silently. The faultline dashboard should show when rate limiting is active ("12 events dropped in last 5 min due to rate limit") so operators can tune thresholds.

5. **Self-hosted means no usage analytics.** Faultline can't phone home to track adoption, feature usage, or error rates. UX decisions must be based on the design brief and dogfooding, not telemetry. Consider an opt-in anonymous usage stats flag for future feedback.

## Open Questions

1. **What is the first-run account creation flow?** The current implementation has a dashboard login page but no first-run setup wizard. Does the first visitor create the admin account? Is there an invite flow? How does a team onboard multiple developers?

2. **How should faultline surface its own errors?** An error tracker that crashes silently is ironic. Should the dashboard show faultline health (Dolt connectivity, ingest throughput, queue depth)? Should faultline dogfood itself — tracking its own errors in its own database?

3. **What is the notification model before Slack integration?** Slack is planned for P5/P6. Between now and then, how does a human learn that an error occurred or that a polecat escalated? Email? Dashboard polling? Webhook to an arbitrary URL?

4. **How should resolved issues age out of the default view?** Sentry's default is "unresolved" — resolved issues disappear. Faultline's agentic loop means many issues self-resolve. If the default view hides resolved issues, users might never see the agentic loop working. Consider showing recently-resolved issues in a "Stabilized today" section.

5. **What is the mobile UX story?** With iOS and Android SDKs in scope, some users will want to check error status on mobile. Is the dashboard responsive? Is there a mobile app? Or is mobile monitoring handled entirely through Slack notifications?

## Integration Points

- **SDK Compatibility (ADR-1)**: Silent drop behavior must be communicated through UX — docs, dashboard settings, capabilities endpoint. Users should never wonder "why isn't my performance data showing up?"
- **Gas Town Integration (ADR-3)**: The bead creation trigger (3+ events in 5min) is invisible to users. The dashboard should surface this threshold: "Polecat dispatch threshold: 3 events in 5 minutes. Customize in project settings."
- **Regression Detection (ADR-4)**: The 24h regression window should be visible in the issue lifecycle. When an issue is resolved, show "Monitoring for regression (23h remaining)" countdown.
- **Brand/UI Design**: The geological metaphor, color palette, and component patterns in DESIGN-BRIEF.md are strong foundations. UX recommendations here are additive — they address interaction patterns, flows, and mental models that the design brief covers visually but not behaviorally.
- **Architecture**: Content negotiation (JSON + HTML from same handlers) is an architectural decision with deep UX implications. The HTML rendering must not be a second-class citizen — if agents get better data from the API than humans get from the dashboard, the dashboard will be abandoned.
