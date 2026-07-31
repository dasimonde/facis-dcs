package design

import (
	. "goa.design/goa/v3/dsl"
)

var HSMKeyInfo = Type("HSMKeyInfo", func() {
	Description("One HSM-held key of this instance: its base CKA_LABEL, purpose, and active version")

	Attribute("label", String, "Base CKA_LABEL of the key in the HSM token")
	Attribute("purpose", String, "What the key signs or derives")
	Attribute("active_version", Int, "Active key version (pki_active_key_version); 1 while never rotated")
	Attribute("updated_at", String, "When the active version last changed (RFC3339); absent while never rotated")

	Required("label", "purpose", "active_version")
})

var KeyInventoryResponse = Type("KeyInventoryResponse", func() {
	Description("The instance's HSM key inventory")

	Attribute("keys", ArrayOfRequired(HSMKeyInfo), "All HSM-held keys with their purpose and active version")

	Required("keys")
})

// Key Inventory Service  (/admin/hsm-keys)
var _ = Service("KeyInventory", func() {
	Description("Read-only inventory of the HSM-held keys (DCS-NFR-SEC-14): labels, purposes, and active versions. Rotation itself is an operator procedure (key-management concept), not an API action.")

	Method("list", func() {
		Description("List the HSM-held keys with purpose, active version from pki_active_key_version, and the version's last-change timestamp.")
		Meta("dcs:requirements", "DCS-NFR-SEC-14")
		Security(JWTAuth, func() {
			Scope("Sys. Administrator")
		})
		Payload(func() {
			Token("token", String, "JWT token")
		})
		Result(KeyInventoryResponse)

		Error("internal_error", ErrorResult, "Internal server error")

		HTTP(func() {
			GET("/admin/hsm-keys")
			Response(StatusOK)
			Response("internal_error", StatusInternalServerError)
		})
	})
})
