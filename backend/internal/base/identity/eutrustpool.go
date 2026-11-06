package identity

import (
	"context"
	"crypto/x509"
	_ "embed"
	"encoding/base64"
	"encoding/pem"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/beevik/etree"

	"digital-contracting-service/internal/base/safehttp"
)

// EU Trusted List constants (ETSI TS 119 612)
const (
	lotlURL       = "https://ec.europa.eu/tools/lotl/eu-lotl.xml"
	svcTypeCAQC   = "http://uri.etsi.org/TrstSvc/Svctype/CA/QC"
	statusGranted = "http://uri.etsi.org/TrstSvc/TrustedList/Svcstatus/granted"
	tslTypeLOTL   = "http://uri.etsi.org/TrstSvc/TrustedList/TSLType/EUlistofthelists"
	tslMimeType   = "application/vnd.etsi.tsl+xml"

	// lotlOJURL is the Official Journal publication of the LOTL signing
	// certificates in certs/lotl-oj.pem.
	lotlOJURL = "https://eur-lex.europa.eu/eli/C/2026/1944/oj"

	// tslHTTPTimeout limits a single list download.
	tslHTTPTimeout = 30 * time.Second

	// tslMaxBodySize limits the size of a single TSL document to protect
	// against oversized responses (the largest national lists are a few MB).
	tslMaxBodySize = 32 << 20 // 32 MiB

	// DefaultRefreshInterval is a sensible default for periodic refreshes.
	// The national lists announce their own NextUpdate, typically weeks in
	// the future, so daily is more than fresh enough.
	DefaultRefreshInterval = 24 * time.Hour
)

// tslClient is the HTTP client used for all trusted-list downloads. It also
// accepts TLS 1.2: each member state hosts its national trusted list on its
// own server, and not all of them offer TLS 1.3. A trusted list is an XML
// document the scheme operator signs, and verifyTSL verifies that signature.
var tslClient = &http.Client{Timeout: tslHTTPTimeout, Transport: safehttp.TLS12Transport()}

//go:embed certs/lotl-oj.pem
var lotlOJCertsPEM []byte

// tslPointer is a pointer of the LOTL to a trusted list (OtherTSLPointer).
// Signers are the certificates that may sign that list.
type tslPointer struct {
	Location string   `xml:"TSLLocation"`
	Signers  []string `xml:"ServiceDigitalIdentities>ServiceDigitalIdentity>DigitalId>X509Certificate"`
	Type     string   `xml:"AdditionalInformation>OtherInformation>TSLType"`
	MimeType string   `xml:"AdditionalInformation>OtherInformation>MimeType"`
}

// tsl is a minimal mapping of an ETSI TS 119 612 trust status list (both
// the LOTL and the national trusted lists share this structure).
type tsl struct {
	IssueTime  string       `xml:"SchemeInformation>ListIssueDateTime"`
	SchemeURIs []string     `xml:"SchemeInformation>SchemeInformationURI>URI"`
	Pointers   []tslPointer `xml:"SchemeInformation>PointersToOtherTSL>OtherTSLPointer"`
	Services   []struct {
		Type   string   `xml:"ServiceInformation>ServiceTypeIdentifier"`
		Status string   `xml:"ServiceInformation>ServiceStatus"`
		Certs  []string `xml:"ServiceInformation>ServiceDigitalIdentity>DigitalId>X509Certificate"`
	} `xml:"TrustServiceProviderList>TrustServiceProvider>TSPServices>TSPService"`
}

// EUTrustPool holds the certificates of all qualified trust service
// providers (CA/QC, status "granted") from the EU Trusted Lists and keeps
// them up to date. It is safe for concurrent use.
//
// Typical usage:
//
//	tp := identity.NewEUTrustPool()
//	if err := tp.Refresh(ctx); err != nil { ... }   // initial build
//	go tp.StartAutoRefresh(ctx, identity.DefaultRefreshInterval)
//
//	// per verification:
//	err := doc.VerifyEIDASCertificate(tp)
type EUTrustPool struct {
	mu          sync.RWMutex
	pool        *x509.CertPool
	certCount   int
	lastRefresh time.Time
	lastErrs    []error
}

