package image

// payloadKey scopes the payload memo. The raster is resampled to
// cells × cell-pixel-size, so the same (id, target) encodes a
// different PNG once the terminal reports a different cell size.
type payloadKey struct {
	placeholderKey
	cellPxW int
	cellPxH int
}
