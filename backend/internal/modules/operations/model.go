package operations

import "time"

type Identity struct {
	Subject string
	Email   string
	Name    string
}

type Member struct {
	ID        string    `json:"id" firestore:"id"`
	Email     string    `json:"email" firestore:"email"`
	Name      string    `json:"name" firestore:"name"`
	Role      string    `json:"role" firestore:"role"`
	Status    string    `json:"status" firestore:"status"`
	CreatedAt time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" firestore:"updatedAt"`
}

type Notification struct {
	ID             string     `json:"id" firestore:"id"`
	Title          string     `json:"title" firestore:"title"`
	Summary        string     `json:"summary" firestore:"summary"`
	Kind           string     `json:"kind" firestore:"kind"`
	Href           string     `json:"href" firestore:"href"`
	IdempotencyKey string     `json:"-" firestore:"idempotencyKey"`
	ReadAt         *time.Time `json:"readAt,omitempty" firestore:"readAt,omitempty"`
	CreatedAt      time.Time  `json:"createdAt" firestore:"createdAt"`
}

type PipelineStage struct {
	ID          string    `json:"id" firestore:"id"`
	Name        string    `json:"name" firestore:"name"`
	Position    int       `json:"position" firestore:"position"`
	Probability int       `json:"probability" firestore:"probability"`
	Closed      bool      `json:"closed" firestore:"closed"`
	Won         bool      `json:"won" firestore:"won"`
	CreatedAt   time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt" firestore:"updatedAt"`
}

type EmailTemplate struct {
	ID        string    `json:"id" firestore:"id"`
	Name      string    `json:"name" firestore:"name"`
	Subject   string    `json:"subject" firestore:"subject"`
	Body      string    `json:"body" firestore:"body"`
	CreatedAt time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt" firestore:"updatedAt"`
}

type EmailDelivery struct {
	ID        string    `json:"id" firestore:"id"`
	MessageID string    `json:"messageId,omitempty" firestore:"messageId,omitempty"`
	Status    string    `json:"status" firestore:"status"`
	CreatedAt time.Time `json:"createdAt" firestore:"createdAt"`
}

type GoogleConnection struct {
	ID             string          `json:"id" firestore:"id"`
	UserEmail      string          `json:"userEmail" firestore:"userEmail"`
	GoogleEmail    string          `json:"googleEmail" firestore:"googleEmail"`
	EncryptedToken string          `json:"-" firestore:"encryptedToken"`
	Tiller         *TillerSettings `json:"tiller,omitempty" firestore:"tiller,omitempty"`
	LastMailSyncAt *time.Time      `json:"lastMailSyncAt,omitempty" firestore:"lastMailSyncAt,omitempty"`
	CreatedAt      time.Time       `json:"createdAt" firestore:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt" firestore:"updatedAt"`
}

type JobExecution struct {
	ID          string    `json:"id" firestore:"id"`
	Type        string    `json:"type" firestore:"type"`
	Status      string    `json:"status" firestore:"status"`
	CompletedAt time.Time `json:"completedAt" firestore:"completedAt"`
}

type TillerSettings struct {
	SpreadsheetID string `json:"spreadsheetId" firestore:"spreadsheetId"`
	Range         string `json:"range" firestore:"range"`
}

type MailMetadata struct {
	ID         string    `json:"id" firestore:"id"`
	From       string    `json:"from" firestore:"from"`
	Subject    string    `json:"subject" firestore:"subject"`
	Snippet    string    `json:"snippet" firestore:"snippet"`
	ReceivedAt time.Time `json:"receivedAt" firestore:"receivedAt"`
	ContactID  string    `json:"contactId,omitempty" firestore:"contactId,omitempty"`
	ThreadID   string    `json:"threadId" firestore:"threadId"`
	CreatedAt  time.Time `json:"createdAt" firestore:"createdAt"`
}

type Transaction struct {
	ID          string    `json:"id" firestore:"id"`
	ExternalID  string    `json:"externalId" firestore:"externalId"`
	Date        string    `json:"date" firestore:"date"`
	Description string    `json:"description" firestore:"description"`
	Merchant    string    `json:"merchant" firestore:"merchant"`
	AmountCents int64     `json:"amountCents" firestore:"amountCents"`
	Source      string    `json:"source" firestore:"source"`
	MatchStatus string    `json:"matchStatus" firestore:"matchStatus"`
	ContactID   string    `json:"contactId,omitempty" firestore:"contactId,omitempty"`
	CostID      string    `json:"costId,omitempty" firestore:"costId,omitempty"`
	CreatedAt   time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt" firestore:"updatedAt"`
}

type Attachment struct {
	ID          string    `json:"id" firestore:"id"`
	FileName    string    `json:"fileName" firestore:"fileName"`
	ContentType string    `json:"contentType" firestore:"contentType"`
	Size        int64     `json:"size" firestore:"size"`
	Kind        string    `json:"kind" firestore:"kind"`
	RecordType  string    `json:"recordType" firestore:"recordType"`
	RecordID    string    `json:"recordId" firestore:"recordId"`
	ObjectName  string    `json:"-" firestore:"objectName"`
	CreatedBy   string    `json:"createdBy" firestore:"createdBy"`
	CreatedAt   time.Time `json:"createdAt" firestore:"createdAt"`
	DownloadURL string    `json:"downloadUrl,omitempty" firestore:"-"`
}

type AuditEntry struct {
	ID         string    `json:"id" firestore:"id"`
	Actor      string    `json:"actor" firestore:"actor"`
	Action     string    `json:"action" firestore:"action"`
	EntityType string    `json:"entityType" firestore:"entityType"`
	EntityID   string    `json:"entityId" firestore:"entityId"`
	Summary    string    `json:"summary" firestore:"summary"`
	CreatedAt  time.Time `json:"createdAt" firestore:"createdAt"`
}

type Export struct {
	Name        string    `json:"name"`
	ContentType string    `json:"contentType"`
	Data        string    `json:"data"`
	CreatedAt   time.Time `json:"createdAt"`
}
