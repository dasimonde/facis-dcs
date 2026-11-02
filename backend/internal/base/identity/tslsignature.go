package identity

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
	"github.com/russellhaering/goxmldsig/etreeutils"
	"github.com/russellhaering/goxmldsig/types"
)

const envelopedSignatureTransform = "http://www.w3.org/2000/09/xmldsig#enveloped-signature"

var xmldsigDigests = map[string]crypto.Hash{
	"http://www.w3.org/2001/04/xmlenc#sha256":       crypto.SHA256,
	"http://www.w3.org/2001/04/xmldsig-more#sha384": crypto.SHA384,
	"http://www.w3.org/2001/04/xmlenc#sha512":       crypto.SHA512,
}

type xmldsigMethod struct {
	hash crypto.Hash
	key  string // "rsa" (PKCS #1 v1.5), "pss" (RSASSA-PSS) or "ecdsa"
}

var xmldsigMethods = map[string]xmldsigMethod{
	"http://www.w3.org/2001/04/xmldsig-more#rsa-sha256":      {crypto.SHA256, "rsa"},
	"http://www.w3.org/2001/04/xmldsig-more#rsa-sha384":      {crypto.SHA384, "rsa"},
	"http://www.w3.org/2001/04/xmldsig-more#rsa-sha512":      {crypto.SHA512, "rsa"},
	"http://www.w3.org/2007/05/xmldsig-more#sha256-rsa-MGF1": {crypto.SHA256, "pss"},
	"http://www.w3.org/2007/05/xmldsig-more#sha384-rsa-MGF1": {crypto.SHA384, "pss"},
	"http://www.w3.org/2007/05/xmldsig-more#sha512-rsa-MGF1": {crypto.SHA512, "pss"},
	"http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha256":    {crypto.SHA256, "ecdsa"},
	"http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha384":    {crypto.SHA384, "ecdsa"},
	"http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha512":    {crypto.SHA512, "ecdsa"},
}

// verifyEnvelopedSignature does the XML-DSig core validation of the
// enveloped signature of root, as ETSI TS 119 612 trusted lists carry it
// (XAdES-BES, enveloped-signature transform, exclusive or inclusive
// canonicalization). One of signers, valid at time at, must verify the
// SignedInfo. Every reference must match: the reference with URI "" covers
// the document without the signature, a reference "#id" covers an element
// of the signature (the XAdES SignedProperties). It returns the canonical
// form of the signed document.
//
// goxmldsig supplies the canonicalization. Its Validate does not apply to
// trusted lists: it stops after 1000 elements, and it expects ASN.1 ECDSA
// signature values where XML-DSig uses r||s.
func verifyEnvelopedSignature(root *etree.Element, signers []*x509.Certificate, at time.Time) ([]byte, error) {
	var sigEl *etree.Element
	for _, child := range root.ChildElements() {
		if child.Tag == dsig.SignatureTag && child.NamespaceURI() == dsig.Namespace {
			if sigEl != nil {
				return nil, errors.New("more than one signature")
			}
			sigEl = child
		}
	}
	if sigEl == nil {
		return nil, errors.New("no signature")
	}
	siEl := dsigChild(sigEl, dsig.SignedInfoTag)
	valueEl := dsigChild(sigEl, dsig.SignatureValueTag)
	if siEl == nil || valueEl == nil || dsigChild(siEl, dsig.CanonicalizationMethodTag) == nil {
		return nil, errors.New("incomplete signature")
	}

	signedInfo, err := canonicalize(siEl, dsigChild(siEl, dsig.CanonicalizationMethodTag).SelectAttrValue(dsig.AlgorithmAttr, ""), "")
	if err != nil {
		return nil, fmt.Errorf("SignedInfo: %w", err)
	}
	sigValue, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(valueEl.Text()), ""))
	if err != nil {
		return nil, fmt.Errorf("SignatureValue: %w", err)
	}
	// Only the verified canonical SignedInfo names the references.
	var si types.SignedInfo
	if err := xml.Unmarshal(signedInfo, &si); err != nil {
		return nil, fmt.Errorf("SignedInfo: %w", err)
	}
	if err := checkSignedInfo(si.SignatureMethod.Algorithm, signedInfo, sigValue, signers, at); err != nil {
		return nil, err
	}

	var content []byte
	for _, ref := range si.References {
		var el *etree.Element
		switch {
		case ref.URI == "":
			el = root.Copy()
		case strings.HasPrefix(ref.URI, "#"):
			if el = findByID(sigEl, ref.URI[1:]); el == nil {
				return nil, fmt.Errorf("reference %s: no such element in the signature", ref.URI)
			}
		default:
			return nil, fmt.Errorf("reference %s: not supported", ref.URI)
		}
		c14nAlg, prefixList := dsig.CanonicalXML10RecAlgorithmId, ""
		for _, tr := range ref.Transforms.Transforms {
			if tr.Algorithm == envelopedSignatureTransform {
				if ref.URI != "" {
					return nil, fmt.Errorf("reference %s: enveloped-signature transform", ref.URI)
				}
				el.RemoveChildAt(sigEl.Index())
				continue
			}
			c14nAlg = dsig.AlgorithmID(tr.Algorithm)
			if tr.InclusiveNamespaces != nil {
				prefixList = tr.InclusiveNamespaces.PrefixList
			}
		}
		data, err := canonicalize(el, string(c14nAlg), prefixList)
		if err != nil {
			return nil, fmt.Errorf("reference %s: %w", ref.URI, err)
		}
		hash, ok := xmldsigDigests[ref.DigestAlgo.Algorithm]
		if !ok {
			return nil, fmt.Errorf("reference %s: digest method %s not supported", ref.URI, ref.DigestAlgo.Algorithm)
		}
		want, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(ref.DigestValue), ""))
		if err != nil {
			return nil, fmt.Errorf("reference %s: DigestValue: %w", ref.URI, err)
		}
		h := hash.New()
		h.Write(data)
		if !bytes.Equal(h.Sum(nil), want) {
			return nil, fmt.Errorf("reference %s: digest mismatch", ref.URI)
		}
		if ref.URI == "" {
			content = data
		}
	}
	if content == nil {
		return nil, errors.New("the signature does not cover the document")
	}
	return content, nil
}

