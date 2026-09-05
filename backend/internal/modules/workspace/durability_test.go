package workspace

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

func durableStores(t *testing.T, check func(*testing.T, Store, string)) {
	t.Helper()
	t.Run("memory", func(t *testing.T) { check(t, NewMemoryStore(), "durability") })
	t.Run("firestore", func(t *testing.T) {
		if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
			t.Skip("FIRESTORE_EMULATOR_HOST is not configured")
		}
		client, err := firestore.NewClient(context.Background(), "kosmos-durability-tests")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = client.Close() })
		check(t, NewFirestoreStore(client), fmt.Sprintf("scope-%d", time.Now().UnixNano()))
	})
}

func TestContactMutationsSurviveContactAndAccountDeletion(t *testing.T) {
	durableStores(t, func(t *testing.T, store Store, scope string) {
		ctx := context.Background()
		outbox := store.(ContactMutationStore)
		account, primary, err := store.CreateAccountWithContact(ctx, scope, Account{Name: "River"}, Contact{Name: "Primary"})
		if err != nil {
			t.Fatal(err)
		}
		contact, err := store.CreateContact(ctx, scope, Contact{Name: "Second", AccountID: account.ID})
		if err != nil {
			t.Fatal(err)
		}
		name := "Updated"
		updated, err := store.UpdateContact(ctx, scope, contact.ID, ContactPatch{Name: &name})
		if err != nil {
			t.Fatal(err)
		}
		if err := store.DeleteContact(ctx, scope, contact.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := store.DeleteAccount(ctx, scope, account.ID); err != nil {
			t.Fatal(err)
		}
		items, err := outbox.ListContactMutations(ctx, scope)
		if err != nil {
			t.Fatal(err)
		}
		want := []ContactMutation{NewContactMutation(primary, "upsert"), NewContactMutation(contact, "upsert"), NewContactMutation(updated, "upsert"), NewContactMutation(updated, "delete"), NewContactMutation(primary, "delete")}
		if len(items) != len(want) {
			t.Fatalf("outbox entries = %d, want %d: %#v", len(items), len(want), items)
		}
		for _, mutation := range want {
			item, found, err := outbox.GetContactMutation(ctx, scope, mutation.ID)
			if err != nil || !found || item.ContactID != mutation.ContactID || item.Action != mutation.Action {
				t.Fatalf("missing durable mutation %#v: %#v %v %v", mutation, item, found, err)
			}
			if err := outbox.CompleteContactMutation(ctx, scope, mutation.ID); err != nil {
				t.Fatal(err)
			}
			if err := outbox.CompleteContactMutation(ctx, scope, mutation.ID); err != nil {
				t.Fatal(err)
			}
		}
		items, err = outbox.ListContactMutations(ctx, scope)
		if err != nil || len(items) != 0 {
			t.Fatalf("completed outbox = %#v, %v", items, err)
		}
	})
}

func TestConcurrentDocumentSavesPreserveEveryRevision(t *testing.T) {
	durableStores(t, func(t *testing.T, store Store, scope string) {
		ctx := context.Background()
		doc, err := store.CreateDocument(ctx, scope, Document{Title: "Original", Body: "first"})
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		start := make(chan struct{})
		for _, body := range []string{"second", "third"} {
			wg.Add(1)
			go func(body string) {
				defer wg.Done()
				<-start
				if _, err := store.UpdateDocument(ctx, scope, doc.ID, DocumentPatch{Body: &body}); err != nil {
					t.Errorf("save: %v", err)
				}
			}(body)
		}
		close(start)
		wg.Wait()
		revisions, err := store.ListDocumentRevisions(ctx, scope, doc.ID)
		if err != nil {
			t.Fatal(err)
		}
		seen := make(map[int]string)
		for _, revision := range revisions {
			seen[revision.Revision] = revision.Body
		}
		if len(revisions) != 2 || seen[1] != "first" || (seen[2] != "second" && seen[2] != "third") {
			t.Fatalf("history lost a committed version: %#v", revisions)
		}
		docs, err := store.ListDocuments(ctx, scope)
		if err != nil || len(docs) != 1 || docs[0].Revision != 3 || docs[0].Body == seen[2] {
			t.Fatalf("current document = %#v, %v", docs, err)
		}
	})
}

func TestStaleDocumentSaveDoesNotChangeHistory(t *testing.T) {
	durableStores(t, func(t *testing.T, store Store, scope string) {
		ctx := context.Background()
		doc, err := store.CreateDocument(ctx, scope, Document{Title: "Original", Body: "first"})
		if err != nil {
			t.Fatal(err)
		}
		body := "second"
		if _, err := store.UpdateDocument(ctx, scope, doc.ID, DocumentPatch{Body: &body, ExpectedRevision: &doc.Revision}); err != nil {
			t.Fatal(err)
		}
		body = "stale"
		if _, err := store.UpdateDocument(ctx, scope, doc.ID, DocumentPatch{Body: &body, ExpectedRevision: &doc.Revision}); !errors.Is(err, ErrDocumentConflict) {
			t.Fatalf("stale save = %v", err)
		}
		revisions, err := store.ListDocumentRevisions(ctx, scope, doc.ID)
		if err != nil || len(revisions) != 1 {
			t.Fatalf("rejected save changed history: %#v %v", revisions, err)
		}
		mux := http.NewServeMux()
		NewModule(store, func(*http.Request) (string, error) { return scope, nil }).RegisterRoutes(mux)
		performJSON[map[string]any](t, mux, http.MethodPatch, "/api/v1/documents/"+doc.ID, `{"body":"stale","expectedRevision":1}`, http.StatusConflict)
	})
}
