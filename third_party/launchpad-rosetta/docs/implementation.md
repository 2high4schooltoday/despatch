# Implementation Notes

This repository is split into three layers.

For the user-facing source format itself, see [`docs/grammar.md`](grammar.md).

## 1. Rust Engine

The Rust engine in [`src/lib.rs`](../src/lib.rs) owns:

- Rosetta parsing
- selector validation
- named selector-set parsing for closed custom vocabularies
- locale fallback resolution
- formatting logic
- bundle and translator APIs

This is the source of truth for behavior.

## 2. Native ABI

The C ABI in [`src/ffi.rs`](../src/ffi.rs) wraps the Rust engine in a minimal FFI-safe surface.

Design choices:

- opaque builder and catalog handles
- UTF-8 strings across the boundary
- explicit string free function for returned heap strings
- thread-local last-error storage for binding-friendly error retrieval
- JSON-based typed argument passing for `format(...)`

The public header is [`include/lp_i18n.h`](../include/lp_i18n.h).

## 3. Language Bindings

Bindings are intentionally thin wrappers over the native layer:

- JavaScript uses `koffi`
- Python uses `ctypes`
- PHP uses built-in `FFI`
- Go uses `purego`

Each binding preserves the same conceptual API so examples stay portable across languages.

Named selector sets are a grammar-level feature, so the bindings do not need special new runtime types for them. Applications continue to pass closed-selector values through the existing `Args.select(...)` APIs.

## Missing-Key Behavior

There are two important lookup modes:

- `Translator`: returns errors on missing keys or invalid formatting state
- `LocaleContext`: returns the key itself when lookup or formatting fails

That behavior matches the Rust implementation and is mirrored in the bindings.

## Testing Strategy

Current tests cover:

- Rust parser and formatting behavior
- JavaScript binding smoke and selector behavior
- Python binding smoke and selector behavior
- PHP binding smoke, selector behavior, locale fallback, and JSON compatibility loading
- Go binding smoke and selector behavior

The binding tests use real native library calls rather than mocked wrappers.
