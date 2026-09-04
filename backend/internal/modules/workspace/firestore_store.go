package workspace

import (
	"context"
	"errors"
	"time"

	"cloud.google.com/go/firestore"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type FirestoreStore struct {
	client *firestore.Client
}

func NewFirestoreStore(client *firestore.Client) *FirestoreStore {
	return &FirestoreStore{client: client}
}

func (s *FirestoreStore) collection(scope, name string) *firestore.CollectionRef {
	return s.client.Collection("organizations").Doc(scope).Collection(name)
}

func listRecords[T any](ctx context.Context, store *FirestoreStore, scope, collection, orderBy string, direction firestore.Direction, assignID func(*T, string)) ([]T, error) {
	ctx, span := otel.Tracer("github.com/NerdsWhoFish/kosmos/workspace").Start(ctx, "firestore.list")
	span.SetAttributes(attribute.String("db.collection.name", collection))
	defer span.End()

	iter := store.collection(scope, collection).OrderBy(orderBy, direction).Documents(ctx)
	defer iter.Stop()
	items := make([]T, 0)
	for {
		document, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			return items, nil
		}
		if err != nil {
			span.RecordError(err)
			return nil, err
		}
		var item T
		if err := document.DataTo(&item); err != nil {
			span.RecordError(err)
			return nil, err
		}
		assignID(&item, document.Ref.ID)
		items = append(items, item)
	}
}

func getRecord[T any](ctx context.Context, store *FirestoreStore, scope, collection, id string, assignID func(*T, string)) (T, error) {
	ctx, span := otel.Tracer("github.com/NerdsWhoFish/kosmos/workspace").Start(ctx, "firestore.get")
	span.SetAttributes(attribute.String("db.collection.name", collection))
	defer span.End()
	var item T
	document, err := store.collection(scope, collection).Doc(id).Get(ctx)
	if status.Code(err) == codes.NotFound {
		return item, errNotFound
	}
	if err != nil {
		span.RecordError(err)
		return item, err
	}
	if err := document.DataTo(&item); err != nil {
		span.RecordError(err)
		return item, err
	}
	assignID(&item, document.Ref.ID)
	return item, nil
}

func createRecord[T any](ctx context.Context, store *FirestoreStore, scope, collection string, item T, prepare func(*T, string, time.Time)) (T, error) {
	ctx, span := otel.Tracer("github.com/NerdsWhoFish/kosmos/workspace").Start(ctx, "firestore.create")
	span.SetAttributes(attribute.String("db.collection.name", collection))
	defer span.End()
	document := store.collection(scope, collection).NewDoc()
	prepare(&item, document.ID, time.Now().UTC())
	if _, err := document.Set(ctx, item); err != nil {
		span.RecordError(err)
		var empty T
		return empty, err
	}
	return item, nil
}

func updateRecord[T any](ctx context.Context, store *FirestoreStore, scope, collection, id string, updates []firestore.Update, assignID func(*T, string)) (T, error) {
	ctx, span := otel.Tracer("github.com/NerdsWhoFish/kosmos/workspace").Start(ctx, "firestore.update")
	span.SetAttributes(attribute.String("db.collection.name", collection))
	defer span.End()
	document := store.collection(scope, collection).Doc(id)
	updates = append(updates, firestore.Update{Path: "updatedAt", Value: time.Now().UTC()})
	if _, err := document.Update(ctx, updates); status.Code(err) == codes.NotFound {
		var empty T
		return empty, errNotFound
	} else if err != nil {
		span.RecordError(err)
		var empty T
		return empty, err
	}
	return getRecord(ctx, store, scope, collection, id, assignID)
}

func (s *FirestoreStore) ListAccounts(ctx context.Context, scope string) ([]Account, error) {
	return listRecords(ctx, s, scope, "accounts", "updatedAt", firestore.Desc, func(item *Account, id string) { item.ID = id })
}

func (s *FirestoreStore) CreateAccount(ctx context.Context, scope string, item Account) (Account, error) {
	return createRecord(ctx, s, scope, "accounts", item, func(item *Account, id string, now time.Time) {
		item.ID, item.CreatedAt, item.UpdatedAt = id, now, now
	})
}

func (s *FirestoreStore) ListContacts(ctx context.Context, scope string) ([]Contact, error) {
	return listRecords(ctx, s, scope, "contacts", "updatedAt", firestore.Desc, func(item *Contact, id string) { item.ID = id })
}

