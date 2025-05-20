//go:build cgo
// +build cgo

package converter

/*

#cgo CFLAGS: -I${SRCDIR}
#cgo LDFLAGS: -ljpeg -ltiff

#include <ccitt_encoder.h>
#include <ccitt_extractor.h>


#include <stdlib.h>
#include <stdint.h>
#include <stdio.h>
#include <stdbool.h>
#include <string.h>
#include <tiffio.h>
#include <jpeglib.h>
#include <math.h>
#include <limits.h>

#include <emmintrin.h> // SSE2

#include <settings.h>

// Convert RGB to grayscale using SSE2
void rgb_to_gray_sse2(const uint8_t* rgb, uint8_t* gray, size_t npixels, int* ccitt_ready) {

    const __m128i coeff_r = _mm_set1_epi16(77);
    const __m128i coeff_g = _mm_set1_epi16(150);
    const __m128i coeff_b = _mm_set1_epi16(29);
    const __m128i zero = _mm_setzero_si128();

    size_t bw_pixels = npixels;

    for (size_t i = 0; i + 8 <= npixels; i += 8) {
        uint8_t r0[8], g0[8], b0[8];

        for (size_t j = 0; j < 8; j++) {
            r0[j] = rgb[(i + j) * 3 + 0];
            g0[j] = rgb[(i + j) * 3 + 1];
            b0[j] = rgb[(i + j) * 3 + 2];
        }

        __m128i r = _mm_unpacklo_epi8(_mm_loadl_epi64((__m128i*)r0), zero);
        __m128i g = _mm_unpacklo_epi8(_mm_loadl_epi64((__m128i*)g0), zero);
        __m128i b = _mm_unpacklo_epi8(_mm_loadl_epi64((__m128i*)b0), zero);

        r = _mm_mullo_epi16(r, coeff_r); // ++
        g = _mm_mullo_epi16(g, coeff_g); // ++
        b = _mm_mullo_epi16(b, coeff_b); // ++
        __m128i sum = _mm_add_epi16(_mm_add_epi16(r, g), b); // ++
        sum = _mm_srli_epi16(sum, 8); // ++

        __m128i res8 = _mm_packus_epi16(sum, zero);
        _mm_storel_epi64((__m128i*)(gray + i), res8);

        uint8_t tmp[8];
        _mm_storel_epi64((__m128i*)tmp, res8);
        for (size_t j = 0; j < 8; j++) {
            if (tmp[j] > LOWER_THRESHOLD && tmp[j] < UPPER_THRESHOLD) {
                bw_pixels--;
            }
        }
    }

    size_t tail = (npixels / 8) * 8;
    for (size_t i = tail; i < npixels; i++) {
        uint8_t r = rgb[i * 3 + 0];
        uint8_t g = rgb[i * 3 + 1];
        uint8_t b = rgb[i * 3 + 2];
        gray[i] = (r * 77 + g * 150 + b * 29) >> 8;
        if (gray[i] > LOWER_THRESHOLD && gray[i] < UPPER_THRESHOLD) {
            //bad_count++;
            bw_pixels--;
        }
    }

    *ccitt_ready = (bw_pixels * 100 >= npixels * CCITT_THRESHOLD);

}

// JPEG encoder from RGBA → JPEG memory
int write_jpeg_to_mem(uint32_t width, uint32_t height, uint8_t* buffer,
                      int quality, int dpi, int gray,
                      unsigned char** out, unsigned long* outSize) {
    struct jpeg_compress_struct cinfo;
    struct jpeg_error_mgr jerr;

    JSAMPROW row_pointer[1];
    size_t row_stride = width * (gray ? 1 : 3);

    cinfo.err = jpeg_std_error(&jerr);
    jpeg_create_compress(&cinfo);
    jpeg_mem_dest(&cinfo, out, outSize);

    cinfo.image_width = width;
    cinfo.image_height = height;
    cinfo.input_components = gray ? 1 : 3;
    cinfo.in_color_space = gray ? JCS_GRAYSCALE : JCS_RGB;

    jpeg_set_defaults(&cinfo);
    jpeg_set_quality(&cinfo, quality, TRUE);
    //jpeg_simple_progression(&cinfo);
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

// Read TIFF raster
int read_raster(const char* path,
                uint32_t** raster, uint16_t* orig_dpi, size_t* orig_width, size_t* orig_height)
{
    TIFF* tif = TIFFOpen(path, "r");
    if (!tif) return -1;

    TIFFSetWarningHandler(NULL);

    size_t width = 0, height = 0;

    if (!TIFFGetField(tif, TIFFTAG_IMAGEWIDTH, &width) ||
        !TIFFGetField(tif, TIFFTAG_IMAGELENGTH, &height)) {
        TIFFClose(tif);
        return -2;
    }

    float xres = 0.0f, yres = 0.0f;
    uint16_t resUnit = RESUNIT_NONE;
    *orig_dpi = 0;

    if (TIFFGetField(tif, TIFFTAG_XRESOLUTION, &xres) &&
        TIFFGetField(tif, TIFFTAG_YRESOLUTION, &yres) &&
        TIFFGetField(tif, TIFFTAG_RESOLUTIONUNIT, &resUnit)) {
        if (resUnit == RESUNIT_INCH) {
            *orig_dpi = (uint16_t)(xres + 0.5f);
        } else if (resUnit == RESUNIT_CENTIMETER) {
            *orig_dpi = (uint16_t)(xres * 2.54f + 0.5f);
        }
    }

    if (width != 0 && height > SIZE_MAX / width) {
        TIFFClose(tif);
        return -3;
    }

    *orig_width = width;
    *orig_height = height;

    *raster = malloc(width * height * sizeof(uint32_t));
    if (!*raster) {
        TIFFClose(tif);
        return -4;
    }

    if (!TIFFReadRGBAImageOriented(tif, width, height, *raster, ORIENTATION_TOPLEFT, 0)) {
        free(*raster);
        TIFFClose(tif);
        return -5;
    }

    TIFFClose(tif);
    return 0;
}

// read pixels to rgb
int read_pxls_from_raster(uint32_t* raster, size_t* width, size_t* height,
                        uint8_t** pxls_buff, bool* gray, int* ccitt_ready)
{

    int ccitt_mode = *ccitt_ready;

    if (!raster || !pxls_buff || !gray || !ccitt_ready) {
        return -1;
    }
    if ((*width) == 0 || (*height) == 0) {
        return -2;
    }
    if ((*width) > SIZE_MAX / (*height)) {
        return -3;
    }
    size_t npixels = (*width) * (*height);

    if (npixels > SIZE_MAX / 3) {
        return -4;
    }

    uint32_t px;
    uint8_t r, g, b;
    size_t dst;
    size_t gr_count = 0;
    uint8_t* rgb = malloc(npixels * 3);
    if (!rgb) {
        return -5;
    }

    for (size_t y = 0; y < (*height); y++) {
        for (size_t x = 0; x < (*width); x++) {

            px = raster[y * (*width) + x];
            dst = (y * (*width) + x) * 3;

            r = TIFFGetR(px);
            g = TIFFGetG(px);
            b = TIFFGetB(px);
            rgb[dst + 0] = r;
            rgb[dst + 1] = g;
            rgb[dst + 2] = b;
            if (abs(r-g) < GRAY_THRESHOLD && abs(r-b) < GRAY_THRESHOLD && abs(g-b) < GRAY_THRESHOLD) {
                gr_count++;
            }
        }
    }

    double gray_ratio = (double)gr_count / (double)(npixels);
    if (gray_ratio > GRAY_RATIO || ccitt_mode == 1) {
        uint8_t* gray_buff =  malloc(npixels);
        if (!gray_buff) {
            free(rgb);
            return -6;
        }
        int ready_for_ccitt = 0;
        rgb_to_gray_sse2(rgb, gray_buff, npixels, &ready_for_ccitt);

        if (ccitt_mode == -1) {
            ready_for_ccitt = 0;
        }
        if (ccitt_mode == 1) {
            ready_for_ccitt = 1;
        }

        *pxls_buff = gray_buff;
        *gray = true;
        *ccitt_ready = ready_for_ccitt;

        free(rgb);
        return 0;
    }

    *pxls_buff = rgb;
    *gray = false;
    *ccitt_ready = false;

    return 0;
}

// read pixels to rgb_resampled
int read_pxls_resampled_from_raster(uint32_t* raster, size_t* width, size_t* height,
                                    uint8_t** pxls_buff, bool* gray, int* ccitt_ready,
                                    int target_dpi, int orig_dpi)
{
    int ccitt_mode = *ccitt_ready;

    if (!raster || !pxls_buff || !gray || !ccitt_ready) {
        return -1;
    }
    if ((*width) == 0 || (*height) == 0) {
        return -2;
    }
    if ((*width) > SIZE_MAX / (*height)) {
        return -3;
    }
    size_t npixels = (*width) * (*height);

    if (npixels > SIZE_MAX / 3) {
        return -4;
    }

    double resample_scale = (double)target_dpi / (double)orig_dpi;
    size_t new_width  = (*width)  * resample_scale + 0.5;
    size_t new_height = (*height) * resample_scale + 0.5;

    npixels = new_width * new_height;
    if (npixels > SIZE_MAX / 3) {
        return -5;
    }

    uint8_t* rgb_resampled = malloc(npixels * 3);
    if (!rgb_resampled) {
        return -6;
    }

    uint32_t px;
    uint8_t r, g, b;
    size_t gr_count = 0;
    size_t px_index;

    // bilinear resampling
    for (size_t y = 0; y < new_height; y++) {
        double fy = y / resample_scale;
        size_t    y0 = (size_t)floor(fy);
        double wy = fy - y0;
        if (y0 < 0)       { y0 = 0;    wy = 0; }
        if (y0 >= (*height)-1) { y0 = (*height)-2; wy = 1; }
        int y1 = y0 + 1;

        for (size_t x = 0; x < new_width; x++) {
            double fx = x / resample_scale;
            size_t    x0 = (size_t)floor(fx);
            double wx = fx - x0;
            if (x0 < 0)      { x0 = 0;    wx = 0; }
            if (x0 >= (*width)-1)  { x0 = (*width)-2;  wx = 1; }
            int x1 = x0 + 1;

            // 4 pixel neighbors
            uint32_t p00 = raster[y0*(*width) + x0];
            uint32_t p10 = raster[y0*(*width) + x1];
            uint32_t p01 = raster[y1*(*width) + x0];
            uint32_t p11 = raster[y1*(*width) + x1];

            // get channels
            uint8_t r00 = TIFFGetR(p00), g00 = TIFFGetG(p00), b00 = TIFFGetB(p00);
            uint8_t r10 = TIFFGetR(p10), g10 = TIFFGetG(p10), b10 = TIFFGetB(p10);
            uint8_t r01 = TIFFGetR(p01), g01 = TIFFGetG(p01), b01 = TIFFGetB(p01);
            uint8_t r11 = TIFFGetR(p11), g11 = TIFFGetG(p11), b11 = TIFFGetB(p11);

            // linear interpolation X
            double r0 = r00*(1-wx) + r10*wx;
            double g0 = g00*(1-wx) + g10*wx;
            double b0 = b00*(1-wx) + b10*wx;
            double r1 = r01*(1-wx) + r11*wx;
            double g1 = g01*(1-wx) + g11*wx;
            double b1 = b01*(1-wx) + b11*wx;

            // linear interpolation Y
            uint8_t rf = (uint8_t)(r0*(1-wy) + r1*wy + 0.5);
            uint8_t gf = (uint8_t)(g0*(1-wy) + g1*wy + 0.5);
            uint8_t bf = (uint8_t)(b0*(1-wy) + b1*wy + 0.5);

            px_index = y * new_width + x;
            rgb_resampled[px_index*3 + 0] = rf;
            rgb_resampled[px_index*3 + 1] = gf;
            rgb_resampled[px_index*3 + 2] = bf;
            if (abs(rf-gf) < GRAY_THRESHOLD && abs(rf-bf) < GRAY_THRESHOLD && abs(gf-bf) < GRAY_THRESHOLD) {
                gr_count++;
            }

        }
    }

    *width = new_width;
    *height = new_height;

    double gray_ratio = (double)gr_count / (double)(npixels);
    if (gray_ratio > GRAY_RATIO || ccitt_mode == 1) {
        uint8_t* gray_buff =  malloc(npixels);

        if (!gray_buff) {
            free(rgb_resampled);
            return -7;
        }
        int ready_for_ccitt = 0;
        rgb_to_gray_sse2(rgb_resampled, gray_buff, npixels, &ready_for_ccitt);

        if (ccitt_mode == -1) {
            ready_for_ccitt = 0;
        }
        if (ccitt_mode == 1) {
            ready_for_ccitt = 1;
        }

        *pxls_buff = gray_buff;
        *gray = true;
        *ccitt_ready = ready_for_ccitt;

        free(rgb_resampled);
        return 0;
    }

    *pxls_buff = rgb_resampled;
    *gray = false;
    *ccitt_ready = false;

    return 0;
}

// TIFF → Data (bytes)
int convert_tiff_to_data(int raw, const char* path,
                        int rgb_quality, int gray_quality,
                        int rgb_target_dpi, int gray_target_dpi,
                         unsigned char** outBuf, unsigned long* outSize,
                         int* ccitt_filter, bool* gray_filter,
                         size_t* outWidth, size_t* outHeight, int* outDpi)
{

    int rc = 0;

    int ccitt_mode = *ccitt_filter;

    uint16_t orig_dpi = 0;
    size_t orig_width = 0, orig_height = 0;
    uint32_t* raster;

    rc = read_raster(path, &raster, &orig_dpi, &orig_width, &orig_height);
    if (rc != 0) {
        return rc;
    }

    size_t width = orig_width;
    size_t height = orig_height;

    uint8_t* pixel_buffer = NULL;
    bool gray = false;
    int ccitt_ready = *ccitt_filter;

    rc = read_pxls_from_raster(raster, &width, &height, &pixel_buffer, &gray, &ccitt_ready);
    if (rc != 0) {
        free(raster);
        free(pixel_buffer);
        return rc;
    }

    if (orig_dpi == 0) {
        orig_dpi = gray ? gray_target_dpi : rgb_target_dpi;
    }

    bool rgb_need_resample = (rgb_target_dpi != orig_dpi);
    bool gray_need_resample = (gray_target_dpi != orig_dpi);


    if (gray) {
        if (gray_need_resample) {
            uint8_t* gray_buff = NULL;
            rc = read_pxls_resampled_from_raster(raster, &width, &height, &gray_buff, &gray, &ccitt_ready,
                gray_target_dpi, orig_dpi);
            if (rc != 0) {
                free(raster);
                free(pixel_buffer);
                return rc;
            }
            free(pixel_buffer);
            pixel_buffer = gray_buff;
        }
    } else if (!gray) {
        if (rgb_need_resample) {
            uint8_t* rgb_buff = NULL;
            rc = read_pxls_resampled_from_raster(raster, &width, &height, &rgb_buff, &gray, &ccitt_ready,
                                                 rgb_target_dpi, orig_dpi);
            //printf("read_pxls_resampled_from_raster rc: height: %zu, width: %zu, gray: %d, ccitt_ready: %d\n", height, width, gray, ccitt_ready);
            if (rc != 0) {
                free(raster);
                free(pixel_buffer);
                return rc;
            }
            free(pixel_buffer);
            pixel_buffer = rgb_buff;
        }
    }

    if ((ccitt_ready && ccitt_mode == 0) || ccitt_mode == 1) {
        *ccitt_filter = 1;
        *gray_filter = false;
        *outBuf = pixel_buffer;
        *outSize = (unsigned long)(width * height);
        *outWidth = width;
        *outHeight = height;
        *outDpi = gray_target_dpi;
        free(raster);
        return 0;
    }

    *ccitt_filter = 0;
    *gray_filter = gray ? true : false;
    if (!gray) {
        if (raw) {
            *outBuf = pixel_buffer;
            *outSize = (unsigned long)(width * height * 3);
            *outWidth = width;
            *outHeight = height;
            *outDpi = rgb_target_dpi;
            free(raster);
            return 0;
        } else {
            rc = write_jpeg_to_mem((uint32_t)width, (uint32_t)height, pixel_buffer, rgb_quality, rgb_target_dpi, gray ? 1 : 0, outBuf, outSize);
            *outDpi = rgb_target_dpi;
        }

    } else {
        if (raw) {
            *outBuf = pixel_buffer;
            *outSize = (unsigned long)(width * height);
            *outWidth = width;
            *outHeight = height;
            *outDpi = gray_target_dpi;
            free(raster);
            return 0;
        } else {
            rc = write_jpeg_to_mem((uint32_t)width, (uint32_t)height, pixel_buffer, gray_quality, gray_target_dpi, gray ? 1 : 0, outBuf, outSize);
            *outDpi = gray_target_dpi;
        }
    }

    *outWidth = width;
    *outHeight = height;
    free(pixel_buffer);
    free(raster);
    return rc == 0 ? 0 : -6;
}

int get_compression_type(const char* path) {
    TIFF* tif = TIFFOpen(path, "r");
    if (!tif) return -1;
    uint16_t compression = 0;
    TIFFGetField(tif, TIFFTAG_COMPRESSION, &compression);
    TIFFClose(tif);
    return (int)compression;
}

void WriteTIFF(const char* filename,
                    uint32_t width, uint32_t height,
                    unsigned char* buf, size_t buf_size,
                    int dpi, int compression, int gray)
{
    TIFF* out = TIFFOpen(filename, "w");
    if (!out) return;

    TIFFSetField(out, TIFFTAG_IMAGEWIDTH, width);
    TIFFSetField(out, TIFFTAG_IMAGELENGTH, height);
    TIFFSetField(out, TIFFTAG_COMPRESSION, compression);
    TIFFSetField(out, TIFFTAG_XRESOLUTION, (float)dpi);
    TIFFSetField(out, TIFFTAG_YRESOLUTION, (float)dpi);
    TIFFSetField(out, TIFFTAG_RESOLUTIONUNIT, RESUNIT_INCH);


    if (compression == COMPRESSION_CCITTFAX4)
    {
        TIFFSetField(out, TIFFTAG_PHOTOMETRIC, PHOTOMETRIC_MINISWHITE);
        TIFFSetField(out, TIFFTAG_FILLORDER, FILLORDER_MSB2LSB);
        TIFFSetField(out, TIFFTAG_ROWSPERSTRIP, height);
        TIFFSetField(out, TIFFTAG_BITSPERSAMPLE, 1);
        TIFFSetField(out, TIFFTAG_SAMPLESPERPIXEL, 1);

        TIFFWriteRawStrip(out, 0, buf, (tmsize_t)buf_size);
    }
        else if (compression == COMPRESSION_JPEG)
    {
        if (gray) {
            TIFFSetField(out, TIFFTAG_PHOTOMETRIC, PHOTOMETRIC_MINISBLACK);
            TIFFSetField(out, TIFFTAG_SAMPLESPERPIXEL, 1);
        } else {
            TIFFSetField(out, TIFFTAG_PHOTOMETRIC, PHOTOMETRIC_RGB);
            TIFFSetField(out, TIFFTAG_SAMPLESPERPIXEL, 3);
        }
        TIFFSetField(out, TIFFTAG_BITSPERSAMPLE,   8);

        TIFFSetField(out, TIFFTAG_PLANARCONFIG,    PLANARCONFIG_CONTIG);

        TIFFSetField(out, TIFFTAG_JPEGQUALITY,  90);
        TIFFSetField(out, TIFFTAG_ROWSPERSTRIP, height);
        TIFFWriteEncodedStrip(out, 0, (tdata_t)buf, (tmsize_t)(width * height * (gray ? 1 : 3)));

    } else {
        if (gray) {
            TIFFSetField(out, TIFFTAG_PHOTOMETRIC, PHOTOMETRIC_MINISBLACK);
            TIFFSetField(out, TIFFTAG_SAMPLESPERPIXEL, 1);
        } else {
            TIFFSetField(out, TIFFTAG_PHOTOMETRIC, PHOTOMETRIC_RGB);
            TIFFSetField(out, TIFFTAG_SAMPLESPERPIXEL, 3);
        }
        TIFFSetField(out, TIFFTAG_BITSPERSAMPLE,   8);
        TIFFSetField(out, TIFFTAG_PLANARCONFIG,    PLANARCONFIG_CONTIG);
        TIFFSetField(out, TIFFTAG_ROWSPERSTRIP, height);
        TIFFWriteEncodedStrip(out, 0, (tdata_t)buf, (tmsize_t)(width * height * (gray ? 1 : 3)));
    }
    TIFFClose(out);
}

*/
import "C"
import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"
)

