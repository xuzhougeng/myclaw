package app

import "context"

type conversationPersistenceDisabledKey struct{}

func WithConversationPersistenceDisabled(ctx context.Context) context.Context {
	return context.WithValue(ctx, conversationPersistenceDisabledKey{}, true)
}

func conversationPersistenceEnabled(ctx context.Context) bool {
	disabled, _ := ctx.Value(conversationPersistenceDisabledKey{}).(bool)
	return !disabled
}
