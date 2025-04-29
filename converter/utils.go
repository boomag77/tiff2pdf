package converter

// otsuThreshold считает оптимальный порог по методу Оцу
func otsuThreshold(gray []byte) uint8 {
	var hist [256]int
	for _, v := range gray {
		hist[v]++
	}
	total := len(gray)
	sum := 0
	for i, c := range hist {
		sum += i * c
	}
	sumB, wB := 0, 0
	var maxVar float64
	var threshold uint8
	for i, c := range hist {
		wB += c
		if wB == 0 {
			continue
		}
		wF := total - wB
		if wF == 0 {
			break
		}
		sumB += i * c
		mB := float64(sumB) / float64(wB)
		mF := float64(sum-sumB) / float64(wF)
		varBetween := float64(wB) * float64(wF) * (mB - mF) * (mB - mF)
		if varBetween > maxVar {
			maxVar = varBetween
			threshold = uint8(i)
		}
	}
	return threshold
}

// morphologyClose выполняет 3×3 замыкание: сначала дилатацию, потом эрозию
func morphologyClose(bin []uint8, w, h int) []uint8 {
	size := w * h
	dil := make([]uint8, size)
	// дилатация
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			idx := y*w + x
			if bin[idx] == 1 ||
				bin[idx-1] == 1 || bin[idx+1] == 1 ||
				bin[idx-w] == 1 || bin[idx+w] == 1 {
				dil[idx] = 1
			}
		}
	}
	// эрозия
	ero := make([]uint8, size)
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			idx := y*w + x
			if dil[idx] == 1 &&
				dil[idx-1] == 1 && dil[idx+1] == 1 &&
				dil[idx-w] == 1 && dil[idx+w] == 1 {
				ero[idx] = 1
			}
		}
	}
	return ero
}

// packGrayTo1BitOtsuClose: Otsu → closing → упаковка 1-bit MSB2LSB
func packGrayTo1BitOtsuClose(gray []byte, width, height int) []byte {
	n := width * height
	// 1) выберем порог
	thresh := otsuThreshold(gray)
	// 2) бинаризация
	bin := make([]uint8, n)
	for i := 0; i < n; i++ {
		if gray[i] < thresh {
			bin[i] = 1
		}
	}
	// 3) замыкание, чтобы заполнить мелкие дыры и соединить прогоны
	closed := morphologyClose(bin, width, height)
	// 4) упаковка в 1-битный буфер
	rowBytes := (width + 7) / 8
	out := make([]byte, rowBytes*height)
	for y := 0; y < height; y++ {
		dst := y * rowBytes
		bitPos := 7
		var b byte
		for x := 0; x < width; x++ {
			if closed[y*width+x] == 1 {
				b |= 1 << bitPos
			}
			bitPos--
			if bitPos < 0 {
				out[dst] = b
				dst++
				b = 0
				bitPos = 7
			}
		}
		if bitPos != 7 {
			out[dst] = b
		}
	}
	return out
}
