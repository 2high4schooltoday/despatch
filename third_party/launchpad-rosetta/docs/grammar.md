# Rosetta Grammar Reference

This document explains the Rosetta source format in detail.

It is intentionally thorough and reflects the current parser and validator implementation in [`src/lib.rs`](../src/lib.rs). If you want the short version first, start with [`docs/quickstart.md`](quickstart.md) and come back here when you want the full syntax reference.

## What A Rosetta File Is

A Rosetta file is a UTF-8 text document with:

1. a required format header
2. one or more metadata header lines
3. a blank line
4. translation entries

Rosetta is designed for human-written locale files. It supports:

- plain string entries
- placeholders such as `{name}`
- selector-based entries for plural, ordinal, gender, and custom select values
- named selector vocabularies through `selector-set:` headers and `select(name)` variables
- multiline values
- per-entry annotations
- locale fallback metadata

## Full File Shape

At a high level, a Rosetta file looks like this:

```lpr
--- launchpad-rosetta 1
locale: en
fallback: en
plural-rule: one other
ordinal-rule: one two few other
selector-set: pronouns = he-him, she-her, they-them

@description Greeting shown on the landing page
hello = Hello, {name}!

reply
  {pronouns: select(pronouns)}
  | he-him = He replied
  | she-her = She replied
  | they-them = They replied
  | * = They replied
```

The parser expects the first line to be exactly:

```text
--- launchpad-rosetta 1
```

Anything else is rejected.

## Informal Grammar

This is an informal grammar for the current implementation:

```text
document           := format-line newline header-lines blank-line entries*
format-line        := "--- launchpad-rosetta 1"

header-lines       := header-line*
header-line        := "locale: " value
                    | "direction: " value
                    | "fallback: " value
                    | "plural-rule: " rule-list
                    | "ordinal-rule: " rule-list
                    | "selector-set: " identifier " = " selector-set-labels

entries            := entry | blank-line | section-line | annotation-line
entry              := inline-entry | block-entry

inline-entry       := key "=" value
block-entry        := key newline indented-entry-line+

indented-entry-line := variable-block
                     | selector-branch
                     | direct-block-value

variable-block     := "{" variable-decl ("," variable-decl)* "}"
variable-decl      := identifier [ ":" variable-kind ]
variable-kind      := "text"
                    | "cardinal"
                    | "ordinal"
                    | "date"
                    | "currency"
                    | "gender"
                    | "list"
                    | "select"
                    | "select(" identifier ")"

selector-branch    := "|" selector-label ("+" selector-label)* "=" value
direct-block-value := "=" value

selector-set-labels := selector-set-label ("," selector-set-label)*

value              := single-line-value | multiline-value
multiline-value    := "```" newline multiline-content newline "```"

annotation-line    := "@" annotation-key whitespace annotation-value
section-line       := "[" any-text "]"
```

That grammar describes structure, but the validator adds more rules. Those are covered below.

## Header Fields

The header is the metadata block at the top of the file, before the first blank line.

Supported keys:

- `locale`
- `direction`
- `fallback`
- `plural-rule`
- `ordinal-rule`
- `selector-set`

Unknown header keys are rejected.

### `locale`

Required.

Example:

```text
locale: en-US
```

The engine normalizes locale tags by:

- trimming whitespace
- converting `_` to `-`
- lowercasing the language subtag
- uppercasing 2-letter region subtags

Examples:

- `en_us` becomes `en-US`
- `RU` becomes `ru`
- `pt-br` becomes `pt-BR`

If a locale string is empty at the API layer, it normalizes to `en`, but a Rosetta file itself must still provide a `locale` header.

### `direction`

Optional.

Example:

```text
direction: rtl
```

The parser stores this value, but the runtime does not currently apply any layout behavior from it. Treat it as metadata for now.

### `fallback`

Optional.

Example:

```text
fallback: en
```

This declares the next locale to try if a key is missing from the current locale.

Fallback is evaluated after:

1. the exact requested locale
2. the language-only locale, if present
3. any explicit fallback chain declared in file metadata
4. `en`, if that locale exists in the bundle

### `plural-rule`

Optional.

Example:

```text
plural-rule: one few many other
```

This header defines the valid cardinal selector labels for the file.

