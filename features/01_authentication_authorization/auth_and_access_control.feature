@FR-UC-01-2
Feature: Authentication Endpoints
  Public auth endpoints should stay reachable without bearer token authentication.
  The other endpoints required a bearer token one.

  Scenario Outline: Public auth endpoint responds successfully
    When the system sends "<method>" request to endpoint "<endpoint>" without payload
    Then the response status is 200
    And the response JSON includes "<field>"

    Examples:
      | method | endpoint     | field      |
      | GET    | /auth/login  | auth_url   |
      | GET    | /auth/logout | logout_url |

  Scenario Outline: Access restricted endpoint responds with access denied (CWE)
    When the system sends "<method>" request to endpoint "<endpoint>" with "<payload>"
    Then the response status is 401

    Examples:
      | method | endpoint                 | payload                                                                                                                  |
      | POST   | /contract/create         | {'did':'placeholder'}                                                                                                    |
      | PUT    | /contract/update         | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                                                                |
      | POST   | /contract/submit         | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                                                                |
      | POST   | /contract/negotiate      | {'did':'placeholder','negotiated_by':'placeholder','change_request':{},'updated_at':'2024-01-01T00:00:00Z'}              |
      | POST   | /contract/respond        | {'id':'placeholder','did':'placeholder','action_flag':'placeholder','responded_by':'placeholder'}                        |
      | GET    | /contract/review         | did=placeholder                                                                                                          |
      | GET    | /contract/retrieve       | {}                                                                                                                       |
      | GET    | /contract/retrieve/placeholder | {}                                                                                                                       |
      | GET    | /contract/history/placeholder  | {}                                                                                                                       |
      | GET    | /contract/search         | {}                                                                                                                       |
      | POST   | /contract/approve        | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                                                                |
      | POST   | /contract/reject         | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z','reason':'placeholder'}                                         |
      | POST   | /contract/store          | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                                                                |
      | POST   | /contract/terminate      | {'did':'placeholder','reason':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                                         |
      | POST   | /contract/audit          | {'did':'placeholder'}                                                                                                    |

  Scenario Outline: Access restricted endpoint responds with access denied (Archive)
    When the system sends "<method>" request to endpoint "<endpoint>" with "<payload>"
    Then the response status is 401

    Examples:
      | method | endpoint          | payload |
      | GET    | /archive/retrieve | {}      |
      | GET    | /archive/search   | {}      |
      | POST   | /archive/store    | {}      |
      | POST   | /archive/terminate| {}      |
      | DELETE | /archive/delete   | {}      |
      | GET    | /archive/audit    | {}      |

  Scenario Outline: Access restricted endpoint responds with access denied (Template Repository)
    When the system sends "<method>" request to endpoint "<endpoint>" with "<payload>"
    Then the response status is 401

    Examples:
      | method | endpoint                  | payload                                                                             |
      | POST   | /template/create          | {'template_type':'placeholder'}                                                     |
      | POST   | /template/copy            | {'did':'placeholder'}                                                                                 |
      | POST   | /template/submit          | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                           |
      | PUT    | /template/update          | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                           |
      | GET    | /template/search          | {}                                                                                  |
      | GET    | /template/retrieve        | {}                                                                                  |
      | GET    | /template/retrieve/placeholder | {}                                                                                  |
      | GET    | /template/history/placeholder   | {}                                                                                  |
      | POST   | /template/verify          | {'did':'placeholder'}                                                               |
      | POST   | /template/approve         | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                           |
      | POST   | /template/reject          | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z','reason':'placeholder'}    |
      | POST   | /template/register        | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                           |
      | POST   | /template/archive         | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'}                           |
      | GET    | /template/audit           | did=placeholder                                                                     |

  Scenario Outline: Access restricted endpoint responds with access denied (Catalogue)
    When the system sends "<method>" request to endpoint "<endpoint>" with "<payload>"
    Then the response status is 401

    Examples:
      | method | endpoint                               | payload                                                                                                                                                                         |
      | GET    | /catalogue/template/retrieve           | offset=0&limit=10                                                                                                                                                               |
      | GET    | /catalogue/template/retrieve/placeholder     | {}                                                                                                                                                                              |
      | POST   | /catalogue/participant/create          | {'legal_name':'placeholder','registration_number':'placeholder','lei_code':'placeholder','ethereum_address':'placeholder','headquarter_address':{},'legal_address':{},'terms_and_conditions':'placeholder'} |
      | POST   | /catalogue/service-offering/create     | {'end_point_url':'placeholder','terms_and_conditions':'placeholder','keywords':['placeholder'],'description':'placeholder'}                                                      |
      | GET    | /catalogue/participant/current         | {}                                                                                                                                                                              |
      | GET    | /catalogue/participant/current/summary | {}                                                                                                                                                                              |
      | GET    | /catalogue/participant/others          | {}                                                                                                                                                                              |
      | GET    | /catalogue/service-offering/current    | {}                                                                                                                                                                              |
      | PUT    | /catalogue/participant/update          | {'legal_name':'placeholder','registration_number':'placeholder','lei_code':'placeholder','ethereum_address':'placeholder','headquarter_address':{},'legal_address':{},'terms_and_conditions':'placeholder'} |
      | PUT    | /catalogue/service-offering/update     | {'end_point_url':'placeholder','terms_and_conditions':'placeholder','keywords':['placeholder'],'description':'placeholder'}                                                      |
      | DELETE | /catalogue/participant/delete          | {}                                                                                                                                                                              |
      | DELETE | /catalogue/service-offering/delete     | {}                                                                                                                                                                              |

  Scenario Outline: Access restricted endpoint responds with access denied (Signature)
    When the system sends "<method>" request to endpoint "<endpoint>" with "<payload>"
    Then the response status is 401

    Examples:
      | method | endpoint                  | payload                                                   |
      | GET    | /signature/retrieve       | {}                                                        |
      | GET    | /signature/retrieve/placeholder | {}                                                        |
      | POST   | /signature/verify         | {'did':'placeholder'}                                     |
      | POST   | /signature/apply          | {'did':'placeholder','updated_at':'2024-01-01T00:00:00Z'} |
      | POST   | /signature/validate       | {'did':'placeholder'}                                     |
      | POST   | /signature/revoke         | {'did':'placeholder'}                                     |
      | GET    | /signature/audit          | did=placeholder                                           |
      | POST   | /signature/compliance     | {'did':'placeholder'}                                     |

  Scenario Outline: Access restricted endpoint responds with access denied (PAC)
    When the system sends "<method>" request to endpoint "<endpoint>" with "<payload>"
    Then the response status is 401

    Examples:
      | method | endpoint                            | payload                          |
      | POST   | /processauditandcompliance/audit    | {'scope':'TEMPLATE_REPOSITORY'}  |
      | GET    | /processauditandcompliance/report   | {}                               |
      | GET    | /processauditandcompliance/monitor  | {}                               |

  Scenario Outline: Access restricted endpoint responds with access denied (External)
    When the system sends "<method>" request to endpoint "<endpoint>" with "<payload>"
    Then the response status is 401

    Examples:
      | method | endpoint            | payload |
      | POST   | /external/action    | {}      |
      | GET    | /external/status    | {}      |
      | POST   | /external/callback  | {}      |

  Scenario Outline: Access restricted endpoint responds with access denied (Webhook)
    When the system sends "<method>" request to endpoint "<endpoint>" with "<payload>"
    Then the response status is 401

    Examples:
      | method | endpoint          | payload |
      | POST   | /webhook/node-red | {}      |

  Scenario Outline: Access restricted endpoint responds with access denied (Peer)
    When the system sends "<method>" request to endpoint "<endpoint>" with "<payload>"
    Then the response status is 401

    Examples:
      | method | endpoint       | payload |
      | GET    | /peer/retrieve | {}      |