package weixin

import (
	"context"
	"fmt"

	"baize/internal/toolcontract"
)

const FileSenderToolName = "weixin_sender"

type FileSenderInput struct {
	ToUserID     string `json:"to_user_id"`
	ContextToken string `json:"context_token"`
	FilePath     string `json:"file_path"`
}

type FileSenderResult struct {
	Tool     string `json:"tool"`
	Status   string `json:"status"`
	FilePath string `json:"file_path"`
}

type FileSender struct {
	send func(context.Context, string, string, string) error
}

func Definition() toolcontract.Spec {
	return toolcontract.Spec{
		Name:              FileSenderToolName,
		Purpose:           "Send an existing local file to a Weixin conversation.",
		Description:       "Deliver one concrete local file through the active Weixin bridge. This module does not search files; it only sends a file path that has already been chosen.",
		InputContract:     "Provide to_user_id, context_token, and file_path. file_path must be an existing local file chosen by another module such as everything_file_search.",
		OutputContract:    "Returns send status and the file path that was delivered.",
		InputJSONExample:  `{"to_user_id":"wxid_xxx","context_token":"ctx-123","file_path":"D:\\exports\\output.csv"}`,
		OutputJSONExample: `{"tool":"weixin_sender","status":"sent","file_path":"D:\\exports\\output.csv"}`,
		Usage:             "Use only after another module has already identified the exact file to send. Do not use this module to search or rank files.",
	}
}

func NewFileSender(client *Client) *FileSender {
	sender := &FileSender{}
	sender.send = func(ctx context.Context, toUserID, contextToken, filePath string) error {
		return client.SendFileMessage(ctx, toUserID, contextToken, filePath)
	}
	return sender
}

func (s *FileSender) SetSendFunc(fn func(context.Context, string, string, string) error) {
	if fn == nil {
		return
	}
	s.send = fn
}

func (s *FileSender) Send(ctx context.Context, toUserID, contextToken, filePath string) error {
	_, err := s.Execute(ctx, FileSenderInput{
		ToUserID:     toUserID,
		ContextToken: contextToken,
		FilePath:     filePath,
	})
	return err
}

func (s *FileSender) Execute(ctx context.Context, input FileSenderInput) (FileSenderResult, error) {
	if s == nil || s.send == nil {
		return FileSenderResult{}, fmt.Errorf("weixin file sender is not initialized")
	}
	if err := s.send(ctx, input.ToUserID, input.ContextToken, input.FilePath); err != nil {
		return FileSenderResult{}, err
	}
	return FileSenderResult{
		Tool:     FileSenderToolName,
		Status:   "sent",
		FilePath: input.FilePath,
	}, nil
}
