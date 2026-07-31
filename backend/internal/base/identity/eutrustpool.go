package identity

import (
	"context"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// EU Trusted List constants (ETSI TS 119 612)
const (
	lotlURL       = "https://ec.europa.eu/tools/lotl/eu-lotl.xml"
	svcTypeCAQC   = "http://uri.etsi.org/TrstSvc/Svctype/CA/QC"
	statusGranted = "http://uri.etsi.org/TrstSvc/TrustedList/Svcstatus/granted"

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

// tslClient is the HTTP client used for all trusted-list downloads.
var tslClient = &http.Client{Timeout: tslHTTPTimeout}

// tsl is a minimal mapping of an ETSI TS 119 612 trust status list (both
// the LOTL and the national trusted lists share this structure).
type tsl struct {
	Pointers []struct {
		Location string `xml:"TSLLocation"`
	} `xml:"SchemeInformation>PointersToOtherTSL>OtherTSLPointer"`
	Services []struct {
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
	pool, count, errs, err := buildEUTrustPool(ctx)
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

// buildEUTrustPool fetches the List of Trusted Lists (LOTL), follows the
// pointers to the national trusted lists, and collects the certificates of
// all trust services of type CA/QC with status "granted".
//
// It returns the pool, the number of certificates added, and any non-fatal
// errors (individual lists that could not be fetched or parsed). A hard
// error is returned only if the LOTL itself is unavailable or the
// resulting pool would be empty.
//
// Note: this implementation trusts the TLS connection to the list servers.
// A strict implementation additionally verifies the XAdES signatures of
// the LOTL (against the pivot certificates published in the Official
// Journal of the EU) and of each national list.
func buildEUTrustPool(ctx context.Context) (*x509.CertPool, int, []error, error) {
	lotl, err := fetchTSL(ctx, lotlURL)
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
		if !strings.HasSuffix(location, ".xml") {
			continue // skip human-readable (PDF) pointers
		}

		national, err := fetchTSL(ctx, location)
		if err != nil {
			// Tolerate individual unreachable lists, but report them.
			errs = append(errs, fmt.Errorf("fetching TSL %s: %w", location, err))
			continue
		}

		for _, svc := range national.Services {
			if strings.TrimSpace(svc.Type) != svcTypeCAQC ||
				strings.TrimSpace(svc.Status) != statusGranted {
				continue
			}
			for _, b64 := range svc.Certs {
				// The XML may contain whitespace/newlines inside the base64.
				der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(b64), ""))
				if err != nil {
					errs = append(errs, fmt.Errorf("TSL %s: decoding certificate: %w", location, err))
					continue
				}
				cert, err := x509.ParseCertificate(der)
				if err != nil {
					errs = append(errs, fmt.Errorf("TSL %s: parsing certificate: %w", location, err))
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

// fetchTSL fetches and parses a trust status list (LOTL or national TSL).
func fetchTSL(ctx context.Context, tslURL string) (*tsl, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tslURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/xml, text/xml")

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

	var list tsl
	if err := xml.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("parsing TSL: %w", err)
	}
	return &list, nil
}
