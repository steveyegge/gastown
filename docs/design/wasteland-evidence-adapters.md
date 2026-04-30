# Wasteland Evidence Adapters

> **Status: Proposal.** No implementation yet; filed to gather feedback before building.

## Problem

Today `gt wl done <id> --evidence <string>` accepts an arbitrary opaque string
and writes it verbatim to `wanted.evidence_url` and `completions.evidence` in
wl-commons. The Spider Protocol (see `internal/wasteland/spider.go`) then
reasons about "evidence variation" at the string level — it can detect that
one rig keeps submitting the same `evidence` value, but it cannot distinguish
*what kind of thing* a given evidence string points at, and it cannot verify
that the pointed-at thing corresponds to real work.

This has three practical consequences:

1. **No verifiability at submission time.** A completion claiming to be a PR
   link is not checked: it might be a typo, a dead link, a PR against a
   different repo, or plain prose. The first time anyone notices is during
   stamp review, by which point the completion is already in the stamps
   graph.
2. **No structure for Spider to exploit.** Spider's confidence-inflation
   detector treats `"https://github.com/x/y/pull/123"` and
   `"https://github.com/x/y/pull/124"` as different strings, which is
   technically correct but loses the information that both are PR links
   against the same repo. A stamper who issues 200 stamps for 200 completions
   that all link to the same repo is a stronger signal than 200 unique
   strings.
3. **No path for future evidence kinds.** As wanted items diversify (docs,
   review sweeps, infra work, physical deliverables) there is no extension
   point. Every new kind of evidence has to be crammed through the opaque
   string.

## Proposal: pluggable evidence adapters

Introduce an `EvidenceAdapter` interface that parses, normalizes, classifies,
and optionally verifies an evidence submission. `gt wl done` routes the
submitted string through a registered adapter before writing to wl-commons;
the resolved fields are persisted alongside the raw string for
backward-compatible replay.

### Adapter interface

```go
package wasteland

// EvidenceKind is a stable enum of evidence categories. New kinds are
// additive; Spider and dashboards should treat unknown kinds as opaque and
// fall back to string-level reasoning.
type EvidenceKind string

const (
    EvidencePR           EvidenceKind = "pr"
    EvidenceCommit       EvidenceKind = "commit"
    EvidenceDoc          EvidenceKind = "doc"           // PR or commit touching docs/
    EvidenceURL          EvidenceKind = "url"           // generic web link
    EvidenceReviewSweep  EvidenceKind = "review_sweep"  // audit-of-N-items evidence
    EvidenceOpaque       EvidenceKind = "opaque"        // fallback — raw string, no structure
)

// ResolvedEvidence is what gets persisted. The raw field preserves exactly
// what the user submitted for audit and for forward-compat replay when
// adapters change.
type ResolvedEvidence struct {
    Kind        EvidenceKind
    Raw         string            // verbatim from --evidence
    Normalized  string            // canonical form (e.g., canonical PR URL)
    Identity    string            // adapter-specific stable identifier (e.g., "github:acme/backend#123")
    Attributes  map[string]string // kind-specific structured fields (repo, sha, title, ...)
    Verified    bool              // adapter ran a liveness/reachability check and it passed
    VerifiedAt  time.Time         // zero value if Verified == false
}

type EvidenceAdapter interface {
    // Name returns a stable adapter ID (e.g., "github-pr"). Used for logging
    // and for the adapter-chain selection.
    Name() string

    // Matches reports whether this adapter can handle the raw string. The
    // registry short-circuits on the first match. Order-sensitive — register
    // more specific adapters before more general ones.
    Matches(raw string) bool

    // Resolve parses, normalizes, and optionally verifies. Verification is
    // opt-in per adapter and gated by config (offline submissions stay
    // possible). Never mutates raw.
    Resolve(ctx context.Context, raw string) (ResolvedEvidence, error)
}
```

### Initial adapter set

- **`github-pr`**: matches `https://github.com/{org}/{repo}/pull/{n}`.
  Normalizes to the canonical URL. Identity:
  `github:{org}/{repo}#{n}`. If verification is enabled and a token is
  available, fetches the PR to confirm it exists and capture title/state.
- **`github-commit`**: matches full or abbreviated commit URLs, and bare
  40-char hashes when `--repo` is supplied. Identity:
  `github:{org}/{repo}@{sha}`.
- **`doc-change`**: a specialization of `github-pr`/`github-commit` that
  fires when the underlying PR/commit touches only `docs/` (or a
  configured set of paths). Spider can then reason about "this rig only
  claims doc completions" as a distinct category.
- **`generic-url`**: any `http(s)://...` that doesn't match a more specific
  adapter. Verifies reachability if enabled.
- **`review-sweep`**: matches a specific schema
  (`review-sweep://{sweep-id}?items=N&findings=M`) introduced for rigs that
  stamp bulk reviews. Identity: `sweep:{sweep-id}`. Gives Spider a clean
  surface to detect rubber-stamp sweeps.
