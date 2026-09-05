package operations

import (
	"context"
	"errors"

	"github.com/NerdsWhoFish/kosmos/backend/internal/modules/workspace"
)

func (m *Module) enqueuePendingContactMutations(ctx context.Context, scope, batchKey string, targets ...string) (int, error) {
	store, ok := m.workspace.(workspace.ContactMutationStore)
	if !ok {
		return 0, errors.New("workspace does not support durable contact synchronization")
	}
	var connection VoiceContactsConnection
	if err := m.store.Get(ctx, scope, "voiceContactsConnections", voiceContactsConnectionID, &connection); errors.Is(err, errNotFound) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	mutations, err := store.ListContactMutations(ctx, scope)
	if err != nil {
		return 0, err
	}
	queued := 0
	for _, mutation := range mutations {
		job := Job{ID: deterministicID("contact-outbox|" + mutation.ID + "|" + batchKey), Type: JobTypeGoogleContactSync, Scope: scope, ConnectionID: connection.ID, ContactID: mutation.ContactID, Action: mutation.Action, Actor: "system", OutboxID: mutation.ID}
		if err := m.enqueueJob(ctx, job, targets...); err != nil {
			return queued, err
		}
		queued++
	}
	return queued, nil
}

func (m *Module) contactJobPending(ctx context.Context, job Job) (bool, error) {
	if job.OutboxID == "" {
		return true, nil
	}
	store, ok := m.workspace.(workspace.ContactMutationStore)
	if !ok {
		return false, errors.New("workspace does not support durable contact synchronization")
	}
	mutation, found, err := store.GetContactMutation(ctx, job.Scope, job.OutboxID)
	if err != nil || !found {
		return false, err
	}
	if mutation.ContactID != job.ContactID || mutation.Action != job.Action {
		return false, errors.New("contact synchronization does not match its durable mutation")
	}
	return true, nil
}

func (m *Module) completeContactJob(ctx context.Context, job Job) error {
	if job.OutboxID == "" {
		return nil
	}
	store, ok := m.workspace.(workspace.ContactMutationStore)
	if !ok {
		return errors.New("workspace does not support durable contact synchronization")
	}
	return store.CompleteContactMutation(ctx, job.Scope, job.OutboxID)
}
