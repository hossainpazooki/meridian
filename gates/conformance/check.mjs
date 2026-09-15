#!/usr/bin/env node
// DATUM, a private governing text. This is its conformance pack: a
// reference checker for GATE_VERDICT rows.
//
//   node check.mjs <rows-dir> [--json] [--verify-pin] [--expect <file>]
//   node check.mjs --verify-pin
//   node check.mjs --write-pin <sha>
//
// Exactly one positional argument is accepted with the first form; an
// extra positional, or any flag outside --json / --verify-pin / --expect,
// is a usage error to stderr, exit 2. --write-pin is a form of its own: it
// takes exactly its sha, and any other argument or flag beside it is a
// usage error that writes nothing. Every entry of the rows
// directory must be a regular file -- a subdirectory, a symlink, or a
// junction exits 2 naming it (`unevaluable: <name> is not a regular file`),
// not skipped.
//
// --expect <file> names the (surface, lane) cells the rows must hold and
// each cell's exact twin mutations; an unexpected cell, an expected cell
// with no rows, or a differing twin set is refused (exit 1). A malformed or
// unreadable expect file exits 2 -- a duplicate JSON key in it included,
// since parsing would otherwise keep the last one and drop the rest
// without a word. It never asserts a status.
//
// Exit 0: every row conforms and the set credits; the claimability per
// (surface, lane) is printed (or emitted as JSON with --json). Crediting is
// per check: a group whose live row reports a check no twin RED as planted
// sets nonzero is PARTIAL, never CLAIMABLE, and its `unfalsified` checks
// are named; crediting never changes the exit code. Given alone
// (no rows dir), --verify-pin prints `ok pin verified` on a clean vendored
// tree, so CI can verify the pin before running the vendored test.mjs.
// Exit 1: one or more refusals, each as `FAIL <file>: <reason>` with a
// reason from reasons.md.
// Exit 2: unevaluable -- no directory, empty directory, a non-regular-file
// entry, a file that is not JSON, a file carrying the same key twice in one
// object at any depth (not unambiguous JSON: a parser would keep one value
// and drop the other without a word), a usage error, or (with --verify-pin)
// a vendored file missing, unlisted or altered, or a PIN line naming
// anything but one of the pack's own files once. Exit 2 is never coerced
// into 0 or 1.
//
// --write-pin writes the file `PIN` beside this file itself, atomically
// (temp file alongside, then rename), and prints only `ok pin written` --
// never the pin body, so there is nothing to redirect. A redirect onto
// `PIN` is still harmful, and nothing here can prevent it: the shell
// truncates `PIN` before node starts, so the file ends empty whatever this
// program does (--verify-pin then refuses the empty file). It refuses
// (exit 2) leaving a pre-existing PIN byte-unchanged -- short of that
// redirect -- when: the sha argument is not 40-hex; the content fails its
// own shape guard (first line the pack name and the sha, lines = hashed
// files + 1); a `.PIN.tmp.*` file is already present (a prior run's
// leftover); the directory cannot be listed; or the write or the rename
// itself fails (measured on Windows: a shell redirect onto `PIN` holds the
// destination open, so the rename throws EPERM). It removes its own temp
// file on failure, and names it when even that removal fails -- see
// reasons.md. File operations go through an injectable fs-like object
// (writePinAtomic's second argument) so these failure paths can be forced
// deterministically on any platform, POSIX included.
//
// The refusal tables below (row, set and expected-set rules) are the whole
// of what can refuse: test.mjs --mutate disables each entry in turn and
// requires a negative fixture to stop being refused. The crediting table
// never refuses, so no negative fixture can notice it disabled; --mutate
// disables each crediting rule in turn and requires a positive fixture's
// expect_credit to change instead.
// No dependencies. Node 20 or later.

