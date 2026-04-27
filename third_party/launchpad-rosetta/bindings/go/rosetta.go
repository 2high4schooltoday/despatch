package rosetta

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

type RosettaError struct {
	Message string
}

func (err *RosettaError) Error() string {
	return err.Message
}

type Gender string

const (
	GenderMasculine Gender = "masculine"
	GenderFeminine  Gender = "feminine"
	GenderNonBinary Gender = "non-binary"
)

type nativeArg struct {
	Type  string `json:"type"`
	Value any    `json:"value"`
}

type Args struct {
	values map[string]nativeArg
}

func NewArgs() *Args {
	return &Args{values: map[string]nativeArg{}}
}

func (args *Args) Text(name, value string) *Args {
	args.values[name] = nativeArg{Type: "text", Value: value}
	return args
}

func (args *Args) Cardinal(name string, value int64) *Args {
	args.values[name] = nativeArg{Type: "cardinal", Value: value}
	return args
}

func (args *Args) Ordinal(name string, value int64) *Args {
	args.values[name] = nativeArg{Type: "ordinal", Value: value}
	return args
}

func (args *Args) Gender(name string, value Gender) *Args {
	args.values[name] = nativeArg{Type: "gender", Value: string(value)}
	return args
}

func (args *Args) Select(name, value string) *Args {
	args.values[name] = nativeArg{Type: "select", Value: value}
	return args
}

func (args *Args) List(name string, value []string) *Args {
	copyValue := append([]string(nil), value...)
	args.values[name] = nativeArg{Type: "list", Value: copyValue}
	return args
}

func (args *Args) JSON() (string, error) {
	if args == nil || len(args.values) == 0 {
		return "", nil
	}
	data, err := json.Marshal(args.values)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type CatalogBuilder struct {
	handle uintptr
}

type Catalog struct {
	handle uintptr
}

type Translator struct {
	catalog *Catalog
	locale  string
}

type LocaleContext struct {
	locale        string
	catalog       *Catalog
	defaultBundle string
}

var (
	loadNativeOnce sync.Once
	loadNativeErr  error

	builderNew               func() uintptr
	builderFree              func(uintptr)
	builderAddRosettaString  func(uintptr, string, string, string) bool
	builderAddJSONCompat     func(uintptr, string, string, string, string) bool
	builderBuild             func(uintptr) uintptr
	catalogFree              func(uintptr)
	catalogHas               func(uintptr, string, string, string) bool
	catalogText              func(uintptr, string, string, string) uintptr
	catalogFormat            func(uintptr, string, string, string, string) uintptr
	catalogBundleLocalesJSON func(uintptr, string) uintptr
	localeEndonymFunc        func(string) uintptr
	localeEnglishNameFunc    func(string) uintptr
	lastErrorMessageFunc     func() uintptr
	stringFree               func(uintptr)
)

func repoRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "."
	}
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))
}

func libraryNames() []string {
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

func candidateLibraryPaths() []string {
	names := libraryNames()
	if len(names) == 0 {
		return nil
	}
	var candidates []string
	if explicit := os.Getenv("LP_I18N_LIBRARY_PATH"); explicit != "" {
		info, err := os.Stat(explicit)
		if err == nil && info.IsDir() {
			for _, name := range names {
				candidates = append(candidates, filepath.Join(explicit, name))
			}
		} else {
			candidates = append(candidates, explicit)
		}
	}
	root := repoRoot()
	for _, directory := range []string{"release", "debug"} {
		for _, name := range names {
			candidates = append(candidates, filepath.Join(root, "target", directory, name))
		}
	}
	return candidates
}

func resolveLibraryPath() (string, error) {
	candidates := candidateLibraryPaths()
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", &RosettaError{
		Message: "unable to locate the Launchpad Rosetta native library. Build it with `cargo build` or set LP_I18N_LIBRARY_PATH.\n" +
			"Searched:\n" + strings.Join(candidates, "\n"),
	}
}

