import test from "node:test";
import assert from "node:assert/strict";

import {
  Args,
  CatalogBuilder,
  Gender,
  localeEnglishName,
  localeEndonym,
} from "../index.js";

const sample = `--- launchpad-rosetta 1
locale: en
plural-rule: one other
selector-set: pronouns = he-him, she-her, they-them, ze-zir, xe-xem

hello = Hello, {name}!

invite
  {host, count: cardinal, role: select}
  | one + admin = {host} invited one admin
  | one + * = {host} invited one person
  | other + admin = {host} invited {count} admins
  | other + * = {host} invited {count} people
  | * + * = {host} invited {count} people

reply
  {pronouns: select(pronouns)}
  | he-him = He replied
  | she-her = She replied
  | they-them = They replied
  | ze-zir = Ze replied
  | xe-xem = Xe replied
  | * = They replied
`;

test("formats text and selectors", () => {
  const catalog = new CatalogBuilder().addRosettaString("project", "sample.lpr", sample).build();
  const translator = catalog.translator("en-US");

  assert.equal(
    translator.format("project", "hello", new Args().text("name", "Quentin")),
    "Hello, Quentin!",
  );
  assert.equal(
    translator.format(
      "project",
      "invite",
      new Args().text("host", "Launchpad").cardinal("count", 4).select("role", "admin"),
    ),
    "Launchpad invited 4 admins",
  );
  assert.equal(
    translator.format(
      "project",
      "reply",
      new Args().select("pronouns", "xe-xem"),
    ),
    "Xe replied",
  );

  catalog.close();
});

test("exposes locale helpers", () => {
  assert.equal(localeEnglishName("ru"), "Russian");
  assert.equal(localeEndonym("de"), "Deutsch");
  assert.equal(Gender.NON_BINARY, "non-binary");
});

test("locale context falls back to the key", () => {
  const catalog = new CatalogBuilder().addRosettaString("project", "sample.lpr", sample).build();
  const locale = catalog.localeContext("en", "project");

  assert.equal(locale.text("missing"), "missing");

  catalog.close();
});
