package server

import (
	"context"
	"errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"log"
)

func (s *GrpcBackend) validateRequestMetadata(ctx context.Context) (newCtx context.Context, err error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		clientId := md.Get("client-id")
		if len(clientId) > 0 {
			if _, wasThere := s.onlineUsers[clientId[0]]; wasThere {
				ctx = context.WithValue(ctx, "client-id", clientId[0])
				return ctx, nil
			}
		}
	}
	return nil, errors.New("verification failed")
}

func (s *GrpcBackend) UnaryServerInterceptor(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {

	// no user id required
	if firstClientRequest := info.FullMethod == "/RegisterUser/Register"; firstClientRequest {
		log.Println("interceptor: /RegisterUser/Register route allowed")
		return handler(ctx, req)
	}

	// validate client id
	newCtx, err := s.validateRequestMetadata(ctx)
	log.Println("[unary interceptor]", "method:", info.FullMethod, "clientId:", newCtx.Value("client-id"))
	if err != nil {
		return nil, err
	}
	return handler(newCtx, req)
}

type customServerStream struct {
	grpc.ServerStream
	ctx context.Context
	md  metadata.MD
}

func (css *customServerStream) Context() context.Context {
	return css.ctx
}

func (css *customServerStream) SendHeader(md metadata.MD) error {
	// Merge any response headers if needed
	for key, values := range css.md {
		md[key] = append(md[key], values...)
	}
	return css.ServerStream.SendHeader(md)
}

func (s *GrpcBackend) StreamServerInterceptor(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	newCtx, err := s.validateRequestMetadata(ss.Context())
	if err != nil {
		return err
	}

	log.Println("[stream interceptor]", "method:", info.FullMethod, "clientId:", newCtx.Value("client-id"))

	md, _ := metadata.FromIncomingContext(ss.Context())

	err = handler(srv, &customServerStream{
		ServerStream: ss,
		ctx:          newCtx,
		md:           md,
	})
	if err != nil {
		log.Printf("Error during streaming: %v", err)
	}
	return err
}
