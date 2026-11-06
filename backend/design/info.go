package design

import (
	. "goa.design/goa/v3/dsl"
)

var _ = Service("Info", func() {
	Description("Build revision of this DCS instance")

	Method("get", func() {
		Description("Returns the git commit SHA this binary was built from")
		NoSecurity()
		Result(func() {
			Attribute("git_sha", String, "Git commit SHA, or unknown if not set at build time")
			Attribute("built_at", String, "RFC3339 build timestamp if set at build time")
			Required("git_sha")
		})
		HTTP(func() {
			GET("/info")
			Response(StatusOK)
		})
	})
})
