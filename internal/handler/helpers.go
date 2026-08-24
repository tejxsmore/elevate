package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func jsonError(c *gin.Context, status int, msg string) {
	c.JSON(status, gin.H{"error": msg})
}

func parseUUIDParam(c *gin.Context, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param(name))
	if err != nil {
		jsonError(c, http.StatusBadRequest, "invalid "+name)
		return uuid.UUID{}, false
	}
	return id, true
}

func parseLimitOffset(c *gin.Context) (int, int) {
	limit := 50
	offset := 0
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	if v := c.Query("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}
