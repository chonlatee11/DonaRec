// Package i18n provides internationalization support for donnarec-api.
// It wraps go-i18n/v2 with a bundle loaded at startup (not per-request).
// Thai is the default locale; English is the secondary locale.
package i18n

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// SetupBundle loads the go-i18n message catalog from the given locales directory.
// Call once at startup; the returned bundle is safe for concurrent use (read-only).
//
// localesDir should contain th.json and en.json files.
//
// Usage:
//
//	bundle, err := i18n.SetupBundle("internal/i18n/locales")
//	localizer := i18n.NewLocalizer(bundle, c.GetHeader("Accept-Language"), "th")
//	msg := localizer.MustLocalize(&i18n.LocalizeConfig{MessageID: "auth.missing_token"})
func SetupBundle(localesDir string) (*i18n.Bundle, error) {
	// Thai is the default locale for this system
	bundle := i18n.NewBundle(language.Thai)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)

	for _, lang := range []string{"th", "en"} {
		path := filepath.Join(localesDir, lang+".json")
		if _, err := bundle.LoadMessageFile(path); err != nil {
			return nil, fmt.Errorf("load %s message catalog from %s: %w", lang, path, err)
		}
	}

	return bundle, nil
}
