package gates

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	meridianv1 "github.com/hossainpazooki/meridian/api/meridian/v1"
	"github.com/hossainpazooki/meridian/internal/canon"
	"github.com/hossainpazooki/meridian/internal/feed"
	"github.com/hossainpazooki/meridian/internal/reader"
	"github.com/hossainpazooki/meridian/internal/readgrpc"
	"github.com/hossainpazooki/meridian/internal/reconcile"
)

// P7, wire fidelity: what a gRPC client receives is byte-for-byte what a
// local recompute over fixtures/base produces, and the client can tell when
// it is not. Every cell drives the SAME adapter over the SAME in-process
// transport; only the Reader behind it differs.

// mismatchKey flattens a reconcile mismatch to a comparable string so the
// server's list and the local list can be set-differenced.
func mismatchKey(instrument, field string, ledger, custodian, delta int64) string {
	return strings.Join([]string{instrument, field, strconv.FormatInt(ledger, 10), strconv.FormatInt(custodian, 10), strconv.FormatInt(delta, 10)}, "|")
}

// symDiff counts entries present in one multiset but not the other.
func symDiff(a, b []string) int64 {
	count := map[string]int64{}
	for _, k := range a {
		count[k]++
	}
	for _, k := range b {
		count[k]--
	}
	var n int64
	for _, v := range count {
		if v < 0 {
			v = -v
		}
		n += v
	}
	return n
}

// p7Check drives one client and returns the counts plus the V3 snapshot
// hash the client received (the row's content hash).
func p7Check(t *testing.T, client meridianv1.ReaderClient, m Manifest) (Counts, string) {
	t.Helper()
	ctx := context.Background()
	c := NewCounts("head_matches_local", "snapshot_rehash_matches_claimed", "snapshot_matches_local_recompute", "reconcile_matches_local")
	basePath := filepath.Join(FixturesDir, "base", "feed.jsonl")

	// head: two fields examined against a direct open of the base feed.
	f, err := feed.Open(basePath)
	if err != nil {
		t.Fatal(err)
	}
	localRecords := f.Len()
	localPrefix, err := f.PrefixHash(localRecords)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	h, err := client.Head(ctx, &meridianv1.HeadRequest{})
	if err != nil {
		t.Fatal(err)
	}
	c.Evaluated["head_matches_local"] = 2
	if h.GetRecords() != localRecords {
		c.Checks["head_matches_local"]++
	}
	if h.GetPrefixHash() != localPrefix {
		c.Checks["head_matches_local"]++
	}

	// snapshots: one AsOf per manifest viewpoint, in name order. The recompute
	// check covers seq, prefix_hash and bytes.
	names := m.Strs("p7", "viewpoints")
	sort.Strings(names)
	c.Evaluated["snapshot_rehash_matches_claimed"] = int64(len(names))
	c.Evaluated["snapshot_matches_local_recompute"] = int64(len(names))
	var lastHash string
	for _, name := range names {
		v := m.Int("viewpoints", name)
		resp, err := client.AsOf(ctx, &meridianv1.AsOfRequest{Seq: v})
		if err != nil {
			t.Fatalf("AsOf %s (%d): %v", name, v, err)
		}
		if "sha256:"+canon.SHA256Hex(resp.GetSnapshot()) != resp.GetSnapshotHash() {
			c.Checks["snapshot_rehash_matches_claimed"]++
		}
		local := ReadFixture(t, "base/feed.jsonl", v)
		if resp.GetSeq() != v || resp.GetPrefixHash() != local.PrefixHash || !bytes.Equal(resp.GetSnapshot(), local.Bytes) {
			c.Checks["snapshot_matches_local_recompute"]++
		}
		lastHash = resp.GetSnapshotHash()
	}

	// reconcile at end_seq against the base statement; the universe is
	// every field the LOCAL reconcile compared.
	endSeq := m.Int("end_seq")
	raw, err := os.ReadFile(filepath.Join(FixturesDir, "base", "statement.json"))
	if err != nil {
		t.Fatal(err)
	}
	st, err := reconcile.LoadStatementBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	localMs, localCompared, err := reconcile.Reconcile(ReadFixture(t, "base/feed.jsonl", endSeq).Doc, st)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Reconcile(ctx, &meridianv1.ReconcileRequest{Seq: endSeq, Statement: raw})
	if err != nil {
		t.Fatal(err)
	}
	var localKeys, wireKeys []string
	for _, x := range localMs {
		localKeys = append(localKeys, mismatchKey(x.Instrument, x.Field, x.Ledger, x.Custodian, x.Delta))
	}
	for _, x := range resp.GetMismatches() {
		wireKeys = append(wireKeys, mismatchKey(x.GetInstrument(), x.GetField(), x.GetLedger(), x.GetCustodian(), x.GetDelta()))
	}
	c.Evaluated["reconcile_matches_local"] = int64(localCompared)
	c.Checks["reconcile_matches_local"] = symDiff(localKeys, wireKeys)
	if resp.GetCompared() != int64(localCompared) {
		c.Checks["reconcile_matches_local"]++
	}
	return c, lastHash
}

