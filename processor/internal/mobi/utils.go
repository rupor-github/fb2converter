//nolint:errcheck
package mobi

import (
	"bytes"
	"encoding/binary"
	"math/big"
)

//nolint
const (
	// important  pdb header offsets
	uniqueIDSseed      = 68
	numberOfPdbRecords = 76

	bookLength      = 4
	bookRecordCount = 8
	firstPdbRecord  = 78

	// important rec0 offsets
	lengthOfBook      = 4
	cryptoType        = 12
	mobiHeaderBase    = 16
	mobiHeaderLength  = 20
	mobiType          = 24
	mobiVersion       = 36
	firstNonText      = 80
	titleOffset       = 84
	firstRescRecord   = 108
	firstContentIndex = 192
	lastContentIndex  = 194
	kf8FdstIndex      = 192
	fcisIndex         = 200
	flisIndex         = 208
	srcsIndex         = 224
	srcsCount         = 228
	primaryIndex      = 244
	datpIndex         = 256
	huffOffset        = 112
	huffTableOffset   = 120

	// exth records of interest
	exthASIN          = 113
	exthStartReading  = 116
	exthKF8Offset     = 121
	exthCoverOffset   = 201
	exthThumbOffset   = 202
	exthThumbnailURI  = 129
	exthCDEType       = 501
	exthCDEContentKey = 504
)

// NOTE: Since I decided to convert verbatim - this is old to_base() implementation originally
// from calibre.ebooks.mobi.utils. The only change - I am using "proper" 32 base alphabet for encoding.
// However seems that in most case simply creating new ULID here would be more than enough.
func convertToRadix32(id string, min int) []byte {

	// For base 32 encoding
	const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

	zero, base, bi, abi, mod := new(big.Int), new(big.Int), new(big.Int), new(big.Int), new(big.Int)

	base.SetInt64(32)

	bi.SetString(id, 16)

	sign := bi.Sign()
	if sign == 0 {
		return []byte("0000000000")
	}
	abi.Abs(bi)

	res := make([]byte, 0, 32)
	for abi.Cmp(zero) > 0 {
		abi.DivMod(abi, base, mod)
		res = append(res, alphabet[int(mod.Int64())])
	}
	for i := len(res); i < min; i++ {
		res = append(res, '0')
	}
	if sign < 0 {
		res = append(res, '-')
	}

	n := len(res)
	for i := 0; i < n/2; i++ {
		res[i], res[n-1-i] = res[n-1-i], res[i]
	}
	return res
}

func getInt16(data []byte, ofs int) int {
	return int(int16(binary.BigEndian.Uint16(data[ofs:])))
}

func getInt32(data []byte, ofs int) int {
	return int(int32(binary.BigEndian.Uint32(data[ofs:])))
}

//nolint:deadcode,unused
func putInt16(data []byte, ofs, val int) []byte {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, uint16(int16(val)))
	if len(data) == 0 {
		return buf
	}
	for i := 0; i < len(buf); i++ {
		data[ofs+i] = buf[i]
	}
	return buf
}

func putInt32(data []byte, ofs, val int) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(int32(val)))
	if len(data) == 0 {
		return buf
	}
	for i := 0; i < len(buf); i++ {
		data[ofs+i] = buf[i]
	}
	return buf
}

func getSectionAddr(data []byte, secno int) (int, int) {

	nsec := getInt16(data, numberOfPdbRecords)
	if secno < 0 || secno >= nsec {
		panic("secno out of range")
	}

	var start, end int
	start = getInt32(data, firstPdbRecord+secno*8)
	if secno == nsec-1 {
		end = len(data)
	} else {
		end = getInt32(data, firstPdbRecord+(secno+1)*8)
	}
	return start, end
}

func getExthParams(rec0 []byte) (int, int, int) {
	ebase := mobiHeaderBase + getInt32(rec0, mobiHeaderLength)
	return ebase, getInt32(rec0, ebase+4), getInt32(rec0, ebase+8)
}

func readExth(rec0 []byte, recnum int) [][]byte {

	var values [][]byte

	ebase, _, enum := getExthParams(rec0)
	ebase += 12

	for enum > 0 {
		exthID := getInt32(rec0, ebase)
		exthLen := getInt32(rec0, ebase+4)
		if exthID == recnum {
			// We might have multiple exths, so build a list.
			values = append(values, rec0[ebase+8:ebase+exthLen])
		}
		enum--
		ebase += exthLen
	}
	return values
}

