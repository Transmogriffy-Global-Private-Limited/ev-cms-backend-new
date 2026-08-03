package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestIDHeader correlates an HTTP response with its completion log.
const RequestIDHeader = "X-Request-ID"

const (
	requestIDKey    = "ev_cms_request_id"
	requestActorKey = "ev_cms_request_actor"
	requestErrorKey = "ev_cms_request_error_code"
	requestDebugKey = "ev_cms_request_debug_sink"
)

// RequestActor contains only trusted, non-secret authentication context that
// may be copied into the HTTP completion log.
type RequestActor struct {
	AuthScope  string
	UserID     string
	CPOID      string
	CustomerID string
	Role       string
}

type requestLogRecord struct {
	Timestamp     string  `json:"timestamp"`
	Level         string  `json:"level"`
	Event         string  `json:"event"`
	RequestID     string  `json:"request_id"`
	Method        string  `json:"method"`
	Route         string  `json:"route"`
	Status        int     `json:"status"`
	DurationMS    float64 `json:"duration_ms"`
	ResponseBytes int     `json:"response_bytes"`
	PeerIP        string  `json:"peer_ip,omitempty"`
	ClientIP      string  `json:"client_ip,omitempty"`
	ErrorCount    int     `json:"error_count,omitempty"`
	AuthScope     string  `json:"auth_scope,omitempty"`
	UserID        string  `json:"user_id,omitempty"`
	CPOID         string  `json:"cpo_id,omitempty"`
	CustomerID    string  `json:"customer_id,omitempty"`
	Role          string  `json:"role,omitempty"`
	ErrorCode     string  `json:"error_code,omitempty"`
}

