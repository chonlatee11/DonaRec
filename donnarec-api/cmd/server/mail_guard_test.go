// Package main — BI-01 (04-REVIEW-PRESHIP.md) guard coverage: the dev-only
// DevSender email backend (which writes donor-PII PDFs unencrypted to local
// disk) must only be wired when MAIL_DEV=1 is explicitly set; any other value
// (including unset) must NOT enable it.
package main

import "testing"

func TestMailDevEnabled(t *testing.T) {
	cases := map[string]bool{
		"1":    true,
		"":     false,
		"0":    false,
		"true": false,
		"yes":  false,
		" 1":   false,
		"1 ":   false,
		"TRUE": false,
		"prod": false,
	}
	for val, want := range cases {
		val, want := val, want
		t.Run("MAIL_DEV="+val, func(t *testing.T) {
			got := mailDevEnabled(func(key string) string {
				if key == "MAIL_DEV" {
					return val
				}
				return ""
			})
			if got != want {
				t.Fatalf("mailDevEnabled(MAIL_DEV=%q) = %v, want %v", val, got, want)
			}
		})
	}
}
