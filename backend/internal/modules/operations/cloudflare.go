package operations

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/workspace"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var cloudflareTracer = otel.Tracer("github.com/NerdsWhoFish/kosmos/backend/operations/cloudflare")

func WithCloudflareProvider(provider CloudflareProvider) ModuleOption {
	return func(module *Module) { module.cloudflare = provider }
}

type CloudflareProvider interface {
	Domains(context.Context, string, string) ([]CloudflareDomain, error)
}

type LiveCloudflareProvider struct {
	client  *http.Client
	baseURL string
}

func NewLiveCloudflareProvider(client *http.Client) LiveCloudflareProvider {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return LiveCloudflareProvider{client: client, baseURL: "https://api.cloudflare.com/client/v4"}
}

type cloudflareZone struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type cloudflareRegistration struct {
	DomainName string     `json:"domain_name"`
	ExpiresAt  *time.Time `json:"expires_at"`
	AutoRenew  bool       `json:"auto_renew"`
	Status     string     `json:"status"`
}

func (p LiveCloudflareProvider) Domains(ctx context.Context, accountID, token string) ([]CloudflareDomain, error) {
	ctx, span := cloudflareTracer.Start(ctx, "cloudflare.domains")
	defer span.End()
	accountID, token = strings.TrimSpace(accountID), strings.TrimSpace(token)
	if accountID == "" || token == "" {
		err := errors.New("Cloudflare account ID and API token are required")
		span.RecordError(err)
		span.SetStatus(codes.Error, "missing credentials")
		return nil, err
	}
	zones, err := p.listZones(ctx, accountID, token)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "zone list failed")
		return nil, fmt.Errorf("list Cloudflare zones: %w", err)
	}
	registrations, err := p.listRegistrations(ctx, accountID, token)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "registration list failed")
		return nil, fmt.Errorf("list Cloudflare registrations: %w", err)
	}
	byName := make(map[string]CloudflareDomain, len(zones)+len(registrations))
	for _, zone := range zones {
		name := strings.ToLower(strings.TrimSpace(zone.Name))
		if name == "" {
			continue
		}
		byName[name] = CloudflareDomain{DomainName: name, ZoneID: zone.ID}
	}
	for _, registration := range registrations {
		name := strings.ToLower(strings.TrimSpace(registration.DomainName))
		if name == "" {
			continue
		}
		domain := byName[name]
		domain.DomainName = name
		domain.Registered = true
		domain.AutoRenew = registration.AutoRenew
		domain.Status = strings.ToLower(registration.Status)
		if registration.ExpiresAt != nil {
			domain.RenewalDate = registration.ExpiresAt.UTC().Format(time.DateOnly)
		}
		byName[name] = domain
	}
	result := make([]CloudflareDomain, 0, len(byName))
	for _, domain := range byName {
		result = append(result, domain)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].DomainName < result[j].DomainName })
	span.SetAttributes(attribute.Int("cloudflare.domain.count", len(result)))
	return result, nil
}

func (p LiveCloudflareProvider) listZones(ctx context.Context, accountID, token string) ([]cloudflareZone, error) {
	result := make([]cloudflareZone, 0)
	for page := 1; ; page++ {
		query := url.Values{"account.id": {accountID}, "page": {strconv.Itoa(page)}, "per_page": {"50"}}
		var response struct {
			Result     []cloudflareZone `json:"result"`
			Success    bool             `json:"success"`
			ResultInfo struct {
				Page       int `json:"page"`
				TotalPages int `json:"total_pages"`
			} `json:"result_info"`
		}
		if err := p.get(ctx, "/zones?"+query.Encode(), token, &response); err != nil {
			return nil, err
		}
		if !response.Success {
			return nil, errors.New("Cloudflare reported a failed zone request")
		}
		result = append(result, response.Result...)
		if response.ResultInfo.TotalPages <= page {
			break
		}
	}
	return result, nil
}

