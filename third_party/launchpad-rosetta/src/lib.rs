use std::collections::{BTreeMap, BTreeSet, HashMap, HashSet};
use std::fs;
use std::path::Path;
use std::sync::Arc;

use anyhow::{Context, Result, anyhow, bail};
use serde::{Deserialize, Serialize};
use thiserror::Error;

mod ffi;
pub use ffi::*;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum Gender {
    Masculine,
    Feminine,
    NonBinary,
}

impl Gender {
    fn label(self) -> &'static str {
        match self {
            Self::Masculine => "masculine",
            Self::Feminine => "feminine",
            Self::NonBinary => "non-binary",
        }
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub enum ArgValue {
    Text(String),
    Cardinal(i64),
    Ordinal(i64),
    Gender(Gender),
    Select(String),
    List(Vec<String>),
}

impl ArgValue {
    fn display(&self) -> String {
        match self {
            Self::Text(value) => value.clone(),
            Self::Cardinal(value) | Self::Ordinal(value) => value.to_string(),
            Self::Gender(value) => value.label().to_owned(),
            Self::Select(value) => value.clone(),
            Self::List(values) => values.join(", "),
        }
    }
}

#[derive(Debug, Clone, Default, PartialEq, Eq, Serialize, Deserialize)]
pub struct Args {
    values: BTreeMap<String, ArgValue>,
}

impl Args {
    #[must_use]
    pub fn new() -> Self {
        Self::default()
    }

    #[must_use]
    pub fn text(mut self, name: impl Into<String>, value: impl Into<String>) -> Self {
        self.values
            .insert(name.into(), ArgValue::Text(value.into()));
        self
    }

    #[must_use]
    pub fn cardinal(mut self, name: impl Into<String>, value: i64) -> Self {
        self.values.insert(name.into(), ArgValue::Cardinal(value));
        self
    }

    #[must_use]
    pub fn ordinal(mut self, name: impl Into<String>, value: i64) -> Self {
        self.values.insert(name.into(), ArgValue::Ordinal(value));
        self
    }

    #[must_use]
    pub fn gender(mut self, name: impl Into<String>, value: Gender) -> Self {
        self.values.insert(name.into(), ArgValue::Gender(value));
        self
    }

    #[must_use]
    pub fn select(mut self, name: impl Into<String>, value: impl Into<String>) -> Self {
        self.values
            .insert(name.into(), ArgValue::Select(value.into()));
        self
    }

    #[must_use]
    pub fn list(mut self, name: impl Into<String>, value: Vec<String>) -> Self {
        self.values.insert(name.into(), ArgValue::List(value));
        self
    }

    pub fn insert(&mut self, name: impl Into<String>, value: ArgValue) {
        self.values.insert(name.into(), value);
    }

    fn get(&self, name: &str) -> Option<&ArgValue> {
        self.values.get(name)
    }
}

#[derive(Debug, Clone)]
pub struct LocaleContext {
    locale: String,
    catalog: Catalog,
    default_bundle: String,
}

impl LocaleContext {
    #[must_use]
    pub fn new(
        locale: impl Into<String>,
        catalog: Catalog,
        default_bundle: impl Into<String>,
    ) -> Self {
        Self {
            locale: normalize_locale_tag(&locale.into()),
            catalog,
            default_bundle: default_bundle.into(),
        }
    }

    #[must_use]
    pub fn english(default_bundle: impl Into<String>) -> Self {
        Self {
            locale: "en".to_owned(),
            catalog: Catalog::empty(),
            default_bundle: default_bundle.into(),
        }
    }

    #[must_use]
    pub fn locale(&self) -> &str {
        &self.locale
    }

    #[must_use]
    pub fn default_bundle(&self) -> &str {
        &self.default_bundle
    }

    #[must_use]
    pub fn with_locale(&self, locale: impl Into<String>) -> Self {
        Self {
            locale: normalize_locale_tag(&locale.into()),
            catalog: self.catalog.clone(),
            default_bundle: self.default_bundle.clone(),
        }
    }

    pub fn translator(&self) -> Result<Translator<'_>, I18nError> {
        self.catalog.translator(&self.locale)
    }

    pub fn text(&self, key: &str) -> String {
        self.translator()
            .and_then(|translator| translator.text(&self.default_bundle, key))
            .unwrap_or_else(|_| key.to_owned())
    }

    pub fn format(&self, key: &str, args: Args) -> String {
        self.translator()
            .and_then(|translator| translator.format(&self.default_bundle, key, args))
            .unwrap_or_else(|_| key.to_owned())
    }

    pub fn bundle_text(&self, bundle: &str, key: &str) -> String {
        self.translator()
            .and_then(|translator| translator.text(bundle, key))
            .unwrap_or_else(|_| key.to_owned())
    }

    pub fn bundle_format(&self, bundle: &str, key: &str, args: Args) -> String {
        self.translator()
            .and_then(|translator| translator.format(bundle, key, args))
            .unwrap_or_else(|_| key.to_owned())
    }

    #[must_use]
    pub fn catalog(&self) -> &Catalog {
        &self.catalog
    }

    #[must_use]
    pub fn bundle_locales(&self, bundle: &str) -> Vec<String> {
        self.catalog.bundle_locales(bundle)
    }
}

#[derive(Debug, Clone, Default)]
pub struct Catalog(Arc<CatalogInner>);

#[derive(Debug, Default)]
struct CatalogInner {
    bundles: HashMap<String, BundleCatalog>,
}

#[derive(Debug, Default)]
struct BundleCatalog {
    locales: HashMap<String, LocaleCatalog>,
}

#[derive(Debug, Clone)]
struct LocaleCatalog {
    locale: String,
    fallback: Option<String>,
    plural_rule: Vec<String>,
    ordinal_rule: Vec<String>,
    entries: HashMap<String, Entry>,
}

#[derive(Debug, Clone)]
enum Entry {
    Value(String),
    Selector(SelectorEntry),
}

#[derive(Debug, Clone)]
struct SelectorEntry {
    variables: Vec<VariableDecl>,
    selector_axes: Vec<SelectorAxis>,
    branches: Vec<SelectorBranch>,
}

