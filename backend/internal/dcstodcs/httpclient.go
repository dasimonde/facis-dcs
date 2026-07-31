package dcstodcs

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
	dcstodcsc "digital-contracting-service/gen/http/dcs_to_dcs/client"
	"digital-contracting-service/internal/base/conf"

	goahttp "goa.design/goa/v3/http"
)

type prefixDoer struct {
	prefix string
	inner  goahttp.Doer
}

func (d *prefixDoer) Do(req *http.Request) (*http.Response, error) {
	req.URL.Path = d.prefix + req.URL.Path
	return d.inner.Do(req)
}

// schemeFallbackDoer sends over https and retries over http only when the
// transport itself failed, mirroring the order identity.FetchDIDDocument already
// resolves did.json in. A peer named by did:web is reached over TLS wherever it
// terminates TLS — shipping a contract to a public peer must not put the PDF and
// its challenge-response signature on the wire in the clear, and an ingress that
// redirects http to https would otherwise be relied on to repair that after the
// fact — while a plain-http deployment (the BDD two-instance suite) still works.
//
// Only connection-level failures fall back: an HTTP response, whatever its
// status, means the peer already handled this request and must not receive a
// second copy of it.
type schemeFallbackDoer struct {
	inner goahttp.Doer
}

func (d *schemeFallbackDoer) Do(req *http.Request) (*http.Response, error) {
	req.URL.Scheme = "https"
	resp, err := d.inner.Do(req)
	if err == nil {
		return resp, nil
	}
	// A body that cannot be rewound makes the request unreplayable, so the https
	// failure is the only answer we can honestly give.
	if req.Body != nil && req.GetBody == nil {
		return resp, err
	}
	retry := req.Clone(req.Context())
	retry.URL.Scheme = "http"
	if req.GetBody != nil {
		body, bodyErr := req.GetBody()
		if bodyErr != nil {
			return resp, err
		}
		retry.Body = body
	}
	retryResp, retryErr := d.inner.Do(retry)
	if retryErr != nil {
		return retryResp, fmt.Errorf("peer unreachable over https (%v) and over http: %w", err, retryErr)
	}
	return retryResp, nil
}

// NewDCSToDCSHttpClient builds the Goa-generated DCS-to-DCS client used to call
// post_pdf/get_provenance on a remote peer resolved from its did:web identifier
// (see identity.DIDWebPath). Requests are authenticated via a per-call did:web
// challenge-response signature carried in the request body, not via this HTTP
// client — there is no bearer token here.
//
// pathPrefix carries the peer's own did:web path segments, so a peer sharing a
// host with other instances is still addressed at its own base rather than
// whichever instance happens to own the host root.
func NewDCSToDCSHttpClient(host string, pathPrefix string) *dcstodcs.Client {
	apiPath := os.Getenv("DCS_API_PATH")
	if apiPath == "" {
		apiPath = "/"
	}
	apiPath = strings.TrimSuffix(pathPrefix, "/") + apiPath
	httpClient := &http.Client{Timeout: conf.HTTPClientTimeout()}
	doer := &prefixDoer{prefix: apiPath, inner: &schemeFallbackDoer{inner: httpClient}}

	c := dcstodcsc.NewClient(
		"https",
		host,
		doer,
		goahttp.RequestEncoder,
		goahttp.ResponseDecoder,
		false,
	)
	postPdf := c.PostPdf()
	erase := c.Erase()
	getProvenance := c.GetProvenance()
	return dcstodcs.NewClient(postPdf, erase, getProvenance)
}
