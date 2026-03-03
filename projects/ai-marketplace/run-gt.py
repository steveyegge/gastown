import subprocess, os

env = os.environ.copy()
home = os.path.expanduser('~')
env['PATH'] = home + '/.local/go/bin:' + home + '/go/bin:' + env.get('PATH', '')

def run(cmd, **kw):
    print(f'$ {cmd}')
    r = subprocess.run(cmd, shell=True, capture_output=True, text=True, env=env, **kw)
    if r.stdout: print(r.stdout.rstrip())
    if r.stderr: print('STDERR:', r.stderr.rstrip()[:500])
    return r

# 1. gt install workspace
run('gt install ~/gt --git')

# 2. cd into workspace and add rig
run('cd ~/gt && gt rig add ai-marketplace https://github.com/your-org/ai-marketplace.git --prefix mkt || true')

# 3. gt convoy list (reads from beads if available)
print('\n=== gt convoy list ===')
r = run('gt convoy list 2>&1 || true')

# 4. Show what sling would do for mkt-00006
print('\n=== Simulated: gt sling mkt-00006 (visual orchestration canvas) ===')
print('Would sling bead mkt-00006 [FR-9: Visual orchestration canvas] to a polecat agent')
print('Requires: tmux + Claude Code CLI installed')
print('Command: gt sling mkt-00006 --runtime claude')

print('\n=== Summary ===')
print('Workspace: ~/gt')
print('Rig: ai-marketplace (prefix: mkt)')
print('Sprint 1 convoy: mkt-cv-s1  (6 beads)')
print('Sprint 2 convoy: mkt-cv-s2  (5 beads)')
print('Sprint 3 convoy: mkt-cv-s3  (5 beads)')
print('')
print('To start working:')
print('  wsl')
print('  source ~/.bashrc')
print('  gt mayor attach   # starts Mayor in tmux')
print('  gt convoy list    # see all active convoys')
print('  gt sling mkt-00006 --runtime claude  # spawn agent for orchestrator canvas')
