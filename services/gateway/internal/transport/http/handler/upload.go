package handler

import (
	"errors"
	"io"
	"mime/multipart"
)

// Upload limits, matching openapi/components/schemas.yaml#/ImageFile and the
// createProduct description.
const (
	// MaxImageBytes is the per-file limit: 5 MiB.
	MaxImageBytes = 5 << 20
	// MaxUploadBytes is the whole multipart request limit: 16 MiB.
	MaxUploadBytes = 16 << 20
	// maxImages is the number of files one request may carry.
	maxImages = 3
	// multipartMemory bounds what the parser keeps in memory before spilling
	// to a temporary file on disk.
	multipartMemory = 8 << 20
)

// errUploadTooLarge reports a file that exceeded the per-file limit.
var errUploadTooLarge = errors.New("upload exceeds the per-file limit")

// readUpload loads one uploaded file, refusing to read past the per-file limit
// even when the header reported a smaller size.
func readUpload(header *multipart.FileHeader) ([]byte, error) {
	file, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	// One byte over the limit is read on purpose: it distinguishes a file at
	// exactly the limit from one that exceeds it.
	data, err := io.ReadAll(io.LimitReader(file, MaxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxImageBytes {
		return nil, errUploadTooLarge
	}
	return data, nil
}
