package envelope

// Test hooks for draft-21 CWT conformance tests (package envelope_test only).

func UnmarshalCOSESign1FullForTest(raw []byte) (protected []byte, unprotected map[int64]any, payload, signature []byte, err error) {
	return unmarshalCOSESign1Full(raw)
}

func COSEKIDFromHeadersForTest(protected []byte, unprotected map[int64]any) (string, error) {
	return coseKIDFromHeaders(protected, unprotected)
}

func DecodeCWTClaimsSetForTest(payload []byte) (map[int64]any, error) {
	return decodeCWTClaimsSet(payload)
}

func ValidateStatusListCWTClaimsForTest(claims map[int64]any) error {
	return validateStatusListCWTClaims(claims)
}

func ValidateStatusListCWTProtectedForTest(protected []byte) error {
	return validateStatusListCWTProtected(protected)
}
