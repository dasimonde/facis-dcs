package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"digital-contracting-service/internal/pathutil"

	goahttp "goa.design/goa/v3/http"
)

func mountFrontend(mux goahttp.Muxer) {
	const staticDir = "/app/web/dist"

	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		return
	}

	apiPathPrefix := pathutil.NormalizePath(os.Getenv("DCS_API_PATH"), "", false)
	uiBasePath := pathutil.NormalizePath(os.Getenv("DCS_UI_PATH"), "/ui/", true)

	apiBase := os.Getenv("DCS_API_PATH")
	if apiBase == "" {
		apiBase = "/"
	}

	mountRedirects(mux, apiPathPrefix, uiBasePath)
	mountStaticHandler(mux, staticDir, uiBasePath, apiBase)
}

// mountRedirects registers redirect handlers so that requests to the API root
// or to the bare UI prefix (without trailing slash) are sent to uiBasePath.
func mountRedirects(mux goahttp.Muxer, apiPathPrefix, uiBasePath string) {
	redirect := func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, uiBasePath, http.StatusMovedPermanently)
	}

	apiRoot := strings.TrimSuffix(apiPathPrefix, "/")

	// Redirect API root → UI, unless they already point to the same place.
	if uiBasePath != apiPathPrefix {
		if apiRoot == "" {
			mux.Handle("GET", "/", redirect)
		} else {
			mux.Handle("GET", apiRoot, redirect)
			mux.Handle("GET", apiRoot+"/", redirect)
		}
	}

	// Redirect bare UI prefix (without trailing slash) → uiBasePath.
	uiPrefix := strings.TrimSuffix(uiBasePath, "/")
	if uiPrefix != "" && uiPrefix != "/" {
		mux.Handle("GET", uiPrefix, redirect)
	}
}

// mountStaticHandler registers the catch-all handler that serves the built
// frontend assets and falls back to index.html for client-side routing.
func mountStaticHandler(mux goahttp.Muxer, staticDir, uiBasePath, apiBase string) {
	uiPrefix := strings.TrimSuffix(uiBasePath, "/")

	serve := func(w http.ResponseWriter, r *http.Request) {
		serveFrontend(w, r, staticDir, uiPrefix, apiBase)
	}

	if uiPrefix == "" || uiPrefix == "/" {
		mux.Handle("GET", "/", serve)
		mux.Handle("GET", "/*", serve)
	} else {
		mux.Handle("GET", uiPrefix+"/*", serve)
	}
}

// uiContentSecurityPolicy permits only same-origin scripts and connections.
// QR codes and bundled assets use data: and blob: URLs; Vue and the print view
// set inline styles. frame-ancestors 'none' and X-Frame-Options stop another
// site from framing the UI to trick a user into approving or signing.
const uiContentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; font-src 'self' data:; connect-src 'self'; object-src 'none'; " +
	"base-uri 'self'; form-action 'self'; frame-ancestors 'none'"

// serveFrontend serves the built UI. The deployment settings go into config.js
// (API base) and the base href of index.html (UI path) at serve time, so the
// image filesystem stays read-only.
func serveFrontend(w http.ResponseWriter, r *http.Request, staticDir, uiPrefix, apiBase string) {
	w.Header().Set("Content-Security-Policy", uiContentSecurityPolicy)
	w.Header().Set("X-Frame-Options", "DENY")

	path := r.URL.Path
	if uiPrefix != "" && uiPrefix != "/" {
		path = strings.TrimPrefix(path, uiPrefix)
	}
	if path == "" {
		path = "/"
	}

	path = filepath.Clean(path)
	path = strings.TrimPrefix(path, "/")
	if path == "config.js" {
		api, _ := json.Marshal(apiBase)
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		_, _ = fmt.Fprintf(w, "window.DCS_CONFIG = { API_BASE_URL: %s }\n", api)
		return
	}
	fullPath := filepath.Join(staticDir, path)

	absStaticDir, err := filepath.Abs(staticDir)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	absFullPath, err := filepath.Abs(fullPath)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	relPath, err := filepath.Rel(absStaticDir, absFullPath)
	if err != nil || strings.HasPrefix(relPath, "..") {
		http.NotFound(w, r)
		return
	}

	if info, err := os.Stat(absFullPath); err == nil && !info.IsDir() && path != "index.html" {
		http.ServeFile(w, r, absFullPath)
		return
	}

	index, err := os.ReadFile(filepath.Join(absStaticDir, "index.html")) // #nosec G304 -- fixed name in the static directory of the image
	if err != nil {
		http.NotFound(w, r)
		return
	}
	baseHref := uiPrefix + "/"
	if uiPrefix == "/" {
		baseHref = "/"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(bytes.ReplaceAll(index, []byte("__DCS_UI_BASE_PATH__"), []byte(html.EscapeString(baseHref))))
}