// NewEUTrustPool creates an empty, not yet populated trust pool.
// Call Refresh before first use.
func NewEUTrustPool() *EUTrustPool {
	return &EUTrustPool{}
}

// Pool returns the current certificate pool, or nil if the pool has never
// been successfully refreshed. With a nil pool VerifyEIDASCertificate skips
// both chain validation and the eIDAS QcCompliance check entirely — only
// hostname and JWK match remain, which is not an eIDAS validation.
func (t *EUTrustPool) Pool() *x509.CertPool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.pool
}

// Stats returns the number of certificates in the pool, the time of the
// last successful refresh, and the non-fatal errors collected during it
// (e.g. individual national lists that were unreachable).
func (t *EUTrustPool) Stats() (certCount int, lastRefresh time.Time, errs []error) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.certCount, t.lastRefresh, t.lastErrs
}

// Refresh rebuilds the pool from the EU Trusted Lists. On success the new
// pool atomically replaces the old one; on failure the previous pool is
// kept, so a temporarily unreachable LOTL does not degrade a running
// service.
func (t *EUTrustPool) Refresh(ctx context.Context) error {
	ojURL, anchors, err := lotlAnchors()
	if err != nil {
		return err
	}
	pool, count, errs, err := buildEUTrustPool(ctx, lotlURL, ojURL, anchors)
	if err != nil {
		return err
	}

	t.mu.Lock()
	t.pool = pool
	t.certCount = count
	t.lastRefresh = time.Now()
	t.lastErrs = errs
	t.mu.Unlock()

	return nil
}

// StartAutoRefresh refreshes the pool periodically until ctx is cancelled.
// Failed refreshes are logged and retried at the next interval; the last
// good pool stays in place. Intended to be run as a goroutine.
func (t *EUTrustPool) StartAutoRefresh(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = DefaultRefreshInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := t.Refresh(ctx); err != nil {
				log.Printf("eu trust pool refresh failed: %v", err)
			}
		}
	}
}

// lotlAnchors returns the Official Journal publication and the LOTL signing
// certificates that it names: the embedded certs/lotl-oj.pem, or the PEM file
// DCS_LOTL_OJ_CERTS_FILE and the URL DCS_LOTL_OJ_URL. A new publication
// replaces both values together.
func lotlAnchors() (string, []*x509.Certificate, error) {
	ojURL := strings.TrimSpace(os.Getenv("DCS_LOTL_OJ_URL"))
	if ojURL == "" {
		ojURL = lotlOJURL
	}
	pemBytes := lotlOJCertsPEM
	if path := strings.TrimSpace(os.Getenv("DCS_LOTL_OJ_CERTS_FILE")); path != "" {
		b, err := os.ReadFile(path) // #nosec G304 G703 -- path comes from operator configuration (environment), not from a request
		if err != nil {
			return "", nil, fmt.Errorf("DCS_LOTL_OJ_CERTS_FILE %s is unreadable: %w", path, err)
		}
		pemBytes = b
	}
	var certs []*x509.Certificate
	for block, rest := pem.Decode(pemBytes); block != nil; block, rest = pem.Decode(rest) {
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return "", nil, fmt.Errorf("parsing LOTL signing certificate: %w", err)
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return "", nil, errors.New("no LOTL signing certificate configured")
	}
	return ojURL, certs, nil
}

