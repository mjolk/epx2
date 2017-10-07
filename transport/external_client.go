package transport

import (
	"google.golang.org/grpc"

	transpb "github.com/mjolk/epx2/transport/transportpb"
)

// ExternalClient is a client stub implementing the KVServiceClient
// interface.
type ExternalClient struct {
	transpb.KVServiceClient
	*grpc.ClientConn
}

// NewExternalClient creates a new PaxosClient.
func NewExternalClient(addr string) (*ExternalClient, error) {
	conn, err := grpc.Dial(addr, clientOpts...)
	if err != nil {
		return nil, err
	}
	client := transpb.NewKVServiceClient(conn)
	return &ExternalClient{client, conn}, nil
}
