package compiler

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
)

// What a reader renders for a page is the page dictionary and every object it
// reaches: the content streams, the resources (fonts, images, form XObjects),
// the annotations and their appearance streams, and the attributes the page
// inherits through /Parent. The comparison below walks that graph on both sides
// in parallel, each object at the definition a reader resolves.
//
// Two documents compare equal when their graphs have the same shape and the
// same values. What is NOT compared is what a re-save may change without
// changing what is rendered:
//   - object numbers (references are followed, not compared),
//   - the order of dictionary keys,
//   - the spelling of a name, string or number (#xx escapes, hex or literal
//     strings, 1 or 1.0),
//   - the encoding of a stream: /FlateDecode data is compared decoded, so a
//     stream compressed or recompressed by a re-save matches its plain original,
//   - how a page's content is split: a /Contents array is compared as the
//     concatenation of its streams, token by token.
//
// Objects held inside object streams (/Type /ObjStm) are not resolved: a copy
// re-saved with object streams reports a mismatch.
// Limit: no object-stream resolution; add it when a supported tool re-saves copies that way.

// pdfValue is one parsed PDF object.
type pdfValue struct {
	kind  byte // 'd' dictionary, 'a' array, 'r' reference, 'n' name, 's' string, 'k' number or keyword
	dict  map[string]pdfValue
	array []pdfValue
	ref   int
	text  string
}

// parsePDFValue parses the object starting at pos and returns it with the
// offset just past it.
func parsePDFValue(pdf []byte, pos, depth int) (pdfValue, int, error) {
	pos = skipPDFSpace(pdf, pos)
	if pos >= len(pdf) {
		return pdfValue{}, pos, fmt.Errorf("unexpected end of object")
	}
	if depth > maxObjectNesting {
		return pdfValue{}, pos, fmt.Errorf("object nests deeper than %d", maxObjectNesting)
	}
	switch {
	case bytes.HasPrefix(pdf[pos:], dictOpen):
		v := pdfValue{kind: 'd', dict: map[string]pdfValue{}}
		pos += len(dictOpen)
		for {
			pos = skipPDFSpace(pdf, pos)
			if pos >= len(pdf) {
				return pdfValue{}, pos, fmt.Errorf("unterminated dictionary")
			}
			if bytes.HasPrefix(pdf[pos:], dictClose) {
				return v, pos + len(dictClose), nil
			}
			if pdf[pos] != '/' {
				return pdfValue{}, pos, fmt.Errorf("dictionary key is not a name")
			}
			keyEnd := skipRegularToken(pdf, pos)
			key := decodePDFName(pdf[pos+1 : keyEnd])
			value, next, err := parsePDFValue(pdf, keyEnd, depth+1)
			if err != nil {
				return pdfValue{}, next, err
			}
			v.dict[key] = value
			pos = next
		}
	case pdf[pos] == '[':
		v := pdfValue{kind: 'a'}
		pos++
		for {
			pos = skipPDFSpace(pdf, pos)
			if pos >= len(pdf) {
				return pdfValue{}, pos, fmt.Errorf("unterminated array")
			}
			if pdf[pos] == ']' {
				return v, pos + 1, nil
			}
			element, next, err := parsePDFValue(pdf, pos, depth+1)
			if err != nil {
				return pdfValue{}, next, err
			}
			v.array = append(v.array, element)
			pos = next
		}
	case pdf[pos] == '(':
		end := skipLiteralString(pdf, pos)
		return pdfValue{kind: 's', text: decodeLiteralString(pdf[pos:end])}, end, nil
	case pdf[pos] == '<':
		end := skipHexString(pdf, pos)
		return pdfValue{kind: 's', text: decodeHexString(pdf[pos+1 : end-1])}, end, nil
	case pdf[pos] == '/':
		end := skipRegularToken(pdf, pos)
		return pdfValue{kind: 'n', text: decodePDFName(pdf[pos+1 : end])}, end, nil
	}
	end := skipRegularToken(pdf, pos)
	if valueEnd, ok := skipPDFValue(pdf, pos, depth); ok && valueEnd > end {
		id, err := strconv.Atoi(string(pdf[pos:end]))
		if err != nil {
			return pdfValue{}, valueEnd, fmt.Errorf("invalid reference")
		}
		return pdfValue{kind: 'r', ref: id}, valueEnd, nil
	}
	token := string(pdf[pos:end])
	if f, err := strconv.ParseFloat(token, 64); err == nil {
		token = strconv.FormatFloat(f, 'g', -1, 64)
	}
	return pdfValue{kind: 'k', text: token}, end, nil
}