#[derive(Debug, Clone)]
struct VariableDecl {
    name: String,
    kind: VariableKind,
    selector_set: Option<String>,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum VariableKind {
    Text,
    Cardinal,
    Ordinal,
    Date,
    Currency,
    Gender,
    List,
    Select,
}

impl VariableKind {
    fn from_str(value: &str) -> Option<Self> {
        match value.trim() {
            "" | "text" => Some(Self::Text),
            "cardinal" => Some(Self::Cardinal),
            "ordinal" => Some(Self::Ordinal),
            "date" => Some(Self::Date),
            "currency" => Some(Self::Currency),
            "gender" => Some(Self::Gender),
            "list" => Some(Self::List),
            "select" => Some(Self::Select),
            _ => None,
        }
    }
}

fn parse_variable_kind(value: &str) -> Option<(VariableKind, Option<String>)> {
    let value = value.trim();
    if let Some(inner) = value
        .strip_prefix("select(")
        .and_then(|rest| rest.strip_suffix(')'))
    {
        let name = inner.trim();
        if is_valid_rosetta_identifier(name) {
            return Some((VariableKind::Select, Some(name.to_owned())));
        }
        return None;
    }
    VariableKind::from_str(value).map(|kind| (kind, None))
}

#[derive(Debug, Clone)]
struct SelectorAxis {
    name: String,
    kind: VariableKind,
    selector_set: Option<String>,
}

#[derive(Debug, Clone)]
struct SelectorBranch {
    labels: Vec<String>,
    value: String,
}

impl Catalog {
    #[must_use]
    pub fn builder() -> CatalogBuilder {
        CatalogBuilder::default()
    }

    #[must_use]
    pub fn empty() -> Self {
        Self::default()
    }

    pub fn translator(&self, requested_locale: &str) -> Result<Translator<'_>, I18nError> {
        Ok(Translator {
            catalog: self,
            requested_locale: normalize_locale_tag(requested_locale),
        })
    }

    #[must_use]
    pub fn bundle_locales(&self, bundle: &str) -> Vec<String> {
        let Some(bundle_catalog) = self.0.bundles.get(bundle) else {
            return Vec::new();
        };
        let mut locales = bundle_catalog.locales.keys().cloned().collect::<Vec<_>>();
        locales.sort();
        locales
    }
}

#[must_use]
pub fn locale_endonym(locale: &str) -> String {
    let normalized = normalize_locale_tag(locale);
    let language = normalized.split('-').next().unwrap_or(normalized.as_str());
    match language {
        "en" => "English".to_owned(),
        "de" => "Deutsch".to_owned(),
        "ru" => "Русский".to_owned(),
        _ => normalized,
    }
}

#[must_use]
pub fn locale_english_name(locale: &str) -> String {
    let normalized = normalize_locale_tag(locale);
    let language = normalized.split('-').next().unwrap_or(normalized.as_str());
    match language {
        "en" => "English".to_owned(),
        "de" => "German".to_owned(),
        "ru" => "Russian".to_owned(),
        _ => normalized,
    }
}

#[derive(Debug, Default)]
pub struct CatalogBuilder {
    documents: Vec<PendingDocument>,
}

#[derive(Debug)]
struct PendingDocument {
    bundle: String,
    document: RosettaDocument,
}

impl CatalogBuilder {
    pub fn push_rosetta_str(&mut self, bundle: &str, source_name: &str, text: &str) -> Result<()> {
        let document = parse_rosetta_document(source_name, text)?;
        self.documents.push(PendingDocument {
            bundle: bundle.to_owned(),
            document,
        });
        Ok(())
    }

    pub fn add_rosetta_str(mut self, bundle: &str, source_name: &str, text: &str) -> Result<Self> {
        self.push_rosetta_str(bundle, source_name, text)?;
        Ok(self)
    }

    pub fn push_rosetta_path(&mut self, bundle: &str, path: &Path) -> Result<()> {
        let contents = fs::read_to_string(path)
            .with_context(|| format!("failed to read Rosetta file `{}`", path.display()))?;
        self.push_rosetta_str(bundle, &path.display().to_string(), &contents)
    }

    pub fn add_rosetta_path(mut self, bundle: &str, path: &Path) -> Result<Self> {
        self.push_rosetta_path(bundle, path)?;
        Ok(self)
    }

    pub fn push_json_compat_str(
        &mut self,
        bundle: &str,
        locale: &str,
        source_name: &str,
        text: &str,
    ) -> Result<()> {
        let document = parse_json_compat_document(locale, source_name, text)?;
        self.documents.push(PendingDocument {
            bundle: bundle.to_owned(),
            document,
        });
        Ok(())
    }

    pub fn add_json_compat_str(
        mut self,
        bundle: &str,
        locale: &str,
        source_name: &str,
        text: &str,
    ) -> Result<Self> {
        self.push_json_compat_str(bundle, locale, source_name, text)?;
        Ok(self)
    }

    pub fn push_json_compat_path(&mut self, bundle: &str, locale: &str, path: &Path) -> Result<()> {
        let contents = fs::read_to_string(path).with_context(|| {
            format!(
                "failed to read compatibility locale file `{}`",
                path.display()
            )
        })?;
        self.push_json_compat_str(bundle, locale, &path.display().to_string(), &contents)
    }

    pub fn add_json_compat_path(mut self, bundle: &str, locale: &str, path: &Path) -> Result<Self> {
        self.push_json_compat_path(bundle, locale, path)?;
        Ok(self)
    }

    pub fn build(self) -> Result<Catalog> {
        let mut bundles = HashMap::<String, BundleCatalog>::new();
        for pending in self.documents {
            let bundle = bundles.entry(pending.bundle).or_default();
            let locale_key = pending.document.locale.clone();
            let locale =
                bundle
                    .locales
                    .entry(locale_key.clone())
                    .or_insert_with(|| LocaleCatalog {
                        locale: locale_key.clone(),
                        fallback: pending.document.fallback.clone(),
                        plural_rule: pending.document.plural_rule.clone(),
                        ordinal_rule: pending.document.ordinal_rule.clone().unwrap_or_default(),
                        entries: HashMap::new(),
                    });
            if locale.fallback != pending.document.fallback {
                bail!(
                    "bundle locale `{}` declared conflicting fallback values",
                    locale_key
                );
            }
            if locale.plural_rule != pending.document.plural_rule {
                bail!(
                    "bundle locale `{}` declared conflicting plural rules",
                    locale_key
                );
            }
            let incoming_ordinal = pending.document.ordinal_rule.unwrap_or_default();
            if locale.ordinal_rule != incoming_ordinal {
                bail!(
                    "bundle locale `{}` declared conflicting ordinal rules",
                    locale_key
                );
            }
            for entry in pending.document.entries {
                if locale
                    .entries
                    .insert(entry.key.clone(), entry.value)
                    .is_some()
                {
                    bail!(
                        "duplicate translation key `{}` in bundle locale `{}`",
                        entry.key,
                        locale_key
                    );
                }
            }
        }
        Ok(Catalog(Arc::new(CatalogInner { bundles })))
    }
}

