// Package edonation — aging.go
//
// Pure, Bangkok-aware aging deadline/bucket computation (FR-31/SC#2, D-68).
// Mirrors internal/receiptno/fiscal_year.go's discipline exactly: load
// Asia/Bangkok exactly once via sync.Once (panic-on-missing-tzdata guard — a
// programming-error guard, not a recoverable runtime error), normalize every
// input time.Time via .In(loc) before any date math, and NEVER read the
// wall clock inside these functions — the caller always passes "now"
// explicitly (testability, 05-RESEARCH.md Pattern 5).
//
// receiptno's own bangkokLoc/loadBangkok are unexported and cannot be
// imported across the package boundary — this file duplicates the same
// sync.Once + LoadLocation("Asia/Bangkok") + panic guard locally rather than
// depending on receiptno internals (05-RESEARCH.md Pattern 5 note).
package edonation

import (
	"sync"
	"time"
)

// AgingBucket classifies an unkeyed issued donation against its e-Donation
// keying deadline (D-68).
type AgingBucket string

const (
	// BucketNotDue means the deadline is more than nearDueDays away.
	BucketNotDue AgingBucket = "not_due"
	// BucketNearDue means 0 <= daysRemaining <= nearDueDays (config-adjustable, D-68).
	BucketNearDue AgingBucket = "near_due"
	// BucketOverdue means the deadline instant has passed (daysRemaining < 0).
	BucketOverdue AgingBucket = "overdue"
)

// agingBangkokLoc caches the loaded Asia/Bangkok *time.Location. Populated
// exactly once by loadAgingBangkok() via agingBangkokOnce and thereafter safe
// for concurrent reads (immutable once set) — same sync.Once discipline as
// receiptno/fiscal_year.go's bangkokLoc/bangkokOnce (CR-01 precedent: a plain
// check-then-set would be a data race on the first concurrent call).
var (
	agingBangkokLoc  *time.Location
	agingBangkokOnce sync.Once
)

// loadAgingBangkok returns the Asia/Bangkok *time.Location, panicking if
// tzdata is unavailable — a programming-error/deployment-configuration guard,
// not a recoverable runtime error (mirrors receiptno.loadBangkok exactly;
// Pitfall 5 — the binary must embed tzdata via 'import _ "time/tzdata"' in
// main.go, or the container must install the tzdata OS package. receiptno
// already requires this today, so no additional deployment change is needed).
func loadAgingBangkok() *time.Location {
	agingBangkokOnce.Do(func() {
		loc, err := time.LoadLocation("Asia/Bangkok")
		if err != nil {
			panic("Asia/Bangkok timezone not available: " + err.Error())
		}
		agingBangkokLoc = loc
	})
	return agingBangkokLoc
}

// computeDeadline returns the 5th of the month AFTER approvedAt's month,
// normalized to Asia/Bangkok (D-68). For a December approval, the deadline is
// January 5th of the FOLLOWING year — time.Date(year, month+1, ...) safely
// normalizes the month-13 overflow into "January of next year" per Go
// stdlib's documented time.Date behavior (Pitfall 1); no hand-written
// December special case is needed or wanted.
//
// This function NEVER reads the wall clock — the caller always passes
// approvedAt explicitly (testability, mirrors receiptno.fiscalYear).
func computeDeadline(approvedAt time.Time) time.Time {
	loc := loadAgingBangkok()
	t := approvedAt.In(loc)
	firstOfNextMonth := time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, loc)
	return firstOfNextMonth.AddDate(0, 0, 4) // day 1 + 4 = day 5
}

// computeBucket classifies approvedAt's aging bucket relative to now, given a
// config-driven nearDueDays threshold (D-68):
//
//	now on/after the deadline instant       → overdue
//	0 <= daysRemaining <= nearDueDays        → near_due
//	daysRemaining > nearDueDays              → not_due
//
// The "on/after the deadline instant" check is a strict !Before comparison
// (not a days>=0 truncated-integer comparison) so that now == deadline
// classifies as overdue rather than near_due at the exact boundary instant.
//
// This function NEVER reads the wall clock — the caller always passes now
// explicitly (testability, mirrors receiptno.fiscalYear).
func computeBucket(approvedAt, now time.Time, nearDueDays int) AgingBucket {
	deadline := computeDeadline(approvedAt)
	nowNorm := now.In(deadline.Location())

	if !nowNorm.Before(deadline) {
		return BucketOverdue
	}

	daysRemaining := int(deadline.Sub(nowNorm).Hours() / 24)
	if daysRemaining <= nearDueDays {
		return BucketNearDue
	}
	return BucketNotDue
}
