package compiler

import (
	"bytes"
	"strconv"
	"strings"
)

// Locating an indirect object by its id, per ISO 32000-1 7.3.10 and 7.3.8.
//
// A definition is "<id> <gen> obj", the tokens separated by ANY run of white
// space (7.2.3), followed by one object and — when that object is a stream —
// its raw data. Every lookup here resolves the definition a reader resolves and
// reads only bytes belonging to it, because each of the ways to get that wrong
// hands a checker bytes the rendered document does not contain:
//
//   - A bare substring match for "19 0 obj" also hits inside "100019 0 obj", so
//     a decoy object whose id merely ENDS in the wanted digits supplies the
//     bytes. A header therefore only counts when its id starts a line.
//   - A fixed "%d 0 obj" spelling misses "19  0 obj" and the superseding
//     "19 1 obj" — a freed and reused object comes back with a raised generation
//     — and silently falls back to an EARLIER definition.
//   - Stream data is arbitrary bytes. Contract text reaches the page content
//     stream and the contract.jsonld attachment VERBATIM, so a clause that says
//     "terminates at the endobj keyword" writes that keyword into the file. Any
//     search for "endobj", "endstream" or "stream" that can reach stream data
//     makes the contract's own words a structural marker: it truncates the
//     attachment at whatever the author wrote and returns the short bytes with
//     no error. A stream's extent is therefore taken ONLY from its dictionary's
//     /Length (7.3.8.2) — the sole terminator a reader uses — and no keyword is
//     sought inside stream data at all. "endobj" is never searched for.

// maxObjectNesting bounds how deeply composite objects may nest before a walk
// gives up. skipPDFObject and skipUntilClose descend once per "<<" or "[", so an
// unbounded document controls the walker's stack depth: a peer PDF nesting a few
// million arrays exhausts the goroutine stack, and a stack overflow is a runtime
// throw, not a panic — net/http's per-connection recover cannot catch it and the
// whole process dies. The deepest object this compiler emits measures 4 levels
// (TestObjectNestingOfARealPDFIsFarBelowTheLimit); the deepest a PAdES or DSS
// revision appends is a handful more. 256 is two orders of magnitude of headroom
// and still bounds the walk to a few tens of kilobytes of stack.
const maxObjectNesting = 256

var (
	objKeyword       = []byte("obj")
	streamKeyword    = []byte("stream")
	endstreamKeyword = []byte("endstream")
	dictOpen         = []byte("<<")
	dictClose        = []byte(">>")
	arrayClose       = []byte("]")
)

// isPDFWhitespace reports whether b is one of the six PDF white-space
// characters (ISO 32000-1 table 1).
func isPDFWhitespace(b byte) bool {
	switch b {
	case 0x00, '\t', '\n', '\f', '\r', ' ':
		return true
	}
	return false
}

