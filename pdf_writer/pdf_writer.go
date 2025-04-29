package pdf_writer

import (
	"bufio"
	"fmt"
	"io"
)

//type PDFWriter = contracts.PDFWriter

type PDFWriter struct {
	objects    []int64
	imageInfos []ImageInfo
	bw         *bufio.Writer
	cw         *countingWriter
	objNum     int

	pagesObjID   int64
	pageIDs      []int64
	catalogObjID int64
}

type ImageInfo struct {
	id     int64
	width  float64
	height float64
}

type countingWriter struct {
	w      io.Writer
	offset int64
}

func NewPDFWriter(dst io.Writer) (*PDFWriter, error) {
	cw := &countingWriter{
		w: dst,
	}
	pw := &PDFWriter{
		cw: cw,
		bw: bufio.NewWriterSize(cw, 8*1024*1024), // 8MB buffer
	}

	if _, err := pw.bw.WriteString("%PDF-1.7\n%\xFF\xFF\xFF\xFF\n"); err != nil {
		return nil, fmt.Errorf("error writing PDF header: %v", err)
	}
	return pw, nil
}

func (cw *countingWriter) Write(p []byte) (n int, err error) {
	n, err = cw.w.Write(p)
	if err == nil {
		cw.offset += int64(n)
	}
	return n, err
}

func (pw *PDFWriter) getOffset() int64 {
	return pw.cw.offset + int64(pw.bw.Buffered())
}

func (pw *PDFWriter) newObject() int64 {
	pw.objNum++
	pw.objects = append(pw.objects, pw.getOffset())
	pw.bw.WriteString(fmt.Sprintf("%d 0 obj\n", pw.objNum))
	return int64(pw.objNum)
}

func (pw *PDFWriter) WriteCCITTImage(width int, height int, data []byte) error {
	imgID := pw.newObject()
	pw.imageInfos = append(pw.imageInfos, ImageInfo{
		id:     imgID,
		width:  float64(width),
		height: float64(height),
	})

	pw.bw.WriteString("<<\n") // <-- Открываем основной словарь изображения
	pw.bw.WriteString("/Type /XObject\n")
	pw.bw.WriteString("/Subtype /Image\n")
	pw.bw.WriteString(fmt.Sprintf("/Width %d\n/Height %d\n", width, height))
	pw.bw.WriteString("/ColorSpace /DeviceGray\n")
	pw.bw.WriteString("/BitsPerComponent 1\n")
	pw.bw.WriteString("/Filter /CCITTFaxDecode\n")

	// Открываем словарь DecodeParms
	pw.bw.WriteString("/DecodeParms <<\n")
	pw.bw.WriteString(fmt.Sprintf("/K -1\n/Columns %d\n/Rows %d\n/BlackIs1 false\n", width, height))
	pw.bw.WriteString(">>\n") // <-- Закрываем только словарь DecodeParms

	// Запись /Length должна быть в основном словаре изображения
	pw.bw.WriteString(fmt.Sprintf("/Length %d\n", len(data)))

	pw.bw.WriteString(">>\n")     // <-- Закрываем основной словарь изображения
	pw.bw.WriteString("stream\n") // <-- Ключевое слово stream сразу после закрытия основного словаря с одним переводом строки
	pw.bw.Write(data)
	pw.bw.WriteString("\nendstream\n")
	pw.bw.WriteString("endobj\n")
	return nil
}

func (pw *PDFWriter) WriteRGBJPEGImage(width int, height int, data []byte) error {
	imgID := pw.newObject()
	pw.imageInfos = append(pw.imageInfos, ImageInfo{
		id:     imgID,
		width:  float64(width),
		height: float64(height),
	})
	pw.bw.WriteString("<<\n/Type /XObject\n/Subtype /Image\n")
	pw.bw.WriteString(fmt.Sprintf("/Width %d\n/Height %d\n", width, height))
	pw.bw.WriteString("/ColorSpace /DeviceRGB\n/BitsPerComponent 8\n")
	pw.bw.WriteString("/Filter /DCTDecode\n")
	pw.bw.WriteString(fmt.Sprintf("/Length %d\n", len(data)))
	pw.bw.WriteString(">>\nstream\n")
	pw.bw.Write(data)
	pw.bw.WriteString("\nendstream\nendobj\n")
	return nil
}

func (pw *PDFWriter) WriteGrayJPEGImage(width int, height int, data []byte) error {
	imgID := pw.newObject()
	pw.imageInfos = append(pw.imageInfos, ImageInfo{
		id:     imgID,
		width:  float64(width),
		height: float64(height),
	})
	pw.bw.WriteString("<<\n/Type /XObject\n/Subtype /Image\n")
	pw.bw.WriteString(fmt.Sprintf("/Width %d\n/Height %d\n", width, height))
	pw.bw.WriteString("/ColorSpace /DeviceGray\n/BitsPerComponent 8\n")
	pw.bw.WriteString("/Filter /DCTDecode\n")

	pw.bw.WriteString(fmt.Sprintf("/Length %d\n", len(data)))
	pw.bw.WriteString(">>\nstream\n")
	pw.bw.Write(data)
	pw.bw.WriteString("\nendstream\nendobj\n")
	return nil
}

func (pw *PDFWriter) WriteContent(imgName string, imgObjID int64, width, height float64) int64 {
	content := fmt.Sprintf(
		"q\n%.2f 0 0 %.2f 0 0 cm\n/%s Do\nQ\n",
		width, height, imgName,
	)
	objID := pw.newObject()
	pw.bw.WriteString("<<\n")
	contentBytes := []byte(content)
	pw.bw.WriteString(fmt.Sprintf("/Length %d\n", len(contentBytes)))
	pw.bw.WriteString(">>\n")
	pw.bw.WriteString("stream\n")
	pw.bw.Write(contentBytes)
	pw.bw.WriteString("endstream\nendobj\n")
	return objID
}

