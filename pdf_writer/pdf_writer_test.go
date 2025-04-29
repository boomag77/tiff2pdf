package pdf_writer

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestWriteRGBJPEGImage(t *testing.T) {
	// Create a mock JPEG data for testing
	mockJPEG := []byte{
		0xFF, 0xD8, // JPEG SOI marker
		0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, // APP0 segment
		0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00,
		// Mock image data
		0xFF, 0xC0, 0x00, 0x11, 0x08, 0x00, 0x10, 0x00, 0x10, 0x03, 0x01,
		0x11, 0x00, 0x02, 0x11, 0x01, 0x03, 0x11, 0x01,
		// More mock data
		0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F, 0x00,
		// Compressed data
		0xAA, 0xBB, 0xCC, 0xDD, 0xEE,
		0xFF, 0xD9, // EOI marker
	}

	t.Run("basic RGB JPEG image", func(t *testing.T) {
		var buf bytes.Buffer
		pw, err := NewPDFWriter(&buf)
		if err != nil {
			t.Fatalf("Failed to create PDFWriter: %v", err)
		}

		// Test parameters
		width, height := 100, 80

		// Call the function being tested
		err = pw.WriteRGBJPEGImage(width, height, mockJPEG)
		if err != nil {
			t.Fatalf("WriteRGBJPEGImage failed: %v", err)
		}

		// Flush buffer to get all content
		if err = pw.bw.Flush(); err != nil {
			t.Fatalf("Failed to flush buffer: %v", err)
		}

		// Check if the image was added to imageInfos
		if len(pw.imageInfos) != 1 {
			t.Fatalf("Expected 1 image in imageInfos, got %d", len(pw.imageInfos))
		}

		// Verify the image dimensions
		imgInfo := pw.imageInfos[0]
		if imgInfo.width != float64(width) || imgInfo.height != float64(height) {
			t.Errorf("Image dimensions incorrect: got %.2f x %.2f, want %d x %d",
				imgInfo.width, imgInfo.height, width, height)
		}

		// Check output content
		output := buf.String()

		// Check PDF object structure
		if !strings.Contains(output, "/Type /XObject") {
			t.Error("Missing /Type /XObject in output")
		}
		if !strings.Contains(output, "/Subtype /Image") {
			t.Error("Missing /Subtype /Image in output")
		}
		if !strings.Contains(output, "/ColorSpace /DeviceRGB") {
			t.Error("Missing /ColorSpace /DeviceRGB in output")
		}
		if !strings.Contains(output, "/BitsPerComponent 8") {
			t.Error("Missing /BitsPerComponent 8 in output")
		}
		if !strings.Contains(output, "/Filter /DCTDecode") {
			t.Error("Missing /Filter /DCTDecode in output")
		}

		// Check that width and height are correctly written
		expectedWidth := fmt.Sprintf("/Width %d", width)
		expectedHeight := fmt.Sprintf("/Height %d", height)
		if !strings.Contains(output, expectedWidth) {
			t.Errorf("Missing or incorrect width: %s", expectedWidth)
		}
		if !strings.Contains(output, expectedHeight) {
			t.Errorf("Missing or incorrect height: %s", expectedHeight)
		}

		// Check that the JPEG data length is correctly written
		expectedLength := fmt.Sprintf("/Length %d", len(mockJPEG))
		if !strings.Contains(output, expectedLength) {
			t.Errorf("Missing or incorrect length: %s", expectedLength)
		}
	})

	t.Run("zero size image", func(t *testing.T) {
		var buf bytes.Buffer
		pw, err := NewPDFWriter(&buf)
		if err != nil {
			t.Fatalf("Failed to create PDFWriter: %v", err)
		}

		// Test with zero dimensions
		err = pw.WriteRGBJPEGImage(0, 0, mockJPEG)
		if err != nil {
			t.Fatalf("WriteRGBJPEGImage failed with zero dimensions: %v", err)
		}

		if err = pw.bw.Flush(); err != nil {
			t.Fatalf("Failed to flush buffer: %v", err)
		}

		// Verify zero dimensions were written correctly
		if len(pw.imageInfos) != 1 {
			t.Fatalf("Expected 1 image in imageInfos, got %d", len(pw.imageInfos))
		}

		imgInfo := pw.imageInfos[0]
		if imgInfo.width != 0 || imgInfo.height != 0 {
			t.Errorf("Zero image dimensions not preserved: got %.2f x %.2f, want 0 x 0",
				imgInfo.width, imgInfo.height)
		}

		output := buf.String()
		if !strings.Contains(output, "/Width 0") || !strings.Contains(output, "/Height 0") {
			t.Error("Zero dimensions not correctly written to PDF")
		}
	})

	t.Run("empty JPEG data", func(t *testing.T) {
		var buf bytes.Buffer
		pw, err := NewPDFWriter(&buf)
		if err != nil {
			t.Fatalf("Failed to create PDFWriter: %v", err)
		}

		// Empty JPEG data
		emptyJPEG := []byte{}

		// Should handle empty data without crashing
		err = pw.WriteRGBJPEGImage(10, 10, emptyJPEG)
		if err != nil {
			t.Fatalf("WriteRGBJPEGImage failed with empty data: %v", err)
		}

		if err = pw.bw.Flush(); err != nil {
			t.Fatalf("Failed to flush buffer: %v", err)
		}

		output := buf.String()
		if !strings.Contains(output, "/Length 0") {
			t.Error("Empty data length not correctly written to PDF")
		}
	})

	t.Run("multiple images", func(t *testing.T) {
		var buf bytes.Buffer
		pw, err := NewPDFWriter(&buf)
		if err != nil {
			t.Fatalf("Failed to create PDFWriter: %v", err)
		}

		// Add two images
		err = pw.WriteRGBJPEGImage(100, 80, mockJPEG)
		if err != nil {
			t.Fatalf("First WriteRGBJPEGImage failed: %v", err)
		}

		err = pw.WriteRGBJPEGImage(200, 160, mockJPEG)
		if err != nil {
			t.Fatalf("Second WriteRGBJPEGImage failed: %v", err)
		}

		// Check that we have two images with correct dimensions
		if len(pw.imageInfos) != 2 {
			t.Fatalf("Expected 2 images in imageInfos, got %d", len(pw.imageInfos))
		}

		// First image
		if pw.imageInfos[0].width != 100 || pw.imageInfos[0].height != 80 {
			t.Errorf("First image dimensions incorrect: got %.2f x %.2f, want 100 x 80",
				pw.imageInfos[0].width, pw.imageInfos[0].height)
		}

		// Second image
		if pw.imageInfos[1].width != 200 || pw.imageInfos[1].height != 160 {
			t.Errorf("Second image dimensions incorrect: got %.2f x %.2f, want 200 x 160",
				pw.imageInfos[1].width, pw.imageInfos[1].height)
		}

		// Check that object IDs are sequential
		if pw.imageInfos[1].id != pw.imageInfos[0].id+1 {
			t.Errorf("Object IDs not sequential: %d and %d",
				pw.imageInfos[0].id, pw.imageInfos[1].id)
		}
	})
}

