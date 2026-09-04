package workspace

import (
	"context"
	"net/http"
	"time"
)

type ScopeFunc func(*http.Request) (string, error)

type Module struct {
	store Store
	scope ScopeFunc
}

func NewModule(store Store, scope ScopeFunc) Module {
	return Module{store: store, scope: scope}
}

func (Module) Name() string { return "workspace" }

type Contact struct {
	ID        string    `json:"id" firestore:"-"`
	Name      string    `json:"name" firestore:"name"`
	Company   string    `json:"company" firestore:"company"`
	Email     string    `json:"email" firestore:"email"`
	Phone     string    `json:"phone" firestore:"phone"`
	Status    string    `json:"status" firestore:"status"`
	CreatedAt time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" firestore:"updatedAt"`
}

type Opportunity struct {
	ID          string    `json:"id" firestore:"-"`
	Name        string    `json:"name" firestore:"name"`
	ContactID   string    `json:"contactId" firestore:"contactId"`
	AmountCents int64     `json:"amountCents" firestore:"amountCents"`
	Stage       string    `json:"stage" firestore:"stage"`
	NextStep    string    `json:"nextStep" firestore:"nextStep"`
	CloseDate   string    `json:"closeDate" firestore:"closeDate"`
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

type Reminder struct {
	ID        string    `json:"id" firestore:"-"`
	ContactID string    `json:"contactId" firestore:"contactId"`
	Title     string    `json:"title" firestore:"title"`
	DueAt     time.Time `json:"dueAt" firestore:"dueAt"`
	Completed bool      `json:"completed" firestore:"completed"`
	CreatedAt time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" firestore:"updatedAt"`
}

type Document struct {
	ID        string    `json:"id" firestore:"-"`
	Title     string    `json:"title" firestore:"title"`
	Body      string    `json:"body" firestore:"body"`
	CreatedAt time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" firestore:"updatedAt"`
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
	CreatedAt     time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt" firestore:"updatedAt"`
}

type ContactPatch struct {
	Name    *string `json:"name"`
	Company *string `json:"company"`
	Email   *string `json:"email"`
	Phone   *string `json:"phone"`
	Status  *string `json:"status"`
}

type OpportunityPatch struct {
	Name        *string `json:"name"`
	ContactID   *string `json:"contactId"`
	AmountCents *int64  `json:"amountCents"`
	Stage       *string `json:"stage"`
	NextStep    *string `json:"nextStep"`
	CloseDate   *string `json:"closeDate"`
}

type DocumentPatch struct {
	Title *string `json:"title"`
	Body  *string `json:"body"`
}

type ReminderPatch struct {
	Completed *bool `json:"completed"`
}

type Store interface {
	ListContacts(context.Context, string) ([]Contact, error)
	GetContact(context.Context, string, string) (Contact, error)
	CreateContact(context.Context, string, Contact) (Contact, error)
	UpdateContact(context.Context, string, string, ContactPatch) (Contact, error)
	ListOpportunities(context.Context, string) ([]Opportunity, error)
	CreateOpportunity(context.Context, string, Opportunity) (Opportunity, error)
	UpdateOpportunity(context.Context, string, string, OpportunityPatch) (Opportunity, error)
	ListActivities(context.Context, string) ([]Activity, error)
	CreateActivity(context.Context, string, Activity) (Activity, error)
	ListReminders(context.Context, string) ([]Reminder, error)
	CreateReminder(context.Context, string, Reminder) (Reminder, error)
	UpdateReminder(context.Context, string, string, ReminderPatch) (Reminder, error)
	ListDocuments(context.Context, string) ([]Document, error)
	CreateDocument(context.Context, string, Document) (Document, error)
	UpdateDocument(context.Context, string, string, DocumentPatch) (Document, error)
	ListCosts(context.Context, string) ([]Cost, error)
	CreateCost(context.Context, string, Cost) (Cost, error)
}
