package status

import "errors"

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