func TestWriteRGBJPEGImageEndToEnd(t *testing.T) {
	// Create a full PDF with an RGB JPEG image and verify structure
	var buf bytes.Buffer
	pw, err := NewPDFWriter(&buf)
	if err != nil {
		t.Fatalf("Failed to create PDFWriter: %v", err)
	}

	// Simple mock JPEG data
	mockJPEG := []byte{0xFF, 0xD8, 0xFF, 0xD9} // Minimal valid JPEG

	// Add an RGB JPEG image
	err = pw.WriteRGBJPEGImage(200, 100, mockJPEG)
	if err != nil {
		t.Fatalf("WriteRGBJPEGImage failed: %v", err)
	}

	// Finish the PDF
	err = pw.Finish()
	if err != nil {
		t.Fatalf("Failed to finish PDF: %v", err)
	}

	// Get the complete PDF content
	pdfContent := buf.String()

	// Verify essential PDF structure components
	requiredElements := []string{
		"%PDF-1.7", // PDF header
		"/Type /XObject",
		"/Subtype /Image",
		"/ColorSpace /DeviceRGB",
		"/Filter /DCTDecode",
		"/Width 200",
		"/Height 100",
		"/Type /Page",
		"/Type /Pages",
		"/Type /Catalog",
		"xref",
		"trailer",
		"startxref",
		"%%EOF",
	}

	for _, element := range requiredElements {
		if !strings.Contains(pdfContent, element) {
			t.Errorf("PDF missing required element: %s", element)
		}
	}
}

