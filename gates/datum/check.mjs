#!/usr/bin/env node
// DATUM conformance pack: reference checker for GATE_VERDICT rows.
//
//   node check.mjs <rows-dir> [--json] [--verify-pin]
//   node check.mjs --write-pin <datum-sha>
//
// Exit 0: every row conforms and the set credits; the claimability per
// (surface, lane) is printed (or emitted as JSON with --json).
// Exit 1: one or more refusals, each as `FAIL <file>: <reason>` with a
// reason from reasons.md.
// Exit 2: unevaluable -- no directory, empty directory, a file that is not
// JSON, or (with --verify-pin) a vendored file missing, unlisted or
// altered. Exit 2 is never coerced into 0.
//
// The rule table below is the whole checker: test.mjs --mutate disables
// each entry in turn and requires a negative fixture to stop being refused.
// No dependencies. Node 20 or later.

import { readFileSync, readdirSync, existsSync, statSync } from "node:fs";
import { join, dirname, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";
import { createHash } from "node:crypto";

const HERE = dirname(fileURLToPath(import.meta.url));
const STATUS_LITERAL = /\b(CLAIMABLE|PARTIAL|UNCLAIMED)\b/;

// ---------------------------------------------------------------- schema
// The schema file is the specification; this is the subset validator that
// implements it. Vendored packs carry a copy of the schema beside check.mjs;
// in the datum repo it lives one level up under schema/.
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

export const RULE_NAMES = [...Object.keys(ROW_RULES), ...Object.keys(SET_RULES)];

// entries: [{file, row}] ; returns [{file, reason}] ; opts.disabled: Set of rule names
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
  return out;
}

// Claimability per (surface, lane), derived only from rows that conform.
// UNEVALUABLE when any row is; CLAIMABLE when the live is GREEN and at least
// one twin exists (every twin is RED as planted, or it would have been
// refused); PARTIAL when exactly one cell kind is present.
export function credit(entries) {
  const groups = new Map();
  for (const e of entries) {
    const k = groupKey(e.row);
    if (!groups.has(k)) groups.set(k, { surface: e.row.surface, lane: e.row.lane, live: null, twins: [] });
    const g = groups.get(k);
    if (e.row.cell === "live") g.live = e.row; else g.twins.push(e.row);
  }
  return [...groups.values()].map((g) => {
    let status;
    if ((g.live && g.live.result === "UNEVALUABLE") || g.twins.some((t) => t.result === "UNEVALUABLE")) status = "UNEVALUABLE";
    else if (g.live && g.twins.length > 0) status = "CLAIMABLE";
    else status = "PARTIAL";
    return { surface: g.surface, lane: g.lane, status, live: g.live ? g.live.result : null, twins: g.twins.length };
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

export function writePin(sha) {
  if (!/^[0-9a-f]{40}$/.test(sha)) throw new Error("--write-pin needs a 40-hex datum commit sha");
  const lines = [`datum ${sha}`];
  for (const p of walk(HERE)) {
    const r = relHere(p);
    if (r === "PIN") continue;
    lines.push(`${sha256lf(readFileSync(p))}  ${r}`);
  }
  return lines.join("\n") + "\n";
}

// returns [] when the vendored tree matches PIN, else a list of problems
export function verifyPin() {
  const pinPath = join(HERE, "PIN");
  if (!existsSync(pinPath)) return ["PIN missing beside check.mjs"];
  const lines = readFileSync(pinPath, "utf8").split("\n").map((l) => l.trimEnd()).filter(Boolean);
  const problems = [];
  if (!/^datum [0-9a-f]{40}$/.test(lines[0] || "")) problems.push("PIN first line must be `datum <sha>`");
  const listed = new Map();
  for (const l of lines.slice(1)) {
    const m = /^([0-9a-f]{64})  (\S+)$/.exec(l);
    if (!m) { problems.push(`PIN line unreadable: ${l}`); continue; }
    listed.set(m[2], m[1]);
  }
  for (const [r, h] of listed) {
    const p = join(HERE, ...r.split("/"));
    if (!existsSync(p)) { problems.push(`PIN mismatch: ${r} listed but missing`); continue; }
    const got = sha256lf(readFileSync(p));
    if (got !== h) problems.push(`PIN mismatch: ${r} altered (sha256 ${got}, pinned ${h})`);
  }
  for (const p of walk(HERE)) {
    const r = relHere(p);
    if (r !== "PIN" && !listed.has(r)) problems.push(`PIN mismatch: ${r} present but unlisted`);
  }
  return problems;
}

// ----------------------------------------------------------------- CLI
function main(argv) {
  const args = [...argv];
  const json = args.includes("--json");
  const doVerify = args.includes("--verify-pin");
  const wp = args.indexOf("--write-pin");
  if (wp !== -1) {
    try { process.stdout.write(writePin(args[wp + 1] || "")); return 0; }
    catch (e) { console.error(e.message); return 2; }
  }
  const positional = args.filter((a) => !a.startsWith("--"));
  if (doVerify) {
    const problems = verifyPin();
    if (problems.length) { for (const p of problems) console.error(p); return 2; }
  }
  const dir = positional[0];
  if (!dir) { console.error("usage: check.mjs <rows-dir> [--json] [--verify-pin] | --write-pin <sha>"); return 2; }
  if (!existsSync(dir) || !statSync(dir).isDirectory()) { console.error(`unevaluable: ${dir} is not a directory`); return 2; }
  // Every regular file in the rows directory is a row. Nothing is skipped by
  // name: a file that does not parse as JSON makes the directory
  // unevaluable, so a refusable row cannot hide behind its extension.
  const files = readdirSync(dir, { withFileTypes: true }).filter((e) => e.isFile()).map((e) => e.name).sort();
  if (files.length === 0) { console.error(`unevaluable: ${dir} holds no rows`); return 2; }
  const entries = [];
  for (const f of files) {
    try { entries.push({ file: f, row: JSON.parse(readFileSync(join(dir, f), "utf8")) }); }
    catch (e) { console.error(`unevaluable: ${f} is not JSON (${e.message}); every file in a rows directory is a row`); return 2; }
  }
  const refusals = checkRows(entries);
  if (refusals.length) {
    for (const r of refusals) console.log(`FAIL ${r.file}: ${r.reason}`);
    return 1;
  }
  const statuses = credit(entries);
  if (json) { console.log(JSON.stringify(statuses, null, 2)); return 0; }
  for (const s of statuses) console.log(`${s.surface} lane${s.lane} ${s.status} (live ${s.live ?? "absent"}, ${s.twins} twin${s.twins === 1 ? "" : "s"})`);
  console.log(`ok ${entries.length} rows conform, ${statuses.length} surface${statuses.length === 1 ? "" : "s"}`);
  return 0;
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  process.exit(main(process.argv.slice(2)));
}
