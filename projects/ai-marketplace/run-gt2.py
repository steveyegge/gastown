#!/usr/bin/env python3
"""After shim installed: run gt rig add + convoy list + sling."""
import subprocess, os

home = os.path.expanduser('~')
go_bin = home + '/go/bin'
env = os.environ.copy()
env['PATH'] = home + '/.local/go/bin:' + go_bin + ':' + env.get('PATH', '')
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

# rig add
print('=== gt rig add ===')
run(f'cd {workspace} && gt rig add ai-marketplace {rig_src} --prefix mkt --adopt --force || true')
print()

# convoy list
print('=== gt convoy list ===')
run(f'cd {workspace} && gt convoy list 2>&1 || true')
print()

# sling mkt-00006 with correct flag
print('=== gt sling mkt-00006 ===')
r = run(f'cd {workspace} && gt sling mkt-00006 --agent claude --dry-run 2>&1 || true')
print()

print('=== Done ===')