func (s *FirestoreStore) GetContact(ctx context.Context, scope, id string) (Contact, error) {
	return getRecord(ctx, s, scope, "contacts", id, func(item *Contact, id string) { item.ID = id })
}

func (s *FirestoreStore) CreateContact(ctx context.Context, scope string, item Contact) (Contact, error) {
	return createRecord(ctx, s, scope, "contacts", item, func(item *Contact, id string, now time.Time) {
		item.ID, item.CreatedAt, item.UpdatedAt = id, now, now
	})
}

func (s *FirestoreStore) UpdateContact(ctx context.Context, scope, id string, patch ContactPatch) (Contact, error) {
	updates := make([]firestore.Update, 0, 7)
	appendStringUpdate := func(path string, value *string) {
		if value != nil {
			updates = append(updates, firestore.Update{Path: path, Value: *value})
		}
	}
	appendStringUpdate("accountId", patch.AccountID)
	appendStringUpdate("name", patch.Name)
	appendStringUpdate("company", patch.Company)
	appendStringUpdate("email", patch.Email)
	appendStringUpdate("phone", patch.Phone)
	appendStringUpdate("status", patch.Status)
	appendStringUpdate("source", patch.Source)
	return updateRecord(ctx, s, scope, "contacts", id, updates, func(item *Contact, id string) { item.ID = id })
}

func (s *FirestoreStore) ListOpportunities(ctx context.Context, scope string) ([]Opportunity, error) {
	return listRecords(ctx, s, scope, "opportunities", "updatedAt", firestore.Desc, func(item *Opportunity, id string) { item.ID = id })
}

func (s *FirestoreStore) CreateOpportunity(ctx context.Context, scope string, item Opportunity) (Opportunity, error) {
	return createRecord(ctx, s, scope, "opportunities", item, func(item *Opportunity, id string, now time.Time) {
		item.ID, item.CreatedAt, item.UpdatedAt = id, now, now
	})
}

func (s *FirestoreStore) UpdateOpportunity(ctx context.Context, scope, id string, patch OpportunityPatch) (Opportunity, error) {
	updates := make([]firestore.Update, 0, 6)
	appendStringUpdate := func(path string, value *string) {
		if value != nil {
			updates = append(updates, firestore.Update{Path: path, Value: *value})
		}
	}
	appendStringUpdate("name", patch.Name)
	appendStringUpdate("contactId", patch.ContactID)
	if patch.AmountCents != nil {
		updates = append(updates, firestore.Update{Path: "amountCents", Value: *patch.AmountCents})
	}
	appendStringUpdate("stage", patch.Stage)
	appendStringUpdate("nextStep", patch.NextStep)
	appendStringUpdate("closeDate", patch.CloseDate)
	appendStringUpdate("ownerEmail", patch.OwnerEmail)
	return updateRecord(ctx, s, scope, "opportunities", id, updates, func(item *Opportunity, id string) { item.ID = id })
}

func (s *FirestoreStore) ListActivities(ctx context.Context, scope string) ([]Activity, error) {
	return listRecords(ctx, s, scope, "activities", "occurredAt", firestore.Desc, func(item *Activity, id string) { item.ID = id })
}

func (s *FirestoreStore) CreateActivity(ctx context.Context, scope string, item Activity) (Activity, error) {
	return createRecord(ctx, s, scope, "activities", item, func(item *Activity, id string, now time.Time) {
		item.ID, item.CreatedAt = id, now
	})
}

func (s *FirestoreStore) ListReminders(ctx context.Context, scope string) ([]Reminder, error) {
	return listRecords(ctx, s, scope, "reminders", "dueAt", firestore.Asc, func(item *Reminder, id string) { item.ID = id })
}

func (s *FirestoreStore) CreateReminder(ctx context.Context, scope string, item Reminder) (Reminder, error) {
	return createRecord(ctx, s, scope, "reminders", item, func(item *Reminder, id string, now time.Time) {
		item.ID, item.CreatedAt, item.UpdatedAt = id, now, now
	})
}