func ensureNative() error {
	loadNativeOnce.Do(func() {
		libraryPath, err := resolveLibraryPath()
		if err != nil {
			loadNativeErr = err
			return
		}
		handle, err := purego.Dlopen(libraryPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			loadNativeErr = err
			return
		}
		purego.RegisterLibFunc(&builderNew, handle, "lp_i18n_catalog_builder_new")
		purego.RegisterLibFunc(&builderFree, handle, "lp_i18n_catalog_builder_free")
		purego.RegisterLibFunc(&builderAddRosettaString, handle, "lp_i18n_catalog_builder_add_rosetta_str")
		purego.RegisterLibFunc(&builderAddJSONCompat, handle, "lp_i18n_catalog_builder_add_json_compat_str")
		purego.RegisterLibFunc(&builderBuild, handle, "lp_i18n_catalog_builder_build")
		purego.RegisterLibFunc(&catalogFree, handle, "lp_i18n_catalog_free")
		purego.RegisterLibFunc(&catalogHas, handle, "lp_i18n_catalog_has")
		purego.RegisterLibFunc(&catalogText, handle, "lp_i18n_catalog_text")
		purego.RegisterLibFunc(&catalogFormat, handle, "lp_i18n_catalog_format")
		purego.RegisterLibFunc(&catalogBundleLocalesJSON, handle, "lp_i18n_catalog_bundle_locales_json")
		purego.RegisterLibFunc(&localeEndonymFunc, handle, "lp_i18n_locale_endonym")
		purego.RegisterLibFunc(&localeEnglishNameFunc, handle, "lp_i18n_locale_english_name")
		purego.RegisterLibFunc(&lastErrorMessageFunc, handle, "lp_i18n_last_error_message")
		purego.RegisterLibFunc(&stringFree, handle, "lp_i18n_string_free")
	})
	return loadNativeErr
}

func stringAt(pointer uintptr) string {
	if pointer == 0 {
		return ""
	}
	var bytes []byte
	for offset := uintptr(0); ; offset++ {
		value := *(*byte)(unsafe.Pointer(pointer + offset))
		if value == 0 {
			break
		}
		bytes = append(bytes, value)
	}
	return string(bytes)
}

func consumeString(pointer uintptr) string {
	if pointer == 0 {
		return ""
	}
	defer stringFree(pointer)
	return stringAt(pointer)
}

func lastNativeError() error {
	if err := ensureNative(); err != nil {
		return err
	}
	message := consumeString(lastErrorMessageFunc())
	if message == "" {
		message = "native lp_i18n call failed"
	}
	return &RosettaError{Message: message}
}

func expectNativeBool(ok bool) error {
	if ok {
		return nil
	}
	return lastNativeError()
}

func expectNativeString(pointer uintptr) (string, error) {
	if pointer == 0 {
		return "", lastNativeError()
	}
	return consumeString(pointer), nil
}

func NewCatalogBuilder() (*CatalogBuilder, error) {
	if err := ensureNative(); err != nil {
		return nil, err
	}
	handle := builderNew()
	if handle == 0 {
		return nil, lastNativeError()
	}
	builder := &CatalogBuilder{handle: handle}
	runtime.SetFinalizer(builder, func(value *CatalogBuilder) {
		_ = value.Close()
	})
	return builder, nil
}

func (builder *CatalogBuilder) AddRosettaString(bundle, sourceName, text string) error {
	if builder.handle == 0 {
		return &RosettaError{Message: "catalog builder is closed"}
	}
	return expectNativeBool(builderAddRosettaString(builder.handle, bundle, sourceName, text))
}

func (builder *CatalogBuilder) AddJSONCompatString(bundle, locale, sourceName, text string) error {
	if builder.handle == 0 {
		return &RosettaError{Message: "catalog builder is closed"}
	}
	return expectNativeBool(builderAddJSONCompat(builder.handle, bundle, locale, sourceName, text))
}

func (builder *CatalogBuilder) Build() (*Catalog, error) {
	if builder.handle == 0 {
		return nil, &RosettaError{Message: "catalog builder is closed"}
	}
	handle := builderBuild(builder.handle)
	builder.handle = 0
	if handle == 0 {
		return nil, lastNativeError()
	}
	catalog := &Catalog{handle: handle}
	runtime.SetFinalizer(catalog, func(value *Catalog) {
		_ = value.Close()
	})
	return catalog, nil
}

