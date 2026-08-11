package customerauth

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// RegisterHALFactRoutes registers the separate service-authenticated callback
// boundary. It deliberately does not reuse a customer or staff bearer.
func RegisterHALFactRoutes(group *gin.RouterGroup, service *Service) {
	group.POST("/hal-facts", func(ctx *gin.Context) {
		var envelope HALFactEnvelope
		if err := decodeJSON(ctx, &envelope); err != nil {
			writeError(ctx, invalidRequest(err))
			return
		}
		if ctx.GetHeader("Idempotency-Key") != envelope.FactID.String() {
			writeError(ctx, &APIError{http.StatusBadRequest, "invalid_hal_fact", "Idempotency-Key must equal fact_id."})
			return
		}
		authorization := ctx.GetHeader("Authorization")
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(ctx, &APIError{http.StatusUnauthorized, "hal_fact_authentication_required", "Service authentication is required."})
			return
		}
		bearer := strings.TrimPrefix(authorization, "Bearer ")
		if err := service.AcceptHALFact(ctx.Request.Context(), bearer, envelope); err != nil {
			writeError(ctx, err)
			return
		}
		ctx.Status(http.StatusNoContent)
	})
}