func (pw *PDFWriter) WritePage(imgName string,
	imgObjID int64,
	contentID int64,
	width, height float64) int64 {
	objID := pw.newObject()
	pw.bw.WriteString("<<\n")
	pw.bw.WriteString("/Type /Page\n")
	pw.bw.WriteString(fmt.Sprintf("/Parent %d 0 R\n", pw.pagesObjID))
	pw.bw.WriteString(fmt.Sprintf("/MediaBox [0 0 %.2f %.2f]\n", width, height))
	//
	pw.bw.WriteString(fmt.Sprintf("/Resources << /XObject << /%s %d 0 R >> >>\n", imgName, imgObjID))

	pw.bw.WriteString(fmt.Sprintf("/Contents %d 0 R\n", contentID))
	pw.bw.WriteString(">>\nendobj\n")
	return objID
}

func (pw *PDFWriter) WritePages(pageObjIDs []int64) int64 {
	objID := pw.newObject()
	pw.bw.WriteString("<<\n")
	pw.bw.WriteString("/Type /Pages\n")
	pw.bw.WriteString(fmt.Sprintf("/Count %d\n", len(pageObjIDs)))
	pw.bw.WriteString("/Kids [\n")
	for _, id := range pageObjIDs {
		pw.bw.WriteString(fmt.Sprintf("%d 0 R ", id))
	}
	pw.bw.WriteString("]\n>>\nendobj\n")
	return objID
}

func (pw *PDFWriter) WriteCatalog() int64 {
	objID := pw.newObject()
	pw.bw.WriteString("<<\n")
	pw.bw.WriteString(fmt.Sprintf("/Type /Catalog\n/Pages %d 0 R\n", pw.pagesObjID))
	pw.bw.WriteString(">>\nendobj\n")
	return objID
}

func (pw *PDFWriter) CreateDocumentStructure() error {
	if err := pw.bw.Flush(); err != nil {
		return fmt.Errorf("error flushing buffer before creating structure: %v", err)
	}

	// 1. Создаём пустой объект Pages
	pw.pagesObjID = pw.newObject()

	pw.bw.WriteString("<<\n")
	pw.bw.WriteString("/Type /Pages\n")
	pw.bw.WriteString("/Count 0\n") // Пока 0
	pw.bw.WriteString("/Kids []\n") // Пока пусто
	pw.bw.WriteString(">>\nendobj\n")

	// 2. Создаём Content и Page для каждого изображения
	for i, info := range pw.imageInfos {
		imgID := info.id
		imgName := fmt.Sprintf("img_%d", i)

		// Сначала Content (команда показать картинку)
		contentID := pw.WriteContent(imgName, imgID, info.width, info.height)

		// Потом Страницу
		pageID := pw.WritePage(imgName, imgID, contentID, info.width, info.height)
		pw.pageIDs = append(pw.pageIDs, pageID)
	}

	// 🔥 ВАЖНО: Обновляем offset для Pages перед его перезаписью!
	pw.objects[pw.pagesObjID-1] = pw.getOffset()

	// 3. Перезаписываем объект Pages с правильным списком Kids
	pw.bw.WriteString(fmt.Sprintf("%d 0 obj\n", pw.pagesObjID))
	pw.bw.WriteString("<<\n")
	pw.bw.WriteString("/Type /Pages\n")
	pw.bw.WriteString(fmt.Sprintf("/Count %d\n", len(pw.pageIDs)))
	pw.bw.WriteString("/Kids [\n")
	for _, id := range pw.pageIDs {
		pw.bw.WriteString(fmt.Sprintf("%d 0 R ", id))
	}
	pw.bw.WriteString("]\n>>\nendobj\n")

	// 4. Создаём Catalog
	pw.catalogObjID = pw.newObject()
	pw.bw.WriteString("<<\n")
	pw.bw.WriteString(fmt.Sprintf("/Type /Catalog\n/Pages %d 0 R\n", pw.pagesObjID))
	pw.bw.WriteString(">>\nendobj\n")

	// 5. Сброс буфера
	if err := pw.bw.Flush(); err != nil {
		return fmt.Errorf("error flushing buffer after creating structure: %v", err)
	}

	return nil
}

func (pw *PDFWriter) Finish() error {

	if err := pw.CreateDocumentStructure(); err != nil {
		return fmt.Errorf("failed to create document structure before finishing: %v", err)
	}

	startXref := pw.cw.offset
	total := len(pw.objects) + 1

	if _, err := fmt.Fprintf(pw.cw.w, "xref\n0 %d\n", total); err != nil {
		return fmt.Errorf("error writing xref header: %v", err)
	}
	if _, err := fmt.Fprintf(pw.cw.w, "%010d %05d f \n", 0, 65535); err != nil {
		return fmt.Errorf("error writing free object xref entry: %v", err)
	}
	for _, off := range pw.objects {
		if _, err := fmt.Fprintf(pw.cw.w, "%010d %05d n \n", off, 0); err != nil {
			return fmt.Errorf("error writing object xref entry: %v", err)
		}
	}

	if _, err := fmt.Fprintf(pw.cw.w,
		"trailer\n<< /Size %d /Root %d 0 R >>\nstartxref\n%d\n%%%%EOF",
		total, pw.catalogObjID, startXref,
	); err != nil {
		return fmt.Errorf("error writing trailer and startxref: %v", err)
	}

	return nil
}