- **`opaque`**: terminal fallback. Always matches. Kind = `EvidenceOpaque`.
  No verification. Preserves today's behavior.

Adapters live in `internal/wasteland/evidence/`; registration is explicit
in `init()` per adapter so the build can drop an adapter by omitting the
import.

### Schema additions

Three new nullable columns on `wanted` and `completions`, mirroring each
other. Default null so rows written by older `gt` versions continue to
work:

- `evidence_kind` (varchar, nullable)
- `evidence_identity` (varchar, nullable, indexed)
- `evidence_verified_at` (timestamp, nullable)

`evidence_url` / `evidence` stay as today — they carry `Raw`. The
normalized form goes into a new `evidence_normalized` column or, if that
table-growth is unwelcome, into `evidence_attributes_json`.

Migration is additive: one Dolt schema commit that adds the columns, no
data rewrite. Back-population is a separate opt-in pass — a command like
`gt wl backfill-evidence` that reads existing rows, runs them through the
registry, and writes the resolved fields. Skippable; Spider already works
without it.

### CLI backward-compat

Today:

```
gt wl done w-abc123 --evidence 'https://github.com/acme/repo/pull/42'
```

After this change the same command works identically from the user's
perspective. Internally, `wl_done.go` calls the registry, writes the
resolved fields, and falls back to `EvidenceOpaque` on no match. No
user-visible change unless they opt into richer output:

```
gt wl done w-abc123 --evidence 'https://github.com/acme/repo/pull/42' --verify
```

`--verify` forces the adapter to run its verification step and fails the
submission if verification fails. Absent the flag, verification is
best-effort and governed by `wl.evidence.verify_default` in config.

### Spider integration

Spider gains three new signal variants that use the resolved fields:

- `SignalSameIdentity`: rig X submits M completions pointing at the same
  `evidence_identity`. Much cleaner than the current "same string" check
  (catches `?utm=...` variants and canonical-vs-raw URL collisions).
- `SignalKindMonoculture`: rig X's completions are 100% `doc_change` across
  N stamps — not proof of fraud, but a useful axis for the scorekeeper.
- `SignalUnverified`: rig X's completions are disproportionately
  `Verified == false` despite verifiable kinds being available.

Crucially, the existing signals keep working unchanged by reading `Raw`
when the resolved fields are null. Spider does not need a flag day.

## Non-goals

- **Not a replacement for stamps or the scorekeeper.** Adapters resolve
  *what was submitted*, not *whether it was good*. Stamps still carry
  subjective valence.
- **Not a policy engine.** An adapter may refuse to resolve a bad URL, but
  it does not decide whether a wanted item's evidence is "sufficient."
  That stays at the stamp layer.
- **Not cross-platform yet.** First adapter set is GitHub-shaped. GitLab /
  Gitea / Forgejo adapters are straightforward follow-ups but out of scope
  for the initial PR.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Verification calls slow down `gt wl done` or leak tokens. | Verification is opt-in and off by default. Adapters read tokens from env vars already used elsewhere (`GITHUB_TOKEN`). Offline rigs are a first-class case. |
| Schema migration breaks wl-commons consumers outside this repo. | All new columns nullable; old readers ignore. No constraint changes. |
| Adapter registry mis-classifies and misleads Spider. | `opaque` always wins on ambiguity; adapter tests assert round-trip on the fixtures directory. |
| Proliferation of adapter kinds. | `EvidenceKind` starts with six values; adding a seventh requires a PR with a design note. Spider treats unknown kinds as opaque, so an unrecognized kind is a performance regression, not a correctness one. |

## Rollout

1. Land the interface + registry + `opaque` adapter with tests. Zero
   behavior change.
2. Add schema columns (nullable). Zero behavior change.
3. Land `github-pr`, `github-commit`, `generic-url` adapters with
   verification off by default. Resolve-only path active; `Raw` unchanged.
4. Wire `gt wl done` to call the registry; persist resolved fields.
5. Spider: add `SignalSameIdentity`, gated so it only fires when at least
   one side of the comparison has a non-null `evidence_identity` (prevents
   false positives during the back-fill gap).
6. Optional: `gt wl backfill-evidence` for historical rows.
7. Follow-ups: `doc-change`, `review-sweep` adapters; GitLab/Gitea.

Each step is independently mergeable and revertable.

## Open questions

- Should `evidence_identity` live in a separate `completion_evidence` table
  rather than widening `completions`? Cleaner normalization, but crosses a
  join for every Spider query.
- Is `--verify` the right UX, or should verification be a post-submission
  `gt wl verify <completion-id>` command (asynchronous, re-runnable)?
- Adapter versioning: when `github-pr` changes its normalization rules, do
  we re-resolve all historical rows, or freeze `Resolved*` at write time
  and accept drift? I lean frozen; the raw string is the source of truth.

Feedback welcome. Happy to split the rollout across more PRs if the
six-step list is too coarse.
