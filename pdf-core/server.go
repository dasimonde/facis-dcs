package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	_ "embed"

	"gopkg.in/yaml.v3"

	"example.com/m/V2/compiler"
)

//go:embed docs/semantic-ontology/linkml/output/linkml.yaml.context.jsonld
var ontologyContext []byte

//go:embed docs/semantic-ontology/linkml/output/linkml.yaml.shacl.merged.ttl
var ontologySHACL []byte

//go:embed api/openapi3.yaml
var openAPI3YAML []byte

func newServer() http.Handler {
	compiler.SetSHACLBytes(ontologySHACL)
	ontologyBaseURL := os.Getenv("DCS_PDF_CORE_ONTOLOGY_BASE_URL")
	if ontologyBaseURL == "" {
		panic("DCS_PDF_CORE_ONTOLOGY_BASE_URL must be set")
	}
	compiler.SetContextDocument(ontologyBaseURL+"/ontology/dcs-pdf-core", ontologyContext)
	svc := &service{}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /version", svc.version)
	mux.HandleFunc("POST /render", svc.render)
	mux.HandleFunc("POST /verify", svc.verify)
	mux.HandleFunc("POST /verify/content", svc.verifyContent)
	mux.HandleFunc("POST /verify/content-match", svc.verifyContentMatch)
	mux.HandleFunc("POST /render/amendment", svc.renderAmendment)
	mux.HandleFunc("POST /render/reanchor", svc.renderReanchor)
	mux.HandleFunc("POST /c2pa/embed", svc.embedC2PASignatures)
	mux.HandleFunc("POST /evidence/embed", svc.embedEvidence)
	mux.HandleFunc("POST /evidence/extract", svc.extractEvidence)
	mux.HandleFunc("POST /claim", svc.claim)
	mux.HandleFunc("POST /manifest/extract", svc.extractManifest)
	mux.HandleFunc("POST /manifest/chain", svc.manifestChain)
	mux.HandleFunc("POST /payload/extract", svc.extractPayload)
	mux.HandleFunc("GET /ontology/dcs-pdf-core", svc.ontologyContext)
	mux.HandleFunc("GET /swagger.json", handleSwagger)
	mux.HandleFunc("GET /ui/", handleUI)
	mux.HandleFunc("GET /index.html", handleUI)
	mux.HandleFunc("GET /ontology/dcs-pdf-core.shacl", handleSHACL)
	mux.HandleFunc("GET /", handleRoot)

	return mux
}

func handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
}

func handleUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, swaggerHTML)
}

func handleSwagger(w http.ResponseWriter, _ *http.Request) {
	// Convert YAML source of truth to JSON for the Swagger UI.
	var doc interface{}
	if err := yaml.Unmarshal(openAPI3YAML, &doc); err != nil {
		http.Error(w, "failed to parse openapi spec", http.StatusInternalServerError)
		return
	}
	jsonBytes, err := json.Marshal(doc)
	if err != nil {
		http.Error(w, "failed to encode openapi spec", http.StatusInternalServerError)
		return
	}
	if pub := os.Getenv("DCS_PDF_CORE_PUBLIC_URL"); pub != "" {
		jsonBytes = bytes.ReplaceAll(jsonBytes, []byte("http://localhost:8080"), []byte(strings.TrimRight(pub, "/")))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(jsonBytes)
}

func handleSHACL(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/turtle; charset=utf-8")
	_, _ = w.Write(ontologySHACL)
}

const swaggerHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
	<title>DCS-PDF-CORE Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>
    :root { color-scheme: light; }
    body { margin: 0; background: linear-gradient(135deg, #f4efe4, #dce8e4); }
    #swagger-ui { min-height: 100vh; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: '/swagger.json',
      dom_id: '#swagger-ui',
      deepLinking: true,
      presets: [SwaggerUIBundle.presets.apis],
      // Force arraybuffer on all API requests so binary responses (application/pdf)
      // are not corrupted by UTF-8 text decoding. JSON error responses are decoded
      // back to text in the responseInterceptor so Swagger UI can still display them.
      requestInterceptor: function(req) {
        if (!req.loadSpec) { req.responseType = 'arraybuffer'; }
        return req;
      },
      responseInterceptor: function(res) {
        var ct = (res.headers && res.headers['content-type']) || '';
        if (res.data instanceof ArrayBuffer && !ct.includes('application/pdf')) {
          var text = new TextDecoder().decode(res.data);
          res.text = text;
          res.data = text;
          try { res.obj = JSON.parse(text); } catch(e) {}
        }
        return res;
      }
    });
  </script>
</body>
</html>
`
