package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExpandWrapper_NilReturnsNil(t *testing.T) {
	ctx := WrapperContext{Rig: "myrig", Polecat: "mypolecat"}
	result := ExpandWrapper(nil, ctx)
	assert.Nil(t, result)
}

func TestExpandWrapper_EmptyReturnsEmpty(t *testing.T) {
	ctx := WrapperContext{Rig: "myrig", Polecat: "mypolecat"}
	result := ExpandWrapper([]string{}, ctx)
	assert.NotNil(t, result)
	assert.Equal(t, []string{}, result)
}

func TestExpandWrapper_StaticArgs(t *testing.T) {
	ctx := WrapperContext{
		Rig:           "myrig",
		Polecat:       "mypolecat",
		InstallPrefix: "gt-abc123",
		WorkDir:       "/home/user/project",
		WorkspaceName: "gt-abc123-myrig--mypolecat",
	}
	wrapper := []string{"exitbox", "run", "--profile=gastown-polecat", "--"}
	result := ExpandWrapper(wrapper, ctx)
	assert.Equal(t, []string{"exitbox", "run", "--profile=gastown-polecat", "--"}, result)
}

func TestExpandWrapper_AllTemplateVariables(t *testing.T) {
	ctx := WrapperContext{
		Rig:           "prodrig",
		Polecat:       "obsidian",
		InstallPrefix: "gt-x7f",
		WorkDir:       "/workspace/repo",
		WorkspaceName: "gt-x7f-prodrig--obsidian",
	}
	wrapper := []string{
		"daytona", "exec", "{{workspace}}",
		"--rig={{rig}}", "--polecat={{polecat}}",
		"--prefix={{install_prefix}}", "--dir={{work_dir}}",
		"--",
	}
	result := ExpandWrapper(wrapper, ctx)
	assert.Equal(t, []string{
		"daytona", "exec", "gt-x7f-prodrig--obsidian",
		"--rig=prodrig", "--polecat=obsidian",
		"--prefix=gt-x7f", "--dir=/workspace/repo",
		"--",
	}, result)
}

func TestExpandWrapper_PartialTemplateUsage(t *testing.T) {
	ctx := WrapperContext{
		Rig:           "myrig",
		Polecat:       "ruby",
		InstallPrefix: "gt-99z",
		WorkDir:       "/home/dev",
		WorkspaceName: "gt-99z-myrig--ruby",
	}
	wrapper := []string{"sandbox", "run", "{{workspace}}", "--verbose", "--"}
	result := ExpandWrapper(wrapper, ctx)
	assert.Equal(t, []string{"sandbox", "run", "gt-99z-myrig--ruby", "--verbose", "--"}, result)
}

func TestExpandWrapper_UnknownVariablesPassThrough(t *testing.T) {
	ctx := WrapperContext{
		Rig:           "myrig",
		Polecat:       "mypolecat",
		InstallPrefix: "gt-abc",
		WorkDir:       "/work",
		WorkspaceName: "gt-abc-myrig--mypolecat",
	}
	wrapper := []string{"cmd", "{{unknown_var}}", "{{workspace}}", "{{also_unknown}}"}
	result := ExpandWrapper(wrapper, ctx)
	assert.Equal(t, []string{"cmd", "{{unknown_var}}", "gt-abc-myrig--mypolecat", "{{also_unknown}}"}, result)
}

func TestExpandWrapper_WorkspaceNamePrecomputed(t *testing.T) {
	ctx := WrapperContext{
		Rig:           "rig1",
		Polecat:       "pol1",
		InstallPrefix: "gt-abc",
		WorkDir:       "/work",
		WorkspaceName: "gt-abc-rig1--pol1",
	}
	// Verify WorkspaceName follows <installPrefix>-<rig>--<polecat> convention
	expected := ctx.InstallPrefix + "-" + ctx.Rig + "--" + ctx.Polecat
	assert.Equal(t, expected, ctx.WorkspaceName)

	wrapper := []string{"{{workspace}}"}
	result := ExpandWrapper(wrapper, ctx)
	assert.Equal(t, []string{expected}, result)
}

func TestExpandWrapper_DoesNotMutateInput(t *testing.T) {
	ctx := WrapperContext{
		Rig:           "myrig",
		Polecat:       "mypolecat",
		InstallPrefix: "gt-abc",
		WorkDir:       "/work",
		WorkspaceName: "gt-abc-myrig--mypolecat",
	}
	wrapper := []string{"cmd", "{{workspace}}", "static"}
	original := make([]string, len(wrapper))
	copy(original, wrapper)

	ExpandWrapper(wrapper, ctx)
	assert.Equal(t, original, wrapper, "ExpandWrapper should not mutate the input slice")
}

func TestExpandInnerEnvValues_NilReturnsNil(t *testing.T) {
	ctx := WrapperContext{Rig: "myrig"}
	result := ExpandInnerEnvValues(nil, ctx)
	assert.Nil(t, result)
}

func TestExpandInnerEnvValues_EmptyReturnsEmpty(t *testing.T) {
	ctx := WrapperContext{Rig: "myrig"}
	result := ExpandInnerEnvValues(map[string]string{}, ctx)
	assert.Equal(t, map[string]string{}, result)
}

func TestExpandInnerEnvValues_ExpandsTemplateVars(t *testing.T) {
	ctx := WrapperContext{
		Rig:           "furiosa",
		Polecat:       "obsidian",
		InstallPrefix: "gt-abc",
		WorkDir:       "/workspace/repo",
		WorkspaceName: "gt-abc-furiosa--obsidian",
	}
	innerEnv := map[string]string{
		"GT_WORKDIR":   "/data/{{rig}}/{{polecat}}",
		"GT_WORKSPACE": "{{workspace}}",
		"GT_PLAIN":     "no-templates-here",
	}
	result := ExpandInnerEnvValues(innerEnv, ctx)

	assert.Equal(t, "/data/furiosa/obsidian", result["GT_WORKDIR"])
	assert.Equal(t, "gt-abc-furiosa--obsidian", result["GT_WORKSPACE"])
	assert.Equal(t, "no-templates-here", result["GT_PLAIN"])
}

func TestExpandInnerEnvValues_DoesNotMutateInput(t *testing.T) {
	ctx := WrapperContext{Rig: "myrig", Polecat: "mypolecat"}
	innerEnv := map[string]string{
		"KEY": "{{rig}}-{{polecat}}",
	}
	original := innerEnv["KEY"]

	result := ExpandInnerEnvValues(innerEnv, ctx)
	assert.Equal(t, "myrig-mypolecat", result["KEY"])
	assert.Equal(t, original, innerEnv["KEY"], "ExpandInnerEnvValues should not mutate input map")
}