// buildEUTrustPool fetches the List of Trusted Lists (LOTL) at lotlLocation,
// verifies its signature (see fetchLOTL), follows the pointers to the
// national trusted lists, verifies the signature of each list against the
// signing certificates that the LOTL names for it, and collects the
// certificates of all trust services of type CA/QC with status "granted".
//
// It returns the pool, the number of certificates added, and any non-fatal
// errors (individual lists that could not be fetched, verified or parsed).
// A hard error is returned only if the LOTL itself is unavailable or not
// verified, or if the resulting pool would be empty.
func buildEUTrustPool(ctx context.Context, lotlLocation, ojURL string, anchors []*x509.Certificate) (*x509.CertPool, int, []error, error) {
	lotl, err := fetchLOTL(ctx, lotlLocation, ojURL, anchors)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("fetching LOTL: %w", err)
	}

	pool := x509.NewCertPool()
	added := 0
	var errs []error

	for _, p := range lotl.Pointers {
		if err := ctx.Err(); err != nil {
			return nil, 0, nil, err
		}

		location := strings.TrimSpace(p.Location)
		if strings.TrimSpace(p.Type) == tslTypeLOTL || strings.TrimSpace(p.MimeType) != tslMimeType {
			continue // the LOTL itself, or a human-readable (PDF) list
		}

		signers, err := parseCertificates(p.Signers)
		if err != nil {
			errs = append(errs, fmt.Errorf("TSL %s: signing certificates in the LOTL: %w", location, err))
			continue
		}
		national, err := fetchTSL(ctx, location, signers, false)
		if err != nil {
			// Tolerate individual unreachable or unverified lists, but report them.
			errs = append(errs, fmt.Errorf("fetching TSL %s: %w", location, err))
			continue
		}

		for _, svc := range national.Services {
			if strings.TrimSpace(svc.Type) != svcTypeCAQC ||
				strings.TrimSpace(svc.Status) != statusGranted {
				continue
			}
			for _, b64 := range svc.Certs {
				cert, err := parseCertificate(b64)
				if err != nil {
					errs = append(errs, fmt.Errorf("TSL %s: %w", location, err))
					continue
				}
				pool.AddCert(cert)
				added++
			}
		}
	}

	if added == 0 {
		return nil, 0, nil, errors.Join(append([]error{errors.New("EU trust pool is empty")}, errs...)...)
	}
	return pool, added, errs, nil
}

// fetchLOTL fetches the LOTL and verifies its signature with the pivot
// mechanism of the European Commission. The certificates of the Official
// Journal publication ojURL sign the first LOTL after that publication. The
// LOTL lists in its SchemeInformationURI the pivot LOTLs, newest first,
// before ojURL. Each pivot LOTL names in its pointer to itself the
// certificates that sign the next LOTL. fetchLOTL verifies the pivots from
// the oldest to the newest and then the current LOTL.
func fetchLOTL(ctx context.Context, location, ojURL string, signers []*x509.Certificate) (*tsl, error) {
	body, err := download(ctx, location)
	if err != nil {
		return nil, err
	}
	var unverified tsl
	if err := xml.Unmarshal(body, &unverified); err != nil {
		return nil, fmt.Errorf("parsing TSL: %w", err)
	}
	pivots, err := pivotURLs(unverified.SchemeURIs, ojURL)
	if err != nil {
		return nil, err
	}
	for i := len(pivots) - 1; i >= 0; i-- {
		pivot, err := fetchTSL(ctx, pivots[i], signers, true)
		if err != nil {
			return nil, fmt.Errorf("pivot LOTL %s: %w", pivots[i], err)
		}
		if signers, err = lotlSigners(pivot); err != nil {
			return nil, fmt.Errorf("pivot LOTL %s: %w", pivots[i], err)
		}
	}
	return verifyTSL(body, signers, false)
}

// pivotURLs returns the pivot LOTLs that the LOTL lists before the Official
// Journal publication ojURL, newest first.
func pivotURLs(schemeURIs []string, ojURL string) ([]string, error) {
	var pivots []string
	for _, uri := range schemeURIs {
		uri = strings.TrimSpace(uri)
		if uri == ojURL {
			return pivots, nil
		}
		if strings.HasSuffix(uri, ".xml") {
			pivots = append(pivots, uri)
		}
	}
	return nil, fmt.Errorf("the LOTL does not name the Official Journal publication %s of its signing certificates; set DCS_LOTL_OJ_URL and DCS_LOTL_OJ_CERTS_FILE to the current publication", ojURL)
}

