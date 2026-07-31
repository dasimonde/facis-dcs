package compiler

import (
	"context"
	"regexp"
	"sync"
	"time"
)

type Signer interface {
	Sign(ctx context.Context, data []byte) ([]byte, error)
}

type glossaryTerm struct {
	Term       string
	Definition string
	TermURI    string // Full URI for the term (e.g. http://www.w3.org/ns/odrl/2/Policy)
}

type clauseSegment struct {
	Type     string // "prose", "ontology-link", "external-link", "typed-value"
	Text     string // For Prose and OntologyLink display
	Ref      string // For OntologyLink: prefixed term (e.g. "sosa:Sensor")
	Href     string // For ExternalLink: full URI
	Value    string // For TypedValue: scalar value
	Datatype string // For TypedValue: XSD or schema.org URI
	Unit     string // For TypedValue: optional unit URI
}

type clauseData struct {
	Segments []clauseSegment // Structured clause with typed segments
}

type sectionData struct {
	Heading     string
	Clauses     []clauseData
	Subsections []sectionData  // recursive; empty for leaf sections
	Order       []sectionChild // document order of this section's direct children
}

// sectionChild records one entry in a section's document-ordered child list.
// It references either a clause (IsSection=false) via its index into Clauses,
// or a subsection (IsSection=true) via its index into Subsections. This mirrors
// the ordered dcs:children of a LayoutNode, so content blocks and nested
// sections render in their authored order rather than clauses-then-subsections.
type sectionChild struct {
	IsSection bool
	Index     int
}

type sigFieldDef struct {
	Name  string
	Label string
}

type documentModel struct {
	Title           string
	Sections        []sectionData
	SignatureFields []sigFieldDef
	Glossary        []glossaryTerm
	NamespaceMap    map[string]string // Maps prefixes (e.g., "odrl") to URIs
	// EmbeddedPayload is the JSON-LD attachment carried into the PDF VERBATIM —
	// the exact bytes submitted, byte-preserved. pdf-core does not re-canonicalize
	// or otherwise transform it; it is opaque payload the renderer never reads
	// (the visible render comes from documentStructure, parsed separately).
	EmbeddedPayload []byte
	PayloadHash     string
	PayloadCID      string // CIDv1(raw,sha256) of EmbeddedPayload — the rendered backlink
	FileID          string
	ContractID      string    // @id from the JSON-LD root node
	CompiledAt      time.Time // timestamp embedded in the dcs.lifecycle assertion
}

type layoutLine struct {
	Text     string
	FontSize float64
	Kind     string
	Depth    int // section nesting depth; 0 = top-level, 1 = subsection, ...
}

type annotationRef struct {
	ObjectID    int
	Term        string
	Rect        [4]float64
	DestPageIdx int     // index into pages slice for internal glossary links; -1 if unused
	DestY       float64 // Y coordinate on DestPage for internal glossary links
	URI         string  // for external ontology links
}

type pageLayout struct {
	ObjectID    int
	ContentID   int
	Lines       []positionedLine
	Annotations []annotationRef
	SigFields   []sigFieldWidget
}

type sigFieldWidget struct {
	Name               string
	Label              string
	Rect               [4]float64
	WidgetObjectID     int
	AppearanceObjectID int
}

type positionedLine struct {
	Text           string
	FontSize       float64
	X              float64
	Y              float64
	MCID           int
	Kind           string
	PageIdx        int // zero-based index of the page this line was placed on
	SectionIdx     int // index of the section this line belongs to (-1 if not in a section)
	LocalClauseIdx int // index of the clause within its section (-1 if not a clause line)
	SectionDepth   int // 0 = top-level section, 1 = subsection, 2 = subsubsection, ...
}

type pdfObject struct {
	ID   int
	Data []byte
}

