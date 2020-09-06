package server

import (
	api "mafia/log/api/v1"
	"mafia/log/lib"

	"context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"

	grpcMiddleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpcAuth "github.com/grpc-ecosystem/go-grpc-middleware/auth"
)

const (
	objectWildcard = "*"
	produceAction  = "produce"
	consumeAction  = "consume"
)

// Force compile checking for interface implementation
var _ api.LogServer = (*grpcServer)(nil)

type CommitLog interface {
	Append(*api.Record) (uint64, error)
	Read(uint64) (*api.Record, error)
}

type Authorizer interface {
	Authorize(subject, object, action string) error
}

type Config struct {
	CommitLog  CommitLog
	Authorizer Authorizer
}

type subjectContextKey struct{}

func subject(ctx context.Context) string {
	return ctx.Value(subjectContextKey{}).(string)
}

func authenticate(ctx context.Context) (context.Context, error) {
	pr, ok := peer.FromContext(ctx)
	if !ok {
		return ctx, lib.CreateStatusErr(
			codes.Unknown,
			"No peer info.",
			"en-US",
			"Couldn't find peer info.",
		).Err()
	}

	if pr.AuthInfo == nil {
		return ctx, lib.CreateStatusErr(
			codes.Unauthenticated,
			"No transport security.",
			"en-US",
			"No transport security being used.",
		).Err()
	}

	tlsInfo := pr.AuthInfo.(credentials.TLSInfo)
	subject := tlsInfo.State.VerifiedChains[0][0].Subject.CommonName
	ctx = context.WithValue(ctx, subjectContextKey{}, subject)

	return ctx, nil
}

type grpcServer struct {
	*Config
}

func NewGRPCServer(config *Config, opts ...grpc.ServerOption) (*grpc.Server, error) {
	opts = append(opts, grpc.StreamInterceptor(
		grpcMiddleware.ChainStreamServer(
			grpcAuth.StreamServerInterceptor(authenticate),
		)), grpc.UnaryInterceptor(grpcMiddleware.ChainUnaryServer(
		grpcAuth.UnaryServerInterceptor(authenticate),
	)))
	gSrv := grpc.NewServer(opts...)
	srv, err := newGrpcServer(config)
	if err != nil {
		return nil, lib.Wrap(err, "Unable to create grpc server wrapper")
	}

	api.RegisterLogServer(gSrv, srv)

	return gSrv, nil
}

func newGrpcServer(config *Config) (srv *grpcServer, err error) {
	srv = &grpcServer{
		Config: config,
	}

	return
}

// Appends log.
func (s *grpcServer) Produce(ctx context.Context, req *api.ProduceRequest) (*api.ProduceResponse, error) {
	if err := s.Authorizer.Authorize(
		subject(ctx),
		objectWildcard,
		produceAction,
	); err != nil {
		return nil, err
	}

	offset, err := s.CommitLog.Append(req.Record)
	if err != nil {
		return nil, lib.Wrap(err, "Unable to append log")
	}

	return &api.ProduceResponse{Offset: offset}, nil
}

// Read log from given offset.
func (s *grpcServer) Consume(ctx context.Context, req *api.ConsumeRequest) (*api.ConsumeResponse, error) {
	if err := s.Authorizer.Authorize(
		subject(ctx),
		objectWildcard,
		consumeAction,
	); err != nil {
		return nil, err
	}

	record, err := s.CommitLog.Read(req.Offset)
	if err != nil {
		return nil, api.ErrOffsetOutOfRange{Offset: req.Offset}
	}

	return &api.ConsumeResponse{Record: record}, nil
}

// Stream data from log storage until error or done context not happened.
func (s *grpcServer) ConsumeStream(req *api.ConsumeRequest, srv api.Log_ConsumeStreamServer) error {
	for {
		select {
		case <-srv.Context().Done():
			return nil
		default:
			res, err := s.Consume(srv.Context(), req)
			switch err.(type) {
			case nil:
			case api.ErrOffsetOutOfRange:
				continue
			default:
				return err
			}

			if err = srv.Send(res); err != nil {
				return err
			}

			req.Offset++
		}
	}
}

// Produce new log and Send response to the client.
func (s *grpcServer) ProduceStream(stream api.Log_ProduceStreamServer) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			return lib.Wrap(err, "Unable to get request from stream")
		}

		res, err := s.Produce(stream.Context(), req)
		if err != nil {
			return lib.Wrap(err, "Unable to proceed Produce logic")
		}

		if err = stream.Send(res); err != nil {
			return lib.Wrap(err, "Unable to send response via stream")
		}
	}
}
