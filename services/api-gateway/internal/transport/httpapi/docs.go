package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Afari-Richmond/payflow/services/api-gateway/internal/docs"
)

// scalarPage embeds a Scalar API Reference viewer pointed at our
// generated OpenAPI spec. Scalar reads the same spec swaggo generates —
// it's just a nicer renderer than the classic Swagger UI.
const scalarPage = `<!doctype html>
<html>
<head>
  <title>PayFlow API Gateway — API Reference</title>
  <meta charset="utf-8" />
</head>
<body>
  <script id="api-reference" data-url="/docs/swagger.json"></script>
  <script src="https://cdn.jsdelivr.net/npm/@scalar/api-reference"></script>
</body>
</html>`

// DocsHandler serves the interactive API reference UI.
func DocsHandler(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(scalarPage))
}

// SwaggerSpecHandler serves the generated OpenAPI spec as JSON, derived
// from the @-annotations on the handlers themselves.
func SwaggerSpecHandler(c *gin.Context) {
	c.Data(http.StatusOK, "application/json", []byte(docs.SwaggerInfo.ReadDoc()))
}
