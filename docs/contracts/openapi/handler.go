package apidocs

import (
	_ "embed"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/swaggest/swgui/v5emb"
)

//go:embed openapi.yaml
var specification []byte

const (
	specificationPath = "/openapi.yaml"
	documentationPath = "/docs/"
)

// Register exposes the canonical OpenAPI contract and a self-contained Swagger
// UI. The UI assets are embedded in the application binary and do not depend
// on a public CDN.
func Register(router *gin.Engine) {
	router.GET(specificationPath, func(ctx *gin.Context) {
		ctx.Header("Cache-Control", "no-cache")
		ctx.Data(http.StatusOK, "application/yaml; charset=utf-8", specification)
	})
	router.GET("/docs", func(ctx *gin.Context) {
		ctx.Redirect(http.StatusTemporaryRedirect, documentationPath)
	})
	router.Any(
		documentationPath+"*any",
		gin.WrapH(v5emb.New(
			"TransEV EV CMS Administrative API",
			specificationPath,
			documentationPath,
		)),
	)
}

// Specification returns a copy for contract validation without permitting a
// caller to mutate the embedded source.
func Specification() []byte {
	result := make([]byte, len(specification))
	copy(result, specification)
	return result
}
