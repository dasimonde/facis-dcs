@FR-UC-01-2
Feature: Authentication Endpoints
  Public auth endpoints should stay reachable without bearer token authentication.
  The other endpoints required a bearer token.

  Scenario Outline: Public auth endpoint responds successfully
    When the system sends "<method>" request to endpoint "<endpoint>" without payload
    Then the response status is 200
    And the response JSON includes "<field>"

    Examples:
      | method | endpoint    | field       |
      | POST   | /auth/login | request_uri |

  Scenario: Logout without a session is rejected
    When the system sends "GET" request to endpoint "/auth/logout" without payload
    Then the response status is 401

  @DCS-IR-SI-03
  Scenario Outline: Access restricted endpoint responds with access denied (CWE)
    When the system sends "<method>" request to endpoint "<endpoint>" with "<payload>"
    Then the response status is 401

    Examples:
      | method | endpoint                       | payload                                                                                                     |
      | POST   | /contract/create               | {'template_did':'placeholder'}                                                                              |
      | PUT    | /contract/update               | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                                                   |
      | POST   | /contract/submit               | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                                                   |
      | POST   | /contract/negotiate            | {'did':'placeholder','negotiated_by':'placeholder','change_request':{},'updated_at':'2024-01-01T00:00:00Z'} |
      | POST   | /contract/respond              | {'id':'placeholder','did':'placeholder','action_flag':'placeholder','responded_by':'placeholder'}           |
      | GET    | /contract/review               | did=placeholder                                                                                             |
      | GET    | /contract/retrieve             | {}                                                                                                          |
      | GET    | /contract/retrieve/placeholder | {}                                                                                                          |
      | GET    | /contract/history/placeholder  | {}                                                                                                          |
      | GET    | /contract/search               | {}                                                                                                          |
      | POST   | /contract/approve              | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                                                   |
      | POST   | /contract/reject               | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z','reason':'placeholder'}                            |
      | POST   | /contract/store                | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                                                   |
      | POST   | /contract/terminate            | {'did':'placeholder','reason':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                            |
      | POST   | /contract/audit                | {'did':'placeholder'}                                                                                       |

  Scenario Outline: Access restricted endpoint responds with access denied (Archive)
    When the system sends "<method>" request to endpoint "<endpoint>" with "<payload>"
    Then the response status is 401

    Examples:
      | method | endpoint           | payload |
      | GET    | /archive/retrieve  | {}      |
      | GET    | /archive/search    | {}      |
      | POST   | /archive/store     | {}      |
      # archive/delete requires did+justification in the query string; without
      # them Goa's request decoder answers 400 before the JWT check can 401,
      # so the row supplies placeholders to reach the auth layer at all.
      | DELETE | /archive/delete?did=placeholder&justification=placeholder | {} |
      # /archive/audit and the /pac endpoints below likewise decode their
      # required justification before the JWT check; placeholders reach 401.
      | GET    | /archive/audit?justification=placeholder | {} |

  @DCS-FR-TR-06
  Scenario Outline: Access restricted endpoint responds with access denied (Template Repository)
    When the system sends "<method>" request to endpoint "<endpoint>" with "<payload>"
    Then the response status is 401

    Examples:
      | method | endpoint                       | payload                                                                          |
      | POST   | /template/create               | {'template_type':'placeholder'}                                                  |
      | POST   | /template/copy                 | {'did':'placeholder'}                                                            |
      | POST   | /template/submit               | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                        |
      | PUT    | /template/update               | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                        |
      | GET    | /template/search               | {}                                                                               |
      | GET    | /template/retrieve             | {}                                                                               |
      | GET    | /template/retrieve/placeholder | {}                                                                               |
      | GET    | /template/history/placeholder  | {}                                                                               |
      | POST   | /template/verify               | {'did':'placeholder'}                                                            |
      | POST   | /template/approve              | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                        |
      | POST   | /template/reject               | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z','reason':'placeholder'} |
      | POST   | /template/register             | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                        |
      | POST   | /template/archive              | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                        |
      | GET    | /template/audit                | did=placeholder                                                                  |

  Scenario Outline: Access restricted endpoint responds with access denied (Catalogue)
    When the system sends "<method>" request to endpoint "<endpoint>" with "<payload>"
    Then the response status is 401

    Examples:
      | method | endpoint                                 | payload                                                                                                                                                                                                     |
      | GET    | /catalogue/template/retrieve             | offset=0&limit=10                                                                                                                                                                                           |
      | GET    | /catalogue/template/retrieve/placeholder | version=1                                                                                                                                                                                                   |
      | GET    | /catalogue/template/search                 | offset=0&limit=10                                                                                                                                                                                           |

  Scenario Outline: Access restricted endpoint responds with access denied (Signature)
    When the system sends "<method>" request to endpoint "<endpoint>" with "<payload>"
    Then the response status is 401

    Examples:
      | method | endpoint                        | payload                                                   |
      | GET    | /signature/retrieve             | {}                                                        |
      | GET    | /signature/retrieve/placeholder | {}                                                        |
      | POST   | /signature/verify               | {'did':'placeholder'}                                     |
      | POST   | /signature/validate             | {'did':'placeholder'}                                     |
      | POST   | /signature/revoke               | {'did':'placeholder','signer_did':'placeholder','reason':'Access-control probe'} |
      | GET    | /signature/audit                | did=placeholder                                           |
      | POST   | /signature/compliance           | {'did':'placeholder'}                                     |

  Scenario Outline: Access restricted endpoint responds with access denied (PAC)
    When the system sends "<method>" request to endpoint "<endpoint>" with "<payload>"
    Then the response status is 401

    Examples:
      | method | endpoint     | payload                         |
      | POST   | /pac/audit   | {'scope':'TEMPLATE_REPOSITORY','justification':'placeholder'} |
      | GET    | /pac/report?justification=placeholder | {} |
      | GET    | /pac/monitor | {}                              |

  Scenario Outline: Access restricted endpoint responds with access denied (Peer)
    When the system sends "<method>" request to endpoint "<endpoint>" with "<payload>"
    Then the response status is 401

    Examples:
      | method | endpoint                   | payload         |
      | GET    | /peer/contracts/provenance | did=placeholder |
