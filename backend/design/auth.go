package design

import (
	. "goa.design/goa/v3/dsl"
)

// Auth Service — Hydra OIDC and OpenID4VP authentication endpoints.
var _ = Service("Auth", func() {
	Description("Authentication endpoints for Hydra OIDC session management and OpenID4VP login.")

	Method("login", func() {
		Description("Starts the Hydra OIDC login flow with an OpenID4VP presentation request.")
		NoSecurity()
		Result(func() {
			Attribute("request_uri", String, "URL that the wallet uses to fetch the OpenID4VP authorization request object")
			Attribute("presentation_url", String, "Wallet deep link (openid4vp://) for QR code generation and mobile wallet support")
			Attribute("state", String, "Login state identifier used to correlate the frontend session, wallet presentation, and Hydra login flow")
			Attribute("authorize_url", String, "Hydra OAuth2 authorize URL that the browser must open to establish the Hydra session")

			Attribute("expires_in", Int, "Seconds until this login state expires")
			Required("request_uri", "presentation_url", "state", "authorize_url", "expires_in")
		})
		HTTP(func() {
			POST("/auth/login")
			Response(StatusOK)
		})
	})

	Method("loginRenew", func() {
		Description("Extends an existing OpenID4VP login state and returns an updated presentation link without changing the OIDC state.")
		NoSecurity()
		Payload(func() {
			Attribute("state", String, "Login state identifier from the current login session")
			Required("state")
		})
		Result(func() {
			Attribute("request_uri", String, "URL that the wallet uses to fetch the OpenID4VP authorization request object")
			Attribute("presentation_url", String, "Wallet deep link (openid4vp://) for QR code generation and mobile wallet support")
			Attribute("state", String, "Login state identifier")
			Attribute("authorize_url", String, "Hydra OAuth2 authorize URL for this login state")
			Attribute("expires_in", Int, "Seconds until this login state expires")
			Required("request_uri", "presentation_url", "state", "authorize_url", "expires_in")
		})
		HTTP(func() {
			POST("/auth/login/renew")
			Response(StatusOK)
		})
	})

	Method("loginChallenge", func() {
		Description("Binds the Hydra login challenge from the browser authorize redirect to a pending OpenID4VP login state.")

		NoSecurity()
		Payload(func() {
			Attribute("state", String, "Login state identifier from the login response")
			Attribute("login_challenge", String, "Hydra login challenge from the authorize redirect to the login UI")
			Required("state", "login_challenge")
		})
		HTTP(func() {
			POST("/auth/login/challenge")
			Response(StatusNoContent)
		})
	})

	Method("consent", func() {
		Description("Handles the Hydra consent callback, accepts the consent challenge via the Admin API, and redirects the browser to continue the OAuth2 flow.")

		NoSecurity()
		Payload(func() {
			Attribute("consent_challenge", String, "Hydra consent challenge from the authorize redirect")
			Required("consent_challenge")
		})
		Result(func() {
			Attribute("location", String, "Hydra redirect_to URL after consent accept")
			Required("location")
		})
		HTTP(func() {
			GET("/auth/consent")
			Param("consent_challenge")
			Response(StatusFound, func() {
				Header("location:Location")
			})
		})
	})

	Method("callback", func() {
		Description("Handles the Hydra OIDC callback: if Hydra returned an OAuth2 error (error/error_description params), clears the session cookies and redirects to the UI with the error details attached as query params; otherwise exchanges the authorization code for tokens, sets the refresh_token and id_token session cookies, and redirects to auth success.")
		NoSecurity()
		Payload(func() {
			Attribute("code", String, "Authorization code from Hydra")
			Attribute("state", String, "OIDC state parameter from Hydra")
			Attribute("error", String, "OAuth2 error code returned by Hydra when authorization fails")
			Attribute("error_description", String, "OAuth2 error description returned by Hydra when authorization fails")

		})
		Result(func() {
			Attribute("location", String, "Frontend redirect location after the token exchange")
			Required("location")
		})
		HTTP(func() {
			GET("/auth/callback")
			Param("code")
			Param("state")
			Param("error")
			Param("error_description")
			Response(StatusFound, func() {
				Header("location:Location")
			})
		})
	})

	Method("refresh", func() {
		Description("Exchanges a refresh token (from HttpOnly cookie) for a new access token.")
		NoSecurity()
		Result(func() {
			Attribute("access_token", String, "JWT access token")
			Attribute("token_type", String, "Token type (Bearer)")
			Attribute("expires_in", Int, "Token expiry in seconds")
			Required("access_token", "token_type", "expires_in")
		})
		HTTP(func() {
			POST("/auth/refresh")
			Response(StatusOK)
		})
	})

	Method("logout", func() {
		Description("Revokes the current refresh token at Hydra, clears the refresh_token and id_token session cookies, and returns the Hydra OIDC end-session (logout) URL for the client to redirect to.")
		NoSecurity()
		Result(func() {
			Attribute("logout_url", String, "Hydra OIDC logout URL")
			Required("logout_url")
		})
		HTTP(func() {
			GET("/auth/logout")
			Response(StatusOK)
		})
	})

	Method("loginStatus", func() {
		Description("Returns the current OpenID4VP login status for frontend polling.")
		NoSecurity()
		Payload(PresentationStatePayload)
		Result(PresentationStatusResult)
		HTTP(func() {
			GET("/auth/login/status")
			Param("state")
			Response(StatusOK)
		})
	})

	Method("logoutComplete", func() {
		Description("Logout callback. Clears refresh token cookie and redirects to home.")
		NoSecurity()
		Result(func() {
			Attribute("location", String, "Frontend redirect location after logout")
			Required("location")
		})
		HTTP(func() {
			GET("/auth/logout-complete")
			Response(StatusFound, func() {
				Header("location:Location")
			})
		})
	})

	Method("presentationRequest", func() {
		Description("Returns a signed OpenID4VP authorization request JWT for request-by-reference wallet retrieval.")
		NoSecurity()
		Payload(func() {
			Attribute("state", String, "Login state identifier from the login response")
			Attribute("wallet_nonce", String, "Wallet-provided nonce echoed in the authorization request object when request_uri_method=post")
			Attribute("wallet_metadata", String, "Wallet metadata JSON submitted with request_uri_method=post")
			Required("state")
		})
		HTTP(func() {
			POST("/auth/presentation/request/{state}")
			GET("/auth/presentation/request/{state}")
			SkipResponseBodyEncodeDecode()
			Response(StatusOK, func() {
				ContentType("application/oauth-authz-req+jwt")
			})
		})
	})

	Method("presentationCallback", func() {
		Description("Handles the wallet direct-post response and completes the Hydra login flow after presentation verification.")
		NoSecurity()
		Payload(PresentationCallbackPayload)
		Result(func() {
			Attribute("redirect_uri", String, "Redirect URI used by the frontend to continue the Hydra OIDC flow after presentation handling")
		})
		HTTP(func() {
			POST("/auth/presentation/callback")
			Response(StatusOK)
		})
	})

	Method("pidPresentation", func() {
		Description("Starts a one-shot OpenID4VP PID presentation flow (no Hydra session).")
		NoSecurity()
		Result(PidPresentationResult)
		HTTP(func() {
			POST("/auth/pid/presentation")
			Response(StatusOK)
		})
	})

	Method("pidPresentationRenew", func() {
		Description("Extends an existing PID presentation state and returns an updated wallet link.")
		NoSecurity()
		Payload(func() {
			Attribute("state", String, "PID presentation state identifier")
			Required("state")
		})
		Result(PidPresentationResult)
		HTTP(func() {
			POST("/auth/pid/presentation/renew")
			Response(StatusOK)
		})
	})

	Method("pidPresentationRequest", func() {
		Description("Returns a signed OpenID4VP authorization request JWT for PID presentation.")
		NoSecurity()
		Payload(func() {
			Attribute("state", String, "PID presentation state identifier")
			Attribute("wallet_nonce", String, "Wallet-provided nonce echoed in the authorization request object when request_uri_method=post")
			Attribute("wallet_metadata", String, "Wallet metadata JSON submitted with request_uri_method=post")
			Required("state")
		})
		HTTP(func() {
			POST("/auth/pid/presentation/request/{state}")
			GET("/auth/pid/presentation/request/{state}")
			SkipResponseBodyEncodeDecode()
			Response(StatusOK, func() {
				ContentType("application/oauth-authz-req+jwt")
			})
		})
	})

	Method("pidPresentationCallback", func() {
		Description("Handles the wallet direct-post response for PID presentation verification.")
		NoSecurity()
		Payload(PresentationCallbackPayload)
		HTTP(func() {
			POST("/auth/pid/presentation/callback")
			Response(StatusNoContent)
		})
	})

	Method("pidPresentationStatus", func() {
		Description("Returns the current PID presentation status for frontend polling.")
		NoSecurity()
		Payload(PresentationStatePayload)
		Result(PresentationStatusResult)
		HTTP(func() {
			GET("/auth/pid/presentation/status")
			Param("state")
			Response(StatusOK)
		})
	})

})

