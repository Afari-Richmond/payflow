package httpapi_test

import "context"

// fakeWebhookForwarder is a test double for httpapi.WebhookForwarder —
// lets tests exercise the HTTP <-> gRPC translation without a real
// network call.
type fakeWebhookForwarder struct {
	err error
}

func (f fakeWebhookForwarder) HandleWebhook(_ context.Context, _ []byte, _ string) error {
	return f.err
}
