@UC-12-06
@skip
Feature: Implemented Protected API Coverage
  Every currently implemented protected API endpoint must reject invalid credentials.

  Scenario Outline: Protected endpoint denies invalid token
    Given a system service provides an invalid API key
    When the system sends "<method>" request to protected endpoint "<endpoint>" with payload "<payload_key>"
    Then the request is denied with an authorization error

    Examples:
      | method | endpoint                         | payload_key             |
      | POST   | /template/create                 | template_create         |
      | POST   | /template/submit                 | template_submit         |
      | PUT    | /template/update                 | template_update         |
      | POST   | /template/update                 | template_update_manage  |
      | GET    | /template/search                 | none                    |
      | GET    | /template/retrieve               | none                    |
      | GET    | /template/retrieve/did:example:1 | none                    |
      | POST   | /template/verify                 | template_verify         |
      | POST   | /template/approve                | template_approve        |
      | POST   | /template/reject                 | template_reject         |
      | POST   | /template/register               | template_register       |
      | POST   | /template/archive                | template_archive        |
      | GET    | /template/audit?did=did:example:1 | none                  |
      | POST   | /contract/create                 | contract_create         |
      | PUT    | /contract/update                 | contract_update         |
      | POST   | /contract/submit                 | contract_submit         |
      | POST   | /contract/negotiate              | contract_negotiate      |
      | POST   | /contract/respond                | contract_respond        |
      | GET    | /contract/review?did=did:example:1 | none                 |
      | GET    | /contract/retrieve               | none                    |
      | GET    | /contract/retrieve/did:example:1 | none                    |
      | POST   | /contract/verify                 | contract_verify         |
      | GET    | /contract/search                 | none                    |
      | POST   | /contract/approve                | contract_approve        |
      | POST   | /contract/reject                 | contract_reject         |
      | POST   | /contract/store                  | contract_store          |
      | POST   | /contract/terminate              | contract_terminate      |
      | POST   | /contract/audit                  | contract_audit          |