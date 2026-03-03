#!/usr/bin/env python3
"""
Install a bd wrapper at ~/go/bin/bd-real + ~/go/bin/bd shim that strips
the --no-daemon flag, which gt expects but the released bd doesn't yet support.
"""
import os, subprocess, sys

home = os.path.expanduser('~')
go_bin = home + '/go/bin'
real_bd = go_bin + '/bd-real'
shim_bd = go_bin + '/bd'

# Rename real bd -> bd-real
existing_bd = go_bin + '/bd'
if os.path.exists(existing_bd):
    os.rename(existing_bd, real_bd)
    print(f'Moved original bd -> {real_bd}')
else:
    print('ERROR: bd binary not found at ' + existing_bd)
    sys.exit(1)

# Write the shim shell script
shim = '''#!/usr/bin/env bash
# bd shim: strips --no-daemon (not in released bd) and passes remaining args
ARGS=()
for arg in "$@"; do
    if [ "$arg" != "--no-daemon" ]; then
        ARGS+=("$arg")
    fi
done
exec "$(dirname "$0")/bd-real" "${ARGS[@]}"
'''

with open(shim_bd, 'w', newline='\n') as f:
    f.write(shim)
os.chmod(shim_bd, 0o755)
print(f'Wrote shim: {shim_bd}')

# Verify
env = os.environ.copy()
env['PATH'] = home + '/.local/go/bin:' + go_bin + ':' + env.get('PATH', '')
r = subprocess.run(['bd', 'version', '--no-daemon'], env=env, capture_output=True, text=True)
print('bd version --no-daemon:', r.stdout.strip() or r.stderr.strip())
print('returncode:', r.returncode)
