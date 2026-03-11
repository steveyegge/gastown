#!/usr/bin/env python3
"""Claude Code PreToolUse hook: auto-route bd create to correct rig.

Intercepts `bd create` commands missing `--rig` and rewrites them to include
the correct `--rig` flag based on keyword matching against a config file.

Config: ~/.claude/hooks/bd-route.yaml (or BD_ROUTE_CONFIG env var)

Installation:
  1. Copy this file to ~/.claude/hooks/bd-route.py
  2. Copy bd-route.yaml to ~/.claude/hooks/bd-route.yaml (edit keywords for your setup)
  3. Add to ~/.claude/settings.json under hooks.PreToolUse:
     {
       "matcher": "Bash",
       "hooks": [{"type": "command", "command": "~/.claude/hooks/bd-route.py"}]
     }
"""

import json
import os
import re
import sys
from pathlib import Path


# --- Config loading ---

def load_config():
    """Load routing rules from YAML config file.

    Uses a simple YAML subset parser (no PyYAML dependency).
    """
    config_path = os.environ.get(
        "BD_ROUTE_CONFIG",
        os.path.expanduser("~/.claude/hooks/bd-route.yaml"),
    )
    path = Path(config_path)
    if not path.exists():
        return []

    return parse_simple_yaml(path.read_text())


def parse_simple_yaml(text):
    """Parse the subset of YAML used by bd-route.yaml.

    Supports:
      rules:
        - match: ["kw1", "kw2"]
          rig: rigname
    """
    rules = []
    current_rule = {}
    in_rules = False

    for line in text.splitlines():
        stripped = line.strip()

        # Skip comments and blank lines
        if not stripped or stripped.startswith("#"):
            continue

        if stripped == "rules:":
            in_rules = True
            continue

        if not in_rules:
            continue

        # New rule entry
        if stripped.startswith("- match:"):
            if current_rule.get("match") and current_rule.get("rig"):
                rules.append(current_rule)
            match_val = stripped[len("- match:"):].strip()
            keywords = parse_yaml_list(match_val)
            current_rule = {"match": keywords, "rig": None}
        elif stripped.startswith("match:"):
            match_val = stripped[len("match:"):].strip()
            keywords = parse_yaml_list(match_val)
            current_rule["match"] = keywords
        elif stripped.startswith("rig:"):
            rig_val = stripped[len("rig:"):].strip().strip('"').strip("'")
            current_rule["rig"] = rig_val

    # Don't forget the last rule
    if current_rule.get("match") and current_rule.get("rig"):
        rules.append(current_rule)

    return rules


def parse_yaml_list(val):
    """Parse a YAML inline list like '["kw1", "kw2"]'."""
    val = val.strip()
    if val.startswith("[") and val.endswith("]"):
        val = val[1:-1]
    parts = []
    for item in val.split(","):
        item = item.strip().strip('"').strip("'")
        if item:
            parts.append(item)
    return parts


# --- Command parsing ---

def is_bd_create(command):
    """Check if this is a bd create command."""
    return bool(re.search(r'\bbd\s+(create|new)\b', command))


def has_rig_or_prefix(command):
    """Check if --rig or --prefix is already specified."""
    return bool(re.search(r'--rig\b|--prefix\b', command))


def extract_text_content(command):
    """Extract title and description text from bd create command for matching."""
    parts = []

    # Extract --title or -t value
    for pattern in [
        r'--title[=\s]+["\']([^"\']+)["\']',
        r'--title[=\s]+(\S+)',
        r'-t\s+["\']([^"\']+)["\']',
        r'-t\s+(\S+)',
    ]:
        m = re.search(pattern, command)
        if m:
            parts.append(m.group(1))
            break

    # Extract --description or -d value
    for pattern in [
        r'--description[=\s]+["\']([^"\']+)["\']',
        r'--description[=\s]+(\S+)',
        r'-d\s+["\']([^"\']+)["\']',
        r'-d\s+(\S+)',
    ]:
        m = re.search(pattern, command)
        if m:
            parts.append(m.group(1))
            break

    # Positional args: bd create "Fix the foo bar"
    positional = re.search(r'\bbd\s+(?:create|new)\s+["\']([^"\']+)["\']', command)
    if positional:
        parts.append(positional.group(1))

    return " ".join(parts).lower()


# --- Matching ---

def match_rules(text, rules):
    """Match text against rules, return matched rig or None.

    Returns the rig only if exactly one rule matches.
    If zero or multiple rules match, returns None (passthrough).
    """
    matched = []
    for rule in rules:
        for kw in rule["match"]:
            if kw.lower() in text:
                matched.append(rule)
                break

    if len(matched) == 1:
        return matched[0]["rig"]
    return None


# --- Command rewriting ---

def rewrite_command(command, rig):
    """Insert --rig <rig> into the bd create command."""
    return re.sub(
        r'(\bbd\s+(?:create|new))\b',
        rf'\1 --rig {rig}',
        command,
        count=1,
    )


# --- Main ---

def main():
    try:
        raw = sys.stdin.read()
        if not raw.strip():
            sys.exit(0)

        data = json.loads(raw)
    except (json.JSONDecodeError, EOFError):
        sys.exit(0)

    tool_name = data.get("tool_name", "")
    tool_input = data.get("tool_input", {})

    # Only process Bash tool calls
    if tool_name != "Bash":
        sys.exit(0)

    command = tool_input.get("command", "")

    # Fast exit: not a bd create command
    if not is_bd_create(command):
        sys.exit(0)

    # Skip if already has --rig or --prefix
    if has_rig_or_prefix(command):
        sys.exit(0)

    # Load config
    rules = load_config()
    if not rules:
        sys.exit(0)

    # Extract text to match against
    text = extract_text_content(command)
    if not text:
        sys.exit(0)

    # Match
    rig = match_rules(text, rules)
    if not rig:
        sys.exit(0)

    # Rewrite
    new_command = rewrite_command(command, rig)
    print(f"[bd-route] Auto-routing to --rig {rig}", file=sys.stderr)

    result = {
        "hookSpecificOutput": {
            "hookEventName": "PreToolUse",
            "permissionDecision": "allow",
            "updatedInput": {"command": new_command},
        }
    }

    # Preserve other tool_input fields (description, timeout, etc.)
    for key, val in tool_input.items():
        if key != "command":
            result["hookSpecificOutput"]["updatedInput"][key] = val

    json.dump(result, sys.stdout)


if __name__ == "__main__":
    main()
