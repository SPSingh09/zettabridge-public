package brokererr

import "fmt"

type Code string

const (
	CodeAuthFailed          Code = "auth_failed"
	CodeInvalidCredentials  Code = "invalid_credentials"
	CodeInvalidSymbol       Code = "invalid_symbol"
	CodeInsufficientMargin  Code = "insufficient_margin"
	CodeMarketClosed        Code = "market_closed"
	CodeRateLimited         Code = "rate_limited"
	CodeBrokerUnreachable   Code = "broker_unreachable"
	CodeUnsupportedAction   Code = "unsupported_action"
	CodeNoPosition          Code = "no_position"
	CodeAccountSuspended    Code = "account_suspended"
	CodeOrgSuspended        Code = "org_suspended"
	CodeLiveNotAllowed      Code = "live_not_allowed"
	CodeAlgoIDRequired      Code = "algo_id_required"
	CodeInternal            Code = "internal"
	CodeBracketNotSupported Code = "bracket_not_supported"
	CodeOrderNotFound       Code = "order_not_found"
	CodeAlreadyCancelled    Code = "already_cancelled"
	// CodePermissionDenied means the broker rejected the call because the
	// API key/app lacks a required permission scope (e.g. Kite Connect's
	// market-data/quote API requires a separate subscription from order
	// placement) — distinct from CodeAuthFailed, which means the session
	// token itself is invalid/expired. Kite returns both as HTTP 401/403,
	// so this must be derived from the API's error_type field, not status
	// code alone (see mapKiteErrorCode in the livebrokers package).
	CodePermissionDenied Code = "permission_denied"
)

// Error is a broker failure with a stable code (logged internally; public message via PublicMessage).
type Error struct {
	Code    Code
	Message string
	Cause   error
}

func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

func New(code Code, message string) *Error {
	return &Error{Code: code, Message: message}
}

func Wrap(code Code, message string, cause error) *Error {
	return &Error{Code: code, Message: message, Cause: cause}
}

// PublicMessage returns a safe user-facing trade error string.
func PublicMessage(code Code) string {
	switch code {
	case CodeAuthFailed:
		return "authentication failed"
	case CodeInvalidCredentials:
		return "invalid broker credentials"
	case CodeInvalidSymbol:
		return "symbol not recognized"
	case CodeInsufficientMargin:
		return "insufficient margin"
	case CodeMarketClosed:
		return "market closed"
	case CodeRateLimited:
		return "broker rate limit exceeded"
	case CodeBrokerUnreachable:
		return "broker temporarily unavailable"
	case CodeUnsupportedAction:
		return "action not supported"
	case CodeNoPosition:
		return "no open position to close"
	case CodeAccountSuspended:
		return "account suspended"
	case CodeOrgSuspended:
		return "organization suspended"
	case CodeLiveNotAllowed:
		return "live trading requires pro plan or higher"
	case CodeAlgoIDRequired:
		return "SEBI algo ID required for live Indian broker; set algo_id on credential (see broker guide)"
	case CodeBracketNotSupported:
		return "bracket/cover orders not supported for this broker or product"
	case CodeOrderNotFound:
		return "order not found at broker"
	case CodeAlreadyCancelled:
		return "order already cancelled"
	case CodePermissionDenied:
		return "broker app is missing a required API permission for this call"
	default:
		return "order failed"
	}
}

// CodeOf extracts a broker error code from an error chain.
func CodeOf(err error) Code {
	if err == nil {
		return ""
	}
	if be, ok := err.(*Error); ok {
		return be.Code
	}
	return CodeInternal
}

// PublicFrom returns the public message for any error.
func PublicFrom(err error) string {
	_, msg := UserFacing(err)
	return msg
}

// TradeFields returns persisted audit fields for a trade rejection.
func TradeFields(err error) (code string, message string) {
	c, msg := UserFacing(err)
	return string(c), msg
}

// Category labels — who is responsible for a trade rejection.
const (
	CategoryZettaBridge = "zettabridge"
	CategoryBroker      = "broker"
)

// brokerCodes are error codes that mean the broker itself rejected or
// couldn't process the order (as opposed to ZettaBridge's own validation,
// compliance, or platform-side checks rejecting it before/without reaching
// the broker).
var brokerCodes = map[string]struct{}{
	string(CodeAuthFailed):          {},
	string(CodeInvalidCredentials):  {},
	string(CodeInvalidSymbol):       {},
	string(CodeInsufficientMargin):  {},
	string(CodeMarketClosed):        {},
	string(CodeBrokerUnreachable):   {},
	string(CodeUnsupportedAction):   {},
	string(CodeNoPosition):          {},
	string(CodeAlgoIDRequired):      {},
	string(CodeBracketNotSupported): {},
	string(CodeOrderNotFound):       {},
	string(CodeAlreadyCancelled):    {},
	string(CodePermissionDenied):    {},
}

// Category classifies a trade's error code as ZettaBridge-side or
// broker-side, so the webhook Summary view can show who is responsible for
// a rejection. Takes trade.ErrorCode directly, which holds either a
// brokererr.Code value or one of guard.ErrCode's validation codes (both are
// persisted in the same plain-string column). CodeRateLimited is bucketed
// as ZettaBridge: in this codebase it fires from ZettaBridge's own
// per-credential rate limiter well before the request would reach the
// broker, not from a broker-returned rate-limit response — a known
// approximation. Anything unmatched (including the empty code left by
// trades rejected before this categorization existed) defaults to
// ZettaBridge rather than assuming the broker was at fault without evidence.
func Category(code string) string {
	if _, ok := brokerCodes[code]; ok {
		return CategoryBroker
	}
	return CategoryZettaBridge
}