func writeExth(rec0 []byte, recnum int, data []byte) []byte {

	var newrec0 bytes.Buffer

	ebase, elen, enum := getExthParams(rec0)
	ebaseIdx, enumIdx := ebase+12, enum

	for enumIdx > 0 {
		exthID := getInt32(rec0, ebaseIdx)
		if exthID == recnum {
			dif := len(data) + 8 - getInt32(rec0, ebaseIdx+4)
			newrec0.Write(rec0)
			buf := newrec0.Bytes()
			newrec0.Reset()
			if dif != 0 {
				putInt32(buf, titleOffset, getInt32(buf, titleOffset)+dif)
			}
			newrec0.Write(buf[:ebase+4])
			binary.Write(&newrec0, binary.BigEndian, uint32(elen+len(data)+8-getInt32(rec0, ebaseIdx+4)))
			binary.Write(&newrec0, binary.BigEndian, uint32(enum))
			newrec0.Write(rec0[ebase+12 : ebaseIdx+4])
			binary.Write(&newrec0, binary.BigEndian, uint32(len(data)+8))
			newrec0.Write(data)
			newrec0.Write(rec0[ebaseIdx+getInt32(rec0, ebaseIdx+4):])
			return newrec0.Bytes()
		}
		enumIdx--
		ebaseIdx += getInt32(rec0, ebaseIdx+4)
	}
	return rec0
}

func addExth(rec0 []byte, num int, data []byte) []byte {

	var newrec0 bytes.Buffer

	ebase, elen, enum := getExthParams(rec0)
	newrecsize := 8 + len(data)

	newrec0.Write(rec0[0 : ebase+4])
	binary.Write(&newrec0, binary.BigEndian, uint32(elen+newrecsize))
	binary.Write(&newrec0, binary.BigEndian, uint32(enum+1))
	binary.Write(&newrec0, binary.BigEndian, uint32(num))
	binary.Write(&newrec0, binary.BigEndian, uint32(newrecsize))
	newrec0.Write(data)
	newrec0.Write(rec0[ebase+12:])

	dataout := newrec0.Bytes()
	putInt32(dataout, titleOffset, getInt32(dataout, titleOffset)+newrecsize)
	return dataout
}

func delExth(rec0 []byte, recnum int) []byte {

	var newrec0 bytes.Buffer

	ebase, elen, enum := getExthParams(rec0)
	for ebaseIdx, enumIdx := ebase+12, 0; enumIdx < enum; {
		exthID := getInt32(rec0, ebaseIdx)
		exthSize := getInt32(rec0, ebaseIdx+4)
		if exthID == recnum {
			newrec0.Write(rec0)
			buf := newrec0.Bytes()
			newrec0.Reset()
			putInt32(buf, titleOffset, getInt32(buf, titleOffset)-exthSize)
			newrec0.Write(buf[:ebaseIdx])
			newrec0.Write(buf[ebaseIdx+exthSize:])
			buf = newrec0.Bytes()
			newrec0.Reset()
			newrec0.Write(buf[0 : ebase+4])
			binary.Write(&newrec0, binary.BigEndian, uint32(elen-exthSize))
			binary.Write(&newrec0, binary.BigEndian, uint32(enum-1))
			newrec0.Write(buf[ebase+12:])
			return newrec0.Bytes()
		}
		enumIdx++
		ebaseIdx += exthSize
	}
	return rec0
}

func readSection(data []byte, secno int) []byte {
	start, end := getSectionAddr(data, secno)
	return data[start:end]
}

