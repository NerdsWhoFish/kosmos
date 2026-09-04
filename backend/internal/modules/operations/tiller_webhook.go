package operations

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

const tillerWebhookConnectionID = "default"

type tillerMoney struct {
	Amount   int64  `json:"amount"`
	Currency string `json:"currency"`
}

type tillerEventLine struct {
	ItemID    string      `json:"item_id"`
	ProductID string      `json:"product_id"`
	PriceID   string      `json:"price_id"`
	Name      string      `json:"name"`
	Quantity  int64       `json:"quantity"`
	Amount    tillerMoney `json:"amount"`
}

type tillerEventEnvelope struct {
	ID            string    `json:"id"`
	SchemaVersion int       `json:"schema_version"`
	Type          string    `json:"type"`
	CreatedAt     time.Time `json:"created_at"`
	AppID         string    `json:"app_id"`
	Data          struct {
		OrderID string            `json:"order_id"`
		Lines   []tillerEventLine `json:"lines"`
	} `json:"data"`
}

func (m *Module) tillerWebhookStatus(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	var connection TillerWebhookConnection
	err := m.store.Get(r.Context(), scope, "tillerWebhookConnections", tillerWebhookConnectionID, &connection)
	if err != nil && !errors.Is(err, errNotFound) {
		writeError(w, http.StatusInternalServerError, "tiller_webhook_load_failed", "Could not load Tiller webhook settings")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connected": err == nil, "endpoint": "/api/v1/webhooks/tiller"})
}

func (m *Module) configureTillerWebhook(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	var request struct{ SigningSecret string }
	if !decodeJSON(w, r, &request, 8<<10) {
		return
	}
	request.SigningSecret = strings.TrimSpace(request.SigningSecret)
	raw, validPrefix := strings.CutPrefix(request.SigningSecret, "whsec_")
	if !validPrefix || len(raw) != 64 {
		writeError(w, http.StatusBadRequest, "invalid_tiller_webhook_secret", "Enter the signing secret returned when the Tiller webhook was created")
		return
	}
	if _, err := hex.DecodeString(raw); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_tiller_webhook_secret", "Enter the signing secret returned when the Tiller webhook was created")
		return
	}
	sealed, err := encrypt(m.key, []byte(request.SigningSecret))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tiller_webhook_save_failed", "Could not protect Tiller webhook settings")
		return
	}
	now := time.Now().UTC()
	connection := TillerWebhookConnection{ID: tillerWebhookConnectionID, EncryptedSecret: sealed, CreatedBy: actor.Email, CreatedAt: now, UpdatedAt: now}
	var existing TillerWebhookConnection
	if m.store.Get(r.Context(), scope, "tillerWebhookConnections", connection.ID, &existing) == nil {
		connection.CreatedAt = existing.CreatedAt
	}
	if err := m.store.Put(r.Context(), scope, "tillerWebhookConnections", connection.ID, connection); err != nil {
		writeError(w, http.StatusInternalServerError, "tiller_webhook_save_failed", "Could not save Tiller webhook settings")
		return
	}
	_ = m.audit(r.Context(), scope, actor.Email, "tiller.webhook_configured", "integration", connection.ID, "Configured authenticated Tiller purchase webhooks")
	writeJSON(w, http.StatusOK, map[string]any{"connected": true, "endpoint": "/api/v1/webhooks/tiller"})
}

func (m *Module) disconnectTillerWebhook(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	if err := m.store.Delete(r.Context(), scope, "tillerWebhookConnections", tillerWebhookConnectionID); err != nil && !errors.Is(err, errNotFound) {
		writeError(w, http.StatusInternalServerError, "tiller_webhook_delete_failed", "Could not disconnect Tiller webhooks")
		return
	}
	_ = m.audit(r.Context(), scope, actor.Email, "tiller.webhook_disconnected", "integration", tillerWebhookConnectionID, "Disconnected Tiller purchase webhooks")
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) tillerProductMappings(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	var mappings []TillerProductMapping
	if err := m.store.List(r.Context(), scope, "tillerProductMappings", &mappings); err != nil {
		writeError(w, http.StatusInternalServerError, "tiller_mapping_load_failed", "Could not load Tiller product mappings")
		return
	}
	sort.Slice(mappings, func(i, j int) bool { return mappings[i].ProductID < mappings[j].ProductID })
	writeJSON(w, http.StatusOK, map[string]any{"mappings": mappings})
}

func (m *Module) configureTillerProductMapping(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	productID := strings.TrimSpace(r.PathValue("id"))
	var request struct{ AccountID, ProductName string }
	if !decodeJSON(w, r, &request, 16<<10) {
		return
	}
	request.AccountID, request.ProductName = strings.TrimSpace(request.AccountID), strings.TrimSpace(request.ProductName)
	if !validTillerResourceID(productID, "prod") || request.AccountID == "" || len(request.ProductName) > 160 {
		writeError(w, http.StatusBadRequest, "invalid_tiller_mapping", "Choose an account and enter a valid Tiller product ID")
		return
	}
	if _, err := m.workspace.GetAccount(r.Context(), scope, request.AccountID); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_tiller_mapping", "Choose an existing Kosmos account")
		return
	}
	now := time.Now().UTC()
	id := deterministicID("tiller-product:" + productID)
	mapping := TillerProductMapping{ID: id, ProductID: productID, ProductName: request.ProductName, AccountID: request.AccountID, CreatedBy: actor.Email, CreatedAt: now, UpdatedAt: now}
	var existing TillerProductMapping
	if m.store.Get(r.Context(), scope, "tillerProductMappings", id, &existing) == nil {
		mapping.CreatedAt = existing.CreatedAt
	}
	if err := m.store.Put(r.Context(), scope, "tillerProductMappings", id, mapping); err != nil {
		writeError(w, http.StatusInternalServerError, "tiller_mapping_save_failed", "Could not save Tiller product mapping")
		return
	}
	_ = m.audit(r.Context(), scope, actor.Email, "tiller.product_mapped", "account", mapping.AccountID, "Mapped "+productID+" to a Kosmos account")
	writeJSON(w, http.StatusOK, mapping)
}

