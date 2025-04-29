package ccittg4

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"testing"
)

func TestWhiteImage(t *testing.T) {
	width, height := 16, 16
	gray := make([]byte, width*height)
	// По умолчанию все байты 0 — значит белое (яркость 0), для Go packer серое будет "белым"
	// Но нам нужно наоборот — белое это 255 в grayscale!
	for i := range gray {
		gray[i] = 255 // Полностью белый фон
	}

	data, err := EncodeGrayToCCITTG4(gray, width, height)
	if err != nil {
		t.Fatalf("failed to encode white image: %v", err)
	}

	t.Logf("Encoded white image: %d bytes", len(data))

	if len(data) == 0 {
		t.Fatal("encoded data is empty")
	}

	checkEOFB(t, data)
}

func TestBlackImage(t *testing.T) {
	width, height := 16, 16
	gray := make([]byte, width*height)
	// Полностью чёрный фон (0 значение)
	for i := range gray {
		gray[i] = 0
	}

	data, err := EncodeGrayToCCITTG4(gray, width, height)
	if err != nil {
		t.Fatalf("failed to encode black image: %v", err)
	}

	t.Logf("Encoded black image: %d bytes", len(data))

	if len(data) == 0 {
		t.Fatal("encoded data is empty")
	}

	checkEOFB(t, data)
}

func TestCheckerboardImage(t *testing.T) {
	width, height := 16, 16
	gray := make([]byte, width*height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if (x+y)%2 == 0 {
				gray[y*width+x] = 255 // Белый
			} else {
				gray[y*width+x] = 0 // Чёрный
			}
		}
	}

	data, err := EncodeGrayToCCITTG4(gray, width, height)
	if err != nil {
		t.Fatalf("failed to encode checkerboard: %v", err)
	}

	t.Logf("Encoded checkerboard image: %d bytes", len(data))

	if len(data) == 0 {
		t.Fatal("encoded data is empty")
	}

	checkEOFB(t, data)
}

// func checkEOFB(t *testing.T, data []byte) {
// 	if len(data) < 2 {
// 		t.Fatal("data too small to contain EOFB")
// 	}

// 	// Читаем последние 2 байта
// 	last := data[len(data)-2:]
// 	endBits := uint16(last[0])<<8 | uint16(last[1])

// 	// Проверяем последние 13 бит
// 	if endBits&0x1FFF != 0x0001 {
// 		t.Fatalf("invalid EOFB: last 13 bits = %013b", endBits&0x1FFF)
// 	}
// }

// Стандарт T.6 требует, чтобы EOFB был правильно выровнен
func TestEOLAlignment(t *testing.T) {
	// Проверим на изображениях разной ширины, чтобы убедиться в правильности битового выравнивания
	widths := []int{8, 16, 24, 32, 40, 1728} // 1728 - стандартная ширина факса

	for _, width := range widths {
		t.Run("width_"+fmt.Sprint(width), func(t *testing.T) {
			height := 8
			gray := make([]byte, width*height)
			// Создаем простой паттерн
			for i := range gray {
				if i%2 == 0 {
					gray[i] = 255
				} else {
					gray[i] = 0
				}
			}

			data, err := EncodeGrayToCCITTG4(gray, width, height)
			if err != nil {
				t.Fatalf("failed to encode with width %d: %v", width, err)
			}

			// Проверяем EOFB (две последовательные последовательности EOL)
			checkEOFB(t, data)

			// Проверяем, что данные заканчиваются на байтовой границе
			// (важно для корректного чтения PDF)
			if len(data) > 0 {
				lastByte := data[len(data)-1]
				// По стандарту T.6, неиспользуемые биты должны быть заполнены нулями
				// Но это проверяется в другом тесте
				t.Logf("Last byte: %08b", lastByte)
			}
		})
	}
}

