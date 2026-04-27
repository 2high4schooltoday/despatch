# Launchpad Rosetta

Launchpad Rosetta is a standalone localization engine for applications that need more than simple string tables.

It parses Rosetta locale documents, resolves locale fallback chains, formats typed arguments, and supports selector-driven translations for plural, ordinal, gender, and custom selectors. The core is written in Rust and exposed through a native ABI with bindings for JavaScript, Python, PHP, and Go.

For pronouns, honorifics, and grammatical agreement, Rosetta now supports named selector sets through `selector-set:` headers and `select(name)` variable declarations. The built-in `gender` kind still exists for compact grammatical branching with the labels `masculine`, `feminine`, and `non-binary`.

## Why Use It

- structured locale files instead of ad hoc string maps
- typed formatting arguments for text, cardinal, ordinal, gender, select, and list values
- selector-based translations for language-aware phrasing
- named selector vocabularies for pronouns, honorifics, agreement classes, and other app-defined categories
- locale fallback resolution such as `en-US -> en`
- reusable native core shared across multiple language runtimes
- JSON compatibility loading for gradual migration from flat translation maps

## What You Get

- the Rust crate in [`src/lib.rs`](src/lib.rs)
- the native ABI in [`src/ffi.rs`](src/ffi.rs)
- the public C header in [`include/lp_i18n.h`](include/lp_i18n.h)
- bindings in:
  - [`bindings/javascript`](bindings/javascript)
  - [`bindings/python`](bindings/python)
  - [`bindings/php`](bindings/php)
  - [`bindings/go`](bindings/go)
- extra docs in:
  - [`docs/quickstart.md`](docs/quickstart.md)
  - [`docs/grammar.md`](docs/grammar.md)
  - [`docs/implementation.md`](docs/implementation.md)

## A Rosetta File

Rosetta documents are meant to be readable and expressive:

```lpr
--- launchpad-rosetta 1
locale: en
plural-rule: one other
selector-set: pronouns = he-him, she-her, they-them, ze-zir, xe-xem

hello = Hello, {name}!

reply
  {pronouns: select(pronouns)}
  | he-him = He replied
  | she-her = She replied
  | they-them = They replied
  | ze-zir = Ze replied
  | xe-xem = Xe replied
  | * = They replied
```

The engine loads one or more locale documents into bundles, then translates by `bundle + key + locale`.

## Rust Quickstart

Add the crate:

```toml
[dependencies]
lp_i18n = { path = "/path/to/launchpad-rosetta" }
```

Use it:

```rust
use lp_i18n::{Args, Catalog, LocaleContext};

let catalog = Catalog::builder()
    .add_rosetta_str(
        "project",
        "en.lpr",
        "--- launchpad-rosetta 1\nlocale: en\nplural-rule: one other\n\nhello = Hello, {name}!\n",
    )?
    .build()?;

let locale = LocaleContext::new("en", catalog, "project");
assert_eq!(
    locale.format("hello", Args::new().text("name", "world")),
    "Hello, world!"
);
# Ok::<(), anyhow::Error>(())
```

## Native Bindings

Build the native library first:

```bash
cargo build
```

This produces a shared library in `target/debug` and, with `cargo build --release`, in `target/release`.

Platform-specific output names:

- macOS: `liblp_i18n.dylib`
- Linux: `liblp_i18n.so`
- Windows: `lp_i18n.dll`

All non-Rust bindings look for the shared library in:

1. `LP_I18N_LIBRARY_PATH`
2. `target/release`
3. `target/debug`

If your compiled library lives elsewhere:

```bash
export LP_I18N_LIBRARY_PATH=/absolute/path/to/library/or/folder
```

## Binding Model

Each binding follows the same high-level API:

- `CatalogBuilder`: collect Rosetta or JSON-compat documents
- `Catalog`: immutable translation catalog
- `Translator`: strict locale-specific translator
- `LocaleContext`: convenience wrapper with a default bundle and key fallback behavior
- `Args`: typed formatting arguments

The common flow is:

1. create a `CatalogBuilder`
2. add one or more translation documents
3. `build()` the catalog
4. create a `Translator` or `LocaleContext`
5. call `text(...)` or `format(...)`

## Docs

- [`docs/quickstart.md`](docs/quickstart.md) for setup and first-use examples
- [`docs/grammar.md`](docs/grammar.md) for the full Rosetta file format and syntax rules
- [`docs/implementation.md`](docs/implementation.md) for engine and binding architecture
- binding-specific READMEs:
  - [`bindings/javascript/README.md`](bindings/javascript/README.md)
  - [`bindings/python/README.md`](bindings/python/README.md)
  - [`bindings/php/README.md`](bindings/php/README.md)
  - [`bindings/go/README.md`](bindings/go/README.md)

## Testing

Repository-level verification commands:

```bash
cargo test
PYTHONPATH=bindings/python python3 -m unittest discover -s bindings/python/tests
cd bindings/go && go test ./...
cd bindings/javascript && npm install && npm test
php -d ffi.enable=1 bindings/php/tests/basic.php
```

## Project Background

Launchpad Rosetta began inside the Launchpad codebase, but this repository is the standalone project: the reusable engine, native ABI, and cross-language bindings live here.
