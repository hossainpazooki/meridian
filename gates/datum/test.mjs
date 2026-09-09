#!/usr/bin/env node
// DATUM conformance pack: the checker's own controls (DATUM rule 4 applied
// to DATUM). Holds three things over every fixture: every PASS case is
// accepted; every FAIL case is refused with a reason that starts with its
// expect_reason; no refusal is emitted that no fixture expected. With
// --mutate, disables each rule in turn and asserts a negative fixture stops
// being refused, so no rule is unfalsified. Also exercises the CLI's exit
// codes and the PIN check. Exit 0 only when everything holds.
//
// No dependencies. Node 20 or later.

import {
  readFileSync, readdirSync, mkdtempSync, writeFileSync, rmSync, cpSync,
  statSync, existsSync,
} from "node:fs";
import { join, dirname, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { createHash } from "node:crypto";
import { checkRows, RULE_NAMES } from "./check.mjs";

const HERE = dirname(fileURLToPath(import.meta.url));
const FIX = join(HERE, "fixtures");
const CHECK = join(HERE, "check.mjs");
const WRITE_STATUS = process.argv.includes("--write-status");
const CHECK_STATUS = process.argv.includes("--check-status");
// the status block is derived from a full green run, mutation included
const MUTATE = process.argv.includes("--mutate") || WRITE_STATUS || CHECK_STATUS;

const failures = [];
const fail = (where, why) => failures.push(`TEST FAIL ${where}: ${why}`);

// ---------- helpers ----------
function listJson(dir) {
  if (!existsSync(dir)) return [];
  return readdirSync(dir).filter((f) => f.endsWith(".json")).sort().map((f) => join(dir, f));
}
function walkJson(dir, out = []) {
  if (!existsSync(dir)) return out;
  for (const e of readdirSync(dir, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
    const p = join(dir, e.name);
    if (e.isDirectory()) walkJson(p, out);
    else if (e.name.endsWith(".json")) out.push(p);
  }
  return out;
}
const sha256lf = (buf) => createHash("sha256").update(buf.toString("utf8").replace(/\r\n/g, "\n")).digest("hex");
const rel = (p) => relative(HERE, p).split(sep).join("/");

// ---------- the reason vocabulary ----------
const REASONS = readFileSync(join(HERE, "reasons.md"), "utf8")
  .split("\n")
  .filter((l) => l.startsWith("- `"))
  .map((l) => l.slice(3, l.indexOf("`", 3)))
  .filter((r) => !r.startsWith("schema: <path>"))
  // "schema: <path> must be integer / must be string / must be object" is
  // documented once; the prefix that reaches a row is "schema: " itself.
  .concat(["schema: "]);
const inVocabulary = (reason) => REASONS.some((r) => reason.startsWith(r));
const expectedByFixture = new Set();

// ---------- load fixtures ----------
function loadCase(path) {
  let c;
  try { c = JSON.parse(readFileSync(path, "utf8")); } catch (e) { fail(rel(path), `not JSON: ${e.message}`); return null; }
  if (typeof c !== "object" || c === null || Array.isArray(c)) { fail(rel(path), "case must be an object"); return null; }
  if (typeof c.case !== "string" || !c.case) fail(rel(path), "case needs a description");
  if (c.expect !== "PASS" && c.expect !== "FAIL") fail(rel(path), "expect must be PASS or FAIL");
  if (c.expect === "FAIL" && (typeof c.expect_reason !== "string" || !c.expect_reason)) fail(rel(path), "FAIL case needs expect_reason");
  if (c.expect === "PASS" && "expect_reason" in c) fail(rel(path), "PASS case must not carry expect_reason");
  if (("row" in c) === ("rows" in c)) fail(rel(path), "case needs exactly one of row / rows");
  if ("rows" in c && !Array.isArray(c.rows)) fail(rel(path), "rows must be an array");
  const rows = "rows" in c ? c.rows : [c.row];
  return { path, name: rel(path), expect: c.expect, reason: c.expect_reason, entries: rows.map((row, i) => ({ file: `${rel(path)}#${i}`, row })) };
}
const positive = listJson(join(FIX, "positive")).map(loadCase).filter(Boolean);
const negative = listJson(join(FIX, "negative")).map(loadCase).filter(Boolean);
if (positive.length === 0) fail("fixtures/positive", "no fixtures");
if (negative.length === 0) fail("fixtures/negative", "no fixtures");

// ---------- 1 + 2 + 3: every fixture, in process ----------
for (const c of positive) {
  const reasons = checkRows(c.entries);
  for (const r of reasons) fail(c.name, `PASS case refused: ${r.reason}`);
}
for (const c of negative) {
  const reasons = checkRows(c.entries);
  if (reasons.length === 0) { fail(c.name, `FAIL case accepted (expected "${c.reason}")`); continue; }
  expectedByFixture.add(c.reason);
  for (const r of reasons) {
    if (!r.reason.startsWith(c.reason)) fail(c.name, `unexpected refusal "${r.reason}" (expected prefix "${c.reason}")`);
    if (!inVocabulary(r.reason)) fail(c.name, `refusal outside reasons.md: "${r.reason}"`);
  }
}
// every vocabulary entry is expected by at least one fixture; a reason no
// fixture can produce is a rule nobody has seen fire.
for (const r of REASONS) {
  if (r === "schema: ") continue;
  if (![...expectedByFixture].some((e) => e.startsWith(r) || r.startsWith(e))) fail("reasons.md", `no negative fixture expects "${r}"`);
}
// every fixture's expect_reason is in the vocabulary
for (const c of negative) if (!inVocabulary(c.reason)) fail(c.name, `expect_reason outside reasons.md: "${c.reason}"`);

// ---------- real rows: PASS fixtures, hash-bound ----------
const REAL = join(FIX, "real");
const realFiles = walkJson(REAL);
const sourceMd = join(REAL, "SOURCE.md");
if (realFiles.length > 0 && !existsSync(sourceMd)) fail("fixtures/real", "rows present but no SOURCE.md binds them");
const bound = new Map();
if (existsSync(sourceMd)) {
  for (const l of readFileSync(sourceMd, "utf8").split("\n")) {
    const m = /^([0-9a-f]{64})  (\S+)$/.exec(l.trim());
    if (m) bound.set(m[2], m[1]);
  }
}
const byRepo = new Map();
for (const p of realFiles) {
  const r = relative(REAL, p).split(sep).join("/");
  const h = sha256lf(readFileSync(p));
  if (!bound.has(r)) fail(`fixtures/real/${r}`, "not listed in SOURCE.md");
  else if (bound.get(r) !== h) fail(`fixtures/real/${r}`, `sha256 ${h} does not match SOURCE.md ${bound.get(r)}`);
  let row;
  try { row = JSON.parse(readFileSync(p, "utf8")); } catch (e) { fail(`fixtures/real/${r}`, `not JSON: ${e.message}`); continue; }
  const repo = r.split("/")[0];
  if (!byRepo.has(repo)) byRepo.set(repo, []);
  byRepo.get(repo).push({ file: `fixtures/real/${r}`, row });
}
for (const [r] of bound) if (!existsSync(join(REAL, r))) fail("fixtures/real/SOURCE.md", `lists missing file ${r}`);
for (const [repo, entries] of byRepo) {
  for (const x of checkRows(entries)) fail(`fixtures/real/${repo}`, `real row refused: ${x.file}: ${x.reason}`);
}

// ---------- CLI exit codes ----------
function run(args, cwd) {
  const r = spawnSync(process.execPath, [CHECK, ...args], { encoding: "utf8", cwd });
  return { code: r.status, out: r.stdout, err: r.stderr };
}
const scratch = mkdtempSync(join(tmpdir(), "datum-test-"));
try {
  // a conforming rows dir: unwrap the set-claimable fixture
  const conf = join(scratch, "conforming");
  const setCase = positive.find((c) => c.entries.length > 1 && c.name.includes("claimable"));
  if (!setCase) fail("cli", "no multi-row PASS fixture to build a conforming directory from");
  else {
    const { mkdirSync } = await import("node:fs");
    mkdirSync(conf);
    setCase.entries.forEach((e, i) => writeFileSync(join(conf, `row-${i}.json`), JSON.stringify(e.row, null, 2) + "\n"));
    let r = run([conf]);
    if (r.code !== 0) fail("cli conforming", `exit ${r.code}, expected 0\n${r.out}${r.err}`);
    if (!/CLAIMABLE/.test(r.out)) fail("cli conforming", `no CLAIMABLE line in output:\n${r.out}`);
    r = run([conf, "--json"]);
    let j; try { j = JSON.parse(r.out); } catch { fail("cli --json", `output is not JSON:\n${r.out}`); }
    if (j && !(Array.isArray(j) && j.length === 1 && j[0].status === "CLAIMABLE" && j[0].twins === 2)) fail("cli --json", `unexpected shape: ${r.out}`);
    // refusal -> 1
    const bad = join(conf, "row-bad.json");
    writeFileSync(bad, JSON.stringify({ ...setCase.entries[0].row, schema: "datum/gate-verdict/9" }) + "\n");
    r = run([conf]);
    if (r.code !== 1) fail("cli refusal", `exit ${r.code}, expected 1`);
    if (!/^FAIL row-bad\.json: schema: schema must be/m.test(r.out)) fail("cli refusal", `no FAIL line for row-bad.json:\n${r.out}`);
    rmSync(bad);
    // not JSON -> 2
    writeFileSync(join(conf, "row-junk.json"), "{ not json\n");
    r = run([conf]);
    if (r.code !== 2) fail("cli non-json", `exit ${r.code}, expected 2`);
    rmSync(join(conf, "row-junk.json"));
    // every file in the rows directory is a row: an upper-case extension is
    // read, not skipped (a refusable row must not hide behind its name)
    const shouty = join(conf, "ROW-LIVE-RED.JSON");
    writeFileSync(shouty, JSON.stringify({ ...setCase.entries[0].row, result: "RED", checks: { duplicate_absorbed: 1, collision_refused: 0 } }) + "\n");
    r = run([conf]);
    if (r.code !== 1) fail("cli upper-case extension", `exit ${r.code}, expected 1 (row skipped by name?)`);
    if (!/ROW-LIVE-RED\.JSON/.test(r.out)) fail("cli upper-case extension", `no FAIL line names the file:\n${r.out}`);
    rmSync(shouty);
    // a non-row file in the rows directory is unevaluable, never ignored
    writeFileSync(join(conf, "notes.txt"), "not a row\n");
    r = run([conf]);
    if (r.code !== 2) fail("cli stray file", `exit ${r.code}, expected 2 (stray file ignored?)`);
    rmSync(join(conf, "notes.txt"));
  }
  // missing dir -> 2 ; empty dir -> 2
  let r = run([join(scratch, "nope")]);
  if (r.code !== 2) fail("cli missing dir", `exit ${r.code}, expected 2`);
  const { mkdirSync } = await import("node:fs");
  const empty = join(scratch, "empty"); mkdirSync(empty);
  r = run([empty]);
  if (r.code !== 2) fail("cli empty dir", `exit ${r.code}, expected 2`);
  // no args -> 2
  r = run([]);
  if (r.code !== 2) fail("cli no args", `exit ${r.code}, expected 2`);

  // ---------- PIN: a vendored copy verifies, an edited one does not ----------
  const vend = join(scratch, "vendor", "datum");
  cpSync(HERE, vend, { recursive: true, filter: (s) => !s.endsWith("PIN") });
  // the vendoring contract: the schema is copied beside check.mjs. In the
  // datum repo it lives under ../schema/; in a vendored copy it is already
  // beside this file (found 2026-09-09 when MERIDIAN first ran the vendored
  // self-test: the ../schema/ path does not exist outside datum).
  const schemaSrc = [join(HERE, "gate-verdict.v1.json"), join(HERE, "..", "schema", "gate-verdict.v1.json")].find(existsSync);
  if (!schemaSrc) fail("pin", "schema gate-verdict.v1.json not found beside test.mjs or under ../schema/");
  else cpSync(schemaSrc, join(vend, "gate-verdict.v1.json"));
  const sha = "0".repeat(40);
  r = spawnSync(process.execPath, [join(vend, "check.mjs"), "--write-pin", sha], { encoding: "utf8" });
  if (r.status !== 0) fail("pin write", `exit ${r.status}: ${r.stderr}`);
  else {
    writeFileSync(join(vend, "PIN"), r.stdout);
    if (!r.stdout.startsWith(`datum ${sha}\n`)) fail("pin write", `PIN does not start with the datum sha line:\n${r.stdout.slice(0, 80)}`);
    let v = spawnSync(process.execPath, [join(vend, "check.mjs"), "--verify-pin", conf], { encoding: "utf8" });
    if (v.status !== 0) fail("pin verify clean", `exit ${v.status}, expected 0\n${v.stdout}${v.stderr}`);
    // altered file -> 2
    const target = join(vend, "reasons.md");
    writeFileSync(target, readFileSync(target, "utf8") + "\nedited locally\n");
    v = spawnSync(process.execPath, [join(vend, "check.mjs"), "--verify-pin", conf], { encoding: "utf8" });
    if (v.status !== 2) fail("pin verify altered", `exit ${v.status}, expected 2`);
    if (!/reasons\.md/.test(v.stderr + v.stdout)) fail("pin verify altered", "does not name the altered file");
    cpSync(join(HERE, "reasons.md"), target);
    // unlisted file -> 2
    writeFileSync(join(vend, "extra.mjs"), "// unlisted\n");
    v = spawnSync(process.execPath, [join(vend, "check.mjs"), "--verify-pin", conf], { encoding: "utf8" });
    if (v.status !== 2) fail("pin verify unlisted", `exit ${v.status}, expected 2`);
    rmSync(join(vend, "extra.mjs"));
    // CRLF re-encoding of a vendored file must still verify (LF-normalised hashing)
    writeFileSync(target, readFileSync(target, "utf8").replace(/\n/g, "\r\n"));
    v = spawnSync(process.execPath, [join(vend, "check.mjs"), "--verify-pin", conf], { encoding: "utf8" });
    if (v.status !== 0) fail("pin verify crlf", `exit ${v.status}, expected 0 (hashes are LF-normalised)\n${v.stdout}${v.stderr}`);
    // missing PIN -> 2
    rmSync(join(vend, "PIN"));
    v = spawnSync(process.execPath, [join(vend, "check.mjs"), "--verify-pin", conf], { encoding: "utf8" });
    if (v.status !== 2) fail("pin verify missing", `exit ${v.status}, expected 2`);
  }
} finally {
  rmSync(scratch, { recursive: true, force: true });
}

// ---------- --mutate: every rule has a fixture that detects it disabled ----------
let mutated = 0;
if (MUTATE) {
  if (!Array.isArray(RULE_NAMES) || RULE_NAMES.length === 0) fail("mutate", "check.mjs exports no RULE_NAMES");
  for (const name of RULE_NAMES || []) {
    const flipped = negative.filter((c) => checkRows(c.entries, { disabled: new Set([name]) }).length === 0);
    if (flipped.length === 0) fail("mutate", `rule "${name}" disabled and every negative fixture is still refused: the rule is unfalsified`);
    mutated++;
  }
  // and the rule table is the whole checker: disabling everything accepts every negative fixture
  const still = negative.filter((c) => checkRows(c.entries, { disabled: new Set(RULE_NAMES) }).length > 0);
  for (const c of still) fail("mutate", `${c.name} is refused with every rule disabled: a refusal outside the rule table`);
}

// ---------- report ----------
if (failures.length) {
  for (const f of failures) console.log(f);
  console.log(`conformance: ${failures.length} failure(s)`);
  process.exit(1);
}
const summary = `ok conformance: ${positive.length} positive, ${negative.length} negative, ${realFiles.length} real, ${REASONS.length - 1} reasons${MUTATE ? `, ${mutated} rules mutated` : ""}`;

// ---------- STATUS.md generated block (DATUM rule 5, applied to this repo) ----------
// Rendered only from a green run; never carries a date or a hand-typed status.
if (WRITE_STATUS || CHECK_STATUS) {
  const BEGIN = "<!-- datum:status:begin -->", END = "<!-- datum:status:end -->";
  const block = [
    BEGIN,
    "generated by `node conformance/test.mjs --write-status` from a green run; CI compares with `--check-status`",
    "",
    "| pack | value |",
    "|---|---|",
    `| self-test | ${summary.replace(/^ok conformance: /, "")} |`,
    `| fixtures | ${positive.length} positive, ${negative.length} negative, ${realFiles.length} real (${[...byRepo.keys()].join(", ") || "no governed repo yet"}) |`,
    `| refusal reasons | ${REASONS.length - 1} |`,
    `| rules | ${RULE_NAMES.length}: ${RULE_NAMES.join(", ")} |`,
    END,
  ].join("\n");
  if (WRITE_STATUS) { console.log(block); process.exit(0); }
  const statusPath = join(HERE, "..", "STATUS.md");
  const text = readFileSync(statusPath, "utf8").replace(/\r\n/g, "\n");
  const a = text.indexOf(BEGIN), b = text.indexOf(END);
  if (a === -1 || b === -1 || b < a) { console.log("STATUS FAIL: markers missing or out of order in STATUS.md"); process.exit(1); }
  const current = text.slice(a, b + END.length);
  if (current !== block) {
    console.log("STATUS FAIL: generated block in STATUS.md differs from a fresh rendering");
    console.log("--- STATUS.md\n" + current + "\n--- fresh\n" + block);
    process.exit(1);
  }
  console.log("ok STATUS.md generated block is fresh");
}
console.log(summary);
