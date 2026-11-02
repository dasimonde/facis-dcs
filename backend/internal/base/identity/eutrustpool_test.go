package identity

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/beevik/etree"
	dsig "github.com/russellhaering/goxmldsig"
)

const testOJURL = "https://eur-lex.example/oj"

type tslSigner struct {
	key  *rsa.PrivateKey
	cert *x509.Certificate
}

func newTSLSigner(t *testing.T, cn string) tslSigner {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return tslSigner{key: key, cert: cert}
}

func (s tslSigner) b64() string { return base64.StdEncoding.EncodeToString(s.cert.Raw) }

// sign adds an enveloped XML signature with exclusive canonicalization, as
// the EU trusted lists carry it.
func (s tslSigner) sign(t *testing.T, xmlDoc string) []byte {
	t.Helper()
	sc, err := dsig.NewSigningContext(s.key, [][]byte{s.cert.Raw})
	if err != nil {
		t.Fatal(err)
	}
	sc.Canonicalizer = dsig.MakeC14N10ExclusiveCanonicalizerWithPrefixList("")
	doc := etree.NewDocument()
	if err := doc.ReadFromString(xmlDoc); err != nil {
		t.Fatal(err)
	}
	signed, err := sc.SignEnveloped(doc.Root())
	if err != nil {
		t.Fatal(err)
	}
	doc.SetRoot(signed)
	b, err := doc.WriteToBytes()
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type pointerFixture struct {
	location, tslType, mimeType string
	signer                      tslSigner
}

// tslFixture renders a trust status list with the given scheme URIs,
// pointers and CA/QC services (granted).
func tslFixture(schemeURIs []string, pointers []pointerFixture, services ...tslSigner) string {
	var b strings.Builder
	b.WriteString(`<TrustServiceStatusList xmlns="http://uri.etsi.org/02231/v2#" xmlns:ns3="http://uri.etsi.org/02231/v2/additionaltypes#" Id="ID0001"><SchemeInformation>`)
	fmt.Fprintf(&b, `<ListIssueDateTime>%s</ListIssueDateTime><SchemeInformationURI>`, time.Now().UTC().Format(time.RFC3339))
	for _, u := range schemeURIs {
		fmt.Fprintf(&b, `<URI xml:lang="en">%s</URI>`, u)
	}
	b.WriteString(`</SchemeInformationURI><PointersToOtherTSL>`)
	for _, p := range pointers {
		fmt.Fprintf(&b, `<OtherTSLPointer><ServiceDigitalIdentities><ServiceDigitalIdentity><DigitalId><X509Certificate>%s</X509Certificate></DigitalId></ServiceDigitalIdentity></ServiceDigitalIdentities><TSLLocation>%s</TSLLocation><AdditionalInformation><OtherInformation><TSLType>%s</TSLType></OtherInformation><OtherInformation><ns3:MimeType>%s</ns3:MimeType></OtherInformation></AdditionalInformation></OtherTSLPointer>`,
			p.signer.b64(), p.location, p.tslType, p.mimeType)
	}
	b.WriteString(`</PointersToOtherTSL></SchemeInformation><TrustServiceProviderList><TrustServiceProvider><TSPServices>`)
	for _, s := range services {
		fmt.Fprintf(&b, `<TSPService><ServiceInformation><ServiceTypeIdentifier>%s</ServiceTypeIdentifier><ServiceStatus>%s</ServiceStatus><ServiceDigitalIdentity><DigitalId><X509Certificate>%s</X509Certificate></DigitalId></ServiceDigitalIdentity></ServiceInformation></TSPService>`,
			svcTypeCAQC, statusGranted, s.b64())
	}
	b.WriteString(`</TSPServices></TrustServiceProvider></TrustServiceProviderList></TrustServiceStatusList>`)
	return b.String()
}

// serveTSLs serves the documents by path over TLS and points tslClient at
// the test server.
func serveTSLs(t *testing.T, docs map[string][]byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc, ok := docs[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(doc)
	}))
	t.Cleanup(srv.Close)
	saved := tslClient
	tslClient = srv.Client()
	t.Cleanup(func() { tslClient = saved })
	return srv
}

