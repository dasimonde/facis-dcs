package main

import (
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

	mountRedirects(mux, apiPathPrefix, uiBasePath)
	mountStaticHandler(mux, staticDir, uiBasePath)
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
func mountStaticHandler(mux goahttp.Muxer, staticDir, uiBasePath string) {
	uiPrefix := strings.TrimSuffix(uiBasePath, "/")

	serve := func(w http.ResponseWriter, r *http.Request) {
		serveFrontend(w, r, staticDir, uiPrefix)
	}

	if uiPrefix == "" || uiPrefix == "/" {
		mux.Handle("GET", "/", serve)
		mux.Handle("GET", "/*", serve)
	} else {
		mux.Handle("GET", uiPrefix+"/*", serve)
	}
}

func serveFrontend(w http.ResponseWriter, r *http.Request, staticDir, uiPrefix string) {
	path := r.URL.Path
	if uiPrefix != "" && uiPrefix != "/" {
		path = strings.TrimPrefix(path, uiPrefix)
	}
	if path == "" {
		path = "/"
	}

	path = filepath.Clean(path)
	path = strings.TrimPrefix(path, "/")
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

	if info, err := os.Stat(absFullPath); err == nil && !info.IsDir() {
		http.ServeFile(w, r, absFullPath)
		return
	}

	indexPath := filepath.Join(absStaticDir, "index.html")
	if _, err := os.Stat(indexPath); err == nil {
		http.ServeFile(w, r, indexPath)
		return
	}

	http.NotFound(w, r)
}
