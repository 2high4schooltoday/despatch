# Quickstart

This page shows the recommended way to use Launchpad Rosetta from Rust and from the native bindings.

If you want the full file-format reference, read [`docs/grammar.md`](grammar.md).

For pronouns, honorifics, and grammatical agreement, prefer named selector sets with `selector-set:` headers and `select(name)` variables. The built-in `gender` kind is still available when you want a compact closed selector with `masculine`, `feminine`, and `non-binary`.

## 1. Build The Native Library

From the repository root:

```bash
cargo build
```

That gives you a shared library in `target/debug` and a release build in `target/release` after `cargo build --release`.

## 2. Prepare A Locale File

Example `en.lpr`:

```lpr
--- launchpad-rosetta 1
locale: en
plural-rule: one other
selector-set: pronouns = he-him, she-her, they-them, ze-zir, xe-xem

hello = Hello, {name}!

files
  {count: cardinal}
  | one = {count} file
  | other = {count} files

reply
  {pronouns: select(pronouns)}
  | he-him = He replied
  | she-her = She replied
  | they-them = They replied
  | ze-zir = Ze replied
  | xe-xem = Xe replied
  | * = They replied
```

## 3. Choose The API Layer

Use `Translator` when you want missing keys to be explicit errors.

Use `LocaleContext` when you want a convenience layer with:

- a default bundle
- key fallback on lookup or formatting errors

## Rust Example

```rust
use lp_i18n::{Args, Catalog};

let catalog = Catalog::builder()
    .add_rosetta_str("project", "en.lpr", include_str!("en.lpr"))?
    .build()?;

let translator = catalog.translator("en-US")?;
let output = translator.format(
    "project",
    "files",
    Args::new().cardinal("count", 3),
)?;

assert_eq!(output, "3 files");
# Ok::<(), anyhow::Error>(())
```

## JavaScript Example

```js
import { Args, CatalogBuilder } from "./bindings/javascript/index.js";

const source = `--- launchpad-rosetta 1
locale: en
plural-rule: one other

hello = Hello, {name}!
`;

const catalog = new CatalogBuilder()
  .addRosettaString("project", "en.lpr", source)
  .build();

const translator = catalog.translator("en");
console.log(
  translator.format("project", "hello", new Args().text("name", "World")),
);
```

## Python Example

```python
from launchpad_rosetta import Args, CatalogBuilder

source = """--- launchpad-rosetta 1
locale: en
plural-rule: one other

hello = Hello, {name}!
"""

catalog = (
    CatalogBuilder()
    .add_rosetta_string("project", "en.lpr", source)
    .build()
)

translator = catalog.translator("en")
print(translator.format("project", "hello", Args().text("name", "World")))
```

## PHP Example

```php
<?php

require 'bindings/php/src/LaunchpadRosetta.php';

use Launchpad\Rosetta\Args;
use Launchpad\Rosetta\CatalogBuilder;

$source = <<<'LPR'
--- launchpad-rosetta 1
locale: en
plural-rule: one other

hello = Hello, {name}!
LPR;

$catalog = (new CatalogBuilder())
    ->addRosettaString('project', 'en.lpr', $source)
    ->build();

$translator = $catalog->translator('en');
echo $translator->format('project', 'hello', (new Args())->text('name', 'World'));
```

Run PHP examples with:

```bash
php -d ffi.enable=1 your-script.php
```

## Go Example

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

## JSON Compatibility Catalogs

If you already have a flat JSON map such as:

```json
{
  "hello": "Hello",
  "farewell": "Goodbye"
}
```

you can load it with the JSON compatibility helpers instead of rewriting it immediately into Rosetta format.

These helpers expect a flat object of `string -> string`.

## Pronouns And Agreement

When you need person-sensitive language, keep identity, pronouns, and grammatical agreement separate:

- use `text` for self-described identity labels that do not need branching
- use `select(name)` for pronouns, titles, and agreement classes
- keep a `*` fallback branch when you want safe behavior for values outside the declared selector set

## Troubleshooting

If a non-Rust binding cannot find the shared library:

1. make sure `cargo build` succeeded
2. check that the shared library exists in `target/debug` or `target/release`
3. set `LP_I18N_LIBRARY_PATH` explicitly

If PHP reports that FFI is disabled, run your command with:

```bash
php -d ffi.enable=1 ...
```
