#!/usr/bin/env python3
"""Gas City Role Parser — parses and validates YAML role definitions.

Prototype implementation of the Gas City declarative role format defined in
docs/gas-city/role-format-design.md. Parses Role and RoleBinding YAML files,
validates against the schema, resolves inheritance, and produces resolved
role objects.

Usage:
    python3 role_parser.py validate roles/builder.role.yaml
    python3 role_parser.py resolve roles/builder.role.yaml --base roles/base.role.yaml
    python3 role_parser.py bind roles/builder.role.yaml --binding bindings/default.binding.yaml
    python3 role_parser.py lint roles/          # Validate all files in directory
"""

from __future__ import annotations

import argparse
import copy
import json
import os
import sys
from dataclasses import dataclass, field
from pathlib import Path
from typing import Any

try:
    import yaml
except ImportError:
    print("ERROR: PyYAML required. Install with: pip install pyyaml", file=sys.stderr)
    sys.exit(1)

# ---------------------------------------------------------------------------
# Schema constants
# ---------------------------------------------------------------------------

API_VERSION = "gascity/v1"
VALID_KINDS = {"Role", "RoleBinding"}
VALID_PERSISTENCE = {"persistent", "persistent-identity", "ephemeral"}
VALID_STATES = {"idle", "working", "stalled", "zombie"}
VALID_ASSIGNMENT_METHODS = {"sling", "self-assign", "directed"}
VALID_COMPLETION_METHODS = {"gt_done", "manual", "auto"}
NAME_PATTERN = r"^[a-z][a-z0-9-]*$"

# ---------------------------------------------------------------------------
# Data structures
# ---------------------------------------------------------------------------

@dataclass
class ToolCapability:
    name: str
    actions: list[str] = field(default_factory=list)
    deny_actions: list[str] = field(default_factory=list)
    constraints: list[str] = field(default_factory=list)

@dataclass
class FilesystemCapability:
    read: list[str] = field(default_factory=list)
    write: list[str] = field(default_factory=list)
    exclude: list[str] = field(default_factory=list)

@dataclass
class NetworkCapability:
    allowed_hosts: list[str] = field(default_factory=list)
    deny_by_default: bool = True

@dataclass
class Capabilities:
    tools: list[ToolCapability] = field(default_factory=list)
    filesystem: FilesystemCapability | None = None
    network: NetworkCapability | None = None

@dataclass
class Identity:
    actor_format: str = ""
    git_author_format: str = ""

@dataclass
class Lifecycle:
    persistence: str = ""
    supervisor: str = ""
    states: list[str] = field(default_factory=list)
    resumable: bool = False
    auto_recycle: bool = False

@dataclass
class Communication:
    can_message: list[str] = field(default_factory=list)
    receives_from: list[str] = field(default_factory=list)
    channels: list[str] = field(default_factory=list)

@dataclass
class Constraints:
    rules: list[str] = field(default_factory=list)
    max_session_duration: str = ""
    max_cost_per_task: float = 0.0
    max_concurrent_sessions: int = 0

@dataclass
class WorkAssignment:
    method: str = ""
    source: str | None = None

@dataclass
class WorkCompletion:
    method: str = ""
    requires_mr: bool = False
    requires_tests: bool = False

@dataclass
class Work:
    assignment: WorkAssignment | None = None
    completion: WorkCompletion | None = None

@dataclass
class Context:
    prime_sections: list[str] = field(default_factory=list)
    env: dict[str, str] = field(default_factory=dict)

@dataclass
class RoleMetadata:
    name: str = ""
    description: str = ""
    labels: dict[str, str] = field(default_factory=dict)

@dataclass
class Role:
    api_version: str = API_VERSION
    kind: str = "Role"
    metadata: RoleMetadata = field(default_factory=RoleMetadata)
    extends: str = ""
    identity: Identity = field(default_factory=Identity)
    lifecycle: Lifecycle = field(default_factory=Lifecycle)
    capabilities: Capabilities = field(default_factory=Capabilities)
    constraints: Constraints = field(default_factory=Constraints)
    communication: Communication = field(default_factory=Communication)
    context: Context = field(default_factory=Context)
    work: Work = field(default_factory=Work)

@dataclass
class Binding:
    role: str = ""
    agent: str = ""
    priority: str = "default"
    overrides: dict[str, Any] = field(default_factory=dict)

@dataclass
class RoleBinding:
    api_version: str = API_VERSION
    kind: str = "RoleBinding"
    metadata: RoleMetadata = field(default_factory=RoleMetadata)
    bindings: list[Binding] = field(default_factory=list)

