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
    if (!*outBuf) {
        free(mb.data);
        return -3;
    }
    memcpy(*outBuf, mb.data + payloadStart, payloadLen);
    *outSize = payloadLen;

    free(mb.data);
    return 0;
}


void rgb_to_gray_sse2(const uint8_t* rgb, uint8_t* gray, int npixels, int* ccitt_ready) {
    // const __m128i coeff_r = _mm_set1_epi16(30); // --
    // const __m128i coeff_g = _mm_set1_epi16(59); // --
    // const __m128i coeff_b = _mm_set1_epi16(11); // --
    const __m128i coeff_r = _mm_set1_epi16(77); // ++
    const __m128i coeff_g = _mm_set1_epi16(150); // ++
    const __m128i coeff_b = _mm_set1_epi16(29); // ++
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

        // r = _mm_mullo_epi16(r, coeff_r); // --
        // g = _mm_mullo_epi16(g, coeff_g); // --
        // b = _mm_mullo_epi16(b, coeff_b); // --

        // __m128i sum = _mm_add_epi16(r, g); // --
        // sum = _mm_add_epi16(sum, b); // --

        // sum = _mm_srli_epi16(sum, 6); // --

        r = _mm_mullo_epi16(r, coeff_r); // ++
        g = _mm_mullo_epi16(g, coeff_g); // ++
        b = _mm_mullo_epi16(b, coeff_b); // ++
        __m128i sum = _mm_add_epi16(_mm_add_epi16(r, g), b); // ++
        sum = _mm_srli_epi16(sum, 8); // ++

        __m128i res8 = _mm_packus_epi16(sum, zero);
        _mm_storel_epi64((__m128i*)(gray + i), res8);
    }

    int bad_count = 0;

    for (; i < npixels; i++) {
        uint8_t r = rgb[i * 3 + 0];
        uint8_t g = rgb[i * 3 + 1];
        uint8_t b = rgb[i * 3 + 2];
        //gray[i] = (r * 30 + g * 59 + b * 11) / 100; // --
        gray[i] = (r * 77 + g * 150 + b * 29) >> 8;   // ++
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
int read_raster(const char* path,
                uint32_t** raster, uint16_t* orig_dpi, uint32_t* orig_width, uint32_t* orig_height)
{
    TIFF* tif = TIFFOpen(path, "r");
    if (!tif) return -1;

    TIFFSetWarningHandler(NULL);

    uint32_t width = 0, height = 0;

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

    *raster = malloc((size_t)width * (size_t)height * sizeof(uint32_t));
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
int read_pxls_from_raster(uint32_t* raster, const size_t width, const size_t height,
                        uint8_t** pxls_buff, bool* gray, bool* ccitt_ready)
{
    if (!raster || !pxls_buff || !gray || !ccitt_ready) {
        return -1;
    }
    if (width == 0 || height == 0) {
        return -2;
    }
    if (width > SIZE_MAX / height) {
        return -3;
    }
    size_t npixels = width * height;

    if (npixels > SIZE_MAX / 3) {
        return -4;
    }

    uint32_t px;
    uint8_t r, g, b;
    size_t dst;
    int gr_count = 0;
    uint8_t* rgb = malloc(npixels * 3);
    if (!rgb) {
        return -5;
    }

    for (size_t y = 0; y < height; y++) {
        for (size_t x = 0; x < width; x++) {

            px = raster[y * width + x];
            dst = (y * width + x) * 3;

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
    if (gray_ratio > GRAY_RATIO) {
        uint8_t* gray_buff =  malloc(npixels);
        if (!gray_buff) {
            free(rgb);
            return -6;
        }
        bool ready_for_ccitt = false;
        rgb_to_gray_sse2(rgb, gray_buff, npixels, &ready_for_ccitt);

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
                                    uint8_t** pxls_buff, bool* gray, bool* ccitt_ready, 
                                    int target_dpi, int orig_dpi)
{
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
    uint32_t new_width  = (uint32_t)((*width)  * resample_scale + 0.5);
    uint32_t new_height = (uint32_t)((*height) * resample_scale + 0.5);

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
    int gr_count = 0;
    int px_index;

    // bilinear resampling
    for (size_t y = 0; y < new_height; y++) {
        double fy = y / resample_scale;
        int    y0 = (int)floor(fy);
        double wy = fy - y0;
        if (y0 < 0)       { y0 = 0;    wy = 0; }
        if (y0 >= (*height)-1) { y0 = (*height)-2; wy = 1; }
        int y1 = y0 + 1;

        for (size_t x = 0; x < new_width; x++) {
            double fx = x / resample_scale;
            int    x0 = (int)floor(fx);
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
    if (gray_ratio > GRAY_RATIO) {
        uint8_t* gray_buff =  malloc(npixels);

        if (!gray_buff) {
            free(rgb_resampled);
            return -7;
        }
        bool ready_for_ccitt = false;
        rgb_to_gray_sse2(rgb_resampled, gray_buff, npixels, &ready_for_ccitt);

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
                         int* ccitt_filter, int* gray_filter,
                         int* outWidth, int* outHeight, int* outDpi)
{

    int rc = 0;

    uint16_t orig_dpi = 0;
    uint32_t orig_width = 0, orig_height = 0;
    uint32_t* raster;

    rc = read_raster(path, &raster, &orig_dpi, &orig_width, &orig_height);
    if (rc != 0) {
        return rc;
    }

    uint32_t width = orig_width;
    uint32_t height = orig_height;


    if (orig_dpi == 0) {
        orig_dpi = 300;
    }
    bool rgb_need_resample = (rgb_target_dpi != orig_dpi);
    bool gray_need_resample = (gray_target_dpi != orig_dpi);

    uint8_t* pixel_buffer = NULL;
    bool gray = false;
    bool ccitt_ready = false;

    rc = read_pxls_from_raster(raster, width, height, &pixel_buffer, &gray, &ccitt_ready);
    if (rc != 0) {
        if (raster != NULL) {
            free(raster);
            raster = NULL;
        }
        if (pixel_buffer != NULL) {
            free(pixel_buffer);
            pixel_buffer = NULL;
        }
        return rc;
    }
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
    } else {
        if (rgb_need_resample) {
            uint8_t* rgb_buff = NULL;
            rc = read_pxls_resampled_from_raster(raster, &width, &height, &rgb_buff, &gray, &ccitt_ready,
                                                 rgb_target_dpi, orig_dpi);
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
    ccitt_ready = 0; // exclude/force CCITT for now

    if (ccitt_ready) {
        *ccitt_filter = 1;
        *gray_filter = 0;
        *outBuf = pixel_buffer;
        *outSize = (unsigned long)(width * height);
        *outWidth = width;
        *outHeight = height;
        *outDpi = gray_target_dpi;
        free(raster);
        return 0;
    }

    *ccitt_filter = 0;
    *gray_filter = gray ? 1 : 0;
    if (!gray) {
        rc = write_jpeg_to_mem(width, height, pixel_buffer, rgb_quality, rgb_target_dpi, !gray, outBuf, outSize);
        *outDpi = rgb_target_dpi;
    } else {
        rc = write_jpeg_to_mem(width, height, pixel_buffer, gray_quality, gray_target_dpi, gray, outBuf, outSize);
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

int ExtractCCITTRaw(const char* path,
                    unsigned char** outBuf,
                    unsigned long* outSize,
                    int* width,
                    int* height)
{
    TIFF* tif = TIFFOpen(path, "r");
    if (!tif) return -1;

    // Получаем размеры
    int w=0, h=0;
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
    *width   = (int)w;
    *height  = (int)h;
    return 0;
}