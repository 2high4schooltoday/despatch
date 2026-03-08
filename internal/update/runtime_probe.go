package update

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"despatch/internal/config"
)

type updaterRuntimeProbeFunc func(context.Context, config.Config) updaterRuntimeStatus

type updaterRuntimeStatus struct {
	PathUnit          systemdUnitState
	ServiceUnit       systemdUnitState
	ServiceExecPath   string
	ServiceExecExists bool
	ProbeErr          error
}

type systemdUnitState struct {
	Name        string
	LoadState   string
	ActiveState string
	SubState    string
	Result      string
	ProbeErr    error
}

func (s updaterRuntimeStatus) Healthy() bool {
	if s.ProbeErr != nil {
		return false
	}
	if !s.PathUnit.Known() || !s.PathUnit.Active() {
		return false
	}
	if !s.ServiceUnit.Known() {
		return false
	}
	return s.ServiceExecExists
}

func (s updaterRuntimeStatus) ConfigDiagnostic(cfg config.Config) *ConfigDiagnostic {
	if s.ProbeErr != nil {
		return &ConfigDiagnostic{
			Reason:     "updater_runtime_probe_failed",
			Detail:     fmt.Sprintf("cannot inspect updater runtime via systemctl: %v", s.ProbeErr),
			RepairHint: updaterRuntimeRepairHint(),
		}
	}
	if !s.ServiceUnit.Known() {
		return &ConfigDiagnostic{
			Reason: "updater_service_missing",
			Detail: fmt.Sprintf(
				"despatch-updater.service is not known to systemd (load_state=%s)",
				stateOrUnknown(s.ServiceUnit.LoadState),
			),
			RepairHint: updaterUnitInstallRepairHint(cfg),
		}
	}
	if !s.PathUnit.Known() || !s.PathUnit.Active() {
		if s.PathUnit.TriggerLimited() || s.ServiceUnit.StartLimited() {
			return &ConfigDiagnostic{
				Reason:     "updater_path_trigger_limited",
				Detail:     updaterPathFailureDetail(s),
				RepairHint: updaterPathResetRepairHint(),
			}
		}
		return &ConfigDiagnostic{
			Reason:     "updater_path_inactive",
			Detail:     updaterPathFailureDetail(s),
			RepairHint: updaterPathActivationRepairHint(),
		}
	}
	if strings.TrimSpace(s.ServiceExecPath) == "" {
		return &ConfigDiagnostic{
			Reason:     "updater_worker_missing",
			Detail:     "cannot resolve ExecStart for despatch-updater.service from installed unit files",
			RepairHint: updaterServiceExecRepairHint(),
		}
	}
	if !s.ServiceExecExists {
		return &ConfigDiagnostic{
			Reason:     "updater_worker_missing",
			Detail:     fmt.Sprintf("despatch-updater.service resolves to missing executable %s", s.ServiceExecPath),
			RepairHint: updaterServiceExecRepairHint(),
		}
	}
	return nil
}

func (s updaterRuntimeStatus) StaleQueueError() string {
	if s.ProbeErr != nil {
		return fmt.Sprintf("queued request was not picked up because updater runtime inspection failed: %v", s.ProbeErr)
	}
	if !s.ServiceUnit.Known() {
		return "queued request was not picked up because despatch-updater.service is not installed or not known to systemd"
	}
	if !s.PathUnit.Known() || !s.PathUnit.Active() {
		if s.PathUnit.TriggerLimited() || s.ServiceUnit.StartLimited() {
			return fmt.Sprintf(
				"queued request was not picked up because despatch-updater.path hit a systemd trigger/start limit (%s)",
				updaterPathFailureSummary(s),
			)
		}
		return fmt.Sprintf("queued request was not picked up because %s", updaterPathFailureDetail(s))
	}
	if strings.TrimSpace(s.ServiceExecPath) == "" || !s.ServiceExecExists {
		return "queued request was not picked up because despatch-updater.service does not resolve to a working updater executable"
	}
	return "queued request was not picked up by updater runtime within 30s; requeue update"
}

func (s systemdUnitState) Known() bool {
	return isSystemdLoadStateKnown(s.LoadState)
}

func (s systemdUnitState) Active() bool {
	return strings.EqualFold(strings.TrimSpace(s.ActiveState), "active")
}

func (s systemdUnitState) TriggerLimited() bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(s.Result)), "trigger-limit")
}

func (s systemdUnitState) StartLimited() bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(s.Result)), "start-limit")
}

func defaultUpdaterRuntimeProbe(ctx context.Context, cfg config.Config) updaterRuntimeStatus {
	status := updaterRuntimeStatus{
		PathUnit:    inspectSystemdUnit(ctx, "despatch-updater.path"),
		ServiceUnit: inspectSystemdUnit(ctx, "despatch-updater.service"),
	}
	execPath, err := resolveUpdaterServiceExecPath(cfg)
	if err == nil {
		status.ServiceExecPath = execPath
	}
	if strings.TrimSpace(execPath) != "" {
		if _, statErr := os.Stat(execPath); statErr == nil {
			status.ServiceExecExists = true
		} else if !os.IsNotExist(statErr) && err == nil {
			status.ProbeErr = statErr
			return status
		}
	}
	if err != nil && !os.IsNotExist(err) {
		status.ProbeErr = err
	}
	if unitErr := firstNonNil(status.PathUnit.ProbeErr, status.ServiceUnit.ProbeErr); unitErr != nil {
		status.ProbeErr = unitErr
	}
	return status
}

