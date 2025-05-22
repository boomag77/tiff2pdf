#include <stdlib.h>

#include "converter.h"


int convert_tiff_to_data(const tiff_convert_options* options,
                         unsigned char** outBuf, 
                         unsigned long* outSize,
                         int* ccitt_filter, 
                         bool* gray_filter,
                         size_t* outWidth, 
                         size_t* outHeight, 
                         int* outDpi)
{

    int rc = 0;

    int ccitt_mode = *ccitt_filter;

    uint16_t orig_dpi = 0;
    size_t orig_width = 0, orig_height = 0;
    uint32_t* raster;

    rc = read_raster(
        options->path, 
        &raster, 
        &orig_dpi, 
        &orig_width, 
        &orig_height
    );

    if (rc != 0) { return rc; }

    size_t width = orig_width;
    size_t height = orig_height;

    uint8_t* pixel_buffer = NULL;
    bool gray = false;
    int ccitt_ready = *ccitt_filter;

    rc = read_pxls_from_raster(
        raster, 
        &width, 
        &height, 
        &pixel_buffer, 
        &gray, 
        &ccitt_ready
    );

    if (rc != 0) {
        free(raster);
        free(pixel_buffer);
        return rc;
    }

    if (orig_dpi == 0) {
        orig_dpi = gray ? options->gray_target_dpi : options->rgb_target_dpi;
    }

    bool rgb_need_resample = (options->rgb_target_dpi != orig_dpi);
    bool gray_need_resample = (options->gray_target_dpi != orig_dpi);


    if (gray) {
        if (gray_need_resample) {
            uint8_t* gray_buff = NULL;
            rc = read_pxls_resampled_from_raster(raster, &width, &height, &gray_buff, &gray, &ccitt_ready,
                options->gray_target_dpi, orig_dpi);
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
            rc = read_pxls_resampled_from_raster(
                raster, 
                &width, 
                &height, 
                &rgb_buff, 
                &gray, 
                &ccitt_ready,
                options->rgb_target_dpi, 
                orig_dpi
            );
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
        *outDpi = options->gray_target_dpi;
        free(raster);
        return 0;
    }

    *ccitt_filter = 0;
    *gray_filter = gray ? true : false;
    if (!gray) {
        if (options->raw) {
            *outBuf = pixel_buffer;
            *outSize = (unsigned long)(width * height * 3);
            *outWidth = width;
            *outHeight = height;
            *outDpi = options->rgb_target_dpi;
            free(raster);
            return 0;
        } else {
            rc = write_jpeg_to_mem((uint32_t)width, 
                                   (uint32_t)height, 
                                   pixel_buffer, 
                                   options->rgb_quality, 
                                   options->rgb_target_dpi, 
                                   gray ? 1 : 0, 
                                   outBuf, outSize
            );
            *outDpi = options->rgb_target_dpi;
        }

    } else {
        if (options->raw) {
            *outBuf = pixel_buffer;
            *outSize = (unsigned long)(width * height);
            *outWidth = width;
            *outHeight = height;
            *outDpi = options->gray_target_dpi;
            free(raster);
            return 0;
        } else {
            rc = write_jpeg_to_mem((uint32_t)width, (uint32_t)height, pixel_buffer, options->gray_quality, options->gray_target_dpi, gray ? 1 : 0, outBuf, outSize);
            *outDpi = options->gray_target_dpi;
        }
    }

    *outWidth = width;
    *outHeight = height;
    free(pixel_buffer);
    free(raster);
    return rc == 0 ? 0 : -6;
}