package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
)

// serveErrorHTML serves an HTML error page for the given status code
func serveErrorHTML(w http.ResponseWriter, statusCode int, htmlDir string) {
	htmlFile := filepath.Join(htmlDir, fmt.Sprintf("%d.html", statusCode))
	
	file, err := os.Open(htmlFile)
	if err != nil {
		log.Printf("[HTML] Failed to open error page %s: %v", htmlFile, err)
		// Fallback to plain text if HTML file doesn't exist
		w.WriteHeader(statusCode)
		fmt.Fprintf(w, "Error %d", statusCode)
		return
	}
	defer file.Close()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	io.Copy(w, file)
}

// loadViewerHTML loads the viewer HTML template
func loadViewerHTML(htmlDir string) (string, error) {
	htmlFile := filepath.Join(htmlDir, "viewer.html")
	
	data, err := os.ReadFile(htmlFile)
	if err != nil {
		return "", fmt.Errorf("failed to read viewer.html: %w", err)
	}
	
	return string(data), nil
}

