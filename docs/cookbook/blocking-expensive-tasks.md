# Blocking Expensive Tasks

**Problem:** Running tests, builds, or other CPU-intensive work while other tasks dispatch, causing overload.

**Solution:** Use `bd dep add --type=blocks` to make expensive tasks block concurrent dispatch.

---

## Quick Start

### Make One Task Exclusive

```bash
bd create "Run full test suite" --type=task
gt block gt-abc
```

While `gt-abc` runs, no other tasks dispatch.

---

## The Pattern

1. File expensive task first
2. Make it block everything: `gt block gt-expensive`
3. Dispatch: `gt sling gt-expensive sfgastown`
4. When complete, close task — normal dispatch resumes

---

## Create `gt block` Helper

Save as `~/go/bin/gt-block`:

```bash
#!/bin/bash
if [ -z "$1" ]; then
    echo "Usage: gt block <task-id>"
    exit 1
fi
READY=$(bd list --ready | grep "^○" | grep -v "$1" | awk '{print $2}')
for task in $READY; do
    bd dep add "$1" "$task" --type=blocks
done
echo "$1 now blocks $(echo $READY | wc -w) tasks"
```

---

## Use Cases

- Full test suite
- Release builds
- Database migrations
- Model training (hours)
- Large data processing

---

## Best Practices

1. File expensive tasks early
2. Use clear names: "Integration tests (30 min)"
3. Don't abuse — only for truly expensive work
4. Unblock when done (close task)
