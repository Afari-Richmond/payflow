package webhook

import (
	"crypto/hmac"
	"crypto/sha512"
	"encoding/hex"
)

// verifySignature checks that signature is a valid hex-encoded
// HMAC-SHA512 of rawBody, keyed with secretKey — proving the request
// genuinely came from Paystack and the body wasn't tampered with in
// transit. Uses hmac.Equal (constant-time) rather than ==, so this
// can't be exploited as a timing side-channel.
func verifySignature(secretKey string, rawBody []byte, signature string) bool {
	if signature == "" {
		return false
	}

	expected := hmac.New(sha512.New, []byte(secretKey))
	expected.Write(rawBody)
	expectedHex := hex.EncodeToString(expected.Sum(nil))

	return hmac.Equal([]byte(expectedHex), []byte(signature))
}
