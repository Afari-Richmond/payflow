package webhook

// event is the minimal shape we read out of a Paystack webhook
// payload. Deliberately narrow — parsed only after signature
// verification, and only the fields this handler actually needs.
type event struct {
	Event string `json:"event"`
	Data  struct {
		Reference string `json:"reference"`
	} `json:"data"`
}

// chargeSuccessEvent is the only event type this handler processes.
// Paystack does not send a corresponding "charge.failed" webhook — a
// failed/abandoned attempt is discovered via VerifyTransaction, not a
// distinct event type.
const chargeSuccessEvent = "charge.success"
