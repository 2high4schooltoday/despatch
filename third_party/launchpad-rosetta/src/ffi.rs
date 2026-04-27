use std::cell::RefCell;
use std::collections::BTreeMap;
use std::ffi::{CStr, CString, c_char};
use std::ptr;

use anyhow::{Result, anyhow, bail};
use serde::Deserialize;
use serde_json::Value;

use crate::{ArgValue, Args, Catalog, CatalogBuilder, Gender};

thread_local! {
    static LAST_ERROR: RefCell<Option<String>> = const { RefCell::new(None) };
}

pub struct CatalogBuilderHandle {
    inner: CatalogBuilder,
}

pub struct CatalogHandle {
    inner: Catalog,
}

#[derive(Debug, Deserialize)]
#[serde(untagged)]
enum JsonArg {
    Text(String),
    Integer(i64),
    StringList(Vec<String>),
    Typed {
        #[serde(rename = "type")]
        kind: String,
        value: Value,
    },
}

fn clear_last_error() {
    LAST_ERROR.with(|slot| {
        *slot.borrow_mut() = None;
    });
}

fn set_last_error(error: impl Into<String>) {
    LAST_ERROR.with(|slot| {
        *slot.borrow_mut() = Some(error.into());
    });
}

fn read_required_string(value: *const c_char, label: &str) -> Result<String> {
    if value.is_null() {
        bail!("{label} must not be null");
    }
    let value = unsafe { CStr::from_ptr(value) };
    Ok(value
        .to_str()
        .map_err(|_| anyhow!("{label} must be valid UTF-8"))?
        .to_owned())
}

fn into_c_string(value: String) -> Result<*mut c_char> {
    Ok(CString::new(value)
        .map_err(|_| anyhow!("string value contains an interior NUL byte"))?
        .into_raw())
}

fn last_error_ptr() -> *mut c_char {
    LAST_ERROR.with(|slot| match slot.borrow().clone() {
        Some(message) => into_c_string(message).unwrap_or(ptr::null_mut()),
        None => ptr::null_mut(),
    })
}

fn parse_args_json(args_json: Option<&str>) -> Result<Args> {
    let Some(args_json) = args_json.filter(|value| !value.trim().is_empty()) else {
        return Ok(Args::new());
    };
    let parsed: BTreeMap<String, JsonArg> =
        serde_json::from_str(args_json).map_err(|error| anyhow!("invalid args json: {error}"))?;
    let mut args = Args::new();
    for (name, value) in parsed {
        args.insert(name, parse_json_arg(value)?);
    }
    Ok(args)
}

fn parse_json_arg(value: JsonArg) -> Result<ArgValue> {
    match value {
        JsonArg::Text(value) => Ok(ArgValue::Text(value)),
        JsonArg::Integer(value) => Ok(ArgValue::Cardinal(value)),
        JsonArg::StringList(value) => Ok(ArgValue::List(value)),
        JsonArg::Typed { kind, value } => match kind.trim().to_ascii_lowercase().as_str() {
            "text" => Ok(ArgValue::Text(json_string(value, "text")?)),
            "cardinal" => Ok(ArgValue::Cardinal(json_i64(value, "cardinal")?)),
            "ordinal" => Ok(ArgValue::Ordinal(json_i64(value, "ordinal")?)),
            "gender" => Ok(ArgValue::Gender(parse_gender(&json_string(
                value, "gender",
            )?)?)),
            "select" => Ok(ArgValue::Select(json_string(value, "select")?)),
            "list" => Ok(ArgValue::List(json_string_list(value)?)),
            other => bail!("unsupported argument type `{other}`"),
        },
    }
}

fn parse_gender(value: &str) -> Result<Gender> {
    match value.trim().to_ascii_lowercase().as_str() {
        "masculine" => Ok(Gender::Masculine),
        "feminine" => Ok(Gender::Feminine),
        "non-binary" => Ok(Gender::NonBinary),
        other => bail!("unsupported gender `{other}`"),
    }
}

#[cfg(test)]
mod tests {
    use super::parse_gender;

    use crate::Gender;

    #[test]
    fn parses_non_binary_gender() {
        assert_eq!(
            parse_gender("non-binary").expect("non-binary should parse"),
            Gender::NonBinary
        );
    }

    #[test]
    fn rejects_legacy_other_gender() {
        let error = parse_gender("other").expect_err("legacy gender should be rejected");
        assert!(
            error.to_string().contains("unsupported gender `other`"),
            "unexpected error: {error}"
        );
    }
}

