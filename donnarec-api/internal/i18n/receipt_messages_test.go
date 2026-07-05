package i18n_test

import (
	"testing"

	gogoi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/stretchr/testify/require"

	"github.com/donnarec/donnarec-api/internal/i18n"
)

// TestBundle_ReceiptAndEmailMessageIDs_DifferByLocale asserts the go-i18n bundle
// resolves the receipt/email message IDs added for Phase 4 (FR-23/26) for both
// th and en locales, and that the two locales produce different, non-empty
// strings — proving the catalog actually drives bilingual receipt/email output
// rather than falling back to the same text (or the bare message ID) for both.
func TestBundle_ReceiptAndEmailMessageIDs_DifferByLocale(t *testing.T) {
	bundle, err := i18n.SetupBundle("locales")
	require.NoError(t, err)

	thLocalizer := gogoi18n.NewLocalizer(bundle, "th")
	enLocalizer := gogoi18n.NewLocalizer(bundle, "en")

	messageIDs := []string{"email.subject", "receipt.header"}

	for _, id := range messageIDs {
		thMsg, err := thLocalizer.Localize(&gogoi18n.LocalizeConfig{MessageID: id})
		require.NoError(t, err, "th localize %s", id)
		require.NotEmpty(t, thMsg, "th %s must not be empty", id)

		enMsg, err := enLocalizer.Localize(&gogoi18n.LocalizeConfig{MessageID: id})
		require.NoError(t, err, "en localize %s", id)
		require.NotEmpty(t, enMsg, "en %s must not be empty", id)

		require.NotEqual(t, thMsg, enMsg, "th vs en must differ for %s", id)
	}
}
