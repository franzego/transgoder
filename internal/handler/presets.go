package handler

import (
	"net/http"
	"strings"

	"github.com/franzego/transcoder/internal/models"
	"github.com/franzego/transcoder/internal/presets"
	"github.com/gin-gonic/gin"
)

// ListPresets godoc
// @Summary List available presets
// @Description Returns server-managed transcode presets
// @Tags presets
// @Produce json
// @Success 200 {object} models.ApiMessage
// @Router /presets [get]
func (h *Handler) ListPresets(c *gin.Context) {
	c.JSON(http.StatusOK, models.ApiMessage{
		Success:  true,
		Message:  "Presets fetched",
		Code:     http.StatusOK,
		Metadata: presets.List(),
	})
}

// GetPreset godoc
// @Summary Get a preset by id
// @Description Returns a single server-managed preset
// @Tags presets
// @Produce json
// @Param id path string true "Preset ID"
// @Success 200 {object} models.ApiMessage
// @Failure 404 {object} models.ApiMessage
// @Router /presets/{id} [get]
func (h *Handler) GetPreset(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	item, ok := presets.Get(id)
	if !ok {
		c.JSON(http.StatusNotFound, models.ApiMessage{
			Success:   false,
			Message:   "Preset not found",
			Code:      http.StatusNotFound,
			ErrorCode: models.ErrorCodePresetNotFound,
		})
		return
	}
	c.JSON(http.StatusOK, models.ApiMessage{
		Success:  true,
		Message:  "Preset fetched",
		Code:     http.StatusOK,
		Metadata: item,
	})
}
