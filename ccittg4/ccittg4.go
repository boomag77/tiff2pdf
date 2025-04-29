package ccittg4

import (
	"bytes"
	"fmt"
	"sync"
)

type BitWriter struct {
	buf   bytes.Buffer
	bits  uint8
	count int
}

var mu sync.Mutex

func (bw *BitWriter) WriteBit(bit uint8) {
	bw.bits = (bw.bits << 1) | (bit & 1)
	bw.count++
	if bw.count == 8 {
		bw.buf.WriteByte(bw.bits)
		bw.bits = 0
		bw.count = 0
	}
}

func (bw *BitWriter) WriteBits(code uint16, length int) {
	for i := length - 1; i >= 0; i-- {
		bw.WriteBit(uint8((code >> i) & 1))
	}
}

func (bw *BitWriter) Flush() {
	if bw.count > 0 {
		bw.bits <<= (8 - bw.count)
		bw.buf.WriteByte(bw.bits)
		bw.bits = 0
		bw.count = 0
	}
}

// func packGrayTo1Bit(gray []byte, width, height int) []byte {
// 	rowBytes := (width + 7) / 8
// 	out := make([]byte, rowBytes*height)

// 	for y := 0; y < height; y++ {
// 		for x := 0; x < width; x++ {
// 			idx := y*rowBytes + (x / 8)
// 			if gray[y*width+x] < 128 {
// 				out[idx] |= 1 << (7 - uint(x)%8)
// 			}
// 		}
// 	}
// 	return out
// }

func packGrayTo1Bit(gray []byte, width, height int) []byte {
	rowBytes := (width + 7) / 8
	out := make([]byte, rowBytes*height)

	for y := 0; y < height; y++ {
		dstRowStart := y * rowBytes
		srcRowStart := y * width
		var b byte
		bitPos := 7

		for x := 0; x < width; x++ {
			if gray[srcRowStart+x] < 128 {
				b |= 1 << bitPos
			}
			bitPos--
			if bitPos < 0 {
				out[dstRowStart] = b
				dstRowStart++
				b = 0
				bitPos = 7
			}
		}

		// если строка не кратна 8 — записать остаток
		if bitPos != 7 {
			out[dstRowStart] = b
		}
	}
	return out
}

func EncodeGrayToCCITTG4(gray []byte, width, height int) ([]byte, error) {
	// mu.Lock()
	// defer mu.Unlock()
	//fmt.Printf("Encoding started, gray len: %d width: %d, height: %d\n", len(gray), width, height)
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("invalid image size")
	}
	if len(gray) != width*height {
		return nil, fmt.Errorf("invalid gray data size")
	}

	packed := packGrayTo1Bit(gray, width, height)
	rowBytes := (width + 7) / 8
	bw := &BitWriter{}

	refLine := make([]byte, rowBytes)
	curLine := make([]byte, rowBytes)

	copy(refLine, make([]byte, rowBytes)) // стартовая reference строка — белая (0)

	for y := 0; y < height; y++ {
		copy(curLine, packed[y*rowBytes:(y+1)*rowBytes])

		// zeroLine := true
		// for _, b := range curLine {
		// 	if b != 0 {
		// 		zeroLine = false
		// 		break
		// 	}
		// }
		// if zeroLine {
		// 	fmt.Printf("⚠️ Line %d: curLine is completely empty (all white)\n", y)
		// }

		encodeLine(bw, refLine, curLine, width, y)
		copy(refLine, curLine)
	}

	// End Of Facsimile Block
	// for i := 0; i < 12; i++ {
	// 	bw.WriteBit(0)
	// }
	// bw.WriteBit(1)

	// bw.Flush()

	bw.WriteEOFB()

	return bw.buf.Bytes(), nil
}

