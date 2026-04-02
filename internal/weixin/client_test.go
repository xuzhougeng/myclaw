package weixin

import (
	"context"
	"crypto/aes"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newTestClient(t *testing.T, got *SendMessageRequest) *Client {
	t.Helper()

	client := NewClient("http://unit.test", "secret-token")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/ilink/bot/sendmessage" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if r.Header.Get("AuthorizationType") != "ilink_bot_token" {
				t.Fatalf("unexpected auth type: %q", r.Header.Get("AuthorizationType"))
			}
			if r.Header.Get("Authorization") != "Bearer secret-token" {
				t.Fatalf("unexpected auth header: %q", r.Header.Get("Authorization"))
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read body: %v", err)
			}
			if err := json.Unmarshal(body, got); err != nil {
				t.Fatalf("decode body: %v", err)
			}

			return &http.Response{
				StatusCode: http.StatusOK,
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				Body: io.NopCloser(strings.NewReader(`{"ret":0}`)),
			}, nil
		}),
	}
	return client
}

func TestSendFileMessageUploadsAndSendsFileItem(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	filePath := filepath.Join(root, "sample.pdf")
	plaintext := []byte("hello from baize")
	if err := os.WriteFile(filePath, plaintext, 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	var uploadReq GetUploadURLRequest
	var sendReq SendMessageRequest
	var uploadedCiphertext []byte

	client := NewClient("http://unit.test", "secret-token")
	client.httpClient = &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch {
			case r.URL.Host == "unit.test" && r.URL.Path == "/ilink/bot/getuploadurl":
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("read getuploadurl body: %v", err)
				}
				if err := json.Unmarshal(body, &uploadReq); err != nil {
					t.Fatalf("decode getuploadurl body: %v", err)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(`{"ret":0,"upload_full_url":"https://cdn.example/upload/file"}`)),
				}, nil
			case r.URL.Host == "cdn.example" && r.URL.Path == "/upload/file":
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("read cdn upload body: %v", err)
				}
				uploadedCiphertext = append([]byte(nil), body...)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"X-Encrypted-Param": []string{"encrypted-download-param"},
					},
					Body: io.NopCloser(strings.NewReader("")),
				}, nil
			case r.URL.Host == "unit.test" && r.URL.Path == "/ilink/bot/sendmessage":
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Fatalf("read sendmessage body: %v", err)
				}
				if err := json.Unmarshal(body, &sendReq); err != nil {
					t.Fatalf("decode sendmessage body: %v", err)
				}
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type": []string{"application/json"},
					},
					Body: io.NopCloser(strings.NewReader(`{"ret":0}`)),
				}, nil
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
				return nil, nil
			}
		}),
	}

	if err := client.SendFileMessage(context.Background(), "user-1", "ctx-1", filePath); err != nil {
		t.Fatalf("send file: %v", err)
	}

	sum := md5.Sum(plaintext)
	if uploadReq.MediaType != UploadMediaTypeFile {
		t.Fatalf("unexpected media type: %d", uploadReq.MediaType)
	}
	if uploadReq.ToUserID != "user-1" {
		t.Fatalf("unexpected upload user: %q", uploadReq.ToUserID)
	}
	if uploadReq.RawSize != int64(len(plaintext)) {
		t.Fatalf("unexpected raw size: %d", uploadReq.RawSize)
	}
	if uploadReq.RawFileMD5 != hex.EncodeToString(sum[:]) {
		t.Fatalf("unexpected raw md5: %q", uploadReq.RawFileMD5)
	}
	if !uploadReq.NoNeedThumb {
		t.Fatal("expected no_need_thumb=true")
	}
	if len(uploadReq.AESKey) != 32 {
		t.Fatalf("expected hex aes key, got %q", uploadReq.AESKey)
	}
	if len(uploadedCiphertext) == 0 || len(uploadedCiphertext)%aes.BlockSize != 0 {
		t.Fatalf("unexpected ciphertext length: %d", len(uploadedCiphertext))
	}

	if sendReq.Msg.ContextToken != "ctx-1" {
		t.Fatalf("unexpected context token: %q", sendReq.Msg.ContextToken)
	}
	if len(sendReq.Msg.ItemList) != 1 {
		t.Fatalf("expected 1 item, got %#v", sendReq.Msg.ItemList)
	}
	item := sendReq.Msg.ItemList[0]
	if item.Type != ItemTypeFile || item.FileItem == nil || item.FileItem.Media == nil {
		t.Fatalf("unexpected file item: %#v", item)
	}
	if item.FileItem.Media.EncryptQueryParam != "encrypted-download-param" {
		t.Fatalf("unexpected encrypted query param: %q", item.FileItem.Media.EncryptQueryParam)
	}
	if item.FileItem.Media.EncryptType != fileEncryptType {
		t.Fatalf("unexpected encrypt type: %d", item.FileItem.Media.EncryptType)
	}
	if item.FileItem.FileName != "sample.pdf" {
		t.Fatalf("unexpected file name: %q", item.FileItem.FileName)
	}
	if item.FileItem.Len != strconv.Itoa(len(plaintext)) {
		t.Fatalf("unexpected file length: %q", item.FileItem.Len)
	}

	aesKey, err := base64.StdEncoding.DecodeString(item.FileItem.Media.AESKey)
	if err != nil {
		t.Fatalf("decode aes key: %v", err)
	}
	if len(aesKey) != 16 {
		t.Fatalf("expected 16-byte aes key, got %d", len(aesKey))
	}
}
