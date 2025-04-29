//go:build cgo
// +build cgo

package converter

/*

#cgo LDFLAGS: -ljpeg -ltiff


#include <stdlib.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>
#include <tiffio.h>
#include <jpeglib.h>
#include <math.h>

#include <emmintrin.h> // SSE2

#ifndef TIFFTAG_JPEGCOLORMODE
#define TIFFTAG_JPEGCOLORMODE 65538
#endif

#ifndef JPEGCOLORMODE_RGB
#define JPEGCOLORMODE_RGB     1
#endif

#define GRAY_THRESHOLD 2

typedef struct {
    unsigned char *data;
    size_t         size;
    size_t         cap;
    toff_t         off;
} MemBuf;

// WriteProc для TIFFClientOpen
static tmsize_t writeCallback(thandle_t h, tdata_t buf, tsize_t sz) {
    MemBuf *m = (MemBuf*)h;
    toff_t newoff = m->off + sz;
    if (newoff > m->cap) {
        size_t newCap = (size_t)(newoff*2 + 1024);
        m->data = realloc(m->data, newCap);
        m->cap = newCap;
    }
    memcpy(m->data + m->off, buf, sz);
    m->off = newoff;
    if ((size_t)newoff > m->size) m->size = newoff;
    return sz;
}
// ReadProc для TIFFClientOpen — возвращает 0 байт, нам это не нужно
static tmsize_t readCallback(thandle_t h, tdata_t buf, tsize_t sz) {
    return 0;
}

// Остальные коллбэки «заглушки»
static toff_t sizeCallback(thandle_t h)   { return ((MemBuf*)h)->size; }
static int     closeCallback(thandle_t h)  { return 0; }
static toff_t seekCallback(thandle_t h, toff_t off, int whence) {
    MemBuf *m = (MemBuf*)h;
    toff_t newpos;
    switch(whence) {
        case SEEK_SET: newpos = off; break;
        case SEEK_CUR: newpos = m->off + off; break;
        case SEEK_END: newpos = m->size + off; break;
        default: return (toff_t)-1;
    }
    if (newpos < 0) return (toff_t)-1;
    m->off = newpos;
    return m->off;
}
static int mapCallback(thandle_t h, tdata_t* p, toff_t* n) {
  *p = NULL; *n = 0;
  return 0;
}
static void    unmapCallback(thandle_t h, tdata_t p, toff_t n) {}


// Генерируем **только** сырые G4-данные (payload), без TIFF-заголовка:
int EncodeRawG4(
    unsigned char *bits1,
    int             width,
    int             height,
    unsigned char **outBuf,
    long           *outSize
) {
    MemBuf mb = { .data = NULL, .cap = 0, .size = 0, .off = 0 };

    // Открываем виртуальный TIFF для записи
    TIFF *tif = TIFFClientOpen(
        "g4", "w",
        (thandle_t)&mb,
        readCallback,
        writeCallback,
        seekCallback,
        closeCallback,
        sizeCallback,
        mapCallback,
        unmapCallback
    );
    if (!tif) {
        free(mb.data);
        return -1;
    }

    // Устанавливаем необходимые теги
    TIFFSetField(tif, TIFFTAG_IMAGEWIDTH,      width);
    TIFFSetField(tif, TIFFTAG_IMAGELENGTH,     height);
    TIFFSetField(tif, TIFFTAG_BITSPERSAMPLE,   1);
    TIFFSetField(tif, TIFFTAG_SAMPLESPERPIXEL, 1);
    TIFFSetField(tif, TIFFTAG_COMPRESSION,     COMPRESSION_CCITTFAX4);
    TIFFSetField(tif, TIFFTAG_PHOTOMETRIC,     PHOTOMETRIC_MINISWHITE);
    TIFFSetField(tif, TIFFTAG_FILLORDER,       FILLORDER_MSB2LSB);
    TIFFSetField(tif, TIFFTAG_ROWSPERSTRIP,    height);

    // Перед кодированием G4 убедимся, что off == size (обычно 0)
    seekCallback((thandle_t)&mb, 0, SEEK_END);
    long payloadStart = mb.size;

    // Записываем один strip — libtiff выполнит G4-кодирование
    tsize_t stripSize = TIFFWriteEncodedStrip(
        tif,
        0,
        bits1,
        ((width + 7) / 8) * height
    );
    TIFFClose(tif);

    if (stripSize < 0) {
        free(mb.data);
        return -2;
    }

    // Выделяем payload: именно тот блок, что libtiff добавил после payloadStart
    long payloadLen = mb.size - payloadStart;
    *outBuf  = (unsigned char*)malloc(payloadLen);
    memcpy(*outBuf, mb.data + payloadStart, payloadLen);
    *outSize = payloadLen;

    free(mb.data);
    return 0;
}


void rgb_to_gray_sse2(const uint8_t* rgb, uint8_t* gray, int npixels, int* ccitt_ready) {
    const __m128i coeff_r = _mm_set1_epi16(30);
    const __m128i coeff_g = _mm_set1_epi16(59);
    const __m128i coeff_b = _mm_set1_epi16(11);
    const __m128i zero = _mm_setzero_si128();

    int i = 0;
    for (; i <= npixels - 8; i += 8) {
        uint8_t r0[8], g0[8], b0[8];

        for (int j = 0; j < 8; j++) {
            r0[j] = rgb[(i + j) * 3 + 0];
            g0[j] = rgb[(i + j) * 3 + 1];
            b0[j] = rgb[(i + j) * 3 + 2];
        }

        __m128i r = _mm_unpacklo_epi8(_mm_loadl_epi64((__m128i*)r0), zero);
        __m128i g = _mm_unpacklo_epi8(_mm_loadl_epi64((__m128i*)g0), zero);
        __m128i b = _mm_unpacklo_epi8(_mm_loadl_epi64((__m128i*)b0), zero);

        r = _mm_mullo_epi16(r, coeff_r);
        g = _mm_mullo_epi16(g, coeff_g);
        b = _mm_mullo_epi16(b, coeff_b);

        __m128i sum = _mm_add_epi16(r, g);
        sum = _mm_add_epi16(sum, b);

        sum = _mm_srli_epi16(sum, 6);

        __m128i res8 = _mm_packus_epi16(sum, zero);
        _mm_storel_epi64((__m128i*)(gray + i), res8);
    }

    int bad_count = 0;

    for (; i < npixels; i++) {
        uint8_t r = rgb[i * 3 + 0];
        uint8_t g = rgb[i * 3 + 1];
        uint8_t b = rgb[i * 3 + 2];
        gray[i] = (r * 30 + g * 59 + b * 11) / 100;
        if (gray[i] > 50 && gray[i] < 200) {
            bad_count++;
        }
    }
    double bad_ratio = (double)bad_count / (double)(npixels);
    if (bad_ratio < 0.05) {
        *ccitt_ready = 1;
    } else {
        *ccitt_ready = 0;
    }

}

// JPEG encoder from RGBA → JPEG memory
int write_jpeg_to_mem(uint32_t width, uint32_t height, uint8_t* buffer,
                      int quality, int dpi, int gray,
                      unsigned char** out, unsigned long* outSize) {
    struct jpeg_compress_struct cinfo;
    struct jpeg_error_mgr jerr;

    JSAMPROW row_pointer[1];
    int row_stride = width * (gray ? 1 : 3);

    cinfo.err = jpeg_std_error(&jerr);
    jpeg_create_compress(&cinfo);
    jpeg_mem_dest(&cinfo, out, outSize);

    cinfo.image_width = width;
    cinfo.image_height = height;
    cinfo.input_components = gray ? 1 : 3;
    cinfo.in_color_space = gray ? JCS_GRAYSCALE : JCS_RGB;

    jpeg_set_defaults(&cinfo);
    jpeg_set_quality(&cinfo, quality, TRUE);
    jpeg_simple_progression(&cinfo);
    cinfo.density_unit = 1;
    cinfo.X_density = dpi;
    cinfo.Y_density = dpi;

    if (!gray) {
        cinfo.comp_info[0].h_samp_factor = 2;
        cinfo.comp_info[0].v_samp_factor = 2;
        cinfo.comp_info[1].h_samp_factor = 1;
        cinfo.comp_info[1].v_samp_factor = 1;
        cinfo.comp_info[2].h_samp_factor = 1;
        cinfo.comp_info[2].v_samp_factor = 1;
    }

    jpeg_start_compress(&cinfo, TRUE);
    while (cinfo.next_scanline < cinfo.image_height) {
        row_pointer[0] = &buffer[cinfo.next_scanline * row_stride];
        jpeg_write_scanlines(&cinfo, row_pointer, 1);
    }
    jpeg_finish_compress(&cinfo);
    jpeg_destroy_compress(&cinfo);

    return 0;
}

// TIFF → JPEG via RGBA
int convert_tiff_to_data(const char* path,
                        int quality, int dpi, double scale,
                         unsigned char** outBuf, unsigned long* outSize,
                         int* ccitt_filter, int* gray_filter,
                         int* outWidth, int* outHeight, int* outDpi)
{

    TIFF* tif = TIFFOpen(path, "r");
    if (!tif) return -1;

	TIFFSetWarningHandler(NULL);

    uint32_t orig_width = 0, orig_height = 0;
    if (!TIFFGetField(tif, TIFFTAG_IMAGEWIDTH, &orig_width) ||
        !TIFFGetField(tif, TIFFTAG_IMAGELENGTH, &orig_height)) {
        TIFFClose(tif);
        return -2;
    }


    uint32_t* raster = _TIFFmalloc((size_t)orig_width * (size_t)orig_height * sizeof(uint32_t));
    if (!raster) {
        TIFFClose(tif);
        return -3;
    }

    if (!TIFFReadRGBAImageOriented(tif, orig_width, orig_height, raster, ORIENTATION_TOPLEFT, 0)) {
        _TIFFfree(raster);
        TIFFClose(tif);
        return -4;
    }

    int width = orig_width;
    int height = orig_height;

    if (scale != 1.0) {
        width = (uint32_t)(width * scale);
        height = (uint32_t)(height * scale);
    }

    int npixels = width * height;

    if (width == 0 || height == 0) {
        _TIFFfree(raster);
        TIFFClose(tif);
        return -5;
    }

    uint8_t* rgb = malloc(npixels * 3);
    if (!rgb) {
        _TIFFfree(raster);
        TIFFClose(tif);
        return -6;
    }
    uint8_t* gray = malloc(npixels);
    if (!gray) {
        fprintf(stderr, "malloc(gray) failed: %d bytes\n", npixels);
        free(rgb);
        _TIFFfree(raster);
        TIFFClose(tif);
        return -7;
    }

    int dst, dst_x, dst_y;
    uint32_t px;
    uint8_t r, g, b;

    int graycount = 0;
    int px_index;

    for (int y = 0; y < height; y++) {
        for (int x = 0; x < width; x++) {

            dst_x = (int)((x + 0.5) / scale);
            dst_y = (int)((y + 0.5) / scale);

            if (dst_x >= orig_width) {
                dst_x = orig_width - 1;
            }
            if (dst_y >= orig_height) {
                dst_y = orig_height - 1;
            }
            px = raster[dst_y * orig_width + dst_x];
            px_index = y * width + x;

            dst = px_index * 3;

            r = TIFFGetR(px);
            g = TIFFGetG(px);
            b = TIFFGetB(px);
            rgb[dst + 0] = r;
            rgb[dst + 1] = g;
            rgb[dst + 2] = b;
            if (abs(r-g) < GRAY_THRESHOLD && abs(r-b) < GRAY_THRESHOLD && abs(g-b) < GRAY_THRESHOLD) {
                graycount++;
            }
            //gray[px_index] = (uint8_t)((r * 30 + g * 59 + b * 11) / 100);

        }
    }

    double gray_ratio = (double)graycount / (double)(npixels);
    int use_gray = (gray_ratio > 0.9);

    int ccitt_ready = 0;
    if (use_gray) {
        rgb_to_gray_sse2(rgb, gray, npixels, &ccitt_ready);
    }

    int rc = -1;
    //ccitt_ready = 0; // exclude CCITT for now

    if (ccitt_ready) {
        *ccitt_filter = 1;
        *gray_filter = 0;
        *outBuf = gray;
        *outSize = (unsigned long)(width * height);
        *outWidth = width;
        *outHeight = height;
        *outDpi = dpi;
        free(rgb);
        _TIFFfree(raster);
        TIFFClose(tif);
        return 0;
    }

    *ccitt_filter = 0;
    *gray_filter = use_gray;
    rc = write_jpeg_to_mem(width, height, use_gray ? gray : rgb, quality, dpi, use_gray, outBuf, outSize);

    *outWidth = width;
    *outHeight = height;
    *outDpi = dpi;

    free(rgb);
    free(gray);
    _TIFFfree(raster);
    TIFFClose(tif);

    return rc == 0 ? 0 : -6;
}


*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"
)

// ConvertTIFFtoData reads TIFF from file path and returns JPEG-encoded []byte + width, height, dpi
func ConvertTIFFtoData(path string, quality int, dpi int, scale float64) (data []byte, ccitt int, gray int, width, height, actualDpi int, err error) {
	cPath := C.CString(path) // Converts Go string to C string
	defer C.free(unsafe.Pointer(cPath))

	var outBuf *C.uchar
	var outSize C.ulong
	var w, h, d C.int
	var use_ccitt C.int
	var use_gray C.int

	rc := C.convert_tiff_to_data(
		cPath,
		C.int(quality),
		C.int(dpi),
		C.double(scale),
		&outBuf, &outSize,
		&use_ccitt,
		&use_gray,
		&w, &h, &d,
	)

	if rc != 0 {
		C.free(unsafe.Pointer(outBuf))
		return nil, 0, 0, 0, 0, 0, fmt.Errorf("convert_tiff_to_data failed with code %d", int(rc))
	}

	if use_ccitt == 1 {
		goGray := C.GoBytes(unsafe.Pointer(outBuf), C.int(outSize))
		packed := packGrayTo1Bit(goGray, int(w), int(h))
		C.free(unsafe.Pointer(outBuf)) // Освобождаем оригинальный буфер

		ccittData, encodeErr := EncodeRawCCITTG4(packed, int(w), int(h))
		if encodeErr != nil {
			return nil, 0, 0, 0, 0, 0, fmt.Errorf("ccittg4 encode failed: %v", encodeErr)
		}

		return ccittData, int(use_ccitt), int(use_gray), int(w), int(h), int(d), nil
	}
	data = C.GoBytes(unsafe.Pointer(outBuf), C.int(outSize))
	C.free(unsafe.Pointer(outBuf))
	return data, int(use_ccitt), int(use_gray), int(w), int(h), int(d), nil
}

func EncodeRawCCITTG4(bits []byte, width, height int) ([]byte, error) {
	if len(bits) != ((width+7)/8)*height {
		return nil, errors.New("invalid packed bits length")
	}
	var outPtr *C.uchar
	var outSize C.long
	ret := C.EncodeRawG4(
		(*C.uchar)(unsafe.Pointer(&bits[0])),
		C.int(width), C.int(height),
		&outPtr, &outSize,
	)
	if ret != 0 {
		return nil, errors.New("libtiff CCITT-G4 encode failed")
	}
	defer C.free(unsafe.Pointer(outPtr))
	return C.GoBytes(unsafe.Pointer(outPtr), C.int(outSize)), nil
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

// // #cgo LDFLAGS: -ltiff -ljpeg -lwebp -lzstd -llzma -ldeflate -ljbig -lLerc -lz
// #cgo LDFLAGS: -static -ljpeg -ltiff
//#cgo LDFLAGS: -ljpeg -ltiff
