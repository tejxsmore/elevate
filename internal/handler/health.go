package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"

	"elevate/internal/database"
)

type HealthHandler struct {
	db    *database.DB
	redis *redis.Client
}

func NewHealthHandler(db *database.DB, redisClient *redis.Client) *HealthHandler {
	return &HealthHandler{db: db, redis: redisClient}
}

func (h *HealthHandler) Health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	status := gin.H{"status": "ok", "time": time.Now().UTC()}
	httpStatus := http.StatusOK

	if err := h.db.Ping(ctx); err != nil {
		status["status"] = "degraded"
		status["database"] = err.Error()
		httpStatus = http.StatusServiceUnavailable
	} else {
		status["database"] = "ok"
	}

	if err := h.redis.Ping(ctx).Err(); err != nil {
		status["status"] = "degraded"
		status["redis"] = err.Error()
		httpStatus = http.StatusServiceUnavailable
	} else {
		status["redis"] = "ok"
	}

	c.JSON(httpStatus, status)
}
