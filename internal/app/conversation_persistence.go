package app

import "context"

type conversationPersistenceDisabledKey struct{}
type conversationPersistenceTrackerKey struct{}

type conversationPersistenceTracker struct {
	persisted bool
}

func WithConversationPersistenceDisabled(ctx context.Context) context.Context {
	return context.WithValue(ctx, conversationPersistenceDisabledKey{}, true)
}

func withConversationPersistenceTracker(ctx context.Context) context.Context {
	if tracker := conversationPersistenceTrackerFromContext(ctx); tracker != nil {
		return ctx
	}
	return context.WithValue(ctx, conversationPersistenceTrackerKey{}, &conversationPersistenceTracker{})
}

func conversationPersistenceEnabled(ctx context.Context) bool {
	disabled, _ := ctx.Value(conversationPersistenceDisabledKey{}).(bool)
	return !disabled
}

func conversationPersistenceTrackerFromContext(ctx context.Context) *conversationPersistenceTracker {
	tracker, _ := ctx.Value(conversationPersistenceTrackerKey{}).(*conversationPersistenceTracker)
	return tracker
}

func conversationAlreadyPersisted(ctx context.Context) bool {
	tracker := conversationPersistenceTrackerFromContext(ctx)
	return tracker != nil && tracker.persisted
}

func markConversationPersisted(ctx context.Context) {
	tracker := conversationPersistenceTrackerFromContext(ctx)
	if tracker == nil {
		return
	}
	tracker.persisted = true
}
