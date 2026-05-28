Feature: Prometheus Monitoring
    Monitoring with Prometheus is enabled to collect metrics from the DCS components. This allows for performance tracking, alerting, and visualization of system health.

    Scenario: /metrics endpoint is exposed
        When the system sends "GET" request to internal endpoint "/metrics"
        Then the response status is 200