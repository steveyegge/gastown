# pi-kimi Witness Patrol Results Summary

## Evaluation Status

**IN PROGRESS**: Standard witness patrol evaluation on sfgastown for agent pi-kimi (pir + Kimi K2.5).

## Test Coverage

- **Class B Tests**: 22 test cases (witness-stuck.yaml + witness-cleanup.yaml)
- **Class A Tests**: 3 test cases (class-a-witness.yaml)
- **Total**: 25 witness-specific test cases

## Metrics Being Tracked

- Token count
- Steps completed  
- Protocol adherence
- Errors

## Next Steps

1. Complete execution of all 25 test cases
2. Analyze results against Claude model baselines
3. Document detailed findings
4. Provide recommendations for pi-kimi deployment in witness roles

## Notes

This evaluation follows the standard Gas Town witness patrol protocol as defined in `internal/formula/formulas/mol-witness-patrol.formula.toml`. The pi-kimi agent combines the pir harness with Kimi K2.5 model capabilities.