package brokererr

import (
	"regexp"
	"strings"
)

var (
	reHTTPURL  = regexp.MustCompile(`https?://[^\s"'<>]+`)
	rePostCall = regexp.MustCompile(`Post\s+"[^"]*"`)
	reDialTCP  = regexp.MustCompile(`dial tcp[^\n]*`)
)

// Sanitize removes internal URLs, hostnames, and transport details from a
// user-visible message.
func Sanitize(message string) string {
	s := strings.TrimSpace(message)
	if s == "" {
		return ""
	}
	s = rePostCall.ReplaceAllString(s, "")
	s = reHTTPURL.ReplaceAllString(s, "")
	s = reDialTCP.ReplaceAllString(s, "")
	for _, prefix := range []string{
		"adapter request failed:",
		"paper engine error:",
		"request failed:",
	} {
		s = strings.ReplaceAll(s, prefix, "")
	}
	s = strings.Join(strings.Fields(s), " ")
	s = strings.TrimSpace(s)

	low := strings.ToLower(s)
	switch {
	case strings.Contains(low, "connection refused"),
		strings.Contains(low, "connect:"),
		strings.Contains(low, "no such host"),
		strings.Contains(low, "i/o timeout"),
		strings.Contains(low, "context deadline exceeded"):
		return PublicMessage(CodeBrokerUnreachable)
	case s == "":
		return PublicMessage(CodeInternal)
	default:
		return s
	}
}

// UserFacing maps any error to a stable code and sanitized message safe for
// trade history, dashboards, and API responses.
func UserFacing(err error) (Code, string) {
	if err == nil {
		return CodeInternal, PublicMessage(CodeInternal)
	}
	if be, ok := err.(*Error); ok {
		msg := be.Message
		if msg == "" {
			msg = PublicMessage(be.Code)
		}
		return be.Code, Sanitize(msg)
	}
	return classifyRaw(err)
}

func classifyRaw(err error) (Code, string) {
	low := strings.ToLower(err.Error())
	switch {
	case strings.Contains(low, "connection refused"),
		strings.Contains(low, "connection failed"),
		strings.Contains(low, "adapter request failed"),
		strings.Contains(low, "adapter url not configured"),
		strings.Contains(low, "adapter http"),
		strings.Contains(low, "no such host"),
		strings.Contains(low, "i/o timeout"),
		strings.Contains(low, "context deadline exceeded"),
		strings.Contains(low, "connect: "):
		return CodeBrokerUnreachable, PublicMessage(CodeBrokerUnreachable)
	case strings.Contains(low, "user not found"):
		return CodeInternal, "account not found"
	default:
		return CodeInternal, PublicMessage(CodeInternal)
	}
}
