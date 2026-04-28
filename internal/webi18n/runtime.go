package webi18n

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	rosetta "launchpad-rosetta/bindings/go"
)

const BundleFormat = "launchpad-rosetta-web-bundle-1"

var (
	inlineEntryPattern  = regexp.MustCompile(`^([A-Za-z0-9_]+)\s*=`)
	blockEntryPattern   = regexp.MustCompile(`^([A-Za-z0-9_]+)\s*$`)
	pluralHeaderPattern = regexp.MustCompile(`^\s+\{([A-Za-z0-9_.-]+):\s*(cardinal|ordinal)\s*\}\s*$`)
	pluralBranchPattern = regexp.MustCompile(`^\s+\|\s*([A-Za-z0-9_*.-]+)\s*=\s*(.*)$`)
)

type Runtime struct {
	bundle  string
	catalog *rosetta.Catalog
	locales map[string]ClientBundle
}

type ClientBundle struct {
	Format   string                 `json:"format"`
	Bundle   string                 `json:"bundle"`
	Locale   string                 `json:"locale"`
	Fallback string                 `json:"fallback,omitempty"`
	Entries  map[string]ClientEntry `json:"entries"`
}

type ClientEntry struct {
	Kind       string            `json:"kind"`
	Value      string            `json:"value,omitempty"`
	Arg        string            `json:"arg,omitempty"`
	PluralType string            `json:"pluralType,omitempty"`
	Forms      map[string]string `json:"forms,omitempty"`
}

type localeSource struct {
	fileName string
	path     string
	source   string
	locale   string
	fallback string
	lines    []string
	entries  []entryDefinition
}

type entryDefinition struct {
	key       string
	kind      string
	lineIndex int
}

type pluralDefinition struct {
	arg        string
	pluralType string
	forms      map[string]string
}

func Load(localesDir, bundle string) (*Runtime, error) {
	configureNativeLibraryPath(localesDir)
	sources, err := readLocaleSources(localesDir)
	if err != nil {
		return nil, err
	}
	builder, err := rosetta.NewCatalogBuilder()
	if err != nil {
		return nil, err
	}
	defer builder.Close()
	for _, source := range sources {
		if err := builder.AddRosettaString(bundle, source.fileName, source.source); err != nil {
			return nil, fmt.Errorf("load %s: %w", source.path, err)
		}
	}
	catalog, err := builder.Build()
	if err != nil {
		return nil, err
	}
	runtime := &Runtime{
		bundle:  bundle,
		catalog: catalog,
		locales: map[string]ClientBundle{},
	}
	for _, source := range sources {
		compiled, err := runtime.buildClientBundle(source)
		if err != nil {
			catalog.Close()
			return nil, err
		}
		runtime.locales[source.locale] = compiled
	}
	return runtime, nil
}

func configureNativeLibraryPath(localesDir string) {
	if libraryPathConfigured(strings.TrimSpace(os.Getenv("LP_I18N_LIBRARY_PATH"))) {
		return
	}
	for _, candidate := range nativeLibraryCandidates(localesDir) {
		if candidate == "" {
			continue
		}
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		_ = os.Setenv("LP_I18N_LIBRARY_PATH", candidate)
		return
	}
}

func libraryPathConfigured(value string) bool {
	if value == "" {
		return false
	}
	info, err := os.Stat(value)
	if err == nil && !info.IsDir() {
		return true
	}
	if err != nil || !info.IsDir() {
		return false
	}
	for _, name := range nativeLibraryNames() {
		candidate := filepath.Join(value, name)
		if candidateInfo, statErr := os.Stat(candidate); statErr == nil && !candidateInfo.IsDir() {
			return true
		}
	}
	return false
}

