# platform-update-policy: requirements

Each line names the domain key it refers to. The rule text lives only in the domain recording.

- When GenTalent observes a pending platform update during runtime or check-updates, `UpdatePolicy` shall handle it according to the unattended update rule. Domain: messaging / UpdatePolicy / assertion / no-unattended-platform-updates
- When a managed platform update fails compatibility verification, `UpdatePolicy` shall require rollback. Domain: messaging / UpdatePolicy / assertion / incompatible-managed-update-requires-rollback
- When an applied platform update fails compatibility verification, `UpdatePolicy` shall record the requirement for adapter code changes. Domain: messaging / UpdatePolicy / assertion / incompatible-update-requires-code-changes
- When an operator requests a managed update to a candidate version, `UpdatePolicy` shall enforce blocked-version refusal. Domain: messaging / UpdatePolicy / assertion / blocked-version-refused-for-managed-update
- When an external platform update fails compatibility during observation, `UpdatePolicy` shall determine GenTalent's run and warning behavior. Domain: messaging / UpdatePolicy / assertion / external-incompatible-update-keeps-running
