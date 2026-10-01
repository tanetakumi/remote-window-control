package nativecapture

type SnapshotRequest struct {
	Handle     uint64
	OutputPath string
}

type SnapshotResult struct {
	Path   string `json:"path"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}
