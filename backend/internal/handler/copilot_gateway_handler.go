package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	pkghttputil "github.com/Wei-Shaw/sub2api/internal/pkg/httputil"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"go.uber.org/zap"
)

// CopilotGatewayHandler handles GitHub Copilot API gateway requests.
//
// It exposes OpenAI-compatible endpoints (/copilot/v1/chat/completions, /copilot/v1/models)
// that proxy to the GitHub Copilot API via CopilotGatewayService.
type CopilotGatewayHandler struct {
	gatewayService        *service.GatewayService
	copilotGatewayService *service.CopilotGatewayService
	billingCacheService   *service.BillingCacheService
	concurrencyHelper     *ConcurrencyHelper
	maxAccountSwitches    int
}

// NewCopilotGatewayHandler creates a new CopilotGatewayHandler.
func NewCopilotGatewayHandler(
	gatewayService *service.GatewayService,
	copilotGatewayService *service.CopilotGatewayService,
	concurrencyService *service.ConcurrencyService,
	billingCacheService *service.BillingCacheService,
	cfg *config.Config,
) *CopilotGatewayHandler {
	pingInterval := time.Duration(0)
	maxAccountSwitches := 3
	if cfg != nil {
		pingInterval = time.Duration(cfg.Concurrency.PingInterval) * time.Second
		if cfg.Gateway.MaxAccountSwitches > 0 {
			maxAccountSwitches = cfg.Gateway.MaxAccountSwitches
		}
	}
	return &CopilotGatewayHandler{
		gatewayService:        gatewayService,
		copilotGatewayService: copilotGatewayService,
		billingCacheService:   billingCacheService,
		concurrencyHelper:     NewConcurrencyHelper(concurrencyService, SSEPingFormatComment, pingInterval),
		maxAccountSwitches:    maxAccountSwitches,
	}
}

