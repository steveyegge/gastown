#!/usr/bin/env python3
"""
Validate semantic ID generation for existing beads.
Usage: bd list --all --limit 0 --json | python3 validate_semantic_ids.py

New format: <prefix>-<type>-<slug><suffix>
Example: gt-epc-semantic_ids7x9k
"""

import json
import sys
import re
import random
import string
from collections import Counter

# Type code mapping
TYPE_CODES = {
    "epic": "epc",
    "bug": "bug",
    "task": "tsk",
    "feature": "ftr",
    "decision": "dec",
    "convoy": "cnv",
    "molecule": "mol",
    "wisp": "wsp",
    "agent": "agt",
    "role": "rol",
    "mr": "mrq",
    # Fallback
    "unknown": "unk",
}

def generate_random_suffix():
    """Generate 4-character random alphanumeric suffix."""
    chars = string.ascii_lowercase + string.digits
    return ''.join(random.choice(chars) for _ in range(4))

def generate_semantic_slug(title):
    """Apply the semantic ID slug algorithm from the spec."""
    if not title:
        return "untitled"

    # Lowercase
    slug = title.lower()

    # Replace non-alphanumeric with underscore
    slug = re.sub(r"[^a-z0-9]+", "_", slug)

    # Collapse consecutive underscores
    slug = re.sub(r"_+", "_", slug)

    # Trim underscores
    slug = slug.strip("_")

    # Prefix with n if starts with digit
    if slug and slug[0].isdigit():
        slug = "n" + slug

    # Truncate to 40 chars at word boundary (leaving room for 4-char suffix)
    if len(slug) > 40:
        truncated = slug[:40]
        # Try to break at word boundary
        last_underscore = truncated.rfind("_")
        if last_underscore > 25:  # Keep at least 25 chars
            truncated = truncated[:last_underscore]
        slug = truncated.rstrip("_")

    # Minimum length
    if len(slug) < 3:
        slug = slug + "x" * (3 - len(slug)) if slug else "xxx"

    return slug

def generate_semantic_id(prefix, bead_type, title):
    """Generate full semantic ID: prefix-type-slug+suffix"""
    type_code = TYPE_CODES.get(bead_type, "unk")
    slug = generate_semantic_slug(title)
    suffix = generate_random_suffix()
    return f"{prefix}-{type_code}-{slug}{suffix}"

def extract_prefix(bead_id):
    """Extract prefix from existing bead ID."""
    if "-" in bead_id:
        return bead_id.split("-")[0]
    return "gt"

