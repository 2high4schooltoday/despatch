import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import koffi from "koffi";

export class RosettaError extends Error {
  constructor(message) {
    super(message);
    this.name = "RosettaError";
  }
}

export const Gender = Object.freeze({
  MASCULINE: "masculine",
  FEMININE: "feminine",
  NON_BINARY: "non-binary",
});

function repoRoot() {
  return path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
}

function libraryNames() {
  switch (process.platform) {
    case "darwin":
      return ["liblp_i18n.dylib"];
    case "linux":
      return ["liblp_i18n.so"];
    case "win32":
      return ["lp_i18n.dll", "liblp_i18n.dll"];
    default:
      throw new RosettaError(`unsupported platform: ${process.platform}`);
  }
}

function candidateLibraryPaths() {
  const candidates = [];
  const explicit = process.env.LP_I18N_LIBRARY_PATH;
  const names = libraryNames();
  if (explicit) {
    const resolved = path.resolve(explicit);
    if (fs.existsSync(resolved) && fs.statSync(resolved).isDirectory()) {
      for (const name of names) {
        candidates.push(path.join(resolved, name));
      }
    } else {
      candidates.push(resolved);
    }
  }
  const root = repoRoot();
  for (const directory of ["release", "debug"]) {
    for (const name of names) {
      candidates.push(path.join(root, "target", directory, name));
    }
  }
  return candidates;
}

function resolveLibraryPath() {
  for (const candidate of candidateLibraryPaths()) {
    if (fs.existsSync(candidate)) {
      return candidate;
    }
  }
  throw new RosettaError(
    "unable to locate the Launchpad Rosetta native library. Build it with `cargo build` " +
      "or set LP_I18N_LIBRARY_PATH.\n" +
      candidateLibraryPaths().join("\n"),
  );
}

const lib = koffi.load(resolveLibraryPath());
const stringFree = lib.func("void lp_i18n_string_free(char *value)");
const HeapString = koffi.disposable("LpI18nString", "str", stringFree);

const native = {
  lastErrorMessage: lib.func(`LpI18nString lp_i18n_last_error_message(void)`),
  localeEndonym: lib.func(`LpI18nString lp_i18n_locale_endonym(const char *locale)`),
  localeEnglishName: lib.func(`LpI18nString lp_i18n_locale_english_name(const char *locale)`),
  catalogBuilderNew: lib.func(`void *lp_i18n_catalog_builder_new(void)`),
  catalogBuilderFree: lib.func(`void lp_i18n_catalog_builder_free(void *builder)`),
  catalogBuilderAddRosettaString: lib.func(
    `bool lp_i18n_catalog_builder_add_rosetta_str(void *builder, const char *bundle, const char *source_name, const char *text)`,
  ),
  catalogBuilderAddJsonCompatString: lib.func(
    `bool lp_i18n_catalog_builder_add_json_compat_str(void *builder, const char *bundle, const char *locale, const char *source_name, const char *text)`,
  ),
  catalogBuilderBuild: lib.func(`void *lp_i18n_catalog_builder_build(void *builder)`),
  catalogFree: lib.func(`void lp_i18n_catalog_free(void *catalog)`),
  catalogHas: lib.func(
    `bool lp_i18n_catalog_has(void *catalog, const char *locale, const char *bundle, const char *key)`,
  ),
  catalogText: lib.func(
    `LpI18nString lp_i18n_catalog_text(void *catalog, const char *locale, const char *bundle, const char *key)`,
  ),
  catalogFormat: lib.func(
    `LpI18nString lp_i18n_catalog_format(void *catalog, const char *locale, const char *bundle, const char *key, const char *args_json)`,
  ),
  catalogBundleLocalesJson: lib.func(
    `LpI18nString lp_i18n_catalog_bundle_locales_json(void *catalog, const char *bundle)`,
  ),
};

function lastErrorMessage() {
  return native.lastErrorMessage() ?? "native lp_i18n call failed";
}

function expectHandle(handle) {
  if (!handle) {
    throw new RosettaError(lastErrorMessage());
  }
  return handle;
}

function expectBoolean(ok) {
  if (!ok) {
    throw new RosettaError(lastErrorMessage());
  }
}

function expectString(value) {
  if (value == null) {
    throw new RosettaError(lastErrorMessage());
  }
  return value;
}

const builderRegistry = new FinalizationRegistry((handle) => {
  if (handle) {
    native.catalogBuilderFree(handle);
  }
});

const catalogRegistry = new FinalizationRegistry((handle) => {
  if (handle) {
    native.catalogFree(handle);
  }
});

export class Args {
  constructor() {
    this.values = {};
  }

  text(name, value) {
    this.values[name] = { type: "text", value };
    return this;
  }

