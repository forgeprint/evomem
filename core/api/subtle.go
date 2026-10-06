package api

import "crypto/subtle"

// constantTimeEqual compares two secrets without leaking how much of one
// matched to anyone who can measure the response.
//
// ConstantTimeCompare returns 0 for inputs of different lengths without
// looking at them, so the length of the configured token is observable. That
// is not what needs protecting; the token itself is.
func constantTimeEqual(given, want string) bool {
	return subtle.ConstantTimeCompare([]byte(given), []byte(want)) == 1
}
