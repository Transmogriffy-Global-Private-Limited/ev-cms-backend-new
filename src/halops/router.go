package halops

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	cmsmiddleware "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/middleware"
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
		cmsmiddleware.LogHandledError(ctx, "hal_facts", factError.Code, factError.Status, err)
		ctx.JSON(factError.Status, gin.H{"error": gin.H{"code": factError.Code, "message": factError.Message}})
		return
	}
	var projectionError *FactProjectionError
	if errors.As(err, &projectionError) {
		cmsmiddleware.LogHandledError(ctx, "hal_facts", projectionError.Code, projectionError.Status, err)
		ctx.JSON(projectionError.Status, gin.H{"error": gin.H{"code": projectionError.Code, "message": projectionError.Message}})
		return
	}
	cmsmiddleware.LogHandledError(ctx, "hal_facts", "internal_error", http.StatusInternalServerError, err)
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "internal_error", "message": "The HAL fact could not be processed."}})
}
