// Package report implements the report domain: authenticated users may report
// posts or other users for moderation review.
//
// Per CLAUDE.md §2.3, reporter identity is NEVER returned in any response body.
// Per CLAUDE.md §12, rate limiting is fail-closed for abuse-sensitive operations.
package report

// ReportTargetType identifies what kind of entity is being reported.
type ReportTargetType string

const (
	ReportTargetPost ReportTargetType = "post"
	ReportTargetUser ReportTargetType = "user"
)

// ReportReason is the machine-readable reason code for a report.
// Values must match the reports_reason_check constraint in
// db/migrations/0013_create_reports_and_studio_indexes.up.sql.
type ReportReason string

const (
	ReasonSpam           ReportReason = "spam"
	ReasonHarassment     ReportReason = "harassment"
	ReasonMisinformation ReportReason = "misinformation"
	ReasonHateSpeech     ReportReason = "hate_speech"
	ReasonViolence       ReportReason = "violence"
	ReasonOther          ReportReason = "other"
)

// validReasons is the canonical server-side set for validation.
// Must stay in sync with the reports_reason_check database constraint.
var validReasons = map[ReportReason]struct{}{
	ReasonSpam:           {},
	ReasonHarassment:     {},
	ReasonMisinformation: {},
	ReasonHateSpeech:     {},
	ReasonViolence:       {},
	ReasonOther:          {},
}

// maxDetailCodePoints is the maximum number of Unicode code points allowed in
// the optional report detail field. Mirrors the reports_detail_length check
// constraint (char_length(detail) <= 500).
const maxDetailCodePoints = 500

// CreateReportRequest is the JSON request body for both report endpoints.
// No reporter identity is present — it is always taken from the JWT context.
type CreateReportRequest struct {
	Reason ReportReason `json:"reason"`
	Detail *string      `json:"detail,omitempty"`
}
