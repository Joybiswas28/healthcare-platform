// Package api serves the OpenAPI description of the HTTP API and a Swagger UI page.
package api

import (
	_ "embed"
	"net/http"
)

// Spec is the OpenAPI 3 document for this API.
//
//go:embed openapi.yaml
var Spec []byte

// SpecHandler serves GET /openapi.yaml.
func SpecHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(Spec)
}

// DocsHandler serves GET /docs: Swagger UI loaded from a pinned CDN version,
// reading /openapi.yaml. "Authorize" accepts the access token from login.
func DocsHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(docsPage))
}

const swaggerUI = "https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.17.14"

const docsPage = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Healthcare API docs</title>
<link rel="stylesheet" href="` + swaggerUI + `/swagger-ui.css">
<style>body { margin: 0; background: #fff; }</style>
</head>
<body>
<div id="docs"></div>
<script src="` + swaggerUI + `/swagger-ui-bundle.js" crossorigin="anonymous"></script>
<script>
  window.ui = SwaggerUIBundle({
    url: "/openapi.yaml",
    dom_id: "#docs",
    deepLinking: true,
    persistAuthorization: true,
    tryItOutEnabled: true,
    docExpansion: "none",
    filter: true,
  });
</script>
</body>
</html>
`
