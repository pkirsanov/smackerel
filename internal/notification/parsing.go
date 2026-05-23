package notification

func ParseSeverity(value string) Severity {
	switch Severity(value) {
	case SeverityInfo, SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical:
		return Severity(value)
	default:
		return SeverityUnknown
	}
}

func ParseDomain(value string) Domain {
	switch Domain(value) {
	case DomainOps, DomainFinance, DomainTravel, DomainPersonal, DomainSystem:
		return Domain(value)
	default:
		return DomainUnknown
	}
}

func ParseIntent(value string) Intent {
	switch Intent(value) {
	case IntentRoutine, IntentInvestigate, IntentOutage, IntentRecovery, IntentMitigation, IntentApproval:
		return Intent(value)
	default:
		return IntentUnknown
	}
}

func validEscalationSeverity(value string) bool {
	return ParseSeverity(value) != SeverityUnknown
}