  cardinal(name, value) {
    this.values[name] = { type: "cardinal", value: Number(value) };
    return this;
  }

  ordinal(name, value) {
    this.values[name] = { type: "ordinal", value: Number(value) };
    return this;
  }

  gender(name, value) {
    this.values[name] = { type: "gender", value };
    return this;
  }

  select(name, value) {
    this.values[name] = { type: "select", value };
    return this;
  }

  list(name, value) {
    this.values[name] = { type: "list", value: [...value] };
    return this;
  }

  toJSON() {
    return JSON.stringify(this.values);
  }
}

export class CatalogBuilder {
  #handle;

  constructor() {
    this.#handle = expectHandle(native.catalogBuilderNew());
    builderRegistry.register(this, this.#handle, this);
  }

  addRosettaString(bundle, sourceName, text) {
    expectBoolean(
      native.catalogBuilderAddRosettaString(this.#assertOpen(), bundle, sourceName, text),
    );
    return this;
  }

  addJsonCompatString(bundle, locale, sourceName, text) {
    expectBoolean(
      native.catalogBuilderAddJsonCompatString(
        this.#assertOpen(),
        bundle,
        locale,
        sourceName,
        text,
      ),
    );
    return this;
  }

  build() {
    const handle = native.catalogBuilderBuild(this.#takeHandle());
    return new Catalog(expectHandle(handle));
  }

  close() {
    if (this.#handle) {
      native.catalogBuilderFree(this.#handle);
      builderRegistry.unregister(this);
      this.#handle = null;
    }
  }

  #assertOpen() {
    if (!this.#handle) {
      throw new RosettaError("catalog builder is closed");
    }
    return this.#handle;
  }

  #takeHandle() {
    const handle = this.#assertOpen();
    builderRegistry.unregister(this);
    this.#handle = null;
    return handle;
  }
}

export class Catalog {
  #handle;

  constructor(handle) {
    this.#handle = handle;
    catalogRegistry.register(this, this.#handle, this);
  }

  translator(locale) {
    return new Translator(this, locale);
  }

  localeContext(locale, defaultBundle) {
    return new LocaleContext(locale, this, defaultBundle);
  }

  has(locale, bundle, key) {
    return Boolean(native.catalogHas(this.#assertOpen(), locale, bundle, key));
  }

  text(locale, bundle, key) {
    return expectString(native.catalogText(this.#assertOpen(), locale, bundle, key));
  }

  format(locale, bundle, key, args = null) {
    return expectString(
      native.catalogFormat(
        this.#assertOpen(),
        locale,
        bundle,
        key,
        args ? args.toJSON() : "",
      ),
    );
  }

  bundleLocales(bundle) {
    return JSON.parse(
      expectString(native.catalogBundleLocalesJson(this.#assertOpen(), bundle)),
    );
  }

  close() {
    if (this.#handle) {
      native.catalogFree(this.#handle);
      catalogRegistry.unregister(this);
      this.#handle = null;
    }
  }

  #assertOpen() {
    if (!this.#handle) {
      throw new RosettaError("catalog is closed");
    }
    return this.#handle;
  }
}

export class Translator {
  constructor(catalog, locale) {
    this.catalog = catalog;
    this.locale = locale;
  }

  has(bundle, key) {
    return this.catalog.has(this.locale, bundle, key);
  }

  text(bundle, key) {
    return this.catalog.text(this.locale, bundle, key);
  }

  format(bundle, key, args = null) {
    return this.catalog.format(this.locale, bundle, key, args);
  }
}

export class LocaleContext {
  constructor(locale, catalog, defaultBundle) {
    this.locale = locale;
    this.catalog = catalog;
    this.defaultBundle = defaultBundle;
  }

  withLocale(locale) {
    return new LocaleContext(locale, this.catalog, this.defaultBundle);
  }

  text(key) {
    try {
      return this.catalog.text(this.locale, this.defaultBundle, key);
    } catch {
      return key;
    }
  }

  format(key, args = null) {
    try {
      return this.catalog.format(this.locale, this.defaultBundle, key, args);
    } catch {
      return key;
    }
  }

  bundleText(bundle, key) {
    try {
      return this.catalog.text(this.locale, bundle, key);
    } catch {
      return key;
    }
  }

  bundleFormat(bundle, key, args = null) {
    try {
      return this.catalog.format(this.locale, bundle, key, args);
    } catch {
      return key;
    }
  }

  bundleLocales(bundle) {
    return this.catalog.bundleLocales(bundle);
  }
}

export function localeEndonym(locale) {
  return expectString(native.localeEndonym(locale));
}

export function localeEnglishName(locale) {
  return expectString(native.localeEnglishName(locale));
}
