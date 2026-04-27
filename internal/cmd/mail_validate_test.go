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