func nativeLibraryCandidates(localesDir string) []string {
	names := nativeLibraryNames()
	if len(names) == 0 {
		return nil
	}
	var roots []string
	if trimmed := strings.TrimSpace(localesDir); trimmed != "" {
		roots = append(roots, filepath.Clean(filepath.Join(trimmed, "..", "..")))
	}
	if exe, err := os.Executable(); err == nil {
		roots = append(roots, filepath.Dir(exe))
	}
	seenRoots := map[string]bool{}
	seenPaths := map[string]bool{}
	var candidates []string
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" || seenRoots[root] {
			continue
		}
		seenRoots[root] = true
		for _, base := range []string{root, filepath.Join(root, "lib")} {
			for _, name := range names {
				candidate := filepath.Join(base, name)
				if seenPaths[candidate] {
					continue
				}
				seenPaths[candidate] = true
				candidates = append(candidates, candidate)
			}
		}
	}
	return candidates
}

func nativeLibraryNames() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"liblp_i18n.dylib"}
	case "linux":
		return []string{"liblp_i18n.so"}
	case "windows":
		return []string{"lp_i18n.dll", "liblp_i18n.dll"}
	default:
		return nil
	}
}

func (runtime *Runtime) Close() error {
	if runtime == nil || runtime.catalog == nil {
		return nil
	}
	err := runtime.catalog.Close()
	runtime.catalog = nil
	return err
}

func (runtime *Runtime) BundleFor(locale string) (ClientBundle, bool) {
	if runtime == nil {
		return ClientBundle{}, false
	}
	for _, candidate := range localeCandidates(locale) {
		bundle, ok := runtime.locales[candidate]
		if ok {
			return bundle, true
		}
	}
	return ClientBundle{}, false
}

func (runtime *Runtime) buildClientBundle(source localeSource) (ClientBundle, error) {
	translator := runtime.catalog.Translator(source.locale)
	entries := make(map[string]ClientEntry, len(source.entries))
	for _, entry := range source.entries {
		value, err := translator.Text(runtime.bundle, entry.key)
		if err == nil {
			entries[entry.key] = ClientEntry{
				Kind:  "text",
				Value: value,
			}
			continue
		}
		if entry.kind != "block" {
			return ClientBundle{}, fmt.Errorf("translate %s:%s: %w", source.locale, entry.key, err)
		}
		plural, pluralErr := extractPluralDefinition(source, entry)
		if pluralErr != nil {
			return ClientBundle{}, fmt.Errorf("parse plural %s:%s: %w", source.locale, entry.key, pluralErr)
		}
		entries[entry.key] = ClientEntry{
			Kind:       "plural",
			Arg:        plural.arg,
			PluralType: plural.pluralType,
			Forms:      plural.forms,
		}
	}
	bundle := ClientBundle{
		Format:  BundleFormat,
		Bundle:  runtime.bundle,
		Locale:  source.locale,
		Entries: entries,
	}
	if source.fallback != "" {
		bundle.Fallback = source.fallback
	}
	return bundle, nil
}

func readLocaleSources(localesDir string) ([]localeSource, error) {
	items, err := os.ReadDir(localesDir)
	if err != nil {
		return nil, err
	}
	var fileNames []string
	for _, item := range items {
		if item.IsDir() || !strings.HasSuffix(item.Name(), ".lpr") {
			continue
		}
		fileNames = append(fileNames, item.Name())
	}
	sort.Strings(fileNames)
	if len(fileNames) == 0 {
		return nil, fmt.Errorf("no .lpr locale files found in %s", localesDir)
	}
	var sources []localeSource
	seenLocales := map[string]string{}
	for _, fileName := range fileNames {
		absolutePath := filepath.Join(localesDir, fileName)
		raw, err := os.ReadFile(absolutePath)
		if err != nil {
			return nil, err
		}
		sourceText := string(raw)
		lines := strings.Split(strings.ReplaceAll(sourceText, "\r\n", "\n"), "\n")
		locale := extractHeaderValue(lines, "locale")
		if locale == "" {
			return nil, fmt.Errorf("missing locale header in %s", absolutePath)
		}
		if prior, exists := seenLocales[locale]; exists {
			return nil, fmt.Errorf("duplicate locale %s in %s and %s", locale, prior, absolutePath)
		}
		seenLocales[locale] = absolutePath
		entries, err := extractEntryDefinitions(lines, absolutePath)
		if err != nil {
			return nil, err
		}
		sources = append(sources, localeSource{
			fileName: fileName,
			path:     absolutePath,
			source:   sourceText,
			locale:   locale,
			fallback: extractHeaderValue(lines, "fallback"),
			lines:    lines,
			entries:  entries,
		})
	}
	return sources, nil
}

