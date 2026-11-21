# platform-update-policy questions

Base: a27dd0b3ba80b029cd254df79b1a7ab2783ffdfe

## Settled Owner Frontier Decisions

### Q1 · Compatibility definition (answered)
An update is incompatible ("requires code changes") if `doctor --contract` / `ContractIssues` / sent-reply checks fail OR `/v1/spec` **major** version bumps (`5.x` -> `6.x`, expected major = `5`). Minor/patch spec bumps where contract checks pass remain compatible with an informational review note (`doc/beeper-api.md` / `desktop-api-go`).

### Q2 · Clearing blocked versions (answered)
Tag each blocked version with the adapter's build/contract revision constant so shipping updated adapter code automatically clears old blocks; newer platform versions are always evaluated fresh; support `--force` as an explicit escape hatch.

### No unattended platform updates (answered)
Detect and surface pending/external updates in logs, `status`, and `check-updates`, never auto-restart/install unattended. On failed managed update, auto-rollback and mark the candidate version as requiring code changes. On failed external update, warn in logs/status ("requires code changes") and keep running unless API calls fail.

### Recorded Domain Entries
- `messaging` / `domain_service` / `UpdatePolicy`
- `messaging` / `UpdatePolicy` / `assertion` / `no-unattended-platform-updates` (`on: AssessUpdate`)
- `messaging` / `UpdatePolicy` / `assertion` / `incompatible-managed-update-requires-rollback` (`on: VerifyAppliedUpdate`)
- `messaging` / `UpdatePolicy` / `assertion` / `incompatible-update-requires-code-changes` (`on: VerifyAppliedUpdate`)
- `messaging` / `UpdatePolicy` / `assertion` / `blocked-version-refused-for-managed-update` (`on: AssessUpdate`)
- `messaging` / `UpdatePolicy` / `assertion` / `external-incompatible-update-keeps-running` (`on: AssessUpdate`)
