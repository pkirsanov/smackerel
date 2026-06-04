package graphapi

// CrossLink is the explainable cross-link contract every spec 080
// endpoint emits when referencing another graph node. The reason
// field is server-derived only (see design.md §2); clients MUST NOT
// synthesize or override it. JSON tags match the wire shape exactly.
type CrossLink struct {
	TargetKind  string `json:"targetKind"`
	TargetID    string `json:"targetId"`
	TargetLabel string `json:"targetLabel"`
	Reason      string `json:"reason"`
}
