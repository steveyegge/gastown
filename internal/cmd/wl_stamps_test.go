package cmd

import (
	"strings"
	"testing"
)

func TestBuildStampsQuery_SubjectOnly(t *testing.T) {
	t.Parallel()
	f := StampsFilter{
		Subject: "gastown",
		Limit:   50,
	}
	got := buildStampsQuery(f)
	if !strings.Contains(got, "subject = 'gastown'") {
		t.Errorf("missing subject filter in %q", got)
	}
	if !strings.Contains(got, "ORDER BY created_at DESC") {
		t.Errorf("missing ORDER BY in %q", got)
	}
	if !strings.Contains(got, "LIMIT 50") {
		t.Errorf("missing LIMIT in %q", got)
	}
}

func TestBuildStampsQuery_AllFilters(t *testing.T) {
	t.Parallel()
	f := StampsFilter{
		Subject:     "gastown",
		Author:      "hop-mayor",
		Skill:       "go",
		ContextType: "boot_block",
		Severity:    "branch",
		Limit:       10,
	}
	got := buildStampsQuery(f)
	for _, substr := range []string{
		"subject = 'gastown'",
		"author = 'hop-mayor'",
		"context_type = 'boot_block'",
		"severity = 'branch'",
		"JSON_CONTAINS(skill_tags, '\"go\"')",
		"LIMIT 10",
	} {
		if !strings.Contains(got, substr) {
			t.Errorf("buildStampsQuery(all) missing %q in %q", substr, got)
		}
	}
}

func TestBuildStampsQuery_EscapesSQL(t *testing.T) {
	t.Parallel()
	f := StampsFilter{
		Subject: "it's",
		Author:  "o'malley",
		Limit:   50,
	}
	got := buildStampsQuery(f)
	if !strings.Contains(got, "it''s") {
		t.Errorf("subject not escaped: %q", got)
	}
	if !strings.Contains(got, "o''malley") {
		t.Errorf("author not escaped: %q", got)
	}
}

func TestFormatValence_JSONString(t *testing.T) {
	t.Parallel()
	got := formatValence(`{"quality": 4, "reliability": 3, "creativity": 2}`)
	if !strings.Contains(got, "Q:4") {
		t.Errorf("missing Q:4 in %q", got)
	}
	if !strings.Contains(got, "R:3") {
		t.Errorf("missing R:3 in %q", got)
	}
	if !strings.Contains(got, "C:2") {
		t.Errorf("missing C:2 in %q", got)
	}
}

func TestFormatValence_Map(t *testing.T) {
	t.Parallel()
	m := map[string]interface{}{
		"quality":     float64(5),
		"reliability": float64(4),
	}
	got := formatValence(m)
	if !strings.Contains(got, "Q:5") || !strings.Contains(got, "R:4") {
		t.Errorf("formatValence(map) = %q, want Q:5 R:4", got)
	}
}

func TestFormatValence_BootBlock(t *testing.T) {
	t.Parallel()
	got := formatValence(`{"quality": 3, "volume": 5}`)
	if !strings.Contains(got, "Q:3") || !strings.Contains(got, "V:5") {
		t.Errorf("formatValence(boot_block) = %q, want Q:3 V:5", got)
	}
}

func TestFormatValence_Nil(t *testing.T) {
	t.Parallel()
	got := formatValence(nil)
	if got != "" {
		t.Errorf("formatValence(nil) = %q, want empty", got)
	}
}

func TestFormatSkillTags_JSONString(t *testing.T) {
	t.Parallel()
	got := formatSkillTags(`["go", "federation"]`)
	if got != "go, federation" {
		t.Errorf("formatSkillTags(json) = %q, want %q", got, "go, federation")
	}
}

func TestFormatSkillTags_Array(t *testing.T) {
	t.Parallel()
	arr := []interface{}{"go", "dolt"}
	got := formatSkillTags(arr)
	if got != "go, dolt" {
		t.Errorf("formatSkillTags(array) = %q, want %q", got, "go, dolt")
	}
}

func TestFormatSkillTags_Nil(t *testing.T) {
	t.Parallel()
	got := formatSkillTags(nil)
	if got != "" {
		t.Errorf("formatSkillTags(nil) = %q, want empty", got)
	}
}

func TestFormatStampDate_Full(t *testing.T) {
	t.Parallel()
	got := formatStampDate("2026-03-19 14:30:00")
	if got != "2026-03-19" {
		t.Errorf("formatStampDate = %q, want %q", got, "2026-03-19")
	}
}

func TestFormatStampDate_Short(t *testing.T) {
	t.Parallel()
	got := formatStampDate("2026")
	if got != "2026" {
		t.Errorf("formatStampDate(short) = %q, want %q", got, "2026")
	}
}

func TestToFloat(t *testing.T) {
	t.Parallel()
	if toFloat(float64(3.5)) != 3.5 {
		t.Error("toFloat(float64) failed")
	}
	if toFloat(42) != 42.0 {
		t.Error("toFloat(int) failed")
	}
	if toFloat("nope") != 0 {
		t.Error("toFloat(string) should return 0")
	}
}
