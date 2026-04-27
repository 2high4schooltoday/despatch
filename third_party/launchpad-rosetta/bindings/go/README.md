# Launchpad Rosetta for Go

This package exposes the Launchpad Rosetta native library through a Go wrapper backed by `purego`.

For the Rosetta file format itself, see [`../../docs/grammar.md`](../../docs/grammar.md).

## Requirements

- Go 1.22+
- a built `lp_i18n` shared library

The binding looks for the native library in:

1. `LP_I18N_LIBRARY_PATH`
2. `target/release`
3. `target/debug`

## Setup

From the repository root:

```bash
cargo build
cd bindings/go
go test ./...
```

## Example

```go
package main

import (
    "fmt"

    rosetta "launchpad-rosetta/bindings/go"
)

const source = `--- launchpad-rosetta 1
locale: en
plural-rule: one other

hello = Hello, {name}!
`

builder, err := rosetta.NewCatalogBuilder()
if err != nil {
    panic(err)
}

if err := builder.AddRosettaString("project", "en.lpr", source); err != nil {
    panic(err)
}

catalog, err := builder.Build()
if err != nil {
    panic(err)
}
defer catalog.Close()

translator := catalog.Translator("en")
value, err := translator.Format("project", "hello", rosetta.NewArgs().Text("name", "World"))
if err != nil {
    panic(err)
}

fmt.Println(value)
```

## Notes

- `CatalogBuilder` owns the native builder handle until `Build()`
- `Catalog.Close()` is recommended for deterministic cleanup
- `Translator` is strict
- `LocaleContext` falls back to the key on lookup or formatting failure
- use `Args.Select(...)` for named selector sets such as pronouns, titles, and agreement classes declared with `select(name)` in Rosetta source
- gender arguments use the Rosetta labels `masculine`, `feminine`, and `non-binary`

## Testing

From the repository root:

```bash
cargo build
cd bindings/go
go test ./...
```