#[derive(Debug)]
pub struct Translator<'a> {
    catalog: &'a Catalog,
    requested_locale: String,
}

impl<'a> Translator<'a> {
    #[must_use]
    pub fn locale(&self) -> &str {
        &self.requested_locale
    }

    pub fn text(&self, bundle: &str, key: &str) -> Result<String, I18nError> {
        self.resolve(bundle, key, Args::new())
    }

    pub fn format(&self, bundle: &str, key: &str, args: Args) -> Result<String, I18nError> {
        self.resolve(bundle, key, args)
    }

    #[must_use]
    pub fn has(&self, bundle: &str, key: &str) -> bool {
        self.locale_candidates(bundle).into_iter().any(|locale| {
            self.catalog
                .0
                .bundles
                .get(bundle)
                .and_then(|bundle_catalog| bundle_catalog.locales.get(&locale))
                .and_then(|locale_catalog| locale_catalog.entries.get(key))
                .is_some()
        })
    }

    fn resolve(&self, bundle: &str, key: &str, args: Args) -> Result<String, I18nError> {
        let Some(bundle_catalog) = self.catalog.0.bundles.get(bundle) else {
            return Err(I18nError::UnknownBundle(bundle.to_owned()));
        };
        for locale_key in self.locale_candidates(bundle) {
            let Some(locale_catalog) = bundle_catalog.locales.get(&locale_key) else {
                continue;
            };
            let Some(entry) = locale_catalog.entries.get(key) else {
                continue;
            };
            return match entry {
                Entry::Value(value) => Ok(interpolate_value(value, &args)),
                Entry::Selector(selector) => {
                    resolve_selector_entry(locale_catalog, selector, &args)
                }
            };
        }
        Err(I18nError::MissingKey {
            bundle: bundle.to_owned(),
            key: key.to_owned(),
            locale: self.requested_locale.clone(),
        })
    }

    fn locale_candidates(&self, bundle: &str) -> Vec<String> {
        let mut ordered = Vec::new();
        let mut seen = HashSet::new();
        let Some(bundle_catalog) = self.catalog.0.bundles.get(bundle) else {
            return ordered;
        };
        let exact = normalize_locale_tag(&self.requested_locale);
        push_locale_candidate(&mut ordered, &mut seen, exact.clone());
        if let Some(language) = exact.split('-').next() {
            push_locale_candidate(&mut ordered, &mut seen, language.to_owned());
        }

        let snapshot = ordered.clone();
        for candidate in snapshot {
            let mut current = Some(candidate);
            while let Some(locale) = current {
                let Some(catalog) = bundle_catalog.locales.get(&locale) else {
                    break;
                };
                let Some(fallback) = catalog.fallback.clone() else {
                    break;
                };
                push_locale_candidate(&mut ordered, &mut seen, normalize_locale_tag(&fallback));
                current = Some(normalize_locale_tag(&fallback));
                if !bundle_catalog
                    .locales
                    .contains_key(&current.clone().unwrap_or_default())
                {
                    break;
                }
            }
        }

        if bundle_catalog.locales.contains_key("en") {
            push_locale_candidate(&mut ordered, &mut seen, "en".to_owned());
        }
        ordered
    }
}

fn push_locale_candidate(ordered: &mut Vec<String>, seen: &mut HashSet<String>, locale: String) {
    if seen.insert(locale.clone()) {
        ordered.push(locale);
    }
}

#[derive(Debug, Error)]
pub enum I18nError {
    #[error("unknown bundle `{0}`")]
    UnknownBundle(String),
    #[error("translation key `{key}` was not found in bundle `{bundle}` for locale `{locale}`")]
    MissingKey {
        bundle: String,
        key: String,
        locale: String,
    },
    #[error("translation key requires argument `{0}`")]
    MissingArgument(String),
    #[error("selector `{selector}` requires argument `{argument}`")]
    MissingSelectorArgument { selector: String, argument: String },
    #[error("selector `{selector}` did not match any branch")]
    SelectorNoMatch { selector: String },
}

#[derive(Debug)]
struct RosettaDocument {
    format_version: u32,
    locale: String,
    direction: Option<String>,
    fallback: Option<String>,
    plural_rule: Vec<String>,
    ordinal_rule: Option<Vec<String>>,
    entries: Vec<RosettaEntry>,
}

#[derive(Debug)]
struct RosettaEntry {
    key: String,
    annotations: BTreeMap<String, String>,
    value: Entry,
}

fn parse_json_compat_document(
    locale: &str,
    source_name: &str,
    text: &str,
) -> Result<RosettaDocument> {
    let parsed: serde_json::Value = serde_json::from_str(text)
        .with_context(|| format!("failed to parse json locale file `{source_name}`"))?;
    let mut entries = Vec::new();
    let map = parsed
        .as_object()
        .ok_or_else(|| anyhow!("json locale file `{source_name}` must be a flat object"))?;
    for (key, value) in map {
        let Some(text) = value.as_str() else {
            bail!("json locale file `{source_name}` key `{key}` must map to text");
        };
        entries.push(RosettaEntry {
            key: key.clone(),
            annotations: BTreeMap::new(),
            value: Entry::Value(text.to_owned()),
        });
    }
    Ok(RosettaDocument {
        format_version: 1,
        locale: normalize_locale_tag(locale),
        direction: Some("ltr".to_owned()),
        fallback: None,
        plural_rule: default_plural_rule_for_locale(locale),
        ordinal_rule: Some(default_ordinal_rule_for_locale(locale)),
        entries,
    })
}

