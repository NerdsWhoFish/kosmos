package operations

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/people/v1"
	"google.golang.org/api/sheets/v4"
)

type GoogleProvider interface {
	Send(context.Context, *oauth2.Token, string, string, string, string) (string, error)
	SendAsAliases(context.Context, *oauth2.Token) ([]string, error)
	RecentMail(context.Context, *oauth2.Token, time.Time) ([]MailMetadata, error)
	TillerRows(context.Context, *oauth2.Token, TillerSettings) ([][]any, error)
	UpsertContact(context.Context, *oauth2.Token, GoogleContact, string) (GoogleContactReference, error)
	DeleteContact(context.Context, *oauth2.Token, string, string) error
}

type LiveGoogleProvider struct {
	config         oauth2.Config
	gmailEndpoint  string
	peopleEndpoint string
}

func NewLiveGoogleProvider(clientID, clientSecret string) LiveGoogleProvider {
	return LiveGoogleProvider{config: oauth2.Config{ClientID: clientID, ClientSecret: clientSecret, Endpoint: google.Endpoint}}
}

func (p LiveGoogleProvider) tokenSource(ctx context.Context, token *oauth2.Token) oauth2.TokenSource {
	return p.config.TokenSource(ctx, token)
}

func (p LiveGoogleProvider) gmailService(ctx context.Context, token *oauth2.Token) (*gmail.Service, error) {
	options := []option.ClientOption{option.WithTokenSource(p.tokenSource(ctx, token))}
	if p.gmailEndpoint != "" {
		options = append(options, option.WithEndpoint(p.gmailEndpoint))
	}
	return gmail.NewService(ctx, options...)
}

func (p LiveGoogleProvider) peopleService(ctx context.Context, token *oauth2.Token) (*people.Service, error) {
	options := []option.ClientOption{option.WithTokenSource(p.tokenSource(ctx, token))}
	if p.peopleEndpoint != "" {
		options = append(options, option.WithEndpoint(p.peopleEndpoint))
	}
	return people.NewService(ctx, options...)
}

func (p LiveGoogleProvider) Send(ctx context.Context, token *oauth2.Token, from, to, subject, body string) (string, error) {
	service, err := p.gmailService(ctx, token)
	if err != nil {
		return "", err
	}
	message := encodeGmailMessage(from, to, subject, body)
	result, err := service.Users.Messages.Send("me", &gmail.Message{Raw: base64.RawURLEncoding.EncodeToString([]byte(message))}).Do()
	if err != nil {
		return "", err
	}
	return result.Id, nil
}

func encodeGmailMessage(from, to, subject, body string) string {
	encodedBody := base64.StdEncoding.EncodeToString([]byte(body))
	lines := make([]string, 0, (len(encodedBody)+75)/76)
	for len(encodedBody) > 76 {
		lines = append(lines, encodedBody[:76])
		encodedBody = encodedBody[76:]
	}
	lines = append(lines, encodedBody)
	return "From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + mime.BEncoding.Encode("UTF-8", subject) + "\r\n" +
		"MIME-Version: 1.0\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n" +
		"Content-Transfer-Encoding: base64\r\n\r\n" +
		strings.Join(lines, "\r\n")
}

func (p LiveGoogleProvider) SendAsAliases(ctx context.Context, token *oauth2.Token) ([]string, error) {
	service, err := p.gmailService(ctx, token)
	if err != nil {
		return nil, err
	}
	result, err := service.Users.Settings.SendAs.List("me").Do()
	if err != nil {
		return nil, err
	}
	aliases := make([]string, 0, len(result.SendAs))
	for _, alias := range result.SendAs {
		if alias.VerificationStatus == "accepted" {
			aliases = append(aliases, strings.ToLower(alias.SendAsEmail))
		}
	}
	return aliases, nil
}

func (p LiveGoogleProvider) RecentMail(ctx context.Context, token *oauth2.Token, since time.Time) ([]MailMetadata, error) {
	service, err := p.gmailService(ctx, token)
	if err != nil {
		return nil, err
	}
	listing, err := service.Users.Messages.List("me").LabelIds("INBOX").MaxResults(50).Do()
	if err != nil {
		return nil, err
	}
	items := make([]MailMetadata, 0, len(listing.Messages))
	for _, summary := range listing.Messages {
		message, err := service.Users.Messages.Get("me", summary.Id).Format("metadata").MetadataHeaders("From", "Subject", "Date").Do()
		if err != nil {
			return nil, err
		}
		item := MailMetadata{ID: message.Id, ThreadID: message.ThreadId, Snippet: message.Snippet, CreatedAt: time.Now().UTC()}
		for _, header := range message.Payload.Headers {
			switch strings.ToLower(header.Name) {
			case "from":
				item.From = header.Value
			case "subject":
				item.Subject = header.Value
			case "date":
				item.ReceivedAt, _ = mail.ParseDate(header.Value)
			}
		}
		if item.ReceivedAt.IsZero() {
			item.ReceivedAt = time.UnixMilli(message.InternalDate).UTC()
		}
		if !item.ReceivedAt.After(since) {
			continue
		}
		items = append(items, item)
	}
	return items, nil
}