// ChatCompletions handles Copilot /v1/chat/completions endpoint (OpenAI-compatible).
//
// The ForcePlatform middleware must be applied on the route group to set the
// platform context to "copilot" so that account selection picks only Copilot accounts.
func (h *CopilotGatewayHandler) ChatCompletions(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	subject, ok := middleware2.GetAuthSubjectFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusInternalServerError, "api_error", "User context not found")
		return
	}
	reqLog := requestLogger(
		c,
		"handler.copilot_gateway.chat_completions",
		zap.Int64("user_id", subject.UserID),
		zap.Int64("api_key_id", apiKey.ID),
		zap.Any("group_id", apiKey.GroupID),
	)

	// Read request body
	body, err := pkghttputil.ReadRequestBodyWithPrealloc(c.Request)
	if err != nil {
		if maxErr, ok := extractMaxBytesError(err); ok {
			h.errorResponse(c, http.StatusRequestEntityTooLarge, "invalid_request_error", buildBodyTooLargeMessage(maxErr.Limit))
			return
		}
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to read request body")
		return
	}
	if len(body) == 0 {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Request body is empty")
		return
	}

	setOpsRequestContext(c, "", false, body)

	// Validate JSON
	if !gjson.ValidBytes(body) {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "Failed to parse request body")
		return
	}

	// Extract model
	modelResult := gjson.GetBytes(body, "model")
	if !modelResult.Exists() || modelResult.Type != gjson.String || modelResult.String() == "" {
		h.errorResponse(c, http.StatusBadRequest, "invalid_request_error", "model is required")
		return
	}
	reqModel := modelResult.String()

	reqStream := gjson.GetBytes(body, "stream").Bool()
	reqLog = reqLog.With(zap.String("model", reqModel), zap.Bool("stream", reqStream))

	setOpsRequestContext(c, reqModel, reqStream, body)

	// Check billing eligibility
	subscription, _ := middleware2.GetSubscriptionFromContext(c)
	if err := h.billingCacheService.CheckBillingEligibility(c.Request.Context(), apiKey.User, apiKey, apiKey.Group, subscription); err != nil {
		reqLog.Info("copilot.billing_eligibility_check_failed", zap.Error(err))
		status, code, message := billingErrorDetails(err)
		h.errorResponse(c, status, code, message)
		return
	}

	// Acquire user concurrency slot
	ctx := c.Request.Context()
	userReleaseFunc, userAcquired, err := h.concurrencyHelper.TryAcquireUserSlot(ctx, subject.UserID, subject.Concurrency)
	if err != nil {
		reqLog.Warn("copilot.user_slot_acquire_failed", zap.Error(err))
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Concurrency limit exceeded, please retry later")
		return
	}
	if !userAcquired {
		h.errorResponse(c, http.StatusTooManyRequests, "rate_limit_error", "Too many concurrent requests, please retry later")
		return
	}
	if userReleaseFunc != nil {
		defer userReleaseFunc()
	}

	// Select a Copilot account.
	// ForcePlatform middleware sets platform = "copilot" in context, so
	// SelectAccountForModelWithExclusions will only pick copilot accounts.
	failedAccountIDs := make(map[int64]struct{})
	switchCount := 0

	for {
		account, err := h.gatewayService.SelectAccountForModelWithExclusions(
			ctx,
			apiKey.GroupID,
			"",        // sessionHash — no sticky session for Copilot
			reqModel,
			failedAccountIDs,
		)
		if err != nil || account == nil {
			if len(failedAccountIDs) == 0 {
				reqLog.Warn("copilot.account_select_failed", zap.Error(err))
				h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available Copilot accounts")
			} else {
				h.errorResponse(c, http.StatusBadGateway, "upstream_error", "All Copilot accounts failed")
			}
			return
		}
		reqLog.Debug("copilot.account_selected",
			zap.Int64("account_id", account.ID),
			zap.String("account_name", account.Name))
		setOpsSelectedAccount(c, account.ID, account.Platform)

		// Forward request to Copilot API
		result, fwdErr := h.copilotGatewayService.ForwardChatCompletions(ctx, c, account, body)
		if fwdErr != nil {
			failedAccountIDs[account.ID] = struct{}{}
			switchCount++
			if switchCount >= h.maxAccountSwitches {
				reqLog.Warn("copilot.failover_exhausted",
					zap.Int64("account_id", account.ID),
					zap.Int("switch_count", switchCount),
					zap.Error(fwdErr))
				h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Upstream request failed")
				return
			}
			reqLog.Warn("copilot.upstream_failover_switching",
				zap.Int64("account_id", account.ID),
				zap.Int("switch_count", switchCount),
				zap.Error(fwdErr))
			continue
		}

		// Handle upstream error responses (non-2xx already forwarded to client by service)
		if result != nil && result.StatusCode != http.StatusOK {
			reqLog.Debug("copilot.request_completed_with_error",
				zap.Int64("account_id", account.ID),
				zap.Int("status", result.StatusCode))
			return
		}

		// Log usage (structured log for now — full billing integration can be added later)
		if result != nil && result.Usage != nil {
			slog.Info("copilot.usage",
				"account_id", account.ID,
				"user_id", subject.UserID,
				"api_key_id", apiKey.ID,
				"model", result.Model,
				"prompt_tokens", result.Usage.PromptTokens,
				"completion_tokens", result.Usage.CompletionTokens,
				"total_tokens", result.Usage.TotalTokens)
		}

		reqLog.Debug("copilot.request_completed",
			zap.Int64("account_id", account.ID),
			zap.Int("switch_count", switchCount))
		return
	}
}

// Models handles Copilot /v1/models endpoint.
func (h *CopilotGatewayHandler) Models(c *gin.Context) {
	apiKey, ok := middleware2.GetAPIKeyFromContext(c)
	if !ok {
		h.errorResponse(c, http.StatusUnauthorized, "authentication_error", "Invalid API key")
		return
	}

	reqLog := requestLogger(c, "handler.copilot_gateway.models",
		zap.Int64("api_key_id", apiKey.ID))

	ctx := c.Request.Context()

	// Select any active Copilot account to query models.
	// ForcePlatform middleware ensures only copilot accounts are returned.
	account, err := h.gatewayService.SelectAccountForModelWithExclusions(ctx, apiKey.GroupID, "", "", nil)
	if err != nil || account == nil {
		reqLog.Warn("copilot.models_account_select_failed", zap.Error(err))
		h.errorResponse(c, http.StatusServiceUnavailable, "api_error", "No available Copilot accounts")
		return
	}

	body, err := h.copilotGatewayService.ListModels(ctx, account)
	if err != nil {
		reqLog.Warn("copilot.models_request_failed",
			zap.Int64("account_id", account.ID),
			zap.Error(err))
		h.errorResponse(c, http.StatusBadGateway, "upstream_error", "Failed to list models")
		return
	}

	c.Data(http.StatusOK, "application/json", body)
}

// errorResponse returns OpenAI API format error response.
func (h *CopilotGatewayHandler) errorResponse(c *gin.Context, status int, errType, message string) {
	c.JSON(status, gin.H{
		"error": gin.H{
			"type":    errType,
			"message": message,
		},
	})
}
