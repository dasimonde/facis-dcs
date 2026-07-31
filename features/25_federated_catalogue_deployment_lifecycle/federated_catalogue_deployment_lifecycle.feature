# Deployment-level acceptance for the co-deployed XFSC Federated Catalogue.
# The destructive fresh-install, legacy-upgrade, disabled-FC and terminal-error
# cases require a dedicated isolated lifecycle runner.  They deliberately stay
# red until that runner can arrange and observe each lifecycle from before Helm
# starts; reducing them to assertions against an already-running stack would not
# test the requirement.

@DCS-IR-SI-01 @DCS-NFR-BR-09 @UC-02
Feature: Federated Catalogue deployment lifecycle

  @REQ-federated-catalogue-deployment-lifecycle-AC1
  Scenario: A co-deployed catalogue uses Fuseki without a Neo4j runtime dependency
    Given the co-deployed Federated Catalogue is enabled
    When I inspect the running Federated Catalogue graph-store deployment
    Then the Federated Catalogue uses the compatible Fuseki graph store
    And no running catalogue workload requires Neo4j, n10s or n10s.graphconfig.show

  @isolated_stack @requires-fc-lifecycle-runner
  @REQ-federated-catalogue-deployment-lifecycle-AC3
  @REQ-federated-catalogue-deployment-lifecycle-AC5
  @REQ-federated-catalogue-deployment-lifecycle-AC6
  Scenario: The first ready transition supports the first publish and read round-trip
    Given a fresh isolated Helm installation with the Federated Catalogue enabled
    When the lifecycle runner observes the first Federated Catalogue readiness transition
    Then the readiness gate has observed a successful health check
    And the first functional catalogue verification succeeds without warm-up or repetition
    And the Federated Catalogue pod has not restarted
    Given I am authenticated with roles: "Template Manager"
    And template "Fresh Catalogue Round Trip" is in "Registered" status
    And no business catalogue operation has run since the first readiness transition
    When I publish template "Fresh Catalogue Round Trip"
    Then get http 200:Success code
    And the catalogue operation completed within the DCS Federated Catalogue client timeout
    Given I am authenticated with roles: "Contract Creator"
    When I retrieve the template catalogue
    Then get http 200:Success code
    And the catalogue result includes template "Fresh Catalogue Round Trip"
    And the catalogue operation completed within the DCS Federated Catalogue client timeout
    When I search the template catalogue by name "Fresh Catalogue Round Trip"
    Then get http 200:Success code
    And the catalogue search result includes template "Fresh Catalogue Round Trip"
    And the catalogue operation completed within the DCS Federated Catalogue client timeout
    And the fresh-stack catalogue operations used no retry

  @isolated_stack @requires-fc-lifecycle-runner
  @REQ-federated-catalogue-deployment-lifecycle-AC4
  Scenario: Upgrading a Neo4j-based development installation replaces the old catalogue
    Given an isolated namespace contains the old Neo4j-based development installation
    When the lifecycle runner upgrades it with the current DCS Helm chart
    Then the Federated Catalogue is replaced completely without migrating old catalogue data
    And no obsolete Neo4j or n10s workload remains
    And the upgraded catalogue satisfies the fresh-install readiness gate

  @isolated_stack @requires-fc-lifecycle-runner
  @REQ-federated-catalogue-deployment-lifecycle-AC7
  Scenario Outline: Disabled catalogue integration skips every catalogue readiness check
    Given Federated Catalogue integration is disabled for the isolated "<entrypoint>"
    When the lifecycle runner starts the "<entrypoint>"
    Then it continues without executing a Federated Catalogue check

    Examples:
      | entrypoint      |
      | development     |
      | Helm deployment |
      | BDD runner      |

  @isolated_stack @requires-fc-lifecycle-runner
  @REQ-federated-catalogue-deployment-lifecycle-AC7
  Scenario Outline: Enabled catalogue integration gates every entrypoint on functional readiness
    Given Federated Catalogue integration is enabled for the isolated "<entrypoint>"
    And catalogue health succeeds but functional verification fails
    When the lifecycle runner starts the "<entrypoint>"
    Then it does not continue past the Federated Catalogue readiness gate

    Examples:
      | entrypoint      |
      | development     |
      | Helm deployment |
      | BDD runner      |

  @isolated_stack @requires-fc-lifecycle-runner
  @REQ-federated-catalogue-deployment-lifecycle-AC8
  Scenario Outline: A terminal catalogue error fails fast with a diagnosis
    Given Federated Catalogue integration is enabled for the isolated "<entrypoint>"
    And the first functional catalogue verification returns a terminal error
    When the lifecycle runner starts the "<entrypoint>"
    Then it exits immediately with the terminal Federated Catalogue diagnosis
    And it performs no blanket multi-minute catalogue wait
    And it performs no schema-sync retry or artificial warm-up

    Examples:
      | entrypoint      |
      | development     |
      | Helm deployment |
      | BDD runner      |
