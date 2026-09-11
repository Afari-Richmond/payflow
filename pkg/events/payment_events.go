package events

// Payment event types. Routing keys on the "payflow.events" exchange
// match these exactly.
const (
	PaymentSucceeded = "payment.succeeded"
	PaymentFailed    = "payment.failed"
)

// PaymentSucceededPayload is the payload for a PaymentSucceeded event.
// AmountMinor is always an integer minor-currency-unit amount — never
// a float.
type PaymentSucceededPayload struct {
	PaymentID   string `json:"payment_id"`
	OrderID     string `json:"order_id"`
	AmountMinor int64  `json:"amount_minor"`
	Currency    string `json:"currency"`
}

// PaymentFailedPayload is the payload for a PaymentFailed event.
type PaymentFailedPayload struct {
	PaymentID string `json:"payment_id"`
	OrderID   string `json:"order_id"`
}
