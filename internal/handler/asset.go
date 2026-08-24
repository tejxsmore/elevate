package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"elevate/internal/service"
)

type AssetHandler struct {
	assets *service.AssetService
}

func NewAssetHandler(
	assets *service.AssetService,
) *AssetHandler {
	return &AssetHandler{
		assets: assets,
	}
}

func (h *AssetHandler) List(
	c *gin.Context,
) {
	assets, err := h.assets.List(
		c.Request.Context(),
	)
	if err != nil {
		jsonError(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	c.JSON(
		http.StatusOK,
		gin.H{
			"assets": assets,
		},
	)
}

func (h *AssetHandler) Get(
	c *gin.Context,
) {
	id, ok := parseUUIDParam(
		c,
		"id",
	)
	if !ok {
		return
	}

	asset, err := h.assets.Get(
		c.Request.Context(),
		id,
	)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		jsonError(
			c,
			http.StatusNotFound,
			"asset not found",
		)
		return
	}

	if err != nil {
		jsonError(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	url, err := h.assets.PublicURL(
		asset,
	)
	if err == nil {
		c.JSON(
			http.StatusOK,
			gin.H{
				"asset": asset,
				"url":   url,
			},
		)
		return
	}

	c.JSON(
		http.StatusOK,
		asset,
	)
}

func (h *AssetHandler) Upload(
	c *gin.Context,
) {
	name := strings.TrimSpace(
		c.PostForm("name"),
	)

	assetType := strings.TrimSpace(
		c.PostForm("asset_type"),
	)

	fileHeader, err := c.FormFile(
		"file",
	)
	if err != nil {
		jsonError(
			c,
			http.StatusBadRequest,
			"file is required",
		)
		return
	}

	file, err := fileHeader.Open()
	if err != nil {
		jsonError(
			c,
			http.StatusBadRequest,
			err.Error(),
		)
		return
	}

	defer file.Close()

	contentType := fileHeader.Header.Get(
		"Content-Type",
	)

	if strings.TrimSpace(contentType) == "" {
		contentType = "application/octet-stream"
	}

	asset, err := h.assets.Upload(
		c.Request.Context(),
		name,
		assetType,
		fileHeader.Filename,
		contentType,
		fileHeader.Size,
		file,
	)
	if err != nil {
		jsonError(
			c,
			http.StatusBadGateway,
			err.Error(),
		)
		return
	}

	url, _ := h.assets.PublicURL(
		asset,
	)

	c.JSON(
		http.StatusCreated,
		gin.H{
			"asset": asset,
			"url":   url,
		},
	)
}

func (h *AssetHandler) Delete(
	c *gin.Context,
) {
	id, ok := parseUUIDParam(
		c,
		"id",
	)
	if !ok {
		return
	}

	if err := h.assets.Delete(
		c.Request.Context(),
		id,
	); err != nil {
		if errors.Is(
			err,
			pgx.ErrNoRows,
		) {
			jsonError(
				c,
				http.StatusNotFound,
				"asset not found",
			)
			return
		}

		jsonError(
			c,
			http.StatusInternalServerError,
			err.Error(),
		)
		return
	}

	c.Status(
		http.StatusNoContent,
	)
}

func parseOptionalSize(
	value string,
) *int64 {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	size, err := strconv.ParseInt(
		value,
		10,
		64,
	)
	if err != nil {
		return nil
	}

	return &size
}
