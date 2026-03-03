#!/usr/bin/env python3
"""Rebuild gt with proper ldflags (BuiltProperly=1), then run workspace setup."""
import subprocess, os, sys

home = os.path.expanduser('~')
go_bin = home + '/.local/go/bin'
user_bin = home + '/go/bin'
env = os.environ.copy()
env['PATH'] = go_bin + ':' + user_bin + ':' + env.get('PATH', '')
env['HOME'] = home

repo = '/mnt/c/gitrepos/gastown'
gt_out = home + '/go/bin/gt'
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

# --- Step 1: get version info from git ---
ver_r = subprocess.run('git -C ' + repo + ' describe --tags --always --dirty 2>/dev/null || echo dev',
                       shell=True, env=env, capture_output=True, text=True)
version = ver_r.stdout.strip() or 'dev'

commit_r = subprocess.run('git -C ' + repo + ' rev-parse --short HEAD 2>/dev/null || echo unknown',
                          shell=True, env=env, capture_output=True, text=True)
commit = commit_r.stdout.strip() or 'unknown'

import datetime
build_time = datetime.datetime.utcnow().strftime('%Y-%m-%dT%H:%M:%SZ')

ldflags = (
    f'-X github.com/steveyegge/gastown/internal/cmd.Version={version} '
    f'-X github.com/steveyegge/gastown/internal/cmd.Commit={commit} '
    f'-X github.com/steveyegge/gastown/internal/cmd.BuildTime={build_time} '
    f'-X github.com/steveyegge/gastown/internal/cmd.BuiltProperly=1'
)

print('=== Rebuilding gt with BuiltProperly=1 ===')
print(f'Version: {version}  Commit: {commit}')
r = run(f'cd {repo} && go build -ldflags "{ldflags}" -o {gt_out} ./cmd/gt')
if r.returncode != 0:
    print('BUILD FAILED — trying go generate first...')
    run(f'cd {repo} && go generate ./...')
    r = run(f'cd {repo} && go build -ldflags "{ldflags}" -o {gt_out} ./cmd/gt')
    if r.returncode != 0:
        print('ERROR: Build failed. Aborting.')
        sys.exit(1)

# Verify
r = run('gt version')
print()

# --- Step 2: gt install ---
print('=== gt install ===')
os.makedirs(workspace, exist_ok=True)
run(f'gt install {workspace} --git')
print()

# --- Step 3: copy beads into workspace ---
print('=== Copying project files into workspace ===')
beads_src = rig_src + '/.beads'
beads_dst = workspace + '/.beads'
run(f'mkdir -p {beads_dst}')
run(f'cp -r {beads_src}/. {beads_dst}/')

gt_src = rig_src + '/.gt'
gt_dst = workspace + '/.gt'
run(f'mkdir -p {gt_dst}')
run(f'cp -r {gt_src}/. {gt_dst}/')
print()

# --- Step 4: rig add ---
print('=== gt rig add ===')
run(f'cd {workspace} && gt rig add ai-marketplace {rig_src} --prefix mkt || true')
print()

# --- Step 5: convoy list ---
print('=== gt convoy list ===')
run(f'cd {workspace} && gt convoy list 2>&1 || true')
print()

# --- Step 6: bd list (beads) ---
print('=== bd list (sprint 1 beads) ===')
run(f'cd {workspace} && bd list --convoy mkt-cv-s1 2>&1 || true')
print()

# --- Step 7: sling mkt-00006 ---
print('=== gt sling mkt-00006 ===')
r = run(f'cd {workspace} && gt sling mkt-00006 --runtime claude 2>&1 || true')
print()

print('=== Done ===')
print(f'Workspace: {workspace}')
print('Run "wsl" then "gt mayor attach" to start the Mayor orchestrator.')