func encodeLine(bw *BitWriter, refLine, curLine []byte, width int, lineNumber int) {
	a0 := -1   // Start at -1, imaginary white pixel
	color := 0 // Starting color is white
	totalPixels := 0

	isEmpty := true
	for _, b := range curLine {
		if b != 0 {
			isEmpty = false
			break
		}
	}

	if isEmpty {
		// Строка вся белая: кодируем один белый run
		writeRun(bw, width, whiteTerminatingCodes, whiteMakeupCodes)
		totalPixels = width
		//fmt.Printf("⚪ Line %d: Encoded full white run (%d pixels)\n", lineNumber, width)
		return
	}

	for {
		a1 := findNextChangingElement(curLine, width, a0, color)
		b1 := findNextChangingElement(refLine, width, a0, color)

		// If we've reached the end of the line
		if a1 >= width {

			break // Exit the loop when we reach the end of line
		}

		b2 := findNextChangingElement(refLine, width, b1, 1-color)

		// Determine coding mode
		if b2 < a1 {
			// Pass Mode
			if b2 > width {
				b2 = width
			}
			bw.WriteBits(0x1, 4) // Pass mode code
			totalPixels += b2 - (a0 + 1)
			a0 = b2
		} else {
			d := a1 - b1
			if d >= -3 && d <= 3 {
				// Vertical Mode
				if a1 > width {
					a1 = width
				}
				bw.WriteBits(verticalCodes[d+3].code, verticalCodes[d+3].length)
				totalPixels += a1 - (a0 + 1)
				a0 = a1
				color = 1 - color
			} else {
				// Horizontal Mode
				a2 := findNextChangingElement(curLine, width, a1, 1-color)

				bw.WriteBits(0x1, 3) // Horizontal mode code

				if a1 > width {
					a1 = width
				}
				if a2 > width {
					a2 = width
				}

				run1 := a1 - (a0 + 1)
				run2 := a2 - a1

				if color == 0 {
					writeRun(bw, run1, whiteTerminatingCodes, whiteMakeupCodes)
					writeRun(bw, run2, blackTerminatingCodes, blackMakeupCodes)
				} else {
					writeRun(bw, run1, blackTerminatingCodes, blackMakeupCodes)
					writeRun(bw, run2, whiteTerminatingCodes, whiteMakeupCodes)
				}

				totalPixels += run1 + run2

				a0 = a2
				if a2 < width {
					color = 1 - color
				}
			}
		}
	}
	//Handle end of line if we haven't reached it
	if a0 < width {
		run := width - (a0 + 1)

		if color == 0 { // текущий цвет белый
			writeRun(bw, run, whiteTerminatingCodes, whiteMakeupCodes)
		} else { // текущий цвет чёрный
			writeRun(bw, run, blackTerminatingCodes, blackMakeupCodes)
		}

		totalPixels += run
	}
	// if totalPixels != width {
	// 	fmt.Printf("❌ Line %d: totalPixels=%d, expected width=%d\n", lineNumber, totalPixels, width)
	// } else {
	// 	fmt.Printf("✅ Line %d: OK (width %d)\n", lineNumber, width)
	// }
}

// Modified function to find changing elements properly
func findNextChangingElement(line []byte, width, pos int, color int) int {
	// If starting before the line, adjust
	if pos < -1 {
		pos = -1
	}

	// Start search from next position
	start := pos + 1

	// If already at or past end, return width
	if start >= width {
		return width
	}

	// Find first pixel of specified color
	for i := start; i < width; i++ {
		bytePos := i / 8
		bitPos := 7 - (i % 8)
		bit := int((line[bytePos] >> bitPos) & 1)

		if bit != color {
			return i
		}
	}

	// No more changes found
	return width
}

func writeRun(bw *BitWriter, run int, termCodes, makeupCodes map[int]struct {
	code   uint16
	length int
}) {

	if run == 0 {
		return
	}
	for run >= 2624 {
		bw.WriteBits(makeupCodes[2560].code, makeupCodes[2560].length)
		run -= 2560
	}
	for run >= 64 {
		v := (run / 64) * 64
		bw.WriteBits(makeupCodes[v].code, makeupCodes[v].length)
		run -= v
	}
	bw.WriteBits(termCodes[run].code, termCodes[run].length)
}

func (bw *BitWriter) WriteEOFB() {
	// Пишем 12 нулей
	for i := 0; i < 12; i++ {
		bw.WriteBit(0)
	}
	// Пишем 1 единицу
	bw.WriteBit(1)

	// Добиваем нулями до конца байта
	for bw.count != 0 {
		bw.WriteBit(0)
	}
}

// func (bw *BitWriter) FlushNoPad() {
// 	if bw.count > 0 {
// 		bw.buf.WriteByte(bw.bits)
// 		bw.bits = 0
// 		bw.count = 0
// 	}
// }