func (m *Module) deleteTillerProductMapping(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	productID := strings.TrimSpace(r.PathValue("id"))
	if err := m.store.Delete(r.Context(), scope, "tillerProductMappings", deterministicID("tiller-product:"+productID)); err != nil && !errors.Is(err, errNotFound) {
		writeError(w, http.StatusInternalServerError, "tiller_mapping_delete_failed", "Could not remove Tiller product mapping")
		return
	}
	_ = m.audit(r.Context(), scope, actor.Email, "tiller.product_unmapped", "integration", productID, "Removed Tiller product mapping")
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) receiveTillerWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
	if err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, "tiller_webhook_too_large", "Tiller webhook body is too large")
		return
	}
	var connection TillerWebhookConnection
	if err := m.store.Get(r.Context(), m.publicScope, "tillerWebhookConnections", tillerWebhookConnectionID, &connection); err != nil {
		writeError(w, http.StatusServiceUnavailable, "tiller_webhook_not_configured", "Tiller webhooks are not configured")
		return
	}
	secret, err := decrypt(m.key, connection.EncryptedSecret)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tiller_webhook_secret_failed", "Could not open Tiller webhook settings")
		return
	}
	if err := verifyTillerSignature(r.Header.Get("Tiller-Signature"), string(secret), body, time.Now().UTC(), 5*time.Minute); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_tiller_signature", "Tiller webhook signature is invalid")
		return
	}
	var envelope tillerEventEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil || envelope.SchemaVersion != 1 || envelope.ID == "" || envelope.AppID == "" || envelope.CreatedAt.IsZero() {
		writeError(w, http.StatusBadRequest, "invalid_tiller_event", "Tiller webhook event is invalid")
		return
	}
	if envelope.Type != "order.paid" {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	var mappings []TillerProductMapping
	if err := m.store.List(r.Context(), m.publicScope, "tillerProductMappings", &mappings); err != nil {
		writeError(w, http.StatusInternalServerError, "tiller_mapping_load_failed", "Could not load Tiller product mappings")
		return
	}
	byProduct := make(map[string]TillerProductMapping, len(mappings))
	for _, mapping := range mappings {
		byProduct[mapping.ProductID] = mapping
	}
	created := 0
	for index, line := range envelope.Data.Lines {
		mapping, ok := byProduct[line.ProductID]
		if !ok {
			continue
		}
		if line.Amount.Amount < 0 || !strings.EqualFold(line.Amount.Currency, "usd") || !validTillerResourceID(line.ProductID, "prod") {
			writeError(w, http.StatusBadRequest, "invalid_tiller_event", "Tiller purchase line is invalid")
			return
		}
		lineKey := line.ItemID
		if lineKey == "" {
			lineKey = fmt.Sprintf("%s:%s:%d", line.ProductID, line.PriceID, index)
		}
		id := deterministicID("tiller-webhook:" + envelope.ID + ":" + lineKey)
		description := strings.TrimSpace(line.Name)
		if description == "" {
			description = line.ProductID
		}
		transaction := Transaction{ID: id, ExternalID: envelope.ID + ":" + lineKey, Date: envelope.CreatedAt.UTC().Format(time.DateOnly), Description: description, Merchant: "Tiller", AmountCents: line.Amount.Amount, Source: "tiller-webhook", MatchStatus: "matched", AccountID: mapping.AccountID, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		if err := m.store.Create(r.Context(), m.publicScope, "transactions", id, transaction); errors.Is(err, errAlreadyExists) {
			continue
		} else if err != nil {
			writeError(w, http.StatusInternalServerError, "tiller_transaction_save_failed", "Could not record Tiller purchase")
			return
		}
		created++
		_ = m.notify(r.Context(), m.publicScope, "Customer purchase", description, "transaction", "/operations", "tiller-purchase:"+id)
	}
	if created > 0 {
		_ = m.audit(r.Context(), m.publicScope, "tiller-webhook", "tiller.purchase_imported", "order", envelope.Data.OrderID, fmt.Sprintf("Recorded %d mapped purchase transactions", created))
	}
	w.WriteHeader(http.StatusNoContent)
}

func validTillerResourceID(value, prefix string) bool {
	return strings.HasPrefix(value, prefix+"_") && len(value) > len(prefix)+1 && len(value) <= 100
}

func verifyTillerSignature(header, secret string, body []byte, now time.Time, tolerance time.Duration) error {
	var timestamp int64
	var candidates []string
	for _, part := range strings.Split(header, ",") {
		if value, ok := strings.CutPrefix(part, "t="); ok {
			parsed, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return errors.New("malformed signature timestamp")
			}
			timestamp = parsed
		} else if value, ok := strings.CutPrefix(part, "v1="); ok {
			candidates = append(candidates, value)
		}
	}
	if timestamp == 0 || len(candidates) == 0 || now.Unix()-timestamp > int64(tolerance.Seconds()) || timestamp-now.Unix() > int64(tolerance.Seconds()) {
		return errors.New("signature outside tolerance")
	}
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d.", timestamp)
	mac.Write(body)
	want := hex.EncodeToString(mac.Sum(nil))
	for _, candidate := range candidates {
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(want)) == 1 {
			return nil
		}
	}
	return errors.New("signature mismatch")
}