fn parse_rosetta_document(source_name: &str, text: &str) -> Result<RosettaDocument> {
    let mut lines = text.lines().enumerate().peekable();
    let Some((_, first_line)) = lines.next() else {
        bail!("Rosetta file `{source_name}` is empty");
    };
    let header = first_line.trim();
    if header != "--- launchpad-rosetta 1" {
        bail!("Rosetta file `{source_name}` must start with `--- launchpad-rosetta 1`");
    }
    let mut locale = None;
    let mut direction = None;
    let mut fallback = None;
    let mut plural_rule = None;
    let mut ordinal_rule = None;
    let mut selector_sets = HashMap::<String, Vec<String>>::new();

    while let Some((_, line)) = lines.peek() {
        if line.trim().is_empty() {
            lines.next();
            break;
        }
        let (line_number, line) = lines.next().expect("peeked line should exist");
        let Some((key, value)) = line.split_once(':') else {
            bail!(
                "Rosetta header line {} in `{source_name}` must look like `key: value`",
                line_number + 1
            );
        };
        let key = key.trim();
        let value = value.trim();
        match key {
            "locale" => locale = Some(normalize_locale_tag(value)),
            "direction" => direction = Some(value.to_owned()),
            "fallback" => fallback = Some(normalize_locale_tag(value)),
            "plural-rule" => plural_rule = Some(split_rule_list(value)),
            "ordinal-rule" => ordinal_rule = Some(split_rule_list(value)),
            "selector-set" => {
                let (name, labels) =
                    parse_selector_set_declaration(value, source_name, line_number + 1)?;
                if selector_sets.insert(name.clone(), labels).is_some() {
                    bail!(
                        "Rosetta file `{source_name}` redeclares selector-set `{name}` on line {}",
                        line_number + 1
                    );
                }
            }
            other => bail!("unknown Rosetta header key `{other}` in `{source_name}`"),
        }
    }

    let locale =
        locale.ok_or_else(|| anyhow!("Rosetta file `{source_name}` is missing `locale`"))?;
    let plural_rule = plural_rule.unwrap_or_else(|| default_plural_rule_for_locale(&locale));
    let ordinal_rule = ordinal_rule.or_else(|| {
        let derived = default_ordinal_rule_for_locale(&locale);
        (!derived.is_empty()).then_some(derived)
    });

    let mut entries = Vec::new();
    let mut pending_annotations = BTreeMap::<String, String>::new();
    while let Some((line_number, line)) = lines.peek().cloned() {
        let trimmed = line.trim();
        if trimmed.is_empty() {
            lines.next();
            continue;
        }
        if trimmed.starts_with('[') && trimmed.ends_with(']') {
            lines.next();
            continue;
        }
        if let Some(annotation) = trimmed.strip_prefix('@') {
            let (key, value) = annotation
                .trim()
                .split_once(char::is_whitespace)
                .ok_or_else(|| {
                    anyhow!(
                        "annotation on line {} in `{source_name}` must include a key and value",
                        line_number + 1
                    )
                })?;
            pending_annotations.insert(key.trim().to_owned(), parse_annotation_value(value.trim()));
            lines.next();
            continue;
        }
        if !line.starts_with(' ') && !line.starts_with('\t') && trimmed.contains('=') {
            lines.next();
            let (key, value_src) = split_entry_assignment(trimmed, source_name, line_number + 1)?;
            let value = parse_inline_value(
                &mut lines,
                value_src,
                indent_width(line),
                source_name,
                line_number + 1,
            )?;
            validate_entry(
                &locale,
                &plural_rule,
                ordinal_rule.as_deref().unwrap_or(&[]),
                &selector_sets,
                &RosettaEntry {
                    key: key.to_owned(),
                    annotations: pending_annotations.clone(),
                    value: Entry::Value(value.clone()),
                },
            )?;
            entries.push(RosettaEntry {
                key: key.to_owned(),
                annotations: std::mem::take(&mut pending_annotations),
                value: Entry::Value(value),
            });
            continue;
        }

        if line.starts_with(' ') || line.starts_with('\t') {
            bail!(
                "unexpected indented line {} in `{source_name}`",
                line_number + 1
            );
        }

        lines.next();
        let key = trimmed.to_owned();
        if !is_valid_rosetta_key(&key) {
            bail!(
                "invalid Rosetta key `{}` on line {} in `{source_name}`",
                key,
                line_number + 1
            );
        }
        let mut variables = Vec::<VariableDecl>::new();
        let mut branches = Vec::<SelectorBranch>::new();
        let mut direct_value = None;
        while let Some((_, next_line)) = lines.peek().cloned() {
            if next_line.trim().is_empty() {
                lines.next();
                break;
            }
            if !next_line.starts_with(' ') && !next_line.starts_with('\t') {
                break;
            }
            let (child_line_number, child_line) = lines.next().expect("peeked line should exist");
            let trimmed_child = child_line.trim();
            if trimmed_child.starts_with('{') && trimmed_child.ends_with('}') {
                variables.extend(parse_variable_decls(
                    trimmed_child,
                    &selector_sets,
                    source_name,
                    child_line_number + 1,
                )?);
                continue;
            }
            if let Some(branch_src) = trimmed_child.strip_prefix('|') {
                let (labels_src, value_src) =
                    split_entry_assignment(branch_src.trim(), source_name, child_line_number + 1)?;
                let labels = labels_src
                    .split('+')
                    .map(|label| label.trim().to_owned())
                    .collect::<Vec<_>>();
                let value = parse_inline_value(
                    &mut lines,
                    value_src,
                    indent_width(child_line),
                    source_name,
                    child_line_number + 1,
                )?;
                branches.push(SelectorBranch { labels, value });
                continue;
            }
            if let Some(value_src) = trimmed_child.strip_prefix('=') {
                let value = parse_inline_value(
                    &mut lines,
                    value_src.trim(),
                    indent_width(child_line),
                    source_name,
                    child_line_number + 1,
                )?;
                direct_value = Some(value);
                continue;
            }
            bail!(
                "unsupported Rosetta entry line {} in `{source_name}`",
                child_line_number + 1
            );
        }

        let value = if !branches.is_empty() {
            let selector_axes = variables
                .iter()
                .filter(|variable| {
                    matches!(
                        variable.kind,
                        VariableKind::Cardinal
                            | VariableKind::Ordinal
                            | VariableKind::Gender
                            | VariableKind::Select
                    )
                })
                .map(|variable| SelectorAxis {
                    name: variable.name.clone(),
                    kind: variable.kind,
                    selector_set: variable.selector_set.clone(),
                })
                .collect::<Vec<_>>();
            Entry::Selector(SelectorEntry {
                variables,
                selector_axes,
                branches,
            })
        } else {
            Entry::Value(direct_value.unwrap_or_default())
        };
        let entry = RosettaEntry {
            key,
            annotations: std::mem::take(&mut pending_annotations),
            value,
        };
        validate_entry(
            &locale,
            &plural_rule,
            ordinal_rule.as_deref().unwrap_or(&[]),
            &selector_sets,
            &entry,
        )?;
        entries.push(entry);
    }

    Ok(RosettaDocument {
        format_version: 1,
        locale,
        direction,
        fallback,
        plural_rule,
        ordinal_rule,
        entries,
    })
}

