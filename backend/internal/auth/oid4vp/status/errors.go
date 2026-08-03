package status

import "errors"

// These texts reach a BDD scenario as the body of a refused presentation, and
// steps/support/status_list_probe.py matches them to tell "the bit is set" from
// "the list could not be used" — the distinction a revocation test exists to
// make, since with no unsigned fallback both refuse the credential. Renaming one
// costs a scenario its diagnosis, not its verdict.
var (
	ErrInvalidStatusSize        = errors.New("invalid status size")
	ErrIndexOutOfRange          = errors.New("status index out of range")
	ErrUnsupportedBitOrder      = errors.New("unsupported bit order")
	ErrStatusRetrieval          = errors.New("status list retrieval failed")
	ErrStatusSignature          = errors.New("status list signature verification failed")
	ErrStatusTrustNotConfigured = errors.New("status list trust configuration is required")
	ErrStatusListNotSecured     = errors.New("status list is not secured")
	ErrStatusDecoding           = errors.New("status list decoding failed")
	ErrStatusDecompression      = errors.New("status list decompression failed")
	ErrPurposeMismatch          = errors.New("status purpose mismatch")
	ErrWrongStatusListType      = errors.New("wrong status list type")
	ErrUnsupportedMediaType     = errors.New("unsupported status list media type")
	ErrStatusURIMismatch        = errors.New("status list subject does not match reference uri")
	// ErrStatusListIssuerMismatch is a status list whose signature verifies
	// against a trusted issuer that is not the one the list itself names as its
	// issuer — any trusted issuer signing for any other one.
	ErrStatusListIssuerMismatch = errors.New("status list is signed by an issuer other than the one it names")
)