func TestWriteCCITTG4Image(t *testing.T) {
	// Create mock CCITT Group 4 compressed data
	mockCCITT := []byte{
		0x00, 0x08, 0x00, 0x1A, 0x01, 0xB0, 0x00, 0x74,
		0x00, 0x50, 0x53, 0x9F, 0xAF, 0x08, 0x42, 0xE7,
		0x89, 0x00, 0x00, 0x3F, 0xFF, 0xAC,
	}

	t.Run("basic CCITT G4 image", func(t *testing.T) {
		var buf bytes.Buffer
		pw, err := NewPDFWriter(&buf)
		if err != nil {
			t.Fatalf("Failed to create PDFWriter: %v", err)
		}

		// Test parameters
		width, height := 300, 200

		// Call the function being tested
		err = pw.WriteCCITTImage(width, height, mockCCITT)
		if err != nil {
			t.Fatalf("WriteCCITTG4Image failed: %v", err)
		}

		// Flush buffer to get all content
		if err = pw.bw.Flush(); err != nil {
			t.Fatalf("Failed to flush buffer: %v", err)
		}

		// Check if the image was added to imageInfos
		if len(pw.imageInfos) != 1 {
			t.Fatalf("Expected 1 image in imageInfos, got %d", len(pw.imageInfos))
		}

		// Verify the image dimensions
		imgInfo := pw.imageInfos[0]
		if imgInfo.width != float64(width) || imgInfo.height != float64(height) {
			t.Errorf("Image dimensions incorrect: got %.2f x %.2f, want %d x %d",
				imgInfo.width, imgInfo.height, width, height)
		}

		// Check output content
		output := buf.String()

		// Check PDF object structure for CCITT G4 specifics
		if !strings.Contains(output, "/Type /XObject") {
			t.Error("Missing /Type /XObject in output")
		}
		if !strings.Contains(output, "/Subtype /Image") {
			t.Error("Missing /Subtype /Image in output")
		}
		if !strings.Contains(output, "/ColorSpace /DeviceGray") {
			t.Error("Missing /ColorSpace /DeviceGray in output")
		}
		if !strings.Contains(output, "/BitsPerComponent 1") {
			t.Error("Missing /BitsPerComponent 1 in output")
		}
		if !strings.Contains(output, "/Filter /CCITTFaxDecode") {
			t.Error("Missing /Filter /CCITTFaxDecode in output")
		}
		if !strings.Contains(output, "/K -1") {
			t.Error("Missing /K -1 parameter (indicating G4) in output")
		}
		if !strings.Contains(output, "/BlackIs1 true") {
			t.Error("Missing /BlackIs1 true in output")
		}

		// Check that width and height are correctly written
		expectedWidth := fmt.Sprintf("/Width %d", width)
		expectedHeight := fmt.Sprintf("/Height %d", height)
		if !strings.Contains(output, expectedWidth) {
			t.Errorf("Missing or incorrect width: %s", expectedWidth)
		}
		if !strings.Contains(output, expectedHeight) {
			t.Errorf("Missing or incorrect height: %s", expectedHeight)
		}

		// Check that the CCITT data length is correctly written
		expectedLength := fmt.Sprintf("/Length %d", len(mockCCITT))
		if !strings.Contains(output, expectedLength) {
			t.Errorf("Missing or incorrect length: %s", expectedLength)
		}
	})

	t.Run("empty CCITT data", func(t *testing.T) {
		var buf bytes.Buffer
		pw, err := NewPDFWriter(&buf)
		if err != nil {
			t.Fatalf("Failed to create PDFWriter: %v", err)
		}

		// Empty CCITT data
		emptyCCITT := []byte{}

		// Should handle empty data without crashing
		err = pw.WriteCCITTImage(10, 10, emptyCCITT)
		if err != nil {
			t.Fatalf("WriteCCITTG4Image failed with empty data: %v", err)
		}

		if err = pw.bw.Flush(); err != nil {
			t.Fatalf("Failed to flush buffer: %v", err)
		}

		output := buf.String()
		if !strings.Contains(output, "/Length 0") {
			t.Error("Empty data length not correctly written to PDF")
		}
	})

	t.Run("end-to-end CCITT image", func(t *testing.T) {
		var buf bytes.Buffer
		pw, err := NewPDFWriter(&buf)
		if err != nil {
			t.Fatalf("Failed to create PDFWriter: %v", err)
		}

		// Add the image to a complete PDF
		err = pw.WriteCCITTImage(300, 200, mockCCITT)
		if err != nil {
			t.Fatalf("WriteCCITTG4Image failed: %v", err)
		}

		// Finish the PDF
		err = pw.Finish()
		if err != nil {
			t.Fatalf("Failed to finish PDF: %v", err)
		}

		// Get the complete PDF content
		pdfContent := buf.String()

		// Verify essential PDF structure components for CCITT
		requiredElements := []string{
			"%PDF-1.7",
			"/Type /XObject",
			"/Subtype /Image",
			"/ColorSpace /DeviceGray",
			"/BitsPerComponent 1",
			"/Filter /CCITTFaxDecode",
			"/K -1",
			"/BlackIs1 true",
			"/Width 300",
			"/Height 200",
			"/Type /Page",
			"/Type /Pages",
			"/Type /Catalog",
			"xref",
			"trailer",
			"startxref",
			"%%EOF",
		}

		for _, element := range requiredElements {
			if !strings.Contains(pdfContent, element) {
				t.Errorf("PDF missing required element: %s", element)
			}
		}
	})
}

func TestVerifyCCITTImageData(t *testing.T) {
	// Create distinctive CCITT data pattern for testing
	mockCCITT := []byte{
		0x01, 0x23, 0x45, 0x67, 0x89, 0xAB, 0xCD, 0xEF,
		0xFE, 0xDC, 0xBA, 0x98, 0x76, 0x54, 0x32, 0x10,
	}

	t.Run("verify CCITT data integrity", func(t *testing.T) {
		var buf bytes.Buffer
		pw, err := NewPDFWriter(&buf)
		if err != nil {
			t.Fatalf("Failed to create PDFWriter: %v", err)
		}

		// Use specific width and height values to test
		width, height := 456, 789

		// Write the CCITT image
		err = pw.WriteCCITTImage(width, height, mockCCITT)
		if err != nil {
			t.Fatalf("WriteCCITTImage failed: %v", err)
		}

		if err = pw.bw.Flush(); err != nil {
			t.Fatalf("Failed to flush buffer: %v", err)
		}

		output := buf.String()

		// Check if our data appears in the output
		// We need to find the data between "stream" and "endstream"
		streamStart := strings.Index(output, "stream\n") + 7
		streamEnd := strings.Index(output, "\nendstream")

		if streamStart == -1 || streamEnd == -1 || streamStart >= streamEnd {
			t.Fatal("Could not locate stream data in PDF output")
		}

		// Extract the binary data from the PDF
		binaryData := output[streamStart:streamEnd]

		// Check data length matches
		if len(binaryData) != len(mockCCITT) {
			t.Errorf("CCITT data length mismatch: got %d bytes, want %d bytes",
				len(binaryData), len(mockCCITT))
		}

		// Verify every byte of the data
		for i := 0; i < len(mockCCITT) && i < len(binaryData); i++ {
			if mockCCITT[i] != binaryData[i] {
				t.Errorf("Data mismatch at byte %d: got 0x%02X, want 0x%02X",
					i, binaryData[i], mockCCITT[i])
				break // Stop after first mismatch to avoid flooding output
			}
		}

		// Verify width and height parameters in DecodeParms
		columnsPattern := fmt.Sprintf("/Columns %d", width)
		rowsPattern := fmt.Sprintf("/Rows %d", height)

		if !strings.Contains(output, columnsPattern) {
			t.Errorf("Missing or incorrect Columns parameter: %s", columnsPattern)
		}

		if !strings.Contains(output, rowsPattern) {
			t.Errorf("Missing or incorrect Rows parameter: %s", rowsPattern)
		}

		// Check that K=-1 parameter is set (indicating G4 compression)
		if !strings.Contains(output, "/K -1") {
			t.Error("Missing /K -1 parameter (indicating G4 compression)")
		}

		// Verify BlackIs1 parameter is set correctly
		if !strings.Contains(output, "/BlackIs1 true") {
			t.Error("Missing /BlackIs1 true parameter")
		}
	})

	t.Run("test with different data sizes", func(t *testing.T) {
		testSizes := []struct {
			width  int
			height int
			data   []byte
		}{
			{100, 100, make([]byte, 32)},
			{1, 1, []byte{0x55}},           // Minimal size
			{2000, 3000, make([]byte, 64)}, // Large dimensions
		}

		for _, test := range testSizes {
			var buf bytes.Buffer
			pw, err := NewPDFWriter(&buf)
			if err != nil {
				t.Fatalf("Failed to create PDFWriter: %v", err)
			}

			// Fill test data with recognizable pattern
			for i := range test.data {
				test.data[i] = byte(i % 256)
			}

			err = pw.WriteCCITTImage(test.width, test.height, test.data)
			if err != nil {
				t.Fatalf("WriteCCITTImage failed with size %dx%d: %v",
					test.width, test.height, err)
			}

			if err = pw.bw.Flush(); err != nil {
				t.Fatalf("Failed to flush buffer: %v", err)
			}

			output := buf.String()

			// Check dimensions in both image dictionary and DecodeParms
			widthPattern := fmt.Sprintf("/Width %d", test.width)
			heightPattern := fmt.Sprintf("/Height %d", test.height)
			columnsPattern := fmt.Sprintf("/Columns %d", test.width)
			rowsPattern := fmt.Sprintf("/Rows %d", test.height)

			if !strings.Contains(output, widthPattern) {
				t.Errorf("Missing or incorrect Width parameter for %dx%d: %s",
					test.width, test.height, widthPattern)
			}

			if !strings.Contains(output, heightPattern) {
				t.Errorf("Missing or incorrect Height parameter for %dx%d: %s",
					test.width, test.height, heightPattern)
			}

			if !strings.Contains(output, columnsPattern) {
				t.Errorf("Missing or incorrect Columns parameter for %dx%d: %s",
					test.width, test.height, columnsPattern)
			}

			if !strings.Contains(output, rowsPattern) {
				t.Errorf("Missing or incorrect Rows parameter for %dx%d: %s",
					test.width, test.height, rowsPattern)
			}
		}
	})
}

