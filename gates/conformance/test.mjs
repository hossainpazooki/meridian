#!/usr/bin/env node
// The conformance pack's own controls, applied to itself. Holds three
// things over every fixture: every PASS case is accepted; every FAIL case
// is refused with a reason that starts with its expect_reason; no refusal
// is emitted that no fixture expected. With --mutate, disables each rule
// in turn and asserts a negative fixture stops being refused, so no rule
// is unfalsified. Also exercises the CLI's exit codes and the PIN check.
// Exit 0 only when everything holds.
//
// No dependencies. Node 20 or later.

import {
  readFileSync, readdirSync, mkdtempSync, writeFileSync, rmSync, cpSync,
  statSync, existsSync, unlinkSync, mkdirSync, appendFileSync, realpathSync,
} from "node:fs";
import { join, dirname, relative, sep, basename } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { spawnSync } from "node:child_process";
import { tmpdir } from "node:os";
import { createHash } from "node:crypto";
import { checkRows, RULE_NAMES, checkPinShape } from "./check.mjs";
// Exports added with per-check crediting and --expect are read through the
// namespace, so a checker that lacks one is a named test failure below, not
// an import-time crash that hides every other control.
import * as PACK from "./check.mjs";

const HERE = dirname(fileURLToPath(import.meta.url));
const FIX = join(HERE, "fixtures");
const CHECK = join(HERE, "check.mjs");
const WRITE_STATUS = process.argv.includes("--write-status");
const CHECK_STATUS = process.argv.includes("--check-status");
// the status block is derived from a full green run, mutation included
const MUTATE = process.argv.includes("--mutate") || WRITE_STATUS || CHECK_STATUS;

// A copy of this file run by this file (the controls below that plant a
// defect in a scratch copy, or build the shape a governed repo ships, and
// re-run the self-test there) is a nested run. A nested run does not start
// further copies of itself -- the parent already runs those controls, and
// letting each child start its own would never terminate. It says so on
// stdout rather than leaving them out silently.
const NESTED_ENV_KEY = "PACK_SELF_TEST_NESTED";
const NESTED = process.env[NESTED_ENV_KEY] === "1";
const NESTED_ENV = { ...process.env, [NESTED_ENV_KEY]: "1" };
if (NESTED) console.log("note: nested run: the controls that re-run the self-test in a scratch copy are run by the parent, not here");
const tail = (s, n = 40) => s.split("\n").slice(-n).join("\n");
const findSchemaSrc = () => [join(HERE, "gate-verdict.v1.json"), join(HERE, "..", "schema", "gate-verdict.v1.json")].find(existsSync);

const failures = [];
const fail = (where, why) => failures.push(`TEST FAIL ${where}: ${why}`);

