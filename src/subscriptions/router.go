package subscriptions

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/Transmogriffy-Global-Private-Limited/ev-cms-backend-new/src/auth"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func RegisterRoutes(group *gin.RouterGroup, authService *auth.Service, service *Service) {
	handler := &Handler{service: service}
	group.Use(noStore, authService.Authenticate(), auth.RequirePlatform())

	group.POST("/plans", handler.createPlan)
	group.GET("/plans", handler.listPlans)
	group.GET("/plans/:plan_id", handler.getPlan)
	group.PUT("/plans/:plan_id/draft", handler.updateDraft)
	group.POST("/plans/:plan_id/publish", handler.publishPlan)
	group.POST("/plans/:plan_id/archive", handler.archivePlan)

	group.POST("/cpos/:cpo_id/subscription", handler.issue)
	group.GET("/cpos/:cpo_id/subscription", handler.getCurrent)
	group.POST("/cpos/:cpo_id/subscription/renew", handler.renew)
	group.POST("/cpos/:cpo_id/subscription/change-plan", handler.changePlan)
	group.POST("/cpos/:cpo_id/subscription/activate", handler.activate)
	group.POST("/cpos/:cpo_id/subscription/pause", handler.pause)
	group.POST("/cpos/:cpo_id/subscription/resume", handler.resume)
	group.POST("/cpos/:cpo_id/subscription/mark-past-due", handler.markPastDue)
	group.POST("/cpos/:cpo_id/subscription/expire", handler.expire)
	group.POST("/cpos/:cpo_id/subscription/cancel", handler.cancel)
	group.GET("/cpos/:cpo_id/subscription/history", handler.history)
	group.GET("/cpos/:cpo_id/entitlements", handler.entitlements)
	group.PUT("/cpos/:cpo_id/entitlement-overrides/:feature_key", handler.setOverride)
	group.DELETE("/cpos/:cpo_id/entitlement-overrides/:feature_key", handler.deleteOverride)
}

func (handler *Handler) createPlan(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	var request CreatePlanRequest
	if !decode(ctx, &request) {
		return
	}
	response, err := handler.service.CreatePlan(ctx.Request.Context(), principal, request)
	write(ctx, http.StatusCreated, response, err)
}

func (handler *Handler) listPlans(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	response, err := handler.service.ListPlans(ctx.Request.Context(), principal)
	write(ctx, http.StatusOK, gin.H{"plans": response}, err)
}

func (handler *Handler) getPlan(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	id, ok := parseID(ctx, "plan_id", "invalid_plan_id")
	if !ok {
		return
	}
	response, err := handler.service.GetPlan(ctx.Request.Context(), principal, id)
	write(ctx, http.StatusOK, response, err)
}

func (handler *Handler) updateDraft(ctx *gin.Context) {
	principal, _ := auth.CurrentPrincipal(ctx)
	id, ok := parseID(ctx, "plan_id", "invalid_plan_id")
	if !ok {
		return
	}
	var request UpdateDraftRequest
	if !decode(ctx, &request) {
		return
	}
	response, err := handler.service.UpdateDraft(ctx.Request.Context(), principal, id, request)
	write(ctx, http.StatusOK, response, err)
}

func (handler *Handler) publishPlan(ctx *gin.Context) {
	handler.planAction(ctx, handler.service.PublishPlan)
}
func (handler *Handler) archivePlan(ctx *gin.Context) {
	handler.planAction(ctx, handler.service.ArchivePlan)
}

func (handler *Handler) planAction(ctx *gin.Context, action func(context.Context, auth.Principal, uuid.UUID) (PlanView, error)) {
	principal, _ := auth.CurrentPrincipal(ctx)
	id, ok := parseID(ctx, "plan_id", "invalid_plan_id")
	if !ok {
		return
	}
	response, err := action(ctx.Request.Context(), principal, id)
	write(ctx, http.StatusOK, response, err)
}

func (handler *Handler) issue(ctx *gin.Context) {
	principal, cpoID, ok := principalAndCPO(ctx)
	if !ok {
		return
	}
	var request IssueRequest
	if !decode(ctx, &request) {
		return
	}
	response, err := handler.service.Issue(ctx.Request.Context(), principal, cpoID, request)
	write(ctx, http.StatusCreated, response, err)
}

func (handler *Handler) getCurrent(ctx *gin.Context) {
	principal, cpoID, ok := principalAndCPO(ctx)
	if !ok {
		return
	}
	response, err := handler.service.GetCurrent(ctx.Request.Context(), principal, cpoID)
	write(ctx, http.StatusOK, response, err)
}

