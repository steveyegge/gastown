<!-- ox-hash: e5e8f24bf160 ver: 0.4.0 -->
Initialize SageOx for this repository.

Use when:
- Setting up SageOx in a new repository for the first time
- Configuring ox CLI integration with your coding agent
- Installing hooks for automatic guidance loading

Keywords: init, initialize, setup, install, configure, first-time, onboarding

## Common Issues

### Already initialized
**Symptom:** `SageOx already initialized in this repository`
**Solution:** This is fine - ox is ready to use. Run `ox agent prime` to load guidance

### Permission denied
**Symptom:** Cannot create .sageox directory
**Solution:** Check write permissions for the repository directory

### Not a git repository
**Symptom:** `not a git repository`
**Solution:** Initialize git first: `git init`

$ox init
