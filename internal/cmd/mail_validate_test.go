package cmd

import (
	"strings"
	"testing"
)

func TestValidateRatifyCC(t *testing.T) {
	tests := []struct {
		name      string
		subject   string
		cc        []string
		body      string
		wantError bool
		// errSubstr — substring expected in error message when wantError is true
		errSubstr string
	}{
		// --- Block: ratify-class subject + mayor cc ---
		{
			name:      "ratify subject with mayor/ cc blocks",
			subject:   "[FOO RATIFY] proposal v3",
			cc:        []string{"mayor/"},
			body:      "Approve this please.",
			wantError: true,
			errSubstr: "Approval Routing",
		},
		{
			name:      "approve subject with bare mayor cc blocks",
			subject:   "Re: approval needed for X",
			cc:        []string{"mayor"},
			body:      "Body.",
			wantError: true,
			errSubstr: "mayor",
		},
		{
			name:      "korean 비준 subject + hq-mayor cc blocks",
			subject:   "[ka-2fc 비준 verdict] proposal stack",
			cc:        []string{"hq-mayor"},
			body:      "Body.",
			wantError: true,
		},
		{
			name:      "RATIFY all-caps + rig-scoped mayor cc blocks",
			subject:   "[BAR RATIFY ack]",
			cc:        []string{"karuna/mayor"},
			body:      "Body.",
			wantError: true,
		},
		{
			name:      "sign-off subject + mayor/ cc blocks",
			subject:   "Sign-off: layer 3 design",
			cc:        []string{"mayor/", "karuna/munger"},
			body:      "Body.",
			wantError: true,
			errSubstr: "mayor/",
		},
		{
			name:      "go/no-go subject + mayor cc blocks",
			subject:   "[X GO/NO-GO] decision",
			cc:        []string{"mayor"},
			body:      "Body.",
			wantError: true,
		},

		// --- Allow: ratify subject + waiver in body ---
		{
			name:    "ratify subject + mayor cc + waiver passes",
			subject: "[Y RATIFY] proposal",
			cc:      []string{"mayor/"},
			body: `Body content.
Approval Routing waiver: Mayor IS the dispatcher of this ratify request,
cc is informational and reflects routing source, not approval seek.`,
			wantError: false,
		},
		{
			name:      "ratify subject + mayor cc + alternate waiver passes",
			subject:   "ratify request: thing",
			cc:        []string{"hq-mayor"},
			body:      "Body.\n\nMayor cc waiver: tiebreaker case (Mayor primary scope per CLAUDE.md L67-72).",
			wantError: false,
		},
		{
			name:      "ratify subject + mayor cc + hyphenated waiver passes",
			subject:   "approval ack",
			cc:        []string{"mayor"},
			body:      "Approval-Routing-Waiver: explicit dispatcher routing.",
			wantError: false,
		},

		// --- Allow: non-ratify subject (mayor cc allowed for routine info) ---
		{
			name:      "non-ratify subject + mayor cc passes",
			subject:   "Status update on Q3",
			cc:        []string{"mayor/"},
			body:      "Body.",
			wantError: false,
		},
		{
			name:      "FYI subject + mayor cc passes",
			subject:   "[FYI] activity log Q4",
			cc:        []string{"mayor/", "karuna/cmo"},
			body:      "Body.",
			wantError: false,
		},
		{
			// Hyphen creates word boundary → "approved" matches as ratify-class.
			// Conservative-by-default; sender uses waiver if intent is non-ratify.
			name:      "hyphenated approval token still triggers (waiver bypass)",
			subject:   "approved-disapproval pattern doc",
			cc:        []string{"mayor/"},
			body:      "Body.",
			wantError: true,
		},

		// --- Allow: ratify subject + no mayor cc ---
		{
			name:      "ratify subject + non-mayor cc passes",
			subject:   "[X RATIFY] thing",
			cc:        []string{"karuna/munger", "occultfusion/scrutor"},
			body:      "Body.",
			wantError: false,
		},
		{
			name:      "ratify subject + empty cc passes",
			subject:   "[X RATIFY] thing",
			cc:        nil,
			body:      "Body.",
			wantError: false,
		},
		{
			name:      "ratify subject + empty-string cc passes",
			subject:   "[X RATIFY] thing",
			cc:        []string{},
			body:      "Body.",
			wantError: false,
		},

		// --- Edge: mayor in `to`, not cc, is out of scope (different surface) ---
		// (validateRatifyCC only inspects cc; mayor as primary recipient is
		//  a separate dispatch decision, not a doctrine violation.)

		// --- Edge: case-insensitivity on subject + cc ---
		{
			name:      "lowercase ratify subject + uppercase MAYOR cc blocks",
			subject:   "[x ratify] thing",
			cc:        []string{"MAYOR/"},
			body:      "Body.",
			wantError: true,
		},
		{
			name:      "mixed case Approval + Mayor cc blocks",
			subject:   "Approval needed",
			cc:        []string{"Mayor"},
			body:      "Body.",
			wantError: true,
		},

		// --- Edge: mayor as substring of an unrelated name does NOT match ---
		{
			name:      "mayor-substring crew name does not match",
			subject:   "[X RATIFY] thing",
			cc:        []string{"karuna/crew/mayor-watcher"},
			body:      "Body.",
			wantError: false,
		},
		{
			name:      "non-mayor address with mayor word does not match",
			subject:   "[X RATIFY] thing",
			cc:        []string{"karuna/observer-of-mayor"},
			body:      "Body.",
			wantError: false,
		},

		// --- Edge: whitespace in cc entries ---
		{
			name:      "whitespace-padded mayor cc still matches",
			subject:   "[X RATIFY] thing",
			cc:        []string{"  mayor/  "},
			body:      "Body.",
			wantError: true,
		},

		// --- Edge: multiple mayor entries reported ---
		{
			name:      "multiple mayor cc entries listed in error",
			subject:   "[X RATIFY] thing",
			cc:        []string{"mayor", "hq-mayor"},
			body:      "Body.",
			wantError: true,
			errSubstr: "mayor, hq-mayor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRatifyCC(tt.subject, tt.cc, tt.body)
			if (err != nil) != tt.wantError {
				t.Fatalf("validateRatifyCC(%q, %v, body) error = %v, wantError = %v",
					tt.subject, tt.cc, err, tt.wantError)
			}
			if tt.wantError && tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
				t.Errorf("error message missing substring %q; got: %s", tt.errSubstr, err.Error())
			}
		})
	}
}