type requestDebugRecord struct {
	Timestamp  string `json:"timestamp"`
	Level      string `json:"level"`
	Event      string `json:"event"`
	RequestID  string `json:"request_id"`
	Method     string `json:"method"`
	Route      string `json:"route"`
	Component  string `json:"component,omitempty"`
	Status     int    `json:"status,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	ErrorType  string `json:"error_type,omitempty"`
	ErrorClass string `json:"error_class,omitempty"`
	SQLState   string `json:"sql_state,omitempty"`
	PeerIP     string `json:"peer_ip,omitempty"`
	ClientIP   string `json:"client_ip,omitempty"`
}

type requestDebugSink struct {
	logger *log.Logger
	now    func() time.Time
}

// RequestLogger emits one safe JSON completion record for every Gin request.
func RequestLogger(writer io.Writer, debugEnabled bool) gin.HandlerFunc {
	return newRequestLogger(writer, debugEnabled, time.Now, uuid.NewString)
}

func newRequestLogger(
	writer io.Writer,
	debugEnabled bool,
	now func() time.Time,
	newRequestID func() string,
) gin.HandlerFunc {
	if writer == nil {
		writer = io.Discard
	}
	accessLogger := log.New(writer, "", 0)

	return func(ctx *gin.Context) {
		startedAt := now()
		requestID := strings.TrimSpace(newRequestID())
		if requestID == "" {
			requestID = uuid.NewString()
		}
		ctx.Set(requestIDKey, requestID)
		ctx.Header(RequestIDHeader, requestID)
		peerIP := requestPeerIP(ctx.Request.RemoteAddr)
		if debugEnabled {
			sink := &requestDebugSink{logger: accessLogger, now: now}
			ctx.Set(requestDebugKey, sink)
			writeRequestDebugRecord(sink, requestDebugRecord{
				Timestamp: startedAt.UTC().Format(time.RFC3339Nano),
				Level:     "DEBUG",
				Event:     "http_request_started",
				RequestID: requestID,
				Method:    ctx.Request.Method,
				Route:     requestRoute(ctx),
				PeerIP:    peerIP,
				ClientIP:  requestClientIP(ctx.Request, peerIP),
			})
		}

		ctx.Next()

		completedAt := now()
		duration := completedAt.Sub(startedAt)
		if duration < 0 {
			duration = 0
		}
		responseBytes := ctx.Writer.Size()
		if responseBytes < 0 {
			responseBytes = 0
		}
		record := requestLogRecord{
			Timestamp:     completedAt.UTC().Format(time.RFC3339Nano),
			Level:         requestLogLevel(ctx.Writer.Status()),
			Event:         "http_request_completed",
			RequestID:     requestID,
			Method:        ctx.Request.Method,
			Route:         requestRoute(ctx),
			Status:        ctx.Writer.Status(),
			DurationMS:    float64(duration.Microseconds()) / 1000,
			ResponseBytes: responseBytes,
			PeerIP:        peerIP,
			ClientIP:      requestClientIP(ctx.Request, peerIP),
			ErrorCount:    len(ctx.Errors),
		}
		if actor, ok := requestActorFromContext(ctx); ok {
			record.AuthScope = actor.AuthScope
			record.UserID = actor.UserID
			record.CPOID = actor.CPOID
			record.CustomerID = actor.CustomerID
			record.Role = actor.Role
		}
		if errorCode, ok := requestErrorCode(ctx); ok {
			record.ErrorCode = errorCode
		}

		payload, err := json.Marshal(record)
		if err == nil {
			accessLogger.Print(string(payload))
		}
	}
}

// RequestID returns the server-generated correlation ID for the current Gin
// request. Endpoint-specific operational logs may include this value.
func RequestID(ctx *gin.Context) (string, bool) {
	value, exists := ctx.Get(requestIDKey)
	if !exists {
		return "", false
	}
	requestID, ok := value.(string)
	return requestID, ok && requestID != ""
}

// SetRequestActor enriches the completion record with trusted authentication
// context. Authentication middleware, not request handlers, owns this call.
func SetRequestActor(ctx *gin.Context, actor RequestActor) {
	ctx.Set(requestActorKey, actor)
}

func requestActorFromContext(ctx *gin.Context) (RequestActor, bool) {
	value, exists := ctx.Get(requestActorKey)
	if !exists {
		return RequestActor{}, false
	}
	actor, ok := value.(RequestActor)
	return actor, ok
}

// SetRequestErrorCode attaches a stable handled API error code to the
// completion record without logging the error message or response body.
func SetRequestErrorCode(ctx *gin.Context, code string) {
	code = strings.TrimSpace(code)
	if code != "" {
		ctx.Set(requestErrorKey, code)
	}
}

// LogHandledError attaches the stable API error code and, when LOG_LEVEL is
// DEBUG, emits a safe diagnostic containing only its component and Go type.
func LogHandledError(
	ctx *gin.Context,
	component string,
	code string,
	status int,
	err error,
) {
	SetRequestErrorCode(ctx, code)
	value, exists := ctx.Get(requestDebugKey)
	if !exists {
		return
	}
	sink, ok := value.(*requestDebugSink)
	if !ok || sink == nil {
		return
	}
	requestID, _ := RequestID(ctx)
	record := requestDebugRecord{
		Timestamp: sink.now().UTC().Format(time.RFC3339Nano),
		Level:     "DEBUG",
		Event:     "http_error_handled",
		RequestID: requestID,
		Method:    ctx.Request.Method,
		Route:     requestRoute(ctx),
		Component: component,
		Status:    status,
		ErrorCode: strings.TrimSpace(code),
	}
	if err != nil {
		record.ErrorType = fmt.Sprintf("%T", err)
		record.ErrorClass, record.SQLState = classifyRequestError(err)
	}
	writeRequestDebugRecord(sink, record)
}

func classifyRequestError(err error) (string, string) {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout", ""
	case errors.Is(err, context.Canceled):
		return "canceled", ""
	}
	type sqlStateError interface {
		SQLState() string
	}
	var databaseError sqlStateError
	if errors.As(err, &databaseError) {
		return "postgresql", strings.TrimSpace(databaseError.SQLState())
	}
	var networkError net.Error
	if errors.As(err, &networkError) {
		return "network", ""
	}
	return "application", ""
}

func writeRequestDebugRecord(
	sink *requestDebugSink,
	record requestDebugRecord,
) {
	payload, err := json.Marshal(record)
	if err == nil {
		sink.logger.Print(string(payload))
	}
}

func requestRoute(ctx *gin.Context) string {
	route := ctx.FullPath()
	if route == "" {
		return "<unmatched>"
	}
	return route
}

func requestErrorCode(ctx *gin.Context) (string, bool) {
	value, exists := ctx.Get(requestErrorKey)
	if !exists {
		return "", false
	}
	code, ok := value.(string)
	return code, ok && code != ""
}

func requestLogLevel(status int) string {
	switch {
	case status >= 500:
		return "ERROR"
	case status >= 400:
		return "WARN"
	default:
		return "INFO"
	}
}

func requestPeerIP(remoteAddress string) string {
	remoteAddress = strings.TrimSpace(remoteAddress)
	if remoteAddress == "" {
		return ""
	}
	host, _, err := net.SplitHostPort(remoteAddress)
	if err == nil {
		return host
	}
	return remoteAddress
}

func requestClientIP(request *http.Request, peerIP string) string {
	parsedPeerIP := net.ParseIP(peerIP)
	if parsedPeerIP == nil || !parsedPeerIP.IsLoopback() {
		return peerIP
	}
	forwardedFor := strings.Split(request.Header.Get("X-Forwarded-For"), ",")
	if len(forwardedFor) == 0 {
		return peerIP
	}
	candidate := strings.TrimSpace(forwardedFor[0])
	if net.ParseIP(candidate) == nil {
		return peerIP
	}
	return candidate
}
