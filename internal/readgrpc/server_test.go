package readgrpc

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	meridianv1 "github.com/hossainpazooki/meridian/api/meridian/v1"
	"github.com/hossainpazooki/meridian/internal/feed"
	"github.com/hossainpazooki/meridian/internal/reader"
	"github.com/hossainpazooki/meridian/internal/reconcile"
)

var fixtures = filepath.Join("..", "..", "fixtures")

// stubReader returns a fixed error from every method, so each row of the
// status-code table can be exercised without a matching on-disk condition.
type stubReader struct{ err error }

func (s stubReader) Head(context.Context) (reader.Head, error)        { return reader.Head{}, s.err }
func (s stubReader) AsOf(context.Context, int64) (reader.AsOf, error) { return reader.AsOf{}, s.err }
func (s stubReader) Reconcile(context.Context, int64, []byte) (reader.Recon, error) {
	return reader.Recon{}, s.err
}

func TestStatusCodeTable(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"not found", reader.ErrNotFound, codes.NotFound},
		{"chain", &feed.ChainError{Seq: 3, Reason: "prev mismatch"}, codes.DataLoss},
		{"seq range", reader.ErrSeqOutOfRange, codes.InvalidArgument},
		{"bad statement", reader.ErrBadStatement, codes.InvalidArgument},
		{"other", errors.New("disk on fire"), codes.Internal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client, stop, err := InProcess(stubReader{err: c.err})
			if err != nil {
				t.Fatal(err)
			}
			defer stop()
			ctx := context.Background()
			_, errH := client.Head(ctx, &meridianv1.HeadRequest{})
			_, errA := client.AsOf(ctx, &meridianv1.AsOfRequest{Seq: -1})
			_, errR := client.Reconcile(ctx, &meridianv1.ReconcileRequest{Seq: -1, Statement: []byte("{}")})
			for _, e := range []error{errH, errA, errR} {
				if status.Code(e) != c.want {
					t.Fatalf("got %v (%v), want %v", status.Code(e), e, c.want)
				}
			}
		})
	}
}

func TestRoundTripMatchesFeedReader(t *testing.T) {
	base := filepath.Join(fixtures, "base", "feed.jsonl")
	fr := reader.FeedReader{Path: base}
	client, stop, err := InProcess(fr)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	ctx := context.Background()

	wantH, _ := fr.Head(ctx)
	gotH, err := client.Head(ctx, &meridianv1.HeadRequest{})
	if err != nil || gotH.GetRecords() != wantH.Records || gotH.GetPrefixHash() != wantH.PrefixHash {
		t.Fatalf("Head over the wire = %v (%v), want %+v", gotH, err, wantH)
	}

	wantA, _ := fr.AsOf(ctx, -1)
	gotA, err := client.AsOf(ctx, &meridianv1.AsOfRequest{Seq: -1})
	if err != nil || gotA.GetSeq() != wantA.Seq || gotA.GetPrefixHash() != wantA.PrefixHash ||
		gotA.GetSnapshotHash() != wantA.SnapshotHash || !bytes.Equal(gotA.GetSnapshot(), wantA.Snapshot) {
		t.Fatalf("AsOf over the wire diverges from FeedReader (%v)", err)
	}

	raw, err := os.ReadFile(filepath.Join(fixtures, "base", "statement.json"))
	if err != nil {
		t.Fatal(err)
	}
	wantR, _ := fr.Reconcile(ctx, -1, raw)
	gotR, err := client.Reconcile(ctx, &meridianv1.ReconcileRequest{Seq: -1, Statement: raw})
	if err != nil || gotR.GetCompared() != wantR.Compared || len(gotR.GetMismatches()) != len(wantR.Mismatches) {
		t.Fatalf("Reconcile over the wire = %v (%v), want %+v", gotR, err, wantR)
	}
}

// A cash mismatch has an empty instrument; the mapping must preserve it and
// every numeric field, not just the count.
type oneMismatch struct{ stubReader }

func (oneMismatch) Reconcile(context.Context, int64, []byte) (reader.Recon, error) {
	return reader.Recon{Compared: 1, Mismatches: []reconcile.Mismatch{{Field: "cash", Ledger: 5, Custodian: 7, Delta: -2}}}, nil
}

func TestMismatchMappingPreservesEveryField(t *testing.T) {
	client, stop, err := InProcess(oneMismatch{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	resp, err := client.Reconcile(context.Background(), &meridianv1.ReconcileRequest{Seq: -1, Statement: []byte("{}")})
	if err != nil {
		t.Fatal(err)
	}
	if resp.GetCompared() != 1 || len(resp.GetMismatches()) != 1 {
		t.Fatalf("resp = %v", resp)
	}
	m := resp.GetMismatches()[0]
	if m.GetInstrument() != "" || m.GetField() != "cash" || m.GetLedger() != 5 || m.GetCustodian() != 7 || m.GetDelta() != -2 {
		t.Fatalf("mismatch mapped as %v", m)
	}
}