// decodePDFName resolves the #xx escapes of a name (7.3.5).
func decodePDFName(raw []byte) string {
	var out []byte
	for i := 0; i < len(raw); i++ {
		if raw[i] == '#' && i+2 < len(raw) {
			if b, err := hex.DecodeString(string(raw[i+1 : i+3])); err == nil {
				out = append(out, b[0])
				i += 2
				continue
			}
		}
		out = append(out, raw[i])
	}
	return string(out)
}

// decodeHexString returns the bytes of a hex string body (7.3.4.3).
func decodeHexString(raw []byte) string {
	digits := make([]byte, 0, len(raw)+1)
	for _, b := range raw {
		if !isPDFWhitespace(b) {
			digits = append(digits, b)
		}
	}
	if len(digits)%2 == 1 {
		digits = append(digits, '0')
	}
	out, err := hex.DecodeString(string(digits))
	if err != nil {
		return "<" + string(raw) + ">"
	}
	return string(out)
}

// decodeLiteralString returns the bytes of a literal string "(...)" with its
// escapes resolved (7.3.4.2).
func decodeLiteralString(raw []byte) string {
	if len(raw) >= 2 {
		raw = raw[1 : len(raw)-1]
	}
	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if c == '\r' {
			// An end-of-line in a literal string reads as a single LF.
			if i+1 < len(raw) && raw[i+1] == '\n' {
				i++
			}
			out = append(out, '\n')
			continue
		}
		if c != '\\' || i+1 >= len(raw) {
			out = append(out, c)
			continue
		}
		i++
		switch e := raw[i]; e {
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case '\r':
			if i+1 < len(raw) && raw[i+1] == '\n' {
				i++
			}
		case '\n':
		default:
			if e >= '0' && e <= '7' {
				n, digits := 0, 0
				for digits < 3 && i < len(raw) && raw[i] >= '0' && raw[i] <= '7' {
					n = n*8 + int(raw[i]-'0')
					i++
					digits++
				}
				i--
				out = append(out, byte(n))
				continue
			}
			out = append(out, e)
		}
	}
	return string(out)
}

// resolvedObject is one resolved indirect object: its value and, for a stream, its
// decoded data.
type resolvedObject struct {
	value  pdfValue
	stream []byte
	// encoded is set when the stream uses a filter this comparison does not
	// decode; stream then holds the raw data.
	encoded bool
}

// resolvePDFObject parses the definition of objID a reader resolves.
func resolvePDFObject(pdf []byte, objID int) (resolvedObject, error) {
	header, ok := lastObjectHeader(pdf, objID)
	if !ok {
		return resolvedObject{}, fmt.Errorf("object %d not found", objID)
	}
	value, _, err := parsePDFValue(pdf[:header.value], header.body, 0)
	if err != nil {
		return resolvedObject{}, fmt.Errorf("object %d: %w", objID, err)
	}
	obj := resolvedObject{value: value}
	if !header.hasStream {
		return obj, nil
	}
	raw := pdf[header.streamStart:header.streamEnd]
	filter, filtered := value.dict["Filter"]
	if filter.kind == 'a' && len(filter.array) == 1 {
		filter = filter.array[0]
	}
	_, hasParams := value.dict["DecodeParms"]
	switch {
	case !filtered:
		obj.stream = raw
	case filter.kind == 'n' && filter.text == "FlateDecode" && !hasParams:
		reader, err := zlib.NewReader(bytes.NewReader(raw))
		if err != nil {
			return resolvedObject{}, fmt.Errorf("object %d: inflate stream: %w", objID, err)
		}
		decoded, err := io.ReadAll(io.LimitReader(reader, 64<<20))
		if err != nil {
			return resolvedObject{}, fmt.Errorf("object %d: inflate stream: %w", objID, err)
		}
		obj.stream = decoded
	default:
		obj.stream, obj.encoded = raw, true
	}
	return obj, nil
}

