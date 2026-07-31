package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	goahttp "goa.design/goa/v3/http"
)

func mountSwaggerUI(mux goahttp.Muxer) {
	apiPathPrefix := getAPIPathPrefix()

	mux.Handle("GET", "/swagger", func(w http.ResponseWriter, r *http.Request) {
		// Build dynamic swagger HTML with correct OpenAPI spec path
		specURL := "./openapi3.json"
		html := buildSwaggerHTML(specURL)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, err := w.Write([]byte(html))
		if err != nil {
			log.Println("Failed to write response:", err)
			return
		}
	})
	mux.Handle("GET", "/swagger/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "./swagger", http.StatusMovedPermanently)
	})

	mux.Handle("GET", "/openapi3.json", func(w http.ResponseWriter, r *http.Request) {
		data, err := os.ReadFile("gen/http/openapi3.json")
		if err != nil {
			http.Error(w, "OpenAPI spec not found", http.StatusNotFound)
			return
		}

		// Parse and modify the OpenAPI spec to inject the correct server URL
		var spec map[string]interface{}
		if err := json.Unmarshal(data, &spec); err != nil {
			http.Error(w, "Failed to parse OpenAPI spec", http.StatusInternalServerError)
			return
		}

		// Rewrite the servers field to the runtime API path prefix; scheme comes
		// from X-Forwarded-Proto (proxy) or TLS status (direct).
		scheme := r.Header.Get("X-Forwarded-Proto")
		if scheme == "" {
			scheme = "http"
			if r.TLS != nil {
				scheme = "https"
			}
		}
		serverURL := scheme + "://" + r.Host + apiPathPrefix
		spec["servers"] = []map[string]interface{}{
			{"url": serverURL},
		}

		modifiedData, err := json.Marshal(spec)
		if err != nil {
			http.Error(w, "Failed to serialize OpenAPI spec", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		_, err = w.Write(modifiedData)
		if err != nil {
			log.Println("Failed to write response:", err)
			return
		}
	})
}

func buildSwaggerHTML(specURL string) string {
	return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <title>DCS API – Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css">
  <style>
    html { box-sizing: border-box; overflow-y: scroll; }
    *, *:before, *:after { box-sizing: inherit; }
    body { margin: 0; background: #fafafa; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    SwaggerUIBundle({
      url: "` + specURL + `",
      dom_id: "#swagger-ui",
      presets: [
        SwaggerUIBundle.presets.apis,
        SwaggerUIBundle.SwaggerUIStandalonePreset,
      ],
      layout: "BaseLayout",
      deepLinking: true,
    });
  </script>
</body>
</html>`
}