fn split_entry_assignment<'a>(
    input: &'a str,
    source_name: &str,
    line_number: usize,
) -> Result<(&'a str, &'a str)> {
    let Some((left, right)) = input.split_once('=') else {
        bail!(
            "Rosetta entry line {} in `{source_name}` must contain `=`",
            line_number
        );
    };
    let left = left.trim();
    let right = right.trim();
    if left.is_empty() {
        bail!(
            "Rosetta entry line {} in `{source_name}` is missing a key or selector label",
            line_number
        );
    }
    Ok((left, right))
}

fn parse_annotation_value(input: &str) -> String {
    if let Some(value) = input
        .strip_prefix('"')
        .and_then(|value| value.strip_suffix('"'))
    {
        value.replace("\\\"", "\"")
    } else {
        input.to_owned()
    }
}

fn parse_inline_value(
    lines: &mut std::iter::Peekable<std::iter::Enumerate<std::str::Lines<'_>>>,
    value_src: &str,
    base_indent: usize,
    source_name: &str,
    line_number: usize,
) -> Result<String> {
    if value_src == "```" {
        return parse_multiline_value(lines, base_indent, source_name, line_number);
    }
    Ok(unescape_single_line(value_src))
}

fn parse_multiline_value(
    lines: &mut std::iter::Peekable<std::iter::Enumerate<std::str::Lines<'_>>>,
    _base_indent: usize,
    source_name: &str,
    line_number: usize,
) -> Result<String> {
    let mut content = Vec::<String>::new();
    let mut min_indent = usize::MAX;
    while let Some((_, line)) = lines.next() {
        if line.trim() == "```" {
            let dedented = content
                .into_iter()
                .map(|line| {
                    if min_indent == usize::MAX || line.trim().is_empty() {
                        line.trim_end().to_owned()
                    } else {
                        line.chars()
                            .skip(min_indent)
                            .collect::<String>()
                            .trim_end()
                            .to_owned()
                    }
                })
                .collect::<Vec<_>>();
            return Ok(dedented.join("\n"));
        }
        if !line.trim().is_empty() {
            min_indent = min_indent.min(indent_width(line));
        }
        content.push(line.to_owned());
    }
    bail!(
        "unterminated multiline Rosetta value starting on line {} in `{source_name}`",
        line_number
    );
}

fn parse_variable_decls(
    input: &str,
    selector_sets: &HashMap<String, Vec<String>>,
    source_name: &str,
    line_number: usize,
) -> Result<Vec<VariableDecl>> {
    let inner = input
        .strip_prefix('{')
        .and_then(|value| value.strip_suffix('}'))
        .ok_or_else(|| anyhow!("invalid variable block"))?;
    let mut variables = Vec::new();
    let mut seen = HashSet::new();
    for chunk in inner.split(',') {
        let chunk = chunk.trim();
        if chunk.is_empty() {
            continue;
        }
        let (name, kind) = chunk
            .split_once(':')
            .map(|(name, kind)| (name.trim(), kind.trim()))
            .unwrap_or((chunk, "text"));
        if !is_valid_rosetta_identifier(name) {
            bail!(
                "invalid variable name `{name}` on line {} in `{source_name}`",
                line_number
            );
        }
        let Some((kind, selector_set)) = parse_variable_kind(kind) else {
            bail!(
                "unknown variable type `{kind}` on line {} in `{source_name}`",
                line_number
            );
        };
        if let Some(set_name) = selector_set.as_ref() {
            if !selector_sets.contains_key(set_name) {
                bail!(
                    "unknown selector-set `{set_name}` on line {} in `{source_name}`",
                    line_number
                );
            }
        }
        if seen.insert(name.to_owned()) {
            variables.push(VariableDecl {
                name: name.to_owned(),
                kind,
                selector_set,
            });
        }
    }
    Ok(variables)
}

fn validate_entry(
    locale: &str,
    plural_rule: &[String],
    ordinal_rule: &[String],
    selector_sets: &HashMap<String, Vec<String>>,
    entry: &RosettaEntry,
) -> Result<()> {
    let _ = &entry.annotations;
    let key_placeholders = entry_placeholders(&entry.value);
    match &entry.value {
        Entry::Value(_) => Ok(()),
        Entry::Selector(selector) => {
            if selector.selector_axes.is_empty() {
                bail!(
                    "Rosetta entry `{}` uses selector branches without selector variables",
                    entry.key
                );
            }
            for branch in &selector.branches {
                if branch.labels.len() != selector.selector_axes.len() {
                    bail!(
                        "Rosetta entry `{}` branch `{}` has {} labels but {} selector axes",
                        entry.key,
                        branch.labels.join(" + "),
                        branch.labels.len(),
                        selector.selector_axes.len()
                    );
                }
                for (label, axis) in branch.labels.iter().zip(&selector.selector_axes) {
                    validate_selector_label(
                        locale,
                        entry,
                        axis,
                        label,
                        plural_rule,
                        ordinal_rule,
                        selector_sets,
                    )?;
                }
            }
            if selector.selector_axes.len() > 1
                && selector
                    .branches
                    .iter()
                    .any(|branch| branch.labels.iter().any(|label| label == "*"))
                && !selector
                    .branches
                    .iter()
                    .any(|branch| branch.labels.iter().all(|label| label == "*"))
            {
                bail!(
                    "Rosetta entry `{}` uses wildcard selector branches but is missing an all-wildcard fallback",
                    entry.key
                );
            }
            for axis in &selector.selector_axes {
                if axis.kind == VariableKind::Select
                    && axis.selector_set.is_none()
                    && !selector.branches.iter().any(|branch| {
                        selector
                            .selector_axes
                            .iter()
                            .position(|candidate| candidate.name == axis.name)
                            .and_then(|index| branch.labels.get(index))
                            .map(|label| label == "*")
                            .unwrap_or(false)
                    })
                {
                    bail!(
                        "Rosetta entry `{}` custom selector `{}` requires a wildcard fallback branch",
                        entry.key,
                        axis.name
                    );
                }
            }

            let has_dynamic_select = selector
                .selector_axes
                .iter()
                .any(|axis| axis.kind == VariableKind::Select && axis.selector_set.is_none());
            if !has_dynamic_select {
                let combinations =
                    selector_combinations(selector, plural_rule, ordinal_rule, selector_sets);
                for combination in combinations {
                    if !selector
                        .branches
                        .iter()
                        .any(|branch| selector_branch_matches(branch, &combination))
                    {
                        bail!(
                            "Rosetta entry `{}` is missing a selector branch for `{}`",
                            entry.key,
                            combination.join(" + ")
                        );
                    }
                }
            }

            if key_placeholders.is_empty() {
                Ok(())
            } else {
                Ok(())
            }
        }
    }
}

