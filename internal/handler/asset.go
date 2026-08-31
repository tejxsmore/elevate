package handler

import (
	"errors"
	"fmt"
	"net/http"
	"path"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"elevate/internal/models"
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

	c.JSON(
		http.StatusOK,
		gin.H{
			"asset": asset,
			"url":   nil,
		},
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

	if strings.TrimSpace(
		contentType,
	) == "" {
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

	c.JSON(
		http.StatusCreated,
		gin.H{
			"asset": asset,
			"url":   nil,
		},
	)
}

func (h *AssetHandler) Open(
	c *gin.Context,
) {
	h.stream(
		c,
		false,
	)
}

func (h *AssetHandler) Download(
	c *gin.Context,
) {
	h.stream(
		c,
		true,
	)
}

func (h *AssetHandler) stream(
	c *gin.Context,
	download bool,
) {
	id, ok := parseUUIDParam(
		c,
		"id",
	)
	if !ok {
		return
	}

	object, err := h.assets.Open(
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
			http.StatusBadGateway,
			err.Error(),
		)
		return
	}

	defer object.Body.Close()

	filename := assetFilename(
		object.Asset,
	)

	disposition := "inline"

	if download {
		disposition = "attachment"
	}

	c.Header(
		"Content-Disposition",
		fmt.Sprintf(
			`%s; filename="%s"`,
			disposition,
			filename,
		),
	)

	c.Header(
		"Cache-Control",
		"private, max-age=300",
	)

	c.Header(
		"X-Content-Type-Options",
		"nosniff",
	)

	c.DataFromReader(
		http.StatusOK,
		object.ContentLength,
		object.ContentType,
		object.Body,
		nil,
	)
}

func (h *AssetHandler) Campaigns(
	c *gin.Context,
) {
	id, ok := parseUUIDParam(
		c,
		"id",
	)
	if !ok {
		return
	}

	campaigns, err := h.assets.AttachedCampaigns(
		c.Request.Context(),
		id,
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
			"campaigns": campaigns,
		},
	)
}

func assetFilename(
	asset models.Asset,
) string {
	name := strings.TrimSpace(
		asset.Name,
	)

	if name == "" {
		name = "asset"
	}

	name = path.Base(name)

	name = strings.ReplaceAll(
		name,
		"\r",
		"",
	)

	name = strings.ReplaceAll(
		name,
		"\n",
		"",
	)

	name = strings.ReplaceAll(
		name,
		`"`,
		"",
	)

	if path.Ext(name) == "" {
		storageExt := path.Ext(
			strings.TrimSpace(
				asset.StoragePath,
			),
		)

		if storageExt != "" {
			name += storageExt
		}
	}

	return name
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

	err := h.assets.Delete(
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