type objectIDs struct {
	catalogID        int
	pagesID          int
	acroFormID       int
	fontID           int
	iccID            int
	outputIntentID   int
	c2paEmbeddedID   int
	c2paFileSpecID   int
	embeddedFileID   int
	fileSpecID       int
	metadataID       int
	fontDescriptorID int
	fontFileID       int
}

type c2paExclusion struct {
	Start  int
	Length int
}

type bmffBox struct {
	Type    string
	Payload []byte
	Raw     []byte
}

type signingMaterial struct {
	certChainDER [][]byte
}

const (
	c2paStoreUUID  = "6332706100110010800000AA00389B71" // c2pa
	c2paManifUUID  = "63326D6100110010800000AA00389B71" // c2ma
	c2paUpdateUUID = "6332756D00110010800000AA00389B71" // c2um
	c2paAsrtUUID   = "6332617300110010800000AA00389B71" // c2as
	c2paClmUUID    = "6332636C00110010800000AA00389B71" // c2cl
	c2paSigUUID    = "6332637300110010800000AA00389B71" // c2cs
	cborUUID       = "63626F7200110010800000AA00389B71" // cbor
)

const (
	envOntologyBaseURL = "DCS_PDF_CORE_ONTOLOGY_BASE_URL"
	// envX5ChainPEM / envX5ChainPEMFile carry the dev CA leaf certificate whose
	// public key matches the backend's dcs-c2pa token key. pdf-core embeds it in
	// the COSE_Sign1 protected header as the RFC 9360 x5chain; it holds no private
	// key — the DCS backend signs the Sig_structure and posts it back.
	envX5ChainPEM     = "DCS_PDF_CORE_C2PA_X5CHAIN_PEM"
	envX5ChainPEMFile = "DCS_PDF_CORE_C2PA_X5CHAIN_PEM_FILE"
)

var (
	signingMaterialOnce   sync.Once
	signingMaterialCached signingMaterial
	signingMaterialErr    error
)

// startXrefPattern captures the byte offset in a trailer's startxref keyword.
// The whitespace between "startxref", the offset and "%%EOF" is matched
// permissively: PDF producers append the incremental-update trailer with
// varying end-of-line runs (digitorus/pdfsign, for one, emits a blank line
// between "startxref" and the offset), all of which are valid per ISO 32000.
var startXrefPattern = regexp.MustCompile(`startxref\s+([0-9]+)\s*%%EOF`)

