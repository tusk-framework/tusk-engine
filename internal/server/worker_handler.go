package server

import (
	"path/filepath"
	"strings"
)

// WorkerHandler is the request boundary used by the HTTP server.
// worker.Pool implements this interface in production.
type WorkerHandler interface {
	HandleRequest(map[string]interface{}) (map[string]interface{}, error)
}

func safePublicPath(projectRoot, publicDir, requestPath string) (string, bool) {
	root, err := filepath.Abs(filepath.Join(projectRoot, publicDir))
	if err != nil {
		return "", false
	}

	requestPath = strings.ReplaceAll(requestPath, "\\", "/")
	requestPath = strings.TrimPrefix(requestPath, "/")
	candidate, err := filepath.Abs(filepath.Join(root, filepath.FromSlash(requestPath)))
	if err != nil {
		return "", false
	}

	relative, err := filepath.Rel(root, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false
	}
	return candidate, true
}