If omitted, the engine uses built-in defaults based on the locale:

- Russian: `one few many other`
- most others: `one other`

Important current behavior:

- the declared rule list is used for validation
- runtime category calculation is currently built in, not fully data-driven
- built-in cardinal category logic currently exists for Russian and the generic `one/other` path

In practice, that means this header should match the runtime behavior for the locale you are using.

### `ordinal-rule`

Optional.

Example:

```text
ordinal-rule: one two few other
```

This header defines the valid ordinal selector labels for the file.

If omitted:

- English defaults to `one two few other`
- most locales default to no ordinal categories

Important current behavior:

- the declared rule list is used for validation
- runtime ordinal category calculation is currently built in
- built-in ordinal logic currently exists for English, with other locales falling back to `other`

### `selector-set`

Optional. Repeatable.

Format:

```text
selector-set: name = label, label, label
```

Examples:

```text
selector-set: pronouns = he-him, she-her, they-them, ze-zir, xe-xem
selector-set: agreement = masc, fem, neutral-e
selector-set: title = mr, ms, mx, dr
```

Rules:

- selector-set names must be valid Rosetta identifiers
- selector-set names must be unique within the file
- labels must be non-empty
- labels must be unique within the set
- labels must be valid Rosetta identifiers
- `*` is not allowed in a selector-set declaration because it is a wildcard, not a vocabulary item

Selector-set declarations let you define a closed vocabulary for a later `select(name)` variable.

## Translation Entries

After the blank line that ends the header, the file contains translation entries.

Rosetta supports two main entry shapes:

- inline entries
- block entries

## Inline Entries

Inline entries are the simplest form:

```lpr
hello = Hello, {name}!
```

Format:

```text
key = value
```

Use inline entries when the translation is short and does not need branches or multiline content.

### Key Rules

A translation key may contain only:

- letters `A-Z` and `a-z`
- digits `0-9`
- underscore `_`
- hyphen `-`
- dot `.`

Valid examples:

- `hello`
- `home.title`
- `settings-save`
- `dialog_confirm_yes`

Invalid examples:

- `hello world`
- `hello/name`
- `price?`

## Block Entries

Block entries begin with a key on its own line:

```lpr
hello
  = Hello, {name}!
```

You use block entries when you need:

- declared variables
- selector branches
- multiline values
- a visually grouped layout

Indented child lines belong to the block entry. A blank line or the next non-indented top-level line ends the block.

### Direct Block Values

A block entry can still contain a single direct value:

```lpr
welcome
  = Welcome to Rosetta
```

This is equivalent to:

```lpr
welcome = Welcome to Rosetta
```

The block form is mostly useful when you want annotations, multiline text, or consistency with selector-style entries.

## Placeholders And Interpolation

Rosetta interpolates placeholders written like:

```text
{name}
```

Example:

```lpr
hello = Hello, {name}!
```

If formatting arguments include `name = "World"`, the output becomes:

```text
Hello, World!
```

### Placeholder Name Rules

Placeholder identifiers may contain only:

- letters `A-Z` and `a-z`
- digits `0-9`
- underscore `_`
- hyphen `-`

Dots are not allowed inside placeholder names.

### Missing Placeholder Arguments

Current behavior:

- missing interpolation arguments do not raise an error
- the placeholder is left in the output as literal text

Example:

```lpr
hello = Hello, {name}!
```

If no `name` argument is supplied, the output remains:

```text
Hello, {name}!
```

This is different from selector arguments, which are required for selector resolution.

## Variable Declarations

Block entries can declare variables with a line like:

```lpr
{name, count: cardinal, role: select}
```

This line must:

- start with `{`
- end with `}`
- contain comma-separated variable declarations

Each variable can be written as:

- `name`
- `name: type`

If no type is given, it defaults to `text`.

### Supported Variable Kinds

The parser currently recognizes:

- `text`
- `cardinal`
- `ordinal`
- `date`
- `currency`
- `gender`
- `list`
- `select`
- `select(name)`

Example:

```lpr
{name, count: cardinal, rank: ordinal, role: select, pronouns: select(pronouns)}
```

### Which Kinds Drive Selector Branching

Only these kinds participate in selector dispatch:

