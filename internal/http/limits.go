package http

const defaultResponseLimitBytes int64 = 16 << 20

const (
	ReleaseAssetResponseLimitBytes     int64 = 256 << 20
	ChecksumManifestResponseLimitBytes int64 = 1 << 20
)
