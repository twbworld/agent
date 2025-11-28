package oss

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"gitee.com/taoJie_1/mall-agent/model/config"
	"gitee.com/taoJie_1/mall-agent/utils"
	"github.com/aliyun/aliyun-oss-go-sdk/oss"
)

// Service 定义对象存储服务的接口
type Service interface {
	// UploadFile 上传 multipart 表单中的文件，并返回对象键。
	UploadFile(file *multipart.FileHeader) (string, error)
	// GetURL 为给定的对象键生成可公开访问的 URL。
	GetURL(objectKey string) string
	// Close 关闭底层客户端连接。
	Close() error
}

type aliyunOssService struct {
	client   *oss.Client
	config   config.Oss
	location *time.Location // 注入时区信息
}

// NewClient 创建一个新的 OSS 服务客户端。
// location 用于注入时区信息。
func NewClient(cfg config.Oss, location *time.Location) (Service, error) {
	// OSS SDK 的 Endpoint 不包含协议头，如果配置中包含了协议头，需要去除
	endpoint := cfg.Endpoint
	if strings.HasPrefix(endpoint, "http://") {
		endpoint = strings.TrimPrefix(endpoint, "http://")
	} else if strings.HasPrefix(endpoint, "https://") {
		endpoint = strings.TrimPrefix(endpoint, "https://")
	}

	client, err := oss.New(endpoint, cfg.AccessKeyId, cfg.AccessKeySecret)
	if err != nil {
		return nil, fmt.Errorf("创建阿里云OSS客户端失败: %w", err)
	}

	return &aliyunOssService{
		client:   client,
		config:   cfg,
		location: location,
	}, nil
}

func (s *aliyunOssService) UploadFile(file *multipart.FileHeader) (string, error) {
	// 注意：BucketName 已经更改为 Bucket
	bucket, err := s.client.Bucket(s.config.Bucket)
	if err != nil {
		return "", fmt.Errorf("获取OSS Bucket失败: %w", err)
	}

	// 打开文件
	src, err := file.Open()
	if err != nil {
		return "", fmt.Errorf("打开上传文件失败: %w", err)
	}
	defer src.Close()

	// 读取文件内容用于哈希
	fileBytes, err := io.ReadAll(src)
	if err != nil {
		return "", fmt.Errorf("读取文件内容失败: %w", err)
	}

	// 生成对象键
	fileHash := utils.HashBytes(fileBytes)
	fileExt := filepath.Ext(file.Filename)
	fileName := fmt.Sprintf("%s%s", fileHash, fileExt)
	// 使用注入的时区信息
	objectKey := fmt.Sprintf("%s%s/%s", s.config.StoragePath, time.Now().In(s.location).Format("images/20060102"), fileName)

	// 上传文件内容
	err = bucket.PutObject(objectKey, bytes.NewReader(fileBytes)) // 修正：使用 bytes.NewReader
	if err != nil {
		return "", fmt.Errorf("上传文件到OSS失败: %w", err)
	}

	return objectKey, nil
}

func (s *aliyunOssService) GetURL(objectKey string) string {
	if s.config.CdnDomain != "" {
		// 如果 CdnDomain 已经包含协议，直接使用。
		cdnURL, err := url.Parse(s.config.CdnDomain)
		if err == nil {
			// 确保路径拼接正确，避免双斜杠或丢失斜杠
			cdnURL.Path = strings.TrimSuffix(cdnURL.Path, "/") + "/" + strings.TrimPrefix(objectKey, "/")
			return cdnURL.String()
		}
		// 如果解析失败，回退到简单拼接
		return fmt.Sprintf("%s/%s", strings.TrimSuffix(s.config.CdnDomain, "/"), strings.TrimPrefix(objectKey, "/"))
	}
	// 回退到原始OSS URL
	// 假设 s.config.Endpoint 是不带协议的域名
	return fmt.Sprintf("https://%s.%s/%s", s.config.Bucket, s.config.Endpoint, objectKey)
}

func (s *aliyunOssService) Close() error {
	// aliyun-oss-go-sdk v2/v3 客户端不需要显式关闭连接。
	return nil
}
