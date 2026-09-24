package handlers

import (
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRegisterHTTPRegistersProfileUtilizationRoute guards the batch
// utilization path against a gin router conflict with the sibling
// `/agent-profiles/:id/*` routes.
func TestRegisterHTTPRegistersProfileUtilizationRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	h := &Handlers{}
	h.registerHTTP(router)

	found := false
	for _, route := range router.Routes() {
		if route.Method == "POST" && route.Path == "/api/v1/agent-profiles/utilization" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("POST /api/v1/agent-profiles/utilization route is not registered")
	}
}