fn json_string(value: Value, label: &str) -> Result<String> {
    value
        .as_str()
        .map(ToOwned::to_owned)
        .ok_or_else(|| anyhow!("{label} arguments must use a string value"))
}

fn json_i64(value: Value, label: &str) -> Result<i64> {
    value
        .as_i64()
        .ok_or_else(|| anyhow!("{label} arguments must use an integer value"))
}

fn json_string_list(value: Value) -> Result<Vec<String>> {
    let array = value
        .as_array()
        .ok_or_else(|| anyhow!("list arguments must use an array value"))?;
    array
        .iter()
        .map(|item| {
            item.as_str()
                .map(ToOwned::to_owned)
                .ok_or_else(|| anyhow!("list arguments must contain only strings"))
        })
        .collect()
}

fn translate_text(
    catalog: &CatalogHandle,
    locale: *const c_char,
    bundle: *const c_char,
    key: *const c_char,
) -> Result<String> {
    let locale = read_required_string(locale, "locale")?;
    let bundle = read_required_string(bundle, "bundle")?;
    let key = read_required_string(key, "key")?;
    catalog
        .inner
        .translator(&locale)?
        .text(&bundle, &key)
        .map_err(Into::into)
}

fn translate_format(
    catalog: &CatalogHandle,
    locale: *const c_char,
    bundle: *const c_char,
    key: *const c_char,
    args_json: *const c_char,
) -> Result<String> {
    let locale = read_required_string(locale, "locale")?;
    let bundle = read_required_string(bundle, "bundle")?;
    let key = read_required_string(key, "key")?;
    let args_json = if args_json.is_null() {
        None
    } else {
        Some(read_required_string(args_json, "args_json")?)
    };
    let args = parse_args_json(args_json.as_deref())?;
    catalog
        .inner
        .translator(&locale)?
        .format(&bundle, &key, args)
        .map_err(Into::into)
}

fn bundle_locales(catalog: &CatalogHandle, bundle: *const c_char) -> Result<String> {
    let bundle = read_required_string(bundle, "bundle")?;
    serde_json::to_string(&catalog.inner.bundle_locales(&bundle))
        .map_err(|error| anyhow!("failed to serialize locale list: {error}"))
}