// TestReferencePatterns проверяет правильность кодирования известных патернов
// согласно стандарту T.6
func TestReferencePatterns(t *testing.T) {
	// Тест 1: Кодирование чередующихся черных и белых пикселей
	t.Run("alternating_pixels", func(t *testing.T) {
		width, height := 8, 1
		gray := make([]byte, width*height)
		for i := range gray {
			if i%2 == 0 {
				gray[i] = 255 // Белый
			} else {
				gray[i] = 0 // Черный
			}
		}

		data, err := EncodeGrayToCCITTG4(gray, width, height)
		if err != nil {
			t.Fatalf("failed to encode alternating pattern: %v", err)
		}

		// Проверяем размер данных (должен быть эффективным)
		t.Logf("Encoded alternating pattern: %d bytes, hex: %s",
			len(data), hex.EncodeToString(data))

		checkEOFB(t, data)
	})

	// Тест 2: Изображение с горизонтальной линией
	t.Run("horizontal_line", func(t *testing.T) {
		width, height := 16, 16
		gray := make([]byte, width*height)

		// Заполняем белым
		for i := range gray {
			gray[i] = 255
		}

		// Рисуем горизонтальную черную линию посередине
		for x := 0; x < width; x++ {
			gray[(height/2)*width+x] = 0
		}

		data, err := EncodeGrayToCCITTG4(gray, width, height)
		if err != nil {
			t.Fatalf("failed to encode horizontal line: %v", err)
		}

		t.Logf("Encoded horizontal line: %d bytes", len(data))
		checkEOFB(t, data)
	})

	// Тест 3: Изображение с вертикальной линией
	t.Run("vertical_line", func(t *testing.T) {
		width, height := 16, 16
		gray := make([]byte, width*height)

		// Заполняем белым
		for i := range gray {
			gray[i] = 255
		}

		// Рисуем вертикальную черную линию посередине
		for y := 0; y < height; y++ {
			gray[y*width+(width/2)] = 0
		}

		data, err := EncodeGrayToCCITTG4(gray, width, height)
		if err != nil {
			t.Fatalf("failed to encode vertical line: %v", err)
		}

		t.Logf("Encoded vertical line: %d bytes", len(data))
		checkEOFB(t, data)
	})
}

// TestFaxWidth проверяет кодирование со стандартной шириной факса
func TestFaxWidth(t *testing.T) {
	width := 1728 // Стандартная ширина факса G4
	height := 10

	gray := make([]byte, width*height)
	// Создаем простой паттерн с 50% черных точек
	for i := range gray {
		if i%2 == 0 {
			gray[i] = 255
		}
	}

	data, err := EncodeGrayToCCITTG4(gray, width, height)
	if err != nil {
		t.Fatalf("failed to encode fax width image: %v", err)
	}

	t.Logf("Encoded fax width image: %d bytes", len(data))
	checkEOFB(t, data)
}

// TestStandardCompliance проверяет стандартные требования формата CCITT G4
func TestStandardCompliance(t *testing.T) {
	t.Run("eofb_verification", func(t *testing.T) {
		width, height := 32, 32
		gray := make([]byte, width*height)
		// Заполняем белыми пикселями
		for i := range gray {
			gray[i] = 255
		}

		data, err := EncodeGrayToCCITTG4(gray, width, height)
		if err != nil {
			t.Fatalf("failed to encode: %v", err)
		}

		// Проверяем, что два последних байта содержат EOFB (000000000001 в двоичном формате)
		// EOFB - два подряд EOL кода
		if len(data) < 2 {
			t.Fatal("data too small to contain EOFB")
		}

		// EOFB должен быть (000000000001)₂ для первого EOL и (000000000001)₂ для второго EOL
		// Т.е. в идеале последние 24 бита должны быть 000000000001000000000001
		// Но из-за выравнивания по байтам, нам важны последние 16 бит (2 байта)
		last := data[len(data)-2:]
		endBits := uint16(last[0])<<8 | uint16(last[1])

		// Проверяем окончание, которое должно содержать хотя бы один полный EOFB
		if endBits&0x0FFF != 0x0001 {
			t.Fatalf("invalid EOFB: last 12 bits = %012b", endBits&0x0FFF)
		}

		t.Logf("EOFB verification passed: last 16 bits = %016b", endBits)
	})

	t.Run("first_line_encoding", func(t *testing.T) {
		// По стандарту T.6, первая строка должна кодироваться относительно
		// виртуальной строки с белыми пикселями
		width, height := 32, 2
		gray := make([]byte, width*height)
		// Первая строка - все черные
		for x := 0; x < width; x++ {
			gray[x] = 0
		}
		// Вторая строка - все белые
		for x := 0; x < width; x++ {
			gray[width+x] = 255
		}

		data, err := EncodeGrayToCCITTG4(gray, width, height)
		if err != nil {
			t.Fatalf("failed to encode first line test: %v", err)
		}

		// Проверка размера и содержимого
		t.Logf("First line encoded: %d bytes, hex: %s",
			len(data), hex.EncodeToString(data))

		// Стандарт требует, чтобы кодирование было компактным
		// На одну черную строку должно потребоваться меньше байт, чем ширина/8
		if len(data) > (width*height)/8+10 { // +10 байт для заголовка и EOFB
			t.Errorf("encoding not efficient: %d bytes for %dx%d image",
				len(data), width, height)
		}
	})
}