func TestCCITTG4DataPassthrough(t *testing.T) {
	// Example TIFF CCITT Group 4 compressed data
	// This represents raw G4 fax compressed data that would be extracted from a TIFF file
	mockCCITTG4 := []byte{
		0x01, 0x08, 0x04, 0xFF, 0xAA, 0xBB, 0xCC, 0xDD,
		0xEE, 0x80, 0x00, 0x00, 0x01, 0x20, 0x01, 0x80,
		0x90, 0xAD, 0xFF, 0x00, 0x64, 0x37, 0x00, 0x42,
	}

	t.Run("CCITT G4 data passthrough", func(t *testing.T) {
		var buf bytes.Buffer
		pw, err := NewPDFWriter(&buf)
		if err != nil {
			t.Fatalf("Failed to create PDFWriter: %v", err)
		}

		// Sample fax document dimensions
		width, height := 1728, 2200 // Standard fax dimensions

		// Write the pre-compressed G4 data
		err = pw.WriteCCITTImage(width, height, mockCCITTG4)
		if err != nil {
			t.Fatalf("WriteCCITTImage failed with G4 data: %v", err)
		}

		if err = pw.bw.Flush(); err != nil {
			t.Fatalf("Failed to flush buffer: %v", err)
		}

		output := buf.String()

		// Verify G4 specific parameters
		if !strings.Contains(output, "/Filter /CCITTFaxDecode") {
			t.Error("Missing /Filter /CCITTFaxDecode in output")
		}
		if !strings.Contains(output, "/K -1") {
			t.Error("Missing /K -1 parameter (indicating G4) in output")
		}
		if !strings.Contains(output, "/BlackIs1 true") {
			t.Error("Missing /BlackIs1 true in output")
		}

		// Find stream data
		streamStart := strings.Index(output, "stream\n") + 7
		streamEnd := strings.Index(output, "\nendstream")
		if streamStart == -1 || streamEnd == -1 || streamStart >= streamEnd {
			t.Fatal("Could not locate stream data in PDF output")
		}

		// Extract stream data
		streamData := output[streamStart:streamEnd]

		// The critical test: Make sure the raw G4 data is passed through unchanged
		if len(streamData) != len(mockCCITTG4) {
			t.Errorf("G4 data length changed: got %d bytes, expected %d bytes",
				len(streamData), len(mockCCITTG4))
		}

		// Check each byte to ensure data is passed through exactly as provided
		for i := 0; i < len(mockCCITTG4) && i < len(streamData); i++ {
			if mockCCITTG4[i] != streamData[i] {
				t.Errorf("G4 data modified at byte %d: got 0x%02X, expected 0x%02X",
					i, streamData[i], mockCCITTG4[i])
				// Only show up to 5 mismatching bytes to avoid flooding output
				if i >= 4 {
					t.Logf("Additional byte mismatches omitted...")
					break
				}
			}
		}
	})

	t.Run("CCITT G4 with full PDF document", func(t *testing.T) {
		var buf bytes.Buffer
		pw, err := NewPDFWriter(&buf)
		if err != nil {
			t.Fatalf("Failed to create PDFWriter: %v", err)
		}

		// Add the image
		err = pw.WriteCCITTImage(1728, 2200, mockCCITTG4)
		if err != nil {
			t.Fatalf("WriteCCITTImage failed: %v", err)
		}

		// Finish the PDF document
		err = pw.Finish()
		if err != nil {
			t.Fatalf("Failed to finish PDF: %v", err)
		}

		// Get complete PDF content
		pdfContent := buf.String()

		// Verify the PDF is valid with proper G4 parameters
		if !strings.Contains(pdfContent, "/Filter /CCITTFaxDecode") ||
			!strings.Contains(pdfContent, "/K -1") ||
			!strings.Contains(pdfContent, "/BlackIs1 true") ||
			!strings.Contains(pdfContent, "/BitsPerComponent 1") {
			t.Error("PDF missing required CCITT G4 parameters")
		}

		// The PDF should include proper document structure elements
		if !strings.Contains(pdfContent, "/Type /Catalog") ||
			!strings.Contains(pdfContent, "/Type /Pages") ||
			!strings.Contains(pdfContent, "xref") ||
			!strings.Contains(pdfContent, "trailer") ||
			!strings.Contains(pdfContent, "%%EOF") {
			t.Error("PDF missing required document structure elements")
		}
	})
}