# ---------------------------------------------------------------------------
# Validation errors
# ---------------------------------------------------------------------------

@dataclass
class ValidationError:
    path: str
    message: str
    severity: str = "error"  # error | warning

    def __str__(self) -> str:
        return f"[{self.severity.upper()}] {self.path}: {self.message}"

class ValidationResult:
    def __init__(self) -> None:
        self.errors: list[ValidationError] = []

    def add(self, path: str, message: str, severity: str = "error") -> None:
        self.errors.append(ValidationError(path, message, severity))

    @property
    def ok(self) -> bool:
        return not any(e.severity == "error" for e in self.errors)

    @property
    def warnings(self) -> list[ValidationError]:
        return [e for e in self.errors if e.severity == "warning"]

    def __str__(self) -> str:
        if self.ok and not self.warnings:
            return "OK"
        return "\n".join(str(e) for e in self.errors)

# ---------------------------------------------------------------------------
# Parsing
# ---------------------------------------------------------------------------

def parse_tool(data: dict) -> ToolCapability:
    return ToolCapability(
        name=data.get("name", ""),
        actions=data.get("actions", []),
        deny_actions=data.get("deny_actions", []),
        constraints=data.get("constraints", []),
    )

def parse_capabilities(data: dict | None) -> Capabilities:
    if not data:
        return Capabilities()
    tools = [parse_tool(t) for t in data.get("tools", [])]
    fs = None
    if "filesystem" in data:
        fsd = data["filesystem"]
        fs = FilesystemCapability(
            read=fsd.get("read", []),
            write=fsd.get("write", []),
            exclude=fsd.get("exclude", []),
        )
    net = None
    if "network" in data:
        nd = data["network"]
        net = NetworkCapability(
            allowed_hosts=nd.get("allowed_hosts", []),
            deny_by_default=nd.get("deny_by_default", True),
        )
    return Capabilities(tools=tools, filesystem=fs, network=net)

def parse_constraints(data: Any) -> Constraints:
    """Parse constraints which can be a list of strings or a dict with mixed keys."""
    if not data:
        return Constraints()
    if isinstance(data, list):
        return Constraints(rules=data)
    if isinstance(data, dict):
        rules = []
        c = Constraints()
        for k, v in data.items():
            if k == "max_session_duration":
                c.max_session_duration = str(v)
            elif k == "max_cost_per_task":
                c.max_cost_per_task = float(v)
            elif k == "max_concurrent_sessions":
                c.max_concurrent_sessions = int(v)
            elif isinstance(v, str):
                rules.append(v)
            # skip unrecognized
        # Also handle list items mixed in (from YAML list + dict notation)
        c.rules = rules
        return c
    return Constraints()

def parse_role(data: dict) -> Role:
    """Parse a raw YAML dict into a Role object."""
    meta_data = data.get("metadata", {})
    identity_data = data.get("identity", {})
    lifecycle_data = data.get("lifecycle", {})
    comm_data = data.get("communication", {})
    ctx_data = data.get("context", {})
    work_data = data.get("work", {})

    role = Role(
        api_version=data.get("apiVersion", API_VERSION),
        kind=data.get("kind", "Role"),
        metadata=RoleMetadata(
            name=meta_data.get("name", ""),
            description=meta_data.get("description", ""),
            labels=meta_data.get("labels", {}),
        ),
        extends=data.get("extends", ""),
        identity=Identity(
            actor_format=identity_data.get("actor_format", ""),
            git_author_format=identity_data.get("git_author_format", ""),
        ),
        lifecycle=Lifecycle(
            persistence=lifecycle_data.get("persistence", ""),
            supervisor=lifecycle_data.get("supervisor", ""),
            states=lifecycle_data.get("states", []),
            resumable=lifecycle_data.get("resumable", False),
            auto_recycle=lifecycle_data.get("auto_recycle", False),
        ),
        capabilities=parse_capabilities(data.get("capabilities")),
        constraints=parse_constraints(data.get("constraints")),
        communication=Communication(
            can_message=comm_data.get("can_message", []),
            receives_from=comm_data.get("receives_from", []),
            channels=[
                c if isinstance(c, str) else c.get("type", "")
                for c in comm_data.get("channels", [])
            ],
        ),
        context=Context(
            prime_sections=ctx_data.get("prime_sections", []),
            env=ctx_data.get("env", {}),
        ),
        work=Work(
            assignment=WorkAssignment(
                method=work_data.get("assignment", {}).get("method", ""),
                source=work_data.get("assignment", {}).get("source"),
            ) if work_data.get("assignment") else None,
            completion=WorkCompletion(
                method=work_data.get("completion", {}).get("method", ""),
                requires_mr=work_data.get("completion", {}).get("requires_mr", False),
                requires_tests=work_data.get("completion", {}).get("requires_tests", False),
            ) if work_data.get("completion") else None,
        ),
    )
    return role

