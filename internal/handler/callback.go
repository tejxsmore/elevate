package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"elevate/internal/repository"
)

type CallbackHandler struct {
	callbacks *repository.CallbackRepo
}

func NewCallbackHandler(callbacks *repository.CallbackRepo) *CallbackHandler {
	return &CallbackHandler{callbacks: callbacks}
}

func (h *CallbackHandler) List(c *gin.Context) {
	callbacks, err := h.callbacks.List(c.Request.Context())
	if err != nil {
		jsonError(c, http.StatusInternalServerError, err.Error())
		return
	}
	c.JSON(http.StatusOK, gin.H{"callbacks": callbacks})
}
