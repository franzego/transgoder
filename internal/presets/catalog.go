package presets

import (
	"fmt"
	"sort"
	"strings"
)

type VideoOptions struct {
	Codec      string `json:"codec"`
	Resolution string `json:"resolution,omitempty"`
	Bitrate    int32  `json:"bitrate,omitempty"`
	Framerate  int32  `json:"framerate,omitempty"`
}

type Preset struct {
	ID          string       `json:"id"`
	Version     int          `json:"version"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Format      string       `json:"format"`
	Options     VideoOptions `json:"options"`
}

// Overrides represents user-provided values that can override the defaults in a preset. All fields are optional, and validation is performed on any provided values to ensure they are within acceptable parameters.
type Overrides struct {
	Codec      *string `json:"codec,omitempty"`
	Resolution *string `json:"resolution,omitempty"`
	Bitrate    *int32  `json:"bitrate,omitempty"`
	Framerate  *int32  `json:"framerate,omitempty"`
	Format     *string `json:"format,omitempty"`
}

// catalog is a hardcoded list of presets. Will neeed to transiton to a dynamic source (e.g. database or config file) if we want to support user-defined presets in the future.
var catalog = map[string]Preset{
	"web-h264-v1": {
		ID:          "web-h264-v1",
		Version:     1,
		Name:        "Web H264",
		Description: "Balanced web playback preset using H264",
		Format:      "mp4",
		Options: VideoOptions{
			Codec:      "h264",
			Resolution: "1080",
			Bitrate:    3500,
			Framerate:  30,
		},
	},
	"mobile-h264-v1": {
		ID:          "mobile-h264-v1",
		Version:     1,
		Name:        "Mobile H264",
		Description: "Mobile-friendly H264 preset",
		Format:      "mp4",
		Options: VideoOptions{
			Codec:      "h264",
			Resolution: "720",
			Bitrate:    1800,
			Framerate:  30,
		},
	},
	"archive-h265-v1": {
		ID:          "archive-h265-v1",
		Version:     1,
		Name:        "Archive H265",
		Description: "Storage-optimized H265 preset",
		Format:      "mp4",
		Options: VideoOptions{
			Codec:      "h265",
			Resolution: "1080",
			Bitrate:    2200,
			Framerate:  30,
		},
	},
}

func List() []Preset {
	items := make([]Preset, 0, len(catalog))
	for _, p := range catalog {
		items = append(items, p)
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})
	return items
}

func Get(id string) (Preset, bool) {
	p, ok := catalog[strings.TrimSpace(strings.ToLower(id))]
	return p, ok
}

// Resolve takes a preset ID and a set of optional overrides, and returns a fully-resolved Preset with all overrides applied. Validation is performed on the overrides to ensure they are within acceptable parameters.
func Resolve(presetID string, overrides Overrides) (Preset, error) {
	base, ok := Get(presetID)
	if !ok {
		return Preset{}, fmt.Errorf("unknown preset_id: %s", presetID)
	}
	resolved := base

	if overrides.Codec != nil {
		c := strings.ToLower(strings.TrimSpace(*overrides.Codec))
		if !isAllowedCodec(c) {
			return Preset{}, fmt.Errorf("unsupported codec override: %s", c)
		}
		resolved.Options.Codec = c
	}
	if overrides.Resolution != nil {
		r, err := normalizeResolution(*overrides.Resolution)
		if err != nil {
			return Preset{}, err
		}
		resolved.Options.Resolution = r
	}
	if overrides.Bitrate != nil {
		if *overrides.Bitrate <= 0 {
			return Preset{}, fmt.Errorf("bitrate must be greater than zero")
		}
		resolved.Options.Bitrate = *overrides.Bitrate
	}
	if overrides.Framerate != nil {
		if *overrides.Framerate <= 0 {
			return Preset{}, fmt.Errorf("framerate must be greater than zero")
		}
		resolved.Options.Framerate = *overrides.Framerate
	}
	if overrides.Format != nil {
		f := strings.ToLower(strings.TrimSpace(*overrides.Format))
		if f == "" {
			return Preset{}, fmt.Errorf("format cannot be empty")
		}
		resolved.Format = f
	}

	return resolved, nil
}

func normalizeResolution(raw string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "1080", "1080p", "1920x1080":
		return "1080", nil
	case "720", "720p", "1280x720":
		return "720", nil
	case "480", "480p", "854x480":
		return "480", nil
	default:
		return "", fmt.Errorf("resolution must be one of 480, 720, 1080")
	}
}

func isAllowedCodec(codec string) bool {
	switch codec {
	case "h264", "h265", "vp9", "av1":
		return true
	default:
		return false
	}
}
