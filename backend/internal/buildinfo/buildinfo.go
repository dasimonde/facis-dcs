package buildinfo

// Set at link time with go build -ldflags -X.
var (
	GitSHA  = "unknown"
	BuiltAt = ""
)
