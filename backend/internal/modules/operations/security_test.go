package operations

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/workspace"
)

type membershipFailureStore struct {
	Store
	memberErr  error
	createRace *Member
}

func (s *membershipFailureStore) Get(ctx context.Context, scope, collection, id string, target any) error {
	if collection == "members" && s.memberErr != nil {
		err := s.memberErr
		s.memberErr = nil
		return err
	}
	return s.Store.Get(ctx, scope, collection, id, target)
}

func (s *membershipFailureStore) Create(ctx context.Context, scope, collection, id string, value any) error {
	if collection == "members" && s.createRace != nil {
		if err := s.Store.Create(ctx, scope, collection, id, *s.createRace); err != nil {
			return err
		}
	}
	return s.Store.Create(ctx, scope, collection, id, value)
}

func TestMembershipLookupFailurePreservesAccess(t *testing.T) {
	for _, member := range []Member{{Role: "member", Status: "disabled"}, {Role: "viewer", Status: "active"}} {
		t.Run(member.Role+"/"+member.Status, func(t *testing.T) {
			m, _, _ := newTestModule(t)
			ctx := context.Background()
			actor := Identity{Email: "restricted@example.com", Name: "Restricted"}
			member.ID, member.Email, member.Name = memberID(actor.Email), actor.Email, actor.Name
			if err := m.store.Put(ctx, m.publicScope, "members", member.ID, member); err != nil {
				t.Fatal(err)
			}
			failure := errors.New("Firestore unavailable")
			m.store = &membershipFailureStore{Store: m.store, memberErr: failure}
			if err := m.CheckAccess(ctx, m.publicScope, actor, true); !errors.Is(err, failure) {
				t.Fatalf("authorization error = %v", err)
			}
			var saved Member
			if err := m.store.Get(ctx, m.publicScope, "members", member.ID, &saved); err != nil {
				t.Fatal(err)
			}
			if saved.Role != member.Role || saved.Status != member.Status {
				t.Fatalf("membership overwritten: %+v", saved)
			}
		})
	}
}

func TestConcurrentMembershipProvisioningPreservesExistingRole(t *testing.T) {
	m, _, _ := newTestModule(t)
	actor := Identity{Email: "viewer@example.com", Name: "Viewer"}
	member := Member{ID: memberID(actor.Email), Email: actor.Email, Name: actor.Name, Role: "viewer", Status: "active"}
	m.store = &membershipFailureStore{Store: m.store, createRace: &member}
	if err := m.CheckAccess(context.Background(), m.publicScope, actor, true); err == nil {
		t.Fatal("provisioning race overwrote viewer")
	}
}

func TestMemberNameUpdateDoesNotOverwriteRole(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	member := Member{ID: "member", Role: "viewer", Status: "disabled", Name: "Old"}
	if err := store.Put(ctx, "scope", "members", member.ID, member); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateMemberName(ctx, "scope", member.ID, "New", time.Now()); err != nil {
		t.Fatal(err)
	}
	var saved Member
	if err := store.Get(ctx, "scope", "members", member.ID, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Role != "viewer" || saved.Status != "disabled" || saved.Name != "New" {
		t.Fatalf("unexpected membership: %+v", saved)
	}
}

func signIntakeRequest(r *http.Request, key []byte, ip string, now time.Time) {
	timestamp := strconv.FormatInt(now.Unix(), 10)
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("kosmos-intake-v1\n" + timestamp + "\n" + ip))
	r.Header.Set("X-Kosmos-Client-IP", ip)
	r.Header.Set("X-Kosmos-Client-Time", timestamp)
	r.Header.Set("X-Kosmos-Client-Signature", hex.EncodeToString(mac.Sum(nil)))
}

func TestSignedIntakeIdentity(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	now := time.Now().UTC()
	for _, test := range []struct {
		name   string
		mutate func(*http.Request)
		valid  bool
	}{
		{"valid", func(*http.Request) {}, true},
		{"spoofed IP", func(r *http.Request) { r.Header.Set("X-Kosmos-Client-IP", "192.0.2.2") }, false},
		{"expired", func(r *http.Request) { signIntakeRequest(r, key, "192.0.2.1", now.Add(-61*time.Second)) }, false},
		{"future", func(r *http.Request) { signIntakeRequest(r, key, "192.0.2.1", now.Add(61*time.Second)) }, false},
		{"duplicate", func(r *http.Request) { r.Header.Add("X-Kosmos-Client-IP", "192.0.2.1") }, false},
		{"missing", func(r *http.Request) { r.Header.Del("X-Kosmos-Client-Signature") }, false},
		{"invalid IP", func(r *http.Request) { signIntakeRequest(r, key, "arbitrary-key", now) }, false},
		{"noncanonical IP", func(r *http.Request) { signIntakeRequest(r, key, "::ffff:192.0.2.1", now) }, false},
		{"forged signature", func(r *http.Request) { signIntakeRequest(r, []byte("different-key"), "192.0.2.1", now) }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/api/v1/intake/contact", nil)
			signIntakeRequest(r, key, "192.0.2.1", now)
			test.mutate(r)
			ip, err := signedClientIP(r, key, now)
			if (err == nil) != test.valid {
				t.Fatalf("ip=%q error=%v", ip, err)
			}
		})
	}
}

