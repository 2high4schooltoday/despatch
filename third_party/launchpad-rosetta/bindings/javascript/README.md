# Launchpad Rosetta for JavaScript

This package exposes the Launchpad Rosetta native library to Node.js.

For the Rosetta file format itself, see [`../../docs/grammar.md`](../../docs/grammar.md).

## Requirements

- Node.js 18+
- a built `lp_i18n` shared library
- `npm install` in this binding directory

## Setup

From the repository root:

```bash
cargo build
cd bindings/javascript
npm install
```

The binding looks for the native `lp_i18n` shared library in:

1. `LP_I18N_LIBRARY_PATH`
2. `target/release`
3. `target/debug`

## Example

```js
import { Args, CatalogBuilder } from "./index.js";

const source = `--- launchpad-rosetta 1
locale: en
plural-rule: one other

hello = Hello, {name}!
`;

const catalog = new CatalogBuilder()
  .addRosettaString("project", "en.lpr", source)
  .build();

const translator = catalog.translator("en-US");
console.log(
  translator.format("project", "hello", new Args().text("name", "World")),
);
catalog.close();
```

## Notes

- `CatalogBuilder` collects source documents and creates a `Catalog`
- `Translator` returns errors on missing keys
- `LocaleContext` is the convenience API with default-bundle behavior and key fallback
- use `Args.select(...)` for named selector sets such as pronouns, titles, and agreement classes declared with `select(name)` in Rosetta source
- gender arguments use the Rosetta labels `masculine`, `feminine`, and `non-binary`
- call `catalog.close()` when you are done if you want deterministic cleanup

## Testing

From the repository root:

```bash
cd bindings/javascript
npm install
npm test
```