func TestVerifyTSLAcceptsOnlyTheNamedSignerAndUntamperedContent(t *testing.T) {
	signer := newTSLSigner(t, "scheme operator")
	other := newTSLSigner(t, "someone else")
	ca := newTSLSigner(t, "qualified CA")
	signed := signer.sign(t, tslFixture(nil, nil, ca))

	list, err := verifyTSL(signed, []*x509.Certificate{signer.cert}, false)
	if err != nil {
		t.Fatalf("valid signature: %v", err)
	}
	if len(list.Services) != 1 || strings.TrimSpace(list.Services[0].Certs[0]) != ca.b64() {
		t.Fatalf("parsed services %+v, want the one CA/QC service", list.Services)
	}

	tampered := bytes.Replace(signed, []byte(statusGranted), []byte(statusGranted+"x"), 1)
	if _, err := verifyTSL(tampered, []*x509.Certificate{signer.cert}, false); err == nil {
		t.Fatal("tampered list verified")
	}

	if _, err := verifyTSL(signed, []*x509.Certificate{other.cert}, false); err == nil {
		t.Fatal("list verified against a certificate that did not sign it")
	}

	if _, err := verifyTSL([]byte(tslFixture(nil, nil, ca)), []*x509.Certificate{signer.cert}, false); err == nil {
		t.Fatal("unsigned list verified")
	}
}

// The member states sign with RSA PKCS #1 v1.5, RSASSA-PSS or ECDSA; an
// XML-DSig ECDSA signature value is r||s, not ASN.1.
func TestCheckSignedInfoSignatureMethods(t *testing.T) {
	signedInfo := []byte("<SignedInfo/>")
	digest := sha256.Sum256(signedInfo)
	rsaSigner := newTSLSigner(t, "rsa")

	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &ecKey.PublicKey, ecKey)
	if err != nil {
		t.Fatal(err)
	}
	ecCert, _ := x509.ParseCertificate(der)
	r, s, err := ecdsa.Sign(rand.Reader, ecKey, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	rs := append(r.FillBytes(make([]byte, 32)), s.FillBytes(make([]byte, 32))...)
	asn1Sig, _ := ecdsa.SignASN1(rand.Reader, ecKey, digest[:])
	pkcs1, _ := rsa.SignPKCS1v15(rand.Reader, rsaSigner.key, crypto.SHA256, digest[:])
	pss, _ := rsa.SignPSS(rand.Reader, rsaSigner.key, crypto.SHA256, digest[:], nil)

	for _, tc := range []struct {
		name, method string
		sig          []byte
		cert         *x509.Certificate
		ok           bool
	}{
		{"rsa", "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256", pkcs1, rsaSigner.cert, true},
		{"rsa-pss", "http://www.w3.org/2007/05/xmldsig-more#sha256-rsa-MGF1", pss, rsaSigner.cert, true},
		{"pss value under the PKCS #1 method", "http://www.w3.org/2001/04/xmldsig-more#rsa-sha256", pss, rsaSigner.cert, false},
		{"ecdsa r||s", "http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha256", rs, ecCert, true},
		{"ecdsa asn.1", "http://www.w3.org/2001/04/xmldsig-more#ecdsa-sha256", asn1Sig, ecCert, false},
		{"sha1", "http://www.w3.org/2000/09/xmldsig#rsa-sha1", pkcs1, rsaSigner.cert, false},
	} {
		err := checkSignedInfo(tc.method, signedInfo, tc.sig, []*x509.Certificate{tc.cert}, time.Now())
		if (err == nil) != tc.ok {
			t.Errorf("%s: err %v, want ok=%v", tc.name, err, tc.ok)
		}
	}
	if err := checkSignedInfo("http://www.w3.org/2001/04/xmldsig-more#rsa-sha256", signedInfo, pkcs1, []*x509.Certificate{rsaSigner.cert}, time.Now().Add(2*time.Hour)); err == nil {
		t.Error("signature verified with a certificate that is not valid at the verification time")
	}
}

