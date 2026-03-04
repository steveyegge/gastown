#!/usr/bin/env python3
"""Tests for Gas City role parser."""

import json
import os
import tempfile
import textwrap
import unittest

import role_parser as rp

# ---------------------------------------------------------------------------
# Test fixtures
# ---------------------------------------------------------------------------

BUILDER_ROLE_YAML = textwrap.dedent("""\
    apiVersion: gascity/v1
    kind: Role
    metadata:
      name: builder
      description: "Implements features and fixes bugs"
      labels:
        family: worker
        scope: rig
    extends: worker-base
    identity:
      actor_format: "{rig}/polecats/{name}"
      git_author_format: "{rig}/polecats/{name}"
    lifecycle:
      persistence: persistent-identity
      supervisor: witness
      states: [idle, working, stalled, zombie]
      resumable: true
      auto_recycle: true
    capabilities:
      tools:
        - name: git
          actions: [commit, push, branch, checkout, rebase, diff, log, status]
          constraints:
            - "push only to feature branches"
        - name: gt
          actions: [prime, mail, done, costs]
        - name: bd
          actions: [show, update, close, sync]
      filesystem:
        read: ["**/*"]
        write: ["src/**", "tests/**", "docs/**"]
        exclude: [".env", "*.secret"]
    constraints:
      max_session_duration: 4h
      max_cost_per_task: 5.0
    communication:
      can_message: [witness, refinery]
      receives_from: [witness, crew, mayor]
      channels: [nudge, mail, hook]
    work:
      assignment:
        method: sling
        source: witness
      completion:
        method: gt_done
        requires_mr: true
""")

WORKER_BASE_YAML = textwrap.dedent("""\
    apiVersion: gascity/v1
    kind: Role
    metadata:
      name: worker-base
      description: "Abstract base for all worker roles"
      labels:
        family: worker
        abstract: "true"
    lifecycle:
      persistence: persistent-identity
      states: [idle, working, stalled, zombie]
      resumable: true
    capabilities:
      tools:
        - name: gt
          actions: [prime, mail, costs]
        - name: bd
          actions: [show]
    communication:
      receives_from: [witness, mayor]
      channels: [nudge, mail, hook]
    context:
      prime_sections:
        - role_instructions
        - current_assignment
        - project_context
        - recent_mail
      env:
        GT_ROLE: "{identity.actor_format}"
        GT_RIG: "{rig}"
        BD_ACTOR: "{identity.actor_format}"
""")

REVIEWER_ROLE_YAML = textwrap.dedent("""\
    apiVersion: gascity/v1
    kind: Role
    metadata:
      name: reviewer
      description: "Reviews code, provides feedback"
      labels:
        family: worker
        scope: rig
    extends: worker-base
    identity:
      actor_format: "{rig}/reviewers/{name}"
      git_author_format: "{rig}/reviewers/{name}"
    lifecycle:
      persistence: persistent-identity
      supervisor: witness
      states: [idle, working]
      resumable: true
      auto_recycle: true
    capabilities:
      tools:
        - name: git
          actions: [diff, log, show, status, checkout]
        - name: gh
          actions: [pr-review, pr-comment]
      filesystem:
        read: ["**/*"]
        write: []
    constraints:
      max_session_duration: 2h
      max_cost_per_task: 2.0
    communication:
      can_message: [witness, refinery, builder]
      receives_from: [witness, refinery, mayor]
      channels: [nudge, mail]
    work:
      assignment:
        method: directed
        source: refinery
      completion:
        method: gt_done
        requires_mr: false
""")

BINDING_YAML = textwrap.dedent("""\
    apiVersion: gascity/v1
    kind: RoleBinding
    metadata:
      name: default
      description: "Default role-to-agent bindings"
    bindings:
      - role: builder
        agent: claude
        priority: default
      - role: reviewer
        agent: claude
        priority: default
        overrides:
          capabilities:
            tools:
              - name: gh
                actions: [pr-review, pr-comment, pr-view]
""")