// make section zero-length but do not delete it to avoid index modifications.
func nullSection(data []byte, secno int) []byte {

	var datalst bytes.Buffer

	nsec := getInt16(data, numberOfPdbRecords)
	secstart, secend := getSectionAddr(data, secno)
	zerosecstart, _ := getSectionAddr(data, 0)
	dif := secend - secstart

	datalst.Write(data[:firstPdbRecord])
	for i := 0; i < secno+1; i++ {
		ofs, flgval := getInt32(data, firstPdbRecord+i*8), getInt32(data, firstPdbRecord+i*8+4)
		binary.Write(&datalst, binary.BigEndian, uint32(ofs))
		binary.Write(&datalst, binary.BigEndian, uint32(flgval))
	}
	for i := secno + 1; i < nsec; i++ {
		ofs, flgval := getInt32(data, firstPdbRecord+i*8), getInt32(data, firstPdbRecord+i*8+4)
		binary.Write(&datalst, binary.BigEndian, uint32(ofs-dif))
		binary.Write(&datalst, binary.BigEndian, uint32(flgval))
	}
	if lpad := zerosecstart - (firstPdbRecord + 8*nsec); lpad > 0 {
		datalst.Write(bytes.Repeat([]byte{0}, lpad))
	}
	datalst.Write(data[zerosecstart:secstart])
	datalst.Write(data[secend:])
	return datalst.Bytes()
}

// writeSection overwrites requested section accounting for different length.
func writeSection(data []byte, secno int, secdata []byte) []byte {

	var datalst bytes.Buffer

	nsec := getInt16(data, numberOfPdbRecords)
	secstart, secend := getSectionAddr(data, secno)
	zerosecstart, _ := getSectionAddr(data, 0)
	dif := len(secdata) - (secend - secstart)

	datalst.Write(data[:uniqueIDSseed])
	binary.Write(&datalst, binary.BigEndian, uint32(2*nsec+1))
	datalst.Write(data[uniqueIDSseed+4 : numberOfPdbRecords])
	binary.Write(&datalst, binary.BigEndian, uint16(nsec))
	for i := 0; i < secno; i++ {
		ofs, flgval := getInt32(data, firstPdbRecord+i*8), getInt32(data, firstPdbRecord+i*8+4)
		binary.Write(&datalst, binary.BigEndian, uint32(ofs))
		binary.Write(&datalst, binary.BigEndian, uint32(flgval))
	}
	binary.Write(&datalst, binary.BigEndian, uint32(secstart))
	binary.Write(&datalst, binary.BigEndian, uint32(2*secno))
	for i := secno + 1; i < nsec; i++ {
		ofs, flgval := getInt32(data, firstPdbRecord+i*8), getInt32(data, firstPdbRecord+i*8+4)
		binary.Write(&datalst, binary.BigEndian, uint32(ofs+dif))
		binary.Write(&datalst, binary.BigEndian, uint32(flgval))
	}
	if lpad := zerosecstart - (firstPdbRecord + 8*nsec); lpad > 0 {
		datalst.Write(bytes.Repeat([]byte{0}, lpad))
	}
	datalst.Write(data[zerosecstart:secstart])
	datalst.Write(secdata)
	datalst.Write(data[secend:])
	return datalst.Bytes()
}

func deleteSectionRange(data []byte, firstsec, lastsec int) []byte {

	var datalst bytes.Buffer

	firstsecstart, _ := getSectionAddr(data, firstsec)
	_, lastsecend := getSectionAddr(data, lastsec)
	zerosecstart, _ := getSectionAddr(data, 0)
	dif := lastsecend - firstsecstart + 8*(lastsec-firstsec+1)
	nsec := getInt16(data, numberOfPdbRecords)

	datalst.Write(data[:uniqueIDSseed])
	binary.Write(&datalst, binary.BigEndian, uint32(2*(nsec-(lastsec-firstsec+1))+1))
	datalst.Write(data[uniqueIDSseed+4 : numberOfPdbRecords])
	binary.Write(&datalst, binary.BigEndian, uint16(nsec-(lastsec-firstsec+1)))
	newstart := zerosecstart - 8*(lastsec-firstsec+1)

	for i := 0; i < firstsec; i++ {
		ofs, flgval := getInt32(data, firstPdbRecord+i*8), getInt32(data, firstPdbRecord+i*8+4)
		ofs -= 8 * (lastsec - firstsec + 1)
		binary.Write(&datalst, binary.BigEndian, uint32(ofs))
		binary.Write(&datalst, binary.BigEndian, uint32(flgval))
	}
	for i := lastsec + 1; i < nsec; i++ {
		ofs := getInt32(data, firstPdbRecord+i*8)
		ofs -= dif
		binary.Write(&datalst, binary.BigEndian, uint32(ofs))
		binary.Write(&datalst, binary.BigEndian, uint32(2*(i-(lastsec-firstsec+1))))
	}
	if lpad := newstart - (firstPdbRecord + 8*(nsec-(lastsec-firstsec+1))); lpad > 0 {
		datalst.Write(bytes.Repeat([]byte{0}, lpad))
	}
	datalst.Write(data[zerosecstart:firstsecstart])
	datalst.Write(data[lastsecend:])
	return datalst.Bytes()
}

