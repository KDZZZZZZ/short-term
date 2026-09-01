package handler

import (
	"errors"
	"io"
	"mime/multipart"
)

// 上传限制，与 openapi/components/schemas.yaml#/ImageFile 和 createProduct 描述一致。
const (
	// MaxImageBytes 是单文件限制：5 MiB。
	MaxImageBytes = 5 << 20
	// MaxUploadBytes 是整个 multipart 请求的限制：16 MiB。
	MaxUploadBytes = 16 << 20
	// maxImages 是单个请求可携带的文件数。
	maxImages = 3
	// multipartMemory 限制解析器在将内容溢出到磁盘临时文件前保留在内存中的大小。
	multipartMemory = 8 << 20
)

// errUploadTooLarge 表示文件超过单文件限制。
var errUploadTooLarge = errors.New("upload exceeds the per-file limit")

// readUpload 加载一个上传文件，即使请求头报告的大小更小，也拒绝读取超过单文件
// 限制的内容。
func readUpload(header *multipart.FileHeader) ([]byte, error) {
	file, err := header.Open()
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	// 有意多读取一个字节：这样可以区分恰好达到限制的文件和超过限制的文件。
	data, err := io.ReadAll(io.LimitReader(file, MaxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxImageBytes {
		return nil, errUploadTooLarge
	}
	return data, nil
}