- `cardinal`
- `ordinal`
- `gender`
- `select`
- `select(name)`

Other kinds can still be declared, but they do not create selector axes.

### Open And Closed `select`

Rosetta has two flavors of custom selector:

- `select`: open-ended; any branch label is allowed
- `select(name)`: closed; branch labels must come from the named selector-set declaration

Use open `select` for application-defined values that are not practical to enumerate.

Use `select(name)` when you want:

- a documented vocabulary
- validation for branch labels
- exhaustive coverage checks when no open selector axis is present
- a clearer contract for translators

### Duplicate Variable Names

If the same variable name appears more than once in a declaration block, later duplicates are ignored by the parser. You should still treat duplicate declarations as a mistake and avoid them.

## Selector Entries

Selector entries let you choose a translation branch based on typed arguments.

Example:

```lpr
reply
  {pronouns: select(pronouns)}
  | he-him = He replied
  | she-her = She replied
  | they-them = They replied
  | * = They replied
```

In this example:

- `pronouns: select(pronouns)` creates a closed selector axis
- each branch provides one label for that axis
- the `*` branch acts as a safe fallback for values outside the declared set

### Selector Branch Syntax

A selector branch looks like:

```text
| label [+ label ...] = value
```

Examples:

```lpr
| one = {count} file
| masculine = He replied
| one + admin = One admin
| xe-xem = Xe replied
```

Labels are split by `+`, trimmed, and matched against selector axes from left to right.

### Valid Labels By Axis Type

For `cardinal` axes:

- labels must come from the file’s `plural-rule`
- or use `*`

For `ordinal` axes:

- labels must come from the file’s `ordinal-rule`
- or use `*`

For `gender` axes:

- `masculine`
- `feminine`
- `non-binary`
- or `*`

For `select` axes:

- any label is allowed
- `*` is also allowed and strongly recommended for fallback behavior

For `select(name)` axes:

- labels must come from the named selector-set declaration
- or use `*`

### Wildcards

`*` means “match anything on this axis”.

Example:

```lpr
| * = They replied
```

This matches any value on that selector axis.

### Specificity Rules

When more than one branch matches:

- the engine prefers the most specific branch
- specificity means “the most non-wildcard labels”

So:

```lpr
| xe-xem = Xe replied
| * = They replied
```

will use the first branch when the selector resolves to `xe-xem`.

If two matching branches have the same specificity, the earlier branch wins.

### Selector Validation Rules

The validator enforces several rules.

#### Rule 1: Selector entries must actually have selector axes

If you declare branches but do not declare any selector-capable variables, the entry is rejected.

#### Rule 2: Branch label count must match selector axis count

If an entry has two selector axes, every branch must provide exactly two labels.

#### Rule 3: Multi-axis wildcard use requires an all-wildcard fallback

If a selector entry has more than one axis and any branch uses `*`, then at least one branch must be all wildcards:

```lpr
| * + * = fallback text
```

This prevents partially wildcarded selector sets from leaving uncovered gaps.

#### Rule 4: Open `select` axes require a wildcard fallback on that axis

For every open `select` axis, at least one branch must use `*` in that axis position.

Example:

```lpr
| admin = ...
| * = ...
```

#### Rule 5: Non-dynamic selector combinations must be fully covered

If an entry uses only:

- `cardinal`
- `ordinal`
- `gender`
- `select(name)`

then the validator checks that every possible combination is covered by at least one branch.

If the entry includes an open `select` axis, exhaustive coverage is not checked because the value space is open-ended.

## Cardinal Selectors

Cardinal selectors are based on numeric values provided as `cardinal` arguments.

Example:

```lpr
files
  {count: cardinal}
  | one = {count} file
  | other = {count} files
```

Current built-in cardinal behavior:

- Russian uses `one`, `few`, `many`, `other`
- most other locales use `one` and `other`

### English Cardinal Example

```lpr
files
  {count: cardinal}
  | one = {count} file
  | other = {count} files
```

Results:

- `1` -> `one`
- `0`, `2`, `3`, ... -> `other`

### Russian Cardinal Example

```lpr
files
  {count: cardinal}
  | one = {count} файл
  | few = {count} файла
  | many = {count} файлов
  | other = {count} файла
```