func TestIntakeLimiterIgnoresSpoofedHeadersAcrossInstances(t *testing.T) {
	store := NewMemoryStore()
	modules := make([]*Module, 2)
	for i := range modules {
		modules[i], _, _ = newTestModule(t)
		modules[i].store = store
	}
	var admitted atomic.Int32
	var group sync.WaitGroup
	for i := 0; i < 30; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			r := httptest.NewRequest(http.MethodPost, "/api/v1/intake/contact", nil)
			r.RemoteAddr = "192.0.2.1:12345"
			r.Header.Set("CF-Connecting-IP", fmt.Sprintf("spoof-%d", i))
			r.Header.Set("X-Forwarded-For", fmt.Sprintf("192.0.2.%d", i))
			w := httptest.NewRecorder()
			if modules[i%2].allowPublicIntake(w, r) {
				admitted.Add(1)
			} else if w.Code != http.StatusTooManyRequests {
				t.Errorf("status=%d", w.Code)
			}
		}(i)
	}
	group.Wait()
	if admitted.Load() != 5 {
		t.Fatalf("admitted=%d", admitted.Load())
	}
}

func TestFirestoreIntakeRateLimitAcrossClients(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("requires Firestore emulator")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stores := make([]*FirestoreStore, 2)
	for i := range stores {
		client, err := firestore.NewClient(ctx, "kosmos-security-test")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = client.Close() })
		stores[i] = NewFirestoreStore(client)
	}
	scope := fmt.Sprintf("intake-%d", time.Now().UnixNano())
	now := time.Now().UTC()
	var admitted atomic.Int32
	var group sync.WaitGroup
	for i := 0; i < 20; i++ {
		group.Add(1)
		go func(i int) {
			defer group.Done()
			allowed, _, err := stores[i%2].AllowRateLimit(ctx, scope, "source", 5, time.Hour, now)
			if err != nil {
				t.Errorf("transaction failed: %v", err)
				return
			}
			if allowed {
				admitted.Add(1)
			}
		}(i)
	}
	group.Wait()
	if admitted.Load() != 5 {
		t.Fatalf("admitted=%d", admitted.Load())
	}
	if allowed, _, err := stores[1].AllowRateLimit(ctx, scope, "source", 5, time.Hour, now.Add(time.Hour)); !allowed || err != nil {
		t.Fatalf("expired quota not reusable: allowed=%v error=%v", allowed, err)
	}
}