var PresentationStatePayload = Type("PresentationStatePayload", func() {
	Description("OpenID4VP presentation state for status polling.")
	Attribute("state", String, "Presentation state identifier from the presentation response")
	Required("state")
})

var PresentationCallbackPayload = Type("PresentationCallbackPayload", func() {
	Description("Wallet direct-post of a verifiable presentation.")
	Attribute("state", String, "Login state identifier from the OpenID4VP request")
	Attribute("vp_token", String, "JSON object serialization keyed by DCQL credential-query id containing arrays of verifiable presentations")
	Attribute("error", String, "Error code when wallet could not return a verifiable presentation")
	Attribute("error_description", String, "Optional wallet-provided details for the error")
	Required("state")
})

var PresentationStatusResult = Type("PresentationStatusResult", func() {
	Attribute("state", String, "Presentation state identifier")
	Attribute("status", String, "Current presentation status: pending, complete, failed, or expired")
	Attribute("expires_in", Int, "Number of seconds remaining before the presentation state expires")
	Attribute("redirect_uri", String, "Redirect URI returned when the presentation status is complete")
	Attribute("error_message", String, "Error message returned when the presentation status is failed")
	Required("state", "status", "expires_in")
})

var PidPresentationResult = Type("PidPresentationResult", func() {
	Attribute("presentation_url", String, "Wallet deep link (openid4vp://) for QR code generation")
	Attribute("state", String, "PID presentation state identifier")
	Attribute("expires_in", Int, "Seconds until this presentation state expires")
	Required("presentation_url", "state", "expires_in")
})