#[unsafe(no_mangle)]
pub extern "C" fn lp_i18n_catalog_builder_new() -> *mut CatalogBuilderHandle {
    clear_last_error();
    Box::into_raw(Box::new(CatalogBuilderHandle {
        inner: Catalog::builder(),
    }))
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn lp_i18n_catalog_builder_free(builder: *mut CatalogBuilderHandle) {
    if builder.is_null() {
        return;
    }
    unsafe {
        drop(Box::from_raw(builder));
    }
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn lp_i18n_catalog_builder_add_rosetta_str(
    builder: *mut CatalogBuilderHandle,
    bundle: *const c_char,
    source_name: *const c_char,
    text: *const c_char,
) -> bool {
    let Some(builder) = (unsafe { builder.as_mut() }) else {
        set_last_error("builder must not be null");
        return false;
    };
    match (
        read_required_string(bundle, "bundle"),
        read_required_string(source_name, "source_name"),
        read_required_string(text, "text"),
    ) {
        (Ok(bundle), Ok(source_name), Ok(text)) => {
            match builder.inner.push_rosetta_str(&bundle, &source_name, &text) {
                Ok(()) => {
                    clear_last_error();
                    true
                }
                Err(error) => {
                    set_last_error(error.to_string());
                    false
                }
            }
        }
        (Err(error), _, _) | (_, Err(error), _) | (_, _, Err(error)) => {
            set_last_error(error.to_string());
            false
        }
    }
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn lp_i18n_catalog_builder_add_json_compat_str(
    builder: *mut CatalogBuilderHandle,
    bundle: *const c_char,
    locale: *const c_char,
    source_name: *const c_char,
    text: *const c_char,
) -> bool {
    let Some(builder) = (unsafe { builder.as_mut() }) else {
        set_last_error("builder must not be null");
        return false;
    };
    match (
        read_required_string(bundle, "bundle"),
        read_required_string(locale, "locale"),
        read_required_string(source_name, "source_name"),
        read_required_string(text, "text"),
    ) {
        (Ok(bundle), Ok(locale), Ok(source_name), Ok(text)) => {
            match builder
                .inner
                .push_json_compat_str(&bundle, &locale, &source_name, &text)
            {
                Ok(()) => {
                    clear_last_error();
                    true
                }
                Err(error) => {
                    set_last_error(error.to_string());
                    false
                }
            }
        }
        (Err(error), _, _, _)
        | (_, Err(error), _, _)
        | (_, _, Err(error), _)
        | (_, _, _, Err(error)) => {
            set_last_error(error.to_string());
            false
        }
    }
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn lp_i18n_catalog_builder_build(
    builder: *mut CatalogBuilderHandle,
) -> *mut CatalogHandle {
    if builder.is_null() {
        set_last_error("builder must not be null");
        return ptr::null_mut();
    }
    let builder = unsafe { Box::from_raw(builder) };
    match builder.inner.build() {
        Ok(catalog) => {
            clear_last_error();
            Box::into_raw(Box::new(CatalogHandle { inner: catalog }))
        }
        Err(error) => {
            set_last_error(error.to_string());
            ptr::null_mut()
        }
    }
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn lp_i18n_catalog_free(catalog: *mut CatalogHandle) {
    if catalog.is_null() {
        return;
    }
    unsafe {
        drop(Box::from_raw(catalog));
    }
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn lp_i18n_catalog_has(
    catalog: *const CatalogHandle,
    locale: *const c_char,
    bundle: *const c_char,
    key: *const c_char,
) -> bool {
    let Some(catalog) = (unsafe { catalog.as_ref() }) else {
        set_last_error("catalog must not be null");
        return false;
    };
    match (
        read_required_string(locale, "locale"),
        read_required_string(bundle, "bundle"),
        read_required_string(key, "key"),
    ) {
        (Ok(locale), Ok(bundle), Ok(key)) => {
            clear_last_error();
            catalog
                .inner
                .translator(&locale)
                .map(|translator| translator.has(&bundle, &key))
                .unwrap_or(false)
        }
        (Err(error), _, _) | (_, Err(error), _) | (_, _, Err(error)) => {
            set_last_error(error.to_string());
            false
        }
    }
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn lp_i18n_catalog_text(
    catalog: *const CatalogHandle,
    locale: *const c_char,
    bundle: *const c_char,
    key: *const c_char,
) -> *mut c_char {
    let Some(catalog) = (unsafe { catalog.as_ref() }) else {
        set_last_error("catalog must not be null");
        return ptr::null_mut();
    };
    match translate_text(catalog, locale, bundle, key).and_then(into_c_string) {
        Ok(value) => {
            clear_last_error();
            value
        }
        Err(error) => {
            set_last_error(error.to_string());
            ptr::null_mut()
        }
    }
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn lp_i18n_catalog_format(
    catalog: *const CatalogHandle,
    locale: *const c_char,
    bundle: *const c_char,
    key: *const c_char,
    args_json: *const c_char,
) -> *mut c_char {
    let Some(catalog) = (unsafe { catalog.as_ref() }) else {
        set_last_error("catalog must not be null");
        return ptr::null_mut();
    };
    match translate_format(catalog, locale, bundle, key, args_json).and_then(into_c_string) {
        Ok(value) => {
            clear_last_error();
            value
        }
        Err(error) => {
            set_last_error(error.to_string());
            ptr::null_mut()
        }
    }
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn lp_i18n_catalog_bundle_locales_json(
    catalog: *const CatalogHandle,
    bundle: *const c_char,
) -> *mut c_char {
    let Some(catalog) = (unsafe { catalog.as_ref() }) else {
        set_last_error("catalog must not be null");
        return ptr::null_mut();
    };
    match bundle_locales(catalog, bundle).and_then(into_c_string) {
        Ok(value) => {
            clear_last_error();
            value
        }
        Err(error) => {
            set_last_error(error.to_string());
            ptr::null_mut()
        }
    }
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn lp_i18n_locale_endonym(locale: *const c_char) -> *mut c_char {
    match read_required_string(locale, "locale")
        .map(|locale| crate::locale_endonym(&locale))
        .and_then(into_c_string)
    {
        Ok(value) => {
            clear_last_error();
            value
        }
        Err(error) => {
            set_last_error(error.to_string());
            ptr::null_mut()
        }
    }
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn lp_i18n_locale_english_name(locale: *const c_char) -> *mut c_char {
    match read_required_string(locale, "locale")
        .map(|locale| crate::locale_english_name(&locale))
        .and_then(into_c_string)
    {
        Ok(value) => {
            clear_last_error();
            value
        }
        Err(error) => {
            set_last_error(error.to_string());
            ptr::null_mut()
        }
    }
}

#[unsafe(no_mangle)]
pub extern "C" fn lp_i18n_last_error_message() -> *mut c_char {
    last_error_ptr()
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn lp_i18n_string_free(value: *mut c_char) {
    if value.is_null() {
        return;
    }
    unsafe {
        drop(CString::from_raw(value));
    }
}
