package operations

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
)

func TestMailCheckpointIsMonotonicAndPreservesConnection(t *testing.T) {
	factories := map[string]func(*testing.T) Store{
		"memory": func(*testing.T) Store { return NewMemoryStore() },
		"firestore": func(t *testing.T) Store {
			if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
				t.Skip("FIRESTORE_EMULATOR_HOST is not configured")
			}
			client, err := firestore.NewClient(context.Background(), "kosmos-checkpoint-tests")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = client.Close() })
			return NewFirestoreStore(client)
		},
	}
	for name, factory := range factories {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			store := factory(t)
			scope := fmt.Sprintf("checkpoint-%d", time.Now().UnixNano())
			connection := GoogleConnection{ID: "connection", EncryptedToken: "latest-rotated-token", Tiller: &TillerSettings{SpreadsheetID: "new-sheet", Range: "Sheet1!A:Z"}}
			if err := store.Put(ctx, scope, "googleConnections", connection.ID, connection); err != nil {
				t.Fatal(err)
			}
			newer := time.Now().UTC().Truncate(time.Microsecond)
			if err := store.AdvanceMailCheckpoint(ctx, scope, connection.ID, newer); err != nil {
				t.Fatal(err)
			}
			if err := store.AdvanceMailCheckpoint(ctx, scope, connection.ID, newer.Add(-time.Hour)); err != nil {
				t.Fatal(err)
			}
			var actual GoogleConnection
			if err := store.Get(ctx, scope, "googleConnections", connection.ID, &actual); err != nil {
				t.Fatal(err)
			}
			if actual.LastMailSyncAt == nil || !actual.LastMailSyncAt.Equal(newer) || actual.EncryptedToken != connection.EncryptedToken || actual.Tiller == nil || *actual.Tiller != *connection.Tiller {
				t.Fatalf("checkpoint changed connection or regressed: %#v", actual)
			}
			if err := store.Delete(ctx, scope, "googleConnections", connection.ID); err != nil {
				t.Fatal(err)
			}
			if err := store.AdvanceMailCheckpoint(ctx, scope, connection.ID, newer.Add(time.Hour)); !errors.Is(err, errNotFound) {
				t.Fatalf("checkpoint recreated disconnected account: %v", err)
			}
		})
	}
}