func extractHeaderValue(lines []string, name string) string {
	prefix := name + ":"
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, prefix) {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
	}
	return ""
}

func extractEntryDefinitions(lines []string, path string) ([]entryDefinition, error) {
	headerNames := map[string]bool{
		"locale":       true,
		"fallback":     true,
		"plural-rule":  true,
		"selector-set": true,
	}
	var entries []entryDefinition
	seenKeys := map[string]bool{}
	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		if strings.HasPrefix(trimmed, "--- launchpad-rosetta ") {
			continue
		}
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			continue
		}
		if parts := strings.SplitN(trimmed, ":", 2); len(parts) == 2 && headerNames[strings.TrimSpace(parts[0])] {
			continue
		}
		if match := inlineEntryPattern.FindStringSubmatch(line); len(match) == 2 {
			if seenKeys[match[1]] {
				return nil, fmt.Errorf("duplicate entry key %s in %s:%d", match[1], path, index+1)
			}
			seenKeys[match[1]] = true
			entries = append(entries, entryDefinition{key: match[1], kind: "inline", lineIndex: index})
			continue
		}
		if match := blockEntryPattern.FindStringSubmatch(line); len(match) == 2 {
			if seenKeys[match[1]] {
				return nil, fmt.Errorf("duplicate entry key %s in %s:%d", match[1], path, index+1)
			}
			seenKeys[match[1]] = true
			entries = append(entries, entryDefinition{key: match[1], kind: "block", lineIndex: index})
			continue
		}
		return nil, fmt.Errorf("unsupported top-level Rosetta line in %s:%d: %s", path, index+1, line)
	}
	return entries, nil
}

func extractPluralDefinition(source localeSource, entry entryDefinition) (pluralDefinition, error) {
	forms := map[string]string{}
	var arg string
	var pluralType string
	for index := entry.lineIndex + 1; index < len(source.lines); index += 1 {
		line := source.lines[index]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			break
		}
		if match := pluralHeaderPattern.FindStringSubmatch(line); len(match) == 3 {
			arg = match[1]
			pluralType = match[2]
			continue
		}
		if match := pluralBranchPattern.FindStringSubmatch(line); len(match) == 3 {
			category := match[1]
			if category == "*" {
				category = "other"
			}
			forms[category] = strings.TrimRight(match[2], " \t")
		}
	}
	if arg == "" || pluralType == "" || len(forms) == 0 {
		return pluralDefinition{}, fmt.Errorf("unsupported selector block for %s in %s:%d", entry.key, source.path, entry.lineIndex+1)
	}
	return pluralDefinition{
		arg:        arg,
		pluralType: pluralType,
		forms:      forms,
	}, nil
}

func localeCandidates(locale string) []string {
	normalized := normalizeLocaleTag(locale)
	if normalized == "" {
		return []string{"en"}
	}
	candidates := []string{normalized}
	if strings.Contains(normalized, "-") {
		candidates = append(candidates, strings.SplitN(normalized, "-", 2)[0])
	}
	if normalized != "en" {
		candidates = append(candidates, "en")
	}
	seen := map[string]bool{}
	var unique []string
	for _, candidate := range candidates {
		if candidate == "" || seen[candidate] {
			continue
		}
		seen[candidate] = true
		unique = append(unique, candidate)
	}
	return unique
}

func normalizeLocaleTag(value string) string {
	raw := strings.TrimSpace(strings.ReplaceAll(value, "_", "-"))
	if raw == "" {
		return "en"
	}
	parts := strings.Split(raw, "-")
	clean := make([]string, 0, len(parts))
	for index, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		switch {
		case index == 0:
			clean = append(clean, strings.ToLower(part))
		case len(part) == 2:
			clean = append(clean, strings.ToUpper(part))
		default:
			clean = append(clean, strings.ToLower(part))
		}
	}
	if len(clean) == 0 {
		return "en"
	}
	return strings.Join(clean, "-")
}
