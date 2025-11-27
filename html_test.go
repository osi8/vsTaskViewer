package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestNewHTMLCache(t *testing.T) {
	// Create temporary HTML directory
	tmpDir, err := os.MkdirTemp("", "html-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create viewer.html
	viewerHTML := `<!DOCTYPE html>
<html>
<head><title>Viewer</title></head>
<body>
	<h1>Task Viewer</h1>
	<p>Task ID: {{.TaskID}}</p>
	<p>WebSocket: {{.WebSocketURL}}</p>
</body>
</html>`
	if err := os.WriteFile(filepath.Join(tmpDir, "viewer.html"), []byte(viewerHTML), 0644); err != nil {
		t.Fatalf("Failed to create viewer.html: %v", err)
	}

	// Create error pages
	errorPages := []int{400, 401, 404, 405, 500}
	for _, code := range errorPages {
		errorHTML := `<html><body><h1>Error ` + strconv.Itoa(code) + `</h1></body></html>`
		filename := filepath.Join(tmpDir, strconv.Itoa(code)+".html")
		if err := os.WriteFile(filename, []byte(errorHTML), 0644); err != nil {
			t.Fatalf("Failed to create %d.html: %v", code, err)
		}
	}

	// Test loading HTML cache
	cache, err := NewHTMLCache(tmpDir)
	if err != nil {
		t.Fatalf("NewHTMLCache() = %v; want nil", err)
	}

	if cache == nil {
		t.Fatal("NewHTMLCache() = nil; want non-nil")
	}

	// Verify viewer HTML is loaded
	viewer := cache.GetViewerHTML()
	if viewer == "" {
		t.Error("NewHTMLCache() viewer HTML is empty")
	}
	if !containsString(viewer, "Task Viewer") {
		t.Errorf("NewHTMLCache() viewer HTML = %q; want to contain 'Task Viewer'", viewer)
	}

	// Verify error pages are loaded
	for _, code := range errorPages {
		page := cache.GetErrorPage(code)
		if page == nil {
			t.Errorf("NewHTMLCache() error page %d is nil", code)
		}
	}
}

func TestNewHTMLCacheMissingViewer(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "html-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Don't create viewer.html
	_, err = NewHTMLCache(tmpDir)
	if err == nil {
		t.Error("NewHTMLCache() with missing viewer.html = nil; want error")
	}
}

func TestNewHTMLCacheMissingErrorPages(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "html-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create only viewer.html
	viewerHTML := `<!DOCTYPE html><html><body>Viewer</body></html>`
	if err := os.WriteFile(filepath.Join(tmpDir, "viewer.html"), []byte(viewerHTML), 0644); err != nil {
		t.Fatalf("Failed to create viewer.html: %v", err)
	}

	// Should still work, error pages are optional
	cache, err := NewHTMLCache(tmpDir)
	if err != nil {
		t.Fatalf("NewHTMLCache() = %v; want nil (error pages are optional)", err)
	}

	// Error pages should be nil
	if cache.GetErrorPage(404) != nil {
		t.Error("NewHTMLCache() error page 404 should be nil when file doesn't exist")
	}
}

func TestHTMLCacheGetViewerHTML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "html-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	viewerHTML := `<html><body>Test Viewer</body></html>`
	if err := os.WriteFile(filepath.Join(tmpDir, "viewer.html"), []byte(viewerHTML), 0644); err != nil {
		t.Fatalf("Failed to create viewer.html: %v", err)
	}

	cache, err := NewHTMLCache(tmpDir)
	if err != nil {
		t.Fatalf("NewHTMLCache() = %v; want nil", err)
	}

	html := cache.GetViewerHTML()
	if html != viewerHTML {
		t.Errorf("HTMLCache.GetViewerHTML() = %q; want %q", html, viewerHTML)
	}
}

func TestHTMLCacheGetErrorPage(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "html-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	viewerHTML := `<html><body>Viewer</body></html>`
	if err := os.WriteFile(filepath.Join(tmpDir, "viewer.html"), []byte(viewerHTML), 0644); err != nil {
		t.Fatalf("Failed to create viewer.html: %v", err)
	}

	errorHTML := `<html><body>404 Not Found</body></html>`
	if err := os.WriteFile(filepath.Join(tmpDir, "404.html"), []byte(errorHTML), 0644); err != nil {
		t.Fatalf("Failed to create 404.html: %v", err)
	}

	cache, err := NewHTMLCache(tmpDir)
	if err != nil {
		t.Fatalf("NewHTMLCache() = %v; want nil", err)
	}

	page := cache.GetErrorPage(404)
	if page == nil {
		t.Fatal("HTMLCache.GetErrorPage(404) = nil; want non-nil")
	}

	if string(page) != errorHTML {
		t.Errorf("HTMLCache.GetErrorPage(404) = %q; want %q", string(page), errorHTML)
	}

	// Test non-existent error page
	page = cache.GetErrorPage(503)
	if page != nil {
		t.Error("HTMLCache.GetErrorPage(503) = non-nil; want nil")
	}
}

func TestServeErrorHTML(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "html-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	viewerHTML := `<html><body>Viewer</body></html>`
	if err := os.WriteFile(filepath.Join(tmpDir, "viewer.html"), []byte(viewerHTML), 0644); err != nil {
		t.Fatalf("Failed to create viewer.html: %v", err)
	}

	errorHTML := `<html><body>404 Not Found</body></html>`
	if err := os.WriteFile(filepath.Join(tmpDir, "404.html"), []byte(errorHTML), 0644); err != nil {
		t.Fatalf("Failed to create 404.html: %v", err)
	}

	cache, err := NewHTMLCache(tmpDir)
	if err != nil {
		t.Fatalf("NewHTMLCache() = %v; want nil", err)
	}

	// Use httptest to test serveErrorHTML
	w := httptest.NewRecorder()
	serveErrorHTML(w, http.StatusNotFound, cache)

	if w.Code != http.StatusNotFound {
		t.Errorf("serveErrorHTML() status = %d; want %d", w.Code, http.StatusNotFound)
	}

	if w.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Errorf("serveErrorHTML() Content-Type = %q; want %q", w.Header().Get("Content-Type"), "text/html; charset=utf-8")
	}

	if string(w.Body.Bytes()) != errorHTML {
		t.Errorf("serveErrorHTML() body = %q; want %q", string(w.Body.Bytes()), errorHTML)
	}
}

func TestServeErrorHTMLFallback(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "html-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	viewerHTML := `<html><body>Viewer</body></html>`
	if err := os.WriteFile(filepath.Join(tmpDir, "viewer.html"), []byte(viewerHTML), 0644); err != nil {
		t.Fatalf("Failed to create viewer.html: %v", err)
	}

	// Don't create error pages
	cache, err := NewHTMLCache(tmpDir)
	if err != nil {
		t.Fatalf("NewHTMLCache() = %v; want nil", err)
	}

	w := httptest.NewRecorder()
	serveErrorHTML(w, http.StatusNotFound, cache)

	if w.Code != http.StatusNotFound {
		t.Errorf("serveErrorHTML() status = %d; want %d", w.Code, http.StatusNotFound)
	}

	// Should use fallback
	body := string(w.Body.Bytes())
	if !containsString(body, "404") && body != "Error 404" {
		t.Logf("serveErrorHTML() fallback body = %q", body)
	}
}


