package db

import "strings"

// automatedPrefixes are first_message prefixes that identify
// automated (roborev) review and fix sessions. Matched
// case-sensitively against the start of first_message.
//
// The review prefix is long enough to be unambiguous. The fix
// prefix requires a trailing newline to avoid false positives
// on user headings like "# Fix Request for login flow".
var automatedPrefixes = []string{
	"You are a code reviewer. Review the code changes shown below.",
	"# Fix Request\n",
}

// IsAutomatedSession returns true if the first message
// matches a known automated review/fix prompt pattern.
func IsAutomatedSession(firstMessage string) bool {
	for _, prefix := range automatedPrefixes {
		if strings.HasPrefix(firstMessage, prefix) {
			return true
		}
	}
	return false
}