func insertSectionRange(datasrc []byte, firstsec, lastsec int, datadst []byte, targetsec int) []byte {

	var datalst bytes.Buffer

	nsec := getInt16(datadst, numberOfPdbRecords)
	zerosecstart, _ := getSectionAddr(datadst, 0)
	insstart, _ := getSectionAddr(datadst, targetsec)
	nins := lastsec - firstsec + 1
	srcstart, _ := getSectionAddr(datasrc, firstsec)
	_, srcend := getSectionAddr(datasrc, lastsec)
	newstart := zerosecstart + 8*nins

	datalst.Write(datadst[:uniqueIDSseed])
	binary.Write(&datalst, binary.BigEndian, uint32(2*(nsec+nins)+1))
	datalst.Write(datadst[uniqueIDSseed+4 : numberOfPdbRecords])
	binary.Write(&datalst, binary.BigEndian, uint16(nsec+nins))

	for i := 0; i < targetsec; i++ {
		ofs, flgval := getInt32(datadst, firstPdbRecord+i*8), getInt32(datadst, firstPdbRecord+i*8+4)
		binary.Write(&datalst, binary.BigEndian, uint32(ofs+8*nins))
		binary.Write(&datalst, binary.BigEndian, uint32(flgval))
	}
	srcstart0, _ := getSectionAddr(datasrc, firstsec)
	for i := 0; i < nins; i++ {
		isrcstart, _ := getSectionAddr(datasrc, firstsec+i)
		binary.Write(&datalst, binary.BigEndian, uint32(insstart+(isrcstart-srcstart0)+8*nins))
		binary.Write(&datalst, binary.BigEndian, uint32(2*(targetsec+i)))
	}
	dif := srcend - srcstart
	for i := targetsec; i < nsec; i++ {
		ofs, _ := getInt32(datadst, firstPdbRecord+i*8), getInt32(datadst, firstPdbRecord+i*8+4)
		binary.Write(&datalst, binary.BigEndian, uint32(ofs+dif+8*nins))
		binary.Write(&datalst, binary.BigEndian, uint32(2*(i+nins)))
	}
	if lpad := newstart - (firstPdbRecord + 8*(nsec+nins)); lpad > 0 {
		datalst.Write(bytes.Repeat([]byte{0}, lpad))
	}
	datalst.Write(datadst[zerosecstart:insstart])
	datalst.Write(datasrc[srcstart:srcend])
	datalst.Write(datadst[insstart:])
	return datalst.Bytes()
}

// This is specific to go - when encoding jpeg standard encoder does not create JFIF APP0 segment and Kindle does not like it.

// JpegDPIType specifyes type of the DPI units
type JpegDPIType uint8

// DPI units type values
const (
	DpiNoUnits JpegDPIType = iota
	DpiPxPerInch
	DpiPxPerSm
)

var (
	marker = []byte{0xFF, 0xE0}                               // APP0 segment marker
	jfif   = []byte{0x4A, 0x46, 0x49, 0x46, 0x00, 0x01, 0x02} // jfif + version
)

// SetJpegDPI creates JFIF APP0 with provided DPI if segment is missing in image.
func SetJpegDPI(buf *bytes.Buffer, dpit JpegDPIType, xdensity, ydensity int16) (*bytes.Buffer, bool) {

	data := buf.Bytes()

	// If JFIF APP0 segment is there - do not do anything
	if bytes.Equal(data[2:4], marker) {
		return buf, false
	}

	var newbuf = new(bytes.Buffer)

	newbuf.Write(data[:2])
	newbuf.Write(marker)
	binary.Write(newbuf, binary.BigEndian, uint16(0x10)) // length
	newbuf.Write(jfif)
	binary.Write(newbuf, binary.BigEndian, uint8(dpit))
	binary.Write(newbuf, binary.BigEndian, uint16(xdensity))
	binary.Write(newbuf, binary.BigEndian, uint16(ydensity))
	binary.Write(newbuf, binary.BigEndian, uint16(0)) // no thumbnail segment
	newbuf.Write(data[2:])

	return newbuf, true
}