func (p LiveCloudflareProvider) listRegistrations(ctx context.Context, accountID, token string) ([]cloudflareRegistration, error) {
	result := make([]cloudflareRegistration, 0)
	cursor := ""
	for {
		query := url.Values{"per_page": {"50"}, "sort_by": {"name"}}
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		var response struct {
			Result     []cloudflareRegistration `json:"result"`
			Success    bool                     `json:"success"`
			ResultInfo struct {
				Cursor string `json:"cursor"`
			} `json:"result_info"`
		}
		path := "/accounts/" + url.PathEscape(accountID) + "/registrar/registrations?" + query.Encode()
		if err := p.get(ctx, path, token, &response); err != nil {
			return nil, err
		}
		if !response.Success {
			return nil, errors.New("Cloudflare reported a failed registration request")
		}
		result = append(result, response.Result...)
		if response.ResultInfo.Cursor == "" || response.ResultInfo.Cursor == cursor {
			break
		}
		cursor = response.ResultInfo.Cursor
	}
	return result, nil
}

func (p LiveCloudflareProvider) get(ctx context.Context, path, token string, target any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(p.baseURL, "/")+path, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	response, err := p.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Cloudflare returned HTTP %d", response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		return err
	}
	return nil
}

func (m *Module) cloudflareStatus(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := m.authorize(w, r)
	if !ok {
		return
	}
	var connection CloudflareConnection
	err := m.store.Get(r.Context(), scope, "cloudflareConnections", "default", &connection)
	if errors.Is(err, errNotFound) {
		writeJSON(w, http.StatusOK, map[string]any{"connected": false})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cloudflare_status_failed", "Could not load Cloudflare connection")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"connected": true, "accountId": connection.AccountID})
}

func (m *Module) configureCloudflare(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	var request struct {
		AccountID string `json:"accountId"`
		APIToken  string `json:"apiToken"`
	}
	if !decodeJSON(w, r, &request, 8<<10) {
		return
	}
	request.AccountID, request.APIToken = strings.ToLower(strings.TrimSpace(request.AccountID)), strings.TrimSpace(request.APIToken)
	_, accountIDError := hex.DecodeString(request.AccountID)
	if len(request.AccountID) != 32 || accountIDError != nil || len(request.APIToken) < 20 || len(request.APIToken) > 4096 {
		writeError(w, http.StatusBadRequest, "invalid_cloudflare_connection", "Enter a valid Cloudflare account ID and API token")
		return
	}
	domains, err := m.cloudflare.Domains(r.Context(), request.AccountID, request.APIToken)
	if err != nil {
		writeError(w, http.StatusBadGateway, "cloudflare_connection_failed", "Cloudflare could not verify that account, Zone Read, and Registrar access are available")
		return
	}
	sealed, err := encrypt(m.key, []byte(request.APIToken))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cloudflare_connection_failed", "Could not protect the Cloudflare connection")
		return
	}
	now := time.Now().UTC()
	connection := CloudflareConnection{ID: "default", AccountID: request.AccountID, EncryptedToken: sealed, CreatedBy: actor.Email, CreatedAt: now, UpdatedAt: now}
	var existing CloudflareConnection
	if err := m.store.Get(r.Context(), scope, "cloudflareConnections", "default", &existing); err == nil {
		connection.CreatedAt = existing.CreatedAt
	}
	if err := m.store.Put(r.Context(), scope, "cloudflareConnections", "default", connection); err != nil {
		writeError(w, http.StatusInternalServerError, "cloudflare_connection_failed", "Could not save the Cloudflare connection")
		return
	}
	_ = m.audit(r.Context(), scope, actor.Email, "cloudflare.connected", "integration", "default", "Connected Cloudflare domain inventory")
	writeJSON(w, http.StatusOK, map[string]any{"connected": true, "accountId": connection.AccountID, "domainCount": len(domains)})
}

func (m *Module) disconnectCloudflare(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok || !m.requireRole(w, r.Context(), scope, actor, "owner", "admin") {
		return
	}
	if err := m.store.Delete(r.Context(), scope, "cloudflareConnections", "default"); err != nil && !errors.Is(err, errNotFound) {
		writeError(w, http.StatusInternalServerError, "cloudflare_disconnect_failed", "Could not disconnect Cloudflare")
		return
	}
	_ = m.audit(r.Context(), scope, actor.Email, "cloudflare.disconnected", "integration", "default", "Disconnected Cloudflare domain inventory")
	w.WriteHeader(http.StatusNoContent)
}

