package status

import "time"

func MapIETFResult(ref Reference, value uint64) Result {
	state := StateUnknown

	switch value {
	case 0x00:
		state = StateValid
	case 0x01:
		state = StateInvalid
	case 0x02:
		state = StateSuspended
	case 0x03, 0x0C, 0x0D, 0x0E, 0x0F:
		state = StateApplicationSpecific
	}

	return newResult(ref, value, state)
}

func MapW3CResult(ref Reference, value uint64) Result {
	if value == 0 {
		return newResult(ref, value, StateValid)
	}

	switch ref.Purpose {
	case "revocation":
		return newResult(ref, value, StateInvalid)
	case "suspension":
		return newResult(ref, value, StateSuspended)
	case "refresh":
		return newResult(ref, value, StateRefreshRequired)
	case "message":
		return newResult(ref, value, StateApplicationSpecific)
	default:
		return newResult(ref, value, StateUnknown)
	}
}

func newResult(ref Reference, value uint64, state State) Result {
	return Result{
		Mechanism: ref.Mechanism,
		State:     state,
		RawValue:  value,
		Purpose:   ref.Purpose,
		URI:       ref.URI,
		Index:     ref.Index,
		CheckedAt: time.Now().UTC(),
	}
}

type Policy interface {
	Evaluate(credential VerifiedCredential, results []Result) (CredentialVerificationResult, error)
	HandleMissingStatus(credential VerifiedCredential) (CredentialVerificationResult, error)
}

type CredentialVerificationResult struct {
	Accepted      bool
	Credential    VerifiedCredential
	StatusResults []Result
	Reason        string
}

type StrictPolicy struct{}

func (StrictPolicy) HandleMissingStatus(credential VerifiedCredential) (CredentialVerificationResult, error) {
	return CredentialVerificationResult{
		Accepted:   false,
		Credential: credential,
		Reason:     "credential has no status reference",
	}, nil
}

func (StrictPolicy) Evaluate(credential VerifiedCredential, results []Result) (CredentialVerificationResult, error) {
	for _, result := range results {
		switch result.State {
		case StateInvalid:
			return CredentialVerificationResult{
				Accepted:      false,
				Credential:    credential,
				StatusResults: results,
				Reason:        "credential is revoked or invalid",
			}, nil
		case StateSuspended:
			return CredentialVerificationResult{
				Accepted:      false,
				Credential:    credential,
				StatusResults: results,
				Reason:        "credential is suspended",
			}, nil
		case StateUnknown, StateApplicationSpecific:
			return CredentialVerificationResult{
				Accepted:      false,
				Credential:    credential,
				StatusResults: results,
				Reason:        "credential status cannot be safely interpreted",
			}, nil
		}
	}

	return CredentialVerificationResult{
		Accepted:      true,
		Credential:    credential,
		StatusResults: results,
	}, nil
}