INVALID_ROLE_YAML = textwrap.dedent("""\
    apiVersion: gascity/v2
    kind: Role
    metadata:
      name: BAD-NAME
    lifecycle:
      persistence: invalid-type
      states: [idle, flying]
    work:
      assignment:
        method: teleport
      completion:
        method: magic
    constraints:
      max_cost_per_task: -5.0
""")

MULTI_DOC_YAML = textwrap.dedent("""\
    apiVersion: gascity/v1
    kind: Role
    metadata:
      name: role-a
      description: "First role"
    lifecycle:
      persistence: ephemeral
    ---
    apiVersion: gascity/v1
    kind: Role
    metadata:
      name: role-b
      description: "Second role"
    lifecycle:
      persistence: persistent
""")

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

def _write_temp(content: str, suffix: str = ".yaml") -> str:
    fd, path = tempfile.mkstemp(suffix=suffix)
    os.write(fd, content.encode())
    os.close(fd)
    return path

# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------

class TestParsing(unittest.TestCase):
    def test_parse_builder_role(self):
        path = _write_temp(BUILDER_ROLE_YAML)
        try:
            objs = rp.parse_file(path)
            self.assertEqual(len(objs), 1)
            role = objs[0]
            self.assertIsInstance(role, rp.Role)
            self.assertEqual(role.metadata.name, "builder")
            self.assertEqual(role.extends, "worker-base")
            self.assertEqual(role.lifecycle.persistence, "persistent-identity")
            self.assertEqual(len(role.capabilities.tools), 3)
            self.assertEqual(role.capabilities.tools[0].name, "git")
            self.assertIn("commit", role.capabilities.tools[0].actions)
            self.assertEqual(role.capabilities.filesystem.write, ["src/**", "tests/**", "docs/**"])
        finally:
            os.unlink(path)

    def test_parse_binding(self):
        path = _write_temp(BINDING_YAML)
        try:
            objs = rp.parse_file(path)
            self.assertEqual(len(objs), 1)
            binding = objs[0]
            self.assertIsInstance(binding, rp.RoleBinding)
            self.assertEqual(binding.metadata.name, "default")
            self.assertEqual(len(binding.bindings), 2)
            self.assertEqual(binding.bindings[0].role, "builder")
            self.assertEqual(binding.bindings[0].agent, "claude")
        finally:
            os.unlink(path)

    def test_parse_multi_document(self):
        path = _write_temp(MULTI_DOC_YAML)
        try:
            objs = rp.parse_file(path)
            self.assertEqual(len(objs), 2)
            self.assertEqual(objs[0].metadata.name, "role-a")
            self.assertEqual(objs[1].metadata.name, "role-b")
        finally:
            os.unlink(path)

    def test_parse_constraints_as_dict(self):
        path = _write_temp(BUILDER_ROLE_YAML)
        try:
            objs = rp.parse_file(path)
            role = objs[0]
            self.assertEqual(role.constraints.max_cost_per_task, 5.0)
            self.assertEqual(role.constraints.max_session_duration, "4h")
        finally:
            os.unlink(path)


