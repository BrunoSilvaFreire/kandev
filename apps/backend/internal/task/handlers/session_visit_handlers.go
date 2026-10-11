package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/models"
)

func (h *TaskHandlers) httpReassignSessionStepVisit(c *gin.Context) {
	var request struct {
		VisitID         string `json:"visit_id"`
		ExpectedVisitID string `json:"expected_visit_id"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid visit assignment"})
		return
	}
	taskID, sessionID := c.Param("id"), c.Param("sessionId")
	if err := h.service.ReassignSessionStepVisit(c.Request.Context(), taskID, sessionID, request.VisitID, request.ExpectedVisitID); err != nil {
		switch {
		case errors.Is(err, models.ErrTaskSessionNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "task session not found"})
		case errors.Is(err, models.ErrStepVisitConflict):
			c.JSON(http.StatusConflict, gin.H{"error": "session visit changed; reload and retry"})
		case errors.Is(err, models.ErrStepVisitRetired), errors.Is(err, models.ErrStepVisitNotFound):
			c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "visit is not eligible for reassignment"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to associate session with visit"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"task_id": taskID, "session_id": sessionID, "visit_id": request.VisitID})
}
