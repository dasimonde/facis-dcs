package hsm

import (
	"fmt"
	"os"
	"strings"
)

// Default per-purpose key labels (DCS_HSM_KEY_* env overrides).
const (
	defaultKeyDID  = "dcs-did"
	defaultKeyVC   = "dcs-vc"
	defaultKeyJAR  = "dcs-oid4vp-jar"
	defaultKeyC2PA = "dcs-c2pa"
	defaultKeyECDH = "dcs-ecdh"

	defaultModulePath = "/usr/lib/softhsm/libsofthsm2.so"
	defaultTokenLabel = "dcs"
)

// ConfigFromEnv reads the PKCS#11 module/token/PIN configuration from the
// environment, applying the documented defaults for module path and token
// label. PKCS11_PIN has no default and must be provided.
func ConfigFromEnv() Config {
	return Config{
		ModulePath: envOr("PKCS11_MODULE_PATH", defaultModulePath),
		TokenLabel: envOr("PKCS11_TOKEN_LABEL", defaultTokenLabel),
		Pin:        strings.TrimSpace(os.Getenv("PKCS11_PIN")),
	}
}

// KeyLabelDID returns the CKA_LABEL of the DID signing key.
func KeyLabelDID() string { return envOr("DCS_HSM_KEY_DID", defaultKeyDID) }

// KeyLabelVC returns the CKA_LABEL of the lifecycle-VC signing key.
func KeyLabelVC() string { return envOr("DCS_HSM_KEY_VC", defaultKeyVC) }

// KeyLabelJAR returns the CKA_LABEL of the OpenID4VP JAR signing key.
func KeyLabelJAR() string { return envOr("DCS_HSM_KEY_JAR", defaultKeyJAR) }

// KeyLabelC2PA returns the CKA_LABEL of the C2PA COSE signing key.
func KeyLabelC2PA() string { return envOr("DCS_HSM_KEY_C2PA", defaultKeyC2PA) }

// KeyLabelECDH returns the CKA_LABEL of the key-agreement (CEK wrap) key.
func KeyLabelECDH() string { return envOr("DCS_HSM_KEY_ECDH", defaultKeyECDH) }

// VersionedLabel returns the CKA_LABEL of a specific key version for a base
// label. Version 1 is the un-suffixed base label produced by the initial token
// provisioning; each rotation adds a "-v<N>" key alongside it, so v2 is
// "<base>-v2" and so on. The old versions' keys stay in the token for
// verification of historical signatures.
func VersionedLabel(base string, version int) string {
	if version <= 1 {
		return base
	}
	return fmt.Sprintf("%s-v%d", base, version)
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
