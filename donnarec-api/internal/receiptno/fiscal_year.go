// Package receiptno implements the gap-less per-fiscal-year receipt number allocator.
// This is the single code path that may hand out a receipt number (D-35).
//
// Fiscal year definition (FR-17, FR-18, D-40, D-41):
//   - Thai government fiscal year: 1 October – 30 September
//   - Year expressed in Buddhist Era (BE = CE + 543)
//   - Oct–Dec of CE year Y → BE fiscal year Y + 544
//   - Jan–Sep of CE year Y → BE fiscal year Y + 543
//
// Package-level tzdata note (Pitfall 5):
//
//	time.LoadLocation("Asia/Bangkok") requires IANA timezone data.
//	Production binaries MUST either:
//	  (a) embed tzdata via  import _ "time/tzdata"  in main.go, or
//	  (b) install tzdata OS package in the container (apt-get install -y tzdata / apk add tzdata).
//	This package calls panic() on LoadLocation failure — a programming-error guard,
//	not a runtime error. Missing tzdata is a deployment configuration bug.
package receiptno

import (
	"sync"
	"time"
)

// bangkokLoc caches the loaded Asia/Bangkok *time.Location.
// It is populated exactly once by loadBangkok() via bangkokOnce and is then
// safe for concurrent reads because the result is immutable. sync.Once is
// required because the very first allocation batch after a cold start can call
// loadBangkok() from many goroutines at once (e.g. concurrent approvals); a
// plain check-then-set would be a data race on the first concurrent call (CR-01).
var (
	bangkokLoc  *time.Location
	bangkokOnce sync.Once
)

// loadBangkok returns the Asia/Bangkok *time.Location, panicking if tzdata is
// unavailable (programming-error guard — deployment configuration bug, not a
// recoverable runtime error).
func loadBangkok() *time.Location {
	bangkokOnce.Do(func() {
		loc, err := time.LoadLocation("Asia/Bangkok")
		if err != nil {
			// Asia/Bangkok is a standard IANA timezone.
			// Panic only if the binary/container is missing tzdata (Pitfall 5).
			// Fix: add 'import _ "time/tzdata"' in main.go, or install tzdata in the container.
			panic("Asia/Bangkok timezone not available: " + err.Error())
		}
		bangkokLoc = loc
	})
	return bangkokLoc
}

// fiscalYear returns the Thai Buddhist Era fiscal year for the given timestamp.
//
// The fiscal year runs from 1 October of CE year Y to 30 September of CE year Y+1,
// expressed as BE year Y+1+543 = Y+544.
//
// Examples (D-40):
//
//	Sep 30 2025 23:59:59 Asia/Bangkok → 2568  (last second of FY 2568)
//	Oct  1 2025 00:00:00 Asia/Bangkok → 2569  (first second of FY 2569)
//	Sep 30 2025 17:00:00 UTC           → 2569  (= Oct 1 00:00 BKK after normalisation)
//	Jan  1 2026 00:00:00 Asia/Bangkok → 2569  (mid-year)
//	Sep 30 2026 23:59:59 Asia/Bangkok → 2569  (last second of FY 2569)
//	Oct  1 2026 00:00:00 Asia/Bangkok → 2570  (first second of FY 2570)
//
// Constraints (D-40, D-41):
//   - Input timezone is ALWAYS normalised to Asia/Bangkok before boundary check.
//   - This function NEVER calls time.Now(); the caller (Phase 3) passes the approval timestamp.
//   - Asia/Bangkok has no DST (UTC+7 all year), so no spring-forward/fall-back edge cases.
func fiscalYear(issueDate time.Time) int {
	loc := loadBangkok()

	// Normalise to Bangkok timezone — regardless of what timezone the caller passes in.
	// issueDate.In(loc) does NOT change the instant; it changes how year/month/day are read.
	t := issueDate.In(loc)

	ceYear := t.Year()
	month := t.Month()

	// Oct (10), Nov (11), Dec (12) → belong to the fiscal year starting Oct 1 of ceYear.
	// BE fiscal year = CE year of October + 1 + 543 = ceYear + 544.
	//
	// Jan (1) through Sep (9) → belong to the fiscal year that started Oct 1 of ceYear-1.
	// BE fiscal year = ceYear + 543.
	if month >= time.October {
		return ceYear + 544
	}
	return ceYear + 543
}
