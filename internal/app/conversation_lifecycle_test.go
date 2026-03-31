package app

import "testing"

func TestResolveConversationLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input ConversationLifecycleInput
		want  ConversationLifecycleResult
	}{
		{
			name: "lookup bound conversation",
			input: ConversationLifecycleInput{
				Mode:                    ConversationLifecycleLookup,
				BoundConversationID:     "bound-1",
				LegacyConversationID:    "legacy-1",
				BoundConversationExists: true,
			},
			want: ConversationLifecycleResult{
				ConversationID: "bound-1",
			},
		},
		{
			name: "bind existing legacy conversation",
			input: ConversationLifecycleInput{
				Mode:                     ConversationLifecycleBindOrCreate,
				LegacyConversationID:     "legacy-1",
				LegacyConversationExists: true,
			},
			want: ConversationLifecycleResult{
				ConversationID:        "legacy-1",
				BindingConversationID: "legacy-1",
				PersistBinding:        true,
			},
		},
		{
			name: "recover missing bound conversation with new session",
			input: ConversationLifecycleInput{
				Mode:                 ConversationLifecycleBindOrCreate,
				BoundConversationID:  "bound-1",
				LegacyConversationID: "legacy-1",
				NextConversationID:   "next-1",
			},
			want: ConversationLifecycleResult{
				ConversationID:        "next-1",
				BindingConversationID: "next-1",
				PersistBinding:        true,
				EnsureConversation:    true,
				ActivateConversation:  true,
				ClearInterfaceState:   true,
				Notice:                "之前对话已丢失，已进入新对话。",
			},
		},
		{
			name: "create default legacy conversation when none exists",
			input: ConversationLifecycleInput{
				Mode:                 ConversationLifecycleBindOrCreate,
				LegacyConversationID: "legacy-1",
			},
			want: ConversationLifecycleResult{
				ConversationID:        "legacy-1",
				BindingConversationID: "legacy-1",
				PersistBinding:        true,
				EnsureConversation:    true,
				ActivateConversation:  true,
				ClearInterfaceState:   true,
				Notice:                "当前对话已开始。",
			},
		},
		{
			name: "force new conversation",
			input: ConversationLifecycleInput{
				Mode:               ConversationLifecycleForceNew,
				NextConversationID: "next-1",
			},
			want: ConversationLifecycleResult{
				ConversationID:        "next-1",
				BindingConversationID: "next-1",
				PersistBinding:        true,
				EnsureConversation:    true,
				ActivateConversation:  true,
				ClearInterfaceState:   true,
			},
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := ResolveConversationLifecycle(tc.input)
			if err != nil {
				t.Fatalf("resolve lifecycle: %v", err)
			}
			if got != tc.want {
				t.Fatalf("unexpected lifecycle result:\nwant: %#v\ngot:  %#v", tc.want, got)
			}
		})
	}
}
