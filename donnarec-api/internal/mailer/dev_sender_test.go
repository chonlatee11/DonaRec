package mailer_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/donnarec/donnarec-api/internal/mailer"
	"github.com/stretchr/testify/require"
)

// TestDevSender_Send_CapturesToDisk asserts DevSender.Send (D-60 dev/local
// implementation) writes the message body and PDF attachment to its OutDir,
// and returns a SendResult with a non-zero SentAt and an empty
// ProviderMessageID (no provider assigns one; there is no real network send).
func TestDevSender_Send_CapturesToDisk(t *testing.T) {
	outDir := t.TempDir()
	sender := &mailer.DevSender{OutDir: outDir}

	msg := mailer.Message{
		To:       "donor@example.com",
		Subject:  "ใบเสร็จรับเงินบริจาค",
		BodyHTML: "<p>ขอบคุณสำหรับการบริจาค</p>",
		BodyText: "ขอบคุณสำหรับการบริจาค",
		Attachment: mailer.Attachment{
			Filename:    "receipt.pdf",
			ContentType: "application/pdf",
			Data:        []byte("%PDF-1.4 fake receipt bytes"),
		},
	}

	result, err := sender.Send(context.Background(), msg)
	require.NoError(t, err)
	require.False(t, result.SentAt.IsZero(), "SentAt must be set on a successful capture")
	require.Empty(t, result.ProviderMessageID, "DevSender has no provider — ProviderMessageID must be empty")

	// Body + attachment must both be findable under OutDir.
	var foundBody, foundAttachment bool
	err = filepath.WalkDir(outDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		switch d.Name() {
		case "body.html":
			foundBody = true
			b, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			require.Contains(t, string(b), "ขอบคุณสำหรับการบริจาค")
		case "receipt.pdf":
			foundAttachment = true
			b, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			require.Equal(t, msg.Attachment.Data, b)
		}
		return nil
	})
	require.NoError(t, err)
	require.True(t, foundBody, "body.html must be written under OutDir")
	require.True(t, foundAttachment, "attachment file must be written under OutDir")
}