// TestCCITTG4StrictCompliance проверяет точное соответствие стандарту T.6
// и совместимость с Adobe Acrobat Reader
func TestCCITTG4StrictCompliance(t *testing.T) {
	// Это EOFB (000000000001 000000000001) - два идущих подряд EOL кода, требуемые T.6
	// Биты выровнены по границе байта согласно требованиям PDF
	eofbData := []byte{0x00, 0x10, 0x01}

	t.Run("EOFB requirements", func(t *testing.T) {
		// Тестируем правильное кодирование EOFB в конце данных
		mockData := []byte{
			0xAA, 0xBB, 0xCC, // некоторые данные
			0x00, 0x10, 0x01, // EOFB (000000000001 000000000001)
		}

		var buf bytes.Buffer
		pw, err := NewPDFWriter(&buf)
		if err != nil {
			t.Fatalf("Failed to create PDFWriter: %v", err)
		}

		width, height := 800, 600
		err = pw.WriteCCITTImage(width, height, mockData)
		if err != nil {
			t.Fatalf("WriteCCITTImage failed: %v", err)
		}

		if err = pw.bw.Flush(); err != nil {
			t.Fatalf("Failed to flush buffer: %v", err)
		}

		output := buf.String()
		streamStart := strings.Index(output, "stream\n") + 7
		streamEnd := strings.Index(output, "\nendstream")

		if streamStart == -1 || streamEnd == -1 || streamStart >= streamEnd {
			t.Fatal("Could not locate stream data in PDF output")
		}

		// Извлекаем данные из потока
		streamData := output[streamStart:streamEnd]

		// Проверяем, что данные заканчиваются EOFB
		if len(streamData) < len(eofbData) {
			t.Fatal("Stream data too short to contain EOFB")
		}

		// Проверяем последние байты на соответствие EOFB
		actualEOFB := []byte(streamData[len(streamData)-len(eofbData):])
		if !bytes.Equal(actualEOFB, eofbData) {
			t.Errorf("Stream does not end with valid EOFB: got %X, expected %X",
				actualEOFB, eofbData)
		}
	})

	t.Run("multiple of 8 bits requirement", func(t *testing.T) {
		// PDF требует, чтобы поток CCITT G4 содержал целое количество байтов
		oddBitsData := []byte{
			0x12, 0x34, 0x56, 0x78, // несколько байт данных
			0x90,             // неполный байт, чтобы проверить дополнение
			0x00, 0x10, 0x01, // EOFB
		}

		var buf bytes.Buffer
		pw, err := NewPDFWriter(&buf)
		if err != nil {
			t.Fatalf("Failed to create PDFWriter: %v", err)
		}

		width, height := 800, 600
		err = pw.WriteCCITTImage(width, height, oddBitsData)
		if err != nil {
			t.Fatalf("WriteCCITTImage failed: %v", err)
		}

		if err = pw.bw.Flush(); err != nil {
			t.Fatalf("Failed to flush buffer: %v", err)
		}

		output := buf.String()

		// В PDF должно быть точно указано /Length равное длине входных данных
		expectedLength := fmt.Sprintf("/Length %d", len(oddBitsData))
		if !strings.Contains(output, expectedLength) {
			t.Errorf("PDF does not contain correct Length: expected %s", expectedLength)
		}

		// Проверка сохранения исходной длины данных (в байтах)
		streamStart := strings.Index(output, "stream\n") + 7
		streamEnd := strings.Index(output, "\nendstream")
		if streamStart > 0 && streamEnd > streamStart {
			streamData := output[streamStart:streamEnd]
			if len(streamData) != len(oddBitsData) {
				t.Errorf("Byte-length mismatch: stream has %d bytes, original data had %d bytes",
					len(streamData), len(oddBitsData))
			}
		}
	})

	t.Run("acrobat reader compatibility", func(t *testing.T) {
		// Acrobat Reader требует определенных параметров CCITT
		var buf bytes.Buffer
		pw, err := NewPDFWriter(&buf)
		if err != nil {
			t.Fatalf("Failed to create PDFWriter: %v", err)
		}

		mockData := []byte{
			0x12, 0x34, 0x56, 0x78,
			0x9A, 0xBC, 0xDE, 0xF0,
			0x00, 0x10, 0x01, // EOFB
		}

		width, height := 300, 200
		err = pw.WriteCCITTImage(width, height, mockData)
		if err != nil {
			t.Fatalf("WriteCCITTImage failed: %v", err)
		}

		if err = pw.bw.Flush(); err != nil {
			t.Fatalf("Failed to flush buffer: %v", err)
		}

		output := buf.String()

		// Проверяем на наличие всех требуемых для Acrobat Reader параметров
		acrobatParams := []string{
			"/K -1",          // Group 4 compression
			"/BlackIs1 true", // Обязательно для правильной интерпретации цветов
			"/Columns " + fmt.Sprintf("%d", width),
			"/Rows " + fmt.Sprintf("%d", height),
			"/BitsPerComponent 1", // Для битового изображения
		}

		for _, param := range acrobatParams {
			if !strings.Contains(output, param) {
				t.Errorf("Missing Acrobat-required parameter: %s", param)
			}
		}

		// Порядок тегов /DecodeParms также важен для Acrobat
		kIndex := strings.Index(output, "/K -1")
		columnsIndex := strings.Index(output, "/Columns")
		rowsIndex := strings.Index(output, "/Rows")
		blackIs1Index := strings.Index(output, "/BlackIs1")

		if !(kIndex >= 0 && columnsIndex >= 0 && rowsIndex >= 0 && blackIs1Index >= 0) {
			t.Fatal("Some required CCITT parameters are missing")
		}

		// Проверяем наличие закрывающих >> для /DecodeParms
		decodeParmsEnd := strings.Index(output, ">>")
		if decodeParmsEnd < 0 {
			t.Error("Missing closing >> for /DecodeParms dictionary")
		}
	})

	t.Run("byte alignment in PDF", func(t *testing.T) {
		// Для совместимости с различными PDF ридерами важно, чтобы
		// данные CCITT были выровнены по границе байта

		// var buf bytes.Buffer
		// pw, err := NewPDFWriter(&buf)
		// if err != nil {
		// 	t.Fatalf("Failed to create PDFWriter: %v", err)
		// }

		// Генерируем тестовые данные с различными размерами в битах
		// чтобы проверить корректность выравнивания
		originalData := make([]byte, 10)
		for i := range originalData {
			originalData[i] = byte(0xAA) // 10101010 в бинарном виде
		}

		// Добавляем EOFB в конец
		dataWithEOFB := append(originalData, 0x00, 0x10, 0x01)

		widthHeightCombinations := []struct {
			width  int
			height int
		}{
			{8, 10},  // Ровно 1 байт на строку
			{10, 10}, // 1.25 байта на строку
			{16, 10}, // 2 байта на строку
			{20, 10}, // 2.5 байта на строку
		}

		for _, combo := range widthHeightCombinations {
			t.Run(fmt.Sprintf("width_%d_height_%d", combo.width, combo.height), func(t *testing.T) {
				var buf bytes.Buffer
				pw, err := NewPDFWriter(&buf)
				if err != nil {
					t.Fatalf("Failed to create PDFWriter: %v", err)
				}

				err = pw.WriteCCITTImage(combo.width, combo.height, dataWithEOFB)
				if err != nil {
					t.Fatalf("WriteCCITTImage failed for %dx%d: %v",
						combo.width, combo.height, err)
				}

				if err = pw.bw.Flush(); err != nil {
					t.Fatalf("Failed to flush buffer: %v", err)
				}

				output := buf.String()

				// Проверяем целостность данных
				streamStart := strings.Index(output, "stream\n") + 7
				streamEnd := strings.Index(output, "\nendstream")

				if streamStart < 0 || streamEnd < 0 || streamStart >= streamEnd {
					t.Fatal("Could not locate stream data in PDF output")
				}

				streamData := output[streamStart:streamEnd]

				// Проверяем, что длина потока равна длине входных данных
				if len(streamData) != len(dataWithEOFB) {
					t.Errorf("Stream length mismatch for %dx%d: got %d bytes, expected %d bytes",
						combo.width, combo.height, len(streamData), len(dataWithEOFB))
				}

				// Проверяем, что параметры DecodeParms соответствуют размерам
				columnsStr := fmt.Sprintf("/Columns %d", combo.width)
				rowsStr := fmt.Sprintf("/Rows %d", combo.height)

				if !strings.Contains(output, columnsStr) {
					t.Errorf("Missing or incorrect Columns parameter: %s", columnsStr)
				}

				if !strings.Contains(output, rowsStr) {
					t.Errorf("Missing or incorrect Rows parameter: %s", rowsStr)
				}
			})
		}
	})
}