def parse_role_binding(data: dict) -> RoleBinding:
    """Parse a raw YAML dict into a RoleBinding object."""
    meta_data = data.get("metadata", {})
    bindings = []
    for bd in data.get("bindings", []):
        bindings.append(Binding(
            role=bd.get("role", ""),
            agent=bd.get("agent", ""),
            priority=bd.get("priority", "default"),
            overrides=bd.get("overrides", {}),
        ))
    return RoleBinding(
        api_version=data.get("apiVersion", API_VERSION),
        kind=data.get("kind", "RoleBinding"),
        metadata=RoleMetadata(
            name=meta_data.get("name", ""),
            description=meta_data.get("description", ""),
            labels=meta_data.get("labels", {}),
        ),
        bindings=bindings,
    )

def load_yaml_file(path: str) -> list[dict]:
    """Load one or more YAML documents from a file."""
    with open(path) as f:
        docs = list(yaml.safe_load_all(f))
    return [d for d in docs if d is not None]

def parse_file(path: str) -> list[Role | RoleBinding]:
    """Parse a YAML file into Role and/or RoleBinding objects."""
    docs = load_yaml_file(path)
    results = []
    for doc in docs:
        kind = doc.get("kind", "Role")
        if kind == "Role":
            results.append(parse_role(doc))
        elif kind == "RoleBinding":
            results.append(parse_role_binding(doc))
        else:
            raise ValueError(f"Unknown kind: {kind}")
    return results

# ---------------------------------------------------------------------------
# Validation
# ---------------------------------------------------------------------------

import re

def validate_role(role: Role) -> ValidationResult:
    """Validate a parsed Role against the Gas City schema."""
    result = ValidationResult()

    # apiVersion
    if role.api_version != API_VERSION:
        result.add("apiVersion", f"expected '{API_VERSION}', got '{role.api_version}'")

    # kind
    if role.kind != "Role":
        result.add("kind", f"expected 'Role', got '{role.kind}'")

    # metadata.name
    if not role.metadata.name:
        result.add("metadata.name", "required field is empty")
    elif not re.match(NAME_PATTERN, role.metadata.name):
        result.add("metadata.name", f"must match pattern {NAME_PATTERN}")

    # Abstract roles don't need full validation
    is_abstract = role.metadata.labels.get("abstract") == "true"

    # lifecycle.persistence
    if role.lifecycle.persistence and role.lifecycle.persistence not in VALID_PERSISTENCE:
        result.add("lifecycle.persistence", f"must be one of {VALID_PERSISTENCE}")

    # lifecycle.states
    for state in role.lifecycle.states:
        if state not in VALID_STATES:
            result.add("lifecycle.states", f"invalid state '{state}', must be one of {VALID_STATES}")

    # capabilities.tools
    for i, tool in enumerate(role.capabilities.tools):
        if not tool.name:
            result.add(f"capabilities.tools[{i}].name", "required field is empty")
        # Check deny_actions don't overlap with actions
        overlap = set(tool.actions) & set(tool.deny_actions)
        if overlap:
            result.add(
                f"capabilities.tools[{i}]",
                f"actions and deny_actions overlap: {overlap}",
                severity="warning",
            )

    # work.assignment.method
    if role.work.assignment and role.work.assignment.method:
        if role.work.assignment.method not in VALID_ASSIGNMENT_METHODS:
            result.add("work.assignment.method", f"must be one of {VALID_ASSIGNMENT_METHODS}")

    # work.completion.method
    if role.work.completion and role.work.completion.method:
        if role.work.completion.method not in VALID_COMPLETION_METHODS:
            result.add("work.completion.method", f"must be one of {VALID_COMPLETION_METHODS}")

    # identity format placeholders
    if role.identity.actor_format and not is_abstract:
        if "{rig}" not in role.identity.actor_format and "{name}" not in role.identity.actor_format:
            result.add(
                "identity.actor_format",
                "should contain {rig} or {name} placeholders",
                severity="warning",
            )

    # constraints sanity
    if role.constraints.max_cost_per_task < 0:
        result.add("constraints.max_cost_per_task", "must be non-negative")
    if role.constraints.max_concurrent_sessions < 0:
        result.add("constraints.max_concurrent_sessions", "must be non-negative")

    return result

