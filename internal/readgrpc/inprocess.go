package readgrpc

import (
	"context"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	meridianv1 "github.com/hossainpazooki/meridian/api/meridian/v1"
	"github.com/hossainpazooki/meridian/internal/reader"
)

// InProcess serves r over an in-memory listener and returns a connected
// client. No port is opened. The P7 gate and the adapter's own tests use
// it so the transport is identical across the live cell and every twin.
// bufconn therefore sits in the non-test build graph (and in cmd/meridian's
// dependency list): package gates cannot import another package's _test.go,
// and the P7 gate is the primary caller.
func InProcess(r reader.Reader) (meridianv1.ReaderClient, func(), error) {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	meridianv1.RegisterReaderServer(srv, New(r))
	go func() { _ = srv.Serve(lis) }()
	conn, err := grpc.NewClient("passthrough:///bufconn",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		srv.Stop()
		return nil, nil, err
	}
	stop := func() { conn.Close(); srv.Stop(); lis.Close() }
	return meridianv1.NewReaderClient(conn), stop, nil
}
