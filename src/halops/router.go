package halops

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// RegisterFactRoutes owns the independently authenticated HAL-to-CMS ingress;
// it is intentionally outside both customer and administrative auth planes.
func RegisterFactRoutes(group *gin.RouterGroup, ingestor *FactIngestor) {
	group.POST("/hal-facts", func(ctx *gin.Context) {
		var envelope FactEnvelope
		decoder := json.NewDecoder(http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 32*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&envelope); err != nil {
			writeFactError(ctx, &FactError{Status: 400, Code: "invalid_hal_fact", Message: "The HAL fact envelope is invalid."})
			return
		}
		if ctx.GetHeader("Idempotency-Key") != envelope.FactID.String() {
			writeFactError(ctx, &FactError{Status: 400, Code: "invalid_hal_fact", Message: "Idempotency-Key must equal fact_id."})
			return
		}
		authorization := ctx.GetHeader("Authorization")
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeFactError(ctx, &FactError{Status: 401, Code: "hal_fact_authentication_required", Message: "Service authentication is required."})
			return
		}
		if err := ingestor.Accept(ctx.Request.Context(), strings.TrimPrefix(authorization, "Bearer "), envelope); err != nil {
			writeFactError(ctx, err)
			return
		}
		ctx.Status(http.StatusNoContent)
	})
}

func writeFactError(ctx *gin.Context, err error) {
	var factError *FactError
	if errors.As(err, &factError) {
		ctx.JSON(factError.Status, gin.H{"error": gin.H{"code": factError.Code, "message": factError.Message}})
		return
	}
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "internal_error", "message": "The HAL fact could not be processed."}})
}
