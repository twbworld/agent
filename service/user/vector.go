package user

import (
	"context"
	"database/sql"

	"gitee.com/taoJie_1/chat/dao"
	"gitee.com/taoJie_1/chat/global"
)

type IVectorService interface {
	Search(ctx context.Context, query string) ([]dao.SearchResult, error)
}

type VectorService struct{}

func NewVectorService() *VectorService {
	return &VectorService{}
}

// Search 在向量数据库中搜索与查询最相似的文档。
func (s *VectorService) Search(ctx context.Context, query string) ([]dao.SearchResult, error) {
	results, err := dao.App.VectorDb.Search(ctx, query, global.Config.Ai.VectorSearchTopK)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return results, nil
}
