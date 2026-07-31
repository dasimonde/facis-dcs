// Command crlcheck materialises an X.509 CRL revocation of the dev signing
// certificate into the contract_signatures table: it parses the leaf serial
// from an x5chain PEM, checks it against a CRL (PEM or DER), and, if revoked,
// stamps cert_revoked_at on every not-yet-marked signature so
// /signature/validate reports a certificate-revocation finding (DCS-OR-C2PA-007).
//
// It is an ops/Job entry point, not an HTTP endpoint: revocation of a shared
// dev signing certificate is a fleet-wide event, not a per-request one.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"

	"digital-contracting-service/internal/base/crl"
)

func main() {
	crlPath := flag.String("crl", "", "path to the CRL file (PEM X509 CRL or DER)")
	certPath := flag.String("cert", "", "path to the signing x5chain/leaf certificate PEM")
	flag.Parse()

	if *crlPath == "" || *certPath == "" {
		fmt.Fprintln(os.Stderr, "crlcheck: -crl and -cert are required")
		os.Exit(1)
	}

	crlRaw, err := os.ReadFile(*crlPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "crlcheck: read CRL: %v\n", err)
		os.Exit(1)
	}
	crlDER, err := crl.ParseCRLDER(crlRaw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "crlcheck: %v\n", err)
		os.Exit(1)
	}

	chainPEM, err := os.ReadFile(*certPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "crlcheck: read cert: %v\n", err)
		os.Exit(1)
	}
	serial, err := crl.LeafSerial(chainPEM)
	if err != nil {
		fmt.Fprintf(os.Stderr, "crlcheck: %v\n", err)
		os.Exit(1)
	}

	revoked, err := crl.IsRevoked(crlDER, serial)
	if err != nil {
		fmt.Fprintf(os.Stderr, "crlcheck: %v\n", err)
		os.Exit(1)
	}
	if !revoked {
		fmt.Printf("crlcheck: leaf serial %s is not revoked; nothing to do\n", serial)
		return
	}

	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		fmt.Fprintln(os.Stderr, "crlcheck: DATABASE_URL is required")
		os.Exit(1)
	}
	db, err := sqlx.Connect("postgres", databaseURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "crlcheck: connect db: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	res, err := db.Exec(
		`UPDATE contract_signatures SET cert_revoked_at = NOW() WHERE cert_revoked_at IS NULL`,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "crlcheck: mark revoked: %v\n", err)
		os.Exit(1)
	}
	n, _ := res.RowsAffected()
	fmt.Printf("crlcheck: leaf serial %s revoked; marked %d signature(s) as certificate-revoked\n", serial, n)
}
