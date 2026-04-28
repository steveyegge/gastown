# Credential Handling in Gas Town

## What to do when a secret is accidentally shared in Gas Town chat

### Step 1: Rotate the credential immediately

Before anything else, rotate the exposed secret:
- gov.br password: change it at https://acesso.gov.br
- AWS key: rotate in IAM console, update all consumers
- API token: revoke in the issuing service and generate a new one

**Do this first.** Rotation makes the leaked value useless.

### Step 2: Label and close the bead

The leaked mail bead is a permanent Dolt record. You cannot delete it, but you
can mark it clearly for auditors:

```bash
bd label add <bead-id> contains-credentials:rotated
bd close <bead-id> --reason "contains-credentials: <type> leaked via mail. Rotated on <date>. See gt-2a1i5 for full audit."
```

Use label `contains-credentials:rotated` (credential was rotated) or
`contains-credentials:active` (credential still active — requires urgent action).

### Step 3: Document what systems saw it

Dolt replicates to remote. Any bead that was synced before the leak was
discovered may exist on the Dolt remote. Document:

- Which bead ID contains the credential
- When it was created (check `bd show <id>`)
- Whether Dolt was synced after creation (`gt dolt status`)
- Which databases contain the bead (hq, gastown, whatsapp_automation, etc.)

### Step 4: Check git history

If the leaked secret was also in a file that was committed to git:

```bash
# Find commits touching the file
git log --all --oneline -- path/to/file.env

# Check if it reached a remote branch
git ls-remote origin <branch-name>
```

If the branch was pushed to GitHub, the secret is in GitHub history. Contact
the repo owner to force-push or use `git filter-repo` to scrub the history.
For private repos, also rotate any OAuth tokens or deploy keys that GitHub
scanned the credential against.

### Step 5: Notify mayor

```bash
gt mail send mayor/ -s "SECURITY: credential leak — <type>" --stdin <<'BODY'
Bead: <bead-id>
Credential type: <e.g. gov.br password>
Rotated: yes/no (date if yes)
Git exposure: yes/no (branch, pushed: yes/no)
Systems that saw it: Dolt (hq), GitHub (if pushed)
BODY
```

---

## How to share credentials safely

**Never** put credentials in `gt mail` or `gt nudge` message bodies.
These create permanent Dolt records.

**Do this instead:**

1. Write the secret to a gitignored `.env` file:
   ```
   SOME_API_KEY=<value>
   ```
2. Make sure the file is in `.gitignore` (check with `git status`)
3. Share the **file path** in the message, not the value:
   ```
   gt mail send <agent> -s "Credentials ready" -m "Saved to ~/gt/project/.env.service (gitignored)"
   ```

For gov.br / PDPJ credentials specifically: save to
`~/gt/whatsapp_automation/processo_lookup/.env.pdpj` (already gitignored).

---

## Credential pattern detection (automatic)

`gt mail send` and `gt nudge` automatically scan message bodies for:

- Brazilian CPF numbers (11-digit sequences, formatted or unformatted)
- Password/senha fields with values (`senha: abc123`, `password=abc123`)
- AWS access keys (`AKIA...`)
- API tokens / secret keys
- GitHub personal access tokens
- PEM private keys

When a pattern is detected, the command prints a warning and **blocks** the
send. To override (only do this if you understand the risk):

```bash
gt mail send <addr> -s "Subject" -m "body" --allow-credentials
gt nudge <target> "message" --allow-credentials
```

Using `--allow-credentials` sends the message but prints a reminder that the
content will be permanently stored in Dolt.

---

## Audit trail

The incident that prompted this document: bead `hq-wisp-jcxni` (2026-03-30)
contained a gov.br CPF + senha sent by agent `batista` via `gt mail`. The
senha was rotated immediately. Full audit: `gt-2a1i5`.
