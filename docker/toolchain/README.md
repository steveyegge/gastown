# Gastown Toolchain Sidecar Image

Base image for K8s agent pod toolchain sidecars.

## Targets

| Target | Tools | Size |
|--------|-------|------|
| `full` | Go, Node, Python, AWS CLI, Docker CLI, kaniko | ~1.5 GB |
| `minimal` | git, jq, make, curl | ~200 MB |

## Build

```bash
docker build --platform linux/amd64 --target full -t gastown-toolchain:full docker/toolchain/
docker build --platform linux/amd64 --target minimal -t gastown-toolchain:minimal docker/toolchain/
```

## Fork

Override build ARGs to customize versions:
```bash
docker build --build-arg GO_VERSION=1.23.5 --build-arg NODE_MAJOR=20 --target full ...
```
