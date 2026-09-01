package application

import (
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/KDZZZZZZ/short-term/platform/errs"
)

// 图片限制，与 openapi/components/schemas.yaml#/ImageFile 一致。
const (
	// MaxImageBytes 是单文件限制：5 MiB。
	MaxImageBytes = 5 << 20
)

// allowedImageTypes 是契约允许的内容类型，并映射到对象键使用的扩展名。
var allowedImageTypes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// ImageUpload 是一个上传文件。
type ImageUpload struct {
	// Data 是完整文件。图片上限为 5 MiB，因此保存在内存中的大小有界；
	// 如果上限更大，则需要改用流式处理。
	Data []byte
	// ContentType 是客户端声明的类型，只作为提示使用；
	// 存储类型来自对字节内容的探测。
	ContentType string
}

// ErrImageTypeNotAllowed 表示文件真实内容不是允许的图片类型。
var ErrImageTypeNotAllowed = errors.New("image type is not allowed")

// detectImageType 返回上传文件的真实内容类型，以及对象键使用的扩展名。
//
// 不信任客户端的 Content-Type：攻击者可以控制它，而从媒体源提供 HTML 或 SVG
// 载荷会形成存储型跨站脚本攻击向量。因此改用 http.DetectContentType 检查开头字节。
func detectImageType(upload ImageUpload) (contentType, extension string, err error) {
	if len(upload.Data) == 0 {
		return "", "", errs.New(errs.CodeValidation, "图片内容为空")
	}
	if len(upload.Data) > MaxImageBytes {
		return "", "", errs.Newf(errs.CodePayloadTooLarge, "单张图片不能超过 %d MiB", MaxImageBytes>>20)
	}

	detected := http.DetectContentType(upload.Data)
	// DetectContentType 可能追加参数，例如 "; charset=utf-8"。
	detected, _, _ = strings.Cut(detected, ";")
	detected = strings.TrimSpace(detected)

	extension, allowed := allowedImageTypes[detected]
	if !allowed {
		return "", "", errs.Wrap(errs.CodeValidation, "图片必须是 JPEG、PNG 或 WebP", ErrImageTypeNotAllowed)
	}
	return detected, extension, nil
}

// objectKey 构造一张商品图片的存储键。键只由服务端生成的标识推导，
// 因此客户端无法影响路径，也无法逃出商品前缀。
func objectKey(productID, imageID, extension string) string {
	return path.Join("products", productID, imageID+extension)
}
