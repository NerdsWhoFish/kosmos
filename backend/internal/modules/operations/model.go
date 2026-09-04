package operations

import "time"

type Identity struct {
	Subject string
	Email   string
	Name    string
	Kind    string
	Access  string
}

type APICredential struct {
	ID          string     `json:"id" firestore:"id"`
	Name        string     `json:"name" firestore:"name"`
	Access      string     `json:"access" firestore:"access"`
	TokenPrefix string     `json:"tokenPrefix" firestore:"tokenPrefix"`
	SecretHash  string     `json:"-" firestore:"secretHash"`
	CreatedBy   string     `json:"createdBy" firestore:"createdBy"`
	RevokedAt   *time.Time `json:"revokedAt,omitempty" firestore:"revokedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt" firestore:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt" firestore:"updatedAt"`
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
	ID        string               `json:"id" firestore:"id"`
	Name      string               `json:"name" firestore:"name"`
	Subject   string               `json:"subject" firestore:"subject"`
	Body      string               `json:"body" firestore:"body"`
	Inputs    []EmailTemplateInput `json:"inputs,omitempty" firestore:"inputs"`
	CreatedAt time.Time            `json:"createdAt" firestore:"createdAt"`
	UpdatedAt time.Time            `json:"updatedAt" firestore:"updatedAt"`
}

type EmailTemplateInput struct {
	Key          string `json:"key" firestore:"key"`
	Label        string `json:"label" firestore:"label"`
	DefaultValue string `json:"defaultValue" firestore:"defaultValue"`
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

type VoiceContactsConnection struct {
	ID             string    `json:"id" firestore:"id"`
	GoogleEmail    string    `json:"googleEmail" firestore:"googleEmail"`
	GoogleSubject  string    `json:"-" firestore:"googleSubject"`
	EncryptedToken string    `json:"-" firestore:"encryptedToken"`
	CreatedBy      string    `json:"createdBy" firestore:"createdBy"`
	CreatedAt      time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt" firestore:"updatedAt"`
}

type GoogleContactMapping struct {
	ID           string     `json:"id" firestore:"id"`
	ContactID    string     `json:"contactId" firestore:"contactId"`
	ResourceName string     `json:"resourceName,omitempty" firestore:"resourceName,omitempty"`
	ETag         string     `json:"-" firestore:"etag,omitempty"`
	Status       string     `json:"status" firestore:"status"`
	LastError    string     `json:"lastError,omitempty" firestore:"lastError,omitempty"`
	LastSyncedAt *time.Time `json:"lastSyncedAt,omitempty" firestore:"lastSyncedAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt" firestore:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt" firestore:"updatedAt"`
}

type GoogleContact struct {
	ID           string
	Name         string
	Email        string
	Phone        string
	Organization string
	LinkedInURL  string
}

type GoogleContactReference struct {
	ResourceName string
	ETag         string
}

type SendAsMapping struct {
	ID          string    `json:"id" firestore:"id"`
	MemberID    string    `json:"memberId" firestore:"memberId"`
	MemberEmail string    `json:"memberEmail" firestore:"memberEmail"`
	Email       string    `json:"email" firestore:"email"`
	UpdatedBy   string    `json:"updatedBy" firestore:"updatedBy"`
	CreatedAt   time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt" firestore:"updatedAt"`
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

type TillerWebhookConnection struct {
	ID              string    `json:"id" firestore:"id"`
	EncryptedSecret string    `json:"-" firestore:"encryptedSecret"`
	CreatedBy       string    `json:"createdBy" firestore:"createdBy"`
	CreatedAt       time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt" firestore:"updatedAt"`
}

type TillerProductMapping struct {
	ID          string    `json:"id" firestore:"id"`
	ProductID   string    `json:"productId" firestore:"productId"`
	ProductName string    `json:"productName,omitempty" firestore:"productName,omitempty"`
	AccountID   string    `json:"accountId" firestore:"accountId"`
	CreatedBy   string    `json:"createdBy" firestore:"createdBy"`
	CreatedAt   time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt" firestore:"updatedAt"`
}

type CloudflareConnection struct {
	ID             string    `json:"id" firestore:"id"`
	AccountID      string    `json:"accountId" firestore:"accountId"`
	EncryptedToken string    `json:"-" firestore:"encryptedToken"`
	CreatedBy      string    `json:"createdBy" firestore:"createdBy"`
	CreatedAt      time.Time `json:"createdAt" firestore:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt" firestore:"updatedAt"`
}

type CloudflareDomain struct {
	DomainName  string `json:"domainName"`
	ZoneID      string `json:"zoneId,omitempty"`
	Registered  bool   `json:"registered"`
	RenewalDate string `json:"renewalDate,omitempty"`
	AutoRenew   bool   `json:"autoRenew"`
	Status      string `json:"status,omitempty"`
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
	AccountID   string    `json:"accountId,omitempty" firestore:"accountId,omitempty"`
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
	ContentHash string    `json:"contentHash,omitempty" firestore:"contentHash,omitempty"`
	CreatedBy   string    `json:"createdBy" firestore:"createdBy"`
	CreatedAt   time.Time `json:"createdAt" firestore:"createdAt"`
	DownloadURL string    `json:"downloadUrl,omitempty" firestore:"-"`
	ViewURL     string    `json:"viewUrl,omitempty" firestore:"-"`
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