// checkSignedInfo verifies the signature value over the canonical
// SignedInfo with the first of signers that is valid at time at and whose
// key verifies it.
func checkSignedInfo(method string, signedInfo, sigValue []byte, signers []*x509.Certificate, at time.Time) error {
	m, ok := xmldsigMethods[method]
	if !ok {
		return fmt.Errorf("signature method %s not supported", method)
	}
	h := m.hash.New()
	h.Write(signedInfo)
	digest := h.Sum(nil)
	for _, cert := range signers {
		if at.Before(cert.NotBefore) || at.After(cert.NotAfter) {
			continue
		}
		key := cert.PublicKey
		if key == nil && m.key == "pss" {
			key = rsaPSSKey(cert)
		}
		switch key := key.(type) {
		case *rsa.PublicKey:
			if m.key == "rsa" && rsa.VerifyPKCS1v15(key, m.hash, digest, sigValue) == nil ||
				m.key == "pss" && rsa.VerifyPSS(key, m.hash, digest, sigValue, nil) == nil {
				return nil
			}
		case *ecdsa.PublicKey:
			half := len(sigValue) / 2
			if m.key == "ecdsa" && len(sigValue)%2 == 0 &&
				ecdsa.Verify(key, digest, new(big.Int).SetBytes(sigValue[:half]), new(big.Int).SetBytes(sigValue[half:])) {
				return nil
			}
		}
	}
	return fmt.Errorf("no signing certificate named for this list (%d, valid at %s) verifies the signature", len(signers), at.UTC().Format(time.RFC3339))
}

var oidRSASSAPSS = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 10}

// rsaPSSKey returns the RSA key of a certificate whose key has the
// RSASSA-PSS algorithm identifier (RFC 4055), as the German trusted list
// signer has. crypto/x509 does not parse that key. It returns nil for any
// other certificate.
func rsaPSSKey(cert *x509.Certificate) crypto.PublicKey {
	var spki struct {
		Algorithm pkix.AlgorithmIdentifier
		PublicKey asn1.BitString
	}
	if _, err := asn1.Unmarshal(cert.RawSubjectPublicKeyInfo, &spki); err != nil || !spki.Algorithm.Algorithm.Equal(oidRSASSAPSS) {
		return nil
	}
	key, err := x509.ParsePKCS1PublicKey(spki.PublicKey.Bytes)
	if err != nil {
		return nil
	}
	return key
}

// canonicalize returns the canonical form of el. A nested element first
// gets the namespace declarations in scope of its parent.
func canonicalize(el *etree.Element, alg, prefixList string) ([]byte, error) {
	var c dsig.Canonicalizer
	switch dsig.AlgorithmID(alg) {
	case dsig.CanonicalXML10ExclusiveAlgorithmId:
		c = dsig.MakeC14N10ExclusiveCanonicalizerWithPrefixList(prefixList)
	case dsig.CanonicalXML10ExclusiveWithCommentsAlgorithmId:
		c = dsig.MakeC14N10ExclusiveWithCommentsCanonicalizerWithPrefixList(prefixList)
	case dsig.CanonicalXML10RecAlgorithmId:
		c = dsig.MakeC14N10RecCanonicalizer()
	case dsig.CanonicalXML10WithCommentsAlgorithmId:
		c = dsig.MakeC14N10WithCommentsCanonicalizer()
	case dsig.CanonicalXML11AlgorithmId:
		c = dsig.MakeC14N11Canonicalizer()
	case dsig.CanonicalXML11WithCommentsAlgorithmId:
		c = dsig.MakeC14N11WithCommentsCanonicalizer()
	default:
		return nil, fmt.Errorf("canonicalization %s not supported", alg)
	}
	if el.Parent() != nil {
		ctx, err := etreeutils.NSBuildParentContext(el)
		if err != nil {
			return nil, err
		}
		if el, err = etreeutils.NSDetatch(ctx, el); err != nil {
			return nil, err
		}
	}
	return c.Canonicalize(el)
}

func dsigChild(el *etree.Element, tag string) *etree.Element {
	for _, child := range el.ChildElements() {
		if child.Tag == tag && child.NamespaceURI() == dsig.Namespace {
			return child
		}
	}
	return nil
}

func findByID(el *etree.Element, id string) *etree.Element {
	if el.SelectAttrValue("Id", "") == id {
		return el
	}
	for _, child := range el.ChildElements() {
		if found := findByID(child, id); found != nil {
			return found
		}
	}
	return nil
}
