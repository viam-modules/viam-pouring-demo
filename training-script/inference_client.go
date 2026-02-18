package trainingscript

import (
	"context"

	mlinferencepb "go.viam.com/api/app/mlinference/v1"
	"go.viam.com/utils/rpc"
)

type InferenceClient struct {
	conn   rpc.ClientConn
	client mlinferencepb.MLInferenceServiceClient
}

func NewInferenceClient(ctx context.Context, apiKey, apiKeyID string) (*InferenceClient, error) {
	conn, err := rpc.DialDirectGRPC(
		ctx,
		"app.viam.com:443",
		nil,
		rpc.WithEntityCredentials(apiKeyID, rpc.Credentials{
			Type:    rpc.CredentialsTypeAPIKey,
			Payload: apiKey,
		}),
	)
	if err != nil {
		return nil, err
	}

	client := mlinferencepb.NewMLInferenceServiceClient(conn)

	return &InferenceClient{
		conn:   conn,
		client: client,
	}, nil
}

func (c *InferenceClient) Close() error {
	return c.conn.Close()
}
