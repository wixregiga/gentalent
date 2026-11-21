Feature: platform-update-policy

  @messaging_UpdatePolicy_no-unattended-platform-updates
  Scenario: A pending platform update waits for explicit operator action
    Given the platform is running version 4.3.200 with spec 5.0.0
    And a verified pending update 4.3.210 is present in the updater cache
    When UpdatePolicy assesses the observation in observe mode
    Then auto-installation is disallowed and managed update waits for explicit operator action

  @messaging_UpdatePolicy_incompatible-managed-update-requires-rollback
  Scenario: A managed update that fails compatibility requires rollback
    Given a managed update from 4.3.200 to 4.3.220 was applied
    When VerifyAppliedUpdate observes a spec major version bump to 6.0.0 or contract issues
    Then RequiresRollback is true and RollbackToVersion is 4.3.200

  @messaging_UpdatePolicy_incompatible-update-requires-code-changes
  Scenario: An incompatible applied update is marked as requiring code changes
    Given a managed update from 4.3.200 to 4.3.220 was applied
    When VerifyAppliedUpdate observes contract issues or a spec major bump
    Then MarkRequiresCodeChanges is true and 4.3.220 is recorded in BlockedVersions for the current adapter revision

  @messaging_UpdatePolicy_blocked-version-refused-for-managed-update
  Scenario: A blocked version is refused for managed update unless forced
    Given 4.3.220 is blocked for the current adapter revision
    When UpdatePolicy assesses a managed update to 4.3.220 without force
    Then ManagedUpdateAllowed is false and CandidateBlocked is true

  @messaging_UpdatePolicy_external-incompatible-update-keeps-running
  Scenario: An external incompatible update keeps GenTalent running with a warning
    Given GenTalent last observed version 4.3.200
    When UpdatePolicy observes an external update to 4.4.0 with spec 6.0.0 in observe mode
    Then KeepRunning is true, RequiresCodeChanges is true, and ExternalUpdateDetected is true