fn validate_selector_label(
    _locale: &str,
    entry: &RosettaEntry,
    axis: &SelectorAxis,
    label: &str,
    plural_rule: &[String],
    ordinal_rule: &[String],
    selector_sets: &HashMap<String, Vec<String>>,
) -> Result<()> {
    if label == "*" {
        return Ok(());
    }
    let valid = match axis.kind {
        VariableKind::Cardinal => plural_rule.iter().any(|item| item == label),
        VariableKind::Ordinal => ordinal_rule.iter().any(|item| item == label),
        VariableKind::Gender => matches!(label, "masculine" | "feminine" | "non-binary"),
        VariableKind::Select => axis
            .selector_set
            .as_ref()
            .map(|set_name| {
                selector_sets
                    .get(set_name)
                    .map(|labels| labels.iter().any(|item| item == label))
                    .unwrap_or(false)
            })
            .unwrap_or(true),
        _ => false,
    };
    if valid {
        Ok(())
    } else {
        bail!(
            "Rosetta entry `{}` uses invalid selector label `{label}` for axis `{}`",
            entry.key,
            axis.name
        )
    }
}

fn selector_combinations(
    selector: &SelectorEntry,
    plural_rule: &[String],
    ordinal_rule: &[String],
    selector_sets: &HashMap<String, Vec<String>>,
) -> Vec<Vec<String>> {
    let mut dimensions = Vec::<Vec<String>>::new();
    for axis in &selector.selector_axes {
        let labels = match axis.kind {
            VariableKind::Cardinal => plural_rule.to_vec(),
            VariableKind::Ordinal => ordinal_rule.to_vec(),
            VariableKind::Gender => vec![
                "masculine".to_owned(),
                "feminine".to_owned(),
                "non-binary".to_owned(),
            ],
            VariableKind::Select => axis
                .selector_set
                .as_ref()
                .and_then(|set_name| selector_sets.get(set_name).cloned())
                .unwrap_or_else(|| vec!["*".to_owned()]),
            _ => Vec::new(),
        };
        dimensions.push(labels);
    }
    if dimensions.is_empty() {
        return Vec::new();
    }
    let mut combinations = vec![Vec::<String>::new()];
    for dimension in dimensions {
        let mut next = Vec::new();
        for prefix in &combinations {
            for label in &dimension {
                let mut combo = prefix.clone();
                combo.push(label.clone());
                next.push(combo);
            }
        }
        combinations = next;
    }
    combinations
}

fn selector_branch_matches(branch: &SelectorBranch, labels: &[String]) -> bool {
    branch
        .labels
        .iter()
        .zip(labels)
        .all(|(branch_label, requested)| branch_label == "*" || branch_label == requested)
}

fn resolve_selector_entry(
    locale: &LocaleCatalog,
    selector: &SelectorEntry,
    args: &Args,
) -> Result<String, I18nError> {
    let mut requested = Vec::new();
    for axis in &selector.selector_axes {
        let Some(value) = args.get(&axis.name) else {
            return Err(I18nError::MissingSelectorArgument {
                selector: axis.name.clone(),
                argument: axis.name.clone(),
            });
        };
        let label = match (axis.kind, value) {
            (VariableKind::Cardinal, ArgValue::Cardinal(number)) => {
                cardinal_category(&locale.locale, *number).to_owned()
            }
            (VariableKind::Ordinal, ArgValue::Ordinal(number)) => {
                ordinal_category(&locale.locale, *number).to_owned()
            }
            (VariableKind::Gender, ArgValue::Gender(gender)) => gender.label().to_owned(),
            (VariableKind::Select, ArgValue::Select(value)) => value.clone(),
            (VariableKind::Text, value) => value.display(),
            (_, value) => value.display(),
        };
        requested.push(label);
    }

    let mut best_match = None::<(&SelectorBranch, usize)>;
    for branch in &selector.branches {
        if !selector_branch_matches(branch, &requested) {
            continue;
        }
        let specificity = branch.labels.iter().filter(|label| *label != "*").count();
        if best_match
            .as_ref()
            .map(|(_, best_specificity)| specificity > *best_specificity)
            .unwrap_or(true)
        {
            best_match = Some((branch, specificity));
        }
    }
    let Some((branch, _)) = best_match else {
        return Err(I18nError::SelectorNoMatch {
            selector: selector
                .selector_axes
                .iter()
                .map(|axis| axis.name.clone())
                .collect::<Vec<_>>()
                .join(", "),
        });
    };
    Ok(interpolate_value(&branch.value, args))
}

fn interpolate_value(value: &str, args: &Args) -> String {
    let mut output = String::new();
    let mut chars = value.chars().peekable();
    while let Some(character) = chars.next() {
        if character == '{' {
            let mut name = String::new();
            let mut closed = false;
            while let Some(next) = chars.next() {
                if next == '}' {
                    closed = true;
                    break;
                }
                name.push(next);
            }
            if closed {
                if let Some(value) = args.get(name.trim()) {
                    output.push_str(&value.display());
                } else {
                    output.push('{');
                    output.push_str(name.trim());
                    output.push('}');
                }
                continue;
            }
            output.push('{');
            output.push_str(&name);
            continue;
        }
        output.push(character);
    }
    output
}

