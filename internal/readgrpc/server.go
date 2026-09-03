// Package readgrpc adapts a reader.Reader to the generated gRPC service.
// It holds no logic: result structs become messages, sentinel errors become
// status codes, and that mapping lives here and nowhere else.
package readgrpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	meridianv1 "github.com/hossainpazooki/meridian/api/meridian/v1"
	"github.com/hossainpazooki/meridian/internal/feed"
	"github.com/hossainpazooki/meridian/internal/reader"
)

type Server struct {
	meridianv1.UnimplementedReaderServer
	r reader.Reader
}

func New(r reader.Reader) *Server { return &Server{r: r} }

func (s *Server) Head(ctx context.Context, _ *meridianv1.HeadRequest) (*meridianv1.HeadResponse, error) {
	h, err := s.r.Head(ctx)
	if err != nil {
		return nil, toStatus(err)
	}
	return &meridianv1.HeadResponse{Records: h.Records, PrefixHash: h.PrefixHash}, nil
}

func (s *Server) AsOf(ctx context.Context, req *meridianv1.AsOfRequest) (*meridianv1.AsOfResponse, error) {
	a, err := s.r.AsOf(ctx, req.GetSeq())
	if err != nil {
		return nil, toStatus(err)
	}
	return &meridianv1.AsOfResponse{Seq: a.Seq, PrefixHash: a.PrefixHash, SnapshotHash: a.SnapshotHash, Snapshot: a.Snapshot}, nil
}

func (s *Server) Reconcile(ctx context.Context, req *meridianv1.ReconcileRequest) (*meridianv1.ReconcileResponse, error) {
	rc, err := s.r.Reconcile(ctx, req.GetSeq(), req.GetStatement())
	if err != nil {
		return nil, toStatus(err)
	}
	out := &meridianv1.ReconcileResponse{Compared: rc.Compared}
	for _, m := range rc.Mismatches {
		out.Mismatches = append(out.Mismatches, &meridianv1.Mismatch{
			Instrument: m.Instrument, Field: m.Field, Ledger: m.Ledger, Custodian: m.Custodian, Delta: m.Delta,
		})
	}
	return out, nil
}

// toStatus is the whole error contract of the API (design section 1).
func toStatus(err error) error {
	var ce *feed.ChainError
	switch {
	case errors.Is(err, reader.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.As(err, &ce):
		return status.Error(codes.DataLoss, err.Error())
	case errors.Is(err, reader.ErrSeqOutOfRange), errors.Is(err, reader.ErrBadStatement):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
