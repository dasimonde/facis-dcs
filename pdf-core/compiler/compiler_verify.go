package compiler

import "fmt"

// ExtractPageContentByteRanges returns the [start, end) byte ranges of every
// page content stream in pdf, in document order. Pages are reached through the
// page tree and each stream's extent comes from its own /Length, so contract
// text — which reaches the content stream verbatim — can neither pass itself off
// as a page nor truncate one by writing "endstream" into a clause.
func ExtractPageContentByteRanges(pdf []byte) ([][2]int, error) {
	streams, err := pageContentStreamRanges(pdf)
	if err != nil {
		return nil, err
	}
	ranges := make([][2]int, 0, len(streams))
	for _, s := range streams {
		ranges = append(ranges, [2]int{s.start, s.end})
	}
	return ranges, nil
}

// rangesOverlap reports whether the half-open intervals [aStart, aEnd) and
// [bStart, bEnd) overlap.
func rangesOverlap(aStart, aEnd, bStart, bEnd int) bool {
	return aStart < bEnd && bStart < aEnd
}

// checkCoverageWithExclusions is the inner implementation of the coverage
// invariant check, accepting explicit exclusions for testability.
// It returns an error if any page content stream byte range overlaps any
// exclusion window — meaning that human-visible content would be excluded from
// the C2PA hard binding hash and therefore unprovenanced.
func checkCoverageWithExclusions(pdf []byte, exclusions []c2paExclusion) error {
	contentRanges, err := ExtractPageContentByteRanges(pdf)
	if err != nil {
		return fmt.Errorf("extracting page content ranges: %w", err)
	}
	for _, r := range contentRanges {
		for _, ex := range exclusions {
			if ex.Length <= 0 {
				continue
			}
			if rangesOverlap(r[0], r[1], ex.Start, ex.Start+ex.Length) {
				return fmt.Errorf(
					"page content stream [%d, %d) overlaps C2PA exclusion [%d, %d): human-visible content is not provenanced",
					r[0], r[1], ex.Start, ex.Start+ex.Length,
				)
			}
		}
	}
	return nil
}

// CheckPageContentC2PACoverage returns an error if any page content stream byte
// in pdfBytes falls within a C2PA exclusion window. A nil return means all
// human-visible content is covered by the hard binding hash.
//
// The C2PA manifest stream (object 9) is the only permitted exclusion region.
// If the exclusion window were to extend into page content territory — due to a
// compiler bug or a tampered manifest — this function detects it.
func CheckPageContentC2PACoverage(pdf []byte) error {
	const c2paObjectID = 9
	streamStart, streamLen, found := findLastObjectStreamRange(pdf, c2paObjectID)
	if !found {
		return fmt.Errorf("C2PA manifest stream (obj %d) not found in PDF", c2paObjectID)
	}
	return checkCoverageWithExclusions(pdf, []c2paExclusion{{Start: streamStart, Length: streamLen}})
}
