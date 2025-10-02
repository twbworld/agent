package embedding

import "context"

// 文本向量化服务的通用接口
type Service interface {
	CreateEmbeddings(ctx context.Context, texts []string) ([][]float32, error)
}
