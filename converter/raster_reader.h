#ifndef RASTER_READER_H
#define RASTER_READER_H

#include <stddef.h>
#include <stdint.h>

int read_raster(const char* path,
                uint32_t** raster,
                 uint16_t* orig_dpi, 
                 size_t* orig_width, 
                 size_t* orig_height);

#endif