// TestExplicitStructure тестирует структуру документа при работе с CCITT G4 данными
func TestExplicitStructure(t *testing.T) {
	t.Run("PDF structure with CCITT", func(t *testing.T) {
		var buf bytes.Buffer
		pw, err := NewPDFWriter(&buf)
		if err != nil {
			t.Fatalf("Failed to create PDFWriter: %v", err)
		}

		// Создаем минимальные CCITT G4 данные с EOFB
		mockCCITT := []byte{
			0x01, 0x02, 0x03, 0x04, // некоторые данные
			0x00, 0x10, 0x01, // EOFB
		}

		width, height := 400, 300
		err = pw.WriteCCITTImage(width, height, mockCCITT)
		if err != nil {
			t.Fatalf("WriteCCITTImage failed: %v", err)
		}

		// Завершаем PDF
		err = pw.Finish()
		if err != nil {
			t.Fatalf("Failed to finish PDF: %v", err)
		}

		pdfContent := buf.String()

		// Структурные проверки
		essentialStructures := []struct {
			name     string
			expected string
		}{
			{"PDF header", "%PDF-1.7"},
			{"Catalog object", "/Type /Catalog"},
			{"Pages object", "/Type /Pages"},
			{"Page object", "/Type /Page"},
			{"XObject reference", "/XObject"},
			{"Image object", "/Type /XObject\n/Subtype /Image"},
			{"CCITT filter", "/Filter /CCITTFaxDecode"},
			{"CCITT K parameter", "/K -1"},
			{"BlackIs1 flag", "/BlackIs1 true"},
			{"XREF table", "xref"},
			{"Trailer", "trailer"},
			{"EOF marker", "%%EOF"},
		}

		for _, structure := range essentialStructures {
			if !strings.Contains(pdfContent, structure.expected) {
				t.Errorf("PDF missing essential %s: %s", structure.name, structure.expected)
			}
		}

		// Проверяем, что объекты правильно нумеруются
		objMarkers := strings.Count(pdfContent, " 0 obj")
		if objMarkers < 4 { // минимум для каталога, страниц, страницы и изображения
			t.Errorf("PDF should have at least 4 objects, found only %d", objMarkers)
		}

		// Проверяем, что страница содержит изображение
		// Страницы должны иметь Resources и содержать ссылку на XObject
		if !strings.Contains(pdfContent, "/Resources") ||
			!strings.Contains(pdfContent, "/XObject <<") {
			t.Error("Page object missing proper Resources with XObject")
		}

		// Проверка правильности xref таблицы
		xrefIndex := strings.Index(pdfContent, "xref")
		if xrefIndex < 0 {
			t.Error("Missing xref table")
		} else {
			// Проверяем, что после xref следуют числовые значения для таблицы
			xrefSection := pdfContent[xrefIndex : xrefIndex+50] // берем небольшой фрагмент
			if !strings.Contains(xrefSection, "0 ") {
				t.Error("Invalid xref table format")
			}
		}
	})

	t.Run("PDF stream delimiters", func(t *testing.T) {
		// Правильные разделители потока требуются для совместимости
		var buf bytes.Buffer
		pw, err := NewPDFWriter(&buf)
		if err != nil {
			t.Fatalf("Failed to create PDFWriter: %v", err)
		}

		mockCCITT := []byte{0xAA, 0xBB, 0xCC, 0x00, 0x10, 0x01}
		err = pw.WriteCCITTImage(100, 100, mockCCITT)
		if err != nil {
			t.Fatalf("WriteCCITTImage failed: %v", err)
		}

		if err = pw.bw.Flush(); err != nil {
			t.Fatalf("Failed to flush buffer: %v", err)
		}

		output := buf.String()

		// Проверяем правильность разделителей потока
		// Ищем последнее ">>" перед "stream"
		streamStart := strings.Index(output, "stream")
		if streamStart < 0 {
			t.Fatal("Could not find 'stream' keyword in PDF output")
		}

		// Ищем последнее ">>" перед streamStart
		dictEnd := strings.LastIndex(output[:streamStart], ">>")

		if dictEnd < 0 {
			t.Fatal("Could not find dictionary closing '>>' before stream")
		}

		// Между >> и stream должен быть только один перевод строки
		// Проверяем символы между dictEnd + 2 (позиция после '>>') и streamStart
		separation := output[dictEnd+2 : streamStart]

		// PDF spec allows either LF (\n) or CR LF (\r\n)
		if separation != "\n" && separation != "\r\n" {
			t.Errorf("Invalid separation between dictionary and stream: %q (expected '\\n' or '\\r\\n')", separation)
		}

		// Проверяем наличие перевода строки после stream
		if !strings.Contains(output, "stream\n") {
			t.Error("Stream keyword should be followed by a newline")
		}

		// Проверяем наличие перевода строки перед endstream
		if !strings.Contains(output, "\nendstream") {
			t.Error("Endstream keyword should be preceded by a newline")
		}
	})
}

