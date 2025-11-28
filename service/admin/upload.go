package admin

import (
	"errors"
	"fmt"
	"gitee.com/taoJie_1/mall-agent/global"
	"mime/multipart"
	"net/http"
	"strings"
)

const (
	// MaxUploadSize 最大上传文件大小，5MB
	MaxUploadSize = 5 * 1024 * 1024
)

var allowedImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
}

// UploadService 定义文件上传操作的接口
type UploadService interface {
	UploadImage(file *multipart.FileHeader) (string, error)
}

type uploadService struct{}

// NewUploadService 创建一个新的 UploadService 实例
func NewUploadService() UploadService {
	return &uploadService{}
}

// UploadImage 处理图片上传逻辑
func (s *uploadService) UploadImage(file *multipart.FileHeader) (string, error) {
	if global.OssService == nil {
		return "", errors.New("OSS 服务未配置或初始化失败")
	}

	// 1. 验证文件大小
	if file.Size > MaxUploadSize {
		return "", fmt.Errorf("文件大小超过限制 (%.2f MB)", float64(MaxUploadSize)/1024/1024)
	}

	// 2. 验证文件类型
	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("无法打开文件: %w", err)
	}
	defer src.Close()

	buffer := make([]byte, 512)
	_, err = src.Read(buffer)
	if err != nil {
		return "", fmt.Errorf("无法读取文件头: %w", err)
	}
	// http.DetectContentType 会返回 "mime/type; charset=..." 的格式
	fileType := strings.Split(http.DetectContentType(buffer), ";")[0]

	if !allowedImageTypes[fileType] {
		return "", fmt.Errorf("不支持的文件类型: %s", fileType)
	}

	// 重置文件读取器，以便后续上传操作能从文件开头读取
	if _, err := src.Seek(0, 0); err != nil {
		return "", fmt.Errorf("无法重置文件读取器: %w", err)
	}

	// 3. 上传文件
	objectKey, err := global.OssService.UploadFile(file)
	if err != nil {
		return "", fmt.Errorf("上传文件失败: %w", err)
	}

	// 4. 返回完整 URL
	url := global.OssService.GetURL(objectKey)

	return url, nil
}
