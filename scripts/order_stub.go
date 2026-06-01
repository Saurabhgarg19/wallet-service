// Order Service stub — demonstrates integration with Wallet Service.
// Usage: go run ./scripts/order_stub.go
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

var (
	baseURL       = "http://localhost:8080/api/v1"
	customerToken = fmt.Sprintf("customer:cust-stub-%d", time.Now().Unix())
	orderSvcToken = "order-service-secret"
)

const (
	initialBalance = 500.0
	topUpAmount    = 300.0
	deductAmount   = 110.0
)

func main() {
	// 1. Create wallet
	walletID := must(createWallet())
	fmt.Printf("[1] Created wallet: %s\n", walletID)

	// 2. Top up
	must(topUp(walletID))
	fmt.Printf("[2] Topped up ₹%.0f → wallet %s\n", topUpAmount, walletID)

	// 3. First deduction (new idempotency key)
	result := mustDeduct(walletID, "order-001", deductAmount)
	fmt.Printf("[3] Deduct order-001: status=%s balance=%.2f cached=%v\n",
		result["status"], result["balance"], result["servedFromIdempotencyCache"])

	// 4. Retry with same idempotency key — must return cached result
	result = mustDeduct(walletID, "order-001", deductAmount)
	fmt.Printf("[4] Retry  order-001: status=%s balance=%.2f cached=%v\n",
		result["status"], result["balance"], result["servedFromIdempotencyCache"])

	// 5. Deduct until insufficient balance
	for i := 2; i <= 9; i++ {
		key := fmt.Sprintf("order-%03d", i)
		res, code := deduct(walletID, key, deductAmount)
		if code >= 400 {
			fmt.Printf("[5.%d] %s → HTTP %d %s: %s\n", i, key, code, res["errorCode"], res["message"])
		} else {
			fmt.Printf("[5.%d] %s → HTTP %d balance=%.2f\n", i, key, code, res["balance"])
		}
	}
}

// --- HTTP helpers ---

func createWallet() (string, error) {
	body := map[string]interface{}{"initialBalance": initialBalance}
	resp, err := post("/wallets", customerToken, body)
	if err != nil {
		return "", err
	}
	return resp["walletId"].(string), nil
}

func topUp(walletID string) (interface{}, error) {
	body := map[string]interface{}{"amount": topUpAmount, "referenceId": "topup-stub"}
	_, err := post("/wallets/"+walletID+"/topup", customerToken, body)
	return nil, err
}

func mustDeduct(walletID, key string, amount float64) map[string]interface{} {
	res, _ := deduct(walletID, key, amount)
	return res
}

func deduct(walletID, key string, amount float64) (map[string]interface{}, int) {
	body := map[string]interface{}{
		"idempotencyKey": key,
		"amount":         amount,
		"referenceId":    key,
	}
	return postRaw("/wallets/"+walletID+"/deduct", orderSvcToken, body)
}

func post(path, token string, body interface{}) (map[string]interface{}, error) {
	res, code := postRaw(path, token, body)
	if code >= 400 {
		return res, fmt.Errorf("HTTP %d: %v", code, res)
	}
	return res, nil
}

func postRaw(path, token string, body interface{}) (map[string]interface{}, int) {
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, baseURL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "request error: %v\n", err)
		os.Exit(1)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	_ = json.Unmarshal(data, &result)
	return result, resp.StatusCode
}

func must[T any](v T, err error) T {
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return v
}