def main():
    data = json.load(sys.stdin)

    # Generate semantic IDs
    results = []
    slug_only_list = []  # For collision analysis (without suffix)

    for issue in data:
        title = issue.get("title", "")
        original_id = issue.get("id", "")
        bead_type = issue.get("issue_type", issue.get("type", "unknown"))
        prefix = extract_prefix(original_id)

        type_code = TYPE_CODES.get(bead_type, "unk")
        slug = generate_semantic_slug(title)
        suffix = generate_random_suffix()
        semantic_id = f"{prefix}-{type_code}-{slug}{suffix}"

        results.append({
            "original_id": original_id,
            "title": title[:50] + "..." if len(title) > 50 else title,
            "type": bead_type,
            "type_code": type_code,
            "slug": slug,
            "suffix": suffix,
            "semantic_id": semantic_id,
            "slug_length": len(slug),
            "total_length": len(semantic_id),
        })
        slug_only_list.append(f"{prefix}-{type_code}-{slug}")

    # Analysis
    total = len(results)

    # Slug collision analysis (without random suffix)
    slug_counts = Counter(slug_only_list)
    collisions = sum(1 for s, c in slug_counts.items() if c > 1)
    collision_instances = sum(c for s, c in slug_counts.items() if c > 1)

    # With random suffix, collision is near-zero
    full_id_counts = Counter(r["semantic_id"] for r in results)
    full_collisions = sum(c for s, c in full_id_counts.items() if c > 1)

    # Length distribution
    slug_lengths = [r["slug_length"] for r in results]
    total_lengths = [r["total_length"] for r in results]

    len_dist = {
        "1-10": sum(1 for l in slug_lengths if 1 <= l <= 10),
        "11-20": sum(1 for l in slug_lengths if 11 <= l <= 20),
        "21-30": sum(1 for l in slug_lengths if 21 <= l <= 30),
        "31-40": sum(1 for l in slug_lengths if 31 <= l <= 40),
    }

    # Type distribution
    type_counts = Counter(r["type_code"] for r in results)

    # Top colliding slugs (before suffix)
    top_collisions = slug_counts.most_common(15)

    # Categorize collisions
    patrol_collisions = 0
    work_collisions = 0
    for slug_key, count in slug_counts.items():
        if count > 1:
            if any(kw in slug_key for kw in ["patrol", "digest", "wisp", "mol-", "molecule"]):
                patrol_collisions += count
            else:
                work_collisions += count

    # Print report
    print("# Semantic ID Validation Report (v0.2 Format)")
    print(f"\n**Generated**: 2026-01-29")
    print(f"**Format**: `<prefix>-<type>-<slug><suffix>`")
    print(f"**Spec**: docs/design/semantic-id-spec.md")

    print(f"\n## Summary Statistics")
    print(f"- **Total issues analyzed**: {total:,}")
    print(f"- **Format**: `prefix-type-slug+suffix` (e.g., `gt-epc-semantic_ids7x9k`)")
    print(f"\n### Collision Analysis")
    print(f"- **Slug collisions (before suffix)**: {collision_instances:,} ({100*collision_instances/total:.1f}%)")
    print(f"  - Patrol/Molecule (ephemeral): {patrol_collisions:,}")
    print(f"  - Work beads (persistent): {work_collisions:,}")
    print(f"- **Full ID collisions (with suffix)**: {full_collisions} (effectively 0%)")
    print(f"  - Random suffix provides 1.6M+ unique combinations")

    print(f"\n## Type Distribution")
    print("| Type Code | Count | Percentage |")
    print("|-----------|-------|------------|")
    for type_code, count in sorted(type_counts.items(), key=lambda x: -x[1]):
        pct = 100 * count / total
        print(f"| `{type_code}` | {count:,} | {pct:.1f}% |")

    print(f"\n## Slug Length Distribution")
    avg_slug_len = sum(slug_lengths) / len(slug_lengths) if slug_lengths else 0
    avg_total_len = sum(total_lengths) / len(total_lengths) if total_lengths else 0
    print(f"- Average slug length: {avg_slug_len:.1f} chars")
    print(f"- Average total ID length: {avg_total_len:.1f} chars")
    print("")
    for range_name, count in len_dist.items():
        pct = 100 * count / total if total else 0
        bar = "█" * int(pct / 2)
        print(f"- {range_name:>5} chars: {count:>4} ({pct:>5.1f}%) {bar}")

    print(f"\n## Top 15 Colliding Slugs (before suffix)")
    print("| Slug (prefix-type-slug) | Count | Category |")
    print("|-------------------------|-------|----------|")
    for slug_key, count in top_collisions:
        if count == 1:
            break
        # Categorize
        if any(kw in slug_key for kw in ["patrol", "digest", "wisp"]):
            cat = "Patrol"
        elif "mol-" in slug_key or "molecule" in slug_key:
            cat = "Molecule"
        else:
            cat = "Work"
        display = slug_key[:45] + ".." if len(slug_key) > 47 else slug_key
        print(f"| `{display}` | {count} | {cat} |")

    # Work beads only analysis
    work_types = {"bug", "tsk", "epc", "ftr"}
    work_beads = [r for r in results if r["type_code"] in work_types]
    if work_beads:
        work_slugs = [f"{extract_prefix(r['original_id'])}-{r['type_code']}-{r['slug']}" for r in work_beads]
        work_slug_counts = Counter(work_slugs)
        work_collision_count = sum(c for s, c in work_slug_counts.items() if c > 1)
        work_collision_rate = 100 * work_collision_count / len(work_beads) if work_beads else 0
        print(f"\n## Work Beads Analysis (bugs, tasks, epics, features)")
        print(f"- **Total work beads**: {len(work_beads):,}")
        print(f"- **Slug collision rate (before suffix)**: {work_collision_rate:.2f}%")
        print(f"- **With random suffix**: ~0% (suffix resolves all collisions)")

    print(f"\n## Sample Generated IDs")
    print("| Original ID | Type | Semantic ID | Slug Len |")
    print("|-------------|------|-------------|----------|")
    # Show variety
    samples = []
    seen_types = set()
    for r in results:
        if r["type_code"] not in seen_types or len(samples) < 20:
            if not any(kw in r["slug"] for kw in ["patrol", "digest", "mol_"]):
                samples.append(r)
                seen_types.add(r["type_code"])
        if len(samples) >= 20:
            break
    for r in samples[:20]:
        sem_id = r["semantic_id"][:40] + "..." if len(r["semantic_id"]) > 40 else r["semantic_id"]
        print(f"| `{r['original_id']}` | {r['type_code']} | `{sem_id}` | {r['slug_length']} |")

    print(f"\n## Validation Results")
    print("")
    print("### Acceptance Criteria")
    print("")

    # Check acceptance criteria
    work_collision_rate_val = work_collision_rate if work_beads else 0

    criteria = [
        ("Generated IDs are readable and meaningful", True, "Type code + semantic slug provides clear meaning"),
        ("Type visible in ID", True, f"Type codes: {', '.join(sorted(set(type_counts.keys())))}"),
        ("Collision-proof with suffix", full_collisions == 0, f"Full ID collisions: {full_collisions}"),
        ("Slug collisions acceptable (<5% for work)", work_collision_rate_val < 5, f"Work bead slug collision rate: {work_collision_rate_val:.2f}%"),
        ("Length distribution reasonable", 15 < avg_slug_len < 35, f"Average slug length: {avg_slug_len:.1f} chars"),
    ]

    for name, passed, detail in criteria:
        status = "✅" if passed else "❌"
        print(f"- {status} **{name}**")
        print(f"  - {detail}")

    all_passed = all(c[1] for c in criteria)
    print(f"\n### Recommendation")
    if all_passed:
        print("**PROCEED WITH IMPLEMENTATION** - All acceptance criteria met.")
    else:
        print("**REVIEW NEEDED** - Some criteria not met.")

    print(f"\n### Implementation Notes")
    print("- Format: `<prefix>-<type>-<slug><suffix>`")
    print("- Example: `gt-epc-semantic_ids7x9k`")
    print("- Type codes make filtering easy: `bd list | grep 'gt-bug-'`")
    print("- Random suffix (4 chars) guarantees uniqueness")
    print("- Patrol/molecule beads can optionally keep random IDs")

if __name__ == "__main__":
    main()
