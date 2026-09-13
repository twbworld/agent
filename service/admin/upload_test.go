package admin

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/twbworld/agent/global"
)

// mockOssService 用于模拟 OSS 接口操作
type mockOssService struct {
	uploadFunc func(file *multipart.FileHeader) (string, error)
	getUrlFunc func(objectKey string) string
}

func (m *mockOssService) UploadFile(file *multipart.FileHeader) (string, error) {
	if m.uploadFunc != nil {
		return m.uploadFunc(file)
	}
	return "mock-key", nil
}

func (m *mockOssService) GetURL(objectKey string) string {
	if m.getUrlFunc != nil {
		return m.getUrlFunc(objectKey)
	}
	return "https://mock.cdn.com/" + objectKey
}

func (m *mockOssService) Close() error { return nil }

// createTestFileHeader 是一个用于在内存中构建合规 *multipart.FileHeader 的辅助函数
func createTestFileHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(content)
	_ = writer.Close()

	req := httptest.NewRequest("POST", "/", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if err := req.ParseMultipartForm(10 << 20); err != nil {
		t.Fatal(err)
	}

	// 从构造的请求中提取出真实的 FileHeader
	return req.MultipartForm.File["file"][0]
}

func TestUploadService_UploadImage(t *testing.T) {
	svc := NewUploadService()

	// 模拟合法的 PNG 文件头
	pngHeader := []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}
	// 模拟非法的文本文件头
	txtHeader := []byte("这是一段普通的文本内容，并非图片格式")

	t.Run("OSS未初始化拦截", func(t *testing.T) {
		global.OssService = nil
		fh := createTestFileHeader(t, "test.png", pngHeader)
		_, err := svc.UploadImage(fh)
		if err == nil || err.Error() != "OSS 服务未配置或初始化失败" {
			t.Errorf("期望错误: 'OSS 服务未配置或初始化失败'，实际得到: %v", err)
		}
	})

	t.Run("文件大小超限拦截", func(t *testing.T) {
		global.OssService = &mockOssService{}
		fh := createTestFileHeader(t, "test.png", pngHeader)
		fh.Size = MaxUploadSize + 1 // 强制把大小设为超限
		_, err := svc.UploadImage(fh)
		if err == nil || !strings.Contains(err.Error(), "文件大小超过限制") {
			t.Errorf("期望文件超大拦截错误，实际得到: %v", err)
		}
	})

	t.Run("不支持的文件类型拦截", func(t *testing.T) {
		global.OssService = &mockOssService{}
		fh := createTestFileHeader(t, "test.txt", txtHeader)
		_, err := svc.UploadImage(fh)
		if err == nil || !strings.Contains(err.Error(), "不支持的文件类型") {
			t.Errorf("期望不支持的文件类型错误，实际得到: %v", err)
		}
	})

	t.Run("上传流失败异常传递", func(t *testing.T) {
		global.OssService = &mockOssService{
			uploadFunc: func(file *multipart.FileHeader) (string, error) {
				return "", errors.New("mock upload err")
			},
		}
		fh := createTestFileHeader(t, "test.png", pngHeader)
		_, err := svc.UploadImage(fh)
		if err == nil || !strings.Contains(err.Error(), "上传文件失败: mock upload err") {
			t.Errorf("期望上传异常抛出，实际得到: %v", err)
		}
	})

	t.Run("成功上传并返回URL", func(t *testing.T) {
		global.OssService = &mockOssService{
			uploadFunc: func(file *multipart.FileHeader) (string, error) {
				return "images/20260911/test.png", nil
			},
			getUrlFunc: func(objectKey string) string {
				return "https://cdn.example.com/" + objectKey
			},
		}
		fh := createTestFileHeader(t, "test.png", pngHeader)
		url, err := svc.UploadImage(fh)
		if err != nil {
			t.Errorf("期望上传成功，不应得到错误: %v", err)
		}

		expectedURL := "https://cdn.example.com/images/20260911/test.png"
		if url != expectedURL {
			t.Errorf("期望 URL 匹配 %s，实际得到 %s", expectedURL, url)
		}
	})
}
