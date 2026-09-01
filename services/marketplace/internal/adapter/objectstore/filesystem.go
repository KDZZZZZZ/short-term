// Package objectstore 实现 Marketplace ObjectStore 端口。
//
// docs/software-design.md 第 9.2 节规定商品图片只能通过此适配器访问对象存储。
// 这里的文件系统实现用于本地开发和集成测试；Alibaba Cloud OSS 实现满足同一接口，
// 不会改变业务代码。
package objectstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Filesystem 将对象存储在基础目录下，并通过公开基础 URL 提供访问。
type Filesystem struct {
	root      string
	publicURL string
}

// ErrUnsafeKey 表示会逃出存储根目录的对象键。
var ErrUnsafeKey = errors.New("objectstore: unsafe object key")

// NewFilesystem 按需创建存储根目录并返回存储器。
// publicURL 是外部世界获取对象时使用的前缀。
func NewFilesystem(root, publicURL string) (*Filesystem, error) {
	if root == "" {
		return nil, errors.New("objectstore: storage root is required")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("objectstore: resolve root: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o750); err != nil {
		return nil, fmt.Errorf("objectstore: create root: %w", err)
	}
	return &Filesystem{root: absolute, publicURL: strings.TrimSuffix(publicURL, "/")}, nil
}

// Put 写入对象，并替换同一键下已有的对象。
//
// 写入先落到临时文件，再执行重命名，因此读取方不会看到只写了一半的图片，
// 写入失败也会保留原有对象。
func (f *Filesystem) Put(_ context.Context, key, _ string, data []byte) error {
	target, err := f.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return fmt.Errorf("objectstore: create directory: %w", err)
	}

	temp, err := os.CreateTemp(filepath.Dir(target), ".upload-*")
	if err != nil {
		return fmt.Errorf("objectstore: create temporary file: %w", err)
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()

	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return fmt.Errorf("objectstore: write object: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("objectstore: close object: %w", err)
	}
	if err := os.Chmod(tempName, 0o640); err != nil {
		return fmt.Errorf("objectstore: set object permissions: %w", err)
	}
	if err := os.Rename(tempName, target); err != nil {
		return fmt.Errorf("objectstore: publish object: %w", err)
	}
	return nil
}

// Delete 删除对象。对象不存在不算错误，因此清理和重试具备幂等性。
func (f *Filesystem) Delete(_ context.Context, key string) error {
	target, err := f.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("objectstore: delete object: %w", err)
	}
	return nil
}

// URL 返回对象的公开位置。
func (f *Filesystem) URL(key string) string {
	if key == "" {
		return ""
	}
	return f.publicURL + "/" + key
}

// Root 返回存储目录，以便部署挂载并提供其中的文件。
func (f *Filesystem) Root() string { return f.root }

// resolve 将对象键转换为根目录内的路径，并拒绝任何会逃出根目录的值。
// 当前对象键由服务端生成；即使将来改变来源，这项检查也能避免静默变成路径穿越。
func (f *Filesystem) resolve(key string) (string, error) {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") {
		return "", ErrUnsafeKey
	}
	target := filepath.Join(f.root, filepath.FromSlash(key))
	if target != f.root && !strings.HasPrefix(target, f.root+string(filepath.Separator)) {
		return "", ErrUnsafeKey
	}
	return target, nil
}
