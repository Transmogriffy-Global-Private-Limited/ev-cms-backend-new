package chargingtrace

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	cmsmiddleware "github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/middleware"
	"github.com/gin-gonic/gin"
)

// RegisterIngressRoutes is deliberately separate from RegisterFactRoutes. The
// trace bearer never authorizes authoritative HAL fact ingestion and vice versa.
func RegisterIngressRoutes(group *gin.RouterGroup, ingestor *Ingestor) {
	group.POST("/hal-trace-events", func(ctx *gin.Context) {
		var envelope Envelope
		decoder := json.NewDecoder(http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 32*1024))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&envelope); err != nil || decoder.Decode(&struct{}{}) != io.EOF {
			writeError(ctx, &Error{Status: 400, Code: "invalid_hal_trace_event", Message: "The trace envelope is invalid."})
			return
		}
		if ctx.GetHeader("Idempotency-Key") != envelope.EventID.String() {
			writeError(ctx, &Error{Status: 400, Code: "invalid_hal_trace_event", Message: "Idempotency-Key must equal event_id."})
			return
		}
		authorization := ctx.GetHeader("Authorization")
		if !strings.HasPrefix(authorization, "Bearer ") {
			writeError(ctx, &Error{Status: 401, Code: "hal_trace_authentication_required", Message: "Trace service authentication is required."})
			return
		}
		if err := ingestor.Accept(ctx.Request.Context(), strings.TrimPrefix(authorization, "Bearer "), envelope); err != nil {
			writeError(ctx, err)
			return
		}
		ctx.Status(http.StatusNoContent)
	})
}

func writeError(ctx *gin.Context, err error) {
	var traceError *Error
	if errors.As(err, &traceError) {
		cmsmiddleware.LogHandledError(ctx, "hal_trace", traceError.Code, traceError.Status, err)
		ctx.JSON(traceError.Status, gin.H{"error": gin.H{"code": traceError.Code, "message": traceError.Message}})
		return
	}
	cmsmiddleware.LogHandledError(ctx, "hal_trace", "internal_error", http.StatusInternalServerError, err)
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "internal_error", "message": "The trace event could not be processed."}})
}