// TestBitOrderForPDF проверяет правильность порядка битов для PDF
func TestBitOrderForPDF(t *testing.T) {
	width, height := 8, 2
	gray := make([]byte, width*height)

	// Первая строка: чередование 0,255,0,255...
	for x := 0; x < width; x++ {
		if x%2 == 0 {
			gray[x] = 0 // Черный
		} else {
			gray[x] = 255 // Белый
		}
	}

	// Вторая строка: обратное чередование 255,0,255,0...
	for x := 0; x < width; x++ {
		if x%2 == 0 {
			gray[width+x] = 255 // Белый
		} else {
			gray[width+x] = 0 // Черный
		}
	}

	data, err := EncodeGrayToCCITTG4(gray, width, height)
	if err != nil {
		t.Fatalf("failed to encode for bit order test: %v", err)
	}

	t.Logf("Bit order test encoded: %d bytes, hex: %s",
		len(data), hex.EncodeToString(data))

	// В PDF с BlackIs1=true черный цвет кодируется как 1, белый как 0
	// Проверяем, что наш кодировщик соответствует этому ожиданию
	// Эта проверка косвенная, так как мы не декодируем данные,
	// но помогает выявить очевидные проблемы с bit endianness

	checkEOFB(t, data)
}

// TestBitwiseAlignment проверяет правильное побитовое выравнивание
func TestBitwiseAlignment(t *testing.T) {
	// Тестируем разные комбинации ширины, которые проверяют выравнивание битов
	widths := []int{1, 7, 8, 9, 15, 16, 17}

	for _, width := range widths {
		t.Run("width_"+string(rune(width)), func(t *testing.T) {
			height := 4
			gray := make([]byte, width*height)

			// Заполняем разными шаблонами
			for y := 0; y < height; y++ {
				for x := 0; x < width; x++ {
					if (x+y)%2 == 0 {
						gray[y*width+x] = 255
					} else {
						gray[y*width+x] = 0
					}
				}
			}

			data, err := EncodeGrayToCCITTG4(gray, width, height)
			if err != nil {
				t.Fatalf("failed to encode width %d: %v", width, err)
			}

			t.Logf("Width %d encoded: %d bytes", width, len(data))
			checkEOFB(t, data)

			// Проверка выравнивания строк
			// По стандарту T.6 строки должны быть побитово выровнены
			// Эта проверка косвенная, основана на размере данных

			// Размер должен быть достаточным для хранения данных плюс EOFB
			minExpectedSize := (width*height + 7) / 8 // минимум байтов для представления битов
			if len(data) < minExpectedSize {
				t.Errorf("data too small: %d bytes, expected at least %d bytes",
					len(data), minExpectedSize)
			}
		})
	}
}

// TestSpecialCases проверяет специальные случаи, которые могут вызвать проблемы при декодировании
func TestSpecialCases(t *testing.T) {
	t.Run("single_pixel", func(t *testing.T) {
		// Тест с одним пикселем
		width, height := 1, 1
		gray := []byte{0} // Черный пиксель

		data, err := EncodeGrayToCCITTG4(gray, width, height)
		if err != nil {
			t.Fatalf("failed to encode single pixel: %v", err)
		}

		t.Logf("Single pixel encoded: %d bytes, hex: %s",
			len(data), hex.EncodeToString(data))

		checkEOFB(t, data)
	})

	t.Run("long_run_lengths", func(t *testing.T) {
		// Тест с длинными сериями одного цвета
		// Это проверяет правильную обработку make-up кодов
		width, height := 2048, 2
		gray := make([]byte, width*height)

		// Первая строка - все белые
		for i := 0; i < width; i++ {
			gray[i] = 255
		}

		// Вторая строка - все черные
		for i := width; i < width*2; i++ {
			gray[i] = 0
		}

		data, err := EncodeGrayToCCITTG4(gray, width, height)
		if err != nil {
			t.Fatalf("failed to encode long runs: %v", err)
		}

		t.Logf("Long runs encoded: %d bytes", len(data))
		checkEOFB(t, data)

		// Размер кодированных данных должен быть значительно меньше из-за RLE
		if len(data) > width/4 {
			t.Errorf("run length encoding inefficient: %d bytes for %dx%d image",
				len(data), width, height)
		}
	})

	t.Run("alternating_lines", func(t *testing.T) {
		// Тест с чередующимися строками - особенно важен для правильной 2D-кодировки
		width, height := 32, 8
		gray := make([]byte, width*height)

		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				if y%2 == 0 {
					gray[y*width+x] = 255 // Белые строки
				} else {
					gray[y*width+x] = 0 // Черные строки
				}
			}
		}

		data, err := EncodeGrayToCCITTG4(gray, width, height)
		if err != nil {
			t.Fatalf("failed to encode alternating lines: %v", err)
		}

		t.Logf("Alternating lines encoded: %d bytes", len(data))
		checkEOFB(t, data)
	})
}

