package workspace

import (
	"context"
	"errors"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/firestorepage"
	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/pagination"
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

func (s *FirestoreStore) ListPage(ctx context.Context, scope, collection string, request pagination.Request, spec pagination.Spec, target any) (pagination.Metadata, error) {
	return firestorepage.List(ctx, s.collection(scope, collection), request, spec, target)
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

func deleteRecord(ctx context.Context, store *FirestoreStore, scope, collection, id string) error {
	ctx, span := otel.Tracer("github.com/NerdsWhoFish/kosmos/workspace").Start(ctx, "firestore.delete")
	span.SetAttributes(attribute.String("db.collection.name", collection))
	defer span.End()
	document := store.collection(scope, collection).Doc(id)
	if _, err := document.Get(ctx); status.Code(err) == codes.NotFound {
		return errNotFound
	} else if err != nil {
		span.RecordError(err)
		return err
	}
	if _, err := document.Delete(ctx); err != nil {
		span.RecordError(err)
		return err
	}
	return nil
}

func (s *FirestoreStore) ListAccounts(ctx context.Context, scope string) ([]Account, error) {
	return listRecords(ctx, s, scope, "accounts", "updatedAt", firestore.Desc, func(item *Account, id string) { item.ID = id })
}

func (s *FirestoreStore) GetAccount(ctx context.Context, scope, id string) (Account, error) {
	return getRecord(ctx, s, scope, "accounts", id, func(item *Account, id string) { item.ID = id })
}

func (s *FirestoreStore) CreateAccount(ctx context.Context, scope string, item Account) (Account, error) {
	return createRecord(ctx, s, scope, "accounts", item, func(item *Account, id string, now time.Time) {
		item.ID, item.CreatedAt, item.UpdatedAt = id, now, now
	})
}

func (s *FirestoreStore) CreateAccountWithContact(ctx context.Context, scope string, account Account, contact Contact) (Account, Contact, error) {
	accountID, err := newID()
	if err != nil {
		return Account{}, Contact{}, err
	}
	contactID, err := newID()
	if err != nil {
		return Account{}, Contact{}, err
	}
	now := time.Now().UTC()
	account.ID, account.CreatedAt, account.UpdatedAt = accountID, now, now
	contact.ID, contact.AccountID, contact.CreatedAt, contact.UpdatedAt = contactID, accountID, now, now
	err = s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		if err := tx.Create(s.collection(scope, "accounts").Doc(accountID), account); err != nil {
			return err
		}
		return tx.Create(s.collection(scope, "contacts").Doc(contactID), contact)
	})
	if err != nil {
		return Account{}, Contact{}, err
	}
	return account, contact, nil
}

func (s *FirestoreStore) UpdateAccount(ctx context.Context, scope, id string, patch AccountPatch) (Account, error) {
	updates := make([]firestore.Update, 0, 7)
	if patch.Name != nil {
		updates = append(updates, firestore.Update{Path: "name", Value: *patch.Name})
	}
	if patch.Websites != nil {
		existing, err := s.GetAccount(ctx, scope, id)
		if err != nil {
			return Account{}, err
		}
		websites := preserveManagedWebsiteMetadata(accountWebsites(existing), *patch.Websites)
		legacyWebsite := ""
		if len(websites) > 0 {
			legacyWebsite = websites[0].URL
		}
		updates = append(updates, firestore.Update{Path: "websites", Value: websites}, firestore.Update{Path: "website", Value: legacyWebsite})
	}
	if patch.Links != nil {
		updates = append(updates, firestore.Update{Path: "links", Value: *patch.Links})
	}
	if patch.BillingEmail != nil {
		updates = append(updates, firestore.Update{Path: "billingEmail", Value: *patch.BillingEmail})
	}
	if patch.Status != nil {
		updates = append(updates, firestore.Update{Path: "status", Value: *patch.Status})
	}
	if patch.Notes != nil {
		updates = append(updates, firestore.Update{Path: "notes", Value: *patch.Notes})
	}
	return updateRecord(ctx, s, scope, "accounts", id, updates, func(item *Account, id string) { item.ID = id })
}

