package operations

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

type GoogleProvider interface {
	Send(context.Context, *oauth2.Token, string, string, string) (string, error)
	RecentMail(context.Context, *oauth2.Token, time.Time) ([]MailMetadata, error)
	TillerRows(context.Context, *oauth2.Token, TillerSettings) ([][]any, error)
}

type LiveGoogleProvider struct{ config oauth2.Config }

func NewLiveGoogleProvider(clientID, clientSecret string) LiveGoogleProvider {
	return LiveGoogleProvider{config: oauth2.Config{ClientID: clientID, ClientSecret: clientSecret, Endpoint: google.Endpoint}}
}

func (p LiveGoogleProvider) tokenSource(ctx context.Context, token *oauth2.Token) oauth2.TokenSource {
	return p.config.TokenSource(ctx, token)
}

func (p LiveGoogleProvider) Send(ctx context.Context, token *oauth2.Token, to, subject, body string) (string, error) {
	service, err := gmail.NewService(ctx, option.WithTokenSource(p.tokenSource(ctx, token)))
	if err != nil {
		return "", err
	}
	message := "To: " + to + "\r\nSubject: " + subject + "\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n" + body
	result, err := service.Users.Messages.Send("me", &gmail.Message{Raw: base64.RawURLEncoding.EncodeToString([]byte(message))}).Do()
	if err != nil {
		return "", err
	}
	return result.Id, nil
}

func (p LiveGoogleProvider) RecentMail(ctx context.Context, token *oauth2.Token, since time.Time) ([]MailMetadata, error) {
	service, err := gmail.NewService(ctx, option.WithTokenSource(p.tokenSource(ctx, token)))
	if err != nil {
		return nil, err
	}
	query := "in:inbox after:" + strconv.FormatInt(since.Unix(), 10)
	listing, err := service.Users.Messages.List("me").Q(query).MaxResults(50).Do()
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
