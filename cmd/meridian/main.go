// Command meridian is the thin CLI over the internal packages. It holds no
// logic of its own: append writes to the feed, replay verifies it, snapshot /
// asof recompute, reconcile compares. Exit codes: 0 ok, 1 usage or mismatch
// or other error, 2 feed chain error.
//
// "Holds no logic of its own" draws a specific line at append's input
// checking: requireNonEmpty rejects a blank --type/--id/--effective because
// that is presence checking, not domain knowledge — no fact about the
// ledger's event model is encoded here. It deliberately does NOT validate
// --type against the closed event-type set (fill/price/action/
// action_amendment) or --effective's YYYY-MM-DD shape, because internal/fold
// already owns and enforces both, and duplicating them here would mean two
// places to keep in sync as that set evolves — exactly the logic this CLI is
// supposed to be thin over. The cost of that choice: `--type feil` still
// exits 0 from append (a non-empty string passes this check), the malformed
// record is still written to the append-only log, and the mistake surfaces
// later — through asof/snapshot/reconcile's refusals array
// (`{"kind":"malformed","detail":"unknown event type feil"}`), not from the
// append command that made it, and not as a non-zero exit anywhere.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/hossainpazooki/meridian/internal/asof"
	"github.com/hossainpazooki/meridian/internal/canon"
	"github.com/hossainpazooki/meridian/internal/feed"
	"github.com/hossainpazooki/meridian/internal/reconcile"
	"github.com/hossainpazooki/meridian/internal/snapshot"
)

func main() { os.Exit(runCLI(os.Args[1:])) }

func fail(err error) int {
	fmt.Fprintln(os.Stderr, "error:", err)
	var ce *feed.ChainError
	if errors.As(err, &ce) {
		return 2
	}
	return 1
}

// requireNonEmpty checks that each named required flag was actually given a
// non-empty value, returning an error naming every one that was not.
//
// feed is APPEND-ONLY: a record written with an empty type or id cannot be
// retracted, only refused later by the fold at read time (as a "malformed"
// entry in some other command's output, at some other moment, never as a
// non-zero exit from the append that created it). Rejecting the omission
// here, before feed.Append is ever called, is the only point where the
// mistake can still be undone instead of merely detected. This checks
// presence only (non-empty) — it does not validate --type against the
// closed event-type set or --effective's YYYY-MM-DD shape; see main.go's
// package doc comment for why that line is drawn here.
func requireNonEmpty(named map[string]string) error {
	var missing []string
	for _, name := range []string{"type", "id", "effective"} {
		if v, ok := named[name]; ok && v == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	parts := make([]string, len(missing))
	for i, m := range missing {
		parts[i] = "--" + m
	}
	return fmt.Errorf("missing required flag(s): %s", strings.Join(parts, ", "))
}

// requireFeedExists stats path and returns an error if it is missing.
//
// feed.Open creates an empty-but-valid feed at a nonexistent path — correct
// for append, which is the only command meant to bring a feed into
// existence. Every read-only command (replay, snapshot, asof, reconcile)
// must call this first: without it, a mistyped --feed silently reports a
// clean empty chain (prefix_hash of all zeros), a well-formed zero-valued
// snapshot, or "mismatches=0" over nothing, AND leaves a stray empty file
// on disk — a green result standing in for no result, in a tool whose whole
// purpose is to make that failure mode impossible.
func requireFeedExists(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("feed does not exist: %s", path)
		}
		return err
	}
	return nil
}

func runCLI(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: meridian <append|replay|snapshot|asof|reconcile> [flags]")
		return 1
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	feedPath := fs.String("feed", "", "feed path")
	seq := fs.Int64("seq", -1, "viewpoint seq (default: end)")
	switch args[0] {
	case "append":
		typ := fs.String("type", "", "event type")
		id := fs.String("id", "", "event id")
		eff := fs.String("effective", "", "effective date YYYY-MM-DD")
		payload := fs.String("payload", "", "payload JSON object")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if err := requireNonEmpty(map[string]string{"type": *typ, "id": *id, "effective": *eff}); err != nil {
			return fail(err)
		}
		v, err := canon.Decode([]byte(*payload))
		if err != nil {
			return fail(err)
		}
		pm, ok := v.(map[string]any)
		if !ok {
			return fail(errors.New("payload must be a JSON object"))
		}
		f, err := feed.Open(*feedPath)
		if err != nil {
			return fail(err)
		}
		defer f.Close()
		rec, err := f.Append(*typ, *id, *eff, pm)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("seq=%d hash=%s\n", rec.Seq, rec.LineHash)
	case "replay":
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if err := requireFeedExists(*feedPath); err != nil {
			return fail(err)
		}
		f, err := feed.Open(*feedPath)
		if err != nil {
			return fail(err)
		}
		defer f.Close()
		h, _ := f.PrefixHash(f.Len())
		fmt.Printf("ok records=%d prefix_hash=%s\n", f.Len(), h)
	case "snapshot":
		out := fs.String("out", "", "output directory")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if err := requireFeedExists(*feedPath); err != nil {
			return fail(err)
		}
		r, err := asof.Read(*feedPath, *seq)
		if err != nil {
			return fail(err)
		}
		p, err := snapshot.Write(*out, r.Bytes, r.Hash)
		if err != nil {
			return fail(err)
		}
		fmt.Printf("%s %s\n", r.Hash, p)
	case "asof":
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if err := requireFeedExists(*feedPath); err != nil {
			return fail(err)
		}
		r, err := asof.Read(*feedPath, *seq)
		if err != nil {
			return fail(err)
		}
		os.Stdout.Write(r.Bytes)
	case "reconcile":
		stmt := fs.String("statement", "", "custodian statement JSON")
		if err := fs.Parse(args[1:]); err != nil {
			return 1
		}
		if err := requireFeedExists(*feedPath); err != nil {
			return fail(err)
		}
		r, err := asof.Read(*feedPath, *seq)
		if err != nil {
			return fail(err)
		}
		st, err := reconcile.LoadStatement(*stmt)
		if err != nil {
			return fail(err)
		}
		ms, compared, err := reconcile.Reconcile(r.Doc, st)
		if err != nil {
			return fail(err)
		}
		for _, m := range ms {
			fmt.Printf("instrument=%s field=%s ledger=%d custodian=%d delta=%d\n", m.Instrument, m.Field, m.Ledger, m.Custodian, m.Delta)
		}
		fmt.Printf("mismatches=%d compared=%d\n", len(ms), compared)
		if len(ms) > 0 {
			return 1
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown command:", args[0])
		return 1
	}
	return 0
}
