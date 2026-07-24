package billing

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func RegisterRoutes(
	group *gin.RouterGroup,
	authService *auth.Service,
	service *Service,
) {
	handler := &Handler{service: service}
	group.Use(noStore, authService.Authenticate(), auth.RequirePlatform())
	group.GET("/cpos/:cpo_id/billing-account", handler.getAccount)
	group.PUT("/cpos/:cpo_id/billing-account", handler.setAccount)
	group.POST("/cpos/:cpo_id/invoices", handler.createInvoice)
	group.GET("/cpos/:cpo_id/invoices", handler.listInvoices)
	group.GET("/invoices/:invoice_id", handler.getInvoice)
	group.POST("/invoices/:invoice_id/issue", handler.issueInvoice)
	group.POST("/invoices/:invoice_id/void", handler.voidInvoice)
	group.POST("/cpos/:cpo_id/payments", handler.recordPayment)
	group.GET("/cpos/:cpo_id/payments", handler.listPayments)
	group.GET("/payments/:payment_id", handler.getPayment)
	group.POST("/payments/:payment_id/void", handler.voidPayment)
	group.GET("/cpos/:cpo_id/billing-timeline", handler.timeline)
}

func (handler *Handler) getAccount(ctx *gin.Context) {
	principal, cpoID, ok := principalAndID(ctx, "cpo_id", "invalid_cpo_id")
	if !ok {
		return
	}
	response, err := handler.service.GetAccount(ctx.Request.Context(), principal, cpoID)
	write(ctx, http.StatusOK, response, err)
}

func (handler *Handler) setAccount(ctx *gin.Context) {
	principal, cpoID, ok := principalAndID(ctx, "cpo_id", "invalid_cpo_id")
	if !ok {
		return
	}
	var request BillingAccountRequest
	if !decode(ctx, &request) {
		return
	}
	response, err := handler.service.SetAccount(ctx.Request.Context(), principal, cpoID, request)
	write(ctx, http.StatusOK, response, err)
}

func (handler *Handler) createInvoice(ctx *gin.Context) {
	principal, cpoID, ok := principalAndID(ctx, "cpo_id", "invalid_cpo_id")
	if !ok {
		return
	}
	var request CreateInvoiceRequest
	if !decode(ctx, &request) {
		return
	}
	response, err := handler.service.CreateInvoice(ctx.Request.Context(), principal, cpoID, request)
	write(ctx, http.StatusCreated, response, err)
}

func (handler *Handler) listInvoices(ctx *gin.Context) {
	principal, cpoID, ok := principalAndID(ctx, "cpo_id", "invalid_cpo_id")
	if !ok {
		return
	}
	response, err := handler.service.ListInvoices(ctx.Request.Context(), principal, cpoID)
	write(ctx, http.StatusOK, gin.H{"invoices": response}, err)
}

func (handler *Handler) getInvoice(ctx *gin.Context) {
	principal, invoiceID, ok := principalAndID(ctx, "invoice_id", "invalid_invoice_id")
	if !ok {
		return
	}
	response, err := handler.service.GetInvoice(ctx.Request.Context(), principal, invoiceID)
	write(ctx, http.StatusOK, response, err)
}

func (handler *Handler) issueInvoice(ctx *gin.Context) {
	principal, invoiceID, ok := principalAndID(ctx, "invoice_id", "invalid_invoice_id")
	if !ok {
		return
	}
	var request IssueInvoiceRequest
	if !decode(ctx, &request) {
		return
	}
	response, err := handler.service.IssueInvoice(ctx.Request.Context(), principal, invoiceID, request)
	write(ctx, http.StatusOK, response, err)
}

func (handler *Handler) voidInvoice(ctx *gin.Context) {
	principal, invoiceID, ok := principalAndID(ctx, "invoice_id", "invalid_invoice_id")
	if !ok {
		return
	}
	var request VoidInvoiceRequest
	if !decode(ctx, &request) {
		return
	}
	response, err := handler.service.VoidInvoice(ctx.Request.Context(), principal, invoiceID, request)
	write(ctx, http.StatusOK, response, err)
}

func (handler *Handler) recordPayment(ctx *gin.Context) {
	principal, cpoID, ok := principalAndID(ctx, "cpo_id", "invalid_cpo_id")
	if !ok {
		return
	}
	var request RecordPaymentRequest
	if !decode(ctx, &request) {
		return
	}
	response, err := handler.service.RecordPayment(ctx.Request.Context(), principal, cpoID, request)
	write(ctx, http.StatusCreated, response, err)
}

func (handler *Handler) listPayments(ctx *gin.Context) {
	principal, cpoID, ok := principalAndID(ctx, "cpo_id", "invalid_cpo_id")
	if !ok {
		return
	}
	response, err := handler.service.ListPayments(ctx.Request.Context(), principal, cpoID)
	write(ctx, http.StatusOK, gin.H{"payments": response}, err)
}

func (handler *Handler) getPayment(ctx *gin.Context) {
	principal, paymentID, ok := principalAndID(ctx, "payment_id", "invalid_payment_id")
	if !ok {
		return
	}
	response, err := handler.service.GetPayment(ctx.Request.Context(), principal, paymentID)
	write(ctx, http.StatusOK, response, err)
}

func (handler *Handler) voidPayment(ctx *gin.Context) {
	principal, paymentID, ok := principalAndID(ctx, "payment_id", "invalid_payment_id")
	if !ok {
		return
	}
	var request VoidPaymentRequest
	if !decode(ctx, &request) {
		return
	}
	response, err := handler.service.VoidPayment(
		ctx.Request.Context(),
		principal,
		paymentID,
		request,
	)
	write(ctx, http.StatusOK, response, err)
}

func (handler *Handler) timeline(ctx *gin.Context) {
	principal, cpoID, ok := principalAndID(ctx, "cpo_id", "invalid_cpo_id")
	if !ok {
		return
	}
	response, err := handler.service.Timeline(ctx.Request.Context(), principal, cpoID)
	write(ctx, http.StatusOK, response, err)
}

func principalAndID(
	ctx *gin.Context,
	parameter string,
	code string,
) (auth.Principal, uuid.UUID, bool) {
	principal, _ := auth.CurrentPrincipal(ctx)
	id, err := uuid.Parse(ctx.Param(parameter))
	if err != nil || id == uuid.Nil {
		writeError(ctx, &auth.APIError{
			Status: http.StatusBadRequest, Code: code,
			Message: "The identifier is invalid.",
		})
		return principal, uuid.Nil, false
	}
	return principal, id, true
}

func decode(ctx *gin.Context, destination any) bool {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 64*1024)
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(ctx, &auth.APIError{
			Status: http.StatusBadRequest, Code: "invalid_request",
			Message: "The request body is invalid.",
		})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(ctx, &auth.APIError{
			Status: http.StatusBadRequest, Code: "invalid_request",
			Message: "The request body must contain one JSON object.",
		})
		return false
	}
	return true
}

func write(ctx *gin.Context, status int, response any, err error) {
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(status, response)
}

func writeError(ctx *gin.Context, err error) {
	var apiError *auth.APIError
	if errors.As(err, &apiError) {
		ctx.JSON(apiError.Status, gin.H{
			"error": gin.H{"code": apiError.Code, "message": apiError.Message},
		})
		return
	}
	ctx.JSON(http.StatusInternalServerError, gin.H{
		"error": gin.H{
			"code": "internal_error", "message": "The request could not be completed.",
		},
	})
}

func noStore(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
	ctx.Next()
}
