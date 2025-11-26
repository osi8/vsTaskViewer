package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

// HTMLCache holds HTML files in memory
type HTMLCache struct {
	viewerHTML string
	errorPages map[int][]byte // status code -> HTML content
	mu         sync.RWMutex
}

// NewHTMLCache creates a new HTML cache and loads all HTML files
func NewHTMLCache(htmlDir string) (*HTMLCache, error) {
	cache := &HTMLCache{
		errorPages: make(map[int][]byte),
	}

	// Load viewer.html
	viewerFile := filepath.Join(htmlDir, "viewer.html")
	data, err := os.ReadFile(viewerFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read viewer.html: %w", err)
	}
	cache.viewerHTML = string(data)
	log.Printf("[HTML] Loaded viewer.html from %s", htmlDir)

	// Load error pages
	errorCodes := []int{400, 401, 404, 405, 500}
	for _, code := range errorCodes {
		errorFile := filepath.Join(htmlDir, fmt.Sprintf("%d.html", code))
		data, err := os.ReadFile(errorFile)
		if err != nil {
			log.Printf("[HTML] Warning: failed to read error page %s: %v (will use fallback)", errorFile, err)
			continue
		}
		cache.errorPages[code] = data
		log.Printf("[HTML] Loaded %d.html from %s", code, htmlDir)
	}

	return cache, nil
}

// GetViewerHTML returns the viewer HTML template
func (c *HTMLCache) GetViewerHTML() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.viewerHTML
}

// GetErrorPage returns the error page HTML for the given status code
func (c *HTMLCache) GetErrorPage(statusCode int) []byte {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.errorPages[statusCode]
}

// serveErrorHTML serves an HTML error page for the given status code
func serveErrorHTML(w http.ResponseWriter, statusCode int, htmlCache *HTMLCache) {
	html := htmlCache.GetErrorPage(statusCode)
	
	if html == nil {
		// Fallback to plain text if HTML file doesn't exist
		log.Printf("[HTML] Error page %d.html not found in cache, using fallback", statusCode)
		w.WriteHeader(statusCode)
		fmt.Fprintf(w, "Error %d", statusCode)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	w.Write(html)
}

// loadViewerHTML loads the viewer HTML template from cache
func loadViewerHTML(htmlCache *HTMLCache) (string, error) {
	html := htmlCache.GetViewerHTML()
	if html == "" {
		return "", fmt.Errorf("viewer.html not found in cache")
	}
	return html, nil
}

