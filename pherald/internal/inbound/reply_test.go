package inbound_test

import (
	"testing"

	"github.com/vasic-digital/herald/pherald/internal/inbound"
)

func TestParseReplyActions(t *testing.T) {
	cases := []struct {
		name       string
		stdout     string
		wantAction string
		wantErr    bool
	}{
		{
			name:       "reply explicit",
			stdout:     `<<<HERALD-REPLY>>> {"action":"reply","text":"hi"}`,
			wantAction: "reply",
		},
		{
			name:       "reply default",
			stdout:     `<<<HERALD-REPLY>>> {"text":"hi"}`,
			wantAction: "reply",
		},
		{
			name:       "issue.open",
			stdout:     `<<<HERALD-REPLY>>> {"action":"issue.open","issue":{"type":"bug","criticality":"high","title":"x","body":"y","labels":["repro"]}}`,
			wantAction: "issue.open",
		},
		{
			name:       "event.emit",
			stdout:     `<<<HERALD-REPLY>>> {"action":"event.emit","event":{"cloudevent_type":"com.example.t","subject":"s","data":{"k":"v"}}}`,
			wantAction: "event.emit",
		},
		{
			name:    "no marker",
			stdout:  `gibberish without the marker`,
			wantErr: true,
		},
		{
			name:    "marker present but no JSON",
			stdout:  `<<<HERALD-REPLY>>> nothing follows`,
			wantErr: true,
		},
		{
			name:    "malformed JSON",
			stdout:  `<<<HERALD-REPLY>>> {oops not valid json`,
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := inbound.ParseReply([]byte(tc.stdout))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got nil; reply=%+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Fatalf("got nil reply without error")
			}
			if got.Action != tc.wantAction {
				t.Fatalf("action: got %q want %q", got.Action, tc.wantAction)
			}
		})
	}
}

func TestParseReplyIssuePayloadDecoded(t *testing.T) {
	stdout := `<<<HERALD-REPLY>>> {"action":"issue.open","issue":{"type":"bug","criticality":"high","title":"X","body":"Y","labels":["a","b"]}}`
	r, err := inbound.ParseReply([]byte(stdout))
	if err != nil {
		t.Fatal(err)
	}
	if r.Issue == nil {
		t.Fatalf("Issue payload nil")
	}
	if r.Issue.Type != "bug" || r.Issue.Criticality != "high" || r.Issue.Title != "X" || r.Issue.Body != "Y" {
		t.Fatalf("Issue fields wrong: %+v", r.Issue)
	}
	if len(r.Issue.Labels) != 2 || r.Issue.Labels[0] != "a" || r.Issue.Labels[1] != "b" {
		t.Fatalf("Issue labels wrong: %+v", r.Issue.Labels)
	}
}

func TestParseReplyEventPayloadDecoded(t *testing.T) {
	stdout := `<<<HERALD-REPLY>>> {"action":"event.emit","event":{"cloudevent_type":"com.example.t","subject":"s","data":{"k":"v","n":42}}}`
	r, err := inbound.ParseReply([]byte(stdout))
	if err != nil {
		t.Fatal(err)
	}
	if r.Event == nil {
		t.Fatalf("Event payload nil")
	}
	if r.Event.CloudEventType != "com.example.t" || r.Event.Subject != "s" {
		t.Fatalf("Event fields wrong: %+v", r.Event)
	}
	if r.Event.Data["k"] != "v" {
		t.Fatalf("Event data missing k=v: %+v", r.Event.Data)
	}
}