const (
	// TIFF compression types
	CompressionNone    = C.COMPRESSION_NONE
	CompressionCCITTG4 = C.COMPRESSION_CCITTFAX4
	CompressionJPEG    = C.COMPRESSION_JPEG
	CompressionLZW     = C.COMPRESSION_LZW
)

type ImageData struct {
	Data      []byte
	CCITT     int
	Gray      bool
	Width     int
	Height    int
	ActualDpi int
}

func ConvertTIFF(path string, convParams ConversionParameters) (ImageData, error) {

	cPath := C.CString(path) // Converts Go string to C string
	defer func() {
		if cPath != nil {
			C.free(unsafe.Pointer(cPath))
		}
	}()

	var outBuf *C.uchar
	var outSize C.ulong
	var w, h C.size_t
	var d C.int
	var use_ccitt C.int
	var use_gray C.bool

	switch convParams.CCITT {
	case "auto":
		use_ccitt = 0
	case "on":
		use_ccitt = 1
	case "off":
		use_ccitt = -1
	}

	comp := C.get_compression_type(cPath)
	if comp == 2 || comp == 3 || comp == 4 {
		// CCITT
		ccitt := 1
		rc := C.extract_ccitt_raw(
			cPath,
			&outBuf, &outSize,
			&w, &h)
		if rc != 0 {
			C.free(unsafe.Pointer(outBuf))
			return ImageData{}, fmt.Errorf("ExtractCCITTRaw failed with code %d", int(rc))
		}
		dataSize := int(outSize)
		data := C.GoBytes(unsafe.Pointer(outBuf), C.int(dataSize))
		if outBuf != nil {
			C.free(unsafe.Pointer(outBuf))
		}
		return ImageData{
			Data:   data,
			CCITT:  ccitt,
			Gray:   false,
			Width:  int(w),
			Height: int(h),
		}, nil
	}

	rawFlag := 0

	if convParams.Raw {
		rawFlag = 1
	}

	rc := C.convert_tiff_to_data(
		C.int(rawFlag),
		cPath,
		C.int(convParams.TargetRGBjpegQuality),
		C.int(convParams.TargetGrayjpegQuality),
		C.int(convParams.TargetRGBdpi),
		C.int(convParams.TargetGraydpi),
		&outBuf, &outSize,
		&use_ccitt,
		&use_gray,
		&w, &h, &d,
	)
	if rc != 0 {
		if outBuf != nil {
			C.free(unsafe.Pointer(outBuf))
		}
		return ImageData{}, fmt.Errorf("convert_tiff_to_data failed with code %d", int(rc))
	}

	if use_ccitt == 1 && convParams.CCITT != "off" {
		dataSize := int(outSize)
		goGray := C.GoBytes(unsafe.Pointer(outBuf), C.int(dataSize))

		// filtered := medianFilterLight(goGray, int(w), int(h))
		// filtered := medianFilterLight(goGray, int(w), int(h))
		//packed := packGrayTo1BitDither(goGray, int(w), int(h))
		packed := packGrayTo1BitOtsuClose(goGray, int(w), int(h))

		C.free(unsafe.Pointer(outBuf)) // Освобождаем оригинальный буфер

		ccittData, encodeErr := EncodeRawCCITTG4(packed, int(w), int(h))
		if encodeErr != nil {
			return ImageData{}, fmt.Errorf("ccittg4 encode failed: %v", encodeErr)
		}

		return ImageData{
			Data:      ccittData,
			CCITT:     int(use_ccitt),
			Gray:      bool(use_gray),
			Width:     int(w),
			Height:    int(h),
			ActualDpi: convParams.TargetGraydpi,
		}, nil
	}
	dataSize := int(outSize)
	data := C.GoBytes(unsafe.Pointer(outBuf), C.int(dataSize))

	if outBuf != nil {
		C.free(unsafe.Pointer(outBuf))
	}

	var actDPI int = 0
	if use_gray {
		actDPI = convParams.TargetGraydpi
	} else {
		actDPI = convParams.TargetRGBdpi
	}

	return ImageData{
		Data:      data,
		CCITT:     int(use_ccitt),
		Gray:      bool(use_gray),
		Width:     int(w),
		Height:    int(h),
		ActualDpi: actDPI,
	}, nil
}

