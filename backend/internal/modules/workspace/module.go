package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	platformmodules "github.com/NerdsWhoFish/kosmos/backend/internal/platform/modules"
	"github.com/NerdsWhoFish/kosmos/backend/internal/platform/pagination"
)

func managedDocumentID(sourceKey string) string {
	hash := sha256.Sum256([]byte("managed-document:" + sourceKey))
	return "managed-" + hex.EncodeToString(hash[:12])
}

type ScopeFunc func(*http.Request) (string, error)
type ActorFunc func(*http.Request) string
type ContactMutationFunc func(context.Context, string, Contact, string) error
type CostDeletionFunc func(context.Context, string, string) error

type Module struct {
	store           Store
	scope           ScopeFunc
	actor           ActorFunc
	contactMutation ContactMutationFunc
	costDeletion    CostDeletionFunc
}

type ModuleOption func(*Module)

func WithContactMutation(handler ContactMutationFunc) ModuleOption {
	return func(module *Module) { module.contactMutation = handler }
}

func WithActor(handler ActorFunc) ModuleOption {
	return func(module *Module) { module.actor = handler }
}

func WithCostDeletion(handler CostDeletionFunc) ModuleOption {
	return func(module *Module) { module.costDeletion = handler }
}

func NewModule(store Store, scope ScopeFunc, options ...ModuleOption) Module {
	module := Module{store: store, scope: scope, actor: func(*http.Request) string { return "workspace" }}
	for _, option := range options {
		option(&module)
	}
	return module
}

func (Module) Name() string { return "workspace" }

func (Module) Manifest() platformmodules.Manifest {
	return platformmodules.Manifest{Name: "workspace", Navigation: []platformmodules.Navigation{{Path: "/contacts", Label: "Contacts", Icon: "contacts"}, {Path: "/accounts", Label: "Accounts", Icon: "accounts"}, {Path: "/opportunities", Label: "Opportunities", Icon: "pipeline"}, {Path: "/documents", Label: "Documents", Icon: "documents"}}, Permissions: []string{"workspace.read", "workspace.write"}, Resources: []string{"accounts", "contacts", "leads", "opportunities", "activities", "reminders", "documents", "costs"}, EventTypes: []string{"contact.created", "opportunity.changed", "reminder.created", "activity.created", "cost.created"}, SearchProviders: []string{"workspace"}, DocumentLinkTargets: []string{"account", "contact", "opportunity", "cost", "document"}}
}

type Contact struct {
	ID          string    `json:"id" firestore:"-"`
	AccountID   string    `json:"accountId" firestore:"accountId"`
	Name        string    `json:"name" firestore:"name"`
	Email       string    `json:"email" firestore:"email"`
	Phone       string    `json:"phone" firestore:"phone"`
	LinkedInURL string    `json:"linkedinUrl" firestore:"linkedinUrl"`
	Source      string    `json:"source" firestore:"source"`
	CreatedAt   time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt" firestore:"updatedAt"`
}

type ContactSource struct {
	ID        string    `json:"id" firestore:"-"`
	Name      string    `json:"name" firestore:"name"`
	CreatedAt time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" firestore:"updatedAt"`
}

type Website struct {
	URL         string `json:"url" firestore:"url"`
	Domain      string `json:"domain" firestore:"domain"`
	Provider    string `json:"provider,omitempty" firestore:"provider,omitempty"`
	ExternalID  string `json:"externalId,omitempty" firestore:"externalId,omitempty"`
	RenewalDate string `json:"renewalDate,omitempty" firestore:"renewalDate,omitempty"`
	AutoRenew   bool   `json:"autoRenew" firestore:"autoRenew"`
	Status      string `json:"status,omitempty" firestore:"status,omitempty"`
}

type AccountLink struct {
	Label string `json:"label" firestore:"label"`
	URL   string `json:"url" firestore:"url"`
}

type Account struct {
	ID           string        `json:"id" firestore:"-"`
	Name         string        `json:"name" firestore:"name"`
	Website      string        `json:"website,omitempty" firestore:"website,omitempty"`
	Websites     []Website     `json:"websites" firestore:"websites"`
	Links        []AccountLink `json:"links" firestore:"links"`
	BillingEmail string        `json:"billingEmail" firestore:"billingEmail"`
	Status       string        `json:"status" firestore:"status"`
	Notes        string        `json:"notes" firestore:"notes"`
	CreatedAt    time.Time     `json:"createdAt" firestore:"createdAt"`
	UpdatedAt    time.Time     `json:"updatedAt" firestore:"updatedAt"`
}