import { readFileSync, readdirSync, existsSync, statSync, writeFileSync, renameSync, unlinkSync, realpathSync } from "node:fs";
import { join, dirname, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { createHash } from "node:crypto";

const HERE = dirname(fileURLToPath(import.meta.url));
const STATUS_LITERAL = /\b(CLAIMABLE|PARTIAL|UNCLAIMED)\b/;

// ---------------------------------------------------------------- schema
// The schema file is the specification; this is the subset validator that
// implements it. Vendored packs carry a copy of the schema beside check.mjs;
// in this repo it lives one level up under schema/.
function loadSchema() {
  for (const p of [join(HERE, "gate-verdict.v1.json"), join(HERE, "..", "schema", "gate-verdict.v1.json")]) {
    if (existsSync(p)) return JSON.parse(readFileSync(p, "utf8"));
  }
  throw new Error("schema gate-verdict.v1.json not found beside check.mjs or under ../schema/");
}
const SCHEMA = loadSchema();
const KNOWN_KEYWORDS = new Set(["$comment", "type", "enum", "pattern", "required", "properties", "additionalProperties", "minimum", "minProperties"]);

const isObject = (v) => typeof v === "object" && v !== null && !Array.isArray(v);
function typeOk(t, v) {
  if (t === "object") return isObject(v);
  if (t === "integer") return Number.isInteger(v);
  if (t === "string") return typeof v === "string";
  throw new Error(`schema uses a type outside the subset: ${t}`);
}

function validate(schema, value, path, errs) {
  for (const k of Object.keys(schema)) if (!KNOWN_KEYWORDS.has(k)) throw new Error(`schema uses a keyword outside the subset: ${k}`);
  if (schema.type !== undefined && !typeOk(schema.type, value)) { errs.push(`schema: ${path} must be ${schema.type}`); return; }
  if (schema.enum !== undefined && !schema.enum.includes(value)) { errs.push(`schema: ${path} must be one of ${schema.enum.join(", ")}`); return; }
  if (schema.pattern !== undefined && !new RegExp(schema.pattern).test(value)) { errs.push(`schema: ${path} must match ${schema.pattern}`); return; }
  if (schema.minimum !== undefined && value < schema.minimum) { errs.push(`schema: ${path} must be >= ${schema.minimum}`); return; }
  if (schema.minProperties !== undefined && Object.keys(value).length < schema.minProperties) {
    errs.push(`schema: ${path} must have at least ${schema.minProperties} key${schema.minProperties === 1 ? "" : "s"}`); return;
  }
  if (!isObject(value)) return;
  const sub = (k) => (path === "row" ? k : `${path}.${k}`);
  for (const k of schema.required || []) if (!(k in value)) errs.push(`schema: missing required field ${sub(k)}`);
  const props = schema.properties || {};
  for (const [k, v] of Object.entries(value)) {
    if (k in props) validate(props[k], v, sub(k), errs);
    else if (schema.additionalProperties === false) errs.push(`schema: unknown field ${sub(k)}`);
    else if (isObject(schema.additionalProperties)) validate(schema.additionalProperties, v, sub(k), errs);
  }
}

// ------------------------------------------------------------- row rules
// Each returns a list of reasons for one row. Rules after `schema` assume
// nothing about shape and return [] on anything malformed, so that a
// mutated (disabled) schema rule cannot crash the checker.
// A violation is a positive count; a negative count is a schema matter.
const allZero = (o) => isObject(o) && Object.values(o).every((v) => v === 0);
const anyPositive = (o) => isObject(o) && Object.values(o).some((v) => typeof v === "number" && v > 0);
// Every string in the row, values and keys alike: a status smuggled in as
// an object key is still a status typed into a row.
function stringsIn(v, out = []) {
  if (typeof v === "string") out.push(v);
  else if (Array.isArray(v)) v.forEach((x) => stringsIn(x, out));
  else if (isObject(v)) for (const [k, x] of Object.entries(v)) { out.push(k); stringsIn(x, out); }
  return out;
}
function plantMatches(checks, expected) {
  if (!isObject(checks) || !isObject(expected)) return false;
  const keys = new Set([...Object.keys(checks), ...Object.keys(expected)]);
  for (const k of keys) if (checks[k] !== expected[k]) return false;
  return true;
}

const ROW_RULES = {
  schema(row) { const errs = []; validate(SCHEMA, row, "row", errs); return errs; },
  unevaluable_reason_iff(row) {
    if (!isObject(row)) return [];
    const has = typeof row.unevaluable_reason === "string" && row.unevaluable_reason.length > 0;
    if (row.result === "UNEVALUABLE" && !has) return ["unevaluable_reason required when result is UNEVALUABLE"];
    if (row.result !== "UNEVALUABLE" && "unevaluable_reason" in row) return ["unevaluable_reason forbidden unless result is UNEVALUABLE"];
    return [];
  },
  planted_iff(row) {
    if (!isObject(row)) return [];
    if (row.cell === "twin" && !("planted" in row)) return ["planted required on twin"];
    if (row.cell === "live" && "planted" in row) return ["planted forbidden on live"];
    return [];
  },
  evaluated_keys(row) {
    if (!isObject(row) || !isObject(row.checks) || !isObject(row.evaluated)) return [];
    const a = Object.keys(row.checks).sort(), b = Object.keys(row.evaluated).sort();
    return a.length === b.length && a.every((k, i) => k === b[i]) ? [] : ["evaluated keys must equal checks keys"];
  },
  evaluated_zero(row) {
    if (!isObject(row) || !isObject(row.evaluated)) return [];
    const zero = Object.entries(row.evaluated).filter(([, v]) => v === 0).map(([k]) => k);
    return zero.length && row.result !== "UNEVALUABLE" ? [`evaluated zero forces UNEVALUABLE (${zero.join(", ")})`] : [];
  },
  live_violations_red(row) {
    if (!isObject(row)) return [];
    return row.cell === "live" && row.result === "GREEN" && anyPositive(row.checks) ? ["live with violations must be RED"] : [];
  },
  twin_zero_not_red(row) {
    if (!isObject(row)) return [];
    return row.cell === "twin" && row.result === "RED" && allZero(row.checks) ? ["twin with no violations must not be RED"] : [];
  },
  twin_plant_match(row) {
    if (!isObject(row) || row.cell !== "twin" || row.result !== "RED" || !isObject(row.planted)) return [];
    return plantMatches(row.checks, row.planted.expected_violations) ? [] : ["twin RED does not match the plant"];
  },
  status_literal(row) {
    const hit = stringsIn(row).find((s) => STATUS_LITERAL.test(s));
    return hit === undefined ? [] : [`status literal in row: "${hit}"`];
  },
};

// ------------------------------------------------------------- set rules
// Run only over rows that passed every row rule. Each receives the group
// (rows sharing surface and lane, in file order) and returns [{file, reason}].
const groupKey = (row) => `${row.surface}:lane${row.lane}`;
const SET_RULES = {
  set_one_live(group, key) {
    const lives = group.filter((e) => e.row.cell === "live");
    return lives.slice(1).map((e) => ({ file: e.file, reason: `second live cell for ${key} (have ${lives[0].file})` }));
  },
  set_twin_unique(group, key) {
    const seen = new Map(), out = [];
    for (const e of group.filter((x) => x.row.cell === "twin")) {
      const m = e.row.planted?.mutation;
      if (seen.has(m)) out.push({ file: e.file, reason: `duplicate twin mutation "${m}" for ${key} (have ${seen.get(m)})` });
      else seen.set(m, e.file);
    }
    return out;
  },
  set_live_red_human(group) {
    return group.filter((e) => e.row.cell === "live" && e.row.result === "RED").map((e) => ({ file: e.file, reason: "live RED needs a human: a real surface failed" }));
  },
  set_twin_green_human(group) {
    return group.filter((e) => e.row.cell === "twin" && e.row.result === "GREEN").map((e) => ({ file: e.file, reason: "twin GREEN needs a human: the gate missed the plant" }));
  },
};

// ------------------------------------------------------ expected-set rules
// Run only when an expect file is given (--expect; parseExpect below builds
// the Map they take, cell key -> sorted twin mutations). A cell is present
// when any row names a readable (surface, lane), whether or not that row
// passed the row rules: a refused row is already refused on its own, and
// must not also read as a missing cell. These rules compare cell presence
// and twin mutation sets only; they never assert a status.
function presentCells(entries) {
  const cells = new Map();
  for (const e of entries) {
    const r = e.row;
    if (!isObject(r) || typeof r.surface !== "string" || !Number.isInteger(r.lane)) continue;
    const k = groupKey(r);
    if (!cells.has(k)) cells.set(k, { file: e.file, twins: new Set() });
    if (r.cell === "twin" && isObject(r.planted) && typeof r.planted.mutation === "string") cells.get(k).twins.add(r.planted.mutation);
  }
  return cells;
}
const EXPECT_RULES = {
  expect_unexpected_cell(present, expected) {
    return [...present].filter(([k]) => !expected.has(k)).map(([k, c]) => ({ file: c.file, reason: `unexpected cell ${k}` }));
  },
  expect_missing_cell(present, expected, expectFile) {
    return [...expected.keys()].filter((k) => !present.has(k)).map((k) => ({ file: expectFile, reason: `expected cell has no rows ${k}` }));
  },
  expect_twin_set(present, expected, expectFile) {
    const out = [];
    for (const [k, want] of expected) {
      const c = present.get(k);
      if (!c) continue;
      const got = [...c.twins].sort();
      if (got.length !== want.length || got.some((m, i) => m !== want[i])) {
        out.push({ file: expectFile, reason: `twin mutation set differs for ${k} (expected [${want.join(", ")}], found [${got.join(", ")}])` });
      }
    }
    return out;
  },
};

export const RULE_NAMES = [...Object.keys(ROW_RULES), ...Object.keys(SET_RULES), ...Object.keys(EXPECT_RULES)];

// entries: [{file, row}] ; returns [{file, reason}] ; opts.disabled: Set of
// rule names ; opts.expect: the cells Map from parseExpect (expected-set
// rules run only when given) ; opts.expectFile: the name refusals about the
// expect file itself are reported against
export function checkRows(entries, opts = {}) {
  const disabled = opts.disabled || new Set();
  const out = [];
  const clean = [];
  for (const e of entries) {
    let bad = false;
    for (const [name, rule] of Object.entries(ROW_RULES)) {
      if (disabled.has(name)) continue;
      for (const reason of rule(e.row)) { out.push({ file: e.file, reason }); bad = true; }
    }
    if (!bad && isObject(e.row)) clean.push(e);
  }
  const groups = new Map();
  for (const e of clean) {
    const k = groupKey(e.row);
    if (!groups.has(k)) groups.set(k, []);
    groups.get(k).push(e);
  }
  for (const [key, group] of groups) {
    for (const [name, rule] of Object.entries(SET_RULES)) {
      if (disabled.has(name)) continue;
      out.push(...rule(group, key));
    }
  }
  if (opts.expect !== undefined && opts.expect !== null) {
    if (!(opts.expect instanceof Map)) throw new Error("checkRows: opts.expect must be the cells Map returned by parseExpect");
    const present = presentCells(entries);
    for (const [name, rule] of Object.entries(EXPECT_RULES)) {
      if (disabled.has(name)) continue;
      out.push(...rule(present, opts.expect, opts.expectFile || "expect"));
    }
  }
  return out;
}

// The --expect file: {"cells":[{"surface":"<s>","lane":<n>,"twins":["<mutation>",...]}]}.
// Returns {ok:true, cells: Map "<surface>:lane<n>" -> sorted twin mutations}
// or {ok:false, problems:[...]}. Strict: an unknown key at either level is
// malformed -- a cell carrying a status among them, since the expected set
// never asserts one -- as are a duplicate cell and a duplicate mutation in
// one cell's list. `twins` is required; a live-only cell lists [].
const EXPECT_CELL_KEYS = new Set(["surface", "lane", "twins"]);

// Duplicate object keys in JSON text that already parses. JSON.parse keeps
// the last of two equal keys and says nothing, so an expect file carrying
// `cells` twice (or a cell carrying `surface` twice) would silently lose
// what it authored first. Keys compare after unescaping, so a key
// spelled with an escape and the same key spelled plainly are one key.
// Returns the duplicated keys, in order found.
export function duplicateKeys(text) {
  const dups = [];
  const stack = []; // {keys: Set, wantKey: bool} for objects, null for arrays
  for (let i = 0; i < text.length; i++) {
    const ch = text[i];
    const top = stack[stack.length - 1];
    if (ch === "{") stack.push({ keys: new Set(), wantKey: true });
    else if (ch === "[") stack.push(null);
    else if (ch === "}" || ch === "]") stack.pop();
    else if (ch === "," && top) top.wantKey = true;
    else if (ch === '"') {
      let j = i + 1;
      while (j < text.length && text[j] !== '"') j += text[j] === "\\" ? 2 : 1;
      if (top && top.wantKey) {
        let key;
        try { key = JSON.parse(text.slice(i, j + 1)); } catch { key = text.slice(i + 1, j); }
        if (top.keys.has(key)) dups.push(key); else top.keys.add(key);
        top.wantKey = false;
      }
      i = j;
    }
  }
  return dups;
}

export function parseExpect(value) {
  const problems = [];
  if (!isObject(value)) return { ok: false, problems: ["must be a JSON object with a cells array"] };
  for (const k of Object.keys(value)) if (k !== "cells") problems.push(`unknown top-level key "${k}"`);
  if (!("cells" in value)) problems.push("missing cells");
  else if (!Array.isArray(value.cells)) problems.push("cells must be an array");
  const cells = new Map();
  const surfacePattern = SCHEMA.properties?.surface?.pattern ? new RegExp(SCHEMA.properties.surface.pattern) : null;
  for (const [i, c] of (Array.isArray(value.cells) ? value.cells : []).entries()) {
    const at = `cells[${i}]`;
    if (!isObject(c)) { problems.push(`${at} must be an object`); continue; }
    for (const k of Object.keys(c)) {
      if (!EXPECT_CELL_KEYS.has(k)) problems.push(`${at}: unknown key "${k}"${k === "status" ? " (an expected set never asserts a status; statuses are derived)" : ""}`);
    }
    const surfaceOk = typeof c.surface === "string" && c.surface.length > 0 && (!surfacePattern || surfacePattern.test(c.surface));
    if (!surfaceOk) problems.push(`${at}: surface must be a string matching the row schema's surface pattern`);
    const laneOk = Number.isInteger(c.lane) && c.lane >= 1;
    if (!laneOk) problems.push(`${at}: lane must be an integer >= 1`);
    let twinsOk = Array.isArray(c.twins) && c.twins.every((m) => typeof m === "string" && m.length > 0);
    if (!twinsOk) problems.push(`${at}: twins must be an array of non-empty mutation strings`);
    else if (new Set(c.twins).size !== c.twins.length) { problems.push(`${at}: twins lists a mutation more than once`); twinsOk = false; }
    if (surfaceOk && laneOk) {
      const k = groupKey(c);
      if (cells.has(k)) problems.push(`${at}: duplicate cell ${k}`);
      else if (twinsOk) cells.set(k, [...c.twins].sort());
    }
  }
  return problems.length ? { ok: false, problems } : { ok: true, cells };
}

// --------------------------------------------------------- crediting rules
// Claimability per (surface, lane), derived only from rows that conform.
// UNEVALUABLE when any row is -- a live row included -- and then no
// crediting rule runs at all: per-check crediting is a statement about
// checks a GREEN live reported, and a group that could not be evaluated
// has made none, so it derives UNEVALUABLE, never PARTIAL, and carries no
// "unfalsified". Otherwise CLAIMABLE when the live is GREEN, at least one
// twin exists (every twin is RED as planted, or it would have been
// refused), and no crediting rule below names an unfalsified check; PARTIAL
// in every other case, with "unfalsified" listing what a crediting rule
// named. Crediting rules are not refusals: they never change the exit
// code. test.mjs --mutate disables each in turn and requires a PASS
// fixture's expect_credit to notice.
const CREDIT_RULES = {
  // Every check key the live row reports must have been seen to fail: set
  // nonzero in the checks of at least one twin RED as planted. A gate whose
  // every twin plants 0 on a check has never shown that check can go red.
  credit_live_checks_falsified(g) {
    if (!g.live || !isObject(g.live.checks)) return [];
    const redAsPlanted = g.twins.filter((t) => t.result === "RED" && isObject(t.planted) && plantMatches(t.checks, t.planted.expected_violations));
    return Object.keys(g.live.checks).filter((k) => !redAsPlanted.some((t) => typeof t.checks[k] === "number" && t.checks[k] > 0));
  },
};
export const CREDIT_RULE_NAMES = Object.keys(CREDIT_RULES);

// opts.disabled: Set of crediting rule names
export function credit(entries, opts = {}) {
  const disabled = opts.disabled || new Set();
  const groups = new Map();
  for (const e of entries) {
    const k = groupKey(e.row);
    if (!groups.has(k)) groups.set(k, { surface: e.row.surface, lane: e.row.lane, live: null, twins: [] });
    const g = groups.get(k);
    if (e.row.cell === "live") g.live = e.row; else g.twins.push(e.row);
  }
  return [...groups.values()].map((g) => {
    const unevaluable = (g.live && g.live.result === "UNEVALUABLE") || g.twins.some((t) => t.result === "UNEVALUABLE");
    const unfalsified = unevaluable ? [] : [...new Set(Object.entries(CREDIT_RULES).filter(([name]) => !disabled.has(name)).flatMap(([, rule]) => rule(g)))].sort();
    let status;
    if (unevaluable) status = "UNEVALUABLE";
    else if (g.live && g.twins.length > 0 && unfalsified.length === 0) status = "CLAIMABLE";
    else status = "PARTIAL";
    const element = { surface: g.surface, lane: g.lane, status, live: g.live ? g.live.result : null, twins: g.twins.length };
    if (status === "PARTIAL" && unfalsified.length > 0) element.unfalsified = unfalsified;
    return element;
  });
}

// ----------------------------------------------------------------- PIN
const sha256lf = (buf) => createHash("sha256").update(buf.toString("utf8").replace(/\r\n/g, "\n")).digest("hex");
function walk(dir, out = []) {
  for (const e of readdirSync(dir, { withFileTypes: true }).sort((a, b) => a.name.localeCompare(b.name))) {
    const p = join(dir, e.name);
    if (e.isDirectory()) walk(p, out); else out.push(p);
  }
  return out;
}
const relHere = (p) => relative(HERE, p).split(sep).join("/");

// Shape guard on --write-pin's own construction: line 1 is the pack name,
// a space and the 40-hex sha (the regex below is that machine line's only
// spelling), and the total line count equals the number of hashed files plus that
// one line. Given how writePin below builds `lines` (one push per walked
// file) this can never actually fail in real use -- the only way to see
// it refuse is to call it directly with crafted lines/fileCount, which is
// how test.mjs exercises it without weakening the guard itself.
export function checkPinShape(lines, fileCount) {
  if (!Array.isArray(lines) || lines.length === 0) return "PIN content is empty";
  if (typeof lines[0] !== "string" || !/^datum [0-9a-f]{40}$/.test(lines[0])) return "PIN first line must be the pack name and a 40-hex commit sha";
  if (lines.length !== fileCount + 1) return `PIN line count ${lines.length} does not match hashed file count ${fileCount} + 1`;
  return null;
}

export function writePin(sha) {
  if (!/^[0-9a-f]{40}$/.test(sha)) throw new Error("--write-pin needs a 40-hex commit sha");
  const files = walk(HERE).filter((p) => relHere(p) !== "PIN");
  const lines = [`datum ${sha}`, ...files.map((p) => `${sha256lf(readFileSync(p))}  ${relHere(p)}`)];
  const problem = checkPinShape(lines, files.length);
  if (problem) throw new Error(problem);
  return lines.join("\n") + "\n";
}

// Writes PIN itself, atomically (temp file beside it, then rename), so a
// refused run never leaves a half-written PIN. (A shell redirect onto PIN
// is outside its reach: the shell truncates the file before node starts.)
// fsImpl is injectable (default: the real fs calls) so the failure paths
// can be forced on any platform -- POSIX rename onto an open file
// ordinarily succeeds, so the real defect (Windows: a shell redirect holds
// the destination open, and the rename throws EPERM) cannot be forced for
// real outside Windows; the injected object lets the same assertion run
// there. Refuses outright, writing nothing, when a `.PIN.tmp.*` file is
// already present beside check.mjs -- a prior run's leftover -- rather than
// racing it with this run's own temp file, and when the directory cannot
// be listed to look for one. Never throws. Returns {ok:true} on success or
// {ok:false, message} on refusal, in which case PIN (if any) is left
// byte-unchanged: the leftover check runs before anything is written, and
// any write or rename failure is followed by removing this run's own temp
// file. When that removal fails too, the temp file stays -- and refuses
// every later run as a leftover -- so the message names it.
const DEFAULT_FS = { writeFileSync, renameSync, unlinkSync, readdirSync };
const codeOf = (e) => String((e && (e.code || e.message)) || e);
export function writePinAtomic(sha, fsImpl = DEFAULT_FS) {
  let content;
  try { content = writePin(sha); }
  catch (e) { return { ok: false, message: e && e.message ? e.message : codeOf(e) }; }
  let leftover;
  try { leftover = fsImpl.readdirSync(HERE).find((n) => n.startsWith(".PIN.tmp.")); }
  catch (e) { return { ok: false, message: `--write-pin failed: cannot list the pack directory to look for a leftover temp file (${codeOf(e)})` }; }
  if (leftover) return { ok: false, message: `--write-pin failed: stale temp file present (${leftover})` };
  const pinPath = join(HERE, "PIN");
  const tmpName = `.PIN.tmp.${process.pid}.${Date.now()}`;
  const tmpPath = join(HERE, tmpName);
  try {
    fsImpl.writeFileSync(tmpPath, content);
    fsImpl.renameSync(tmpPath, pinPath);
  } catch (e) {
    let left = "";
    try { fsImpl.unlinkSync(tmpPath); }
    catch (u) {
      if (!(u && u.code === "ENOENT")) left = `; temp file ${tmpName} could not be removed (${codeOf(u)}) and refuses every later --write-pin until it is removed by hand`;
    }
    return { ok: false, message: `--write-pin failed: ${codeOf(e)}${left}` };
  }
  return { ok: true };
}

// returns [] when the vendored tree matches PIN, else a list of problems.
// A PIN that verifies holds exactly its first line and one line per file of
// the pack, each listed once under the name the pack walk gives it: a line
// naming a file outside the pack (through ..), a second spelling of a
// listed file, or a repeated line does not verify. So a verified PIN carries
// no text beyond the machine line and the pack's own file names -- which is
// what lets the self-test's wording control leave it out.
export function verifyPin() {
  const pinPath = join(HERE, "PIN");
  if (!existsSync(pinPath)) return ["PIN missing beside check.mjs"];
  const lines = readFileSync(pinPath, "utf8").split("\n").map((l) => l.trimEnd()).filter(Boolean);
  const problems = [];
  if (!/^datum [0-9a-f]{40}$/.test(lines[0] || "")) problems.push("PIN first line must be the pack name and a 40-hex commit sha");
  const listed = new Map();
  for (const l of lines.slice(1)) {
    const m = /^([0-9a-f]{64})  (\S+)$/.exec(l);
    if (!m) { problems.push(`PIN line unreadable: ${l}`); continue; }
    if (listed.has(m[2])) { problems.push(`PIN line repeats ${m[2]}`); continue; }
    listed.set(m[2], m[1]);
  }
  const packFiles = new Set(walk(HERE).map(relHere).filter((r) => r !== "PIN"));
  for (const [r, h] of listed) {
    if (!packFiles.has(r)) {
      problems.push(existsSync(join(HERE, ...r.split("/"))) ? `PIN mismatch: ${r} listed but not a file of the pack` : `PIN mismatch: ${r} listed but missing`);
      continue;
    }
    const got = sha256lf(readFileSync(join(HERE, ...r.split("/"))));
    if (got !== h) problems.push(`PIN mismatch: ${r} altered (sha256 ${got}, pinned ${h})`);
  }
  for (const r of packFiles) {
    if (!listed.has(r)) problems.push(`PIN mismatch: ${r} present but unlisted`);
  }
  return problems;
}

// ----------------------------------------------------------------- CLI
const USAGE = "usage: check.mjs <rows-dir> [--json] [--verify-pin] [--expect <file>] | check.mjs --verify-pin | check.mjs --write-pin <sha>";
const KNOWN_FLAGS = new Set(["--json", "--verify-pin"]);

function main(argv) {
  // --write-pin is a form of its own: exactly `--write-pin <sha>`. Any other
  // argument or flag beside it -- a rows directory, --json, a typo, a second
  // --write-pin -- is a usage error that writes nothing, not an argument
  // ignored while a PIN is written anyway.
  if (argv.includes("--write-pin")) {
    // a missing value, spelled as nothing, an empty string or another flag
    if (argv.length !== 2 || argv[0] !== "--write-pin" || argv[1] === "" || argv[1].startsWith("--")) { console.error(USAGE); return 2; }
    const result = writePinAtomic(argv[1]);
    if (!result.ok) { console.error(result.message); return 2; }
    console.log("ok pin written");
    return 0;
  }
  // --expect consumes the next argument as its value; a missing value, a
  // value that is itself a flag, or a second --expect is a usage error.
  const args = [];
  let expectPath = null;
  for (let i = 0; i < argv.length; i++) {
    if (argv[i] !== "--expect") { args.push(argv[i]); continue; }
    const v = argv[i + 1];
    if (expectPath !== null || v === undefined || v.startsWith("--")) { console.error(USAGE); return 2; }
    expectPath = v;
    i++;
  }
  const flags = args.filter((a) => a.startsWith("--"));
  const positional = args.filter((a) => !a.startsWith("--"));
  const unknown = flags.filter((f) => !KNOWN_FLAGS.has(f));
  // Exactly one positional argument, and no flag outside the known set: an
  // extra positional or an unrecognised flag (a typo included) is a usage
  // error, not a silently-ignored argument. --expect applies to a rows
  // directory, so it is a usage error without one.
  if (unknown.length > 0 || positional.length > 1) { console.error(USAGE); return 2; }
  if (expectPath !== null && positional.length === 0) { console.error(USAGE); return 2; }
  const json = flags.includes("--json");
  const doVerify = flags.includes("--verify-pin");
  if (doVerify) {
    const problems = verifyPin();
    if (problems.length) { for (const p of problems) console.error(p); return 2; }
    // Given alone, --verify-pin is a complete, successful invocation: CI
    // can verify the pin before running the vendored test.mjs at all.
    if (positional.length === 0) { console.log("ok pin verified"); return 0; }
  }
  const dir = positional[0];
  if (!dir) { console.error(USAGE); return 2; }
  if (!existsSync(dir) || !statSync(dir).isDirectory()) { console.error(`unevaluable: ${dir} is not a directory`); return 2; }
  const dirents = readdirSync(dir, { withFileTypes: true });
  if (dirents.length === 0) { console.error(`unevaluable: ${dir} holds no rows`); return 2; }
  // Every entry of the rows directory must be a regular file. A
  // subdirectory, a symlink, or a junction is named and refused, not
  // silently skipped; a file that does not parse as JSON (checked below)
  // makes the directory unevaluable the same way, so a refusable row
  // cannot hide behind its shape or its extension.
  for (const e of dirents) {
    if (!e.isFile()) { console.error(`unevaluable: ${e.name} is not a regular file`); return 2; }
  }
  const files = dirents.map((e) => e.name).sort();
  const entries = [];
  for (const f of files) {
    let text;
    try { text = readFileSync(join(dir, f), "utf8"); entries.push({ file: f, row: JSON.parse(text) }); }
    catch (e) { console.error(`unevaluable: ${f} is not JSON (${e.message}); every file in a rows directory is a row`); return 2; }
    // The same key twice in one object, at any depth, is not unambiguous
    // JSON: a parser keeps one value and drops the other without a word, so
    // the row would be judged on whichever it kept. Unevaluable, the same
    // class as a file that is not JSON.
    const dups = duplicateKeys(text);
    if (dups.length) {
      for (const k of dups) console.error(`unevaluable: ${f}: duplicate key ${JSON.stringify(k)}; a row with the same key twice in one object is not unambiguous JSON`);
      return 2;
    }
  }
  // An expect file that cannot be read, is not JSON, or is malformed makes
  // the run unevaluable: exit 2, never a refusal and never ignored.
  let expect = null;
  if (expectPath !== null) {
    let raw, text;
    try { text = readFileSync(expectPath, "utf8"); raw = JSON.parse(text); }
    catch (e) { console.error(`unevaluable: expect file ${expectPath} is not readable JSON (${e.code || e.message})`); return 2; }
    const dups = duplicateKeys(text);
    if (dups.length) { for (const k of dups) console.error(`unevaluable: expect file ${expectPath}: duplicate key ${JSON.stringify(k)}`); return 2; }
    const parsed = parseExpect(raw);
    if (!parsed.ok) { for (const p of parsed.problems) console.error(`unevaluable: expect file ${expectPath}: ${p}`); return 2; }
    expect = parsed.cells;
  }
  const refusals = checkRows(entries, { expect, expectFile: expectPath });
  if (refusals.length) {
    for (const r of refusals) console.log(`FAIL ${r.file}: ${r.reason}`);
    return 1;
  }
  const statuses = credit(entries);
  if (json) { console.log(JSON.stringify(statuses, null, 2)); return 0; }
  for (const s of statuses) {
    const unf = s.unfalsified ? `; unfalsified: ${s.unfalsified.join(", ")}` : "";
    console.log(`${s.surface} lane${s.lane} ${s.status} (live ${s.live ?? "absent"}, ${s.twins} twin${s.twins === 1 ? "" : "s"}${unf})`);
  }
  console.log(`ok ${entries.length} rows conform, ${statuses.length} surface${statuses.length === 1 ? "" : "s"}`);
  return 0;
}

// Compares real paths, not the literal strings: ESM module resolution
// realpaths import.meta.url by default, but process.argv[1] is left exactly
// as typed, so a strict-string comparison never matches when check.mjs is
// reached through a symlink or a Windows junction -- main() silently never
// runs, exit 0, no output (measured 2026-09-14).
function isEntryPoint() {
  if (!process.argv[1]) return false;
  const here = fileURLToPath(import.meta.url);
  try { return realpathSync(here) === realpathSync(process.argv[1]); }
  catch { return here === process.argv[1]; }
}
if (isEntryPoint()) {
  process.exit(main(process.argv.slice(2)));
}
