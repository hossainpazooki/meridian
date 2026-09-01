package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func build(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "meridian")
	if os.PathSeparator == '\\' {
		bin += ".exe"
	}
	out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput()
	if err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	return bin
}

func run(t *testing.T, bin string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return string(out), code
}

// runSplit is like run but keeps stdout and stderr separate, needed to
// assert "prints nothing to stdout" and "names the path on stderr"
// independently.
func runSplit(t *testing.T, bin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return outBuf.String(), errBuf.String(), code
}

func TestCLIRoundTrip(t *testing.T) {
	bin := build(t)
	dir := t.TempDir()
	feedPath := filepath.Join(dir, "feed.jsonl")
	out, code := run(t, bin, "append", "--feed", feedPath, "--type", "fill", "--id", "e1", "--effective", "2026-01-05",
		"--payload", `{"instrument":"AAA","price":1000,"qty":100,"side":"buy","trade_id":"T-1","venue":"X"}`)
	if code != 0 || !strings.HasPrefix(out, "seq=1 hash=") {
		t.Fatalf("%d %s", code, out)
	}
	run(t, bin, "append", "--feed", feedPath, "--type", "price", "--id", "e2", "--effective", "2026-01-06", "--payload", `{"instrument":"AAA","price":1100}`)
	out, code = run(t, bin, "replay", "--feed", feedPath)
	if code != 0 || !strings.HasPrefix(out, "ok records=2 prefix_hash=sha256:") {
		t.Fatalf("%d %s", code, out)
	}
	outDir := filepath.Join(dir, "snap")
	out, code = run(t, bin, "snapshot", "--feed", feedPath, "--out", outDir)
	if code != 0 {
		t.Fatal(out)
	}
	fields := strings.Fields(out)
	if len(fields) != 2 || !strings.HasPrefix(fields[0], "sha256:") {
		t.Fatal(out)
	}
	raw, _ := os.ReadFile(fields[1])
	asofOut, _ := run(t, bin, "asof", "--feed", feedPath, "--seq", "2")
	if string(raw) != asofOut {
		t.Fatal("asof output must equal the snapshot bytes")
	}
	stmt := filepath.Join(dir, "s.json")
	os.WriteFile(stmt, []byte(`{"as_of_seq":2,"cash":-100000,"holdings":[{"cost_basis":100000,"instrument":"AAA","quantity":100}]}`), 0o644)
	out, code = run(t, bin, "reconcile", "--feed", feedPath, "--statement", stmt)
	if code != 0 || !strings.Contains(out, "mismatches=0 compared=3") {
		t.Fatalf("%d %s", code, out)
	}
	os.WriteFile(stmt, []byte(`{"as_of_seq":2,"cash":-100000,"holdings":[{"cost_basis":100001,"instrument":"AAA","quantity":100}]}`), 0o644)
	out, code = run(t, bin, "reconcile", "--feed", feedPath, "--statement", stmt)
	if code != 1 || !strings.Contains(out, "instrument=AAA field=cost_basis ledger=100000 custodian=100001 delta=-1") {
		t.Fatalf("%d %s", code, out)
	}
	// tamper -> replay exit 2
	rawFeed, _ := os.ReadFile(feedPath)
	os.WriteFile(feedPath, []byte(strings.Replace(string(rawFeed), `"price":1000`, `"price":1001`, 1)), 0o644)
	_, code = run(t, bin, "replay", "--feed", feedPath)
	if code != 2 {
		t.Fatalf("tampered feed must exit 2, got %d", code)
	}
}

// TestReadCommandsRejectMissingFeed pins the fix: replay, snapshot, asof and
// reconcile must not silently succeed (nor create a file) when --feed names
// a path that does not exist. Before the fix, feed.Open's create-on-absent
// behavior made a typo look like a clean empty ledger: "ok records=0
// prefix_hash=sha256:000...", a well-formed zero-valued snapshot document,
// or "mismatches=0 compared=0" — a green result standing in for no result.
func TestReadCommandsRejectMissingFeed(t *testing.T) {
	bin := build(t)
	dir := t.TempDir()

	cases := []struct {
		name string
		args []string
	}{
		{"replay", []string{"replay", "--feed", ""}},
		{"snapshot", []string{"snapshot", "--feed", "", "--out", ""}},
		{"asof", []string{"asof", "--feed", "", "--seq", "1"}},
		{"reconcile", []string{"reconcile", "--feed", "", "--statement", ""}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			feedPath := filepath.Join(dir, "missing-"+c.name+".jsonl")
			outDir := filepath.Join(dir, "snap-"+c.name)
			stmtPath := filepath.Join(dir, "stmt-"+c.name+".json")
			os.WriteFile(stmtPath, []byte(`{"as_of_seq":1,"cash":0,"holdings":[]}`), 0o644)

			args := make([]string, len(c.args))
			copy(args, c.args)
			// Substitute the table's empty-string placeholders by flag name
			// (not position), so the case table above stays readable.
			for i := 0; i < len(args)-1; i++ {
				switch args[i] {
				case "--feed":
					args[i+1] = feedPath
				case "--out":
					args[i+1] = outDir
				case "--statement":
					args[i+1] = stmtPath
				}
			}

			stdout, stderr, code := runSplit(t, bin, args...)

			if code != 1 {
				t.Fatalf("%s: expected exit 1, got %d (stdout=%q stderr=%q)", c.name, code, stdout, stderr)
			}
			if stdout != "" {
				t.Fatalf("%s: expected empty stdout, got %q", c.name, stdout)
			}
			if !strings.Contains(stderr, feedPath) {
				t.Fatalf("%s: expected stderr to name the missing path %q, got %q", c.name, feedPath, stderr)
			}
			if _, err := os.Stat(feedPath); !os.IsNotExist(err) {
				t.Fatalf("%s: feed file must NOT be created for a read command, but Stat returned err=%v", c.name, err)
			}
		})
	}
}

