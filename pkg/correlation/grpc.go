package correlation

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// UnaryServerInterceptor extracts a correlation ID from incoming gRPC
// metadata (key MetadataKey) into the request context, generating one
// if the caller didn't send one, so every internal gRPC call carries
// an ID even when its origin predates this package.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		id := fromIncomingMetadata(ctx)
		if id == "" {
			id = New()
		}
		return handler(WithID(ctx, id), req)
	}
}

func fromIncomingMetadata(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	vals := md.Get(MetadataKey)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}
