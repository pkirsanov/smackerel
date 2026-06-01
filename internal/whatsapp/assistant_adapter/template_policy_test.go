// TP-072-09 — SCN-072-A09: the WhatsApp renderer MUST NOT silently
// wrap free-form replies in WhatsApp message templates. SCOPE-2
// enforces this structurally: the renderer only produces text or
// interactive families. There is no template field on TextMessage
// or InteractiveMessage, and no OutboundMessageKind value names a
// template. Any future addition of template support MUST flow
// through an explicit operator-runbook surface, not the default
// render path.
//
// This test guards the structural invariant by exercising every
// AssistantResponse shape the renderer supports and asserting that
// the OutboundMessageKind is always one of the three closed-vocab
// non-template families.

package assistant_adapter

import (
	"reflect"
	"strings"
	"testing"

	"github.com/smackerel/smackerel/internal/assistant/contracts"
)

func TestRender_NeverEmitsTemplateFamily(t *testing.T) {
	cases := []struct {
		name string
		resp contracts.AssistantResponse
	}{
		{
			name: "plain body in 24h window",
			resp: contracts.AssistantResponse{Body: "Forecast for tomorrow is sunny."},
		},
		{
			name: "capture acknowledgement",
			resp: contracts.AssistantResponse{
				Status:       contracts.StatusSavedAsIdea,
				CaptureRoute: true,
				Body:         "saved as an idea — i'll surface it later.",
			},
		},
		{
			name: "reset acknowledgement",
			resp: contracts.AssistantResponse{Body: "Cleared pending choice."},
		},
		{
			name: "disambiguation 3 choices",
			resp: contracts.AssistantResponse{
				Body: "Which one?",
				DisambiguationPrompt: &contracts.DisambiguationPrompt{
					DisambiguationRef: "r",
					Choices: []contracts.DisambiguationChoice{
						{Number: 1, Label: "A"}, {Number: 2, Label: "B"}, {Number: 3, Label: "C"},
					},
				},
			},
		},
		{
			name: "confirm card",
			resp: contracts.AssistantResponse{
				Body: "Confirm?",
				ConfirmCard: &contracts.ConfirmCard{
					ConfirmRef: "r", ProposedAction: "Do X", PositiveLabel: "Yes", NegativeLabel: "No",
				},
			},
		},
		{
			name: "error",
			resp: contracts.AssistantResponse{
				Status:     contracts.StatusUnavailable,
				ErrorCause: contracts.ErrProviderUnavailable,
				Body:       "Provider down.",
			},
		},
	}
	allowed := map[OutboundMessageKind]struct{}{
		OutboundText:               {},
		OutboundInteractiveButtons: {},
		OutboundInteractiveList:    {},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out, err := Render(tc.resp, renderTestMaxChars)
			if err != nil {
				t.Fatalf("Render: %v", err)
			}
			if _, ok := allowed[out.Kind]; !ok {
				t.Fatalf("kind %q is NOT one of the closed-vocab non-template families", out.Kind)
			}
			if strings.Contains(strings.ToLower(string(out.Kind)), "template") {
				t.Fatalf("kind %q contains the substring 'template' — template wrapping is forbidden on the default path", out.Kind)
			}
		})
	}
}

// Structural guard — TextMessage and InteractiveMessage MUST NOT
// expose any field named "Template" (or matching template_*). A
// future agent adding template support via the operator-runbook
// flow MUST add a separate surface, not extend these types.
func TestRender_OutboundTypesHaveNoTemplateField(t *testing.T) {
	check := func(t *testing.T, v interface{}) {
		t.Helper()
		rt := reflect.TypeOf(v)
		for i := 0; i < rt.NumField(); i++ {
			name := strings.ToLower(rt.Field(i).Name)
			if strings.Contains(name, "template") {
				t.Errorf("%s has forbidden field %q", rt.Name(), rt.Field(i).Name)
			}
		}
	}
	check(t, TextMessage{})
	check(t, InteractiveMessage{})
	check(t, OutboundMessage{})
}