type Opportunity struct {
	ID          string    `json:"id" firestore:"-"`
	Name        string    `json:"name" firestore:"name"`
	AccountID   string    `json:"accountId" firestore:"accountId"`
	ContactID   string    `json:"contactId" firestore:"contactId"`
	AmountCents int64     `json:"amountCents" firestore:"amountCents"`
	Stage       string    `json:"stage" firestore:"stage"`
	NextStep    string    `json:"nextStep" firestore:"nextStep"`
	CloseDate   string    `json:"closeDate" firestore:"closeDate"`
	OwnerEmail  string    `json:"ownerEmail" firestore:"ownerEmail"`
	CreatedAt   time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt" firestore:"updatedAt"`
}

type Activity struct {
	ID            string    `json:"id" firestore:"-"`
	ContactID     string    `json:"contactId" firestore:"contactId"`
	OpportunityID string    `json:"opportunityId" firestore:"opportunityId"`
	Kind          string    `json:"kind" firestore:"kind"`
	Body          string    `json:"body" firestore:"body"`
	OccurredAt    time.Time `json:"occurredAt" firestore:"occurredAt"`
	CreatedAt     time.Time `json:"createdAt" firestore:"createdAt"`
}

type AccountEvent struct {
	ID         string    `json:"id" firestore:"-"`
	AccountID  string    `json:"accountId" firestore:"accountId"`
	Kind       string    `json:"kind" firestore:"kind"`
	Action     string    `json:"action" firestore:"action"`
	Title      string    `json:"title" firestore:"title"`
	Summary    string    `json:"summary" firestore:"summary"`
	Actor      string    `json:"actor" firestore:"actor"`
	EntityType string    `json:"entityType" firestore:"entityType"`
	EntityID   string    `json:"entityId" firestore:"entityId"`
	OccurredAt time.Time `json:"occurredAt" firestore:"occurredAt"`
	CreatedAt  time.Time `json:"createdAt" firestore:"createdAt"`
}

type Reminder struct {
	ID         string    `json:"id" firestore:"-"`
	AccountID  string    `json:"accountId,omitempty" firestore:"accountId,omitempty"`
	ContactID  string    `json:"contactId" firestore:"contactId"`
	SourceKey  string    `json:"sourceKey,omitempty" firestore:"sourceKey,omitempty"`
	Title      string    `json:"title" firestore:"title"`
	DueAt      time.Time `json:"dueAt" firestore:"dueAt"`
	Completed  bool      `json:"completed" firestore:"completed"`
	OwnerEmail string    `json:"ownerEmail" firestore:"ownerEmail"`
	CreatedAt  time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt" firestore:"updatedAt"`
}

type Document struct {
	ID        string       `json:"id" firestore:"-"`
	SourceKey string       `json:"sourceKey,omitempty" firestore:"sourceKey,omitempty"`
	Title     string       `json:"title" firestore:"title"`
	Body      string       `json:"body" firestore:"body"`
	Links     []RecordLink `json:"links" firestore:"links"`
	Revision  int          `json:"revision" firestore:"revision"`
	CreatedAt time.Time    `json:"createdAt" firestore:"createdAt"`
	UpdatedAt time.Time    `json:"updatedAt" firestore:"updatedAt"`
}

type RecordLink struct {
	Type string `json:"type" firestore:"type"`
	ID   string `json:"id" firestore:"id"`
}

type DocumentRevision struct {
	ID         string       `json:"id" firestore:"-"`
	DocumentID string       `json:"documentId" firestore:"documentId"`
	Title      string       `json:"title" firestore:"title"`
	Body       string       `json:"body" firestore:"body"`
	Links      []RecordLink `json:"links" firestore:"links"`
	Revision   int          `json:"revision" firestore:"revision"`
	CreatedAt  time.Time    `json:"createdAt" firestore:"createdAt"`
}

type Cost struct {
	ID            string    `json:"id" firestore:"-"`
	Vendor        string    `json:"vendor" firestore:"vendor"`
	Description   string    `json:"description" firestore:"description"`
	AmountCents   int64     `json:"amountCents" firestore:"amountCents"`
	Category      string    `json:"category" firestore:"category"`
	IncurredOn    string    `json:"incurredOn" firestore:"incurredOn"`
	Recurring     bool      `json:"recurring" firestore:"recurring"`
	Recurrence    string    `json:"recurrence" firestore:"recurrence"`
	TaxDeductible bool      `json:"taxDeductible" firestore:"taxDeductible"`
	Notes         string    `json:"notes" firestore:"notes"`
	RenewalDate   string    `json:"renewalDate" firestore:"renewalDate"`
	PaymentMethod string    `json:"paymentMethod" firestore:"paymentMethod"`
	ReviewState   string    `json:"reviewState" firestore:"reviewState"`
	CreatedAt     time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt" firestore:"updatedAt"`
}

