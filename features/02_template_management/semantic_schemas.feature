# ------------------------------- FC roles and error response -------------------------------
# Federated Catalogue Roles
# [
#   "Ro-MU-A",
#   "Ro-PA-A",
#   "Ro-SD-A",
#   "Ro-MU-CA",
#   "uma_protection"
# ]

# Call FC APIs without FC role:
# {
#   "code": "forbidden_error",
#   "message": "User does not have permission to execute this request."
# }

# ------------------------------- scenarios -------------------------------
@skip
@UC-02-08 @FR-TR-03
Feature: Create and Maintain Semantic Schemas
  Template Managers create and manage semantic schemas
  used for template validation, including version control
  and schema lifecycle management.

  # UC-02-08, page 61, 75
  Scenario: Create a semantic schema
    Given I am authenticated with roles: "Ro-MU-CA"
    # section 4.2.2, stimulus 8
    # ⚠️ Also need FC roles, otherwise request will be rejected
    When I create a schema "contract-base-v1"
    # Implement check: can be verified via FC GET /schemas
    Then the schema is available for template linking

  # UC-02-08, page 61, 75
  Scenario: Link schema to template
    Given I am authenticated with roles: "Template Creator"
    And schema "contract-base-v1" exists
    # Use asset.go and FC POST /verification
    When I link schema "contract-base-v1" to template "Standard NDA"
    # May need to disable FC sinature verification if the testing server has no SSL certificate
    Then the template enforces schema conformity

  Scenario: Unauthorized role cannot create schema
    Given I am authenticated with roles: "Template Creator"
    When I create a schema "contract-base-v1"
    # ⚠️ FC checks only FC roles and doesn't consider DCS roles
    Then the request is denied with an authorization error

  # FR-TR-03: Semantic Hub for Schema Storage with Versioning
  Scenario: Create versioned semantic schema
    Given I am authenticated with roles: "Template Manager"
    When I create schema "contract-base" with version "1.0"
    Then the schema is created with version "1.0"
    And the schema version is tracked in the Semantic Hub
    # FC uses the same endpoint for create and update, and overwrite existing versions.
    # DCS need to check existing schema first before creating a new version.
    And the schema is marked as the current version

  # schema versioning, page 75
  Scenario: Update schema with new version
    Given I am authenticated with roles: "Template Manager"
    And schema "contract-base" with version "1.0" exists
    When I create schema "contract-base" with version "2.0"
    Then the new version "2.0" is created
    And version "1.0" remains accessible
    # ⚠️ FC schema updates don't notify partner FCs automatically.
    # The schema version selection may be configuration-driven
    And version "2.0" is marked as the current version

  # ⚠️ Inferred from page 75 (schema versioning), not explicitly defiend
  Scenario: Access previous schema versions
    Given I am authenticated with roles: "Template Manager"
    And schema "contract-base" with versions "1.0, 1.1, 2.0" exists
    When I retrieve schema "contract-base" with version "1.1"
    Then I receive the schema content for version "1.1"
    And I can see the full version history

  Scenario: Template references specific schema version
    Given I am authenticated with roles: "Template Manager"
    And schema "contract-base" with versions "1.0, 2.0" exists
    When I link schema "contract-base" with version "1.0" to template "Legacy NDA"
    Then the template is validated against schema version "1.0"
    And upgrading the schema does not affect the template validation

  # ❌ This goes beyond the SRS.
  Scenario: Deprecate schema version
    Given I am authenticated with roles: "Template Manager"
    And schema "contract-base" with version "1.0" is in use
    When I deprecate schema "contract-base" with version "1.0"
    Then the schema version is marked as deprecated
    # ⚠️ FC doesn't reject unknown or no-existing schema content.
    # So schema version management needs to be handled somewhere
    And templates using deprecated schema receive a warning
    And new templates cannot link to the deprecated version

  # I feel a bit over deisigned here, the UC-02-08 (page 75) doesn't explicitly require this.
  Scenario: Schema version compatibility check
    Given I am authenticated with roles: "Template Manager"
    And template "Standard NDA" uses schema "contract-base" with version "1.0"
    When I check compatibility with schema "contract-base" with version "2.0"
    Then the system analyzes schema differences
    And I receive a compatibility report with required changes

  # ❌ This goes beyond the SRS.
  Scenario: Migrate template to new schema version
    Given I am authenticated with roles: "Template Manager"
    And template "Standard NDA" uses schema "contract-base" with version "1.0"
    And schema "contract-base" with version "2.0" is backward compatible
    When I migrate template "Standard NDA" to schema version "2.0"
    Then the template is updated to reference version "2.0"
    And the migration is logged with old and new versions

  Scenario: Schema version history audit
    Given I am authenticated with roles: "Auditor"
    And schema "contract-base" has multiple versions
    When I retrieve the version history for schema "contract-base"
    Then I see all versions with creation timestamps
    And I see the author of each version
    And I see the change summary for each version