// metadataStream returns the decoded XMP metadata stream the current catalog
// names, or nil when there is none.
func metadataStream(pdf []byte) []byte {
	rootID, ok := currentRootObjID(pdf)
	if !ok {
		return nil
	}
	catalog, err := resolvePDFObject(pdf, rootID)
	if err != nil || catalog.value.dict["Metadata"].kind != 'r' {
		return nil
	}
	metadata, err := resolvePDFObject(pdf, catalog.value.dict["Metadata"].ref)
	if err != nil || metadata.encoded {
		return nil
	}
	return metadata.stream
}

// pageGraphComparison compares the object graphs reached from the pages of a
// candidate and a reference document.
type pageGraphComparison struct {
	candidate, reference []byte
	// signatureLayer, when set, leaves signature widgets (/FT /Sig) out of the
	// /Annots of both sides: a PAdES signer adds or fills one, and what it shows
	// is the signer's own appearance.
	signatureLayer bool
	visited        map[[2]int]bool
}

// object compares the candidate object c with the reference object r.
func (g *pageGraphComparison) object(c, r int) error {
	pair := [2]int{c, r}
	if g.visited[pair] {
		return nil
	}
	g.visited[pair] = true
	cObj, err := resolvePDFObject(g.candidate, c)
	if err != nil {
		return fmt.Errorf("submitted PDF: %w", err)
	}
	rObj, err := resolvePDFObject(g.reference, r)
	if err != nil {
		return fmt.Errorf("reference PDF: %w", err)
	}
	if (cObj.stream == nil) != (rObj.stream == nil) {
		return fmt.Errorf("object %d: stream on one side only", r)
	}
	if cObj.stream != nil {
		if cObj.encoded || rObj.encoded {
			// An undecoded filter is compared as written: same filter, same parameters, same bytes.
			for _, key := range []string{"Filter", "DecodeParms"} {
				if err := g.value(cObj.value.dict[key], rObj.value.dict[key]); err != nil {
					return fmt.Errorf("object %d /%s: %w", r, key, err)
				}
			}
		}
		if !bytes.Equal(cObj.stream, rObj.stream) {
			return fmt.Errorf("object %d: stream data differs", r)
		}
	}
	if err := g.value(cObj.value, rObj.value); err != nil {
		return fmt.Errorf("object %d: %w", r, err)
	}
	return nil
}

// streamEncodingKeys are the stream dictionary entries that describe how the
// data is written rather than what it is; the data is compared decoded instead.
var streamEncodingKeys = map[string]bool{"Length": true, "Filter": true, "DecodeParms": true, "DL": true}

// value compares two parsed values, following references.
func (g *pageGraphComparison) value(c, r pdfValue) error {
	if c.kind != r.kind {
		return fmt.Errorf("value types differ")
	}
	switch c.kind {
	case 'r':
		return g.object(c.ref, r.ref)
	case 'a':
		if len(c.array) != len(r.array) {
			return fmt.Errorf("array of %d elements, want %d", len(c.array), len(r.array))
		}
		for i := range r.array {
			if err := g.value(c.array[i], r.array[i]); err != nil {
				return fmt.Errorf("[%d]: %w", i, err)
			}
		}
		return nil
	case 'd':
		return g.dictionary(c, r)
	}
	if c.text != r.text {
		return fmt.Errorf("%q, want %q", c.text, r.text)
	}
	return nil
}

// dictionary compares two dictionaries entry by entry. A page's /Contents is
// compared as rendered content, and a page's /Annots without signature widgets
// when the signature layer is tolerated.
func (g *pageGraphComparison) dictionary(c, r pdfValue) error {
	isPage := r.dict["Type"].kind == 'n' && r.dict["Type"].text == "Page"
	_, isStream := r.dict["Length"]
	keys := map[string]bool{}
	for key := range c.dict {
		keys[key] = true
	}
	for key := range r.dict {
		keys[key] = true
	}
	for key := range keys {
		if isStream && streamEncodingKeys[key] {
			continue
		}
		cv, cOK := c.dict[key]
		rv, rOK := r.dict[key]
		if cOK != rOK {
			return fmt.Errorf("/%s present on one side only", key)
		}
		var err error
		switch {
		case isPage && key == "Contents":
			err = g.pageContents(cv, rv)
		case isPage && key == "Annots" && g.signatureLayer:
			err = g.value(g.withoutSignatureWidgets(g.candidate, cv), g.withoutSignatureWidgets(g.reference, rv))
		default:
			err = g.value(cv, rv)
		}
		if err != nil {
			return fmt.Errorf("/%s: %w", key, err)
		}
	}
	return nil
}

