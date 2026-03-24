package design

import (
	. "goa.design/goa/v3/dsl"
	cors "goa.design/plugins/v3/cors/dsl" // Kein Punkt, sondern Alias 'cors'
)

// JWTAuth defines the JWT-based security scheme backed by Keycloak OIDC.
var JWTAuth = JWTSecurity("jwt", func() {
	Description("Keycloak OIDC JWT Bearer token. Scopes correspond to Keycloak client roles.")
	Scope("Archive Manager", "Manage archived contracts and evidence")
	Scope("Contract Observer", "Read-only access to archived contracts")
	Scope("Contract Creator", "Create new contract drafts")
	Scope("Sys. Contract Creator", "System-level contract creation")
	Scope("Contract Negotiator", "Negotiate contract terms")
	Scope("Contract Reviewer", "Review submitted contracts")
	Scope("Sys. Contract Reviewer", "System-level contract review")
	Scope("Contract Approver", "Approve or reject contracts")
	Scope("Sys. Contract Approver", "System-level contract approval")
	Scope("Contract Manager", "Manage contract lifecycle")
	Scope("Sys. Contract Manager", "System-level contract management")
	Scope("Contract Signer", "Sign contracts digitally")
	Scope("Sys. Contract Signer", "System-level contract signing")
	Scope("Template Creator", "Create new templates")
	Scope("Template Reviewer", "Review submitted templates")
	Scope("Template Approver", "Approve or reject templates")
	Scope("Template Manager", "Manage template lifecycle")
	Scope("Auditor", "Perform audits and generate reports")
	Scope("Compliance Officer", "Monitor compliance and report incidents")
	Scope("System Administrator", "Maintains system configurations, permissions, and user access")
})

// API root
var _ = API("dcs", func() {
	Title("DCS API Server")
	Version("0.0.1")

	cors.Origin("*", func() {
		cors.Headers("Content-Type", "Authorization", "X-Shared-Secret", "X-Api-Version")
		cors.Methods("GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS")
		cors.MaxAge(100)
	})
	// Global error definitions mapped to HTTP status codes.
	Error("unauthorized", String, "Credentials are invalid or missing.")
	Error("forbidden", String, "Insufficient permissions.")

	HTTP(func() {
		Response("unauthorized", StatusUnauthorized)
		Response("forbidden", StatusForbidden)
	})

	Server("dcs", func() {
		Host("local", func() {
			URI("http://localhost:8991")
		})
	})
})