// mislabeledReader serves the truth with a lie on the label: correct bytes,
// wrong snapshot_hash. A client that trusts the hash field instead of
// rehashing the bytes it received cannot see this.
type mislabeledReader struct{ reader.Reader }

func (r mislabeledReader) AsOf(ctx context.Context, seq int64) (reader.AsOf, error) {
	a, err := r.Reader.AsOf(ctx, seq)
	if err != nil {
		return a, err
	}
	b := []byte(a.SnapshotHash)
	last := len(b) - 1
	if b[last] == '0' {
		b[last] = '1'
	} else {
		b[last] = '0'
	}
	a.SnapshotHash = string(b)
	return a, nil
}

func p7Params(m Manifest) map[string]any {
	names := m.Strs("p7", "viewpoints")
	vp := map[string]any{}
	for _, n := range names {
		vp[n] = m.Int("viewpoints", n)
	}
	return map[string]any{"viewpoints": vp, "end_seq": m.Int("end_seq"), "listener": "bufconn"}
}

func TestP7WireFidelity(t *testing.T) {
	m := LoadManifest(t)
	params := p7Params(m)
	basePath := filepath.Join(FixturesDir, "base", "feed.jsonl")
	rows := int64(len(ReadFixture(t, "base/feed.jsonl", -1).State.Positions))

	// live
	live := reader.FeedReader{Path: basePath}
	client, stop, err := readgrpc.InProcess(live)
	if err != nil {
		t.Fatal(err)
	}
	c, hash := p7Check(t, client, m)
	stop()
	Emit(t, Row{Prop: 7, Cell: "live", Scope: "gRPC client over bufconn vs local recompute, fixtures/base", ContentHash: hash,
		Basis: "sha256 of canonical snapshot bytes as received by the gRPC client", Rows: rows, Params: params, Counts: c})

	// twin 1: a valid, re-chained feed that is not the base feed.
	wrong := reader.FeedReader{Path: filepath.Join(FixturesDir, filepath.FromSlash(m.Str("p7", "wrong_feed")))}
	client, stop, err = readgrpc.InProcess(wrong)
	if err != nil {
		t.Fatal(err)
	}
	cw, hw := p7Check(t, client, m)
	stop()
	Emit(t, Row{Prop: 7, Cell: "twin", Scope: "gRPC client over bufconn vs local recompute; server reads fixtures/p2/mutated as if it were fixtures/base", ContentHash: hw,
		Basis: "sha256 of canonical snapshot bytes as received by the gRPC client (twin server)", Rows: rows, Params: params, Counts: cw, Planted: ptr(m.Planted("p7", "twin_wrong_feed"))})

	// twin 2: correct bytes, mislabeled hash.
	client, stop, err = readgrpc.InProcess(mislabeledReader{live})
	if err != nil {
		t.Fatal(err)
	}
	cm, hm := p7Check(t, client, m)
	stop()
	Emit(t, Row{Prop: 7, Cell: "twin", Scope: "gRPC client over bufconn vs local recompute; server mislabels snapshot_hash on every AsOf", ContentHash: hm,
		Basis: "snapshot_hash field as received (deliberately wrong); the bytes rehash to the base V3 hash", Rows: rows, Params: params, Counts: cm, Planted: ptr(m.Planted("p7", "twin_mislabeled"))})
}