// withoutSignatureWidgets returns the annotations array v of pdf without the
// signature widgets in it.
func (g *pageGraphComparison) withoutSignatureWidgets(pdf []byte, v pdfValue) pdfValue {
	if v.kind == 'r' {
		if obj, err := resolvePDFObject(pdf, v.ref); err == nil {
			v = obj.value
		}
	}
	if v.kind != 'a' {
		return v
	}
	kept := pdfValue{kind: 'a'}
	for _, element := range v.array {
		annotation := element
		if element.kind == 'r' {
			if obj, err := resolvePDFObject(pdf, element.ref); err == nil {
				annotation = obj.value
			}
		}
		if isSignatureWidget(pdf, annotation) {
			continue
		}
		kept.array = append(kept.array, element)
	}
	return kept
}

// isSignatureWidget reports whether annotation is the widget of a signature
// field: the field and its widget merged into one dictionary (/FT /Sig), or a
// widget whose /Parent is the signature field.
func isSignatureWidget(pdf []byte, annotation pdfValue) bool {
	if ft := annotation.dict["FT"]; ft.kind == 'n' && ft.text == "Sig" {
		return true
	}
	parent := annotation.dict["Parent"]
	if annotation.dict["Subtype"].text != "Widget" || parent.kind != 'r' {
		return false
	}
	field, err := resolvePDFObject(pdf, parent.ref)
	if err != nil {
		return false
	}
	ft := field.value.dict["FT"]
	return ft.kind == 'n' && ft.text == "Sig"
}

// pageContents compares two /Contents values as the content a reader renders:
// the decoded data of every stream, concatenated, split into tokens.
func (g *pageGraphComparison) pageContents(c, r pdfValue) error {
	cTokens, err := contentTokens(g.candidate, c)
	if err != nil {
		return fmt.Errorf("submitted PDF: %w", err)
	}
	rTokens, err := contentTokens(g.reference, r)
	if err != nil {
		return fmt.Errorf("reference PDF: %w", err)
	}
	if !bytes.Equal(cTokens, rTokens) {
		return &contentMismatch{candidate: cTokens, reference: rTokens}
	}
	return nil
}

// contentMismatch reports where two pages' content diverges.
type contentMismatch struct{ candidate, reference []byte }

func (m *contentMismatch) Error() string {
	return "content does not match compiled output " + firstDiffSnippet(m.candidate, m.reference)
}

// contentTokens returns the page content v refers to — one stream or an array
// of streams — as its tokens joined by single spaces.
func contentTokens(pdf []byte, v pdfValue) ([]byte, error) {
	refs := []pdfValue{v}
	if v.kind == 'a' {
		refs = v.array
	}
	var data []byte
	for _, ref := range refs {
		if ref.kind != 'r' {
			return nil, fmt.Errorf("/Contents entry is not a reference")
		}
		obj, err := resolvePDFObject(pdf, ref.ref)
		if err != nil {
			return nil, err
		}
		if obj.stream == nil || obj.encoded {
			return nil, fmt.Errorf("/Contents object %d is not a readable stream", ref.ref)
		}
		data = append(append(data, obj.stream...), '\n')
	}
	var out []byte
	for pos := skipPDFSpace(data, 0); pos < len(data); pos = skipPDFSpace(data, pos) {
		end, ok := skipPDFObject(data, pos, 0)
		if !ok || end <= pos {
			return nil, fmt.Errorf("content stream nests too deeply")
		}
		if len(out) > 0 {
			out = append(out, ' ')
		}
		out = append(out, data[pos:end]...)
		pos = end
	}
	return out, nil
}
