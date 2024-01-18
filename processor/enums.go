package processor

import (
	"strings"
)

// OutputFmt specification of requested output type.
type OutputFmt int

// Supported output formats
const (
	OEpub                OutputFmt = iota // epub
	OKepub                                // kepub
	OAzw3                                 // azw3
	OMobi                                 // mobi
	UnsupportedOutputFmt                  //
)

// ParseFmtString converts string to enum value. Case insensitive.
func ParseFmtString(format string) OutputFmt {

	for i := OEpub; i < UnsupportedOutputFmt; i++ {
		if strings.EqualFold(i.String(), format) {
			return i
		}
	}
	return UnsupportedOutputFmt
}

// NotesFmt specification of requested notes presentation.
type NotesFmt int

// Supported notes formats
const (
	NDefault            NotesFmt = iota // default
	NInline                             // inline
	NBlock                              // block
	NFloat                              // float
	NFloatOld                           // float-old
	NFloatNew                           // float-new
	NFloatNewMore                       // float-new-more
	UnsupportedNotesFmt                 //
)

// ParseNotesString converts string to enum value. Case insensitive.
func ParseNotesString(format string) NotesFmt {

	for i := NDefault; i < UnsupportedNotesFmt; i++ {
		if strings.EqualFold(i.String(), format) {
			return i
		}
	}
	return UnsupportedNotesFmt
}

// TOCPlacement specifies placement of toc page
type TOCPlacement int

// Supported TOC page placements
const (
	TOCNone                 TOCPlacement = iota // none
	TOCBefore                                   // before
	TOCAfter                                    // after
	UnsupportedTOCPlacement                     //
)

// ParseTOCPlacementString converts string to enum value. Case insensitive.
func ParseTOCPlacementString(format string) TOCPlacement {

	for i := TOCNone; i < UnsupportedTOCPlacement; i++ {
		if strings.EqualFold(i.String(), format) {
			return i
		}
	}
	return UnsupportedTOCPlacement
}

// TOCType specifies type of the generated toc
type TOCType int

// Supported TOC types
const (
	TOCTypeNormal      TOCType = iota // normal
	TOCTypeKindle                     // kindle
	TOCTypeFlat                       // flat
	UnsupportedTOCType                //
)

// ParseTOCTypeString converts string to enum value. Case insensitive.
func ParseTOCTypeString(format string) TOCType {

	for i := TOCTypeNormal; i < UnsupportedTOCType; i++ {
		if strings.EqualFold(i.String(), format) {
			return i
		}
	}
	return UnsupportedTOCType
}

// APNXGeneration specifies placement of APNX file - Kindle only
type APNXGeneration int

// Supported TOC page placements
const (
	APNXNone                  APNXGeneration = iota // none
	APNXEInk                                        // eink
	APNXApp                                         // app
	UnsupportedAPNXGeneration                       //
)

// ParseAPNXGenerationSring converts string to enum value. Case insensitive.
func ParseAPNXGenerationSring(format string) APNXGeneration {

	for i := APNXNone; i < UnsupportedAPNXGeneration; i++ {
		if strings.EqualFold(i.String(), format) {
			return i
		}
	}
	return UnsupportedAPNXGeneration
}

// StampPlacement specifies how to stamp cover.
type StampPlacement int

// Supported TOC page placements
const (
	StampNone                 StampPlacement = iota // none
	StampTop                                        // top
	StampMiddle                                     // middle
	StampBottom                                     // bottom
	UnsupportedStampPlacement                       //
)

// ParseStampPlacementString converts string to enum value. Case insensitive.
func ParseStampPlacementString(format string) StampPlacement {

	for i := StampNone; i < UnsupportedStampPlacement; i++ {
		if strings.EqualFold(i.String(), format) {
			return i
		}
	}
	return UnsupportedStampPlacement
}

// CoverProcessing specifies how cover image would be processed (if applicable)
type CoverProcessing int

// Supported processing modes
const (
	CoverNone                  CoverProcessing = iota // none
	CoverKeepAR                                       // keepAR
	CoverStretch                                      // stretch
	UnsupportedCoverProcessing                        //
)

// ParseCoverProcessingString converts string to enum value. Case insensitive.
func ParseCoverProcessingString(format string) CoverProcessing {

	for i := CoverNone; i < UnsupportedCoverProcessing; i++ {
		if strings.EqualFold(i.String(), format) {
			return i
		}
	}
	return UnsupportedCoverProcessing
}
