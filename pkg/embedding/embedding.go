package embedding

import "context"

type Service interface {
	CreateEmbeddings(ctx context.Context, texts []string) ([][]float32, error)
}