func (handler *Handler) renew(ctx *gin.Context) {
	principal, cpoID, ok := principalAndCPO(ctx)
	if !ok {
		return
	}
	var request RenewRequest
	if !decode(ctx, &request) {
		return
	}
	response, err := handler.service.Renew(ctx.Request.Context(), principal, cpoID, request)
	write(ctx, http.StatusOK, response, err)
}

func (handler *Handler) changePlan(ctx *gin.Context) {
	principal, cpoID, ok := principalAndCPO(ctx)
	if !ok {
		return
	}
	var request ChangePlanRequest
	if !decode(ctx, &request) {
		return
	}
	response, err := handler.service.ChangePlan(ctx.Request.Context(), principal, cpoID, request)
	write(ctx, http.StatusOK, response, err)
}

func (handler *Handler) activate(ctx *gin.Context) { handler.transition(ctx, handler.service.Activate) }
func (handler *Handler) pause(ctx *gin.Context)    { handler.transition(ctx, handler.service.Pause) }
func (handler *Handler) resume(ctx *gin.Context)   { handler.transition(ctx, handler.service.Resume) }
func (handler *Handler) markPastDue(ctx *gin.Context) {
	handler.transition(ctx, handler.service.MarkPastDue)
}
func (handler *Handler) expire(ctx *gin.Context) { handler.transition(ctx, handler.service.Expire) }
func (handler *Handler) cancel(ctx *gin.Context) { handler.transition(ctx, handler.service.Cancel) }

func (handler *Handler) transition(ctx *gin.Context, action func(context.Context, auth.Principal, uuid.UUID, TransitionRequest) (SubscriptionView, error)) {
	principal, cpoID, ok := principalAndCPO(ctx)
	if !ok {
		return
	}
	var request TransitionRequest
	if !decode(ctx, &request) {
		return
	}
	response, err := action(ctx.Request.Context(), principal, cpoID, request)
	write(ctx, http.StatusOK, response, err)
}

func (handler *Handler) history(ctx *gin.Context) {
	principal, cpoID, ok := principalAndCPO(ctx)
	if !ok {
		return
	}
	response, err := handler.service.History(ctx.Request.Context(), principal, cpoID)
	write(ctx, http.StatusOK, gin.H{"history": response}, err)
}

func (handler *Handler) entitlements(ctx *gin.Context) {
	principal, cpoID, ok := principalAndCPO(ctx)
	if !ok {
		return
	}
	response, err := handler.service.EffectiveEntitlements(ctx.Request.Context(), principal, cpoID)
	write(ctx, http.StatusOK, response, err)
}

func (handler *Handler) setOverride(ctx *gin.Context) {
	principal, cpoID, ok := principalAndCPO(ctx)
	if !ok {
		return
	}
	var request OverrideRequest
	if !decode(ctx, &request) {
		return
	}
	response, err := handler.service.SetOverride(ctx.Request.Context(), principal, cpoID, ctx.Param("feature_key"), request)
	write(ctx, http.StatusOK, response, err)
}

func (handler *Handler) deleteOverride(ctx *gin.Context) {
	principal, cpoID, ok := principalAndCPO(ctx)
	if !ok {
		return
	}
	err := handler.service.DeleteOverride(ctx.Request.Context(), principal, cpoID, ctx.Param("feature_key"), strings.TrimSpace(ctx.Query("reason")))
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func principalAndCPO(ctx *gin.Context) (auth.Principal, uuid.UUID, bool) {
	principal, _ := auth.CurrentPrincipal(ctx)
	id, ok := parseID(ctx, "cpo_id", "invalid_cpo_id")
	return principal, id, ok
}

func parseID(ctx *gin.Context, parameter, code string) (uuid.UUID, bool) {
	id, err := uuid.Parse(ctx.Param(parameter))
	if err != nil || id == uuid.Nil {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: code, Message: "The identifier is invalid."})
		return uuid.Nil, false
	}
	return id, true
}

func decode(ctx *gin.Context, destination any) bool {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 32*1024)
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "The request body is invalid."})
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeError(ctx, &auth.APIError{Status: http.StatusBadRequest, Code: "invalid_request", Message: "The request body must contain one JSON object."})
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
		ctx.JSON(apiError.Status, gin.H{"error": gin.H{"code": apiError.Code, "message": apiError.Message}})
		return
	}
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"code": "internal_error", "message": "The request could not be completed."}})
}

func noStore(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	ctx.Header("Pragma", "no-cache")
	ctx.Next()
}
