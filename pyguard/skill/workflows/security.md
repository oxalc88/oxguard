# Security

1. Run with the requested root and scope:

   ```sh
   pyguard security --output agent
   ```

2. Read every sub-gate: Bandit, pip-audit and detect-secrets complete independently. Missing engines or skipped audits are execution errors.
3. Keep native rule IDs, locations, package identity and fixed versions. Never expose secret values.
4. Development-only use does not waive a vulnerability. Never add acceptance entries just to pass.
5. Accept a `.oxguard-dependency-policy.json` review only when authorized and valid per [dependency exposure reviews](https://github.com/oxalc88/oxguard/blob/main/docs/guides/pyguard-result-contract.md#dependency-exposure-reviews).

Done: findings per sub-gate and any accepted reviews.
