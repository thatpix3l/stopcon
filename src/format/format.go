package format

import (
	"fmt"
	"regexp"
)

type token struct {
	name            string
	Index           int
	captureGroup    string
	formatSpecifier string
}

type tokens struct {
	Slice []token
	Map   map[string]*token
}

type matcher struct {
	base     string
	Tokens   tokens
	Layout   string
	regexStr string
	Regex    *regexp.Regexp
}

func (m matcher) compile() matcher {

	m.Tokens.Map = map[string]*token{}

	// List of specifiers
	formatSpecifiers := make([]any, len(m.Tokens.Slice))

	// List of capture groups
	captureGroups := make([]any, len(m.Tokens.Slice))

	// For each token...
	for i := range m.Tokens.Slice {

		// Get reference to current token
		t := &m.Tokens.Slice[i]

		// Set token's index relative to order used in specifiers
		t.Index = i

		// Store in ordered list of tokens
		m.Tokens.Map[t.name] = t

		// Add specifier to list
		formatSpecifiers[i] = t.formatSpecifier

		// Add capture group to list
		captureGroups[i] = "(" + t.captureGroup + ")"

	}

	// Create format
	m.Layout = fmt.Sprintf(m.base, formatSpecifiers...)

	// Create regex string
	m.regexStr = fmt.Sprintf("^"+m.base+"$", captureGroups...)

	// Create regex
	m.Regex = regexp.MustCompile(m.regexStr)

	return m
}

var (
	tokenDate      = token{name: "date", captureGroup: "[0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}_[0-9]{2}_[0-9]{2}", formatSpecifier: "%s"}
	tokenId        = token{name: "id", captureGroup: "[0-9]{4}", formatSpecifier: "%s"}
	tokenIndex     = token{name: "index", captureGroup: "[0-9]{2}", formatSpecifier: "%02d"}
	tokenExtension = token{name: "extension", captureGroup: "[a-zA-Z0-9]+", formatSpecifier: "%s"}
	tokenCodec     = token{name: "codec", captureGroup: "[XH]", formatSpecifier: "%s"}
)

// Regex and format for a raw video.
var Raw = matcher{
	base:   "G%s%s%s.%s",
	Tokens: tokens{Slice: []token{tokenCodec, tokenIndex, tokenId, tokenExtension}},
}.compile()

// Regex and format for a renamed video.
var Renamed = matcher{
	base:   "Recording _-_ Date %s _-_ ID %s _-_ Part %s.%s",
	Tokens: tokens{Slice: []token{tokenDate, tokenId, tokenIndex, tokenExtension}},
}.compile()

// Regex and format for a merged video.
var Merged = matcher{
	base:   "Recording _-_ Date %s _-_ ID %s.%s",
	Tokens: tokens{Slice: []token{tokenDate, tokenId, tokenExtension}},
}.compile()