func inspectSystemdUnit(ctx context.Context, unitName string) systemdUnitState {
	state := systemdUnitState{Name: strings.TrimSpace(unitName)}
	loadState, err := systemctlShowValue(ctx, unitName, "LoadState")
	if err != nil {
		state.LoadState = "unknown"
		state.ActiveState = "unknown"
		state.ProbeErr = fmt.Errorf("systemctl show %s failed: %w", strings.TrimSpace(unitName), err)
		return state
	}
	state.LoadState = loadState
	activeState, err := systemctlShowValue(ctx, unitName, "ActiveState")
	if err != nil {
		state.ActiveState = "unknown"
		state.ProbeErr = fmt.Errorf("systemctl show %s active state failed: %w", strings.TrimSpace(unitName), err)
		return state
	}
	state.ActiveState = activeState
	subState, err := systemctlShowValue(ctx, unitName, "SubState")
	if err == nil {
		state.SubState = subState
	}
	result, err := systemctlShowValue(ctx, unitName, "Result")
	if err == nil {
		state.Result = result
	}
	return state
}

func systemctlShowValue(ctx context.Context, unitName, property string) (string, error) {
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(checkCtx, "systemctl", "show", "--property="+strings.TrimSpace(property), "--value", strings.TrimSpace(unitName))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%v (%s)", err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func resolveUpdaterServiceExecPath(cfg config.Config) (string, error) {
	files := []string{updaterServiceUnitPath(cfg)}
	overrideDir := updaterServiceOverrideDir(cfg)
	entries, err := os.ReadDir(overrideDir)
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if err == nil {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			names = append(names, entry.Name())
		}
		sort.Strings(names)
		for _, name := range names {
			files = append(files, filepath.Join(overrideDir, name))
		}
	}
	execStart := ""
	foundAny := false
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		foundAny = true
		inService := false
		for _, line := range strings.Split(string(raw), "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
				continue
			}
			if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
				inService = strings.EqualFold(trimmed, "[Service]")
				continue
			}
			if !inService || !strings.HasPrefix(trimmed, "ExecStart=") {
				continue
			}
			value := strings.TrimSpace(strings.TrimPrefix(trimmed, "ExecStart="))
			if value == "" {
				execStart = ""
				continue
			}
			if execStart == "" {
				execStart = value
			}
		}
	}
	if !foundAny {
		return "", os.ErrNotExist
	}
	fields := strings.Fields(execStart)
	if len(fields) == 0 {
		return "", fmt.Errorf("no ExecStart found in updater service units")
	}
	return fields[0], nil
}

func updaterRuntimeRepairHint() string {
	return "run systemctl status despatch-updater.path --no-pager && systemctl status despatch-updater.service --no-pager"
}

func updaterUnitInstallRepairHint(cfg config.Config) string {
	return fmt.Sprintf(
		"install despatch-updater.path and despatch-updater.service into %s, run systemctl daemon-reload, then reset-failed and enable --now despatch-updater.path",
		shQuote(cfg.UpdateSystemdUnitDir),
	)
}

func updaterPathActivationRepairHint() string {
	return "run systemctl daemon-reload && systemctl reset-failed despatch-updater.service despatch-updater.path && systemctl enable --now despatch-updater.path"
}

func updaterPathResetRepairHint() string {
	return "run systemctl reset-failed despatch-updater.service despatch-updater.path && systemctl restart despatch-updater.path"
}

func updaterServiceExecRepairHint() string {
	return "reinstall the current release so /opt/despatch/despatch-update-worker or /opt/despatch/despatch matches despatch-updater.service, then run systemctl daemon-reload && systemctl reset-failed despatch-updater.service despatch-updater.path && systemctl restart despatch-updater.path"
}

func stateOrUnknown(v string) string {
	if strings.TrimSpace(v) == "" {
		return "unknown"
	}
	return strings.TrimSpace(v)
}

func markdownlessShQuote(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "'\"'\"'") + "'"
}

func firstNonNil(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func updaterPathFailureDetail(s updaterRuntimeStatus) string {
	return fmt.Sprintf("despatch-updater.path is not active (%s)", updaterPathFailureSummary(s))
}

func updaterPathFailureSummary(s updaterRuntimeStatus) string {
	parts := []string{
		fmt.Sprintf("path_load_state=%s", stateOrUnknown(s.PathUnit.LoadState)),
		fmt.Sprintf("path_active_state=%s", stateOrUnknown(s.PathUnit.ActiveState)),
	}
	if strings.TrimSpace(s.PathUnit.SubState) != "" {
		parts = append(parts, fmt.Sprintf("path_sub_state=%s", stateOrUnknown(s.PathUnit.SubState)))
	}
	if strings.TrimSpace(s.PathUnit.Result) != "" {
		parts = append(parts, fmt.Sprintf("path_result=%s", stateOrUnknown(s.PathUnit.Result)))
	}
	if strings.TrimSpace(s.ServiceUnit.ActiveState) != "" {
		parts = append(parts, fmt.Sprintf("service_active_state=%s", stateOrUnknown(s.ServiceUnit.ActiveState)))
	}
	if strings.TrimSpace(s.ServiceUnit.SubState) != "" {
		parts = append(parts, fmt.Sprintf("service_sub_state=%s", stateOrUnknown(s.ServiceUnit.SubState)))
	}
	if strings.TrimSpace(s.ServiceUnit.Result) != "" {
		parts = append(parts, fmt.Sprintf("service_result=%s", stateOrUnknown(s.ServiceUnit.Result)))
	}
	return strings.Join(parts, " ")
}
