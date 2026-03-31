package weixin

import (
	"context"
	"fmt"
)

type FileSender struct {
	send func(context.Context, string, string, string) error
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
	if s == nil || s.send == nil {
		return fmt.Errorf("weixin file sender is not initialized")
	}
	return s.send(ctx, toUserID, contextToken, filePath)
}