// isPDFDelimiter reports whether b is one of the delimiter characters
// (ISO 32000-1 table 2), which terminate a keyword just as white space does.
func isPDFDelimiter(b byte) bool {
	switch b {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

// objectHeader is one definition of an indirect object.
type objectHeader struct {
	// start is the offset of the object id's first digit.
	start int
	// body is the offset at which the definition's content begins: past the
	// "obj" keyword and the end-of-line closing it.
	body int
	// value is the offset just past the object the definition holds — its
	// dictionary — and before any stream keyword.
	value int
	// streamStart and streamEnd bound the raw stream data, valid only when
	// hasStream is set.
	streamStart, streamEnd int
	hasStream              bool
}

// objectHeaders returns every definition of objID in pdf, in file order.
func objectHeaders(pdf []byte, objID int) []objectHeader {
	definitions := objectDefinitions(pdf, objID)
	headers := make([]objectHeader, 0, len(definitions))
	for _, definition := range definitions {
		if header, ok := resolveObject(pdf, definition); ok {
			headers = append(headers, header)
		}
	}
	return headers
}

// objectDefinitions returns every definition of objID with its start and body
// offsets only. Resolving what the definition HOLDS is separate so that an
// indirect /Length can be read without re-entering stream resolution.
func objectDefinitions(pdf []byte, objID int) []objectHeader {
	id := []byte(strconv.Itoa(objID))
	var definitions []objectHeader
	for searchFrom := 0; searchFrom+len(id) <= len(pdf); {
		rel := bytes.Index(pdf[searchFrom:], id)
		if rel < 0 {
			break
		}
		at := searchFrom + rel
		searchFrom = at + 1
		if at != 0 && pdf[at-1] != '\n' && pdf[at-1] != '\r' {
			continue
		}
		body, ok := objectHeaderBody(pdf, at+len(id))
		if !ok {
			continue
		}
		definitions = append(definitions, objectHeader{start: at, body: body})
	}
	return definitions
}

// resolveObject fills in what a definition holds: the extent of its object and,
// when that object is a stream, the range of its data. It reports false when the
// object nests past maxObjectNesting, so a definition no walker can bound is
// treated as absent rather than as one spanning the rest of the file.
func resolveObject(pdf []byte, header objectHeader) (objectHeader, bool) {
	valueStart := skipPDFSpace(pdf, header.body)
	value, ok := skipPDFObject(pdf, valueStart, 0)
	if !ok {
		return objectHeader{}, false
	}
	header.value = value
	start, end, ok := streamExtent(pdf, pdf[valueStart:header.value], header.value)
	if ok {
		header.streamStart, header.streamEnd, header.hasStream = start, end, true
	}
	return header, true
}

// streamExtent returns the range of the stream data following the object
// dictionary that ends at valueEnd. The keyword is only recognised where a
// stream may begin — directly after the dictionary — so a dictionary key named
// /stream can never be read as it, and the data's extent comes from /Length, so
// the data's own bytes are never inspected. The declared length must land on
// "endstream": a length that does not is a malformed object, reported as having
// no stream rather than answered with a guess.
func streamExtent(pdf, dict []byte, valueEnd int) (start, end int, ok bool) {
	pos := skipPDFSpace(pdf, valueEnd)
	if !bytes.HasPrefix(pdf[pos:], streamKeyword) {
		return 0, 0, false
	}
	pos += len(streamKeyword)
	switch {
	case pos < len(pdf) && pdf[pos] == '\n':
		pos++
	case pos+1 < len(pdf) && pdf[pos] == '\r' && pdf[pos+1] == '\n':
		pos += 2
	default:
		// 7.3.8.1 requires CRLF or LF after the keyword, never a bare CR.
		return 0, 0, false
	}
	// Compared as a remaining-bytes budget, never as pos+length: /Length
	// 9223372036854775807 makes that sum wrap negative, passing the guard and
	// indexing the slice below out of range.
	length, ok := streamLength(pdf, dict)
	if !ok || length > len(pdf)-pos {
		return 0, 0, false
	}
	if !bytes.HasPrefix(pdf[skipPDFSpace(pdf, pos+length):], endstreamKeyword) {
		return 0, 0, false
	}
	return pos, pos + length, true
}

// streamLength returns the declared length of the stream whose dictionary is
// dict, resolving an indirect reference against pdf.
func streamLength(pdf, dict []byte) (int, bool) {
	value, ok := dictEntry(dict, "Length")
	if !ok {
		return 0, false
	}
	if length, err := strconv.Atoi(string(value)); err == nil {
		return length, length >= 0
	}
	objID, ok := indirectReferenceID(value)
	if !ok {
		return 0, false
	}
	return indirectInteger(pdf, objID)
}

// indirectReferenceID returns the object id of an indirect reference "12 0 R".
func indirectReferenceID(value []byte) (int, bool) {
	fields := strings.Fields(string(value))
	if len(fields) != 3 || fields[2] != "R" {
		return 0, false
	}
	objID, err := strconv.Atoi(fields[0])
	if err != nil {
		return 0, false
	}
	return objID, true
}

// indirectInteger returns the value of the numeric object objID, the form a
// /Length reference points at. It reads the object's value only — never a
// stream — so a reference cycle cannot recurse.
func indirectInteger(pdf []byte, objID int) (int, bool) {
	definitions := objectDefinitions(pdf, objID)
	if len(definitions) == 0 {
		return 0, false
	}
	start := skipPDFSpace(pdf, definitions[len(definitions)-1].body)
	end, ok := skipPDFObject(pdf, start, 0)
	if !ok {
		return 0, false
	}
	value, err := strconv.Atoi(string(pdf[start:end]))
	if err != nil {
		return 0, false
	}
	return value, value >= 0
}

// dictEntry returns the value of key in the dictionary dict, searching its top
// level only so that a nested dictionary's entry of the same name — /Params
// << /Size ... >> beside /Length — is not mistaken for it.
func dictEntry(dict []byte, key string) ([]byte, bool) {
	if !bytes.HasPrefix(dict, dictOpen) {
		return nil, false
	}
	name := []byte("/" + key)
	for pos := len(dictOpen); pos < len(dict); {
		pos = skipPDFSpace(dict, pos)
		if pos >= len(dict) || dict[pos] != '/' {
			return nil, false
		}
		keyEnd := skipRegularToken(dict, pos)
		valueStart := skipPDFSpace(dict, keyEnd)
		valueEnd, ok := skipPDFValue(dict, valueStart, 0)
		if !ok {
			return nil, false
		}
		if bytes.Equal(dict[pos:keyEnd], name) {
			return dict[valueStart:valueEnd], true
		}
		if valueEnd <= pos {
			return nil, false
		}
		pos = valueEnd
	}
	return nil, false
}

// skipPDFValue returns the offset just past the value beginning at pos,
// treating the three tokens of an indirect reference ("12 0 R") as one value.
func skipPDFValue(pdf []byte, pos, depth int) (int, bool) {
	end, ok := skipPDFObject(pdf, pos, depth)
	if !ok {
		return 0, false
	}
	if !isDigits(pdf[pos:end]) {
		return end, true
	}
	generation := skipPDFSpace(pdf, end)
	generationEnd, ok := skipPDFObject(pdf, generation, depth)
	if !ok {
		return 0, false
	}
	if !isDigits(pdf[generation:generationEnd]) {
		return end, true
	}
	keyword := skipPDFSpace(pdf, generationEnd)
	if keyword >= len(pdf) || pdf[keyword] != 'R' {
		return end, true
	}
	if next := keyword + 1; next < len(pdf) && !isPDFWhitespace(pdf[next]) && !isPDFDelimiter(pdf[next]) {
		return end, true
	}
	return keyword + 1, true
}

func isDigits(token []byte) bool {
	if len(token) == 0 {
		return false
	}
	for _, b := range token {
		if b < '0' || b > '9' {
			return false
		}
	}
	return true
}

// skipPDFObject returns the offset just past the complete object beginning at
// pos. Composite objects are walked token by token so that a literal string's
// parens, a hex string's angle brackets and a name's characters are read as
// content and never as structure; an unterminated object consumes the rest of
// the file. depth is the number of composites already entered; nesting past
// maxObjectNesting reports false rather than descending further.
func skipPDFObject(pdf []byte, pos, depth int) (int, bool) {
	if pos >= len(pdf) {
		return len(pdf), true
	}
	if depth > maxObjectNesting {
		return 0, false
	}
	switch {
	case pdf[pos] == '(':
		return skipLiteralString(pdf, pos), true
	case bytes.HasPrefix(pdf[pos:], dictOpen):
		return skipUntilClose(pdf, pos+len(dictOpen), dictClose, depth+1)
	case pdf[pos] == '<':
		return skipHexString(pdf, pos), true
	case pdf[pos] == '[':
		return skipUntilClose(pdf, pos+1, arrayClose, depth+1)
	}
	return skipRegularToken(pdf, pos), true
}

// skipUntilClose walks the members of a dictionary or array until its closing
// token.
func skipUntilClose(pdf []byte, pos int, closer []byte, depth int) (int, bool) {
	for pos < len(pdf) {
		pos = skipPDFSpace(pdf, pos)
		if pos >= len(pdf) {
			break
		}
		if bytes.HasPrefix(pdf[pos:], closer) {
			return pos + len(closer), true
		}
		next, ok := skipPDFObject(pdf, pos, depth)
		if !ok {
			return 0, false
		}
		if next <= pos {
			break
		}
		pos = next
	}
	return len(pdf), true
}

// skipLiteralString walks a "(...)" string, honouring the backslash escapes and
// the balanced inner parens of 7.3.4.2.
func skipLiteralString(pdf []byte, pos int) int {
	depth := 0
	for ; pos < len(pdf); pos++ {
		switch pdf[pos] {
		case '\\':
			pos++
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return pos + 1
			}
		}
	}
	return len(pdf)
}

// skipHexString walks a "<...>" string.
func skipHexString(pdf []byte, pos int) int {
	if end := bytes.IndexByte(pdf[pos:], '>'); end >= 0 {
		return pos + end + 1
	}
	return len(pdf)
}

// skipRegularToken walks a name, a number or a keyword — the run of regular
// characters after an optional leading solidus. A stray delimiter is consumed
// alone so that a walk always makes progress.
func skipRegularToken(pdf []byte, pos int) int {
	if pos < len(pdf) && pdf[pos] == '/' {
		pos++
	}
	start := pos
	for pos < len(pdf) && !isPDFWhitespace(pdf[pos]) && !isPDFDelimiter(pdf[pos]) {
		pos++
	}
	if pos == start && pos < len(pdf) {
		pos++
	}
	return pos
}

// skipPDFSpace advances past white space and comments (7.2.3).
func skipPDFSpace(pdf []byte, pos int) int {
	for pos < len(pdf) {
		if isPDFWhitespace(pdf[pos]) {
			pos++
			continue
		}
		if pdf[pos] != '%' {
			return pos
		}
		for pos < len(pdf) && pdf[pos] != '\n' && pdf[pos] != '\r' {
			pos++
		}
	}
	return pos
}

// objectHeaderBody parses the rest of a header — white space, the generation
// number, white space, "obj" — starting just past the object id, and returns
// the offset at which the definition's content begins.
func objectHeaderBody(pdf []byte, pos int) (int, bool) {
	pos, ok := skipPDFWhitespace(pdf, pos)
	if !ok {
		return 0, false
	}
	generation := pos
	for pos < len(pdf) && pdf[pos] >= '0' && pdf[pos] <= '9' {
		pos++
	}
	if pos == generation {
		return 0, false
	}
	pos, ok = skipPDFWhitespace(pdf, pos)
	if !ok {
		return 0, false
	}
	if !bytes.HasPrefix(pdf[pos:], objKeyword) {
		return 0, false
	}
	pos += len(objKeyword)
	if pos < len(pdf) && !isPDFWhitespace(pdf[pos]) && !isPDFDelimiter(pdf[pos]) {
		return 0, false
	}
	return pos + endOfLineLength(pdf, pos), true
}

// skipPDFWhitespace advances past a run of at least one white-space character.
func skipPDFWhitespace(pdf []byte, pos int) (int, bool) {
	start := pos
	for pos < len(pdf) && isPDFWhitespace(pdf[pos]) {
		pos++
	}
	return pos, pos > start
}

// endOfLineLength returns the length of the end-of-line sequence at pos — CRLF,
// LF or a bare CR — or 0 when there is none.
func endOfLineLength(pdf []byte, pos int) int {
	if pos >= len(pdf) {
		return 0
	}
	if pdf[pos] == '\n' {
		return 1
	}
	if pdf[pos] == '\r' {
		if pos+1 < len(pdf) && pdf[pos+1] == '\n' {
			return 2
		}
		return 1
	}
	return 0
}

// firstObjectHeader returns the FIRST definition of objID — the genesis object
// of an incrementally updated document.
func firstObjectHeader(pdf []byte, objID int) (objectHeader, bool) {
	definitions := objectDefinitions(pdf, objID)
	if len(definitions) == 0 {
		return objectHeader{}, false
	}
	return resolveObject(pdf, definitions[0])
}

// lastObjectHeader returns the definition of objID a reader resolves — the
// last one, since an incremental update supersedes an object by appending a new
// definition.
func lastObjectHeader(pdf []byte, objID int) (objectHeader, bool) {
	definitions := objectDefinitions(pdf, objID)
	if len(definitions) == 0 {
		return objectHeader{}, false
	}
	return resolveObject(pdf, definitions[len(definitions)-1])
}

// objectStreamData returns the half-open range of a definition's raw stream
// data.
func objectStreamData(pdf []byte, header objectHeader) (start, end int, ok bool) {
	if !header.hasStream {
		return 0, 0, false
	}
	return header.streamStart, header.streamEnd, true
}

// lastObjectStreamData returns the raw stream data of the definition of objID a
// reader resolves.
func lastObjectStreamData(pdf []byte, objID int) (start, end int, ok bool) {
	header, found := lastObjectHeader(pdf, objID)
	if !found {
		return 0, 0, false
	}
	return objectStreamData(pdf, header)
}

// firstObjectStreamData returns the raw stream data of objID's genesis
// definition.
func firstObjectStreamData(pdf []byte, objID int) (start, end int, ok bool) {
	header, found := firstObjectHeader(pdf, objID)
	if !found {
		return 0, 0, false
	}
	return objectStreamData(pdf, header)
}

// lastObjectBody returns the range of the object held by the definition of
// objID a reader resolves — its dictionary, without any stream that follows.
func lastObjectBody(pdf []byte, objID int) (start, end int, ok bool) {
	header, found := lastObjectHeader(pdf, objID)
	if !found {
		return 0, 0, false
	}
	return skipPDFSpace(pdf, header.body), header.value, true
}
