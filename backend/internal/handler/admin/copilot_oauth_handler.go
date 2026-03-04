package admin

import (
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

// CopilotOAuthHandler handles Copilot device OAuth endpoints.
type CopilotOAuthHandler struct {
	copilotOAuthService *service.CopilotOAuthService
}

// NewCopilotOAuthHandler creates a new CopilotOAuthHandler.
func NewCopilotOAuthHandler(copilotOAuthService *service.CopilotOAuthService) *CopilotOAuthHandler {
	return &CopilotOAuthHandler{copilotOAuthService: copilotOAuthService}
}

// StartDeviceFlow initiates the GitHub device OAuth flow.
// POST /api/v1/admin/copilot/oauth/device-code
func (h *CopilotOAuthHandler) StartDeviceFlow(c *gin.Context) {
	session, sessionID, err := h.copilotOAuthService.StartDeviceFlow(c.Request.Context())
	if err != nil {
		response.InternalError(c, "Failed to start device flow: "+err.Error())
		return
	}

	response.Success(c, gin.H{
		"session_id":       sessionID,
		"user_code":        session.UserCode,
		"verification_uri": session.VerificationURI,
		"expires_in":       int(time.Until(session.ExpiresAt).Seconds()),
		"interval":         session.Interval,
	})
}

// PollDeviceFlowRequest is the request for polling device flow status.
type PollDeviceFlowRequest struct {
	SessionID string `json:"session_id" binding:"required"`
}

// PollDeviceFlow polls for the device flow completion.
// POST /api/v1/admin/copilot/oauth/poll
func (h *CopilotOAuthHandler) PollDeviceFlow(c *gin.Context) {
	var req PollDeviceFlowRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Invalid request: "+err.Error())
		return
	}

	token, result, err := h.copilotOAuthService.PollDeviceFlow(c.Request.Context(), req.SessionID)
	if err != nil {
		errMsg := err.Error()
		// These are expected polling states, not errors
		if strings.Contains(errMsg, "authorization_pending") {
			response.Success(c, gin.H{
				"status":  "pending",
				"message": "Waiting for user authorization...",
			})
			return
		}
		if strings.Contains(errMsg, "slow_down") {
			response.Success(c, gin.H{
				"status":  "slow_down",
				"message": "Please slow down polling...",
			})
			return
		}
		response.BadRequest(c, "Device flow error: "+errMsg)
		return
	}

	response.Success(c, gin.H{
		"status":       "complete",
		"github_token": token,
		"github_login": result.GitHubLogin,
		"github_name":  result.GitHubName,
		"github_id":    result.GitHubID,
	})
}