fn entry_placeholders(entry: &Entry) -> BTreeSet<String> {
    let mut placeholders = BTreeSet::new();
    match entry {
        Entry::Value(value) => collect_placeholders(value, &mut placeholders),
        Entry::Selector(selector) => {
            for branch in &selector.branches {
                collect_placeholders(&branch.value, &mut placeholders);
            }
        }
    }
    placeholders
}

fn collect_placeholders(value: &str, placeholders: &mut BTreeSet<String>) {
    let mut chars = value.chars().peekable();
    while let Some(character) = chars.next() {
        if character != '{' {
            continue;
        }
        let mut name = String::new();
        while let Some(next) = chars.next() {
            if next == '}' {
                if is_valid_rosetta_identifier(name.trim()) {
                    placeholders.insert(name.trim().to_owned());
                }
                break;
            }
            name.push(next);
        }
    }
}

fn is_valid_rosetta_key(key: &str) -> bool {
    key.chars()
        .all(|character| matches!(character, 'A'..='Z' | 'a'..='z' | '0'..='9' | '_' | '-' | '.'))
}

fn is_valid_rosetta_identifier(key: &str) -> bool {
    !key.is_empty()
        && key
            .chars()
            .all(|character| matches!(character, 'A'..='Z' | 'a'..='z' | '0'..='9' | '_' | '-'))
}

fn split_rule_list(value: &str) -> Vec<String> {
    value
        .split_whitespace()
        .map(|item| item.trim().to_owned())
        .filter(|item| !item.is_empty())
        .collect()
}

fn parse_selector_set_declaration(
    value: &str,
    source_name: &str,
    line_number: usize,
) -> Result<(String, Vec<String>)> {
    let Some((name, labels_src)) = value.split_once('=') else {
        bail!(
            "Rosetta selector-set on line {} in `{source_name}` must look like `selector-set: name = label, label`",
            line_number
        );
    };
    let name = name.trim();
    if !is_valid_rosetta_identifier(name) {
        bail!(
            "invalid selector-set name `{name}` on line {} in `{source_name}`",
            line_number
        );
    }
    let mut labels = Vec::new();
    let mut seen = HashSet::new();
    for raw_label in labels_src.split(',') {
        let label = raw_label.trim();
        if label.is_empty() {
            bail!(
                "selector-set `{name}` on line {} in `{source_name}` contains an empty label",
                line_number
            );
        }
        if label == "*" {
            bail!(
                "selector-set `{name}` on line {} in `{source_name}` must not include `*`",
                line_number
            );
        }
        if !is_valid_rosetta_identifier(label) {
            bail!(
                "invalid selector-set label `{label}` on line {} in `{source_name}`",
                line_number
            );
        }
        if !seen.insert(label.to_owned()) {
            bail!(
                "selector-set `{name}` on line {} in `{source_name}` contains duplicate label `{label}`",
                line_number
            );
        }
        labels.push(label.to_owned());
    }
    if labels.is_empty() {
        bail!(
            "selector-set `{name}` on line {} in `{source_name}` must declare at least one label",
            line_number
        );
    }
    Ok((name.to_owned(), labels))
}

fn normalize_locale_tag(value: &str) -> String {
    let value = value.trim();
    if value.is_empty() {
        return "en".to_owned();
    }
    let normalized = value.replace('_', "-");
    let mut parts = normalized.split('-');
    let Some(language) = parts.next() else {
        return "en".to_owned();
    };
    let mut output = vec![language.to_ascii_lowercase()];
    for part in parts {
        if part.len() == 2 {
            output.push(part.to_ascii_uppercase());
        } else {
            output.push(part.to_ascii_lowercase());
        }
    }
    output.join("-")
}

fn default_plural_rule_for_locale(locale: &str) -> Vec<String> {
    match normalize_locale_tag(locale)
        .split('-')
        .next()
        .unwrap_or("en")
    {
        "ru" => vec![
            "one".to_owned(),
            "few".to_owned(),
            "many".to_owned(),
            "other".to_owned(),
        ],
        _ => vec!["one".to_owned(), "other".to_owned()],
    }
}

fn default_ordinal_rule_for_locale(locale: &str) -> Vec<String> {
    match normalize_locale_tag(locale)
        .split('-')
        .next()
        .unwrap_or("en")
    {
        "en" => vec![
            "one".to_owned(),
            "two".to_owned(),
            "few".to_owned(),
            "other".to_owned(),
        ],
        _ => Vec::new(),
    }
}

fn cardinal_category(locale: &str, value: i64) -> &'static str {
    match normalize_locale_tag(locale)
        .split('-')
        .next()
        .unwrap_or("en")
    {
        "ru" => {
            let abs = value.abs();
            let mod10 = abs % 10;
            let mod100 = abs % 100;
            if mod10 == 1 && mod100 != 11 {
                "one"
            } else if (2..=4).contains(&mod10) && !(12..=14).contains(&mod100) {
                "few"
            } else if mod10 == 0 || (5..=9).contains(&mod10) || (11..=14).contains(&mod100) {
                "many"
            } else {
                "other"
            }
        }
        _ => {
            if value == 1 {
                "one"
            } else {
                "other"
            }
        }
    }
}

fn ordinal_category(locale: &str, value: i64) -> &'static str {
    match normalize_locale_tag(locale)
        .split('-')
        .next()
        .unwrap_or("en")
    {
        "en" => {
            let abs = value.abs();
            let mod10 = abs % 10;
            let mod100 = abs % 100;
            if mod10 == 1 && mod100 != 11 {
                "one"
            } else if mod10 == 2 && mod100 != 12 {
                "two"
            } else if mod10 == 3 && mod100 != 13 {
                "few"
            } else {
                "other"
            }
        }
        _ => "other",
    }
}

fn unescape_single_line(value: &str) -> String {
    let mut output = String::new();
    let mut chars = value.chars();
    while let Some(character) = chars.next() {
        if character != '\\' {
            output.push(character);
            continue;
        }
        match chars.next() {
            Some('n') => output.push('\n'),
            Some('t') => output.push('\t'),
            Some('\\') => output.push('\\'),
            Some('{') => output.push('{'),
            Some(other) => {
                output.push('\\');
                output.push(other);
            }
            None => output.push('\\'),
        }
    }
    output
}

