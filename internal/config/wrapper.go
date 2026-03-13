package config

import "strings"

// WrapperContext provides values for template expansion in exec-wrapper args.
type WrapperContext struct {
	Rig           string
	Polecat       string
	InstallPrefix string
	WorkDir       string
	WorkspaceName string // pre-computed: <installPrefix>-<rig>--<polecat>
}

// newWrapperReplacer builds the shared template replacer for WrapperContext.
func newWrapperReplacer(ctx WrapperContext) *strings.Replacer {
	return strings.NewReplacer(
		"{{workspace}}", ctx.WorkspaceName,
		"{{rig}}", ctx.Rig,
		"{{polecat}}", ctx.Polecat,
		"{{install_prefix}}", ctx.InstallPrefix,
		"{{work_dir}}", ctx.WorkDir,
	)
}

// ExpandWrapper replaces {{var}} placeholders in wrapper args.
func ExpandWrapper(wrapper []string, ctx WrapperContext) []string {
	if wrapper == nil {
		return nil
	}
	if len(wrapper) == 0 {
		return []string{}
	}
	replacer := newWrapperReplacer(ctx)
	expanded := make([]string, len(wrapper))
	for i, arg := range wrapper {
		expanded[i] = replacer.Replace(arg)
	}
	return expanded
}

// ExpandInnerEnvValues replaces {{var}} placeholders in inner env map values.
// Keys are not expanded. Returns a new map; the input is not mutated.
// Returns nil if the input is nil or empty.
func ExpandInnerEnvValues(innerEnv map[string]string, ctx WrapperContext) map[string]string {
	if len(innerEnv) == 0 {
		return innerEnv
	}
	replacer := newWrapperReplacer(ctx)
	expanded := make(map[string]string, len(innerEnv))
	for k, v := range innerEnv {
		if !ValidateEnvKey(k) {
			continue
		}
		expanded[k] = replacer.Replace(v)
	}
	return expanded
}