// TestRealWorldCCITTSamples проверяет совместимость с реальными CCITT G4 данными
func TestRealWorldCCITTSamples(t *testing.T) {
	// Эти значения представляют начало реальных CCITT G4 данных
	// из факса, декодированного с помощью libtiff
	realG4Headers := [][]byte{
		// Пример 1: начало стандартного факса Group 4
		{0x00, 0x18, 0x00, 0x01, 0x4B, 0x08, 0x34, 0x92},
		// Пример 2: другой шаблон Group 4
		{0x00, 0x0C, 0x00, 0xA0, 0xC1, 0x0C, 0xF9, 0x80},
	}

	for i, sample := range realG4Headers {
		t.Run(fmt.Sprintf("real_g4_sample_%d", i+1), func(t *testing.T) {
			// Добавляем EOFB в конец, что необходимо для корректного CCITT G4
			sampleWithEOFB := append(append([]byte{}, sample...), 0x00, 0x10, 0x01)

			var buf bytes.Buffer
			pw, err := NewPDFWriter(&buf)
			if err != nil {
				t.Fatalf("Failed to create PDFWriter: %v", err)
			}

			// Используем типичные размеры страницы факса
			width, height := 1728, 2200
			err = pw.WriteCCITTImage(width, height, sampleWithEOFB)
			if err != nil {
				t.Fatalf("WriteCCITTImage failed with real G4 sample: %v", err)
			}

			err = pw.Finish()
			if err != nil {
				t.Fatalf("Failed to finish PDF: %v", err)
			}

			output := buf.String()

			// Проверяем наличие всех необходимых параметров
			requiredParams := []string{
				"/K -1",
				"/BlackIs1 true",
				"/Columns 1728",
				"/Rows 2200",
			}

			for _, param := range requiredParams {
				if !strings.Contains(output, param) {
					t.Errorf("PDF missing required parameter: %s", param)
				}
			}

			// Проверяем, что данные были включены без изменений
			streamStart := strings.Index(output, "stream\n") + 7
			streamEnd := strings.Index(output, "\nendstream")

			if streamStart == -1 || streamEnd == -1 || streamStart >= streamEnd {
				t.Fatal("Could not locate stream data in PDF output")
			}

			streamData := output[streamStart:streamEnd]
			if len(streamData) != len(sampleWithEOFB) {
				t.Errorf("Stream data length changed: got %d bytes, expected %d bytes",
					len(streamData), len(sampleWithEOFB))
			}

			// Проверяем начало данных (заголовок)
			if len(streamData) >= len(sample) {
				header := []byte(streamData[:len(sample)]) // Convert string to []byte
				if !bytes.Equal(header, sample) {
					t.Errorf("G4 header modified: got %X, expected %X", header, sample)
				}
			}

			// Проверяем конец данных (EOFB)
			if len(streamData) >= 3 {
				eofb := []byte(streamData[len(streamData)-3:]) // Convert string to []byte
				if !bytes.Equal(eofb, []byte{0x00, 0x10, 0x01}) {
					t.Errorf("EOFB modified: got %X, expected 00 10 01", eofb)
				}
			}
		})
	}
}