func (p LiveGoogleProvider) TillerRows(ctx context.Context, token *oauth2.Token, settings TillerSettings) ([][]any, error) {
	service, err := sheets.NewService(ctx, option.WithTokenSource(p.tokenSource(ctx, token)))
	if err != nil {
		return nil, err
	}
	result, err := service.Spreadsheets.Values.Get(settings.SpreadsheetID, settings.Range).Do()
	if err != nil {
		return nil, err
	}
	return result.Values, nil
}

func (p LiveGoogleProvider) UpsertContact(ctx context.Context, token *oauth2.Token, contact GoogleContact, resourceName string) (GoogleContactReference, error) {
	service, err := p.peopleService(ctx, token)
	if err != nil {
		return GoogleContactReference{}, err
	}
	if resourceName == "" {
		resourceName, err = findGoogleContact(ctx, service, contact.ID)
		if err != nil {
			return GoogleContactReference{}, err
		}
	}
	person := googlePerson(contact)
	if resourceName == "" {
		return createGoogleContact(ctx, service, person)
	}
	current, err := service.People.Get(resourceName).PersonFields("metadata").Context(ctx).Do()
	if isGoogleNotFound(err) {
		return createGoogleContact(ctx, service, person)
	}
	if err != nil {
		return GoogleContactReference{}, err
	}
	person.Etag = current.Etag
	person.Metadata = current.Metadata
	updated, err := service.People.UpdateContact(resourceName, person).
		UpdatePersonFields("names,emailAddresses,phoneNumbers,organizations,urls,externalIds").
		PersonFields("metadata").Context(ctx).Do()
	if isGoogleNotFound(err) {
		return createGoogleContact(ctx, service, googlePerson(contact))
	}
	if err != nil {
		return GoogleContactReference{}, err
	}
	return GoogleContactReference{ResourceName: updated.ResourceName, ETag: updated.Etag}, nil
}

func createGoogleContact(ctx context.Context, service *people.Service, person *people.Person) (GoogleContactReference, error) {
	created, err := service.People.CreateContact(person).PersonFields("metadata").Context(ctx).Do()
	if err != nil {
		return GoogleContactReference{}, err
	}
	return GoogleContactReference{ResourceName: created.ResourceName, ETag: created.Etag}, nil
}

func isGoogleNotFound(err error) bool {
	var providerError *googleapi.Error
	return errors.As(err, &providerError) && providerError.Code == http.StatusNotFound
}

func (p LiveGoogleProvider) DeleteContact(ctx context.Context, token *oauth2.Token, contactID, resourceName string) error {
	service, err := p.peopleService(ctx, token)
	if err != nil {
		return err
	}
	if resourceName == "" {
		resourceName, err = findGoogleContact(ctx, service, contactID)
		if err != nil || resourceName == "" {
			return err
		}
	}
	_, err = service.People.DeleteContact(resourceName).Context(ctx).Do()
	var providerError *googleapi.Error
	if errors.As(err, &providerError) && providerError.Code == 404 {
		return nil
	}
	return err
}

func findGoogleContact(ctx context.Context, service *people.Service, contactID string) (string, error) {
	pageToken := ""
	for {
		call := service.People.Connections.List("people/me").PersonFields("externalIds").PageSize(1000).Context(ctx)
		if pageToken != "" {
			call = call.PageToken(pageToken)
		}
		page, err := call.Do()
		if err != nil {
			return "", err
		}
		for _, person := range page.Connections {
			for _, externalID := range person.ExternalIds {
				if externalID.Type == "kosmos" && externalID.Value == contactID {
					return person.ResourceName, nil
				}
			}
		}
		if page.NextPageToken == "" {
			return "", nil
		}
		pageToken = page.NextPageToken
	}
}

func googlePerson(contact GoogleContact) *people.Person {
	person := &people.Person{
		Names:           []*people.Name{{UnstructuredName: contact.Name}},
		ExternalIds:     []*people.ExternalId{{Type: "kosmos", Value: contact.ID}},
		ForceSendFields: []string{"EmailAddresses", "PhoneNumbers", "Organizations", "Urls"},
	}
	if contact.Email != "" {
		person.EmailAddresses = []*people.EmailAddress{{Value: contact.Email, Type: "work"}}
	} else {
		person.EmailAddresses = []*people.EmailAddress{}
	}
	if contact.Phone != "" {
		person.PhoneNumbers = []*people.PhoneNumber{{Value: contact.Phone, Type: "work"}}
	} else {
		person.PhoneNumbers = []*people.PhoneNumber{}
	}
	if contact.Organization != "" {
		person.Organizations = []*people.Organization{{Name: contact.Organization, Type: "work"}}
	} else {
		person.Organizations = []*people.Organization{}
	}
	if contact.LinkedInURL != "" {
		person.Urls = []*people.Url{{Value: contact.LinkedInURL, Type: "profile"}}
	} else {
		person.Urls = []*people.Url{}
	}
	return person
}

func tokenJSON(token *oauth2.Token) ([]byte, error) { return json.Marshal(token) }

func parseToken(value []byte) (*oauth2.Token, error) {
	var token oauth2.Token
	if err := json.NewDecoder(bytes.NewReader(value)).Decode(&token); err != nil {
		return nil, err
	}
	if token.RefreshToken == "" {
		return nil, fmt.Errorf("Google did not return offline access")
	}
	return &token, nil
}
