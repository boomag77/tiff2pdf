package tests

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"tiff2pdf/converter"
	"tiff2pdf/pdf_writer"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	//"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

func TestCCITTIntegrationWithRawTIFF(t *testing.T) {
	// Путь к тестовому несжатому TIFF файлу
	testTiffPath := filepath.Join("testdata", "raw_test.tif")

	// Пропускаем тест, если тестовый файл не найден
	if _, err := os.Stat(testTiffPath); os.IsNotExist(err) {
		t.Skip("Skipping test: raw_test.tif not found in testdata directory")
	}

	t.Run("convert raw TIFF to CCITT and then to PDF", func(t *testing.T) {
		// Шаг 1: Конвертация несжатого TIFF в CCITT данные
		quality := 85
		dpi := 300
		scale := 1.0

		// Вызываем конвертер
		data, ccitt, gray, width, height, actualDpi, err := converter.ConvertTIFFtoData(testTiffPath, quality, dpi, scale)
		if err != nil {
			t.Fatalf("Failed to convert TIFF to data: %v", err)
		}

		// Проверяем, что конвертер правильно обработал наш несжатый TIFF
		t.Logf("Conversion result - Width: %d, Height: %d, DPI: %d, Data size: %d bytes",
			width, height, actualDpi, len(data))
		t.Logf("CCITT: %v, Gray: %v", ccitt != 0, gray != 0)

		// Шаг 2: Создаем PDF с использованием полученных данных
		var pdfBuf bytes.Buffer
		pw, err := pdf_writer.NewPDFWriter(&pdfBuf)
		if err != nil {
			t.Fatalf("Failed to create PDF writer: %v", err)
		}

		// В зависимости от того, какие данные вернул конвертер, используем соответствующий метод
		if ccitt != 0 {
			// Записываем CCITT данные
			err = pw.WriteCCITTImage(width, height, data)
			if err != nil {
				t.Fatalf("Failed to write CCITT image to PDF: %v", err)
			}
		} else if gray != 0 {
			// Записываем JPEG (оттенки серого)
			err = pw.WriteGrayJPEGImage(width, height, data)
			if err != nil {
				t.Fatalf("Failed to write grayscale JPEG image to PDF: %v", err)
			}
		} else {
			// Записываем JPEG (RGB)
			err = pw.WriteRGBJPEGImage(width, height, data)
			if err != nil {
				t.Fatalf("Failed to write RGB JPEG image to PDF: %v", err)
			}
		}

		// Завершаем создание PDF
		err = pw.Finish()
		if err != nil {
			t.Fatalf("Failed to finish PDF: %v", err)
		}

		// Шаг 3: Проверяем, что PDF создан корректно
		pdfContent := pdfBuf.String()

		// Базовые проверки структуры PDF
		requiredPDFElements := []string{
			"%PDF-1.7",
			"/Type /Catalog",
			"/Type /Pages",
			"/Type /Page",
			"/Type /XObject",
			"/Subtype /Image",
			"xref",
			"trailer",
			"startxref",
			"%%EOF",
		}

		for _, element := range requiredPDFElements {
			if !strings.Contains(pdfContent, element) {
				t.Errorf("PDF missing required element: %s", element)
			}
		}

		// Проверка специфических для CCITT параметров, если данные были в формате CCITT
		if ccitt != 0 {
			ccittParams := []string{
				"/Filter /CCITTFaxDecode",
				"/K -1",
				"/BlackIs1 true",
				"/BitsPerComponent 1",
				"/ColorSpace /DeviceGray",
			}

			for _, param := range ccittParams {
				if !strings.Contains(pdfContent, param) {
					t.Errorf("PDF missing CCITT parameter: %s", param)
				}
			}

			// Проверяем, что размеры изображения указаны правильно
			widthParam := fmt.Sprintf("/Width %d", width)
			heightParam := fmt.Sprintf("/Height %d", height)
			columnsParam := fmt.Sprintf("/Columns %d", width)
			rowsParam := fmt.Sprintf("/Rows %d", height)

			dimensionParams := []string{widthParam, heightParam, columnsParam, rowsParam}
			for _, param := range dimensionParams {
				if !strings.Contains(pdfContent, param) {
					t.Errorf("PDF missing dimension parameter: %s", param)
				}
			}
		}

		// Проверяем наличие данных в потоке
		streamStart := strings.Index(pdfContent, "stream\n")
		streamEnd := strings.Index(pdfContent, "\nendstream")

		if streamStart == -1 || streamEnd == -1 || streamStart >= streamEnd {
			t.Fatal("Could not locate stream data in PDF")
		}

		// Проверяем длину данных
		expectedLengthParam := fmt.Sprintf("/Length %d", len(data))
		if !strings.Contains(pdfContent, expectedLengthParam) {
			t.Errorf("PDF missing or incorrect Length parameter: %s", expectedLengthParam)
		}

		// Сохраняем PDF для визуальной проверки
		pdfOutputPath := filepath.Join("testdata", "raw_test_output.pdf")
		if err := os.WriteFile(pdfOutputPath, pdfBuf.Bytes(), 0644); err != nil {
			t.Logf("Failed to save test PDF: %v", err)
		} else {
			t.Logf("Test PDF saved to %s for visual inspection", pdfOutputPath)
		}

		// Дополнительная проверка самих данных в потоке PDF
		streamStart += 7 // пропускаем "stream\n"
		pdfData := pdfContent[streamStart:streamEnd]

		// Сравниваем длину данных
		if len(pdfData) != len(data) {
			t.Errorf("Data length mismatch in PDF stream: got %d bytes, expected %d bytes",
				len(pdfData), len(data))
		}

		// Проверяем содержимое данных (проверяем первые и последние 100 байт или меньше)
		// для экономии времени теста при больших файлах
		checkLength := 100
		if len(data) < checkLength {
			checkLength = len(data)
		}

		// Проверка начала данных
		for i := 0; i < checkLength; i++ {
			if i < len(data) && i < len(pdfData) && data[i] != pdfData[i] {
				t.Errorf("Data mismatch at byte %d: expected 0x%02X, got 0x%02X",
					i, data[i], pdfData[i])
				break // Останавливаемся после первого несовпадения
			}
		}

		// Проверка конца данных
		if len(data) > checkLength {
			for i := 0; i < checkLength; i++ {
				idx := len(data) - checkLength + i
				pdfIdx := len(pdfData) - checkLength + i
				if idx >= 0 && pdfIdx >= 0 && idx < len(data) && pdfIdx < len(pdfData) {
					if data[idx] != pdfData[pdfIdx] {
						t.Errorf("Data mismatch at end byte %d: expected 0x%02X, got 0x%02X",
							idx, data[idx], pdfData[pdfIdx])
						break
					}
				}
			}
		}

		// Подсчет контрольной суммы для сравнения данных
		dataHash := calculateHash(data)
		pdfDataHash := calculateHash([]byte(pdfData))

		if dataHash != pdfDataHash {
			t.Errorf("PDF stream data hash mismatch: expected %s, got %s",
				dataHash, pdfDataHash)
		}

		// Дополнительно проверяем размер PDF файла
		pdfSize := len(pdfBuf.Bytes())
		t.Logf("Final PDF size: %d bytes", pdfSize)

		// Проверка, что PDF не пустой и имеет разумный размер
		if pdfSize < len(data) {
			t.Errorf("PDF size too small: %d bytes (smaller than image data: %d bytes)",
				pdfSize, len(data))
		}
	})
}

