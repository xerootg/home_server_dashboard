// Embedded static files and documentation for the dashboard.
package main

//go:generate npm install
//go:generate npm run build

import (
	"embed"
	"io/fs"
)

//go:embed static/app.js static/app.js.map static/index.html static/style.css static/favicon.svg
var staticFiles embed.FS

//go:embed docs/*
var docsFiles embed.FS

// getStaticFS returns a filesystem rooted at the static directory.
func getStaticFS() (fs.FS, error) {
	return fs.Sub(staticFiles, "static")
}

// getDocsFS returns a filesystem rooted at the docs directory.
func getDocsFS() (fs.FS, error) {
	return fs.Sub(docsFiles, "docs")
}

// readDocsFile reads a file from the embedded docs directory.
func readDocsFile(name string) ([]byte, error) {
	return docsFiles.ReadFile("docs/" + name)
}

// readStaticFile reads a file from the embedded static directory.
func readStaticFile(name string) ([]byte, error) {
	return staticFiles.ReadFile("static/" + name)
}