// ---------- helpers ----------
// A fixture directory holds its cases directly. Every entry counts: a file
// whose name does not end in .json (in any case; an upper-case extension is
// read, not skipped), a subdirectory, or a link is refused by name -- a case
// left out silently is a control nobody runs.
function listCases(dir) {
  if (!existsSync(dir)) return [];
  const out = [];
  const byCodeUnit = (a, b) => (a.name < b.name ? -1 : a.name > b.name ? 1 : 0);
  for (const e of readdirSync(dir, { withFileTypes: true }).sort(byCodeUnit)) {
    const r = rel(join(dir, e.name));
    if (!e.isFile()) fail(r, "not a regular file: a fixture directory holds its cases directly");
    else if (!/\.json$/i.test(e.name)) fail(r, "not a case: every file in a fixture directory must be a .json case");
    else out.push(join(dir, e.name));
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
  let c, text;
  try { text = readFileSync(path, "utf8"); c = JSON.parse(text); } catch (e) { fail(rel(path), `not JSON: ${e.message}`); return null; }
  // the same key twice in one object: a parser keeps one and drops the
  // other without a word, so the case would run as something nobody wrote
  const dups = typeof PACK.duplicateKeys === "function" ? PACK.duplicateKeys(text) : [];
  if (typeof PACK.duplicateKeys !== "function") fail(rel(path), "check.mjs exports no duplicateKeys, so a duplicate key in a case cannot be refused");
  if (dups.length) { fail(rel(path), `duplicate key ${dups.map((k) => JSON.stringify(k)).join(", ")}: a case with the same key twice in one object is not unambiguous JSON`); return null; }
  if (typeof c !== "object" || c === null || Array.isArray(c)) { fail(rel(path), "case must be an object"); return null; }
  if (typeof c.case !== "string" || !c.case) fail(rel(path), "case needs a description");
  if (c.expect !== "PASS" && c.expect !== "FAIL") fail(rel(path), "expect must be PASS or FAIL");
  if (c.expect === "FAIL" && (typeof c.expect_reason !== "string" || !c.expect_reason)) fail(rel(path), "FAIL case needs expect_reason");
  if (c.expect === "PASS" && "expect_reason" in c) fail(rel(path), "PASS case must not carry expect_reason");
  if (("row" in c) === ("rows" in c)) fail(rel(path), "case needs exactly one of row / rows");
  if ("rows" in c && !Array.isArray(c.rows)) fail(rel(path), "rows must be an array");
  // expect_credit: the derived crediting (check.mjs --json's array) a PASS
  // case must produce; required on every set-level PASS case, optional on a
  // single row, forbidden on a FAIL case (a refused set derives nothing).
  if ("expect_credit" in c) {
    if (c.expect !== "PASS") fail(rel(path), "FAIL case must not carry expect_credit");
    else if (!Array.isArray(c.expect_credit)) fail(rel(path), "expect_credit must be an array");
  } else if (c.expect === "PASS" && "rows" in c) fail(rel(path), "set-level PASS case needs expect_credit");
  // expect_file: the content of an --expect file, run against the rows
  let expectCells;
  if ("expect_file" in c) {
    if (typeof PACK.parseExpect !== "function") fail(rel(path), "check.mjs exports no parseExpect, so expect_file cannot be applied");
    else {
      const parsed = PACK.parseExpect(c.expect_file);
      if (!parsed.ok) fail(rel(path), `expect_file malformed: ${parsed.problems.join("; ")}`);
      else expectCells = parsed.cells;
    }
  }
  const rows = "rows" in c ? c.rows : [c.row];
  return {
    path, name: rel(path), expect: c.expect, reason: c.expect_reason,
    expectCredit: c.expect_credit, expectCells,
    entries: rows.map((row, i) => ({ file: `${rel(path)}#${i}`, row })),
  };
}
// the options a fixture's rows are checked with: its expect block, if any
const optsFor = (c, extra = {}) => (c.expectCells ? { ...extra, expect: c.expectCells, expectFile: `${c.name}#expect` } : extra);
const sameCredit = (a, b) => JSON.stringify(a) === JSON.stringify(b);
// the fixtures directory itself holds the three fixture directories and nothing else
{
  const FIXTURE_DIRS = new Set(["positive", "negative", "real"]);
  if (existsSync(FIX)) {
    for (const e of readdirSync(FIX, { withFileTypes: true })) {
      if (!(e.isDirectory() && FIXTURE_DIRS.has(e.name))) fail(rel(join(FIX, e.name)), "not a fixture directory: only positive/, negative/ and real/ sit here");
    }
  }
}
const positive = listCases(join(FIX, "positive")).map(loadCase).filter(Boolean);
const negative = listCases(join(FIX, "negative")).map(loadCase).filter(Boolean);
if (positive.length === 0) fail("fixtures/positive", "no fixtures");
if (negative.length === 0) fail("fixtures/negative", "no fixtures");

// ---------- 1 + 2 + 3: every fixture, in process ----------
for (const c of positive) {
  const reasons = checkRows(c.entries, optsFor(c));
  for (const r of reasons) fail(c.name, `PASS case refused: ${r.reason}`);
  // per-check crediting: a PASS case's derived crediting equals its
  // expect_credit exactly (status, counts, and the unfalsified list)
  if (reasons.length === 0 && c.expectCredit !== undefined) {
    const got = PACK.credit(c.entries);
    if (!sameCredit(got, c.expectCredit)) fail(c.name, `credit ${JSON.stringify(got)}, expected ${JSON.stringify(c.expectCredit)}`);
  }
}
for (const c of negative) {
  const reasons = checkRows(c.entries, optsFor(c));
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
// Layout: SOURCE.md, and one directory per governed repo holding that
// repo's rows directly. Every regular file in a repo directory counts,
// whatever its name or the case of its extension, so a row cannot sit
// outside the binding or the live/twin count by being named LIVE.JSON; a
// file that is not a .json row (a README, say) is refused, not ignored; and
// an empty repo directory is counted as holding nothing. Anything else at
// the top, a nested directory, or a link in either place is refused too.
const REAL = join(FIX, "real");
const sourceMd = join(REAL, "SOURCE.md");
const realFiles = [];
const byRepo = new Map();
{
  const byName = (a, b) => a.name.localeCompare(b.name);
  if (existsSync(REAL)) {
    for (const e of readdirSync(REAL, { withFileTypes: true }).sort(byName)) {
      if (e.name === "SOURCE.md" && e.isFile()) continue;
      if (!e.isDirectory()) { fail(`fixtures/real/${e.name}`, "not a repo directory: only SOURCE.md and one directory per governed repo sit here"); continue; }
      byRepo.set(e.name, []);
      for (const f of readdirSync(join(REAL, e.name), { withFileTypes: true }).sort(byName)) {
        const r = `${e.name}/${f.name}`;
        if (!f.isFile()) fail(`fixtures/real/${r}`, "not a regular file: a repo directory holds its rows directly");
        else if (!/\.json$/i.test(f.name)) fail(`fixtures/real/${r}`, "not a row: every file in a repo directory must be a .json row");
        else realFiles.push(join(REAL, e.name, f.name));
      }
    }
  }
}
if (realFiles.length > 0 && !existsSync(sourceMd)) fail("fixtures/real", "rows present but no SOURCE.md binds them");
const bound = new Map();
if (existsSync(sourceMd)) {
  for (const l of readFileSync(sourceMd, "utf8").split("\n")) {
    const m = /^([0-9a-f]{64})  (\S+)$/.exec(l.trim());
    if (m) bound.set(m[2], m[1]);
  }
}
for (const p of realFiles) {
  const r = relative(REAL, p).split(sep).join("/");
  const h = sha256lf(readFileSync(p));
  if (!bound.has(r)) fail(`fixtures/real/${r}`, "not listed in SOURCE.md");
  else if (bound.get(r) !== h) fail(`fixtures/real/${r}`, `sha256 ${h} does not match SOURCE.md ${bound.get(r)}`);
  let row, text;
  try { text = readFileSync(p, "utf8"); row = JSON.parse(text); } catch (e) { fail(`fixtures/real/${r}`, `not JSON: ${e.message}`); continue; }
  const dups = typeof PACK.duplicateKeys === "function" ? PACK.duplicateKeys(text) : [];
  if (dups.length) { fail(`fixtures/real/${r}`, `duplicate key ${dups.map((k) => JSON.stringify(k)).join(", ")}: a row with the same key twice in one object is not unambiguous JSON`); continue; }
  byRepo.get(r.split("/")[0]).push({ file: `fixtures/real/${r}`, row });
}
// The pack is measured against at least one governed repo's real rows: a
// missing fixtures/real, or one holding no repo directory, is refused here
// by name rather than surfacing only through another control's setup.
if (byRepo.size === 0) fail("fixtures/real", `no governed repo directory${existsSync(REAL) ? "" : " (fixtures/real is missing)"}: the pack needs at least one governed repo's real rows`);
for (const [r] of bound) if (!existsSync(join(REAL, r))) fail("fixtures/real/SOURCE.md", `lists missing file ${r}`);
for (const [repo, entries] of byRepo) {
  // The real-row layout is one live and one twin per governed repo, not
  // every row a surface happens to have run; a directory short a cell (an
  // empty one included), or carrying a second one, is refused rather than
  // folded silently into whatever crediting the rows happen to produce.
  const liveCount = entries.filter((e) => e.row && e.row.cell === "live").length;
  const twinCount = entries.filter((e) => e.row && e.row.cell === "twin").length;
  if (liveCount !== 1 || twinCount !== 1) fail(`fixtures/real/${repo}`, `must hold exactly one live and one twin row (found ${liveCount} live, ${twinCount} twin)`);
  for (const x of checkRows(entries)) fail(`fixtures/real/${repo}`, `real row refused: ${x.file}: ${x.reason}`);
}

// ---------- test.mjs itself refuses a real-fixture dir short a cell ----------
// A vendored-shape copy of the pack (schema beside check.mjs, same shape
// the CLI/PIN exercises below build) with one governed repo's twin row
// removed must make this same check fail when test.mjs is re-run against
// the copy -- proof the control lives in the vendored pack, not only here.
// The repo is whichever governed repo directory sorts first, not one named
// here. A nested copy does not run it again: its parent already has, and a
// copy with its rows planted away would otherwise fail this control's setup
// instead of the named layout check.
if (!NESTED) {
  const countRoot = mkdtempSync(join(tmpdir(), "pack-realcount-"));
  try {
    cpSync(HERE, countRoot, { recursive: true, filter: (s) => !s.endsWith("PIN") });
    const countSchemaSrc = [join(HERE, "gate-verdict.v1.json"), join(HERE, "..", "schema", "gate-verdict.v1.json")].find(existsSync);
    if (!countSchemaSrc) fail("real-fixture count setup", "schema gate-verdict.v1.json not found beside test.mjs or under ../schema/");
    else cpSync(countSchemaSrc, join(countRoot, "gate-verdict.v1.json"));
    const countReal = join(countRoot, "fixtures", "real");
    const firstRepo = existsSync(countReal) ? readdirSync(countReal, { withFileTypes: true }).filter((e) => e.isDirectory()).map((e) => e.name).sort()[0] : undefined;
    const repoDir = firstRepo === undefined ? null : join(countReal, firstRepo);
    const twinFiles = repoDir ? readdirSync(repoDir).filter((n) => n.includes("-twin-")) : [];
    if (twinFiles.length === 0) fail("real-fixture count setup", "no governed repo's twin fixture found under fixtures/real to remove for the control");
    else {
      rmSync(join(repoDir, twinFiles[0]));
      const r5 = spawnSync(process.execPath, [join(countRoot, "test.mjs")], { encoding: "utf8", env: NESTED_ENV });
      if (r5.status === 0) fail("real-fixture count", `test.mjs passed with ${firstRepo}'s real fixtures missing its twin row`);
      if (!/must hold exactly one live and one twin/.test(r5.stdout)) fail("real-fixture count", `no matching failure reported:\n${r5.stdout}${r5.stderr}`);
    }
  } finally {
    rmSync(countRoot, { recursive: true, force: true });
  }
}

// ---------- the shape a governed repo ships: a vendored copy with a written PIN ----------
// A governed repo holds the pack under gates/conformance/ with the schema
// beside check.mjs and a PIN that check.mjs --write-pin wrote. Its CI runs
// --verify-pin alone, then the vendored self-test. Build exactly that shape
// and run exactly that order: every step must exit 0. A pack whose self-test
// passes only without its own PIN beside it is not the pack a governed repo
// ships.
if (!NESTED) {
  const shipRoot = mkdtempSync(join(tmpdir(), "pack-shipped-"));
  try {
    const shipped = join(shipRoot, "gates", "conformance");
    cpSync(HERE, shipped, { recursive: true, filter: (s) => !s.endsWith("PIN") });
    const src = findSchemaSrc();
    if (!src) fail("shipped shape setup", "schema gate-verdict.v1.json not found beside test.mjs or under ../schema/");
    else {
      cpSync(src, join(shipped, "gate-verdict.v1.json"));
      const steps = [
        ["write pin", [join(shipped, "check.mjs"), "--write-pin", "5".repeat(40)], /^ok pin written$/m],
        ["verify pin alone", [join(shipped, "check.mjs"), "--verify-pin"], /^ok pin verified$/m],
        ["self-test --mutate", [join(shipped, "test.mjs"), "--mutate"], /^ok conformance: /m],
      ];
      for (const [what, args, okRe] of steps) {
        const r = spawnSync(process.execPath, args, { encoding: "utf8", env: NESTED_ENV, cwd: shipRoot });
        if (r.status !== 0 || !okRe.test(r.stdout)) { fail(`shipped shape ${what}`, `exit ${r.status}, expected 0\n${tail(r.stdout + r.stderr)}`); break; }
      }
    }
  } finally {
    rmSync(shipRoot, { recursive: true, force: true });
  }
}

// ---------- defects planted in a scratch copy: real-row layout and wording ----------
// One vendored-shape copy carries several planted defects at once; the
// self-test re-run there must refuse each one by its own named failure.
// Real-row layout: an empty repo directory, a directory holding only a
// README, an upper-case unbound live row in its own directory, and an
// upper-case twin row that IS bound in SOURCE.md (so it is counted, not
// merely unlisted). Wording: one probe line that does not carry the name
// but does carry a local path, a rule number, a ruling id and a
// section-sign citation. The probe text is assembled from pieces so this
// file's own source never holds what it plants.
if (!NESTED) {
  const plantRoot = mkdtempSync(join(tmpdir(), "pack-planted-"));
  try {
    cpSync(HERE, plantRoot, { recursive: true, filter: (s) => !s.endsWith("PIN") });
    const src = findSchemaSrc();
    if (src) cpSync(src, join(plantRoot, "gate-verdict.v1.json"));
    const realDir = join(plantRoot, "fixtures", "real");
    const plantRepo = existsSync(realDir) ? readdirSync(realDir, { withFileTypes: true }).filter((e) => e.isDirectory()).map((e) => e.name).sort()[0] : undefined;
    const merDir = join(realDir, plantRepo === undefined ? "no-governed-repo" : plantRepo);
    const merFiles = existsSync(merDir) ? readdirSync(merDir).sort() : [];
    const liveName = merFiles.find((n) => n.includes("-live-"));
    const twinName = merFiles.find((n) => n.includes("-twin-"));
    if (!src || !liveName || !twinName) fail("planted defects setup", "schema or a governed repo's bound live/twin rows not found");
    else {
      mkdirSync(join(realDir, "emptyrepo"));
      mkdirSync(join(realDir, "docsonly"));
      writeFileSync(join(realDir, "docsonly", "README.md"), "notes, not a row\n");
      mkdirSync(join(realDir, "shouty"));
      cpSync(join(merDir, liveName), join(realDir, "shouty", "LIVE.JSON"));
      cpSync(join(merDir, twinName), join(merDir, "EXTRA-TWIN.JSON"));
      appendFileSync(join(realDir, "SOURCE.md"), `${sha256lf(readFileSync(join(merDir, "EXTRA-TWIN.JSON")))}  ${plantRepo}/EXTRA-TWIN.JSON\n`);
      const probe = ["probe: see ", "~", "/", "dev", "/secret, governing ", "rule", " 4, ruling ", "R", "3, design ", String.fromCharCode(0xa7), "4"].join("");
      appendFileSync(join(plantRoot, "reasons.md"), `\n${probe}\n`);
      // Everything below is assembled from pieces for the same reason as the
      // probe above: this file's own source must not hold what it plants.
      const nm = String.fromCharCode(100, 97, 116, 117, 109);
      const ru = ["ru", "le"].join("");
      const esc = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
      const baseRow = JSON.parse(readFileSync(join(FIX, "positive", "01-live-green.json"), "utf8")).row;
      // fixture directories hold .json cases directly: a stray file, a
      // subdirectory holding a case, an upper-case extension (read, not
      // skipped), and anything but the three fixture directories at the top
      writeFileSync(join(plantRoot, "fixtures", "positive", "notes.txt"), "not a case\n");
      mkdirSync(join(plantRoot, "fixtures", "negative", "sub"));
      writeFileSync(join(plantRoot, "fixtures", "negative", "sub", "99-hidden-case.json"), JSON.stringify({ case: "hidden by a subdirectory", expect: "FAIL", expect_reason: "schema: ", row: baseRow }));
      writeFileSync(join(plantRoot, "fixtures", "negative", "70-shouty.JSON"), JSON.stringify({ case: "upper-case extension", expect: "FAIL", expect_reason: "schema: ", row: baseRow }));
      writeFileSync(join(plantRoot, "fixtures", "stray.md"), "not a fixture directory\n");
      mkdirSync(join(plantRoot, "fixtures", "extra"));
      writeFileSync(join(plantRoot, "fixtures", "extra", "01.json"), "{}\n");
      // a case file and a real row carrying the same key twice
      writeFileSync(join(plantRoot, "fixtures", "positive", "98-dup-key.json"), `{"case":"same key twice","expect":"FAIL","expect":"PASS","row":${JSON.stringify(baseRow)}}`);
      mkdirSync(join(realDir, "dupkey"));
      const dupRow = JSON.stringify(baseRow).replace('"lane":1', '"lane":2,"lane":1');
      writeFileSync(join(realDir, "dupkey", "live.json"), dupRow);
      appendFileSync(join(realDir, "SOURCE.md"), `${sha256lf(Buffer.from(dupRow))}  dupkey/live.json\n`);
      // an authored PIN in a vendored copy, which does not verify: scanned
      writeFileSync(join(plantRoot, "PIN"), `${nm} ${"a".repeat(40)}\nsee ${ru} 7\n`);
      // file and directory names in the vendored set
      writeFileSync(join(plantRoot, `notes-${ru}-7.txt`), "clean\n");
      writeFileSync(join(plantRoot, `${nm}-copy.txt`), "clean\n");
      mkdirSync(join(plantRoot, ["R", "3-drafts"].join("")));
      writeFileSync(join(plantRoot, ["R", "3-drafts"].join(""), "readme.txt"), "clean\n");
      // accidental forms of a local path, a rule number, an id and a
      // section-sign citation, each on its own line with the name absent;
      // and the machine forms spelled in markdown prose
      const probeLines = [
        ["probe-msys ", "/c/", "Users", "/someone/", "dev"],
        ["probe-windows C:", "\\", "Users", "\\", "someone", "\\", "dev", "\\", "x"],
        ["probe-profile ", "%", "USERPROFILE", "%", "\\", "dev"],
        ["probe-hash ", ru, " #7"],
        ["probe-hyphen ", ru, "-7"],
        ["probe-spaced-id per ", "R", " 3"],
        ["probe-hyphen-id per ", "P", "-9"],
        ["probe-entity design ", "&", "sect;", "4"],
        ["probe-literal rows carry ", nm, "/gate-verdict/1"],
        ["probe-template the first line is ", nm, " $", "{sha}"],
        ["probe-regex the first line matches ", nm, " [0-9a-f]{40}"],
      ].map((xs) => xs.join(""));
      appendFileSync(join(plantRoot, "reasons.md"), probeLines.join("\n") + "\n");
      // a vendored file that is not UTF-8: UTF-16 with and without a byte-order mark
      const u16 = Buffer.from(["probe-utf16 ", "~", "/", "dev ", ru, " 7"].join(""), "utf16le");
      writeFileSync(join(plantRoot, "utf16-bom.txt"), Buffer.concat([Buffer.from([0xff, 0xfe]), u16]));
      writeFileSync(join(plantRoot, "utf16-nobom.txt"), u16);
      const rp = spawnSync(process.execPath, [join(plantRoot, "test.mjs")], { encoding: "utf8", env: NESTED_ENV });
      if (rp.status !== 1) fail("planted defects", `exit ${rp.status}, expected 1\n${tail(rp.stdout + rp.stderr)}`);
      const plantedLine = (s) => new RegExp(`^TEST FAIL ${s}`, "m");
      for (const [where, re] of [
        ["fixture layout: stray file", plantedLine("fixtures/positive/notes\\.txt: not a case")],
        ["fixture layout: subdirectory", plantedLine("fixtures/negative/sub: not a regular file")],
        ["fixture layout: upper-case extension is read", plantedLine("fixtures/negative/70-shouty\\.JSON: FAIL case accepted")],
        ["fixture layout: stray file at the top", plantedLine("fixtures/stray\\.md: not a fixture directory")],
        ["fixture layout: stray directory at the top", plantedLine("fixtures/extra: not a fixture directory")],
        ["duplicate key in a case file", plantedLine('fixtures/positive/98-dup-key\\.json: duplicate key "expect"')],
        ["duplicate key in a real row", plantedLine('fixtures/real/dupkey/live\\.json: duplicate key "lane"')],
        ["authored PIN that does not verify", plantedLine("PIN: present but does not verify, so it is scanned as authored text")],
        ["authored PIN is scanned", plantedLine(`wording: PIN:2: a ${ru} number`)],
        ["file name: a rule number", plantedLine(`wording: ${esc(`notes-${ru}-7.txt`)}: file name: a ${ru} number`)],
        ["file name: the name", plantedLine(`wording: ${esc(`${nm}-copy.txt`)}: file name: the name appears`)],
        ["directory name: an id", plantedLine(`wording: ${["R", "3-drafts"].join("")}: file name: a ruling or contract id`)],
        ["wording: MSYS user path", plantedLine("wording: reasons\\.md:\\d+: a local path: probe-msys")],
        ["wording: Windows user path", plantedLine("wording: reasons\\.md:\\d+: a local path: probe-windows")],
        ["wording: profile variable", plantedLine("wording: reasons\\.md:\\d+: a local path: probe-profile")],
        ["wording: rule and hash", plantedLine(`wording: reasons\\.md:\\d+: a ${ru} number: probe-hash`)],
        ["wording: rule and hyphen", plantedLine(`wording: reasons\\.md:\\d+: a ${ru} number: probe-hyphen `)],
        ["wording: spaced id", plantedLine("wording: reasons\\.md:\\d+: a ruling or contract id: probe-spaced-id")],
        ["wording: hyphenated id", plantedLine("wording: reasons\\.md:\\d+: a ruling or contract id: probe-hyphen-id")],
        ["wording: section entity", plantedLine("wording: reasons\\.md:\\d+: a section-sign citation: probe-entity")],
        ["markdown: schema literal", plantedLine("wording: reasons\\.md:\\d+: the name appears outside the allowed forms: probe-literal")],
        ["markdown: PIN template", plantedLine("wording: reasons\\.md:\\d+: the name appears outside the allowed forms: probe-template")],
        ["markdown: PIN regex source", plantedLine("wording: reasons\\.md:\\d+: the name appears outside the allowed forms: probe-regex")],
        ["encoding: UTF-16 with a byte-order mark", plantedLine("wording: utf16-bom\\.txt: not valid UTF-8")],
        ["encoding: UTF-16 without a byte-order mark", plantedLine("wording: utf16-nobom\\.txt:1: a control character")],
        ["real layout: empty repo directory", /^TEST FAIL fixtures\/real\/emptyrepo: must hold exactly one live and one twin row \(found 0 live, 0 twin\)/m],
        ["real layout: README-only directory, the file", /^TEST FAIL fixtures\/real\/docsonly\/README\.md: /m],
        ["real layout: README-only directory, the count", /^TEST FAIL fixtures\/real\/docsonly: must hold exactly one live and one twin row \(found 0 live, 0 twin\)/m],
        ["real layout: upper-case row is hash-bound", /^TEST FAIL fixtures\/real\/shouty\/LIVE\.JSON: not listed in SOURCE\.md/m],
        ["real layout: upper-case row is counted", /^TEST FAIL fixtures\/real\/shouty: must hold exactly one live and one twin row \(found 1 live, 0 twin\)/m],
        ["real layout: bound upper-case twin is counted", plantedLine(`fixtures/real/${esc(plantRepo)}: must hold exactly one live and one twin row \\(found 1 live, 2 twin\\)`)],
        ["wording probe: local path", /^TEST FAIL wording: reasons\.md:\d+: a local path/m],
        ["wording probe: rule number", /^TEST FAIL wording: reasons\.md:\d+: a rule number/m],
        ["wording probe: ruling id", /^TEST FAIL wording: reasons\.md:\d+: a ruling or contract id/m],
        ["wording probe: section-sign citation", /^TEST FAIL wording: reasons\.md:\d+: a section-sign citation/m],
        ["wording: commit-sha sub-check says it was not evaluable in a vendored copy", /^note: wording: the commit-sha sub-check was not evaluable here/m],
      ]) {
        if (!re.test(rp.stdout)) fail(`planted defects ${where}`, `no line matching ${re}\n${tail(rp.stdout + rp.stderr)}`);
      }
    }
  } finally {
    rmSync(plantRoot, { recursive: true, force: true });
  }
}

// ---------- defects planted in a copy in this repo's own layout ----------
// The pack's own repo layout (conformance/ beside schema/) never holds a
// PIN: one is generated only in a vendored copy, by --write-pin. A file
// named PIN here is authored, so it is refused and scanned, never left out.
// The same copy holds no governed repo's real rows at all: the pack needs at
// least one, and says so by name rather than through another control's setup.
if (!NESTED) {
  const layoutRoot = mkdtempSync(join(tmpdir(), "pack-repo-layout-"));
  try {
    const packDir = join(layoutRoot, "conformance");
    cpSync(HERE, packDir, { recursive: true, filter: (s) => !s.endsWith("PIN") && !s.endsWith("gate-verdict.v1.json") });
    const src = findSchemaSrc();
    if (!src) fail("repo layout defects setup", "schema gate-verdict.v1.json not found beside test.mjs or under ../schema/");
    else {
      mkdirSync(join(layoutRoot, "schema"));
      cpSync(src, join(layoutRoot, "schema", "gate-verdict.v1.json"));
      const realDir = join(packDir, "fixtures", "real");
      for (const e of readdirSync(realDir, { withFileTypes: true })) if (e.isDirectory()) rmSync(join(realDir, e.name), { recursive: true, force: true });
      const nm = String.fromCharCode(100, 97, 116, 117, 109);
      writeFileSync(join(packDir, "PIN"), `${nm} ${"b".repeat(40)}\n`);
      const rl = spawnSync(process.execPath, [join(packDir, "test.mjs")], { encoding: "utf8", env: NESTED_ENV, cwd: layoutRoot });
      if (rl.status !== 1) fail("repo layout defects", `exit ${rl.status}, expected 1\n${tail(rl.stdout + rl.stderr)}`);
      for (const [where, re] of [
        ["a PIN in the repo layout is refused", /^TEST FAIL PIN: a PIN file in the pack's own repo layout is refused/m],
        ["a PIN in the repo layout is scanned", /^TEST FAIL wording: PIN:1: the name appears outside the allowed forms/m],
        ["no governed repo's real rows", /^TEST FAIL fixtures\/real: no governed repo directory/m],
      ]) {
        if (!re.test(rl.stdout)) fail(`repo layout defects ${where}`, `no line matching ${re}\n${tail(rl.stdout + rl.stderr)}`);
      }
    }
  } finally {
    rmSync(layoutRoot, { recursive: true, force: true });
  }
}

// ---------- --write-pin's shape guard ----------
// The guard can never actually fire from writePin's own construction (one
// line per walked file, always in step with the count); exercise it
// directly with crafted lines/fileCount instead of weakening it.
{
  const sha = "1".repeat(40);
  const good = [`datum ${sha}`, "aa  x", "bb  y"];
  if (checkPinShape(good, 2) !== null) fail("pin shape guard", `well-shaped content refused: ${checkPinShape(good, 2)}`);
  if (checkPinShape([`nope ${sha}`, "aa  x"], 1) === null) fail("pin shape guard", "malformed first line accepted");
  if (checkPinShape(good, 5) === null) fail("pin shape guard", "line-count mismatch accepted (2 file lines against 5 claimed hashed files)");
  if (checkPinShape([], 0) === null) fail("pin shape guard", "empty content accepted");
}

// ---------- CLI exit codes ----------
function run(args, cwd) {
  const r = spawnSync(process.execPath, [CHECK, ...args], { encoding: "utf8", cwd });
  return { code: r.status, out: r.stdout, err: r.stderr };
}
const scratch = mkdtempSync(join(tmpdir(), "pack-test-"));
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
    // a row file carrying the same key twice, at the top or nested, is not
    // unambiguous JSON: unevaluable, exit 2, the same class as not JSON --
    // never judged on whichever value a parser happened to keep
    {
      // The duplicate replaces the conforming live row itself, so a parser
      // keeping the last value sees a set that conforms and would exit 0.
      const row0 = join(conf, "row-0.json");
      const row0Bytes = readFileSync(row0);
      const compact = JSON.stringify(setCase.entries[0].row);
      for (const [where, from, to, key] of [
        ["cli row duplicate top-level key", '"result":"GREEN"', '"result":"RED","result":"GREEN"', "result"],
        ["cli row duplicate nested key", '"checks":{"duplicate_absorbed":0', '"checks":{"duplicate_absorbed":1,"duplicate_absorbed":0', "duplicate_absorbed"],
      ]) {
        const text = compact.replace(from, to);
        if (text === compact) { fail(where, `setup: ${from} not found in the conforming row`); continue; }
        writeFileSync(row0, text);
        const rd = run([conf]);
        if (rd.code !== 2) fail(where, `exit ${rd.code}, expected 2 (duplicate key coerced?)\n${rd.out}${rd.err}`);
        if (/^FAIL /m.test(rd.out)) fail(where, `a FAIL line was printed for an unevaluable row file:\n${rd.out}`);
        if (!new RegExp(`^unevaluable: row-0\\.json: duplicate key "${key}"`, "m").test(rd.err)) fail(where, `stderr does not name the file and the key:\n${rd.err}`);
        writeFileSync(row0, row0Bytes);
      }
    }
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
    // a subdirectory in the rows directory is a refusable entry, named
    // and reported, not silently skipped down to fewer rows
    const subdirEntry = join(conf, "a-subdir");
    mkdirSync(subdirEntry);
    writeFileSync(join(subdirEntry, "row.json"), "{ not json\n"); // "holding a refusable row"
    r = run([conf]);
    if (r.code !== 2) fail("cli subdirectory entry", `exit ${r.code}, expected 2 (subdirectory skipped by type?)`);
    if (!/unevaluable: a-subdir is not a regular file/.test(r.out + r.err)) fail("cli subdirectory entry", `no reason naming the subdirectory:\n${r.out}${r.err}`);
    rmSync(subdirEntry, { recursive: true });
    // a junction is the same kind of entry on Windows (POSIX has none); the
    // same code path (dirent.isFile() is false for a symlink or a junction
    // alike) means a representative case on each platform is enough
    if (process.platform === "win32") {
      const { symlinkSync } = await import("node:fs");
      const junctionEntry = join(conf, "a-junction");
      symlinkSync(conf, junctionEntry, "junction");
      r = run([conf]);
      if (r.code !== 2) fail("cli junction entry", `exit ${r.code}, expected 2 (junction skipped by type?)`);
      if (!/unevaluable: a-junction is not a regular file/.test(r.out + r.err)) fail("cli junction entry", `no reason naming the junction:\n${r.out}${r.err}`);
      rmSync(junctionEntry, { recursive: true });
    } else {
      const { symlinkSync } = await import("node:fs");
      const symlinkEntry = join(conf, "a-symlink");
      symlinkSync(join(conf, "row-0.json"), symlinkEntry);
      r = run([conf]);
      if (r.code !== 2) fail("cli symlink entry", `exit ${r.code}, expected 2 (symlink skipped by type?)`);
      if (!/unevaluable: a-symlink is not a regular file/.test(r.out + r.err)) fail("cli symlink entry", `no reason naming the symlink:\n${r.out}${r.err}`);
      rmSync(symlinkEntry);
    }
    // two positionals -> 2, usage on stderr, not "first one wins"
    r = run([conf, conf]);
    if (r.code !== 2) fail("cli two positionals", `exit ${r.code}, expected 2`);
    if (!/^usage:/m.test(r.err)) fail("cli two positionals", `no usage line on stderr:\n${r.err}`);
    // an unrecognised flag (a plausible typo) -> 2, usage on stderr
    r = run([conf, "--verify-pn"]);
    if (r.code !== 2) fail("cli unknown flag", `exit ${r.code}, expected 2`);
    if (!/^usage:/m.test(r.err)) fail("cli unknown flag", `no usage line on stderr:\n${r.err}`);
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
  const vend = join(scratch, "vendor", "pack");
  cpSync(HERE, vend, { recursive: true, filter: (s) => !s.endsWith("PIN") });
  // the vendoring contract: the schema is copied beside check.mjs. In this
  // repo it lives under ../schema/; in a vendored copy it is already
  // beside this file (found 2026-09-09 when MERIDIAN first ran the vendored
  // self-test: the ../schema/ path does not exist outside this repo).
  const schemaSrc = [join(HERE, "gate-verdict.v1.json"), join(HERE, "..", "schema", "gate-verdict.v1.json")].find(existsSync);
  if (!schemaSrc) fail("pin", "schema gate-verdict.v1.json not found beside test.mjs or under ../schema/");
  else cpSync(schemaSrc, join(vend, "gate-verdict.v1.json"));
  const sha = "0".repeat(40);
  r = spawnSync(process.execPath, [join(vend, "check.mjs"), "--write-pin", sha], { encoding: "utf8" });
  if (r.status !== 0) fail("pin write", `exit ${r.status}: ${r.stderr}${r.stdout}`);
  else {
    // success: PIN is written by check.mjs itself, stdout carries
    // only the one-line confirmation and no sha256 lines
    if (r.stdout.trim() !== "ok pin written") fail("pin write", `unexpected stdout: ${JSON.stringify(r.stdout)}`);
    if (/[0-9a-f]{64}/.test(r.stdout)) fail("pin write", "stdout must not contain a sha256 line");
    if (!existsSync(join(vend, "PIN"))) fail("pin write", "PIN was not written to disk");
    const pinContent = readFileSync(join(vend, "PIN"), "utf8");
    if (!pinContent.startsWith(`datum ${sha}\n`)) fail("pin write", `PIN does not start with its own first line:\n${pinContent.slice(0, 80)}`);
    let v = spawnSync(process.execPath, [join(vend, "check.mjs"), "--verify-pin", conf], { encoding: "utf8" });
    if (v.status !== 0) fail("pin verify clean", `exit ${v.status}, expected 0\n${v.stdout}${v.stderr}`);

    // --verify-pin given alone, no rows dir: a complete, successful
    // invocation on a clean vendored tree, so CI can verify the pin before
    // running the vendored test.mjs at all
    let sv = spawnSync(process.execPath, [join(vend, "check.mjs"), "--verify-pin"], { encoding: "utf8" });
    if (sv.status !== 0) fail("pin verify standalone clean", `exit ${sv.status}, expected 0\n${sv.stdout}${sv.stderr}`);
    if (sv.stdout.trim() !== "ok pin verified") fail("pin verify standalone clean", `unexpected stdout: ${JSON.stringify(sv.stdout)}`);

    // a bad sha exits 2, leaves the pre-existing PIN byte-identical,
    // and drops no temp file in the vendored directory
    const pinBefore = readFileSync(join(vend, "PIN"));
    const entriesBefore = new Set(readdirSync(vend));
    const bad = spawnSync(process.execPath, [join(vend, "check.mjs"), "--write-pin", "not-a-sha"], { encoding: "utf8" });
    if (bad.status !== 2) fail("pin write bad sha", `exit ${bad.status}, expected 2`);
    const pinAfter = readFileSync(join(vend, "PIN"));
    if (!pinBefore.equals(pinAfter)) fail("pin write bad sha", "PIN changed by a refused --write-pin");
    const newEntries = readdirSync(vend).filter((n) => !entriesBefore.has(n));
    if (newEntries.length) fail("pin write bad sha", `left file(s) behind: ${newEntries.join(", ")}`);

    // --write-pin takes exactly one argument, its sha. Anything else beside
    // it -- a flag, a positional, a rows directory, a second --write-pin --
    // is a usage error (exit 2, usage on stderr) that writes nothing, rather
    // than being ignored while a PIN is written anyway.
    {
      const vend3 = join(scratch, "vendor3", "pack");
      cpSync(HERE, vend3, { recursive: true, filter: (s) => !s.endsWith("PIN") });
      if (schemaSrc) cpSync(schemaSrc, join(vend3, "gate-verdict.v1.json"));
      const shaW = "6".repeat(40);
      for (const args of [
        ["--write-pin", shaW, "--bogus"],
        ["somedir", "otherdir", "--bogus", "--write-pin", shaW],
        [conf, "--json", "--write-pin", shaW],
        ["--write-pin", shaW, "extra"],
        ["--write-pin", shaW, "--write-pin", shaW],
        ["--verify-pin", "--write-pin", shaW],
        ["--write-pin", shaW, "--expect", "x.json"],
        ["--write-pin"],
        // a missing value spelled as a second flag or an empty string is a
        // usage error too, not a sha that fails its own shape
        ["--write-pin", "--write-pin"],
        ["--write-pin", ""],
      ]) {
        const where = `write-pin with other arguments [${args.map((a) => (a === conf ? "<rows-dir>" : a)).join(" ")}]`;
        const rw = spawnSync(process.execPath, [join(vend3, "check.mjs"), ...args], { encoding: "utf8" });
        if (rw.status !== 2) fail(where, `exit ${rw.status}, expected 2\n${rw.stdout}${rw.stderr}`);
        if (!/^usage:/m.test(rw.stderr)) fail(where, `no usage line on stderr:\n${rw.stderr}`);
        if (existsSync(join(vend3, "PIN"))) { fail(where, "PIN was written"); rmSync(join(vend3, "PIN")); }
        const strayW = readdirSync(vend3).filter((n) => n.startsWith(".PIN.tmp."));
        if (strayW.length) { fail(where, `left temp file(s) behind: ${strayW.join(", ")}`); for (const n of strayW) rmSync(join(vend3, n)); }
      }
    }

    // A PIN that verifies lists exactly the pack's own files, each once, and
    // nothing else: a line naming a file outside the pack (reached through
    // ..) or repeating a listed file carries text no file name in the pack
    // holds, so it must not verify -- a verified PIN is left out of the
    // wording scan.
    {
      const pinPath = join(vend, "PIN");
      const pinGood = readFileSync(pinPath);
      const outside = join(vend, "..", "outside.txt");
      writeFileSync(outside, "outside the pack\n");
      for (const [where, extra, re] of [
        ["pin verify line outside the pack", `${sha256lf(readFileSync(outside))}  ../outside.txt\n`, /PIN mismatch: \.\.\/outside\.txt listed but not a file of the pack/],
        ["pin verify repeated line", `${pinGood.toString("utf8").split("\n")[1]}\n`, /PIN line repeats /],
      ]) {
        writeFileSync(pinPath, Buffer.concat([pinGood, Buffer.from(extra)]));
        const vx = spawnSync(process.execPath, [join(vend, "check.mjs"), "--verify-pin"], { encoding: "utf8" });
        if (vx.status !== 2) fail(where, `exit ${vx.status}, expected 2\n${vx.stdout}${vx.stderr}`);
        if (!re.test(vx.stderr)) fail(where, `no problem matching ${re}:\n${vx.stderr}`);
        writeFileSync(pinPath, pinGood);
      }
      rmSync(outside);
    }

    // altered file -> 2
    const target = join(vend, "reasons.md");
    writeFileSync(target, readFileSync(target, "utf8") + "\nedited locally\n");
    v = spawnSync(process.execPath, [join(vend, "check.mjs"), "--verify-pin", conf], { encoding: "utf8" });
    if (v.status !== 2) fail("pin verify altered", `exit ${v.status}, expected 2`);
    if (!/reasons\.md/.test(v.stderr + v.stdout)) fail("pin verify altered", "does not name the altered file");
    // standalone --verify-pin sees the same alteration and refuses too
    sv = spawnSync(process.execPath, [join(vend, "check.mjs"), "--verify-pin"], { encoding: "utf8" });
    if (sv.status !== 2) fail("pin verify standalone altered", `exit ${sv.status}, expected 2`);
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
    // an empty PIN -- what a shell redirect onto PIN leaves, since the shell
    // truncates the file before node starts -- is refused, exit 2
    writeFileSync(join(vend, "PIN"), "");
    v = spawnSync(process.execPath, [join(vend, "check.mjs"), "--verify-pin"], { encoding: "utf8" });
    if (v.status !== 2) fail("pin verify empty", `exit ${v.status}, expected 2\n${v.stdout}${v.stderr}`);
    // missing PIN -> 2
    rmSync(join(vend, "PIN"));
    v = spawnSync(process.execPath, [join(vend, "check.mjs"), "--verify-pin", conf], { encoding: "utf8" });
    if (v.status !== 2) fail("pin verify missing", `exit ${v.status}, expected 2`);
  }

  // ---------- --write-pin robustness: injectable rename failure, a leftover-temp refusal, and the measured Windows defect ----------
  {
    const vend2 = join(scratch, "vendor2", "pack");
    cpSync(HERE, vend2, { recursive: true, filter: (s) => !s.endsWith("PIN") });
    if (schemaSrc) cpSync(schemaSrc, join(vend2, "gate-verdict.v1.json"));
    const sha3 = "3".repeat(40);

    // Injected rename failure, in-process against the vendored copy's own
    // module (its own HERE is vend2, not this repo, from its own
    // import.meta.url): deterministic on every platform, including POSIX
    // CI where renaming over an open file ordinarily succeeds and the real
    // defect below cannot be forced.
    const vendMod = await import(pathToFileURL(join(vend2, "check.mjs")).href);
    const fakeFs = {
      writeFileSync,
      renameSync: () => { const e = new Error("simulated"); e.code = "EPERM"; throw e; },
      unlinkSync,
      readdirSync,
    };
    const before = readdirSync(vend2);
    const res = vendMod.writePinAtomic(sha3, fakeFs);
    if (res.ok) fail("write-pin injected rename failure", "expected ok:false, got ok:true");
    else if (!/EPERM/.test(res.message)) fail("write-pin injected rename failure", `message does not carry the code: ${res.message}`);
    const strayNew = readdirSync(vend2).filter((n) => !before.includes(n) && n.startsWith(".PIN.tmp."));
    if (strayNew.length) fail("write-pin injected rename failure", `left temp file(s) behind: ${strayNew.join(", ")}`);
    if (existsSync(join(vend2, "PIN"))) fail("write-pin injected rename failure", "PIN was written despite the refused rename");

    // The leftover scan itself cannot throw: a directory listing that fails
    // is a refusal carrying its code, not an uncaught exception.
    {
      let threw = null, resR;
      const failingList = () => { const e = new Error("simulated"); e.code = "EACCES"; throw e; };
      try { resR = vendMod.writePinAtomic(sha3, { writeFileSync, renameSync: () => {}, unlinkSync, readdirSync: failingList }); } catch (e) { threw = e; }
      if (threw) fail("write-pin leftover scan cannot throw", `threw ${threw.code || threw.message}`);
      else if (resR.ok || !/EACCES/.test(resR.message)) fail("write-pin leftover scan cannot throw", `expected ok:false carrying EACCES, got ${JSON.stringify(resR)}`);
      if (existsSync(join(vend2, "PIN"))) fail("write-pin leftover scan cannot throw", "PIN was written");
    }
    // Rename and the clean-up unlink both fail: the temp file stays, and
    // every later --write-pin refuses it as a leftover, so the refusal must
    // name the file left behind rather than claim nothing remains.
    {
      const beforeU = readdirSync(vend2);
      let threw = null, resU;
      const failWith = (code) => () => { const e = new Error("simulated"); e.code = code; throw e; };
      try { resU = vendMod.writePinAtomic(sha3, { writeFileSync, renameSync: failWith("EBUSY"), unlinkSync: failWith("EPERM"), readdirSync }); } catch (e) { threw = e; }
      const leftU = readdirSync(vend2).filter((n) => !beforeU.includes(n) && n.startsWith(".PIN.tmp."));
      if (threw) fail("write-pin temp left behind is named", `threw ${threw.code || threw.message}`);
      else if (resU.ok) fail("write-pin temp left behind is named", "expected ok:false, got ok:true");
      else if (leftU.length !== 1 || !resU.message.includes(leftU[0])) fail("write-pin temp left behind is named", `message ${JSON.stringify(resU.message)} does not name the file left behind (${leftU.join(", ") || "none"})`);
      for (const n of leftU) rmSync(join(vend2, n));
    }

    // A leftover .PIN.tmp.* from a prior run refuses the next one outright,
    // writing nothing, rather than racing it with this run's own temp file.
    const stale = join(vend2, ".PIN.tmp.stale.1");
    writeFileSync(stale, "leftover\n");
    const r3 = spawnSync(process.execPath, [join(vend2, "check.mjs"), "--write-pin", sha3], { encoding: "utf8" });
    if (r3.status !== 2) fail("write-pin leftover temp refusal", `exit ${r3.status}, expected 2`);
    if (existsSync(join(vend2, "PIN"))) fail("write-pin leftover temp refusal", "PIN was written despite a stale temp file present");
    if (!existsSync(stale)) fail("write-pin leftover temp refusal", "the pre-existing stale temp file was itself removed (not this run's to clean up)");
    rmSync(stale);

    // The measured Windows defect: `node check.mjs --write-pin <sha> > PIN`
    // -- the shell's redirect target IS the rename destination, so on
    // Windows the rename throws EPERM. Before the fix this was an uncaught
    // exception (exit 1, a stray temp file, a truncated empty PIN); after
    // the fix it is a clean refusal, exit 2, no leftover temp file. Real
    // reproduction is Windows-only: POSIX rename onto an open file succeeds.
    if (process.platform === "win32") {
      const sha4 = "4".repeat(40);
      const cmd = `"${process.execPath}" "${join(vend2, "check.mjs")}" --write-pin ${sha4} > PIN`;
      const r4 = spawnSync(cmd, { cwd: vend2, shell: true, encoding: "utf8" });
      if (r4.status !== 2) fail("write-pin windows redirect repro", `exit ${r4.status}, expected 2 (node check.mjs --write-pin <sha> > PIN)\n${r4.stdout}${r4.stderr}`);
      const strayAfterRedirect = readdirSync(vend2).filter((n) => n.startsWith(".PIN.tmp."));
      if (strayAfterRedirect.length) fail("write-pin windows redirect repro", `left temp file(s) behind: ${strayAfterRedirect.join(", ")}`);
    }
  }

  // ---------- check.mjs runs main() when invoked through a symlinked or junctioned path ----------
  {
    const { symlinkSync, mkdirSync: mkdirSync2 } = await import("node:fs");
    const linkDir = join(scratch, "linked-pack");
    if (process.platform === "win32") symlinkSync(HERE, linkDir, "junction");
    else symlinkSync(HERE, linkDir, "dir");
    const linkedCheck = join(linkDir, "check.mjs");
    // a refusable rows dir reached only through the link: today (unpatched)
    // main() never runs, because argv[1] (the linked path) and
    // import.meta.url (realpathed by ESM resolution) compare unequal by
    // strict string equality, so the process exits 0 with no output at all.
    const badRows = join(scratch, "linked-bad-rows");
    mkdirSync2(badRows);
    const templateRow = JSON.parse(readFileSync(join(HERE, "fixtures", "positive", "01-live-green.json"), "utf8")).row;
    writeFileSync(join(badRows, "row.json"), JSON.stringify({ ...templateRow, schema: "datum/gate-verdict/9" }) + "\n");
    const r6 = spawnSync(process.execPath, [linkedCheck, badRows], { encoding: "utf8" });
    if (r6.status !== 1) fail("linked invocation", `exit ${r6.status}, expected 1 (main() not reached through the link?)\nstdout:${r6.stdout}\nstderr:${r6.stderr}`);
    if (!/^FAIL row\.json:/m.test(r6.stdout)) fail("linked invocation", `no FAIL line through the link:\n${r6.stdout}`);
  }

  // ---------- per-check crediting through the CLI (--json and text) ----------
  {
    const { mkdirSync: mkdirSync3 } = await import("node:fs");
    const partialCase = positive.find((c) => c.name.includes("partial-one-check-unfalsified"));
    if (!partialCase) fail("crediting cli", "no partial-one-check-unfalsified fixture to build a rows directory from");
    else {
      const dir8 = join(scratch, "credit-rows");
      mkdirSync3(dir8);
      partialCase.entries.forEach((e, i) => writeFileSync(join(dir8, `row-${i}.json`), JSON.stringify(e.row, null, 2) + "\n"));
      let r8 = run([dir8, "--json"]);
      if (r8.code !== 0) fail("crediting cli --json", `exit ${r8.code}, expected 0 (crediting never changes the exit code)\n${r8.out}${r8.err}`);
      let j8; try { j8 = JSON.parse(r8.out); } catch { fail("crediting cli --json", `output is not JSON:\n${r8.out}`); }
      if (j8 && !(Array.isArray(j8) && j8.length === 1 && j8[0].status === "PARTIAL" && sameCredit(j8[0].unfalsified, ["duplicate_absorbed"]))) {
        fail("crediting cli --json", `expected one PARTIAL element with unfalsified ["duplicate_absorbed"], got: ${r8.out}`);
      }
      r8 = run([dir8]);
      if (r8.code !== 0) fail("crediting cli text", `exit ${r8.code}, expected 0\n${r8.out}${r8.err}`);
      if (!/^example-lane1-p1 lane1 PARTIAL .*unfalsified: duplicate_absorbed\b/m.test(r8.out)) fail("crediting cli text", `text output does not name the unfalsified key:\n${r8.out}`);
    }
    const claimCase = positive.find((c) => c.name.includes("claimable-twins-cover-every-check"));
    if (!claimCase) fail("crediting cli", "no claimable-twins-cover-every-check fixture to build a rows directory from");
    else {
      const dir8c = join(scratch, "credit-rows-claimable");
      mkdirSync3(dir8c);
      claimCase.entries.forEach((e, i) => writeFileSync(join(dir8c, `row-${i}.json`), JSON.stringify(e.row, null, 2) + "\n"));
      const r8c = run([dir8c, "--json"]);
      let j8c; try { j8c = JSON.parse(r8c.out); } catch { fail("crediting cli --json claimable", `output is not JSON:\n${r8c.out}`); }
      if (j8c && !(Array.isArray(j8c) && j8c.length === 1 && j8c[0].status === "CLAIMABLE" && !("unfalsified" in j8c[0]))) {
        fail("crediting cli --json claimable", `expected one CLAIMABLE element with no unfalsified key, got: ${r8c.out}`);
      }
    }
  }

  // ---------- --expect through the CLI ----------
  {
    const { mkdirSync: mkdirSync4 } = await import("node:fs");
    const setCase12 = positive.find((c) => c.name.includes("12-set-claimable-two-twins"));
    if (!setCase12) fail("expect cli", "no 12-set-claimable-two-twins fixture to build a rows directory from");
    else {
      const dir7 = join(scratch, "expect-rows");
      mkdirSync4(dir7);
      setCase12.entries.forEach((e, i) => writeFileSync(join(dir7, `row-${i}.json`), JSON.stringify(e.row, null, 2) + "\n"));
      const exp = (name, content) => { const p = join(scratch, name); writeFileSync(p, typeof content === "string" ? content : JSON.stringify(content)); return p; };
      const good = exp("expect-good.json", { cells: [{ surface: "example-lane1-p1", lane: 1, twins: ["duplicate_fill", "collision_payload"] }] });
      let r7 = run([dir7, "--expect", good]);
      if (r7.code !== 0) fail("cli expect match", `exit ${r7.code}, expected 0\n${r7.out}${r7.err}`);
      r7 = run([dir7, "--json", "--expect", good]);
      let j7; try { j7 = JSON.parse(r7.out); } catch { fail("cli expect --json", `output is not JSON:\n${r7.out}`); }
      if (j7 && !(Array.isArray(j7) && j7.length === 1 && j7[0].status === "CLAIMABLE")) fail("cli expect --json", `unexpected shape: ${r7.out}`);
      // each refusal: exit 1, a FAIL line with the named reason
      const refusals = [
        ["cli unexpected cell", { cells: [{ surface: "example-lane1-p9", lane: 1, twins: ["collision_payload", "duplicate_fill"] }] }, /^FAIL .*: unexpected cell example-lane1-p1:lane1/m],
        ["cli expected cell has no rows", { cells: [{ surface: "example-lane1-p1", lane: 1, twins: ["collision_payload", "duplicate_fill"] }, { surface: "example-lane2-p1", lane: 2, twins: [] }] }, /^FAIL .*: expected cell has no rows example-lane2-p1:lane2/m],
        ["cli twin set differs", { cells: [{ surface: "example-lane1-p1", lane: 1, twins: ["collision_payload"] }] }, /^FAIL .*: twin mutation set differs for example-lane1-p1:lane1/m],
      ];
      for (const [where, content, re] of refusals) {
        const r = run([dir7, "--expect", exp(`expect-${where.replace(/\W+/g, "-")}.json`, content)]);
        if (r.code !== 1) fail(where, `exit ${r.code}, expected 1\n${r.out}${r.err}`);
        if (!re.test(r.out)) fail(where, `no FAIL line matching ${re}:\n${r.out}${r.err}`);
      }
      // a malformed or unreadable expect file is unevaluable: exit 2, no FAIL line
      const malformed = [
        ["cli expect not JSON", exp("expect-junk.json", "{ not json\n")],
        ["cli expect missing file", join(scratch, "no-such-expect.json")],
        ["cli expect top-level array", exp("expect-array.json", [])],
        ["cli expect no cells", exp("expect-nocells.json", { cell: [] })],
        ["cli expect unknown top-level key", exp("expect-extra.json", { cells: [], note: "x" })],
        ["cli expect cell asserts a status", exp("expect-status.json", { cells: [{ surface: "example-lane1-p1", lane: 1, twins: ["collision_payload", "duplicate_fill"], status: "CLAIMABLE" }] })],
        ["cli expect lane string", exp("expect-lane.json", { cells: [{ surface: "example-lane1-p1", lane: "1", twins: [] }] })],
        ["cli expect twins missing", exp("expect-notwins.json", { cells: [{ surface: "example-lane1-p1", lane: 1 }] })],
        ["cli expect twin not string", exp("expect-twin-num.json", { cells: [{ surface: "example-lane1-p1", lane: 1, twins: [1] }] })],
        ["cli expect duplicate twin", exp("expect-dup-twin.json", { cells: [{ surface: "example-lane1-p1", lane: 1, twins: ["collision_payload", "collision_payload"] }] })],
        ["cli expect duplicate cell", exp("expect-dup-cell.json", { cells: [{ surface: "example-lane1-p1", lane: 1, twins: [] }, { surface: "example-lane1-p1", lane: 1, twins: [] }] })],
        // a duplicate JSON key: JSON.parse keeps the last one silently, so an
        // authored list of cells (or a cell's own field) would be dropped
        // without a word -- the very kind of silent loss the file exists to
        // refuse. Both are written as text; an object literal cannot hold them.
        ["cli expect duplicate top-level key", exp("expect-dup-key.json", '{"cells":[],"cells":[{"surface":"example-lane1-p1","lane":1,"twins":["collision_payload","duplicate_fill"]}]}')],
        ["cli expect duplicate key in a cell", exp("expect-dup-key-cell.json", '{"cells":[{"surface":"example-lane1-p9","surface":"example-lane1-p1","lane":1,"twins":["collision_payload","duplicate_fill"]}]}')],
      ];
      for (const [where, p] of malformed) {
        const r = run([dir7, "--expect", p]);
        if (r.code !== 2) fail(where, `exit ${r.code}, expected 2 (malformed expect file coerced?)\n${r.out}${r.err}`);
        if (/^FAIL /m.test(r.out)) fail(where, `a FAIL line was printed for an unevaluable expect file:\n${r.out}`);
        if (r.code === 2 && !/^unevaluable: expect file /m.test(r.err)) fail(where, `stderr does not name the expect file as unevaluable:\n${r.err}`);
        if (where.includes("duplicate") && where.includes("key") && !/duplicate key "(cells|surface)"/.test(r.err)) fail(where, `stderr does not name the duplicate key:\n${r.err}`);
      }
      // usage: --expect with no value, given twice, or with no rows directory
      for (const [where, args] of [
        ["cli expect no value", [dir7, "--expect"]],
        ["cli expect value is a flag", [dir7, "--expect", "--json"]],
        ["cli expect twice", [dir7, "--expect", good, "--expect", good]],
        ["cli expect without rows dir", ["--expect", good]],
      ]) {
        const r = run(args);
        if (r.code !== 2) fail(where, `exit ${r.code}, expected 2`);
        if (!/^usage:/m.test(r.err)) fail(where, `no usage line on stderr:\n${r.err}`);
      }
    }
  }
} finally {
  rmSync(scratch, { recursive: true, force: true });
}

// ---------- vendored-set wording ----------
// Across everything vendored -- this directory plus the schema file (one
// level up in this repo, beside check.mjs in a vendored copy) -- the private
// repo's own name may appear exactly once: check.mjs's header, spelled out
// deliberately as "<name>, a private governing text". Elsewhere it is
// refused unless it is one of two machine forms that stay by design, and
// then only in code or data (a .mjs or .json file), never in markdown
// prose: the schema identifier literal (the row's own "schema" field value,
// lower-case, unchanged), or the PIN's first line as code -- its
// construction or its regex source. Prose describes that line in words.
//
// The PIN. In this repo's own layout there is no generated PIN: a file named
// PIN anywhere under this directory is authored, so it is refused and
// scanned like any other file. In a vendored copy the PIN is left out of the
// scan only once check.mjs's own verifyPin accepts it, which holds it to its
// machine first line and one line per file of the pack, each listed once; a
// PIN that does not verify is refused and scanned as authored text.
//
// Independently of whether the name is on the line, every authored line,
// and every file and directory name in the vendored set, is also refused
// when it carries: a local path (home-relative with either slash, a user
// directory on Windows, MSYS or POSIX, a drive letter, a dev directory
// segment, a home or profile variable); a rule number (after a space, hash,
// hyphen, colon, dot or underscore, or in roman numerals of two letters or
// more); a ruling or contract id (an upper-case R, P or D and digits, with
// at most one space, hyphen, dot or underscore between, standing alone); a
// section-sign citation (the sign, its HTML entities, or its escapes); a
// control character or an invisible format character; or a commit sha of
// the pack's own history (a hex run of 7 to 63 characters, in any case,
// holding the first seven characters of a commit reachable in this repo,
// wherever it sits: after an underscore or a letter included; a run of 64 is
// a sha256 digest and is not read). A file that is not valid UTF-8 is
// refused outright, since no check here can read it. Not caught: a single
// roman letter after the rule word, and an abbreviated sha shorter than
// seven characters, which ordinary words and numbers match. The commit check
// needs that history. In this repo's own layout it is read with git and must
// be readable in full, or the control fails; in a vendored copy the history
// is not there to read, and the control prints that this one sub-check was
// not evaluable there instead of passing it silently.
//
// This block builds the name from character codes rather than spelling it,
// and every pattern below is written so that its own source does not match
// it, so that vendoring this very file into a public repo does not
// reintroduce what it exists to police.
const NAME = String.fromCharCode(68, 65, 84, 85, 77); // upper-case, five letters
const nameLower = NAME.toLowerCase();
const nameAnyCase = new RegExp(NAME, "i");
const REQUIRED_PHRASE = `${NAME}, a private governing text`;
const SCHEMA_ID_RE = new RegExp(`${nameLower}\\/gate-verdict\\/\\d+`, "g");
const PIN_TEMPLATE_RE = new RegExp(`${nameLower} \\$\\{[A-Za-z0-9_]+\\}`, "g");
const PIN_REGEX_SRC_RE = new RegExp(`${nameLower} \\[0-9a-f\\]\\{40\\}`, "g");
const LOCAL_PATH_RES = [
  /~[\\/]/,
  /[\\/](?:Users|home)[\\/]/i,
  /[\\/]dev(?![\w.-])/i,
  /(?<![\w])[A-Za-z]:[\\/]/,
  new RegExp(["%(?:USERPROFILE|HOMEPATH|HOMEDRIVE|LOCALAPPDATA|APPDATA)", "%"].join(""), "i"),
  new RegExp(["\\$", "\\{?HOME\\b|\\$", "env:(?:USERPROFILE|HOME)\\b"].join(""), "i"),
];
// the word in any case, as before; roman numerals upper-case only, so that
// ordinary words spelled with those letters are not read as numerals
const RULE_NUMBER_RE = /(?<![A-Za-z])[Rr][Uu][Ll][Ee][Ss]?[\s#:_.-]*(?:[Nn][Oo]\.?\s*)?(?:\d|[IVXLC]{2,}(?![A-Za-z]))/;
const RULING_ID_RE = /(?<![\w-])[RPD][ _.-]?\d+[a-z]?(?!\w)/;
const SECTION_SIGN_RE = new RegExp([
  String.fromCharCode(0xa7),
  ["&", "sect;"].join(""),
  ["&", "#0*167;"].join(""),
  ["&", "#x0*a7;"].join(""),
  ["\\\\", "u\\{?0*a7\\}?"].join(""),
  ["\\\\", "x", "a7"].join(""),
  ["%C2", "%A7"].join(""),
].join("|"), "i");
// Built from code points, so this file's own bytes stay printable ASCII.
const cc = (...codes) => String.fromCharCode(...codes);
const CONTROL_CHAR_RE = new RegExp(`[${cc(0)}-${cc(8)}${cc(11)}${cc(12)}${cc(14)}-${cc(31)}${cc(127)}]`);
const INVISIBLE_RE = new RegExp(`[${cc(0x200b)}-${cc(0x200f)}${cc(0x202a)}-${cc(0x202e)}${cc(0x2060)}${cc(0x2066)}-${cc(0x2069)}${cc(0xfeff)}]`);
const HEX_RUN_RE = /[0-9a-f]+/gi;
const UTF8_FATAL = new TextDecoder("utf-8", { fatal: true });
function walkAll(dir, files = [], dirs = []) {
  if (!existsSync(dir)) return { files, dirs };
  for (const e of readdirSync(dir, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
    const p = join(dir, e.name);
    if (e.isDirectory()) { dirs.push(p); walkAll(p, files, dirs); } else files.push(p);
  }
  return { files, dirs };
}
// Which layout this is: the pack's own repo (this directory beside schema/)
// or a vendored copy (the schema beside check.mjs). What hangs on it -- a PIN
// refused outright, and the commit-sha sub-check read against the real
// history instead of printed as a note -- is lenient only in a vendored copy,
// so the decision reads only facts that planting files cannot turn toward
// vendored: the schema one level up under schema/; or, with that file moved
// away, git reporting a work tree whose top holds this directory, with
// check.mjs here, and whose index tracks schema/gate-verdict.v1.json. A
// schema copy beside check.mjs is not consulted; in this repo's layout it is
// refused by name below. A planted file can only turn a vendored copy toward
// this layout, where everything is stricter.
// The pack is never its own repository. A .git entry inside the pack
// directory is refused in either layout: git run from this directory would
// stop at it instead of the enclosing repository, and no vendored copy
// carries one.
if (existsSync(join(HERE, ".git"))) fail(".git", "a .git entry inside the pack directory is refused: the pack is never its own repository, and git run from here would stop at it");
function isRepoLayout() {
  if (existsSync(join(HERE, "..", "schema", "gate-verdict.v1.json"))) return true;
  // git is asked from the parent directory, so a .git planted in this
  // directory cannot stand in for the enclosing repository.
  const top = spawnSync("git", ["-C", join(HERE, ".."), "rev-parse", "--show-toplevel"], { encoding: "utf8" });
  const t = top.status === 0 ? top.stdout.trim() : "";
  if (!t) return false;
  try {
    if (realpathSync.native(join(t, basename(HERE), "check.mjs")) !== realpathSync.native(CHECK)) return false;
  } catch {
    return false;
  }
  return spawnSync("git", ["-C", t, "ls-files", "--error-unmatch", "--", "schema/gate-verdict.v1.json"], { encoding: "utf8" }).status === 0;
}
// The pack's own commit history, when this is the pack's own repo layout.
function packHistory() {
  const root = join(HERE, "..");
  const repoLayout = isRepoLayout();
  if (!repoLayout) return { evaluable: false, repoLayout, why: "a vendored copy: the pack's own history is not reachable from here" };
  const git = (...args) => spawnSync("git", ["-C", root, ...args], { encoding: "utf8" });
  const top = git("rev-parse", "--show-toplevel");
  if (top.status !== 0) return { evaluable: false, repoLayout, why: `git cannot read this repo (${String(top.stderr || top.error || "").trim()})` };
  let same = false;
  try { same = realpathSync.native(top.stdout.trim()) === realpathSync.native(root); } catch { same = false; }
  if (!same) return { evaluable: false, repoLayout, why: `the enclosing git repository is not this one (${top.stdout.trim()})` };
  const shallow = git("rev-parse", "--is-shallow-repository");
  if (shallow.status !== 0 || shallow.stdout.trim() !== "false") return { evaluable: false, repoLayout, why: "a shallow clone: the full history is needed" };
  const list = git("rev-list", "--all");
  const shas = list.status === 0 ? list.stdout.split("\n").map((s) => s.trim()).filter((s) => /^[0-9a-f]{40}$/.test(s)) : [];
  if (shas.length === 0) return { evaluable: false, repoLayout, why: "no commit could be listed" };
  return { evaluable: true, repoLayout, shas };
}
const HISTORY = packHistory();
if (!HISTORY.evaluable) {
  if (HISTORY.repoLayout) fail("wording", `the commit-sha sub-check cannot be evaluated in this repo's own layout: ${HISTORY.why}`);
  else console.log(`note: wording: the commit-sha sub-check was not evaluable here (${HISTORY.why})`);
}
const SCHEMA_FILE = join(HERE, "..", "schema", "gate-verdict.v1.json");
const VENDORED = walkAll(HERE);
let pinLeftOut = false;
if (HISTORY.repoLayout) {
  for (const p of VENDORED.files) {
    if (/^pin$/i.test(basename(p))) fail(rel(p), "a PIN file in the pack's own repo layout is refused: a PIN is generated only in a vendored copy, so this one is authored, and it is scanned as authored text");
    // check.mjs reads a schema beside itself before the one under schema/,
    // so a copy here would silently stand in for the real schema
    if (!rel(p).includes("/") && /^gate-verdict\.v1\.json$/i.test(basename(p))) fail(rel(p), "a schema copy beside check.mjs in the pack's own repo layout is refused: here the schema is schema/gate-verdict.v1.json one level up, check.mjs would read this copy first, and a copy here does not make this layout a vendored one");
  }
} else if (existsSync(join(HERE, "PIN"))) {
  const problems = typeof PACK.verifyPin === "function" ? PACK.verifyPin() : ["check.mjs exports no verifyPin"];
  if (problems.length === 0) pinLeftOut = true;
  else fail("PIN", `present but does not verify, so it is scanned as authored text (${problems.length} problem(s); first: ${problems[0]})`);
}
const VENDORED_FILES = [...VENDORED.files, SCHEMA_FILE].filter(existsSync).filter((p) => !(pinLeftOut && rel(p) === "PIN"));
{
  let phraseHits = 0;
  const phraseFiles = [];
  for (const p of VENDORED_FILES) {
    const text = readFileSync(p, "utf8");
    const n = text.split(REQUIRED_PHRASE).length - 1;
    if (n > 0) { phraseHits += n; phraseFiles.push(rel(p)); }
  }
  if (phraseHits !== 1) {
    fail("wording", `the required header phrase appears ${phraseHits} time(s) across the vendored set (${phraseFiles.join(", ") || "nowhere"}), expected exactly 1`);
  } else if (phraseFiles[0] !== rel(join(HERE, "check.mjs"))) {
    fail("wording", `the required header phrase appears in ${phraseFiles[0]}, expected conformance/check.mjs`);
  }
  for (const p of VENDORED_FILES) {
    const bytes = readFileSync(p);
    let text;
    try { text = UTF8_FATAL.decode(bytes); }
    catch { fail("wording", `${rel(p)}: not valid UTF-8, so no wording check can read it`); text = bytes.toString("utf8"); }
    for (const problem of wordingProblems(rel(p), text)) fail("wording", problem);
  }
  // file and directory names travel with the bytes (a PIN lists every file
  // name), so they carry the same refusals as a line of text
  for (const p of [...VENDORED.files, ...VENDORED.dirs]) {
    const r = rel(p);
    for (const problem of lineProblems(`${r}: file name`, r)) fail("wording", problem);
  }
}
// one file's text -> the wording problems in it, as "<file>:<line>: <what>";
// history is packHistory()'s result, or a synthetic one for the probes. The
// machine forms are allowed only in a .mjs or .json file. A leading
// byte-order mark is not text; one anywhere else is refused as invisible.
function wordingProblems(name, raw, history = HISTORY) {
  const out = [];
  let stripped = (raw.charCodeAt(0) === 0xfeff ? raw.slice(1) : raw).split(REQUIRED_PHRASE).join("");
  if (/\.(mjs|json)$/.test(name)) stripped = stripped.replace(SCHEMA_ID_RE, "").replace(PIN_TEMPLATE_RE, "").replace(PIN_REGEX_SRC_RE, "");
  stripped.split("\n").forEach((line, i) => out.push(...lineProblems(`${name}:${i + 1}`, line, history)));
  return out;
}
// one line, or one file name -> its problems, each as "<at>: <what>: <line>"
function lineProblems(at, line, history = HISTORY) {
  const out = [];
  const shown = CONTROL_CHAR_RE.test(line) || INVISIBLE_RE.test(line) ? JSON.stringify(line.trim()).slice(1, -1).replace(/[^\x20-\x7e]/g, (ch) => `\\u${ch.charCodeAt(0).toString(16).padStart(4, "0")}`) : line.trim();
  if (nameAnyCase.test(line)) out.push(`${at}: the name appears outside the allowed forms: ${shown}`);
  if (LOCAL_PATH_RES.some((re) => re.test(line))) out.push(`${at}: a local path: ${shown}`);
  if (RULE_NUMBER_RE.test(line)) out.push(`${at}: a rule number: ${shown}`);
  if (RULING_ID_RE.test(line)) out.push(`${at}: a ruling or contract id: ${shown}`);
  if (SECTION_SIGN_RE.test(line)) out.push(`${at}: a section-sign citation: ${shown}`);
  if (CONTROL_CHAR_RE.test(line)) out.push(`${at}: a control character: ${shown}`);
  if (INVISIBLE_RE.test(line)) out.push(`${at}: an invisible format character: ${shown}`);
  if (history && history.evaluable) {
    for (const m of line.matchAll(HEX_RUN_RE)) {
      const run = m[0].toLowerCase();
      if (run.length < 7 || run.length >= 64) continue;
      if (history.shas.some((s) => run.includes(s.slice(0, 7)))) { out.push(`${at}: a commit sha of the pack's own history: ${run}`); break; }
    }
  }
  return out;
}
// Probes, in process: each line below leaves the name out, so only the
// sub-checks that do not depend on the name can catch it. The pack's own
// history is given here as a synthetic one-commit list, so the commit-sha
// sub-check is exercised the same way in every layout.
{
  const probeSha = "1234567890abcdef1234567890abcdef12345678";
  const probeSha2 = "abcdef1234567890abcdef1234567890abcdef12";
  const probeHistory = { evaluable: true, shas: [probeSha, probeSha2] };
  const piece = (...xs) => xs.join("");
  for (const [where, text, re] of [
    ["local path, home-relative", piece("see ", "~", "/", "dev/secret"), /: a local path/],
    ["local path, device", piece("at ", "/", "dev", "/", "sda"), /: a local path/],
    ["rule number", piece("governing ", "rule", " 4"), /: a rule number/],
    ["rule number, plural", piece("see ", "Rules", " 7 and 8"), /: a rule number/],
    ["ruling id", piece("per ", "R", "3"), /: a ruling or contract id/],
    ["contract id", piece("per ", "P", "10"), /: a ruling or contract id/],
    ["section-sign citation", piece("design ", String.fromCharCode(0xa7), "4"), /: a section-sign citation/],
    ["own commit sha, full", `at ${probeSha}`, /: a commit sha of the pack's own history/],
    ["own commit sha, abbreviated", `at ${probeSha.slice(0, 7)}`, /: a commit sha of the pack's own history/],
    ["all at once, the name absent", piece("see ", "~", "/", "dev/secret at ", probeSha, ", governing ", "rule", " 8"), /: a local path[\s\S]*: a rule number[\s\S]*: a commit sha of the pack's own history|: a local path/],
    ["local path, MSYS user directory with no trailing slash", piece("in ", "/c/", "Users", "/someone/", "dev"), /: a local path/],
    ["local path, Windows user directory", piece("in C:", "\\", "Users", "\\", "someone", "\\", "dev", "\\", "x"), /: a local path/],
    ["local path, Windows user directory escaped", piece("in C:", "\\\\", "Users", "\\\\", "someone"), /: a local path/],
    ["local path, profile variable", piece("in ", "%", "USERPROFILE", "%", "\\", "dev"), /: a local path/],
    ["local path, home variable", piece("in ", "$", "HOME", "/", "dev"), /: a local path/],
    ["local path, home-relative with a backslash", piece("in ", "~", "\\", "dev"), /: a local path/],
    ["rule number, hash", piece("see ", "rule", " #7"), /: a rule number/],
    ["rule number, hyphen", piece("see ", "rule", "-7"), /: a rule number/],
    ["rule number, underscore", piece("see my_", "rule", "_7"), /: a rule number/],
    ["rule number, roman", piece("see ", "Rule", " VII"), /: a rule number/],
    ["rule number, mixed case", piece("see ", "RuLeS", " 4"), /: a rule number/],
    ["rule number, no space", piece("see ", "rule", "4"), /: a rule number/],
    ["ruling id, spaced", piece("per ", "R", " 3"), /: a ruling or contract id/],
    ["contract id, hyphenated", piece("per ", "P", "-9"), /: a ruling or contract id/],
    ["section sign, named entity", piece("design ", "&", "sect;", "4"), /: a section-sign citation/],
    ["section sign, numeric entity", piece("design ", "&", "#167;", "4"), /: a section-sign citation/],
    ["section sign, escape", piece("design ", "\\", "u00a7", "4"), /: a section-sign citation/],
    ["own commit sha after an underscore", `pinned_${probeSha.slice(0, 7)}`, /: a commit sha of the pack's own history/],
    ["own commit sha after a letter", `built${probeSha.slice(0, 7)}`, /: a commit sha of the pack's own history/],
    ["own commit sha between hex letters", `deadbeef${probeSha.slice(0, 7)}abc`, /: a commit sha of the pack's own history/],
    ["own commit sha, upper-case, after a letter", `built${probeSha2.slice(0, 9).toUpperCase()}`, /: a commit sha of the pack's own history/],
  ]) {
    let got;
    try { got = wordingProblems("probe", text, probeHistory); } catch (e) { fail(`wording probe ${where}`, `threw ${e.message}`); continue; }
    if (!got.some((g) => re.test(g))) fail(`wording probe ${where}`, `not caught: got ${JSON.stringify(got)}`);
  }
  // the three sub-checks the combined line carries are each reported
  const combined = wordingProblems("probe", piece("see ", "~", "/", "dev/secret at ", probeSha, ", governing ", "rule", " 8"), probeHistory);
  for (const what of ["a local path", "a rule number", "a commit sha of the pack's own history"]) {
    if (!combined.some((g) => g.includes(`: ${what}`))) fail("wording probe combined line", `"${what}" not reported: ${JSON.stringify(combined)}`);
  }
  // and nothing is invented: a surface name with an upper-case segment, a
  // hex run that is not in the history, and a sha when history is not
  // evaluable are not problems
  for (const [where, text, hist] of [
    ["surface with an upper-case segment", "\"surface\": \"Example-P1\"", probeHistory],
    ["hex run outside the history", "gate_sha a087ab2c60981f166df6161ead505907e529bf44", probeHistory],
    ["sha when history is not evaluable", `at ${probeSha}`, { evaluable: false, why: "probe" }],
    ["a sha256 digest that happens to hold a commit prefix", `sha256:${"0".repeat(20)}${probeSha.slice(0, 7)}${"0".repeat(37)}`, probeHistory],
    ["a word ending in the rule word, then a digit", ["over", "rule", " 7 times"].join(""), probeHistory],
    ["a web address", "https://example.com/x", probeHistory],
    ["a path segment that only starts like the device directory", ["gates", "/", "devtools"].join(""), probeHistory],
    ["the rule word followed by a pronoun", ["the ", "rules", " I wrote"].join(""), probeHistory],
    ["a rule-table identifier", "RULE_NAMES 16", probeHistory],
  ]) {
    let got;
    try { got = wordingProblems("probe", text, hist); } catch (e) { fail(`wording probe no false positive: ${where}`, `threw ${e.message}`); continue; }
    if (got.length) fail(`wording probe no false positive: ${where}`, `reported ${JSON.stringify(got)}`);
  }
}

// In this repo's own layout the history is real. HEAD must be among the
// listed commits, and an abbreviated HEAD sha in a probe line must be
// caught against it -- so the sub-check that ran over the vendored set was
// looking at a history that holds this very commit, not an empty list.
if (HISTORY.evaluable) {
  const head = spawnSync("git", ["-C", join(HERE, ".."), "rev-parse", "HEAD"], { encoding: "utf8" });
  const headSha = head.status === 0 ? head.stdout.trim() : "";
  if (!HISTORY.shas.includes(headSha)) fail("wording history", `HEAD ${headSha || "(unreadable)"} is not among the ${HISTORY.shas.length} listed commits`);
  else if (!wordingProblems("probe", `built at ${headSha.slice(0, 7)}`).some((g) => g.includes(": a commit sha of the pack's own history"))) {
    fail("wording history", "an abbreviated HEAD sha in a probe line was not caught against the real history");
  } else if (!wordingProblems("probe", `pinned_${headSha.slice(0, 7)}`).some((g) => g.includes(": a commit sha of the pack's own history"))) {
    fail("wording history", "an abbreviated HEAD sha after an underscore was not caught against the real history");
  }
}

// ---------- files planted in a full-history copy cannot flip the layout ----------
// Two things above hang on the layout: in the pack's own repo a PIN is
// refused outright and the commit-sha sub-check reads the real history; in a
// vendored copy the PIN is left out once it verifies and that sub-check is a
// note. The vendored reading is the lenient one, so no planted file may turn
// this repo toward it. In a full-history copy of this repo, plant at once: a
// schema beside check.mjs, HEAD's abbreviated sha in a vendored file, and a
// PIN that --write-pin wrote with HEAD's sha (so it verifies). Three times:
// a stray copy with schema/ left in place, the schema moved there out of
// schema/, and that move with a .git planted in this directory as well. The
// self-test re-run in each copy must refuse every plant by name.
// And the other direction, so the layout is not read too eagerly: a vendored
// copy placed under gates/ inside that same git work tree, with its own PIN,
// is still a vendored copy, exit 0 with the note. The copies need this repo's
// full history, so they run only where that history was read; a vendored
// copy has no repo of the pack's own to copy, and says so.
if (!NESTED && HISTORY.evaluable) {
  const headSha = (spawnSync("git", ["-C", join(HERE, ".."), "rev-parse", "HEAD"], { encoding: "utf8" }).stdout || "").trim();
  const sha7 = headSha.slice(0, 7);
  const vendoredNote = /^note: wording: the commit-sha sub-check was not evaluable here/m;
  for (const [variant, move, plantGit] of [
    ["stray schema copy", false, false],
    ["schema moved beside check.mjs", true, false],
    ["schema moved beside check.mjs, .git planted in the pack", true, true],
  ]) {
    const where = `layout plant, ${variant}`;
    const flipRoot = mkdtempSync(join(tmpdir(), "pack-layout-plant-"));
    try {
      const repo = join(flipRoot, "repo");
      cpSync(join(HERE, ".."), repo, { recursive: true });
      const pack = join(repo, basename(HERE));
      const schemaTop = join(repo, "schema", "gate-verdict.v1.json");
      cpSync(schemaTop, join(pack, "gate-verdict.v1.json"));
      if (move) rmSync(schemaTop);
      // A .git planted in the pack directory stops any git command run from
      // there at the planted entry instead of the enclosing repository.
      if (plantGit) appendFileSync(join(pack, ".git"), "gitdir: nowhere\n");
      appendFileSync(join(pack, "reasons.md"), `\nbuilt at ${sha7}\n`);
      const wp = spawnSync(process.execPath, [join(pack, "check.mjs"), "--write-pin", headSha], { encoding: "utf8" });
      const vp = spawnSync(process.execPath, [join(pack, "check.mjs"), "--verify-pin"], { encoding: "utf8" });
      if (wp.status !== 0 || vp.status !== 0) { fail(`${where} setup`, `--write-pin exit ${wp.status}, --verify-pin exit ${vp.status}, expected 0 and 0 (the planted PIN must verify)\n${wp.stdout}${wp.stderr}${vp.stdout}${vp.stderr}`); continue; }
      const rf = spawnSync(process.execPath, [join(pack, "test.mjs")], { encoding: "utf8", env: NESTED_ENV, cwd: repo });
      if (rf.status !== 1) fail(where, `exit ${rf.status}, expected 1\n${tail(rf.stdout + rf.stderr)}`);
      const expected = [
        ["the schema beside check.mjs is refused", /^TEST FAIL gate-verdict\.v1\.json: a schema copy beside check\.mjs in the pack's own repo layout is refused/m],
        ["the PIN is refused", /^TEST FAIL PIN: a PIN file in the pack's own repo layout is refused/m],
        ["HEAD's sha in a vendored file is caught", new RegExp(`^TEST FAIL wording: reasons\\.md:\\d+: a commit sha of the pack's own history: ${sha7}$`, "m")],
      ];
      if (plantGit) expected.push(["the .git inside the pack is refused", /^TEST FAIL \.git: a \.git entry inside the pack directory is refused/m]);
      for (const [what, re] of expected) {
        if (!re.test(rf.stdout)) fail(`${where}: ${what}`, `no line matching ${re}\n${tail(rf.stdout + rf.stderr)}`);
      }
      if (vendoredNote.test(rf.stdout)) fail(`${where}: read as a vendored copy`, `the commit-sha sub-check printed its not-evaluable note\n${tail(rf.stdout + rf.stderr)}`);
      if (!move) {
        const gated = join(repo, "gates", basename(HERE));
        cpSync(HERE, gated, { recursive: true, filter: (s) => !s.endsWith("PIN") && !s.endsWith("gate-verdict.v1.json") });
        cpSync(schemaTop, join(gated, "gate-verdict.v1.json"));
        const gp = spawnSync(process.execPath, [join(gated, "check.mjs"), "--write-pin", "8".repeat(40)], { encoding: "utf8" });
        const rg = gp.status === 0 ? spawnSync(process.execPath, [join(gated, "test.mjs")], { encoding: "utf8", env: NESTED_ENV, cwd: repo }) : gp;
        const gw = "layout plant, a vendored copy inside a git work tree";
        if (rg.status !== 0) fail(gw, `exit ${rg.status}, expected 0 (read as the pack's own repo?)\n${tail(rg.stdout + rg.stderr)}`);
        else if (!vendoredNote.test(rg.stdout)) fail(gw, `no not-evaluable note: the commit-sha sub-check did not read it as a vendored copy\n${tail(rg.stdout + rg.stderr)}`);
      }
    } finally {
      rmSync(flipRoot, { recursive: true, force: true });
    }
  }
} else if (!NESTED && !HISTORY.repoLayout) {
  console.log("note: layout: the control that plants files in a full-history copy was not run here (a vendored copy: there is no repo of the pack's own to copy)");
}

// ---------- --mutate: every rule has a fixture that detects it disabled ----------
let mutated = 0;
if (MUTATE) {
  if (!Array.isArray(RULE_NAMES) || RULE_NAMES.length === 0) fail("mutate", "check.mjs exports no RULE_NAMES");
  for (const name of RULE_NAMES || []) {
    const flipped = negative.filter((c) => checkRows(c.entries, optsFor(c, { disabled: new Set([name]) })).length === 0);
    if (flipped.length === 0) fail("mutate", `rule "${name}" disabled and every negative fixture is still refused: the rule is unfalsified`);
    mutated++;
  }
  // and the rule table is the whole checker: disabling everything accepts every negative fixture
  const still = negative.filter((c) => checkRows(c.entries, optsFor(c, { disabled: new Set(RULE_NAMES) })).length > 0);
  for (const c of still) fail("mutate", `${c.name} is refused with every rule disabled: a refusal outside the rule table`);
}
// Crediting rules are not refusals, so a negative fixture cannot notice one
// disabled. Their mutation unit is the positive set fixture's expect_credit:
// each crediting rule disabled in turn must change the derived crediting of
// at least one PASS case, or the rule is unfalsified. With every crediting
// rule disabled, crediting falls back to cell presence alone.
let creditMutated = 0;
if (MUTATE) {
  const creditNames = PACK.CREDIT_RULE_NAMES;
  if (!Array.isArray(creditNames) || creditNames.length === 0) fail("mutate", "check.mjs exports no CREDIT_RULE_NAMES");
  else {
    for (const name of creditNames) {
      const noticed = positive.filter((c) => c.expectCredit !== undefined && !sameCredit(PACK.credit(c.entries, { disabled: new Set([name]) }), c.expectCredit));
      if (noticed.length === 0) fail("mutate", `crediting rule "${name}" disabled and every PASS case still derives its expect_credit: the rule is unfalsified`);
      creditMutated++;
    }
  }
}

// ---------- report ----------
if (failures.length) {
  for (const f of failures) console.log(f);
  console.log(`conformance: ${failures.length} failure(s)`);
  process.exit(1);
}
const summary = `ok conformance: ${positive.length} positive, ${negative.length} negative, ${realFiles.length} real, ${REASONS.length - 1} reasons${MUTATE ? `, ${mutated} rules mutated, ${creditMutated} crediting rule${creditMutated === 1 ? "" : "s"} mutated` : ""}`;

// ---------- STATUS.md generated block (this repo's own review discipline, applied to itself) ----------
// Rendered only from a green run; never carries a date or a hand-typed status.
if (WRITE_STATUS || CHECK_STATUS) {
  const BEGIN = "<!-- pack:status:begin -->", END = "<!-- pack:status:end -->";
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
    `| crediting rules | ${(PACK.CREDIT_RULE_NAMES || []).length}: ${(PACK.CREDIT_RULE_NAMES || []).join(", ")} |`,
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
