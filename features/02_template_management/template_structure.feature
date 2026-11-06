# DCS-FR-TR-02, DCS-FR-TR-23, DCS-FR-TR-26: a template has a hierarchy level
# (frame agreement, agreement, sub-agreement, addendum) and requires only
# templates of a higher level. Submission checks the dependencies
# (backend/internal/base/templatestructure).

@UC-02
Feature: Template levels and required templates

  @clean_db @DCS-FR-TR-02 @DCS-FR-TR-23 @DCS-FR-TR-26
  Scenario: A frame agreement template cannot require another frame agreement
    Given a registered frame template "Level Frame Template"
    When I submit a frame template "Second Frame Template" that requires template "Level Frame Template"
    Then the request is denied with a client error
    And template "Second Frame Template" is still in "Draft" status
