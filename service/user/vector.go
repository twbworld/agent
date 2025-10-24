package user

import (
	"context"
	"database/sql"

	"gitee.com/taoJie_1/mall-agent/dao"
	"gitee.com/taoJie_1/mall-agent/global"
)

type VectorService interface {
	// 在向量数据库中搜索与查询最相似的文档。
	Search(ctx context.Context, query string) ([]dao.SearchResult, error)
}

type vectorService struct{}

func NewVectorService() *vectorService {
	return &vectorService{}
}

func (s *vectorService) Search(ctx context.Context, query string) ([]dao.SearchResult, error) {
	results, err := dao.App.VectorDb.Search(ctx, query, int(global.Config.Ai.VectorSearchTopK))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	return results, nil
}