type ContactPatch struct {
	AccountID   *string `json:"accountId"`
	Name        *string `json:"name"`
	Email       *string `json:"email"`
	Phone       *string `json:"phone"`
	LinkedInURL *string `json:"linkedinUrl"`
	Source      *string `json:"source"`
}

type AccountPatch struct {
	Name         *string        `json:"name"`
	Websites     *[]Website     `json:"websites"`
	Links        *[]AccountLink `json:"links"`
	BillingEmail *string        `json:"billingEmail"`
	Status       *string        `json:"status"`
	Notes        *string        `json:"notes"`
}

type OpportunityPatch struct {
	Name        *string `json:"name"`
	AccountID   *string `json:"accountId"`
	ContactID   *string `json:"contactId"`
	AmountCents *int64  `json:"amountCents"`
	Stage       *string `json:"stage"`
	NextStep    *string `json:"nextStep"`
	CloseDate   *string `json:"closeDate"`
	OwnerEmail  *string `json:"ownerEmail"`
}

type DocumentPatch struct {
	Title *string       `json:"title"`
	Body  *string       `json:"body"`
	Links *[]RecordLink `json:"links"`
}

type ReminderPatch struct {
	Completed  *bool   `json:"completed"`
	OwnerEmail *string `json:"ownerEmail"`
}

type CostPatch struct {
	Vendor        *string `json:"vendor"`
	Description   *string `json:"description"`
	AmountCents   *int64  `json:"amountCents"`
	Category      *string `json:"category"`
	IncurredOn    *string `json:"incurredOn"`
	Recurring     *bool   `json:"recurring"`
	Recurrence    *string `json:"recurrence"`
	TaxDeductible *bool   `json:"taxDeductible"`
	Notes         *string `json:"notes"`
	RenewalDate   *string `json:"renewalDate"`
	PaymentMethod *string `json:"paymentMethod"`
	ReviewState   *string `json:"reviewState"`
}

type Store interface {
	ListPage(context.Context, string, string, pagination.Request, pagination.Spec, any) (pagination.Metadata, error)
	ListAccounts(context.Context, string) ([]Account, error)
	GetAccount(context.Context, string, string) (Account, error)
	CreateAccount(context.Context, string, Account) (Account, error)
	CreateAccountWithContact(context.Context, string, Account, Contact) (Account, Contact, error)
	UpdateAccount(context.Context, string, string, AccountPatch) (Account, error)
	DeleteAccount(context.Context, string, string) ([]Contact, error)
	CreateAccountEvent(context.Context, string, AccountEvent) (AccountEvent, error)
	ListAccountEventsPage(context.Context, string, string, pagination.Request, string) ([]AccountEvent, pagination.Metadata, error)
	LinkWebsiteRenewal(context.Context, string, string, Website, []Reminder) (Account, []Reminder, error)
	ListContacts(context.Context, string) ([]Contact, error)
	GetContact(context.Context, string, string) (Contact, error)
	CreateContact(context.Context, string, Contact) (Contact, error)
	UpdateContact(context.Context, string, string, ContactPatch) (Contact, error)
	DeleteContact(context.Context, string, string) error
	ListContactSources(context.Context, string) ([]ContactSource, error)
	CreateContactSource(context.Context, string, ContactSource) (ContactSource, error)
	ListOpportunities(context.Context, string) ([]Opportunity, error)
	GetOpportunity(context.Context, string, string) (Opportunity, error)
	CreateOpportunity(context.Context, string, Opportunity) (Opportunity, error)
	UpdateOpportunity(context.Context, string, string, OpportunityPatch) (Opportunity, error)
	DeleteOpportunity(context.Context, string, string) error
	ListActivities(context.Context, string) ([]Activity, error)
	CreateActivity(context.Context, string, Activity) (Activity, error)
	ListReminders(context.Context, string) ([]Reminder, error)
	CreateReminder(context.Context, string, Reminder) (Reminder, error)
	UpdateReminder(context.Context, string, string, ReminderPatch) (Reminder, error)
	ListDocuments(context.Context, string) ([]Document, error)
	CreateDocument(context.Context, string, Document) (Document, error)
	SyncManagedDocument(context.Context, string, string, Document) (Document, bool, error)
	UpdateDocument(context.Context, string, string, DocumentPatch) (Document, error)
	DeleteDocument(context.Context, string, string) error
	ListDocumentRevisions(context.Context, string, string) ([]DocumentRevision, error)
	CreateDocumentRevision(context.Context, string, DocumentRevision) (DocumentRevision, error)
	ListCosts(context.Context, string) ([]Cost, error)
	GetCost(context.Context, string, string) (Cost, error)
	CreateCost(context.Context, string, Cost) (Cost, error)
	UpdateCost(context.Context, string, string, CostPatch) (Cost, error)
	DeleteCost(context.Context, string, string) error
}
