package update

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"despatch/internal/config"
)

const updateQueuePickupGrace = 30 * time.Second

type pendingRequest struct {
	Path    string
	Request ApplyRequest
}

func requestQueuePattern(cfg config.Config) string {
	return filepath.Join(requestDir(cfg), "update-request-*.json")
}

func requestQueuePath(req ApplyRequest, cfg config.Config) string {
	requestID := sanitizePathToken(strings.TrimSpace(req.RequestID))
	if requestID == "" {
		requestID = "request"
	}
	ts := req.RequestedAt.UTC()
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	return filepath.Join(requestDir(cfg), fmt.Sprintf("update-request-%020d-%s.json", ts.UnixNano(), requestID))
}

func pendingRequestPaths(cfg config.Config) ([]string, error) {
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	add := func(path string) {
		clean := filepath.Clean(path)
		if clean == "" {
			return
		}
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}

	if _, err := os.Stat(requestPath(cfg)); err == nil {
		add(requestPath(cfg))
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	matches, err := filepath.Glob(requestQueuePattern(cfg))
	if err != nil {
		return nil, err
	}
	sort.Strings(matches)
	for _, match := range matches {
		add(match)
	}
	return out, nil
}

func pendingRequests(cfg config.Config) ([]pendingRequest, error) {
	paths, err := pendingRequestPaths(cfg)
	if err != nil {
		return nil, err
	}
	out := make([]pendingRequest, 0, len(paths))
	for _, path := range paths {
		var req ApplyRequest
		if err := readJSONFile(path, &req); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			req.Mode = ApplyModeApply
		}
		req.Mode = normalizeApplyMode(req.Mode)
		out = append(out, pendingRequest{
			Path:    path,
			Request: req,
		})
	}
	return out, nil
}

func firstPendingRequestPath(cfg config.Config) (string, error) {
	paths, err := pendingRequestPaths(cfg)
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", os.ErrNotExist
	}
	return paths[0], nil
}

func removePendingRequestPaths(cfg config.Config) error {
	paths, err := pendingRequestPaths(cfg)
	if err != nil {
		return err
	}
	for _, path := range paths {
		if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
			return removeErr
		}
	}
	return nil
}
