package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// handlePhotoUpload streams a Telegram photo into the unified upload
// pipeline (POST /v1/photos/upload). The handler:
//
//   - selects the largest PhotoSize so OCR/classification has the
//     richest pixels available,
//   - downloads the bytes via the Telegram file API (the bot token
//     stays on the server — it never leaves the request to /upload),
//   - forwards the bytes to the core API as a multipart form with
//     `source_channel=telegram` and `source_ref={chat_id}:{message_id}`
//     so SCN-040-010 can prove every Telegram capture lands in the
//     same downstream pipeline as mobile/web uploads,
//   - acknowledges the user with the freshly minted photo id, but
//     defers any classification commentary to the async pipeline.
//
// Errors are reported back to the user with a short message — the
// detailed error is logged for the operator.
func (b *Bot) handlePhotoUpload(ctx context.Context, msg *tgbotapi.Message, caption string) {
	if msg == nil || len(msg.Photo) == 0 {
		return
	}
	largest := msg.Photo[0]
	for _, candidate := range msg.Photo[1:] {
		if int64(candidate.FileSize) > int64(largest.FileSize) {
			largest = candidate
		}
	}
	fileURL, err := b.api.GetFileDirectURL(largest.FileID)
	if err != nil {
		slog.Error("telegram photo: get file URL failed", "error", err, "file_id", largest.FileID)
		b.reply(msg.Chat.ID, "x Couldn't fetch that photo from Telegram — try again.")
		return
	}
	body, contentType, err := b.downloadTelegramFile(ctx, fileURL)
	if err != nil {
		slog.Error("telegram photo: download failed", "error", err, "file_id", largest.FileID)
		b.reply(msg.Chat.ID, "x Couldn't download that photo — try again.")
		return
	}
	sourceRef := fmt.Sprintf("%d:%d", msg.Chat.ID, msg.MessageID)
	resp, err := b.postPhotoUpload(ctx, telegramPhotoUploadRequest{
		Filename:    fmt.Sprintf("telegram-%s.jpg", largest.FileUniqueID),
		ContentType: contentType,
		File:        body,
		Channel:     "telegram",
		SourceRef:   sourceRef,
		Caption:     caption,
	})
	if err != nil {
		slog.Error("telegram photo: upload failed", "error", err, "source_ref", sourceRef)
		b.reply(msg.Chat.ID, "x Saving the photo failed — try again.")
		return
	}
	ack := fmt.Sprintf(". Photo saved (%s) — classifying…", resp.PhotoID)
	if caption != "" {
		ack = fmt.Sprintf(". Photo saved with caption \"%s\" — classifying…", caption)
	}
	b.replyWithMapping(ctx, msg.Chat.ID, ack, resp.ArtifactID)
}

func (b *Bot) downloadTelegramFile(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", fmt.Errorf("build telegram file request: %w", err)
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("download telegram file: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download telegram file: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("read telegram file: %w", err)
	}
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "image/jpeg"
	}
	return body, contentType, nil
}

type telegramPhotoUploadRequest struct {
	Filename    string
	ContentType string
	File        []byte
	Channel     string
	SourceRef   string
	Caption     string
}

type telegramPhotoUploadResponse struct {
	PhotoID    string `json:"photo_id"`
	ArtifactID string `json:"artifact_id"`
}

func (b *Bot) postPhotoUpload(ctx context.Context, request telegramPhotoUploadRequest) (*telegramPhotoUploadResponse, error) {
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	if err := writer.WriteField("source_channel", request.Channel); err != nil {
		return nil, err
	}
	if err := writer.WriteField("source_ref", request.SourceRef); err != nil {
		return nil, err
	}
	if request.Caption != "" {
		if err := writer.WriteField("caption", request.Caption); err != nil {
			return nil, err
		}
	}
	part, err := writer.CreateFormFile("file", request.Filename)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(request.File); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	uploadURL := strings.TrimRight(b.baseURL, "/") + "/v1/photos/upload"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if b.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.authToken)
	}
	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read upload response: %w", err)
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload failed (%d): %s", resp.StatusCode, string(body))
	}
	var out telegramPhotoUploadResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode upload response: %w", err)
	}
	if out.PhotoID == "" {
		return nil, fmt.Errorf("upload response missing photo_id: %s", string(body))
	}
	return &out, nil
}
