package update

import (
	"path/filepath"
	"strings"

	"despatch/internal/config"
)

func requestDir(cfg config.Config) string {
	return filepath.Join(cfg.UpdateBaseDir, "request")
}

func statusDir(cfg config.Config) string {
	return filepath.Join(cfg.UpdateBaseDir, "status")
}

func lockDir(cfg config.Config) string {
	return filepath.Join(cfg.UpdateBaseDir, "lock")
}

func workDir(cfg config.Config) string {
	return filepath.Join(cfg.UpdateBaseDir, "work")
}

func backupsDir(cfg config.Config) string {
	return filepath.Join(cfg.UpdateBaseDir, "backups")
}

func requestPath(cfg config.Config) string {
	return filepath.Join(requestDir(cfg), "update-request.json")
}

func statusPath(cfg config.Config) string {
	return filepath.Join(statusDir(cfg), "update-status.json")
}

func autoStatusPath(cfg config.Config) string {
	return filepath.Join(statusDir(cfg), "update-auto-status.json")
}

func lockPath(cfg config.Config) string {
	return filepath.Join(lockDir(cfg), "update.lock")
}

func updaterPathUnitPath(cfg config.Config) string {
	return filepath.Join(cfg.UpdateSystemdUnitDir, "despatch-updater.path")
}

func updaterServiceUnitPath(cfg config.Config) string {
	return filepath.Join(cfg.UpdateSystemdUnitDir, "despatch-updater.service")
}

func updaterServiceOverrideDir(cfg config.Config) string {
	return filepath.Join(cfg.UpdateSystemdUnitDir, "despatch-updater.service.d")
}

func installDeployDir(cfg config.Config) string {
	return filepath.Join(cfg.UpdateInstallDir, "deploy")
}

func preparedPayloadDir(cfg config.Config, version string) string {
	token := sanitizePathToken(version)
	if strings.TrimSpace(token) == "" {
		token = "latest"
	}
	return filepath.Join(workDir(cfg), "prepared-"+token)
}