func calculateHash(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// Добавим функцию теста для проверки читаемости PDF
// Добавим функцию теста для проверки читаемости PDF
// Заменим функцию TestPDFReadability полностью
// Заменим функцию TestPDFReadability полностью
func TestPDFReadability(t *testing.T) {
	// Путь к тестовому PDF, созданному предыдущим тестом
	pdfOutputPath := filepath.Join("testdata", "raw_test_output.pdf")

	// Пропускаем тест, если файл не найден
	if _, err := os.Stat(pdfOutputPath); os.IsNotExist(err) {
		t.Skip("Skipping test: raw_test_output.pdf not found in testdata directory")
	}

	t.Run("verify PDF readability", func(t *testing.T) {
		// Настраиваем конфигурацию для pdfcpu
		config := model.NewDefaultConfiguration()
		config.ValidationMode = model.ValidationRelaxed

		// Проверяем базовую валидность PDF
		if err := api.ValidateFile(pdfOutputPath, config); err != nil {
			t.Errorf("PDF validation failed: %v", err)
		} else {
			t.Log("PDF passed validation checks")
		}

		// Пробуем извлечь изображения из PDF
		imageDir := filepath.Join("testdata", "extracted_images")
		if err := os.MkdirAll(imageDir, 0755); err != nil {
			t.Logf("Could not create directory for extracted images: %v", err)
		} else {
			extractPrefix := filepath.Join(imageDir, "image")

			// Используем наиболее стабильный метод API
			err := api.ExtractImagesFile(pdfOutputPath, extractPrefix, nil, config)
			if err != nil {
				t.Errorf("Failed to extract images: %v", err)
			} else {
				t.Log("Images successfully extracted from PDF")

				// Проверяем, что файлы действительно были созданы
				files, err := os.ReadDir(imageDir)
				if err != nil {
					t.Logf("Failed to read extracted images directory: %v", err)
				} else if len(files) == 0 {
					t.Error("No image files were extracted")
				} else {
					t.Logf("Successfully extracted %d image files", len(files))

					// Проверим размер первого файла
					if len(files) > 0 {
						firstImagePath := filepath.Join(imageDir, files[0].Name())
						fileInfo, err := os.Stat(firstImagePath)
						if err != nil {
							t.Logf("Failed to get extracted image file info: %v", err)
						} else {
							t.Logf("Extracted image size: %d bytes", fileInfo.Size())
							if fileInfo.Size() == 0 {
								t.Error("Extracted image file is empty")
							}
						}
					}
				}
			}
		}

		// Удаляем код с использованием неподдерживаемого api.InfoFile
		// и заменяем его на прямое использование командной строки pdfinfo
		// или продолжаем с оставшимися проверками

		// Проверяем дополнительно с помощью pdfinfo, если он доступен
		if _, err := exec.LookPath("pdfinfo"); err == nil {
			cmd := exec.Command("pdfinfo", pdfOutputPath)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Logf("pdfinfo check failed: %v", err)
			} else {
				outputStr := string(output)
				t.Logf("PDF info from pdfinfo:\n%s", outputStr)

				// Проверяем наличие страниц
				if !strings.Contains(outputStr, "Pages:") {
					t.Error("pdfinfo doesn't show page count")
				}
			}
		} else {
			t.Log("pdfinfo not available, skipping additional PDF info check")
		}
	})
}