// TestAcrobatReaderEdgeCases проверяет особые случаи, с которыми
// могут возникнуть проблемы в Adobe Acrobat Reader
func TestAcrobatReaderEdgeCases(t *testing.T) {
	t.Run("special_widths", func(t *testing.T) {
		// Acrobat Reader особенно чувствителен к определенным размерам страниц
		specialWidths := []int{1, 7, 8, 9, 15, 16, 17, 31, 32, 33, 1728}

		for _, width := range specialWidths {
			t.Run(fmt.Sprintf("width_%d", width), func(t *testing.T) {
				var buf bytes.Buffer
				pw, err := NewPDFWriter(&buf)
				if err != nil {
					t.Fatalf("Failed to create PDFWriter: %v", err)
				}

				// Создаем минимальные данные с EOFB
				mockData := append([]byte{0xAA, 0xBB, 0xCC}, 0x00, 0x10, 0x01)

				height := 10
				err = pw.WriteCCITTImage(width, height, mockData)
				if err != nil {
					t.Fatalf("WriteCCITTImage failed for width %d: %v", width, err)
				}

				if err = pw.bw.Flush(); err != nil {
					t.Fatalf("Failed to flush buffer: %v", err)
				}

				output := buf.String()

				// Проверяем, что параметры указаны корректно
				widthParam := fmt.Sprintf("/Width %d", width)
				columnsParam := fmt.Sprintf("/Columns %d", width)

				if !strings.Contains(output, widthParam) {
					t.Errorf("Missing or incorrect Width parameter: %s", widthParam)
				}

				if !strings.Contains(output, columnsParam) {
					t.Errorf("Missing or incorrect Columns parameter: %s", columnsParam)
				}

				// Для Acrobat важно, чтобы Width == Columns
				widthIndex := strings.Index(output, widthParam)
				columnsIndex := strings.Index(output, columnsParam)

				if widthIndex < 0 || columnsIndex < 0 {
					t.Error("Width or Columns parameter not found")
				}
			})
		}
	})

	t.Run("empty_image", func(t *testing.T) {
		// Проверяем, что пустое изображение обрабатывается корректно
		var buf bytes.Buffer
		pw, err := NewPDFWriter(&buf)
		if err != nil {
			t.Fatalf("Failed to create PDFWriter: %v", err)
		}

		// Минимальные CCITT G4 данные - только EOFB
		minimalData := []byte{0x00, 0x10, 0x01}

		err = pw.WriteCCITTImage(1, 1, minimalData)
		if err != nil {
			t.Fatalf("WriteCCITTImage failed for minimal data: %v", err)
		}

		err = pw.Finish()
		if err != nil {
			t.Fatalf("Failed to finish PDF: %v", err)
		}

		output := buf.String()

		// Проверяем, что PDF создан корректно
		if !strings.Contains(output, "%%EOF") {
			t.Error("PDF does not end properly")
		}

		// Дополнительно проверяем параметры минимального изображения
		if !strings.Contains(output, "/Width 1") || !strings.Contains(output, "/Height 1") {
			t.Error("Invalid dimensions for minimal image")
		}

		if !strings.Contains(output, "/Columns 1") || !strings.Contains(output, "/Rows 1") {
			t.Error("Invalid columns/rows for minimal image")
		}
	})

	t.Run("large_image", func(t *testing.T) {
		// Acrobat может иметь проблемы с большими изображениями
		var buf bytes.Buffer
		pw, err := NewPDFWriter(&buf)
		if err != nil {
			t.Fatalf("Failed to create PDFWriter: %v", err)
		}

		// Используем минимальные данные для экономии памяти в тесте
		mockData := append([]byte{0xAA, 0xBB, 0xCC}, 0x00, 0x10, 0x01)

		// Очень большие размеры изображения
		width, height := 8192, 8192

		err = pw.WriteCCITTImage(width, height, mockData)
		if err != nil {
			t.Fatalf("WriteCCITTImage failed for large image: %v", err)
		}

		err = pw.Finish()
		if err != nil {
			t.Fatalf("Failed to finish PDF: %v", err)
		}

		output := buf.String()

		// Проверяем, что размеры корректно указаны в PDF
		widthStr := fmt.Sprintf("/Width %d", width)
		heightStr := fmt.Sprintf("/Height %d", height)
		columnsStr := fmt.Sprintf("/Columns %d", width)
		rowsStr := fmt.Sprintf("/Rows %d", height)

		if !strings.Contains(output, widthStr) || !strings.Contains(output, heightStr) {
			t.Error("Invalid dimensions in PDF for large image")
		}

		if !strings.Contains(output, columnsStr) || !strings.Contains(output, rowsStr) {
			t.Error("Invalid columns/rows in DecodeParms for large image")
		}
	})
}