func TestBuildEUTrustPoolRejectsANationalListWithAWrongSignature(t *testing.T) {
	oj := newTSLSigner(t, "LOTL signer")
	operatorA := newTSLSigner(t, "operator A")
	operatorB := newTSLSigner(t, "operator B")
	impostor := newTSLSigner(t, "impostor")
	caA := newTSLSigner(t, "CA A")
	caB := newTSLSigner(t, "CA B")

	docs := map[string][]byte{}
	srv := serveTSLs(t, docs)
	lotl := tslFixture([]string{testOJURL}, []pointerFixture{
		{srv.URL + "/lotl.xml", tslTypeLOTL, tslMimeType, oj},
		{srv.URL + "/a.xml", "http://uri.etsi.org/TrstSvc/TrustedList/TSLType/EUgeneric", tslMimeType, operatorA},
		{srv.URL + "/b.xml", "http://uri.etsi.org/TrstSvc/TrustedList/TSLType/EUgeneric", tslMimeType, operatorB},
	})
	docs["/lotl.xml"] = oj.sign(t, lotl)
	docs["/a.xml"] = operatorA.sign(t, tslFixture(nil, nil, caA))
	docs["/b.xml"] = impostor.sign(t, tslFixture(nil, nil, caB))

	pool, count, errs, err := buildEUTrustPool(context.Background(), srv.URL+"/lotl.xml", testOJURL, []*x509.Certificate{oj.cert})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("pool has %d certificates, want only the one of the list with a valid signature", count)
	}
	if _, err := caA.cert.Verify(x509.VerifyOptions{Roots: pool}); err != nil {
		t.Fatalf("CA of the verified list is not in the pool: %v", err)
	}
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "/b.xml") {
		t.Fatalf("errors %v, want one for /b.xml", errs)
	}

	if _, _, _, err := buildEUTrustPool(context.Background(), srv.URL+"/lotl.xml", testOJURL, []*x509.Certificate{impostor.cert}); err == nil {
		t.Fatal("LOTL accepted with a signer that the Official Journal does not name")
	}
}

func TestFetchLOTLFollowsThePivotLOTLs(t *testing.T) {
	oj := newTSLSigner(t, "signer named in the Official Journal")
	next := newTSLSigner(t, "signer named in the pivot LOTL")

	docs := map[string][]byte{}
	srv := serveTSLs(t, docs)
	docs["/pivot-1.xml"] = oj.sign(t, tslFixture([]string{testOJURL}, []pointerFixture{
		{srv.URL + "/lotl.xml", tslTypeLOTL, tslMimeType, next},
	}))
	docs["/lotl.xml"] = next.sign(t, tslFixture([]string{srv.URL + "/pivot-1.xml", testOJURL}, []pointerFixture{
		{srv.URL + "/lotl.xml", tslTypeLOTL, tslMimeType, next},
	}))
	// The same LOTL without the pivot: no chain from the Official Journal to
	// its signer.
	docs["/no-pivot.xml"] = next.sign(t, tslFixture([]string{testOJURL}, nil))
	// A LOTL that does not name the configured publication.
	docs["/other-oj.xml"] = oj.sign(t, tslFixture([]string{"https://eur-lex.example/newer-oj"}, nil))

	anchors := []*x509.Certificate{oj.cert}
	if _, err := fetchLOTL(context.Background(), srv.URL+"/lotl.xml", testOJURL, anchors); err != nil {
		t.Fatalf("LOTL with pivot: %v", err)
	}
	if _, err := fetchLOTL(context.Background(), srv.URL+"/no-pivot.xml", testOJURL, anchors); err == nil {
		t.Fatal("LOTL verified against a signer that neither the Official Journal nor a pivot names")
	}
	if _, err := fetchLOTL(context.Background(), srv.URL+"/other-oj.xml", testOJURL, anchors); err == nil {
		t.Fatal("LOTL verified although it does not name the configured Official Journal publication")
	}
}

func TestLOTLAnchorsEmbedTheOfficialJournalCertificates(t *testing.T) {
	ojURL, certs, err := lotlAnchors()
	if err != nil {
		t.Fatal(err)
	}
	if ojURL != lotlOJURL || len(certs) != 6 {
		t.Fatalf("anchors %s with %d certificates, want %s with 6", ojURL, len(certs), lotlOJURL)
	}
}

// The live EU LOTL and national lists verify against the pinned Official
// Journal certificates.
func TestEUTrustPoolLive(t *testing.T) {
	if os.Getenv("INTEGRATION") == "" {
		t.Skip("set INTEGRATION=1 to download and verify the live EU trusted lists")
	}
	ojURL, anchors, err := lotlAnchors()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	_, count, errs, err := buildEUTrustPool(ctx, lotlURL, ojURL, anchors)
	if err != nil {
		t.Fatal(err)
	}
	// An unreachable list or an unparsable certificate is tolerated; a list
	// whose signature does not verify is not.
	for _, e := range errs {
		if strings.Contains(e.Error(), "verifying TSL signature") {
			t.Errorf("%v", e)
		} else {
			t.Logf("skipped: %v", e)
		}
	}
	t.Logf("%d certificates, %d errors", count, len(errs))
}
