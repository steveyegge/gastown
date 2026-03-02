# pi-kimi Witness Patrol Results Summary

## Evaluation Status

**READY**: Infrastructure prepared for pi-kimi witness patrol evaluation on sfgastown.
**TO RUN EVALUATION**:

```bash
cd gt-model-eval
./run-witness-tests.sh
```

This will execute all 25 witness-specific test cases (22 Class B + 3 Class A) and generate results in JSON format.

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
5. Run evaluation using `./run-witness-tests.sh` script
6. Analyze generated results against Claude model baselines

## Notes

This evaluation follows the standard Gas Town witness patrol protocol as defined in `internal/formula/formulas/mol-witness-patrol.formula.toml`. The pi-kimi agent combines the pir harness with Kimi K2.5 model capabilities.