## Ordinal Selectors

Ordinal selectors are based on values supplied as `ordinal` arguments.

Example:

```lpr
place
  {rank: ordinal}
  | one = {rank}st place
  | two = {rank}nd place
  | few = {rank}rd place
  | other = {rank}th place
```

Current built-in ordinal behavior:

- English uses `one`, `two`, `few`, `other`
- other locales currently resolve to `other`

## Gender Selectors

Gender selectors use these labels:

- `masculine`
- `feminine`
- `non-binary`

Example:

```lpr
reply
  {gender: gender}
  | masculine = He replied
  | feminine = She replied
  | non-binary = They replied
```

`other` is not a valid gender label in Rosetta. Use `non-binary` explicitly.

Use `gender` when you want a compact closed grammatical selector. For pronouns, honorifics, kinship terms, and agreement systems that vary by product or locale policy, prefer `selector-set` plus `select(name)`.

## Custom Select Selectors

Custom selects come in two forms:

- open `select`
- closed `select(name)`

### Open `select`

Open `select` uses the raw string value of the `select` argument.

Example:

```lpr
badge
  {role: select}
  | admin = Administrator
  | moderator = Moderator
  | * = Member
```

This is useful when you have application-defined categories.

### Closed `select(name)`

Closed `select(name)` uses a named selector-set declared in the file header.

Example:

```lpr
--- launchpad-rosetta 1
locale: en
plural-rule: one other
selector-set: pronouns = he-him, she-her, they-them, ze-zir, xe-xem

reply
  {pronouns: select(pronouns)}
  | he-him = He replied
  | she-her = She replied
  | they-them = They replied
  | ze-zir = Ze replied
  | xe-xem = Xe replied
  | * = They replied
```

This is the recommended model for pronouns, titles, and grammatical agreement classes when you want a documented vocabulary and stronger validation.

## Multiline Values

Any inline value, direct block value, or selector branch value can become multiline by using `` ``` `` as the value on the same line.

Example:

````lpr
body = ```
  Hello,
  world.