func (s *FirestoreStore) DeleteAccount(ctx context.Context, scope, id string) ([]Contact, error) {
	accountRef := s.collection(scope, "accounts").Doc(id)
	if _, err := accountRef.Get(ctx); status.Code(err) == codes.NotFound {
		return nil, errNotFound
	} else if err != nil {
		return nil, err
	}
	contacts, err := s.ListContacts(ctx, scope)
	if err != nil {
		return nil, err
	}
	opportunities, err := s.ListOpportunities(ctx, scope)
	if err != nil {
		return nil, err
	}
	activities, err := s.ListActivities(ctx, scope)
	if err != nil {
		return nil, err
	}
	reminders, err := s.ListReminders(ctx, scope)
	if err != nil {
		return nil, err
	}
	documents, err := s.ListDocuments(ctx, scope)
	if err != nil {
		return nil, err
	}
	revisions, err := listRecords(ctx, s, scope, "documentRevisions", "createdAt", firestore.Desc, func(item *DocumentRevision, documentID string) { item.ID = documentID })
	if err != nil {
		return nil, err
	}

	deletedContacts := make([]Contact, 0)
	contactIDs := make(map[string]struct{})
	deleteRefs := make([]*firestore.DocumentRef, 0)
	for _, contact := range contacts {
		if contact.AccountID == id {
			deletedContacts = append(deletedContacts, contact)
			contactIDs[contact.ID] = struct{}{}
			deleteRefs = append(deleteRefs, s.collection(scope, "contacts").Doc(contact.ID))
		}
	}
	opportunityIDs := make(map[string]struct{})
	for _, opportunity := range opportunities {
		if opportunity.AccountID == id {
			opportunityIDs[opportunity.ID] = struct{}{}
			deleteRefs = append(deleteRefs, s.collection(scope, "opportunities").Doc(opportunity.ID))
		}
	}
	for _, activity := range activities {
		_, contactDeleted := contactIDs[activity.ContactID]
		_, opportunityDeleted := opportunityIDs[activity.OpportunityID]
		if contactDeleted || opportunityDeleted {
			deleteRefs = append(deleteRefs, s.collection(scope, "activities").Doc(activity.ID))
		}
	}
	for _, reminder := range reminders {
		_, contactDeleted := contactIDs[reminder.ContactID]
		if reminder.AccountID == id || contactDeleted {
			deleteRefs = append(deleteRefs, s.collection(scope, "reminders").Doc(reminder.ID))
		}
	}

	type documentUpdate struct {
		ref   *firestore.DocumentRef
		links []RecordLink
	}
	updates := make([]documentUpdate, 0)
	deletedDocuments := make(map[string]struct{})
	for _, document := range documents {
		links := remainingAccountLinks(document.Links, id, contactIDs, opportunityIDs)
		if len(links) == len(document.Links) {
			continue
		}
		ref := s.collection(scope, "documents").Doc(document.ID)
		if len(links) == 0 {
			deletedDocuments[document.ID] = struct{}{}
			deleteRefs = append(deleteRefs, ref)
			continue
		}
		updates = append(updates, documentUpdate{ref: ref, links: links})
	}
	for _, revision := range revisions {
		if _, deleted := deletedDocuments[revision.DocumentID]; deleted {
			deleteRefs = append(deleteRefs, s.collection(scope, "documentRevisions").Doc(revision.ID))
		}
	}

	const batchLimit = 450
	writeCount := 0
	batch := s.client.Batch()
	commit := func() error {
		if writeCount == 0 {
			return nil
		}
		if _, err := batch.Commit(ctx); err != nil {
			return err
		}
		batch = s.client.Batch()
		writeCount = 0
		return nil
	}
	for _, update := range updates {
		batch.Update(update.ref, []firestore.Update{{Path: "links", Value: update.links}, {Path: "updatedAt", Value: time.Now().UTC()}})
		writeCount++
		if writeCount == batchLimit {
			if err := commit(); err != nil {
				return nil, err
			}
		}
	}
	for _, ref := range deleteRefs {
		batch.Delete(ref)
		writeCount++
		if writeCount == batchLimit {
			if err := commit(); err != nil {
				return nil, err
			}
		}
	}
	batch.Delete(accountRef)
	writeCount++
	if err := commit(); err != nil {
		return nil, err
	}
	return deletedContacts, nil
}