func saveDataToTIFFFile(tiffMode string, origFilePath string, outputs []string, width, height int, data []byte, dpi int, compression int, gray bool) error {

	//fmt.Println("saveDataToTIFFFile: filePath:", filePath, "width:", width, "height:", height, "dpi:", dpi)

	// switch tiffMode {
	// case "convert":
	// case "replace":
	// case "append":
	// default:
	// 	return fmt.Errorf("unsupported tiffMode: %s", tiffMode)
	// }

	if tiffMode == "convert" {
		base := filepath.Base(origFilePath)
		fileName := strings.TrimSuffix(base, filepath.Ext(base))
		tmpProcessedFileName := fileName + ".tmp"
		processedFileName := fileName + ".tif"

		cBuf := C.CBytes(data)
		defer C.free(cBuf)
		grayInt := 0
		if gray {
			grayInt = 1
		}

		var wg sync.WaitGroup
		errs := make(chan error, len(outputs))

		for _, outDir := range outputs {
			outDir := outDir // Capture to local variable
			wg.Add(1)
			go func() {
				defer wg.Done()
				tmpProcessedFilePath := filepath.Join(outDir, tmpProcessedFileName)
				cTmp := C.CString(tmpProcessedFilePath)
				defer C.free(unsafe.Pointer(cTmp))

				C.WriteTIFF(
					cTmp,
					C.uint32_t(width),
					C.uint32_t(height),
					(*C.uchar)(cBuf),
					C.size_t(len(data)),
					C.int(dpi),
					C.int(compression),
					C.int(grayInt),
				)
				info, err := os.Stat(tmpProcessedFilePath)
				if err != nil {
					errs <- fmt.Errorf("failed to get file info: %v", err)
					return
				}
				if info.Size() == 0 {
					errs <- fmt.Errorf("file is empty: %s", tmpProcessedFilePath)
					return
				}
				if err := os.Rename(tmpProcessedFilePath, filepath.Join(outDir, processedFileName)); err != nil {
					errs <- fmt.Errorf("failed to rename file: %v", err)
					return
				}
				errs <- nil
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				return fmt.Errorf("error saving TIFF file: %v", err)
			}
		}
	} else if tiffMode == "replace" {
		base := filepath.Base(origFilePath)
		fileName := strings.TrimSuffix(base, filepath.Ext(base))
		tmpProcessedFileName := fileName + ".tmp"
		processedFileName := fileName + ".tif"
		tmpProcessedFilePath := filepath.Join(filepath.Dir(origFilePath), tmpProcessedFileName)
		processedFilePath := filepath.Join(filepath.Dir(origFilePath), processedFileName)
		cBuf := C.CBytes(data)
		defer C.free(cBuf)
		grayInt := 0
		if gray {
			grayInt = 1
		}
		cTmp := C.CString(tmpProcessedFilePath)
		defer C.free(unsafe.Pointer(cTmp))
		C.WriteTIFF(
			cTmp,
			C.uint32_t(width),
			C.uint32_t(height),
			(*C.uchar)(cBuf),
			C.size_t(len(data)),
			C.int(dpi),
			C.int(compression),
			C.int(grayInt),
		)
		info, err := os.Stat(tmpProcessedFilePath)
		if err != nil {
			return fmt.Errorf("failed to get file info: %v", err)
		}
		if info.Size() == 0 {
			return fmt.Errorf("file is empty: %s", tmpProcessedFilePath)
		}
		_ = os.Remove(origFilePath)
		if err := os.Rename(tmpProcessedFilePath, processedFilePath); err != nil {
			return fmt.Errorf("failed to rename file: %v", err)
		}
	} else if tiffMode == "append" {
		base := filepath.Base(origFilePath)
		fileName := strings.TrimSuffix(base, filepath.Ext(base))
		tmpProcessedFileName := fileName + ".tmp"
		processedFileName := "_" + fileName + ".tif"
		tmpProcessedFilePath := filepath.Join(filepath.Dir(origFilePath), tmpProcessedFileName)
		processedFilePath := filepath.Join(filepath.Dir(origFilePath), processedFileName)
		cBuf := C.CBytes(data)
		defer C.free(cBuf)
		grayInt := 0
		if gray {
			grayInt = 1
		}
		cTmp := C.CString(tmpProcessedFilePath)
		defer C.free(unsafe.Pointer(cTmp))
		C.WriteTIFF(
			cTmp,
			C.uint32_t(width),
			C.uint32_t(height),
			(*C.uchar)(cBuf),
			C.size_t(len(data)),
			C.int(dpi),
			C.int(compression),
			C.int(grayInt),
		)
		info, err := os.Stat(tmpProcessedFilePath)
		if err != nil {
			return fmt.Errorf("failed to get file info: %v", err)
		}
		if info.Size() == 0 {
			return fmt.Errorf("file is empty: %s", tmpProcessedFilePath)
		}
		if err := os.Rename(tmpProcessedFilePath, processedFilePath); err != nil {
			return fmt.Errorf("failed to rename file: %v", err)
		}
	} else {
		return fmt.Errorf("unsupported tiffMode: %s", tiffMode)
	}

	return nil
}

func EncodeRawCCITTG4(bits []byte, width, height int) ([]byte, error) {
	if len(bits) != ((width+7)/8)*height {
		return nil, errors.New("invalid packed bits length")
	}
	var outPtr *C.uchar
	var outSize C.ulong
	ret := C.EncodeRawG4(
		(*C.uchar)(unsafe.Pointer(&bits[0])),
		C.size_t(width), C.size_t(height),
		&outPtr, &outSize,
	)
	if ret != 0 {
		return nil, errors.New("libtiff CCITT-G4 encode failed")
	}
	defer C.free(unsafe.Pointer(outPtr))
	size := int(outSize)
	return C.GoBytes(unsafe.Pointer(outPtr), C.int(size)), nil
}

// // #cgo LDFLAGS: -ltiff -ljpeg -lwebp -lzstd -llzma -ldeflate -ljbig -lLerc -lz
// #cgo LDFLAGS: -static -ljpeg -ltiff
//#cgo LDFLAGS: -ljpeg -ltiff
