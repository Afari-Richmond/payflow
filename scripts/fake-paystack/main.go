// Command fake-paystack is a dev-only stand-in for the real Paystack
// API, for local testing without real test-mode credentials. It
// implements just enough of /transaction/initialize and
// /transaction/verify/:reference to exercise payment-service's
// provider integration end to end.
//
// Never point this at anything but PAYSTACK_BASE_URL in a local .env.
// It performs no signature checks of its own and holds no real money.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"sync"
)

type txn struct {
	amount   float64
	currency string
}

func main() {
	port := flag.String("port", "4123", "port to listen on")
	flag.Parse()

	var mu sync.Mutex
	store := map[string]txn{}

	mux := http.NewServeMux()

	mux.HandleFunc("/transaction/initialize", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "invalid body", http.StatusBadRequest)
			return
		}
		ref, _ := body["reference"].(string)
		amount, _ := body["amount"].(float64)
		currency, _ := body["currency"].(string)

		mu.Lock()
		store[ref] = txn{amount: amount, currency: currency}
		mu.Unlock()

		log.Printf("initialize reference=%s amount=%v currency=%s", ref, amount, currency)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":  true,
			"message": "Authorization URL created",
			"data": map[string]any{
				"authorization_url": "https://fake-paystack.test/pay/" + ref,
				"access_code":       "fake_access_code",
				"reference":         ref,
			},
		})
	})

	mux.HandleFunc("/transaction/verify/", func(w http.ResponseWriter, r *http.Request) {
		ref := strings.TrimPrefix(r.URL.Path, "/transaction/verify/")

		mu.Lock()
		t, ok := store[ref]
		mu.Unlock()
		if !ok {
			http.Error(w, "unknown reference", http.StatusNotFound)
			return
		}

		log.Printf("verify reference=%s amount=%v currency=%s", ref, t.amount, t.currency)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status":  true,
			"message": "Verification successful",
			"data": map[string]any{
				"status":    "success",
				"reference": ref,
				"amount":    t.amount,
				"currency":  t.currency,
			},
		})
	})

	log.Printf("fake-paystack listening on :%s (initialize + verify only, no real Paystack calls)", *port)
	log.Fatal(http.ListenAndServe(":"+*port, mux))
}
