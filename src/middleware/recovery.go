package middleware

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/gin-gonic/gin"
)

type panicLogRecord struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Event     string `json:"event"`
	RequestID string `json:"request_id,omitempty"`
	Method    string `json:"method"`
	Route     string `json:"route"`
	PanicType string `json:"panic_type"`
	Stack     string `json:"stack"`
}

// Recovery returns panic recovery middleware that emits a correlated JSON
// diagnostic without dumping the request or the recovered panic value.
func Recovery(writer io.Writer) gin.HandlerFunc {
	if writer == nil {
		writer = io.Discard
	}
	panicLogger := log.New(writer, "", 0)

	return gin.CustomRecoveryWithWriter(io.Discard, func(ctx *gin.Context, recovered any) {
		route := ctx.FullPath()
		if route == "" {
			route = "<unmatched>"
		}
		requestID, _ := RequestID(ctx)
		record := panicLogRecord{
			Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
			Level:     "ERROR",
			Event:     "http_panic_recovered",
			RequestID: requestID,
			Method:    ctx.Request.Method,
			Route:     route,
			PanicType: fmt.Sprintf("%T", recovered),
			Stack:     string(debug.Stack()),
		}
		if payload, err := json.Marshal(record); err == nil {
			panicLogger.Print(string(payload))
		}
		SetRequestErrorCode(ctx, "internal_error")
		ctx.AbortWithStatus(http.StatusInternalServerError)
	})
}