class TestValidation(unittest.TestCase):
    def test_valid_builder(self):
        path = _write_temp(BUILDER_ROLE_YAML)
        try:
            role = rp.parse_file(path)[0]
            result = rp.validate_role(role)
            self.assertTrue(result.ok, str(result))
        finally:
            os.unlink(path)

    def test_valid_binding(self):
        path = _write_temp(BINDING_YAML)
        try:
            binding = rp.parse_file(path)[0]
            result = rp.validate_binding(binding)
            self.assertTrue(result.ok, str(result))
        finally:
            os.unlink(path)

    def test_invalid_role(self):
        path = _write_temp(INVALID_ROLE_YAML)
        try:
            role = rp.parse_file(path)[0]
            result = rp.validate_role(role)
            self.assertFalse(result.ok)
            # Check specific errors
            error_paths = [e.path for e in result.errors]
            self.assertIn("apiVersion", error_paths, "should catch bad apiVersion")
            self.assertIn("metadata.name", error_paths, "should catch bad name")
            self.assertIn("lifecycle.persistence", error_paths, "should catch bad persistence")
            self.assertIn("lifecycle.states", error_paths, "should catch bad state")
            self.assertIn("work.assignment.method", error_paths, "should catch bad assignment method")
            self.assertIn("work.completion.method", error_paths, "should catch bad completion method")
            self.assertIn("constraints.max_cost_per_task", error_paths, "should catch negative cost")
        finally:
            os.unlink(path)

    def test_abstract_role_skips_identity_warning(self):
        path = _write_temp(WORKER_BASE_YAML)
        try:
            role = rp.parse_file(path)[0]
            result = rp.validate_role(role)
            self.assertTrue(result.ok, str(result))
            # Abstract roles should not warn about missing identity placeholders
            identity_warnings = [e for e in result.warnings if "actor_format" in e.path]
            self.assertEqual(len(identity_warnings), 0)
        finally:
            os.unlink(path)

    def test_empty_binding_role(self):
        yaml_content = textwrap.dedent("""\
            apiVersion: gascity/v1
            kind: RoleBinding
            metadata:
              name: bad-binding
            bindings:
              - role: ""
                agent: claude
        """)
        path = _write_temp(yaml_content)
        try:
            binding = rp.parse_file(path)[0]
            result = rp.validate_binding(binding)
            self.assertFalse(result.ok)
        finally:
            os.unlink(path)


