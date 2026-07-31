package dcstodcs

import (
	"errors"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	"digital-contracting-service/internal/base/envelope"
)

// WireWrappedCEK converts a wrapped-CEK record into its peer wire form.
func WireWrappedCEK(w *envelope.WrappedCEK) *dcstodcs.DCSToDCSWrappedCEK {
	return &dcstodcs.DCSToDCSWrappedCEK{
		Alg: w.Alg,
		Kid: w.KID,
		Epk: &dcstodcs.DCSToDCSEphemeralPublicKey{
			Kty: w.EPK.Kty,
			Crv: w.EPK.Crv,
			X:   w.EPK.X,
			Y:   w.EPK.Y,
		},
		Wrapped: w.Wrapped,
	}
}

// EnvelopeWrappedCEK converts a peer wire wrapped CEK back into the envelope
// form; the envelope layer validates algorithm and curve on unwrap.
func EnvelopeWrappedCEK(w *dcstodcs.DCSToDCSWrappedCEK) (*envelope.WrappedCEK, error) {
	if w == nil || w.Epk == nil {
		return nil, errors.New("wrapped cek carries no ephemeral public key")
	}
	return &envelope.WrappedCEK{
		Alg: w.Alg,
		KID: w.Kid,
		EPK: envelope.EphemeralPublicKey{
			Kty: w.Epk.Kty,
			Crv: w.Epk.Crv,
			X:   w.Epk.X,
			Y:   w.Epk.Y,
		},
		Wrapped: w.Wrapped,
	}, nil
}