// lotlSigners returns the certificates that a LOTL names in its pointer to
// itself.
func lotlSigners(lotl *tsl) ([]*x509.Certificate, error) {
	for _, p := range lotl.Pointers {
		if strings.TrimSpace(p.Type) == tslTypeLOTL {
			return parseCertificates(p.Signers)
		}
	}
	return nil, errors.New("no pointer to the LOTL")
}

// fetchTSL fetches a trust status list (LOTL or national TSL) and verifies
// its signature against signers (see verifyTSL).
func fetchTSL(ctx context.Context, tslURL string, signers []*x509.Certificate, atIssueTime bool) (*tsl, error) {
	body, err := download(ctx, tslURL)
	if err != nil {
		return nil, err
	}
	return verifyTSL(body, signers, atIssueTime)
}

func download(ctx context.Context, tslURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tslURL, nil)
	if err != nil {
		return nil, err
	}
	// Some member states serve their list as text/plain, and some refuse
	// the default Go user agent.
	req.Header.Set("Accept", tslMimeType+", application/xml, text/xml, */*")
	req.Header.Set("User-Agent", "digital-contracting-service")

	resp, err := tslClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s from %s", resp.Status, tslURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, tslMaxBodySize))
	if err != nil {
		return nil, fmt.Errorf("reading TSL: %w", err)
	}
	return body, nil
}

// verifyTSL verifies the enveloped XML signature of a trust status list and
// parses the signed content only. The signing certificate must be one of
// signers and valid now, or at the ListIssueDateTime of the list when
// atIssueTime is set (a pivot LOTL keeps the signature of its issue date).
func verifyTSL(body []byte, signers []*x509.Certificate, atIssueTime bool) (*tsl, error) {
	doc := etree.NewDocument()
	if err := doc.ReadFromBytes(body); err != nil {
		return nil, fmt.Errorf("parsing TSL: %w", err)
	}
	if doc.Root() == nil {
		return nil, errors.New("parsing TSL: no root element")
	}

	at := time.Now()
	if atIssueTime {
		var unverified tsl
		if err := xml.Unmarshal(body, &unverified); err != nil {
			return nil, fmt.Errorf("parsing TSL: %w", err)
		}
		issued, err := time.Parse(time.RFC3339, strings.TrimSpace(unverified.IssueTime))
		if err != nil {
			return nil, fmt.Errorf("parsing ListIssueDateTime: %w", err)
		}
		at = issued
	}
	signed, err := verifyEnvelopedSignature(doc.Root(), signers, at)
	if err != nil {
		return nil, fmt.Errorf("verifying TSL signature: %w", err)
	}

	var list tsl
	if err := xml.Unmarshal(signed, &list); err != nil {
		return nil, fmt.Errorf("parsing TSL: %w", err)
	}
	return &list, nil
}

// parseCertificates returns the signing certificates of a pointer that
// crypto/x509 can parse (it does not parse brainpool keys, for example).
func parseCertificates(b64s []string) ([]*x509.Certificate, error) {
	certs := make([]*x509.Certificate, 0, len(b64s))
	errs := []error{errors.New("no usable certificate")}
	for _, b64 := range b64s {
		cert, err := parseCertificate(b64)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return nil, errors.Join(errs...)
	}
	return certs, nil
}

// parseCertificate parses a base64 DER certificate of a trusted list. The
// XML may contain whitespace and newlines inside the base64.
func parseCertificate(b64 string) (*x509.Certificate, error) {
	der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(b64), ""))
	if err != nil {
		return nil, fmt.Errorf("decoding certificate: %w", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("parsing certificate: %w", err)
	}
	return cert, nil
}
