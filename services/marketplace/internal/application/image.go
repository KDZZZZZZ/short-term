package application

import (
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/KDZZZZZZ/short-term/platform/errs"
)

// Image limits, matching openapi/components/schemas.yaml#/ImageFile.
const (
	// MaxImageBytes is the per-file limit: 5 MiB.
	MaxImageBytes = 5 << 20
)

// allowedImageTypes are the content types the contract permits, mapped to the
// extension used in the object key.
var allowedImageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// ImageUpload is one uploaded file.
type ImageUpload struct {
	// Data is the whole file. Images are capped at 5 MiB, so holding one in
	// memory is bounded; a larger limit would need streaming instead.
	Data []byte
	// ContentType is what the client declared. It is used only as a hint: the
	// stored type comes from sniffing the bytes.
	ContentType string
}

// ErrImageTypeNotAllowed reports a file whose real content is not an allowed
// image type.
var ErrImageTypeNotAllowed = errors.New("image type is not allowed")

// detectImageType returns the real content type of an upload and the extension
// to use for its object key.
//
// The client's Content-Type is not trusted: an attacker controls it, and an
// HTML or SVG payload served from the media origin would be a stored
// cross-site scripting vector. http.DetectContentType inspects the leading
// bytes instead.
func detectImageType(upload ImageUpload) (contentType, extension string, err error) {
	if len(upload.Data) == 0 {
		return "", "", errs.New(errs.CodeValidation, "图片内容为空")
	}
	if len(upload.Data) > MaxImageBytes {
		return "", "", errs.Newf(errs.CodePayloadTooLarge, "单张图片不能超过 %d MiB", MaxImageBytes>>20)
	}

	detected := http.DetectContentType(upload.Data)
	// DetectContentType may append parameters, for example "; charset=utf-8".
	detected, _, _ = strings.Cut(detected, ";")
	detected = strings.TrimSpace(detected)

	extension, allowed := allowedImageTypes[detected]
	if !allowed {
		return "", "", errs.Wrap(errs.CodeValidation, "图片必须是 JPEG、PNG 或 WebP", ErrImageTypeNotAllowed)
	}
	return detected, extension, nil
}

// objectKey builds the storage key for one product image. Keys are derived
// from server-generated identifiers only, so a client cannot influence the
// path and cannot escape the product's prefix.
func objectKey(productID, imageID, extension string) string {
	return path.Join("products", productID, imageID+extension)
}