def validate_binding(binding: RoleBinding) -> ValidationResult:
    """Validate a parsed RoleBinding."""
    result = ValidationResult()

    if binding.api_version != API_VERSION:
        result.add("apiVersion", f"expected '{API_VERSION}', got '{binding.api_version}'")
    if binding.kind != "RoleBinding":
        result.add("kind", f"expected 'RoleBinding', got '{binding.kind}'")
    if not binding.metadata.name:
        result.add("metadata.name", "required field is empty")

    for i, b in enumerate(binding.bindings):
        if not b.role:
            result.add(f"bindings[{i}].role", "required field is empty")
        if not b.agent:
            result.add(f"bindings[{i}].agent", "required field is empty")

    # Check for duplicate role bindings
    roles_seen = {}
    for i, b in enumerate(binding.bindings):
        if b.role in roles_seen:
            result.add(
                f"bindings[{i}].role",
                f"duplicate binding for role '{b.role}' (first at index {roles_seen[b.role]})",
                severity="warning",
            )
        roles_seen[b.role] = i

    return result

# ---------------------------------------------------------------------------
# Inheritance resolution
# ---------------------------------------------------------------------------

def _merge_tool_lists(parent_tools: list[ToolCapability], child_tools: list[ToolCapability]) -> list[ToolCapability]:
    """Merge tool capability lists: child adds to or overrides parent tools."""
    tools_by_name: dict[str, ToolCapability] = {}
    for t in parent_tools:
        tools_by_name[t.name] = copy.deepcopy(t)
    for t in child_tools:
        if t.name in tools_by_name:
            existing = tools_by_name[t.name]
            # Merge actions (union)
            merged_actions = list(set(existing.actions) | set(t.actions))
            # Apply deny_actions
            denied = set(t.deny_actions)
            merged_actions = [a for a in merged_actions if a not in denied]
            existing.actions = sorted(merged_actions)
            existing.deny_actions = t.deny_actions
            # Merge constraints
            existing.constraints = list(set(existing.constraints) | set(t.constraints))
        else:
            tools_by_name[t.name] = copy.deepcopy(t)
    return list(tools_by_name.values())

def resolve_inheritance(child: Role, parent: Role) -> Role:
    """Resolve a child role's inheritance from a parent role.

    Rules:
    - Scalar fields: child overrides parent (if set)
    - List fields: child merges with parent (union)
    - Nested objects: deep merge
    - deny_actions: removes inherited actions
    """
    resolved = copy.deepcopy(parent)

    # Metadata: child always wins
    resolved.metadata = copy.deepcopy(child.metadata)
    resolved.api_version = child.api_version
    resolved.kind = child.kind
    resolved.extends = child.extends

    # Identity: child overrides if set
    if child.identity.actor_format:
        resolved.identity.actor_format = child.identity.actor_format
    if child.identity.git_author_format:
        resolved.identity.git_author_format = child.identity.git_author_format

    # Lifecycle: child scalars override; lists merge
    if child.lifecycle.persistence:
        resolved.lifecycle.persistence = child.lifecycle.persistence
    if child.lifecycle.supervisor:
        resolved.lifecycle.supervisor = child.lifecycle.supervisor
    if child.lifecycle.states:
        resolved.lifecycle.states = child.lifecycle.states  # Override, not merge (states define a set)
    if child.lifecycle.resumable:
        resolved.lifecycle.resumable = child.lifecycle.resumable
    if child.lifecycle.auto_recycle:
        resolved.lifecycle.auto_recycle = child.lifecycle.auto_recycle

    # Capabilities: merge tools, override filesystem/network
    resolved.capabilities.tools = _merge_tool_lists(
        resolved.capabilities.tools, child.capabilities.tools
    )
    if child.capabilities.filesystem is not None:
        resolved.capabilities.filesystem = copy.deepcopy(child.capabilities.filesystem)
    if child.capabilities.network is not None:
        resolved.capabilities.network = copy.deepcopy(child.capabilities.network)

    # Constraints: merge rules, override limits
    resolved.constraints.rules = list(
        set(resolved.constraints.rules) | set(child.constraints.rules)
    )
    if child.constraints.max_session_duration:
        resolved.constraints.max_session_duration = child.constraints.max_session_duration
    if child.constraints.max_cost_per_task:
        resolved.constraints.max_cost_per_task = child.constraints.max_cost_per_task
    if child.constraints.max_concurrent_sessions:
        resolved.constraints.max_concurrent_sessions = child.constraints.max_concurrent_sessions

    # Communication: merge lists
    resolved.communication.can_message = sorted(
        set(resolved.communication.can_message) | set(child.communication.can_message)
    )
    resolved.communication.receives_from = sorted(
        set(resolved.communication.receives_from) | set(child.communication.receives_from)
    )
    resolved.communication.channels = sorted(
        set(resolved.communication.channels) | set(child.communication.channels)
    )

    # Context: merge
    resolved.context.prime_sections = list(
        dict.fromkeys(resolved.context.prime_sections + child.context.prime_sections)
    )
    resolved.context.env.update(child.context.env)

    # Work: child overrides
    if child.work.assignment:
        resolved.work.assignment = copy.deepcopy(child.work.assignment)
    if child.work.completion:
        resolved.work.completion = copy.deepcopy(child.work.completion)

    return resolved

