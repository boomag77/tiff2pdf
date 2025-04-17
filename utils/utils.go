package utils

import (
	"fmt"
	"os"

	"github.com/dsoprea/go-exif/v3"
	exifcommon "github.com/dsoprea/go-exif/v3/common"
)

func GetTIFFDPI(filePath string) (float64, float64, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return 300, 300, err
	}

	rawExif, err := exif.SearchAndExtractExif(data)
	if err != nil {
		return 300, 300, fmt.Errorf("EXIF not found: %v", err)
	}

	im := exifcommon.NewIfdMapping()
	ti := exif.NewTagIndex()
	if err := exifcommon.LoadStandardIfds(im); err != nil {
		return 300, 300, err
	}

	_, index, err := exif.Collect(im, ti, rawExif)
	if err != nil {
		return 300, 300, err
	}

	dpiX, dpiY := 300.0, 300.0

	if tag, err := index.RootIfd.FindTagWithName("XResolution"); err == nil {
		if val, err := tag[0].Value(); err == nil {
			if rats, ok := val.([]exifcommon.Rational); ok && len(rats) > 0 {
				dpiX = float64(rats[0].Numerator) / float64(rats[0].Denominator)
			}
		}
	}

	if tag, err := index.RootIfd.FindTagWithName("YResolution"); err == nil {
		if val, err := tag[0].Value(); err == nil {
			if rats, ok := val.([]exifcommon.Rational); ok && len(rats) > 0 {
				dpiY = float64(rats[0].Numerator) / float64(rats[0].Denominator)
			}
		}
	}

	if tag, err := index.RootIfd.FindTagWithName("ResolutionUnit"); err == nil {
		if val, err := tag[0].Value(); err == nil {
			if u, ok := val.(uint16); ok && u == 3 {
				dpiX *= 2.54
				dpiY *= 2.54
			}
		}
	}

	return dpiX, dpiY, nil
}