func TestSubjectIsRatifyClass(t *testing.T) {
	tests := []struct {
		subject string
		want    bool
	}{
		// Match
		{"[X RATIFY] thing", true},
		{"ratify request", true},
		{"Ratified design", true},
		{"approval ack", true},
		{"approve this", true},
		{"approved by Munger", true},
		{"sign-off needed", true},
		{"sign off ack", true},
		{"GO/NO-GO call", true},
		{"GO/NO GO check", true},
		{"비준 요청", true},
		{"승인 부탁", true},
		{"재가 요청", true},
		// No match
		{"status update", false},
		{"FYI activity", false},
		// "approved-disapproval" — hyphen creates word-boundary so "approved" matches.
		// Conservative-by-default: treat as ratify-class. Senders use waiver to bypass.
		{"approved-disapproval research", true},
		{"reapprove this thing", false}, // word-boundary requires \bapprove\b
		{"unapproved spec", false},
	}

	for _, tt := range tests {
		t.Run(tt.subject, func(t *testing.T) {
			if got := subjectIsRatifyClass(tt.subject); got != tt.want {
				t.Errorf("subjectIsRatifyClass(%q) = %v, want %v", tt.subject, got, tt.want)
			}
		})
	}
}

func TestMayorCCMatches(t *testing.T) {
	tests := []struct {
		cc   []string
		want []string
	}{
		{[]string{"mayor/"}, []string{"mayor/"}},
		{[]string{"mayor"}, []string{"mayor"}},
		{[]string{"hq-mayor"}, []string{"hq-mayor"}},
		{[]string{"karuna/mayor"}, []string{"karuna/mayor"}},
		{[]string{"karuna/mayor/foo"}, []string{"karuna/mayor/foo"}},
		{[]string{"MAYOR/"}, []string{"MAYOR/"}},
		{[]string{"karuna/cmo", "mayor/", "occultfusion/scrutor"}, []string{"mayor/"}},
		{[]string{"karuna/observer-of-mayor"}, nil}, // mayor as substring → no match
		{[]string{"mayor-watcher"}, nil},
		{[]string{"karuna/crew/mayor-bot"}, nil},
		{[]string{"  mayor/  "}, []string{"mayor/"}}, // whitespace trimmed
		{[]string{}, nil},
		{nil, nil},
	}

	for _, tt := range tests {
		t.Run(strings.Join(tt.cc, ","), func(t *testing.T) {
			got := mayorCCMatches(tt.cc)
			if len(got) != len(tt.want) {
				t.Fatalf("mayorCCMatches(%v) = %v, want %v", tt.cc, got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("mayorCCMatches(%v)[%d] = %q, want %q", tt.cc, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBodyHasApprovalRoutingWaiver(t *testing.T) {
	tests := []struct {
		body string
		want bool
	}{
		{"Approval Routing waiver: reason", true},
		{"approval routing waiver: reason", true},
		{"APPROVAL ROUTING WAIVER: reason", true},
		{"Approval-Routing-Waiver: reason", true},
		{"Mayor CC waiver: reason", true},
		{"mayor-cc-waiver: reason", true},
		{"\nSome body\n\nApproval Routing waiver: explicit\n", true},
		// No match
		{"approval routing", false},
		{"waiver", false},
		{"Approval Routing exception", false}, // exception ≠ waiver
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.body, func(t *testing.T) {
			if got := bodyHasApprovalRoutingWaiver(tt.body); got != tt.want {
				t.Errorf("bodyHasApprovalRoutingWaiver(%q) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}

// TestValidateRecipientAddressFormat covers ka-n85 syntactic pre-flight.
// Test cases organized as: (1) trigger-evidence cases from the originating
// incident, (2) the eight valid address shapes the resolver supports,
// (3) the malformation classes the validator catches.
func TestValidateRecipientAddressFormat(t *testing.T) {
	tests := []struct {
		name      string
		addr      string
		wantError bool
		errSubstr string
	}{
		// (1) Trigger evidence — the addresses Munger's broadcast hit.
		// karuna/fe_crew & karuna/ad are syntactically VALID; resolver catches
		// the semantic typo (`fe_crew` doesn't resolve to a registered crew).
		// 2026-04-28 hotfix: underscore is canonical for some crews
		// (`karuna/backend_auth`); validator must NOT reject underscore.
		{"trigger karuna/fe_crew (syntactically valid; resolver catches typo)", "karuna/fe_crew", false, ""},
		{"trigger karuna/ad (syntactically valid; resolver catches semantic miss)", "karuna/ad", false, ""},

		// (2) Valid address shapes. Pre-flight is permissive on shape so
		// resolver-known patterns pass through without false positives.
		{"bare role mayor", "mayor", false, ""},
		{"bare role with trailing slash", "mayor/", false, ""},
		{"rig role 2-segment witness", "karuna/witness", false, ""},
		{"rig role 2-segment refinery", "karuna/refinery", false, ""},
		{"rig crew 3-segment", "karuna/crew/munger", false, ""},
		{"rig polecats 3-segment", "gastown/polecats/toast", false, ""},
		{"channel: prefix", "channel:announce", false, ""},
		{"queue: prefix", "queue:work", false, ""},
		{"@town pattern", "@town", false, ""},
		{"@rig wildcard", "@rig/karuna", false, ""},
		{"hq-* town-scope id", "hq-mayor", false, ""},
		{"hyphen-rich crew name", "occultfusion/crew/atlas", false, ""},
		// 2026-04-28 hotfix regression guards: underscore in canonical crew
		// names. Block-impact evidence: BE_auth (karuna/backend_auth) was
		// rejected by the over-strict v1 validator, blocking all comms.
		{"underscore in canonical 2-segment crew (backend_auth)", "karuna/backend_auth", false, ""},
		{"underscore in canonical 3-segment crew (crew/backend_auth)", "karuna/crew/backend_auth", false, ""},

		// (3) Malformations the validator must catch.
		{"uppercase in name", "Karuna/crew/Munger", true, "uppercase"},
		{"whitespace in name", "karuna/ crew/munger", true, "whitespace"},
		{"empty segment middle", "karuna//munger", true, "empty segment"},
		// Trailing slash on multi-segment is tolerated as normalization
		// (matches resolver's TrimSuffix behavior); the validator is not
		// the place to reject an address the resolver would normalize.
		{"trailing-slash on multi-segment is normalization, not error", "karuna/crew/munger/", false, ""},
		{"4 segments too deep", "karuna/crew/sub/munger", true, "segments"},
		{"just a slash", "/", true, "just a slash"},
		{"prefix-only", "channel:", true, "no body"},

		// Edge: empty input is caller's responsibility, not ours. We return
		// nil so this function isn't the source of "address required" errors.
		{"empty input no-op", "", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRecipientAddressFormat(tt.addr)
			if tt.wantError {
				if err == nil {
					t.Fatalf("validateRecipientAddressFormat(%q) returned nil, want error containing %q", tt.addr, tt.errSubstr)
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("validateRecipientAddressFormat(%q) error = %q, want substring %q", tt.addr, err.Error(), tt.errSubstr)
				}
			} else {
				if err != nil {
					t.Errorf("validateRecipientAddressFormat(%q) unexpected error: %v", tt.addr, err)
				}
			}
		})
	}
}

// TestValidateRecipientAddressesAggregatesErrors verifies the multi-error
// path: one bad `to` and one bad cc each surface in the combined error,
// and the caller can fix both in a single round-trip rather than
// discovering them one at a time.
func TestValidateRecipientAddressesAggregatesErrors(t *testing.T) {
	err := validateRecipientAddresses("karuna/ crew", []string{"karuna/crew/munger", "Bad UPPER"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "to:") {
		t.Errorf("error should label the `to` failure; got: %s", msg)
	}
	if !strings.Contains(msg, "cc:") {
		t.Errorf("error should label the `cc` failure; got: %s", msg)
	}
	if !strings.Contains(msg, "whitespace") {
		t.Errorf("error should mention whitespace (the `to` failure); got: %s", msg)
	}
	if !strings.Contains(msg, "uppercase") {
		t.Errorf("error should mention uppercase (the `cc` failure); got: %s", msg)
	}
}

// TestValidateRecipientAddressesAllValid is the no-op happy path: every
// address passes pre-flight, validateRecipientAddresses returns nil.
func TestValidateRecipientAddressesAllValid(t *testing.T) {
	if err := validateRecipientAddresses("karuna/crew/munger", []string{"mayor/", "occultfusion/witness", "karuna/backend_auth"}); err != nil {
		t.Errorf("all-valid input should return nil, got: %v", err)
	}
}
