#ifndef CONVERTER_H
#define CONVERTER_H

#include <stdint.h>
#include <stddef.h>
#include <stdbool.h>

void rgb_to_gray_sse2(const uint8_t* rgb, uint8_t* gray, size_t npixels, int* ccitt_ready);

int read_raster(const char* path,
                uint32_t** raster,
                 uint16_t* orig_dpi, 
                 size_t* orig_width, 
                 size_t* orig_height);

int read_pxls_from_raster(uint32_t* raster, size_t* width, size_t* height,
                        uint8_t** pxls_buff, bool* gray, int* ccitt_ready);

int read_pxls_resampled_from_raster(uint32_t* raster, size_t* width, size_t* height,
                                    uint8_t** pxls_buff, bool* gray, int* ccitt_ready,
                                    int target_dpi, int orig_dpi);

typedef struct {
    const char* path;
    int raw;
    int rgb_quality;
    int gray_quality;
    int rgb_target_dpi;
    int gray_target_dpi;
} tiff_convert_options;

int convert_tiff_to_data(const tiff_convert_options* options,
                         unsigned char** outBuf, unsigned long* outSize,
                         int* ccitt_filter, bool* gray_filter,
                         size_t* outWidth, size_t* outHeight, int* outDpi);

int write_jpeg_to_mem(uint32_t width, uint32_t height, uint8_t* buffer,
                      int quality, int dpi, int gray,
                      unsigned char** out, unsigned long* outSize);


int encode_raw_g4(
    unsigned char *bits1,
    size_t          width,
    size_t          height,
    unsigned char   **outBuf,
    unsigned long   *outSize
);

int extract_ccitt_raw(const char*     path,
                    unsigned char** outBuf,
                    unsigned long*  outSize,
                    size_t*         width,
                    size_t*         height);

int get_compression_type(const char* path);

void write_tiff(const char* filename,
                    uint32_t width, uint32_t height,
                    unsigned char* buf, size_t buf_size,
                    int dpi, int compression, int gray);
       
                    
#endif // CONVERTER_H