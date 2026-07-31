package compiler

// RendererVersion is the semver identifier for this renderer build. Bump this
// when any change to the PDF rendering pipeline would produce different output
// bytes for the same JSON-LD input, so that cached PDFs can be invalidated.
const RendererVersion = "1.0.1"
