package workspace

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/firestore/apiv1/firestorepb"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

func TestFirestoreRejectsContactMutationAndOutboxTogether(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST is not configured")
	}
	ctx := context.Background()
	var reject atomic.Bool
	intercept := func(ctx context.Context, method string, request, reply any, conn *grpc.ClientConn, invoke grpc.UnaryInvoker, options ...grpc.CallOption) error {
		if commit, ok := request.(*firestorepb.CommitRequest); ok && reject.Load() {
			for _, write := range commit.Writes {
				if strings.Contains(write.GetUpdate().GetName(), "/contactMutationOutbox/") || strings.Contains(write.GetUpdate().GetName(), "/documentRevisions/") {
					return status.Error(codes.PermissionDenied, "injected durable-write failure")
				}
			}
		}
		return invoke(ctx, method, request, reply, conn, options...)
	}
	address := os.Getenv("FIRESTORE_EMULATOR_HOST")
	t.Setenv("FIRESTORE_EMULATOR_HOST", "")
	conn, err := grpc.NewClient("passthrough:///"+address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithUnaryInterceptor(intercept))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client, err := firestore.NewClient(ctx, "kosmos-atomicity-tests", option.WithGRPCConn(conn))
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	store := NewFirestoreStore(client)
	scope := fmt.Sprintf("atomicity-%d", time.Now().UnixNano())
	reject.Store(true)
	if _, err := store.CreateContact(ctx, scope, Contact{Name: "Rejected"}); err == nil {
		t.Fatal("injected commit failure was ignored")
	}
	contacts, err := store.ListContacts(ctx, scope)
	if err != nil || len(contacts) != 0 {
		t.Fatalf("contact committed without its outbox: %#v %v", contacts, err)
	}
	if _, _, err := store.CreateAccountWithContact(ctx, scope, Account{Name: "Rejected account"}, Contact{Name: "Primary"}); err == nil {
		t.Fatal("account mutation failure was ignored")
	}
	accounts, err := store.ListAccounts(ctx, scope)
	if err != nil || len(accounts) != 0 {
		t.Fatalf("account committed without its contact outbox: %#v %v", accounts, err)
	}
	reject.Store(false)
	contact, err := store.CreateContact(ctx, scope, Contact{Name: "Original"})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := store.CreateDocument(ctx, scope, Document{Title: "Original", Body: "first"})
	if err != nil {
		t.Fatal(err)
	}
	reject.Store(true)
	name := "Rejected update"
	if _, err := store.UpdateContact(ctx, scope, contact.ID, ContactPatch{Name: &name}); err == nil {
		t.Fatal("update failure was ignored")
	}
	if err := store.DeleteContact(ctx, scope, contact.ID); err == nil {
		t.Fatal("delete failure was ignored")
	}
	actual, err := store.GetContact(ctx, scope, contact.ID)
	if err != nil || actual.Name != "Original" {
		t.Fatalf("failed outbox write changed contact: %#v %v", actual, err)
	}
	body := "second"
	if _, err := store.UpdateDocument(ctx, scope, doc.ID, DocumentPatch{Body: &body}); err == nil {
		t.Fatal("history failure was ignored")
	}
	docs, err := store.ListDocuments(ctx, scope)
	if err != nil || len(docs) != 1 || docs[0].Body != "first" || docs[0].Revision != 1 {
		t.Fatalf("document committed without its history: %#v %v", docs, err)
	}
}