func (s *FirestoreStore) LinkWebsiteRenewal(ctx context.Context, scope, id string, website Website, reminders []Reminder) (Account, []Reminder, error) {
	accountRef := s.collection(scope, "accounts").Doc(id)
	reminderCollection := s.collection(scope, "reminders")
	reminderRefs := make([]*firestore.DocumentRef, len(reminders))
	for index := range reminders {
		reminderRefs[index] = reminderCollection.Doc(reminders[index].ID)
	}
	var account Account
	result := make([]Reminder, len(reminders))
	err := s.client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		document, err := tx.Get(accountRef)
		if status.Code(err) == codes.NotFound {
			return errNotFound
		}
		if err != nil {
			return err
		}
		if err := document.DataTo(&account); err != nil {
			return err
		}
		existingReminders, err := tx.Documents(reminderCollection.Where("accountId", "==", id)).GetAll()
		if err != nil {
			return err
		}
		exists := make([]bool, len(reminders))
		for index, ref := range reminderRefs {
			document, err := tx.Get(ref)
			if status.Code(err) == codes.NotFound {
				continue
			}
			if err != nil {
				return err
			}
			if err := document.DataTo(&result[index]); err != nil {
				return err
			}
			result[index].ID = reminders[index].ID
			exists[index] = true
		}
		now := time.Now().UTC()
		account.ID = id
		account.Websites = mergeWebsite(accountWebsites(account), website)
		account.Website = account.Websites[0].URL
		account.UpdatedAt = now
		if err := tx.Update(accountRef, []firestore.Update{{Path: "websites", Value: account.Websites}, {Path: "website", Value: account.Website}, {Path: "updatedAt", Value: now}}); err != nil {
			return err
		}
		desired := make(map[string]struct{}, len(reminders))
		for _, reminder := range reminders {
			desired[reminder.ID] = struct{}{}
		}
		prefix := "cloudflare:" + website.Domain + ":"
		for _, document := range existingReminders {
			var reminder Reminder
			if err := document.DataTo(&reminder); err != nil {
				return err
			}
			_, current := desired[document.Ref.ID]
			if strings.HasPrefix(reminder.SourceKey, prefix) && !current {
				if err := tx.Delete(document.Ref); err != nil {
					return err
				}
			}
		}
		for index, reminder := range reminders {
			if exists[index] {
				continue
			}
			reminder.CreatedAt, reminder.UpdatedAt = now, now
			if err := tx.Create(reminderRefs[index], reminder); err != nil {
				return err
			}
			result[index] = reminder
		}
		return nil
	})
	if err != nil {
		return Account{}, nil, err
	}
	return account, result, nil
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
	updates := make([]firestore.Update, 0, 6)
	appendStringUpdate := func(path string, value *string) {
		if value != nil {
			updates = append(updates, firestore.Update{Path: path, Value: *value})
		}
	}
	appendStringUpdate("accountId", patch.AccountID)
	appendStringUpdate("name", patch.Name)
	appendStringUpdate("email", patch.Email)
	appendStringUpdate("phone", patch.Phone)
	appendStringUpdate("linkedinUrl", patch.LinkedInURL)
	appendStringUpdate("source", patch.Source)
	return updateRecord(ctx, s, scope, "contacts", id, updates, func(item *Contact, id string) { item.ID = id })
}

func (s *FirestoreStore) DeleteContact(ctx context.Context, scope, id string) error {
	_, err := s.collection(scope, "contacts").Doc(id).Delete(ctx, firestore.Exists)
	if status.Code(err) == codes.NotFound || status.Code(err) == codes.FailedPrecondition {
		return errNotFound
	}
	return err
}

func (s *FirestoreStore) ListContactSources(ctx context.Context, scope string) ([]ContactSource, error) {
	return listRecords(ctx, s, scope, "contactSources", "name", firestore.Asc, func(item *ContactSource, id string) { item.ID = id })
}

func (s *FirestoreStore) CreateContactSource(ctx context.Context, scope string, item ContactSource) (ContactSource, error) {
	return createRecord(ctx, s, scope, "contactSources", item, func(item *ContactSource, id string, now time.Time) {
		item.ID, item.CreatedAt, item.UpdatedAt = id, now, now
	})
}

func (s *FirestoreStore) ListOpportunities(ctx context.Context, scope string) ([]Opportunity, error) {
	return listRecords(ctx, s, scope, "opportunities", "updatedAt", firestore.Desc, func(item *Opportunity, id string) { item.ID = id })
}

func (s *FirestoreStore) GetOpportunity(ctx context.Context, scope, id string) (Opportunity, error) {
	return getRecord(ctx, s, scope, "opportunities", id, func(item *Opportunity, id string) { item.ID = id })
}

func (s *FirestoreStore) CreateOpportunity(ctx context.Context, scope string, item Opportunity) (Opportunity, error) {
	return createRecord(ctx, s, scope, "opportunities", item, func(item *Opportunity, id string, now time.Time) {
		item.ID, item.CreatedAt, item.UpdatedAt = id, now, now
	})
}

func (s *FirestoreStore) UpdateOpportunity(ctx context.Context, scope, id string, patch OpportunityPatch) (Opportunity, error) {
	updates := make([]firestore.Update, 0, 7)
	appendStringUpdate := func(path string, value *string) {
		if value != nil {
			updates = append(updates, firestore.Update{Path: path, Value: *value})
		}
	}
	appendStringUpdate("name", patch.Name)
	appendStringUpdate("accountId", patch.AccountID)
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

func (s *FirestoreStore) DeleteOpportunity(ctx context.Context, scope, id string) error {
	return deleteRecord(ctx, s, scope, "opportunities", id)
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

func (s *FirestoreStore) DeleteDocument(ctx context.Context, scope, id string) error {
	document := s.collection(scope, "documents").Doc(id)
	if _, err := document.Get(ctx); status.Code(err) == codes.NotFound {
		return errNotFound
	} else if err != nil {
		return err
	}
	batch := s.client.Batch()
	batch.Delete(document)
	iter := s.collection(scope, "documentRevisions").Where("documentId", "==", id).Documents(ctx)
	defer iter.Stop()
	for {
		revision, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return err
		}
		batch.Delete(revision.Ref)
	}
	_, err := batch.Commit(ctx)
	return err
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
