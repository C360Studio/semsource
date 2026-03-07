package image

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
)

const (
	// defaultThumbnailMaxDim is the maximum pixel dimension (width or height)
	// for a generated thumbnail.
	defaultThumbnailMaxDim = 512

	// thumbnailMinSourceDim is the minimum source dimension below which no
	// thumbnail is generated — there is no benefit in scaling down tiny images.
	thumbnailMinSourceDim = 64
)

// generateAndStoreThumbnail creates a thumbnail of the image and stores it via
// h.store. It returns the storage key on success, or an empty string when no
// thumbnail is applicable (SVG, WebP, already-small images). The error return
// is non-nil only for unexpected failures; callers should treat it as advisory.
func (h *ImageHandler) generateAndStoreThumbnail(ctx context.Context, content []byte, path, root, instance string) (string, error) {
	ext := strings.ToLower(filepath.Ext(path))
	// SVG is XML-based; WebP requires a non-standard decoder. Skip both.
	if ext == ".svg" || ext == ".webp" {
		return "", nil
	}

	// Decode the source image to inspect its dimensions.
	src, _, err := image.Decode(bytes.NewReader(content))
	if err != nil {
		return "", fmt.Errorf("decode for thumbnail: %w", err)
	}

	bounds := src.Bounds()
	srcW, srcH := bounds.Dx(), bounds.Dy()

	// No thumbnail needed when the image already fits within the max dimension.
	if srcW <= defaultThumbnailMaxDim && srcH <= defaultThumbnailMaxDim {
		return "", nil
	}

	// No thumbnail for images that are too small in both dimensions.
	if srcW < thumbnailMinSourceDim && srcH < thumbnailMinSourceDim {
		return "", nil
	}

	// Scale dimensions while preserving the original aspect ratio.
	thumbW, thumbH := thumbnailDimensions(srcW, srcH, defaultThumbnailMaxDim)

	// Resize with bilinear interpolation for good quality at reasonable cost.
	dst := image.NewRGBA(image.Rect(0, 0, thumbW, thumbH))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, bounds, draw.Over, nil)

	// Encode as JPEG — good compression for photographic thumbnails.
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 80}); err != nil {
		return "", fmt.Errorf("encode thumbnail: %w", err)
	}

	thumbKey := fmt.Sprintf("images/%s/%s/thumbnail", slugify(root), instance)
	if err := h.store.Put(ctx, thumbKey, buf.Bytes()); err != nil {
		return "", fmt.Errorf("store thumbnail: %w", err)
	}

	return thumbKey, nil
}

// thumbnailDimensions returns the (width, height) for a thumbnail whose
// longest side equals maxDim, preserving the original aspect ratio.
func thumbnailDimensions(srcW, srcH, maxDim int) (int, int) {
	if srcW >= srcH {
		return maxDim, srcH * maxDim / srcW
	}
	return srcW * maxDim / srcH, maxDim
}
