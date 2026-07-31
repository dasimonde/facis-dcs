package handler

import "digital-contracting-service/internal/auth/oid4vp/status"

func requireStatusTrust(trust *status.TrustConfig) error {
	if trust == nil {
		return status.ErrStatusTrustNotConfigured
	}
	return nil
}