func (m *Module) cloudflareDomains(w http.ResponseWriter, r *http.Request) {
	scope, _, ok := m.authorize(w, r)
	if !ok {
		return
	}
	domains, err := m.loadCloudflareDomains(r.Context(), scope)
	if errors.Is(err, errNotFound) {
		writeError(w, http.StatusConflict, "cloudflare_not_connected", "Connect Cloudflare in Settings first")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "cloudflare_domains_failed", "Could not load Cloudflare domains")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"domains": domains})
}

func (m *Module) linkCloudflareDomain(w http.ResponseWriter, r *http.Request) {
	scope, actor, ok := m.authorize(w, r)
	if !ok {
		return
	}
	var request struct {
		AccountID   string `json:"accountId"`
		DomainName  string `json:"domainName"`
		RenewalDate string `json:"renewalDate"`
	}
	if !decodeJSON(w, r, &request, 8<<10) {
		return
	}
	request.AccountID = strings.TrimSpace(request.AccountID)
	request.DomainName = strings.ToLower(strings.TrimSpace(request.DomainName))
	request.RenewalDate = strings.TrimSpace(request.RenewalDate)
	if request.AccountID == "" || request.DomainName == "" {
		writeError(w, http.StatusBadRequest, "invalid_cloudflare_domain", "Choose an account and Cloudflare domain")
		return
	}
	domains, err := m.loadCloudflareDomains(r.Context(), scope)
	if err != nil {
		writeError(w, http.StatusBadGateway, "cloudflare_domains_failed", "Could not load Cloudflare domains")
		return
	}
	var selected CloudflareDomain
	for _, domain := range domains {
		if domain.DomainName == request.DomainName {
			selected = domain
			break
		}
	}
	if selected.DomainName == "" {
		writeError(w, http.StatusBadRequest, "invalid_cloudflare_domain", "That domain is not available in the connected Cloudflare account")
		return
	}
	renewalDate := selected.RenewalDate
	if renewalDate == "" {
		renewalDate = request.RenewalDate
	}
	renewal, err := time.Parse(time.DateOnly, renewalDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "renewal_date_required", "Enter the registrar renewal date for this domain")
		return
	}
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		writeError(w, http.StatusInternalServerError, "reminder_schedule_failed", "Could not schedule renewal reminders")
		return
	}
	renewal = time.Date(renewal.Year(), renewal.Month(), renewal.Day(), 9, 0, 0, 0, location)
	website := workspace.Website{URL: "https://" + selected.DomainName, Domain: selected.DomainName, Provider: "cloudflare", ExternalID: selected.ZoneID, RenewalDate: renewalDate, AutoRenew: selected.AutoRenew, Status: selected.Status}
	reminders := make([]workspace.Reminder, 0, 3)
	for _, days := range []int{30, 14, 7} {
		sourceKey := fmt.Sprintf("cloudflare:%s:%s:%dd", selected.DomainName, renewalDate, days)
		reminders = append(reminders, workspace.Reminder{ID: deterministicID(scope + ":" + sourceKey), AccountID: request.AccountID, SourceKey: sourceKey, Title: fmt.Sprintf("%s renews in %d days", selected.DomainName, days), DueAt: renewal.AddDate(0, 0, -days), OwnerEmail: actor.Email})
	}
	account, savedReminders, err := m.workspace.LinkWebsiteRenewal(r.Context(), scope, request.AccountID, website, reminders)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cloudflare_link_failed", "Could not link the domain and renewal reminders")
		return
	}
	_ = m.audit(r.Context(), scope, actor.Email, "cloudflare.domain_linked", "account", account.ID, "Linked "+selected.DomainName+" and renewal reminders")
	writeJSON(w, http.StatusOK, map[string]any{"account": account, "domain": website, "reminders": savedReminders})
}

func (m *Module) loadCloudflareDomains(ctx context.Context, scope string) ([]CloudflareDomain, error) {
	var connection CloudflareConnection
	if err := m.store.Get(ctx, scope, "cloudflareConnections", "default", &connection); err != nil {
		return nil, err
	}
	plaintext, err := decrypt(m.key, connection.EncryptedToken)
	if err != nil {
		return nil, err
	}
	return m.cloudflare.Domains(ctx, connection.AccountID, string(plaintext))
}