class TestInheritance(unittest.TestCase):
    def test_resolve_builder_from_worker_base(self):
        base_path = _write_temp(WORKER_BASE_YAML)
        builder_path = _write_temp(BUILDER_ROLE_YAML)
        try:
            base_roles = rp.parse_file(base_path)
            builder_roles = rp.parse_file(builder_path)

            all_roles = {}
            for obj in base_roles + builder_roles:
                if isinstance(obj, rp.Role):
                    all_roles[obj.metadata.name] = obj

            resolved = rp.resolve_role_chain(all_roles, "builder")

            # Should have builder's metadata
            self.assertEqual(resolved.metadata.name, "builder")

            # Should have merged tools (gt from both, bd from both, git from builder)
            tool_names = {t.name for t in resolved.capabilities.tools}
            self.assertIn("git", tool_names)
            self.assertIn("gt", tool_names)
            self.assertIn("bd", tool_names)

            # gt actions should be merged (parent: prime, mail, costs + child: prime, mail, done, costs)
            gt_tool = next(t for t in resolved.capabilities.tools if t.name == "gt")
            self.assertIn("prime", gt_tool.actions)
            self.assertIn("done", gt_tool.actions)  # From child
            self.assertIn("costs", gt_tool.actions)

            # bd actions should be merged (parent: show + child: show, update, close, sync)
            bd_tool = next(t for t in resolved.capabilities.tools if t.name == "bd")
            self.assertIn("show", bd_tool.actions)
            self.assertIn("update", bd_tool.actions)
            self.assertIn("close", bd_tool.actions)
            self.assertIn("sync", bd_tool.actions)

            # Should inherit context from parent
            self.assertIn("role_instructions", resolved.context.prime_sections)
            self.assertIn("GT_ROLE", resolved.context.env)

            # Communication should be merged
            self.assertIn("witness", resolved.communication.receives_from)
            self.assertIn("mayor", resolved.communication.receives_from)
            self.assertIn("crew", resolved.communication.receives_from)  # From child

        finally:
            os.unlink(base_path)
            os.unlink(builder_path)

    def test_resolve_reviewer_restricts_filesystem(self):
        base_path = _write_temp(WORKER_BASE_YAML)
        reviewer_path = _write_temp(REVIEWER_ROLE_YAML)
        try:
            all_roles = {}
            for obj in rp.parse_file(base_path) + rp.parse_file(reviewer_path):
                if isinstance(obj, rp.Role):
                    all_roles[obj.metadata.name] = obj

            resolved = rp.resolve_role_chain(all_roles, "reviewer")

            # Reviewer should have empty write list (restrictive override)
            self.assertEqual(resolved.capabilities.filesystem.write, [])

            # Should have reviewer's states (override, not merge)
            self.assertEqual(resolved.lifecycle.states, ["idle", "working"])

        finally:
            os.unlink(base_path)
            os.unlink(reviewer_path)

    def test_circular_inheritance_detected(self):
        yaml_a = textwrap.dedent("""\
            apiVersion: gascity/v1
            kind: Role
            metadata:
              name: role-a
            extends: role-b
        """)
        yaml_b = textwrap.dedent("""\
            apiVersion: gascity/v1
            kind: Role
            metadata:
              name: role-b
            extends: role-a
        """)
        path_a = _write_temp(yaml_a)
        path_b = _write_temp(yaml_b)
        try:
            all_roles = {}
            for obj in rp.parse_file(path_a) + rp.parse_file(path_b):
                if isinstance(obj, rp.Role):
                    all_roles[obj.metadata.name] = obj

            with self.assertRaises(ValueError) as ctx:
                rp.resolve_role_chain(all_roles, "role-a")
            self.assertIn("circular", str(ctx.exception))
        finally:
            os.unlink(path_a)
            os.unlink(path_b)

    def test_unknown_parent_detected(self):
        yaml_content = textwrap.dedent("""\
            apiVersion: gascity/v1
            kind: Role
            metadata:
              name: orphan
            extends: nonexistent-parent
        """)
        path = _write_temp(yaml_content)
        try:
            all_roles = {}
            for obj in rp.parse_file(path):
                if isinstance(obj, rp.Role):
                    all_roles[obj.metadata.name] = obj

            with self.assertRaises(ValueError) as ctx:
                rp.resolve_role_chain(all_roles, "orphan")
            self.assertIn("nonexistent-parent", str(ctx.exception))
        finally:
            os.unlink(path)

    def test_deny_actions_remove_inherited(self):
        parent_yaml = textwrap.dedent("""\
            apiVersion: gascity/v1
            kind: Role
            metadata:
              name: parent-role
            capabilities:
              tools:
                - name: git
                  actions: [commit, push, rebase, diff, log]
        """)
        child_yaml = textwrap.dedent("""\
            apiVersion: gascity/v1
            kind: Role
            metadata:
              name: read-only-child
            extends: parent-role
            capabilities:
              tools:
                - name: git
                  actions: [diff, log, show]
                  deny_actions: [commit, push, rebase]
        """)
        parent_path = _write_temp(parent_yaml)
        child_path = _write_temp(child_yaml)
        try:
            all_roles = {}
            for obj in rp.parse_file(parent_path) + rp.parse_file(child_path):
                if isinstance(obj, rp.Role):
                    all_roles[obj.metadata.name] = obj

            resolved = rp.resolve_role_chain(all_roles, "read-only-child")
            git_tool = next(t for t in resolved.capabilities.tools if t.name == "git")

            # Should have diff, log, show but NOT commit, push, rebase
            self.assertIn("diff", git_tool.actions)
            self.assertIn("log", git_tool.actions)
            self.assertIn("show", git_tool.actions)
            self.assertNotIn("commit", git_tool.actions)
            self.assertNotIn("push", git_tool.actions)
            self.assertNotIn("rebase", git_tool.actions)
        finally:
            os.unlink(parent_path)
            os.unlink(child_path)


class TestSerialization(unittest.TestCase):
    def test_round_trip(self):
        path = _write_temp(BUILDER_ROLE_YAML)
        try:
            role = rp.parse_file(path)[0]
            d = rp.role_to_dict(role)
            self.assertEqual(d["apiVersion"], "gascity/v1")
            self.assertEqual(d["metadata"]["name"], "builder")
            self.assertEqual(d["extends"], "worker-base")
            self.assertIn("capabilities", d)
            self.assertEqual(len(d["capabilities"]["tools"]), 3)
        finally:
            os.unlink(path)

    def test_json_output(self):
        path = _write_temp(BUILDER_ROLE_YAML)
        try:
            role = rp.parse_file(path)[0]
            d = rp.role_to_dict(role)
            json_str = json.dumps(d, indent=2)
            parsed = json.loads(json_str)
            self.assertEqual(parsed["metadata"]["name"], "builder")
        finally:
            os.unlink(path)


if __name__ == "__main__":
    unittest.main()