def resolve_role_chain(roles: dict[str, Role], name: str, _seen: set[str] | None = None) -> Role:
    """Resolve full inheritance chain for a named role."""
    if _seen is None:
        _seen = set()
    if name in _seen:
        raise ValueError(f"circular inheritance detected: {name}")
    _seen.add(name)

    role = roles[name]
    if not role.extends:
        return copy.deepcopy(role)

    if role.extends not in roles:
        raise ValueError(f"role '{name}' extends unknown role '{role.extends}'")

    parent = resolve_role_chain(roles, role.extends, _seen)
    return resolve_inheritance(role, parent)

# ---------------------------------------------------------------------------
# Serialization
# ---------------------------------------------------------------------------

def role_to_dict(role: Role) -> dict:
    """Convert a Role to a dict suitable for YAML/JSON output."""
    d: dict[str, Any] = {
        "apiVersion": role.api_version,
        "kind": role.kind,
        "metadata": {
            "name": role.metadata.name,
            "description": role.metadata.description,
        },
    }
    if role.metadata.labels:
        d["metadata"]["labels"] = role.metadata.labels
    if role.extends:
        d["extends"] = role.extends
    if role.identity.actor_format or role.identity.git_author_format:
        d["identity"] = {}
        if role.identity.actor_format:
            d["identity"]["actor_format"] = role.identity.actor_format
        if role.identity.git_author_format:
            d["identity"]["git_author_format"] = role.identity.git_author_format
    if role.lifecycle.persistence:
        d["lifecycle"] = {
            "persistence": role.lifecycle.persistence,
            "supervisor": role.lifecycle.supervisor,
            "states": role.lifecycle.states,
            "resumable": role.lifecycle.resumable,
            "auto_recycle": role.lifecycle.auto_recycle,
        }
    if role.capabilities.tools:
        d["capabilities"] = {"tools": []}
        for t in role.capabilities.tools:
            td: dict[str, Any] = {"name": t.name}
            if t.actions:
                td["actions"] = t.actions
            if t.deny_actions:
                td["deny_actions"] = t.deny_actions
            if t.constraints:
                td["constraints"] = t.constraints
            d["capabilities"]["tools"].append(td)
        if role.capabilities.filesystem:
            d["capabilities"]["filesystem"] = {
                "read": role.capabilities.filesystem.read,
                "write": role.capabilities.filesystem.write,
                "exclude": role.capabilities.filesystem.exclude,
            }
        if role.capabilities.network:
            d["capabilities"]["network"] = {
                "allowed_hosts": role.capabilities.network.allowed_hosts,
                "deny_by_default": role.capabilities.network.deny_by_default,
            }
    if role.communication.can_message or role.communication.receives_from:
        d["communication"] = {
            "can_message": role.communication.can_message,
            "receives_from": role.communication.receives_from,
            "channels": role.communication.channels,
        }
    return d

# ---------------------------------------------------------------------------
# CLI commands
# ---------------------------------------------------------------------------

def cmd_validate(args: argparse.Namespace) -> int:
    """Validate one or more role/binding files."""
    paths = _expand_paths(args.files)
    exit_code = 0
    for path in paths:
        try:
            objects = parse_file(path)
            for obj in objects:
                if isinstance(obj, Role):
                    result = validate_role(obj)
                elif isinstance(obj, RoleBinding):
                    result = validate_binding(obj)
                else:
                    continue
                name = obj.metadata.name or "(unnamed)"
                if result.ok:
                    print(f"  PASS  {path} ({name})")
                    for w in result.warnings:
                        print(f"        {w}")
                else:
                    print(f"  FAIL  {path} ({name})")
                    for e in result.errors:
                        print(f"        {e}")
                    exit_code = 1
        except Exception as e:
            print(f"  ERROR {path}: {e}")
            exit_code = 1
    return exit_code