// srgbICCProfile is a minimal 308-byte ICC v2 profile for an sRGB monitor
// device. It satisfies the structural requirements that strict PDF viewers such
// as Adobe Acrobat enforce when parsing the /DestOutputProfile stream of a PDF
// OutputIntent: a 128-byte header whose bytes 36-39 are "acsp", a tag count, a
// tag table with valid offsets, XYZ primary/white-point tags, and identity
// (count=0) curve tags shared across R/G/B channels.
var srgbICCProfile = []byte{
	// ---- ICC header (128 bytes) ----
	// [0-3]  Profile size = 308 (0x00000134)
	0x00, 0x00, 0x01, 0x34,
	// [4-7]  Preferred CMM type (0 = unspecified)
	0x00, 0x00, 0x00, 0x00,
	// [8-11] Version 2.1.0
	0x02, 0x10, 0x00, 0x00,
	// [12-15] Profile class 'mntr' (display device)
	0x6D, 0x6E, 0x74, 0x72,
	// [16-19] Color space 'RGB '
	0x52, 0x47, 0x42, 0x20,
	// [20-23] PCS 'XYZ '
	0x58, 0x59, 0x5A, 0x20,
	// [24-35] Creation date/time (zeros = unspecified)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// [36-39] Signature 'acsp' — required by ICC spec and validated by Acrobat
	0x61, 0x63, 0x73, 0x70,
	// [40-43] Primary platform (0 = unidentified)
	0x00, 0x00, 0x00, 0x00,
	// [44-47] Profile flags
	0x00, 0x00, 0x00, 0x00,
	// [48-51] Device manufacturer
	0x00, 0x00, 0x00, 0x00,
	// [52-55] Device model
	0x00, 0x00, 0x00, 0x00,
	// [56-63] Device attributes
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// [64-67] Rendering intent = 0 (perceptual)
	0x00, 0x00, 0x00, 0x00,
	// [68-79] PCS illuminant D50 in s15Fixed16: X=0.9642 Y=1.0000 Z=0.8249
	0x00, 0x00, 0xF6, 0xD6,
	0x00, 0x01, 0x00, 0x00,
	0x00, 0x00, 0xD3, 0x2B,
	// [80-83] Profile creator
	0x00, 0x00, 0x00, 0x00,
	// [84-99] Profile ID (MD5, not computed)
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	// [100-127] Reserved
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00,

	// ---- Tag count = 7 ----
	0x00, 0x00, 0x00, 0x07,

	// ---- Tag table (7 × 12 bytes; data starts at byte 216 = 0xD8) ----
	// Each entry: tag-signature[4] offset[4] size[4]
	// rXYZ at 0xD8 (216), size 20
	0x72, 0x58, 0x59, 0x5A, 0x00, 0x00, 0x00, 0xD8, 0x00, 0x00, 0x00, 0x14,
	// gXYZ at 0xEC (236), size 20
	0x67, 0x58, 0x59, 0x5A, 0x00, 0x00, 0x00, 0xEC, 0x00, 0x00, 0x00, 0x14,
	// bXYZ at 0x100 (256), size 20
	0x62, 0x58, 0x59, 0x5A, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x14,
	// wtpt at 0x114 (276), size 20
	0x77, 0x74, 0x70, 0x74, 0x00, 0x00, 0x01, 0x14, 0x00, 0x00, 0x00, 0x14,
	// rTRC at 0x128 (296), size 12 (shared identity curve)
	0x72, 0x54, 0x52, 0x43, 0x00, 0x00, 0x01, 0x28, 0x00, 0x00, 0x00, 0x0C,
	// gTRC at 0x128 (296), size 12 (shared)
	0x67, 0x54, 0x52, 0x43, 0x00, 0x00, 0x01, 0x28, 0x00, 0x00, 0x00, 0x0C,
	// bTRC at 0x128 (296), size 12 (shared)
	0x62, 0x54, 0x52, 0x43, 0x00, 0x00, 0x01, 0x28, 0x00, 0x00, 0x00, 0x0C,

	// ---- Tag data ----

	// rXYZ: sRGB red primary (D50-adapted) — 'XYZ ' type + reserved + X Y Z
	// X=0.4361 Y=0.2225 Z=0.0139 in s15Fixed16
	0x58, 0x59, 0x5A, 0x20, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x6F, 0xA8, 0x00, 0x00, 0x38, 0xF6, 0x00, 0x00, 0x03, 0x8F,

	// gXYZ: sRGB green primary (D50-adapted)
	// X=0.3856 Y=0.7169 Z=0.0970
	0x58, 0x59, 0x5A, 0x20, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x62, 0xB5, 0x00, 0x00, 0xB7, 0x84, 0x00, 0x00, 0x18, 0xDA,

	// bXYZ: sRGB blue primary (D50-adapted)
	// X=0.1430 Y=0.0606 Z=0.7141
	0x58, 0x59, 0x5A, 0x20, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x24, 0xA0, 0x00, 0x00, 0x0F, 0x84, 0x00, 0x00, 0xB6, 0xCC,

	// wtpt: D50 white point
	// X=0.9642 Y=1.0000 Z=0.8249
	0x58, 0x59, 0x5A, 0x20, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xF6, 0xD6, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0xD3, 0x2B,

	// rTRC/gTRC/bTRC (shared): 'curv' type, count=0 means identity (linear) curve
	0x63, 0x75, 0x72, 0x76, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}
