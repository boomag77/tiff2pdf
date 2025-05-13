//go:build cgo
// +build cgo

package converter

/*

#cgo LDFLAGS: -ljpeg -ltiff


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

#ifndef TIFFTAG_JPEGCOLORMODE
#define TIFFTAG_JPEGCOLORMODE 65538
#endif

#ifndef JPEGCOLORMODE_RGB
#define JPEGCOLORMODE_RGB     1
#endif

#define GRAY_THRESHOLD 2
#define GRAY_RATIO 0.9
#define LOWER_THRESHOLD 30
#define UPPER_THRESHOLD 225
#define CCITT_THRESHOLD 98


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
    size_t          width,
    size_t          height,
    unsigned char   **outBuf,
    unsigned long   *outSize
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
    unsigned long payloadStart = mb.size;

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
    unsigned long payloadLen = mb.size - payloadStart;
    *outBuf  = (unsigned char*)malloc(payloadLen);
    if (!*outBuf) {
        free(mb.data);
        return -3;
    }
    memcpy(*outBuf, mb.data + payloadStart, payloadLen);
    *outSize = payloadLen;

    free(mb.data);
    return 0;
}


void rgb_to_gray_sse2(const uint8_t* rgb, uint8_t* gray, size_t npixels, int* ccitt_ready) {
    // const __m128i coeff_r = _mm_set1_epi16(30); // --
    // const __m128i coeff_g = _mm_set1_epi16(59); // --
    // const __m128i coeff_b = _mm_set1_epi16(11); // --
    const __m128i coeff_r = _mm_set1_epi16(77); // ++
    const __m128i coeff_g = _mm_set1_epi16(150); // ++
    const __m128i coeff_b = _mm_set1_epi16(29); // ++
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

    //size_t bad_count = 0;
    size_t tail = (npixels / 8) * 8;
    for (size_t i = tail; i < npixels; i++) {
        uint8_t r = rgb[i * 3 + 0];
        uint8_t g = rgb[i * 3 + 1];
        uint8_t b = rgb[i * 3 + 2];
        //gray[i] = (r * 30 + g * 59 + b * 11) / 100; // --
        gray[i] = (r * 77 + g * 150 + b * 29) >> 8;   // ++
        if (gray[i] > LOWER_THRESHOLD && gray[i] < UPPER_THRESHOLD) {
            //bad_count++;
            bw_pixels--;
        }
    }
    // double bad_ratio = (double)bad_count / (double)(npixels);
    // if (bad_ratio < 0.0001) {
    //     *ccitt_ready = 1;
    // } else {
    //     *ccitt_ready = 0;
    // }

    //size_t good_count = npixels - bad_count;

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

// TIFF → JPEG via RGBA
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

            // четыре пикселя вокруг источника
            uint32_t p00 = raster[y0*(*width) + x0];
            uint32_t p10 = raster[y0*(*width) + x1];
            uint32_t p01 = raster[y1*(*width) + x0];
            uint32_t p11 = raster[y1*(*width) + x1];

            // извлекаем каналы
            uint8_t r00 = TIFFGetR(p00), g00 = TIFFGetG(p00), b00 = TIFFGetB(p00);
            uint8_t r10 = TIFFGetR(p10), g10 = TIFFGetG(p10), b10 = TIFFGetB(p10);
            uint8_t r01 = TIFFGetR(p01), g01 = TIFFGetG(p01), b01 = TIFFGetB(p01);
            uint8_t r11 = TIFFGetR(p11), g11 = TIFFGetG(p11), b11 = TIFFGetB(p11);

            // линейная интерполяция по X
            double r0 = r00*(1-wx) + r10*wx;
            double g0 = g00*(1-wx) + g10*wx;
            double b0 = b00*(1-wx) + b10*wx;
            double r1 = r01*(1-wx) + r11*wx;
            double g1 = g01*(1-wx) + g11*wx;
            double b1 = b01*(1-wx) + b11*wx;

            // линейная интерполяция по Y
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

// TIFF → JPEG via RGB
int convert_tiff_to_data(const char* path,
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
            printf("read_pxls_resampled_from_raster rc: height: %zu, width: %zu, gray: %d, ccitt_ready: %d\n", height, width, gray, ccitt_ready);
            if (rc != 0) {
                free(raster);
                free(pixel_buffer);
                return rc;
            }
            free(pixel_buffer);
            pixel_buffer = rgb_buff;
        }
    }


    // int npixels = width * height;


    //int ccitt_ready = 0;

    //uint8_t* gray;

    // if (use_gray) {
    //     gray = malloc(npixels);
    //     rgb_to_gray_sse2(rgb, gray, npixels, &ccitt_ready);
    // }

    //int rc = -1;
    //ccitt_ready = 0; // exclude/force CCITT for now

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
        rc = write_jpeg_to_mem((uint32_t)width, (uint32_t)height, pixel_buffer, rgb_quality, rgb_target_dpi, gray ? 1 : 0, outBuf, outSize);
        *outDpi = rgb_target_dpi;
    } else {
        rc = write_jpeg_to_mem((uint32_t)width, (uint32_t)height, pixel_buffer, gray_quality, gray_target_dpi, gray ? 1 : 0, outBuf, outSize);
        *outDpi = gray_target_dpi;
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

int ExtractCCITTRaw(const char*     path,
                    unsigned char** outBuf,
                    unsigned long*  outSize,
                    size_t*         width,
                    size_t*         height)
{
    TIFF* tif = TIFFOpen(path, "r");
    if (!tif) return -1;

    // Получаем размеры
    size_t w=0, h=0;
    TIFFGetField(tif, TIFFTAG_IMAGEWIDTH,  &w);
    TIFFGetField(tif, TIFFTAG_IMAGELENGTH, &h);

    // Убедимся, что это действительно CCITT-G4
    uint16_t comp=0;
    TIFFGetField(tif, TIFFTAG_COMPRESSION, &comp);
    if (comp != COMPRESSION_CCITTFAX4) {
        TIFFClose(tif);
        return -2;
    }

    // Считаем количество строчек (strips)
    int nstrips = TIFFNumberOfStrips(tif);

    // Узнаём общий размер всех raw-строк
    toff_t total = 0;
    for (int i = 0; i < nstrips; i++) {
        total += TIFFRawStripSize(tif, i);
    }

    // Выделяем буфер и читаем подряд
    unsigned char* buf = malloc(total);
    if (!buf) {
        TIFFClose(tif);
        return -3;
    }
    toff_t offset = 0;
    for (int i = 0; i < nstrips; i++) {
        int sz = TIFFRawStripSize(tif, i);
        TIFFReadRawStrip(tif, i, buf + offset, sz);
        offset += sz;
    }

    TIFFClose(tif);

    *outBuf  = buf;
    *outSize = total;
    *width   = w;
    *height  = h;
    return 0;
}

// void WriteCCITTTIFF(const char* filename,
//                     int width, int height,
//                     unsigned char* ccitt_buf, size_t ccitt_size,
//                     int dpi) {
//     TIFF* out = TIFFOpen(filename, "w");
//     if (!out) return;
//     TIFFSetField(out, TIFFTAG_IMAGEWIDTH, (uint32)width);
//     TIFFSetField(out, TIFFTAG_IMAGELENGTH, (uint32)height);
//     TIFFSetField(out, TIFFTAG_COMPRESSION, COMPRESSION_CCITTFAX4);
//     TIFFSetField(out, TIFFTAG_PHOTOMETRIC, PHOTOMETRIC_MINISWHITE);
//     TIFFSetField(out, TIFFTAG_FILLORDER, FILLORDER_MSB2LSB);
//     TIFFSetField(out, TIFFTAG_ROWSPERSTRIP, (uint32)height);
//     TIFFSetField(out, TIFFTAG_XRESOLUTION, (float)dpi);
//     TIFFSetField(out, TIFFTAG_YRESOLUTION, (float)dpi);
//     TIFFSetField(out, TIFFTAG_RESOLUTIONUNIT, RESUNIT_INCH);
//     TIFFWriteRawStrip(out, 0, ccitt_buf, (tmsize_t)ccitt_size);
//     TIFFClose(out);
// }

*/
import "C"
import (
	"errors"
	"fmt"
	"unsafe"
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
		rc := C.ExtractCCITTRaw(
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

	rc := C.convert_tiff_to_data(
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

		//filtered := medianFilterLight(goGray, int(w), int(h))
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
		//return ccittData, int(use_ccitt), int(use_gray), int(w), int(h), dpi, nil
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

// ConvertTIFFtoData reads TIFF from file path and returns JPEG-encoded []byte + width, height, dpi
// func ConvertTIFFtoData(path string, quality int, dpi int) (data []byte, ccitt int, gray int, width, height, actualDpi int, err error) {
// 	cPath := C.CString(path) // Converts Go string to C string
// 	defer func() {
// 		if cPath != nil {
// 			C.free(unsafe.Pointer(cPath))
// 		}
// 	}()

// 	var outBuf *C.uchar
// 	var outSize C.ulong
// 	var w, h, d C.int
// 	var use_ccitt C.int
// 	var use_gray C.int

// 	comp := C.get_compression_type(cPath)
// 	if comp == 2 || comp == 3 || comp == 4 {
// 		// CCITT
// 		ccitt := 1
// 		rc := C.ExtractCCITTRaw(
// 			cPath,
// 			&outBuf, &outSize,
// 			&w, &h)
// 		if rc != 0 {
// 			C.free(unsafe.Pointer(outBuf))
// 			return nil, 0, 0, 0, 0, 0, fmt.Errorf("ExtractCCITTRaw failed with code %d", int(rc))
// 		}
// 		data = C.GoBytes(unsafe.Pointer(outBuf), C.int(outSize))
// 		if outBuf != nil {
// 			C.free(unsafe.Pointer(outBuf))
// 		}
// 		return data, ccitt, 0, int(w), int(h), dpi, nil
// 	}

// 	rc := C.convert_tiff_to_data(
// 		cPath,
// 		C.int(quality),
// 		C.int(dpi),
// 		&outBuf, &outSize,
// 		&use_ccitt,
// 		&use_gray,
// 		&w, &h, &d,
// 	)

// 	if rc != 0 {
// 		if outBuf != nil {
// 			C.free(unsafe.Pointer(outBuf))
// 		}
// 		return nil, 0, 0, 0, 0, 0, fmt.Errorf("convert_tiff_to_data failed with code %d", int(rc))
// 	}

// 	if use_ccitt == 1 {
// 		goGray := C.GoBytes(unsafe.Pointer(outBuf), C.int(outSize))

// 		//filtered := medianFilterLight(goGray, int(w), int(h))
// 		packed := packGrayTo1BitOtsuClose(goGray, int(w), int(h))

// 		C.free(unsafe.Pointer(outBuf)) // Освобождаем оригинальный буфер

// 		ccittData, encodeErr := EncodeRawCCITTG4(packed, int(w), int(h))
// 		if encodeErr != nil {
// 			return nil, 0, 0, 0, 0, 0, fmt.Errorf("ccittg4 encode failed: %v", encodeErr)
// 		}

// 		return ccittData, int(use_ccitt), int(use_gray), int(w), int(h), dpi, nil
// 	}

// 	data = C.GoBytes(unsafe.Pointer(outBuf), C.int(outSize))

// 	if outBuf != nil {
// 		C.free(unsafe.Pointer(outBuf))
// 	}
// 	return data, int(use_ccitt), int(use_gray), int(w), int(h), dpi, nil
// }

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
