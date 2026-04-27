#ifndef LP_I18N_H
#define LP_I18N_H

#include <stdbool.h>

#ifdef __cplusplus
extern "C" {
#endif

typedef struct lp_i18n_catalog_builder lp_i18n_catalog_builder;
typedef struct lp_i18n_catalog lp_i18n_catalog;

lp_i18n_catalog_builder *lp_i18n_catalog_builder_new(void);
void lp_i18n_catalog_builder_free(lp_i18n_catalog_builder *builder);
bool lp_i18n_catalog_builder_add_rosetta_str(
    lp_i18n_catalog_builder *builder,
    const char *bundle,
    const char *source_name,
    const char *text
);
bool lp_i18n_catalog_builder_add_json_compat_str(
    lp_i18n_catalog_builder *builder,
    const char *bundle,
    const char *locale,
    const char *source_name,
    const char *text
);
lp_i18n_catalog *lp_i18n_catalog_builder_build(lp_i18n_catalog_builder *builder);

void lp_i18n_catalog_free(lp_i18n_catalog *catalog);
bool lp_i18n_catalog_has(
    const lp_i18n_catalog *catalog,
    const char *locale,
    const char *bundle,
    const char *key
);
char *lp_i18n_catalog_text(
    const lp_i18n_catalog *catalog,
    const char *locale,
    const char *bundle,
    const char *key
);
char *lp_i18n_catalog_format(
    const lp_i18n_catalog *catalog,
    const char *locale,
    const char *bundle,
    const char *key,
    const char *args_json
);
char *lp_i18n_catalog_bundle_locales_json(
    const lp_i18n_catalog *catalog,
    const char *bundle
);
char *lp_i18n_locale_endonym(const char *locale);
char *lp_i18n_locale_english_name(const char *locale);
char *lp_i18n_last_error_message(void);
void lp_i18n_string_free(char *value);

#ifdef __cplusplus
}
#endif

#endif