func (builder *CatalogBuilder) Close() error {
	if builder.handle != 0 {
		builderFree(builder.handle)
		builder.handle = 0
	}
	runtime.SetFinalizer(builder, nil)
	return nil
}

func (catalog *Catalog) Translator(locale string) *Translator {
	return &Translator{catalog: catalog, locale: locale}
}

func (catalog *Catalog) LocaleContext(locale, defaultBundle string) *LocaleContext {
	return &LocaleContext{locale: locale, catalog: catalog, defaultBundle: defaultBundle}
}

func (catalog *Catalog) Has(locale, bundle, key string) (bool, error) {
	if catalog.handle == 0 {
		return false, &RosettaError{Message: "catalog is closed"}
	}
	return catalogHas(catalog.handle, locale, bundle, key), nil
}

func (catalog *Catalog) Text(locale, bundle, key string) (string, error) {
	if catalog.handle == 0 {
		return "", &RosettaError{Message: "catalog is closed"}
	}
	return expectNativeString(catalogText(catalog.handle, locale, bundle, key))
}

func (catalog *Catalog) Format(locale, bundle, key string, args *Args) (string, error) {
	if catalog.handle == 0 {
		return "", &RosettaError{Message: "catalog is closed"}
	}
	payload, err := args.JSON()
	if err != nil {
		return "", err
	}
	return expectNativeString(catalogFormat(catalog.handle, locale, bundle, key, payload))
}

func (catalog *Catalog) BundleLocales(bundle string) ([]string, error) {
	if catalog.handle == 0 {
		return nil, &RosettaError{Message: "catalog is closed"}
	}
	raw, err := expectNativeString(catalogBundleLocalesJSON(catalog.handle, bundle))
	if err != nil {
		return nil, err
	}
	var locales []string
	if err := json.Unmarshal([]byte(raw), &locales); err != nil {
		return nil, err
	}
	return locales, nil
}

func (catalog *Catalog) Close() error {
	if catalog.handle != 0 {
		catalogFree(catalog.handle)
		catalog.handle = 0
	}
	runtime.SetFinalizer(catalog, nil)
	return nil
}

func (translator *Translator) Locale() string {
	return translator.locale
}

func (translator *Translator) Has(bundle, key string) (bool, error) {
	return translator.catalog.Has(translator.locale, bundle, key)
}

func (translator *Translator) Text(bundle, key string) (string, error) {
	return translator.catalog.Text(translator.locale, bundle, key)
}

func (translator *Translator) Format(bundle, key string, args *Args) (string, error) {
	return translator.catalog.Format(translator.locale, bundle, key, args)
}

func (locale *LocaleContext) Locale() string {
	return locale.locale
}

func (locale *LocaleContext) DefaultBundle() string {
	return locale.defaultBundle
}

func (locale *LocaleContext) WithLocale(next string) *LocaleContext {
	return &LocaleContext{locale: next, catalog: locale.catalog, defaultBundle: locale.defaultBundle}
}

func (locale *LocaleContext) Text(key string) string {
	value, err := locale.catalog.Text(locale.locale, locale.defaultBundle, key)
	if err != nil {
		return key
	}
	return value
}

func (locale *LocaleContext) Format(key string, args *Args) string {
	value, err := locale.catalog.Format(locale.locale, locale.defaultBundle, key, args)
	if err != nil {
		return key
	}
	return value
}

func (locale *LocaleContext) BundleText(bundle, key string) string {
	value, err := locale.catalog.Text(locale.locale, bundle, key)
	if err != nil {
		return key
	}
	return value
}

func (locale *LocaleContext) BundleFormat(bundle, key string, args *Args) string {
	value, err := locale.catalog.Format(locale.locale, bundle, key, args)
	if err != nil {
		return key
	}
	return value
}

func (locale *LocaleContext) BundleLocales(bundle string) ([]string, error) {
	return locale.catalog.BundleLocales(bundle)
}

func LocaleEndonym(locale string) (string, error) {
	if err := ensureNative(); err != nil {
		return "", err
	}
	return expectNativeString(localeEndonymFunc(locale))
}

func LocaleEnglishName(locale string) (string, error) {
	if err := ensureNative(); err != nil {
		return "", err
	}
	return expectNativeString(localeEnglishNameFunc(locale))
}