// TestAppendStillCreatesMissingFeed pins the deliberate asymmetry: append is
// the one command allowed to bring a feed into existence. If this ever
// regresses to also requiring a pre-existing feed, `meridian append` on a
// fresh ledger would break.
func TestAppendStillCreatesMissingFeed(t *testing.T) {
	bin := build(t)
	dir := t.TempDir()
	feedPath := filepath.Join(dir, "brand-new.jsonl")

	if _, err := os.Stat(feedPath); !os.IsNotExist(err) {
		t.Fatalf("precondition: feed must not exist yet, Stat err=%v", err)
	}

	out, code := run(t, bin, "append", "--feed", feedPath, "--type", "fill", "--id", "e1", "--effective", "2026-01-05",
		"--payload", `{"instrument":"AAA","price":1000,"qty":100,"side":"buy","trade_id":"T-1","venue":"X"}`)
	if code != 0 || !strings.HasPrefix(out, "seq=1 hash=") {
		t.Fatalf("append on missing feed should create it and succeed: %d %s", code, out)
	}
	if _, err := os.Stat(feedPath); err != nil {
		t.Fatalf("append must have created the feed file, Stat err=%v", err)
	}
}

// TestAppendRejectsMissingRequiredFlags pins fix round 2: append must refuse
// a blank --type, --id, or --effective before ever calling feed.Append,
// rather than writing an unidentifiable record into the append-only log
// that can only be caught later (and by a different command) as a
// "malformed" refusal. Covers both cases the team lead asked to assert
// explicitly: a fresh path is left uncreated, and an existing feed is left
// byte-for-byte unchanged.
func TestAppendRejectsMissingRequiredFlags(t *testing.T) {
	bin := build(t)

	goodArgs := func(feedPath string) []string {
		return []string{"append", "--feed", feedPath, "--type", "fill", "--id", "e1", "--effective", "2026-01-05",
			"--payload", `{"instrument":"AAA","price":1000,"qty":100,"side":"buy","trade_id":"T-1","venue":"X"}`}
	}

	cases := []struct {
		omit string // which flag to blank out
	}{
		{"type"},
		{"id"},
		{"effective"},
	}

	for _, c := range cases {
		t.Run("omit-"+c.omit+"/fresh-path-not-created", func(t *testing.T) {
			dir := t.TempDir()
			feedPath := filepath.Join(dir, "feed.jsonl")
			args := goodArgs(feedPath)
			blankFlag(args, c.omit)

			stdout, stderr, code := runSplit(t, bin, args...)
			if code != 1 {
				t.Fatalf("omit --%s: expected exit 1, got %d (stdout=%q stderr=%q)", c.omit, code, stdout, stderr)
			}
			if stdout != "" {
				t.Fatalf("omit --%s: expected empty stdout, got %q", c.omit, stdout)
			}
			if !strings.Contains(stderr, "--"+c.omit) {
				t.Fatalf("omit --%s: expected stderr to name the missing flag, got %q", c.omit, stderr)
			}
			if _, err := os.Stat(feedPath); !os.IsNotExist(err) {
				t.Fatalf("omit --%s: feed file must NOT be created, but Stat returned err=%v", c.omit, err)
			}
		})

		t.Run("omit-"+c.omit+"/existing-feed-unchanged", func(t *testing.T) {
			dir := t.TempDir()
			feedPath := filepath.Join(dir, "feed.jsonl")
			// Seed one valid record so the feed exists with known content.
			seedOut, seedCode := run(t, bin, goodArgs(feedPath)...)
			if seedCode != 0 || !strings.HasPrefix(seedOut, "seq=1 hash=") {
				t.Fatalf("seed append failed: %d %s", seedCode, seedOut)
			}
			before, err := os.ReadFile(feedPath)
			if err != nil {
				t.Fatal(err)
			}

			args := goodArgs(feedPath)
			blankFlag(args, c.omit)
			_, code := run(t, bin, args...)
			if code != 1 {
				t.Fatalf("omit --%s: expected exit 1 against an existing feed, got %d", c.omit, code)
			}

			after, err := os.ReadFile(feedPath)
			if err != nil {
				t.Fatal(err)
			}
			if string(before) != string(after) {
				t.Fatalf("omit --%s: feed content changed after a rejected append\nbefore=%q\nafter=%q", c.omit, before, after)
			}
		})
	}
}

// blankFlag rewrites args (in the "--flag value" shape goodArgs produces) so
// the named flag's value is the empty string, in place.
func blankFlag(args []string, flag string) {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == "--"+flag {
			args[i+1] = ""
			return
		}
	}
}
