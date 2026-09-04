package operations

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/workspace"
)

func TestTillerPurchaseWebhookMapsProductsIdempotently(t *testing.T) {
	module, mux, workspaceStore := newTestModule(t)
	account, err := workspaceStore.CreateAccount(context.Background(), "nerds-who-fish", workspace.Account{Name: "River Labs", Status: "customer"})
	if err != nil {
		t.Fatal(err)
	}
	performJSON[struct {
		Members []Member `json:"members"`
	}](t, mux, http.MethodGet, "/api/v1/members", "", http.StatusOK)
	secret := "whsec_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	performJSON[map[string]any](t, mux, http.MethodPut, "/api/v1/integrations/tiller/webhook", `{"signingSecret":"`+secret+`"}`, http.StatusOK)
	performJSON[TillerProductMapping](t, mux, http.MethodPut, "/api/v1/integrations/tiller/product-mappings/prod_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", `{"productName":"Guided trip","accountId":"`+account.ID+`"}`, http.StatusOK)

	body := []byte(`{"id":"evt_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","schema_version":1,"type":"order.paid","created_at":"2026-09-04T12:00:00Z","app_id":"app_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","data":{"order_id":"ord_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","lines":[{"item_id":"item_1","product_id":"prod_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","price_id":"price_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","name":"Guided trip","quantity":1,"amount":{"amount":25000,"currency":"usd"}},{"item_id":"item_2","product_id":"prod_unmapped","price_id":"price_unmapped","name":"Ignored product","quantity":1,"amount":{"amount":1000,"currency":"usd"}}]}}`)
	postSignedTillerWebhook(t, mux, body, secret, http.StatusNoContent)
	postSignedTillerWebhook(t, mux, body, secret, http.StatusNoContent)

	var transactions []Transaction
	if err := module.store.List(context.Background(), "nerds-who-fish", "transactions", &transactions); err != nil {
		t.Fatal(err)
	}
	if len(transactions) != 1 || transactions[0].AccountID != account.ID || transactions[0].AmountCents != 25000 || transactions[0].Source != "tiller-webhook" || transactions[0].MatchStatus != "matched" {
		t.Fatalf("transactions = %#v", transactions)
	}
}

func TestTillerWebhookRejectsInvalidSignature(t *testing.T) {
	_, mux, _ := newTestModule(t)
	performJSON[struct {
		Members []Member `json:"members"`
	}](t, mux, http.MethodGet, "/api/v1/members", "", http.StatusOK)
	secret := "whsec_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	performJSON[map[string]any](t, mux, http.MethodPut, "/api/v1/integrations/tiller/webhook", `{"signingSecret":"`+secret+`"}`, http.StatusOK)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/tiller", bytes.NewReader([]byte(`{}`)))
	request.Header.Set("Tiller-Signature", "t=1,v1=bad")
	recorder := httptest.NewRecorder()
	mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func postSignedTillerWebhook(t *testing.T, handler http.Handler, body []byte, secret string, want int) {
	t.Helper()
	timestamp := time.Now().UTC().Unix()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(timestamp, 10) + "."))
	mac.Write(body)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/tiller", bytes.NewReader(body))
	request.Header.Set("Tiller-Signature", "t="+strconv.FormatInt(timestamp, 10)+",v1="+hex.EncodeToString(mac.Sum(nil)))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != want {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, want, recorder.Body.String())
	}
}
