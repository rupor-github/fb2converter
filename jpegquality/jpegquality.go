package jpegquality

import (
	"bytes"
	"errors"
	"io"
)

// Errors
var (
	ErrInvalidJPEG  = errors.New("Invalid JPEG header")
	ErrWrongTable   = errors.New("ERROR: Wrong size for quantization table")
	ErrShortSegment = errors.New("short segment length")
	ErrShortDQT     = errors.New("DQT section too short")
)

// Fixed bug base on HuangYeWuDeng [ttys3/jpegquality](https://github.com/ttys3/jpegquality/commit/6176ce2bb32baad02c5b3dcd977dbc2eab406312)

// idct.go
const blockSize = 64 // A DCT block is 8x8.

type block [blockSize]int32

const (
	//from  /usr/lib/go/src/image/jpeg/reader.go
	dhtMarker = 0xc4 // Define Huffman Table.
	dqtMarker = 0xdb // Define Quantization Table.
	maxTq     = 3
)

var quant [maxTq + 1]block // Quantization tables, in zig-zag order.

// for the DQT marker -- start --
// Sample quantization tables from JPEG spec --- only needed for
// guesstimate of quality factor.  Note these are in zigzag order.

var stdLuminanceQuantTbl = [64]int{
	16, 11, 12, 14, 12, 10, 16, 14,
	13, 14, 18, 17, 16, 19, 24, 40,
	26, 24, 22, 22, 24, 49, 35, 37,
	29, 40, 58, 51, 61, 60, 57, 51,
	56, 55, 64, 72, 92, 78, 64, 68,
	87, 69, 55, 56, 80, 109, 81, 87,
	95, 98, 103, 104, 103, 62, 77, 113,
	121, 112, 100, 120, 92, 101, 103, 99,
}

var stdChrominanceQuantTbl = [64]int{
	17, 18, 18, 24, 21, 24, 47, 26,
	26, 47, 99, 66, 56, 66, 99, 99,
	99, 99, 99, 99, 99, 99, 99, 99,
	99, 99, 99, 99, 99, 99, 99, 99,
	99, 99, 99, 99, 99, 99, 99, 99,
	99, 99, 99, 99, 99, 99, 99, 99,
	99, 99, 99, 99, 99, 99, 99, 99,
	99, 99, 99, 99, 99, 99, 99, 99,
}

var deftabs = [2][64]int{
	stdLuminanceQuantTbl, stdChrominanceQuantTbl,
}

// for the DQT marker -- end --

// Qualitier ...
type Qualitier interface {
	Quality() int
}

type jpegReader struct {
	rs io.ReadSeeker
	q  int
}

// NewWithBytes ...
func NewWithBytes(buf []byte) (qr Qualitier, err error) {
	return New(bytes.NewReader(buf))
}

// New ...
func New(rs io.ReadSeeker) (qr Qualitier, err error) {
	jr := &jpegReader{rs: rs}
	_, err = jr.rs.Seek(0, 0)
	if err != nil {
		return
	}

	var (
		sign = make([]byte, 2)
	)
	_, err = jr.rs.Read(sign)
	if err != nil {
		return
	}
	if sign[0] != 0xff && sign[1] != 0xd8 {
		err = ErrInvalidJPEG
		return
	}

	jr.q, err = jr.readQuality()
	if err != nil {
		return
	}
	qr = jr
	return
}

func (jr *jpegReader) readQuality() (q int, err error) {
	for {
		mark := jr.readMarker()
		if mark == 0 {
			err = ErrInvalidJPEG
			return
		}
		var (
			length, tableindex int
			sign               = make([]byte, 2)
			// qualityAvg    = make([]float64, 3)
		)
		_, err = jr.rs.Read(sign)
		if err != nil {
			return
		}

		length = int(sign[0])<<8 + int(sign[1]) - 2
		if length < 0 {
			err = ErrShortSegment
			return
		}

		if (mark & 0xff) != dqtMarker { // not a quantization table
			_, err = jr.rs.Seek(int64(length), 1)
			if err != nil {
				return
			}
			continue
		}

		if length%65 != 0 {
			err = ErrWrongTable
			return
		}

		var tabuf = make([]byte, length)
		var n int
		n, err = jr.rs.Read(tabuf)
		if err != nil {
			return
		}

		allones := 1
		var cumsf, cumsf2 float64
		buf := tabuf[0:n]

		var reftable [64]int

		a := 0
		for a < n {
			tableindex = int(tabuf[a] & 0x0f)
			a++

			//precision: (c>>4) ? 16 : 8
			// precision := 8
			// if int8(buf[0])>>4 != 0 {
			// 	precision = 16
			// }

			if tableindex < 2 {
				reftable = deftabs[tableindex]
			}
			// Read in the table, compute statistics relative to reference table
			if a+64 > n {
				err = ErrShortDQT
				return
			}
			for coefindex := 0; coefindex < 64 && a < n; coefindex++ {
				var val int

				if tableindex>>4 != 0 {
					temp := int(buf[a])
					a++
					temp *= 256
					val = int(buf[a]) + temp
					a++
				} else {
					val = int(buf[a])
					a++
				}

				// scaling factor in percent
				x := 100.0 * float64(val) / float64(reftable[coefindex])
				cumsf += x
				cumsf2 += x * x
				// separate check for all-ones table (Q 100)
				if val != 1 {
					allones = 0
				}
			}

			if 0 != len(reftable) { // terse output includes quality
				var qual float64
				cumsf /= 64.0 // mean scale factor
				cumsf2 /= 64.0
				//var2 = cumsf2 - (cumsf * cumsf); // variance
				if allones == 1 { // special case for all-ones table
					qual = 100.0
				} else if cumsf <= 100.0 {
					qual = (200.0 - cumsf) / 2.0
				} else {
					qual = 5000.0 / cumsf
				}

				if tableindex == 0 {
					q = (int)(qual + 0.5)
					return
				}
			}
		}

	}
}

func getTableName(index int) string {
	if index > 0 {
		return "chrominance"
	}
	return "luminance"
}

func (jr *jpegReader) readMarker() int {
	var (
		mark = make([]byte, 2)
		err  error
	)

ReadAgain:
	_, err = jr.rs.Read(mark)
	if err != nil {
		return 0
	}
	if mark[0] != 0xff || mark[1] == 0xff || mark[1] == 0x00 {
		goto ReadAgain
	}

	return int(mark[0])<<8 + int(mark[1])
}

func (jr *jpegReader) Quality() int {
	return jr.q
}