def cmd_resolve(args: argparse.Namespace) -> int:
    """Resolve inheritance and print the resolved role."""
    all_roles: dict[str, Role] = {}

    # Load base files first
    for base_path in (args.base or []):
        for obj in parse_file(base_path):
            if isinstance(obj, Role):
                all_roles[obj.metadata.name] = obj

    # Load target file
    target_name = None
    for obj in parse_file(args.file):
        if isinstance(obj, Role):
            all_roles[obj.metadata.name] = obj
            target_name = obj.metadata.name

    if not target_name:
        print("ERROR: no Role found in input file", file=sys.stderr)
        return 1

    try:
        resolved = resolve_role_chain(all_roles, target_name)
        output = role_to_dict(resolved)
        if args.json:
            print(json.dumps(output, indent=2))
        else:
            print(yaml.dump(output, default_flow_style=False, sort_keys=False))
    except ValueError as e:
        print(f"ERROR: {e}", file=sys.stderr)
        return 1
    return 0

def cmd_bind(args: argparse.Namespace) -> int:
    """Show binding for a role."""
    role_objs = parse_file(args.file)
    binding_objs = parse_file(args.binding)

    role = None
    for obj in role_objs:
        if isinstance(obj, Role):
            role = obj
            break

    binding = None
    for obj in binding_objs:
        if isinstance(obj, RoleBinding):
            binding = obj
            break

    if not role:
        print("ERROR: no Role found in input file", file=sys.stderr)
        return 1
    if not binding:
        print("ERROR: no RoleBinding found in binding file", file=sys.stderr)
        return 1

    matched = None
    for b in binding.bindings:
        if b.role == role.metadata.name:
            matched = b
            break

    if not matched:
        print(f"No binding found for role '{role.metadata.name}'")
        return 1

    print(f"Role:     {matched.role}")
    print(f"Agent:    {matched.agent}")
    print(f"Priority: {matched.priority}")
    if matched.overrides:
        print(f"Overrides: {json.dumps(matched.overrides, indent=2)}")
    return 0

def cmd_lint(args: argparse.Namespace) -> int:
    """Lint all YAML files in a directory."""
    paths = _expand_paths(args.files)
    if not paths:
        print("No .yaml/.yml files found")
        return 0
    # Delegate to validate
    args.files = paths
    return cmd_validate(args)

def _expand_paths(paths: list[str]) -> list[str]:
    """Expand directories to individual YAML files."""
    expanded = []
    for p in paths:
        path = Path(p)
        if path.is_dir():
            expanded.extend(str(f) for f in path.rglob("*.yaml"))
            expanded.extend(str(f) for f in path.rglob("*.yml"))
        elif path.exists():
            expanded.append(str(path))
        else:
            expanded.append(str(path))  # Let parse_file handle the error
    return sorted(expanded)

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main() -> int:
    parser = argparse.ArgumentParser(
        description="Gas City Role Parser — parse and validate YAML role definitions"
    )
    sub = parser.add_subparsers(dest="command", required=True)

    # validate
    p_validate = sub.add_parser("validate", help="Validate role/binding files")
    p_validate.add_argument("files", nargs="+", help="YAML files or directories")

    # resolve
    p_resolve = sub.add_parser("resolve", help="Resolve inheritance and print result")
    p_resolve.add_argument("file", help="Role YAML file")
    p_resolve.add_argument("--base", nargs="*", help="Base role files to load")
    p_resolve.add_argument("--json", action="store_true", help="Output as JSON")

    # bind
    p_bind = sub.add_parser("bind", help="Show binding for a role")
    p_bind.add_argument("file", help="Role YAML file")
    p_bind.add_argument("--binding", required=True, help="Binding YAML file")

    # lint
    p_lint = sub.add_parser("lint", help="Validate all YAML files in directory")
    p_lint.add_argument("files", nargs="+", help="Directories or files to lint")

    args = parser.parse_args()

    commands = {
        "validate": cmd_validate,
        "resolve": cmd_resolve,
        "bind": cmd_bind,
        "lint": cmd_lint,
    }
    return commands[args.command](args)

if __name__ == "__main__":
    sys.exit(main())
