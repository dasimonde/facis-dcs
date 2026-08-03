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
