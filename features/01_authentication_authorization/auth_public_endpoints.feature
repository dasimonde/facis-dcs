@UC-01-03
Feature: Public Authentication Endpoints
  Public auth endpoints should stay reachable without bearer authentication.

  Scenario Outline: Public auth endpoint responds successfully
    When the system sends "<method>" request to public endpoint "<endpoint>"
    Then the response status is 200
    And the response JSON includes "<field>"

    Examples:
      | method | endpoint    | field      |
      | GET    | /auth/login | auth_url   |
      | GET    | /auth/logout | logout_url |