fn indent_width(line: &str) -> usize {
    line.chars()
        .take_while(|character| character.is_whitespace())
        .count()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn parses_and_formats_simple_rosetta() {
        let catalog = Catalog::builder()
            .add_rosetta_str(
                "project",
                "test",
                r#"--- launchpad-rosetta 1
locale: en
plural-rule: one other

greeting = Hello, {name}!
"#,
            )
            .expect("rosetta should parse")
            .build()
            .expect("catalog should build");
        let translator = catalog.translator("en").expect("translator");
        assert_eq!(
            translator
                .format("project", "greeting", Args::new().text("name", "Quentin"))
                .expect("format"),
            "Hello, Quentin!"
        );
    }

    #[test]
    fn formats_russian_plural_rules() {
        let catalog = Catalog::builder()
            .add_rosetta_str(
                "project",
                "ru",
                r#"--- launchpad-rosetta 1
locale: ru
plural-rule: one few many other

files
  {count: cardinal}
  | one = {count} файл
  | few = {count} файла
  | many = {count} файлов
  | other = {count} файла
"#,
            )
            .expect("rosetta should parse")
            .build()
            .expect("catalog should build");
        let translator = catalog.translator("ru").expect("translator");
        assert_eq!(
            translator
                .format("project", "files", Args::new().cardinal("count", 1))
                .expect("format"),
            "1 файл"
        );
        assert_eq!(
            translator
                .format("project", "files", Args::new().cardinal("count", 2))
                .expect("format"),
            "2 файла"
        );
        assert_eq!(
            translator
                .format("project", "files", Args::new().cardinal("count", 5))
                .expect("format"),
            "5 файлов"
        );
    }

    #[test]
    fn resolves_locale_fallback_chain() {
        let catalog = Catalog::builder()
            .add_rosetta_str(
                "project",
                "en",
                r#"--- launchpad-rosetta 1
locale: en
plural-rule: one other

hello = Hello
"#,
            )
            .expect("rosetta should parse")
            .build()
            .expect("catalog should build");
        let translator = catalog.translator("en-US").expect("translator");
        assert_eq!(translator.text("project", "hello").expect("text"), "Hello");
    }

    #[test]
    fn formats_named_selector_set_pronouns() {
        let catalog = Catalog::builder()
            .add_rosetta_str(
                "project",
                "pronouns",
                r#"--- launchpad-rosetta 1
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
"#,
            )
            .expect("rosetta should parse")
            .build()
            .expect("catalog should build");
        let translator = catalog.translator("en").expect("translator");
        assert_eq!(
            translator
                .format("project", "reply", Args::new().select("pronouns", "xe-xem"),)
                .expect("format"),
            "Xe replied"
        );
    }

    #[test]
    fn uses_wildcard_for_unknown_named_selector_value() {
        let catalog = Catalog::builder()
            .add_rosetta_str(
                "project",
                "pronouns-fallback",
                r#"--- launchpad-rosetta 1
locale: en
plural-rule: one other
selector-set: pronouns = he-him, she-her, they-them

reply
  {pronouns: select(pronouns)}
  | he-him = He replied
  | she-her = She replied
  | they-them = They replied
  | * = They replied
"#,
            )
            .expect("rosetta should parse")
            .build()
            .expect("catalog should build");
        let translator = catalog.translator("en").expect("translator");
        assert_eq!(
            translator
                .format(
                    "project",
                    "reply",
                    Args::new().select("pronouns", "fae-faer"),
                )
                .expect("format"),
            "They replied"
        );
    }

    #[test]
    fn formats_non_binary_gender_selector() {
        let catalog = Catalog::builder()
            .add_rosetta_str(
                "project",
                "gender",
                r#"--- launchpad-rosetta 1
locale: en
plural-rule: one other

reply
  {gender: gender}
  | masculine = He replied
  | feminine = She replied
  | non-binary = They replied
"#,
            )
            .expect("rosetta should parse")
            .build()
            .expect("catalog should build");
        let translator = catalog.translator("en").expect("translator");
        assert_eq!(
            translator
                .format(
                    "project",
                    "reply",
                    Args::new().gender("gender", Gender::NonBinary),
                )
                .expect("format"),
            "They replied"
        );
    }

    #[test]
    fn rejects_unknown_selector_set_reference() {
        let error = Catalog::builder()
            .add_rosetta_str(
                "project",
                "missing-selector-set",
                r#"--- launchpad-rosetta 1
locale: en
plural-rule: one other

reply
  {pronouns: select(pronouns)}
  | * = They replied
"#,
            )
            .expect_err("unknown selector-set should be rejected");
        assert!(
            error
                .to_string()
                .contains("unknown selector-set `pronouns`"),
            "unexpected error: {error}"
        );
    }

    #[test]
    fn rejects_invalid_named_selector_label() {
        let error = Catalog::builder()
            .add_rosetta_str(
                "project",
                "invalid-selector-label",
                r#"--- launchpad-rosetta 1
locale: en
plural-rule: one other
selector-set: pronouns = he-him, they-them

reply
  {pronouns: select(pronouns)}
  | he-him = He replied
  | ze-zir = Ze replied
  | * = They replied
"#,
            )
            .expect_err("invalid selector-set branch label should be rejected");
        assert!(
            error
                .to_string()
                .contains("invalid selector label `ze-zir` for axis `pronouns`"),
            "unexpected error: {error}"
        );
    }

    #[test]
    fn rejects_incomplete_named_selector_coverage() {
        let error = Catalog::builder()
            .add_rosetta_str(
                "project",
                "incomplete-selector-set",
                r#"--- launchpad-rosetta 1
locale: en
plural-rule: one other
selector-set: pronouns = he-him, they-them

reply
  {pronouns: select(pronouns)}
  | he-him = He replied
"#,
            )
            .expect_err("closed selector-set coverage should be enforced");
        assert!(
            error
                .to_string()
                .contains("missing a selector branch for `they-them`"),
            "unexpected error: {error}"
        );
    }

    #[test]
    fn rejects_legacy_other_gender_selector_label() {
        let error = Catalog::builder()
            .add_rosetta_str(
                "project",
                "legacy-gender",
                r#"--- launchpad-rosetta 1
locale: en
plural-rule: one other

reply
  {gender: gender}
  | masculine = He replied
  | feminine = She replied
  | other = They replied
"#,
            )
            .expect_err("legacy gender selector label should be rejected");
        assert!(
            error
                .to_string()
                .contains("invalid selector label `other` for axis `gender`"),
            "unexpected error: {error}"
        );
    }
}