```
````

Or inside a block entry:

````lpr
body
  = ```
    Hello,
    world.
  ```
````

Or inside a branch:

````lpr
message
  {count: cardinal}
  | one = ```
    There is one result.
    Please review it.
  ```
  | other = ```
    There are {count} results.
    Please review them.
  ```
````

### Multiline Dedenting

When a multiline value closes, the parser:

- finds the minimum indentation across non-empty content lines
- removes that indentation from all non-empty lines
- trims trailing whitespace from each line

This makes it possible to indent multiline values cleanly in the file without preserving that indentation in the final output.

## Escaping

Single-line values support a small set of escape sequences:

- `\n` -> newline
- `\t` -> tab
- `\\` -> backslash
- `\{` -> literal `{`

Unknown escapes are preserved as written.

Example:

```lpr
literal = Use \{name\} for documentation
```

Current note:

- there is explicit support for escaping `{`
- there is no dedicated `\}` escape

## Annotations

Annotations are lines that begin with `@` and attach to the next entry.

Example:

```lpr
@description Greeting shown in the header
@owner "marketing"
hello = Hello
```

Annotation syntax:

```text
@key value
```

The parser:

- reads the annotation key up to the first whitespace
- treats the rest of the line as the value
- supports quoted annotation values with `\"` unescaping

Current behavior:

- annotations are parsed and stored
- the runtime does not currently assign built-in semantics to them

Think of them as metadata reserved for tooling, docs, or future behavior.

## Section Headers

Lines shaped like:

```text
[Common]
```

are accepted and ignored.

They currently have no runtime meaning. You can use them as visual grouping markers inside locale files.

## Blank Lines And Indentation

Blank lines are meaningful in a few ways:

- a blank line ends the header section
- blank lines between entries are ignored
- a blank line inside a block entry ends that block entry

Indentation rules:

- top-level entries must not start indented
- block-entry child lines must be indented
- indentation width is not semantically named, but indentation is how the parser knows a line belongs to a block entry

An indented line at the top level is an error.

## JSON Compatibility Format

Rosetta also supports loading a flat JSON map through the compatibility loader.

Example:

```json
{
  "hello": "Hello",
  "farewell": "Goodbye"
}
```

Rules:

- the JSON root must be an object
- every value must be a string
- nested objects are not supported

This mode is useful for migration from existing translation tables, but Rosetta’s richer syntax lives in `.lpr` files.

## File-Level Validation And Common Errors

The parser and validator reject a number of invalid shapes.

Common examples:

### Missing or invalid format line

```text
--- launchpad 1
```

Rejected because the header must be exactly:

```text
--- launchpad-rosetta 1
```

### Missing `locale`

Rejected because every Rosetta file must declare a locale.

### Unknown header keys

Rejected:

```text
timezone: UTC
```

### Invalid keys

Rejected:

```lpr
hello world = Hi
```

because spaces are not allowed in keys.

### Invalid selector labels

Rejected:

```lpr
files
  {count: cardinal}
  | few = {count} files
```

for an English file with `plural-rule: one other`, because `few` is not a valid cardinal label there.

### Selector branches without selector variables

Rejected:

```lpr
hello
  {name}
  | one = Hello
```

because `name` is `text`, not a selector axis.

### Multi-axis wildcard without an all-wildcard fallback

Rejected:

```lpr
invite
  {count: cardinal, role: select}
  | one + * = one person
  | other + admin = many admins
```

because there is no `| * + * = ...` fallback.

## Runtime Behavior Summary

After parsing and validation, runtime lookup works like this:

1. choose a bundle
2. choose a locale
3. try the exact locale
4. try the language-only locale
5. follow explicit fallback metadata
6. try `en`, if present
7. if the entry is plain text, interpolate placeholders
8. if the entry is a selector, resolve selector labels and choose the best matching branch

## Current Limitations And Important Notes

These points are especially useful if you are designing a format around Rosetta.

### `date` and `currency` kinds are recognized but not yet special-cased at runtime

The parser accepts `date` and `currency` in variable declarations, but they do not currently have dedicated formatting behavior in the runtime APIs.

### Rule headers are not fully data-driven runtime grammars

`plural-rule` and `ordinal-rule` participate in validation, but runtime category selection is still implemented in Rust for specific locale behaviors rather than generated from those header lists.

### Missing interpolation args remain literal

If a placeholder argument is not provided, the placeholder text stays in the output.

### Missing selector args are errors

Selector resolution does require the needed selector arguments.

### Section markers are ignored

`[Section]` lines are accepted for readability only.

### Annotations are metadata only

They are parsed and stored, but the runtime does not assign built-in meaning to them yet.

## Recommended Authoring Style

If you are just getting started, these conventions work well:

- keep keys stable and descriptive, such as `home.title` or `dialog.confirm`
- use inline entries for short strings
- use block entries for anything with selectors or multiline text
- use `selector-set` plus `select(name)` for pronouns, titles, and agreement classes
- always include a wildcard fallback for open `select` axes
- keep a wildcard fallback for closed selectors too when you want safe behavior for unexpected runtime values
- prefer neutral wording when a message does not need person-specific branching
- prefer complete branch coverage instead of relying on implicit behavior
- use annotations for translator notes and tooling metadata
- group related keys with `[Section]` markers if that helps readability

## Minimal Valid Example

```lpr
--- launchpad-rosetta 1
locale: en
plural-rule: one other

hello = Hello
```

## Rich Example

````lpr
--- launchpad-rosetta 1
locale: en-US
fallback: en
plural-rule: one other
ordinal-rule: one two few other
selector-set: pronouns = he-him, she-her, they-them, ze-zir, xe-xem

[Home]

@description Heading shown above the dashboard
home.title = Welcome back, {name}

home.files
  {count: cardinal}
  | one = You have {count} file
  | other = You have {count} files

home.badge
  {role: select}
  | admin = Administrator
  | moderator = Moderator
  | * = Member

home.reply
  {pronouns: select(pronouns)}
  | he-him = He replied
  | she-her = She replied
  | they-them = They replied
  | ze-zir = Ze replied
  | xe-xem = Xe replied
  | * = They replied

home.place
  {rank: ordinal}
  | one = {rank}st place
  | two = {rank}nd place
  | few = {rank}rd place
  | other = {rank}th place

home.body = ```
  This text can span
  multiple lines.
```
````
