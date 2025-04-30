package converter

import "sort"

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

func medianFilter(gray []byte, w, h int) []byte {
	out := make([]byte, len(gray))
	copy(out, gray)
	for y := 1; y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			// соберём 3×3 соседей
			s := gray[(y-1)*w+x-1 : (y+2)*w+x+2]
			sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
			out[y*w+x] = s[len(s)/2]
		}
	}
	return out
}

func medianFilterLight(gray []byte, w, h int) []byte {
	out := make([]byte, len(gray))
	copy(out, gray)
	var win [9]byte

	for y := 1; y < h-1; y++ {
		base := y * w
		for x := 1; x < w-1; x++ {
			// Собираем 9 соседей
			idx := 0
			for dy := -1; dy <= 1; dy++ {
				row := (y+dy)*w + x - 1
				win[idx+0] = gray[row+0]
				win[idx+1] = gray[row+1]
				win[idx+2] = gray[row+2]
				idx += 3
			}
			// Сортируем 9 элементов
			sort.Slice(win[:], func(i, j int) bool { return win[i] < win[j] })
			// Средний элемент
			out[base+x] = win[4]
		}
	}
	return out
}

func packGrayTo1BitDither(gray []byte, width, height int) []byte {
	// Плавающий буфер для накопления ошибок
	buf := make([]float64, len(gray))
	for i, v := range gray {
		buf[i] = float64(v)
	}

	rowBytes := (width + 7) / 8
	out := make([]byte, rowBytes*height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			idx := y*width + x
			old := buf[idx]
			var newVal float64
			var bitVal uint8
			// порог 128: темнее → чёрный (1), светлее → белый (0)
			if old < 128 {
				newVal = 0
				bitVal = 1
			} else {
				newVal = 255
				bitVal = 0
			}
			err := old - newVal

			// упаковываем бит
			bytePos := y*rowBytes + x/8
			bitPos := 7 - (x % 8)
			if bitVal == 1 {
				out[bytePos] |= 1 << bitPos
			}

			// распространяем ошибку
			// Право: 7/16
			if x+1 < width {
				buf[idx+1] += err * (7.0 / 16.0)
			}
			// Снизу-лево: 3/16
			if x-1 >= 0 && y+1 < height {
				buf[(y+1)*width+(x-1)] += err * (3.0 / 16.0)
			}
			// Снизу: 5/16
			if y+1 < height {
				buf[(y+1)*width+x] += err * (5.0 / 16.0)
			}
			// Снизу-право: 1/16
			if x+1 < width && y+1 < height {
				buf[(y+1)*width+(x+1)] += err * (1.0 / 16.0)
			}
		}
	}

	return out
}

// packGrayTo1BitClean делает:
// 1) простую пороговую бинаризацию (threshold=128)
// 2) «очистку» изолированных чёрных пикселов (если у пиксела нет чёрных соседей по 4-связности, он становится белым)
// 3) упаковку в 1-битный MSB2LSB буфер
func packGrayTo1BitClean(gray []byte, width, height int) []byte {
	n := width * height
	bin := make([]uint8, n)

	// 1) Threshold
	for i := 0; i < n; i++ {
		if gray[i] < 128 {
			bin[i] = 1
		}
	}

	// 2) Морфологическая очистка: уберём «одинокие» чёрные точки
	// (одноразовый проход; если нужно – повторить дважды)
	for y := 1; y < height-1; y++ {
		base := y * width
		for x := 1; x < width-1; x++ {
			idx := base + x
			if bin[idx] == 1 {
				// сумма 4-соседей: L, R, U, D
				sum := bin[idx-1] + bin[idx+1] + bin[idx-width] + bin[idx+width]
				if sum == 0 {
					bin[idx] = 0
				}
			}
		}
	}

	// 3) Упаковка в 1-битный буфер
	rowBytes := (width + 7) / 8
	out := make([]byte, rowBytes*height)
	for y := 0; y < height; y++ {
		dstRow := y * rowBytes
		srcRow := y * width
		var b byte
		bit := 7
		for x := 0; x < width; x++ {
			if bin[srcRow+x] == 1 {
				b |= 1 << bit
			}
			bit--
			if bit < 0 {
				out[dstRow] = b
				dstRow++
				b = 0
				bit = 7
			}
		}
		if bit != 7 {
			out[dstRow] = b
		}
	}
	return out
}

func downsampleGray(src []byte, w, h, w2, h2 int) []byte {
	dst := make([]byte, w2*h2)
	for y2 := 0; y2 < h2; y2++ {
		y1 := y2 * h / h2
		for x2 := 0; x2 < w2; x2++ {
			x1 := x2 * w / w2
			dst[y2*w2+x2] = src[y1*w+x1]
		}
	}
	return dst
}
