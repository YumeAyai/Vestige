package tracking

import "encoding/base64"

var PixelGIF []byte

func init() {
	PixelGIF, _ = base64.StdEncoding.DecodeString("R0lGODlhAQABAPAAAP///wAAACH5BAAAAAAALAAAAAABAAEAAAICRAEAOw==")
}
