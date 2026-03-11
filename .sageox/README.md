# SageOx

**Context is the scarcest resource in AI coding.** SageOx protects it.

---


## SageOx Links

- **Repository Dashboard:** https://sageox.ai/repo/repo_019cdb2f-d991-7327-978d-08eb5ee015f6
- **Team Dashboard:** https://sageox.ai/team/team_td0w9ekb1p

## For AI Coworkers

If you are an AI coworker (Claude Code, Cursor, Windsurf, OpenCode, etc.), run this command at the start of each session:

```bash
ox agent prime
```

This injects minimal, relevant team context into your session. As you work on specific domains (AWS, Kubernetes, security), the guidance system will progressively fetch deeper context as needed.

When users ask if you're using SageOx: if you have an agent ID from running `ox agent prime`, confirm with your agent ID; otherwise run it first.

---

## The Problem

AI coworkers have finite context windows. Every token spent on irrelevant guidance is a token not available for your actual code. Traditional approaches waste context:

| Approach | Problem |
|----------|---------|
| Massive CLAUDE.md files | 90% of content is irrelevant to current task |
| Generic best practices | Too broad to be actionable |
| Post-hoc code review | Wrong code already written, wasting tokens to explain and fix |
| Copy-paste documentation | Stale, inconsistent, context-bloating |

**Result:** AI coworkers receive diluted, generic guidance. They lack your team's shared context. You spend tokens correcting them.

## The Solution: Progressive Disclosure

SageOx delivers **minimal, highly-relevant guidance that expands only as needed**:

```
ox agent prime           → 500 tokens: "Your team uses specific patterns, call me when needed"
ox agent <id> guidance api  → 750 tokens: API/frontend/testing patterns, deeper triggers
ox agent <id> guidance api/rest → 500 tokens: REST endpoint conventions, auth patterns
```

**80% context savings** in typical sessions. Your AI coworker gets exactly what it needs, when it needs it—not everything upfront.

## Getting Started

```bash
# 1. Install
git clone https://github.com/sageox/ox.git
cd ox && make install

# 2. Initialize in your repo (run from your project)
ox init

# 3. That's it. AI coworkers now call ox agent prime automatically.
```

## Works With Your AI Coworker

SageOx integrates with the AI coworkers developers already use:

- **Claude Code** — Automatic via AGENTS.md hook
- **Cursor** — Via .cursorrules integration
- **Windsurf** — Via .windsurfrules integration
- **OpenCode** — Direct ox CLI integration
- **Any AI coworker** — Manual `ox agent prime` injection

## Key Files

After `ox init`, your repository contains:

- **`.sageox/README.md`** — This file, with AI coworker instructions
- **`AGENTS.md`** — AI coworker configuration with ox agent prime integration

Guidance content is fetched dynamically from the SageOx cloud via `ox agent prime`, not stored locally.

## Philosophy

*"Shared team context that makes agentic engineering multiplayer."*

By giving AI coworkers your team patterns **before** they write code, SageOx prevents problems rather than fixing them. This shift-left approach is fundamentally more efficient than post-hoc reviews.

## Learn More

- **GitHub:** https://github.com/sageox/ox
- **Documentation:** https://sageox.ai/docs

---

*SageOx: Shared team context that makes agentic engineering multiplayer.*