func TestRateLimitExpiresAndIsScoped(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()
	now := time.Now()
	for i := 0; i < 5; i++ {
		if ok, _, err := store.AllowRateLimit(ctx, "scope", "key", 5, time.Hour, now); !ok || err != nil {
			t.Fatalf("ok=%v error=%v", ok, err)
		}
	}
	if ok, retry, err := store.AllowRateLimit(ctx, "scope", "key", 5, time.Hour, now); ok || retry != time.Hour || err != nil {
		t.Fatalf("ok=%v retry=%v error=%v", ok, retry, err)
	}
	if ok, _, err := store.AllowRateLimit(ctx, "other", "key", 5, time.Hour, now); !ok || err != nil {
		t.Fatal("scopes shared quota")
	}
	if ok, _, err := store.AllowRateLimit(ctx, "scope", "next", 5, time.Hour, now.Add(time.Hour)); !ok || err != nil {
		t.Fatal("expiration failed")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.collection("scope", "intakeRateLimits")["key"]; exists {
		t.Fatal("expired client retained")
	}
}

func TestRateLimitOutOfOrderAdmissionPreservesLatestExpiry(t *testing.T) {
	now := time.Now()
	window, _, _ := (rateLimitWindow{}).admit(5, time.Hour, now.Add(time.Minute))
	window, allowed, _ := window.admit(5, time.Hour, now)
	if !allowed || !window.ExpiresAt.Equal(now.Add(time.Hour+time.Minute)) {
		t.Fatalf("window expired before the latest request: %+v", window)
	}
}

type unavailableRateStore struct{ Store }

func (s unavailableRateStore) AllowRateLimit(context.Context, string, string, int, time.Duration, time.Time) (bool, time.Duration, error) {
	return false, 0, errors.New("unavailable")
}

func TestIntakeFailsClosedOnMissingSigningKeyOrStoreFailure(t *testing.T) {
	for _, test := range []string{"missing signing key", "store unavailable"} {
		t.Run(test, func(t *testing.T) {
			m, _, _ := newTestModule(t)
			if test == "missing signing key" {
				WithIntakeSigningKey(nil)(m)
			} else {
				m.store = unavailableRateStore{m.store}
			}
			w := httptest.NewRecorder()
			if m.allowPublicIntake(w, httptest.NewRequest(http.MethodPost, "/api/v1/intake/contact", nil)) || w.Code != http.StatusServiceUnavailable {
				t.Fatalf("status=%d", w.Code)
			}
		})
	}
}

func TestSpreadsheetTextPreservesDataWithoutFormulaEvaluation(t *testing.T) {
	for _, value := range []string{"=1+1", "+123", "-formula", "@SUM(A1)", " \t=1+1", "\r\n=1+1", "＝1+1", "＋123", "－1", "＠SUM(A1)"} {
		if got := spreadsheetText(value); got != "\t"+value {
			t.Errorf("%q => %q", value, got)
		}
	}
	for _, value := range []string{"Ada", "1+1", "a@example.com", "", "quoted \"text\""} {
		if got := spreadsheetText(value); got != value {
			t.Errorf("changed safe text %q", value)
		}
	}
}

func TestPublicInquiryCSVFormulaIsNeutralized(t *testing.T) {
	_, mux, ws := newTestModule(t)
	performJSON[map[string]string](t, mux, http.MethodPost, "/api/v1/intake/contact", `{"name":"=1+1","email":"formula@example.com","phone":"+15551234567"}`, http.StatusCreated)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/exports/contacts", nil))
	rows, err := csv.NewReader(strings.NewReader(w.Body.String())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if rows[1][0] != "\t=1+1" || rows[1][3] != "\t+15551234567" {
		t.Fatalf("unsafe export: %q", rows[1])
	}
	contacts, _ := ws.ListContacts(context.Background(), "nerds-who-fish")
	if contacts[0].Name != "=1+1" {
		t.Fatal("stored data modified")
	}
}

type failingInquiryWorkspace struct{ Workspace }

func (s failingInquiryWorkspace) CreateActivity(context.Context, string, workspace.Activity) (workspace.Activity, error) {
	return workspace.Activity{}, errors.New("unavailable")
}

func TestEveryPublicInquiryPersistsWithoutDuplicatingContact(t *testing.T) {
	_, mux, ws := newTestModule(t)
	for i, message := range []string{"First inquiry\nwith line breaks", "Another inquiry"} {
		w := httptest.NewRecorder()
		body := fmt.Sprintf(`{"name":"Ada","email":"ada@example.com","company":"River Labs","phone":"+15551234567","message":%q}`, message)
		mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/intake/contact", strings.NewReader(body)))
		if w.Code != http.StatusCreated && w.Code != http.StatusAccepted {
			t.Fatalf("inquiry %d status=%d: %s", i, w.Code, w.Body.String())
		}
	}
	contacts, _ := ws.ListContacts(context.Background(), "nerds-who-fish")
	activities, _ := ws.ListActivities(context.Background(), "nerds-who-fish")
	if len(contacts) != 1 || len(activities) != 2 {
		t.Fatalf("contacts=%d activities=%d", len(contacts), len(activities))
	}
	if !strings.Contains(activities[0].Body, "Another inquiry") || !strings.Contains(activities[1].Body, "First inquiry\nwith line breaks") {
		t.Fatalf("messages=%+v", activities)
	}
	for _, activity := range activities {
		if activity.ContactID != contacts[0].ID || !strings.Contains(activity.Body, "River Labs") {
			t.Fatalf("incomplete inquiry: %+v", activity)
		}
	}
}

func TestInquiryFailureIsRetryableForExistingContact(t *testing.T) {
	m, mux, ws := newTestModule(t)
	m.workspace = failingInquiryWorkspace{ws}
	body := `{"name":"Ada","email":"ada@example.com","message":"Retain this inquiry"}`
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/intake/contact", strings.NewReader(body)))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", w.Code)
	}
	m.workspace = ws
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/intake/contact", strings.NewReader(body)))
	if w.Code != http.StatusAccepted {
		t.Fatalf("retry status=%d", w.Code)
	}
	activities, _ := ws.ListActivities(context.Background(), m.publicScope)
	if len(activities) != 1 || !strings.Contains(activities[0].Body, "Retain this inquiry") {
		t.Fatalf("activities=%+v", activities)
	}
}
