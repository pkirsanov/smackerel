package experience

// This file is the spec 106 (SCOPE-106-03) renderer-neutral MUTATION-FEEDBACK
// presentation foundation. It maps ABSTRACT, owner-classified command outcomes
// to a closed MutationState vocabulary and enforces the hard invariant of the
// scope (SCN-106-010): success is announced ONLY after complete persistence and
// authoritative read-back; a PARTIAL outcome is NEVER styled or announced as
// complete, regardless of how much core state persisted.
//
// The presenter NEVER queries a domain store, owns idempotency/CSRF/versions/
// transactions, or parses a raw error string. Domain owners keep those; they
// pass an ABSTRACT OwnerMutationOutcome across the seam and the presenter maps
// it. It imports no domain logic.

// MutationState is the closed set of command feedback states. It is an
// INDEPENDENT axis from ViewState (content) and Availability.
type MutationState string

const (
	MutationIdle       MutationState = "idle"
	MutationPending    MutationState = "pending"
	MutationPersisted  MutationState = "persisted"
	MutationIdempotent MutationState = "idempotent"
	MutationConflict   MutationState = "conflict"
	MutationRefused    MutationState = "refused"
	MutationPartial    MutationState = "partial"
	MutationFailed     MutationState = "failed"
)

func (s MutationState) valid() bool {
	switch s {
	case MutationIdle, MutationPending, MutationPersisted, MutationIdempotent,
		MutationConflict, MutationRefused, MutationPartial, MutationFailed:
		return true
	}
	return false
}

// OwnerMutationKind is the closed set of ABSTRACT command outcomes a domain
// owner reports to the shared presenter across the seam.
type OwnerMutationKind string

const (
	MutationOwnerIdle              OwnerMutationKind = "idle"               // -> idle
	MutationOwnerAccepted          OwnerMutationKind = "accepted"           // user command accepted locally -> pending
	MutationOwnerCommittedReadBack OwnerMutationKind = "committed_readback" // complete commit + authoritative read-back -> persisted
	MutationOwnerAlreadyCommitted  OwnerMutationKind = "already_committed"  // identical command already committed -> idempotent
	MutationOwnerStateChanged      OwnerMutationKind = "state_changed"      // authoritative state changed -> conflict
	MutationOwnerRejectedPreWrite  OwnerMutationKind = "rejected_pre_write" // policy/auth/readiness rejected before business write -> refused
	MutationOwnerCorePlusPending   OwnerMutationKind = "core_plus_pending"  // core persisted + named outstanding dependency -> partial
	MutationOwnerTransactionFailed OwnerMutationKind = "transaction_failed" // transaction/dependency failed -> failed
)

func (k OwnerMutationKind) valid() bool {
	_, ok := mutationKindToState[k]
	return ok
}

// mutationKindToState is the total, closed mapping from an abstract owner
// command outcome to the presented MutationState.
var mutationKindToState = map[OwnerMutationKind]MutationState{
	MutationOwnerIdle:              MutationIdle,
	MutationOwnerAccepted:          MutationPending,
	MutationOwnerCommittedReadBack: MutationPersisted,
	MutationOwnerAlreadyCommitted:  MutationIdempotent,
	MutationOwnerStateChanged:      MutationConflict,
	MutationOwnerRejectedPreWrite:  MutationRefused,
	MutationOwnerCorePlusPending:   MutationPartial,
	MutationOwnerTransactionFailed: MutationFailed,
}

// OwnerMutationOutcome is the ABSTRACT, content-free command outcome an owner
// passes across the seam. It carries NO raw content.
type OwnerMutationOutcome struct {
	Kind  OwnerMutationKind
	Owner string
	// ReadBackConfirmed is the owner's proof of authoritative read-back after a
	// complete commit. A persisted outcome REQUIRES it; success is announced
	// only when it is true.
	ReadBackConfirmed bool
	// OutstandingDependencyCode names the still-outstanding dependency of a
	// partial outcome (content-free). REQUIRED for a partial outcome and
	// forbidden on any other kind.
	OutstandingDependencyCode string
}

// MutationFeedbackPresenter maps abstract owner command outcomes to the shared
// MutationState and answers the completeness/success questions truthfully.
type MutationFeedbackPresenter struct{}

// Present maps an abstract owner command outcome to a MutationState. It fails
// closed to *F106PresentationError when the Kind is unknown, when the
// outstanding-dependency code is unsafe (not content-free), when a partial
// outcome omits its outstanding dependency, when a non-partial outcome carries
// an outstanding dependency, or when a persisted outcome lacks authoritative
// read-back. It never converts a failure or partial into success.
func (MutationFeedbackPresenter) Present(o OwnerMutationOutcome) (MutationState, error) {
	if !o.Kind.valid() {
		return "", &F106PresentationError{Surface: "mutation", Violations: []string{"unknown-mutation-kind:" + string(o.Kind)}}
	}

	var v []string
	if o.OutstandingDependencyCode != "" && !safeCode(o.OutstandingDependencyCode) {
		v = append(v, "unsafe-outstanding-dependency-code")
	}
	if o.Kind == MutationOwnerCorePlusPending {
		if o.OutstandingDependencyCode == "" {
			v = append(v, "partial-missing-outstanding-dependency")
		}
	} else if o.OutstandingDependencyCode != "" {
		v = append(v, "outstanding-dependency-on-non-partial:"+string(o.Kind))
	}
	// A persisted (committed + read back) outcome must actually prove read-back.
	if o.Kind == MutationOwnerCommittedReadBack && !o.ReadBackConfirmed {
		v = append(v, "persisted-without-readback")
	}
	if len(v) > 0 {
		return "", &F106PresentationError{Surface: "mutation", Violations: v}
	}
	return mutationKindToState[o.Kind], nil
}

// IsComplete reports whether a terminal MutationState means the command is fully
// done. It is true ONLY for persisted and idempotent. It is NEVER true for
// partial — a partial outcome is never complete regardless of how much core
// state persisted.
func (MutationFeedbackPresenter) IsComplete(s MutationState) bool {
	switch s {
	case MutationPersisted, MutationIdempotent:
		return true
	default:
		return false
	}
}

// AnnouncesSuccess reports whether the shared presenter may announce or style
// the outcome as success. Success is announced ONLY from a persisted outcome
// (validated to have authoritative read-back) or an owner-complete idempotent
// outcome with confirmed read-back. Partial, pending, conflict, refused, and
// failed are NEVER announced as success.
func (p MutationFeedbackPresenter) AnnouncesSuccess(o OwnerMutationOutcome) (bool, error) {
	s, err := p.Present(o)
	if err != nil {
		return false, err
	}
	switch s {
	case MutationPersisted:
		return true, nil
	case MutationIdempotent:
		return o.ReadBackConfirmed, nil
	default:
		return false, nil
	}
}