func (s *FirestoreStore) UpdateReminder(ctx context.Context, scope, id string, patch ReminderPatch) (Reminder, error) {
	updates := make([]firestore.Update, 0, 2)
	if patch.Completed != nil {
		updates = append(updates, firestore.Update{Path: "completed", Value: *patch.Completed})
	}
	if patch.OwnerEmail != nil {
		updates = append(updates, firestore.Update{Path: "ownerEmail", Value: *patch.OwnerEmail})
	}
	return updateRecord(ctx, s, scope, "reminders", id, updates, func(item *Reminder, id string) { item.ID = id })
}

func (s *FirestoreStore) ListDocuments(ctx context.Context, scope string) ([]Document, error) {
	return listRecords(ctx, s, scope, "documents", "updatedAt", firestore.Desc, func(item *Document, id string) { item.ID = id })
}

func (s *FirestoreStore) CreateDocument(ctx context.Context, scope string, item Document) (Document, error) {
	return createRecord(ctx, s, scope, "documents", item, func(item *Document, id string, now time.Time) {
		item.ID, item.CreatedAt, item.UpdatedAt = id, now, now
		item.Revision = 1
	})
}

func (s *FirestoreStore) UpdateDocument(ctx context.Context, scope, id string, patch DocumentPatch) (Document, error) {
	updates := make([]firestore.Update, 0, 2)
	if patch.Title != nil {
		updates = append(updates, firestore.Update{Path: "title", Value: *patch.Title})
	}
	if patch.Body != nil {
		updates = append(updates, firestore.Update{Path: "body", Value: *patch.Body})
	}
	if patch.Links != nil {
		updates = append(updates, firestore.Update{Path: "links", Value: *patch.Links})
	}
	updates = append(updates, firestore.Update{Path: "revision", Value: firestore.Increment(1)})
	return updateRecord(ctx, s, scope, "documents", id, updates, func(item *Document, id string) { item.ID = id })
}

func (s *FirestoreStore) ListDocumentRevisions(ctx context.Context, scope, documentID string) ([]DocumentRevision, error) {
	items, err := listRecords(ctx, s, scope, "documentRevisions", "createdAt", firestore.Desc, func(item *DocumentRevision, id string) { item.ID = id })
	if err != nil {
		return nil, err
	}
	filtered := make([]DocumentRevision, 0)
	for _, item := range items {
		if item.DocumentID == documentID {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (s *FirestoreStore) CreateDocumentRevision(ctx context.Context, scope string, item DocumentRevision) (DocumentRevision, error) {
	return createRecord(ctx, s, scope, "documentRevisions", item, func(item *DocumentRevision, id string, now time.Time) {
		item.ID, item.CreatedAt = id, now
	})
}

func (s *FirestoreStore) ListCosts(ctx context.Context, scope string) ([]Cost, error) {
	return listRecords(ctx, s, scope, "costs", "incurredOn", firestore.Desc, func(item *Cost, id string) { item.ID = id })
}

func (s *FirestoreStore) CreateCost(ctx context.Context, scope string, item Cost) (Cost, error) {
	return createRecord(ctx, s, scope, "costs", item, func(item *Cost, id string, now time.Time) {
		item.ID, item.CreatedAt, item.UpdatedAt = id, now, now
	})
}

func (s *FirestoreStore) UpdateCost(ctx context.Context, scope, id string, patch CostPatch) (Cost, error) {
	updates := make([]firestore.Update, 0, 12)
	addString := func(path string, value *string) {
		if value != nil {
			updates = append(updates, firestore.Update{Path: path, Value: *value})
		}
	}
	addBool := func(path string, value *bool) {
		if value != nil {
			updates = append(updates, firestore.Update{Path: path, Value: *value})
		}
	}
	addString("vendor", patch.Vendor)
	addString("description", patch.Description)
	if patch.AmountCents != nil {
		updates = append(updates, firestore.Update{Path: "amountCents", Value: *patch.AmountCents})
	}
	addString("category", patch.Category)
	addString("incurredOn", patch.IncurredOn)
	addBool("recurring", patch.Recurring)
	addString("recurrence", patch.Recurrence)
	addBool("taxDeductible", patch.TaxDeductible)
	addString("notes", patch.Notes)
	addString("renewalDate", patch.RenewalDate)
	addString("paymentMethod", patch.PaymentMethod)
	addString("reviewState", patch.ReviewState)
	return updateRecord(ctx, s, scope, "costs", id, updates, func(item *Cost, id string) { item.ID = id })
}
