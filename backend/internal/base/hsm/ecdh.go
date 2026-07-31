package hsm

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"errors"
	"fmt"
	"strings"

	"github.com/miekg/pkcs11"
)

// DeriveECDH performs CKM_ECDH1_DERIVE (CKD_NULL) between the token private
// key with the given CKA_LABEL and the peer's P-256 public key, returning the
// raw 32-byte shared secret (the X coordinate). It opens its own PKCS#11
// session from ConfigFromEnv, independent of the crypto11 signing context;
// crypto11 exposes no derive API. There is no software fallback: a token or
// mechanism failure is returned as an error.
func DeriveECDH(label string, peerPub *ecdsa.PublicKey) ([]byte, error) {
	cfg := ConfigFromEnv()
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, errors.New("hsm key label is required")
	}
	if peerPub == nil || peerPub.Curve != elliptic.P256() {
		return nil, errors.New("peer public key must be ECDSA P-256")
	}
	if strings.TrimSpace(cfg.Pin) == "" {
		return nil, errors.New("PKCS11_PIN is required")
	}

	ctx := pkcs11.New(cfg.ModulePath)
	if ctx == nil {
		return nil, fmt.Errorf("load pkcs11 module %q", cfg.ModulePath)
	}
	defer ctx.Destroy()

	// The module may already be initialised by the crypto11 signing context;
	// Cryptoki state is per process, so never finalise here.
	if err := ctx.Initialize(); err != nil {
		var pe pkcs11.Error
		if !errors.As(err, &pe) || pe != pkcs11.CKR_CRYPTOKI_ALREADY_INITIALIZED {
			return nil, fmt.Errorf("initialize pkcs11 module: %w", err)
		}
	}

	slot, err := findSlot(ctx, cfg.TokenLabel)
	if err != nil {
		return nil, err
	}

	session, err := ctx.OpenSession(slot, pkcs11.CKF_SERIAL_SESSION)
	if err != nil {
		return nil, fmt.Errorf("open pkcs11 session: %w", err)
	}
	defer func() { _ = ctx.CloseSession(session) }()

	if err := ctx.Login(session, pkcs11.CKU_USER, cfg.Pin); err != nil {
		var pe pkcs11.Error
		if !errors.As(err, &pe) || pe != pkcs11.CKR_USER_ALREADY_LOGGED_IN {
			return nil, fmt.Errorf("pkcs11 login: %w", err)
		}
	}

	priv, err := findPrivateKey(ctx, session, label)
	if err != nil {
		return nil, err
	}

	// Uncompressed EC point 0x04 || X || Y of the peer public key.
	ecdhPub, err := peerPub.ECDH()
	if err != nil {
		return nil, fmt.Errorf("peer public key: %w", err)
	}
	point := ecdhPub.Bytes()

	params := pkcs11.NewECDH1DeriveParams(pkcs11.CKD_NULL, nil, point)
	mech := []*pkcs11.Mechanism{pkcs11.NewMechanism(pkcs11.CKM_ECDH1_DERIVE, params)}
	template := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_SECRET_KEY),
		pkcs11.NewAttribute(pkcs11.CKA_KEY_TYPE, pkcs11.CKK_GENERIC_SECRET),
		pkcs11.NewAttribute(pkcs11.CKA_VALUE_LEN, 32),
		pkcs11.NewAttribute(pkcs11.CKA_TOKEN, false),
		pkcs11.NewAttribute(pkcs11.CKA_SENSITIVE, false),
		pkcs11.NewAttribute(pkcs11.CKA_EXTRACTABLE, true),
	}

	secret, err := ctx.DeriveKey(session, mech, priv, template)
	if err != nil {
		return nil, fmt.Errorf("derive ecdh secret for %q: %w", label, err)
	}
	defer func() { _ = ctx.DestroyObject(session, secret) }()

	attrs, err := ctx.GetAttributeValue(session, secret, []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_VALUE, nil),
	})
	if err != nil {
		return nil, fmt.Errorf("read derived secret: %w", err)
	}
	if len(attrs) == 0 {
		return nil, errors.New("derived secret has no CKA_VALUE")
	}
	if len(attrs[0].Value) != 32 {
		return nil, fmt.Errorf("derived secret has unexpected length %d", len(attrs[0].Value))
	}
	return attrs[0].Value, nil
}

func findSlot(ctx *pkcs11.Ctx, tokenLabel string) (uint, error) {
	slots, err := ctx.GetSlotList(true)
	if err != nil {
		return 0, fmt.Errorf("list pkcs11 slots: %w", err)
	}
	for _, slot := range slots {
		info, err := ctx.GetTokenInfo(slot)
		if err != nil {
			continue
		}
		if strings.TrimSpace(info.Label) == tokenLabel {
			return slot, nil
		}
	}
	return 0, fmt.Errorf("pkcs11 token %q not found", tokenLabel)
}

func findPrivateKey(ctx *pkcs11.Ctx, session pkcs11.SessionHandle, label string) (pkcs11.ObjectHandle, error) {
	template := []*pkcs11.Attribute{
		pkcs11.NewAttribute(pkcs11.CKA_CLASS, pkcs11.CKO_PRIVATE_KEY),
		pkcs11.NewAttribute(pkcs11.CKA_LABEL, label),
	}
	if err := ctx.FindObjectsInit(session, template); err != nil {
		return 0, fmt.Errorf("find hsm key %q: %w", label, err)
	}
	handles, _, err := ctx.FindObjects(session, 1)
	if finErr := ctx.FindObjectsFinal(session); err == nil {
		err = finErr
	}
	if err != nil {
		return 0, fmt.Errorf("find hsm key %q: %w", label, err)
	}
	if len(handles) == 0 {
		return 0, fmt.Errorf("hsm key %q not found in token", label)
	}
	return handles[0], nil
}
