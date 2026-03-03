#!/usr/bin/env python3
"""
Full Gas Town workspace setup using gt dolt start.
1. Start dolt server via gt dolt start
2. gt dolt init-rig ai_marketplace
3. gt rig add ai_marketplace --adopt
4. Seed 16 issues via bd create
5. gt convoy list / bd list
6. gt sling --dry-run
"""
import subprocess, os, sys, json, time

home = os.path.expanduser('~')
go_bin = home + '/go/bin'
local_bin = home + '/.local/bin'
env = os.environ.copy()
env['PATH'] = (home + '/.local/go/bin:' + go_bin + ':' + local_bin +
               ':/usr/local/bin:/usr/bin:/bin:' + env.get('PATH', ''))
env['HOME'] = home

workspace = home + '/gt'
rig_src = '/mnt/c/gitrepos/gastown/projects/ai-marketplace'

def run(cmd, **kwargs):
    print('$', cmd)
    r = subprocess.run(cmd, shell=True, env=env, capture_output=True, text=True, **kwargs)
    if r.stdout.strip():
        print(r.stdout.strip())
    if r.stderr.strip():
        print('STDERR:', r.stderr.strip())
    return r

# ─── Step 1: Fix rig.toml name ────────────────────────────────────────────────
rig_toml_path = workspace + '/.gt/rig.toml'
if os.path.exists(rig_toml_path):
    with open(rig_toml_path) as f:
        content = f.read()
    if '"ai-marketplace"' in content:
        content = content.replace('"ai-marketplace"', '"ai_marketplace"')
        with open(rig_toml_path, 'w', newline='\n') as f:
            f.write(content)
        print('Fixed rig.toml: ai-marketplace → ai_marketplace')

# ─── Step 2: Start dolt server ────────────────────────────────────────────────
print('\n=== gt dolt status ===')
r = run(f'cd {workspace} && gt dolt status 2>&1 || true')
already_running = 'running' in (r.stdout + r.stderr).lower()

if not already_running:
    print('\n=== gt dolt start (background) ===')
    bg = subprocess.Popen(
        f'cd {workspace} && gt dolt start',
        shell=True, env=env,
        stdout=open('/tmp/dolt.log', 'w'),
        stderr=subprocess.STDOUT
    )
    print(f'Dolt server starting (PID {bg.pid})...')
    time.sleep(5)
    r = run(f'cd {workspace} && gt dolt status 2>&1 || true')
    print()

# ─── Step 3: Init rig database ────────────────────────────────────────────────
print('=== gt dolt init-rig ai_marketplace ===')
run(f'cd {workspace} && gt dolt init-rig ai_marketplace --prefix mkt 2>&1 || true')
print()

# ─── Step 4: gt rig add ───────────────────────────────────────────────────────
print('=== gt rig add ai_marketplace ===')
run(f'cd {workspace} && gt rig add ai_marketplace {rig_src} --prefix mkt --adopt --force 2>&1 || true')
print()

# ─── Step 5: Seed issues ──────────────────────────────────────────────────────
print('=== Seeding 16 beads ===')
issues_jsonl = rig_src + '/.beads/issues.jsonl'
if os.path.exists(issues_jsonl):
    with open(issues_jsonl) as f:
        issues = [json.loads(line) for line in f if line.strip()]

    # Check if beads already exist
    r = run(f'cd {workspace} && bd list --json --limit 1 2>&1 || true')
    already_seeded = False
    if r.stdout:
        try:
            existing = json.loads(r.stdout)
            if existing:
                print(f'Beads already exist; skipping seed.')
                already_seeded = True
        except Exception:
            pass

    if not already_seeded:
        for issue in issues:
            title = issue.get('title', '').replace('"', '\\"')
            desc  = issue.get('description', '').split('\n')[0].replace('"', '\\"')[:200]
            itype = issue.get('issue_type', 'task')
            pri   = issue.get('priority', 2)
            cmd = (f'cd {workspace} && bd create --title "{title}" '
                   f'--type {itype} --priority {pri} --desc "{desc}" -q 2>&1 || true')
            run(cmd)
else:
    print('WARNING: issues.jsonl not found')
print()

# ─── Step 6: bd list ─────────────────────────────────────────────────────────
print('=== bd list (open issues) ===')
run(f'cd {workspace} && bd list --status open --pretty 2>&1 || '
    f'bd list --status open 2>&1 || true')
print()

# ─── Step 7: gt convoy list ───────────────────────────────────────────────────
print('=== gt convoy list ===')
run(f'cd {workspace} && gt convoy list 2>&1 || true')
print()

# ─── Step 8: gt sling --dry-run ───────────────────────────────────────────────
print('=== gt sling <first-bead> --agent claude --dry-run ===')
r = run(f'cd {workspace} && bd list --json --limit 1 2>&1 || true')
sling_id = None
if r.stdout:
    try:
        beads = json.loads(r.stdout)
        if beads:
            sling_id = beads[0].get('id')
    except Exception:
        pass
if sling_id:
    run(f'cd {workspace} && gt sling {sling_id} --agent claude --dry-run 2>&1 || true')
else:
    print('No beads found — bd list may need a running dolt server.')
print()

print('=== Setup Complete ===')
print(f'Workspace: {workspace}')
print()
print('Persistent usage (in WSL):')
print('  cd ~/gt && gt dolt start &    # persist dolt server')
print('  gt mayor attach               # Mayor orchestrator in tmux')
print('  bd list                       # view beads')
print('  gt sling <id> --agent claude  # spawn polecat agent')

