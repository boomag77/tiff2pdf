#ifndef CCITT_EXTRACTOR_H
#define CCITT_EXTRACTOR_H

#include <stddef.h>

int extract_ccitt_raw(const char*     path,
                    unsigned char** outBuf,
                    unsigned long*  outSize,
                    size_t*         width,
                    size_t*         height);


#endif