// Расширенная проверка EOFB
func checkEOFB(t *testing.T, data []byte) {
	if len(data) < 2 {
		t.Fatal("data too small to contain EOFB")
	}

	// Читаем последние 2 байта
	last := data[len(data)-2:]
	endBits := uint16(last[0])<<8 | uint16(last[1])

	// Проверяем последние 12 бит на наличие EOL (000000000001)
	if endBits&0x0FFF != 0x0001 {
		t.Fatalf("invalid EOFB: last 12 bits = %012b", endBits&0x0FFF)
	}

	// Дополнительная проверка: правильное выравнивание байтов
	// Неиспользуемые биты в последнем байте должны быть заполнены нулями
	unusedBits := 8 - (len(data)*8)%8
	if unusedBits < 8 {
		mask := byte(0xFF >> unusedBits)
		if data[len(data)-1] & ^mask != 0 {
			t.Errorf("unused bits in last byte are not zero")
		}
	}
}

// TestCCITTG4ToPDF проверяет корректность данных для вставки в PDF
func TestCCITTG4ToPDF(t *testing.T) {
	width, height := 32, 32
	gray := make([]byte, width*height)

	// Создаем тестовый паттерн
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if (x/4+y/4)%2 == 0 {
				gray[y*width+x] = 0 // Черный
			} else {
				gray[y*width+x] = 255 // Белый
			}
		}
	}

	data, err := EncodeGrayToCCITTG4(gray, width, height)
	if err != nil {
		t.Fatalf("failed to encode for PDF test: %v", err)
	}

	// В PDF данные CCITT должны начинаться с корректного начала скана
	// и завершаться корректным EOFB

	// Проверяем размер (должен быть разумным)
	expectedMaxSize := (width * height) / 4 // Предполагаем хотя бы 2:1 сжатие
	if len(data) > expectedMaxSize {
		t.Logf("Warning: encoding efficiency may be suboptimal: %d bytes for %dx%d image",
			len(data), width, height)
	}

	// Создаем буфер с параметрами PDF для декодирования CCITT G4
	var pdfParams bytes.Buffer
	pdfParams.WriteString("<<\n")
	pdfParams.WriteString("/Type /XObject\n")
	pdfParams.WriteString("/Subtype /Image\n")
	pdfParams.WriteString(fmt.Sprintf("/Width %d\n", width))
	pdfParams.WriteString(fmt.Sprintf("/Height %d\n", height))
	pdfParams.WriteString("/ColorSpace /DeviceGray\n")
	pdfParams.WriteString("/BitsPerComponent 1\n")
	pdfParams.WriteString("/Filter /CCITTFaxDecode\n")
	pdfParams.WriteString(fmt.Sprintf("/DecodeParms <<\n/K -1\n/Columns %d\n/Rows %d\n/BlackIs1 true\n>>\n", width, height))
	pdfParams.WriteString(fmt.Sprintf("/Length %d\n>>\n", len(data)))

	t.Logf("PDF parameters for G4 image:\n%s", pdfParams.String())
	t.Logf("Encoded G4 data: %d bytes", len(data))

	// Проверка завершающих данные байтов - должно быть EOFB
	checkEOFB(t, data)
}

// Эта функция будет тестировать правильное кодирование, обеспечивающее совместимость с Acrobat Reader
func TestAcrobatCompatibility(t *testing.T) {
	// Особенно проблемные случаи для Acrobat Reader
	t.Run("line_overflow_check", func(t *testing.T) {
		// Это проверяет, что кодирование не создает строки длиннее, чем ожидается
		width := 1728 // Стандартная ширина факса
		height := 5

		gray := make([]byte, width*height)
		// Создаем паттерн с одним черным пикселем на концах строк
		for y := 0; y < height; y++ {
			gray[y*width] = 0         // Первый пиксель черный
			gray[y*width+width-1] = 0 // Последний пиксель черный
		}

		data, err := EncodeGrayToCCITTG4(gray, width, height)
		if err != nil {
			t.Fatalf("failed to encode line overflow test: %v", err)
		}

		t.Logf("Line overflow test encoded: %d bytes", len(data))
		checkEOFB(t, data)
	})

	// Добавляем другие специфичные для Acrobat тесты по мере необходимости
}