// Добавим более простую функцию для проверки PDF без зависимости от деталей API pdfcpu
func TestPDFBasicReadability(t *testing.T) {
	pdfOutputPath := filepath.Join("testdata", "raw_test_output.pdf")

	if _, err := os.Stat(pdfOutputPath); os.IsNotExist(err) {
		t.Skip("Skipping test: raw_test_output.pdf not found")
	}

	t.Run("basic PDF validation", func(t *testing.T) {
		// Проверяем простым чтением файла
		pdfData, err := os.ReadFile(pdfOutputPath)
		if err != nil {
			t.Fatalf("Failed to read PDF file: %v", err)
		}

		// Проверяем базовую структуру PDF
		pdfStr := string(pdfData)

		// Проверяем заголовок PDF
		if !strings.HasPrefix(pdfStr, "%PDF-") {
			t.Error("Invalid PDF header")
		}

		// Проверяем EOF маркер
		if !strings.Contains(pdfStr, "%%EOF") {
			t.Error("Missing PDF EOF marker")
		}

		// Проверяем, что файл имеет разумный размер
		fileSizeKB := float64(len(pdfData)) / 1024.0
		t.Logf("PDF file size: %.2f KB", fileSizeKB)

		if fileSizeKB < 1.0 {
			t.Error("PDF file is suspiciously small (< 1KB)")
		}

		// Проверяем наличие ключевых структурных элементов для PDF с изображением
		requiredElements := []string{
			"/Type /Catalog",
			"/Type /Pages",
			"/Type /Page",
			"/Type /XObject",
			"/Subtype /Image",
			"stream",
			"endstream",
		}

		for _, element := range requiredElements {
			if !strings.Contains(pdfStr, element) {
				t.Errorf("PDF missing required element: %s", element)
			}
		}

		// Проверяем наличие CCITT параметров
		if strings.Contains(pdfStr, "/Filter /CCITTFaxDecode") {
			t.Log("PDF contains CCITT compressed image")

			ccittParams := []string{"/K -1", "/BlackIs1", "/Columns ", "/Rows "}
			for _, param := range ccittParams {
				if !strings.Contains(pdfStr, param) {
					t.Errorf("PDF missing CCITT parameter: %s", param)
				}
			}
		}

		t.Log("PDF basic structure verification passed")
	})
}
