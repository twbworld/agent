package dao

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/internal/vector"
	"gitee.com/taoJie_1/chat/model/common"
	chroma "github.com/amikos-tech/chroma-go/pkg/api/v2"
	"github.com/amikos-tech/chroma-go/pkg/embeddings"
)

// CannedResponseVectorIDPrefix 是向量数据库中快捷回复文档ID的前缀
// 用于区分不同来源的文档，便于管理和识别
const CannedResponseVectorIDPrefix = "cw_canned_"

// 向量数据库中元数据的键名
const (
	VectorMetadataKeyKeyword  = "keyword"
	VectorMetadataKeyAnswer   = "answer"
	VectorMetadataKeySourceID = "source_id"
)

// SearchResult represents a single item returned from a vector search.
type SearchResult struct {
	Answer     string
	Similarity float32
	SourceID   int64
}

type VectorDb struct {
	CollectionName string
}

// BatchUpsert 将关键词规则批量插入或更新到向量数据库
func (d *VectorDb) BatchUpsert(ctx context.Context, docs []common.KeywordRule) (int, error) {
	if global.VectorDb == nil {
		return 0, fmt.Errorf("向量数据库客户端未初始化")
	}
	if len(docs) == 0 {
		return 0, nil
	}

	// 将数据库模型 (common.KeywordRule) 转换为通用的向量文档模型 (vector.Document)
	documents := make([]vector.Document, len(docs))
	for i, doc := range docs {
		documents[i] = vector.Document{
			ID: fmt.Sprintf("%s%d", CannedResponseVectorIDPrefix, doc.Id),
			Metadata: map[string]interface{}{
				VectorMetadataKeyKeyword:  doc.CannedResponse.ShortCode,
				VectorMetadataKeyAnswer:   doc.CannedResponse.Content,
				VectorMetadataKeySourceID: int64(doc.Id),
			},
			Embedding: doc.Embedding,
		}
	}

	err := global.VectorDb.Upsert(ctx, d.CollectionName, documents)
	if err != nil {
		return 0, fmt.Errorf("批量更新/插入文档到向量数据库失败: %w", err)
	}

	return len(docs), nil
}

// 清理已被删除的旧关键字
func (d *VectorDb) PruneStale(ctx context.Context, activeIDs []string) (int, error) {
	if global.VectorDb == nil {
		return 0, fmt.Errorf("向量数据库客户端未初始化")
	}

	col, err := global.VectorDb.GetOrCreateCollection(ctx, d.CollectionName)
	if err != nil {
		return 0, fmt.Errorf("获取向量集合 '%s' 失败: %w", d.CollectionName, err)
	}

	results, err := col.Get(ctx, chroma.WithIncludeGet(chroma.IncludeURIs))
	if err != nil {
		return 0, fmt.Errorf("从向量数据库获取所有文档ID失败: %w", err)
	}

	existingIDs := results.GetIDs()
	if len(existingIDs) == 0 {
		return 0, nil
	}

	activeIDSet := make(map[string]struct{}, len(activeIDs))
	for _, id := range activeIDs {
		activeIDSet[id] = struct{}{}
	}

	var staleIDs []chroma.DocumentID
	for _, id := range existingIDs {
		idStr := string(id)
		if !strings.HasPrefix(idStr, CannedResponseVectorIDPrefix) {
			continue
		}
		if _, ok := activeIDSet[idStr]; !ok {
			staleIDs = append(staleIDs, id)
		}
	}

	if len(staleIDs) == 0 {
		return 0, nil
	}

	err = col.Delete(ctx, chroma.WithIDsDelete(staleIDs...))
	if err != nil {
		return 0, fmt.Errorf("从向量数据库删除过期条目失败: %w", err)
	}

	return len(staleIDs), nil
}

// Search 根据查询文本从向量数据库中获取最相似的内容
func (d *VectorDb) Search(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	if global.VectorDb == nil {
		return nil, fmt.Errorf("向量数据库客户端未初始化")
	}
	if global.EmbeddingService == nil {
		return nil, fmt.Errorf("向量化服务未初始化")
	}

	// 1. 为查询文本创建向量
	queryEmbeddings, err := global.EmbeddingService.CreateEmbeddings(ctx, []string{query})
	if err != nil {
		return nil, fmt.Errorf("为查询文本创建向量失败: %w", err)
	}
	if len(queryEmbeddings) == 0 {
		return nil, fmt.Errorf("未能为查询文本生成向量")
	}
	queryEmbedding := embeddings.NewEmbeddingFromFloat32(queryEmbeddings[0])

	// 2. 获取集合
	col, err := global.VectorDb.GetOrCreateCollection(ctx, d.CollectionName)
	if err != nil {
		return nil, fmt.Errorf("获取向量集合 '%s' 失败: %w", d.CollectionName, err)
	}

	if topK == 0 {
		topK = 1
	}

	// 3. 执行向量查询
	qr, err := col.Query(
		ctx,
		chroma.WithQueryEmbeddings(queryEmbedding),
		chroma.WithNResults(topK),
		chroma.WithIncludeQuery(chroma.IncludeMetadatas, chroma.IncludeDistances),
	)
	if err != nil {
		return nil, fmt.Errorf("在向量数据库中查询失败: %w", err)
	}

	// 4. 解析并返回结果
	if qr.CountGroups() == 0 {
		return nil, sql.ErrNoRows
	}

	distancesGroups := qr.GetDistancesGroups()
	metadatasGroups := qr.GetMetadatasGroups()

	// 查询结果按查询向量分组，每组内的结果数量由 WithNResults 决定
	if len(distancesGroups) < 1 || len(metadatasGroups) < 1 {
		return nil, sql.ErrNoRows
	}

	distances := distancesGroups[0]
	metadatas := metadatasGroups[0]

	if len(distances) == 0 || len(metadatas) == 0 {
		return nil, sql.ErrNoRows
	}

	var results []SearchResult
	for i := 0; i < len(distances); i++ {
		distance := distances[i]
		metadata := metadatas[i]

		answer, ok := metadata.GetString(VectorMetadataKeyAnswer)
		if !ok {
			global.Log.Warnf("无法从元数据中解析回答: %v", metadata)
			continue
		}

		sourceID, ok := metadata.GetFloat(VectorMetadataKeySourceID)
		if !ok {
			global.Log.Warnf("无法从元数据中解析 source_id: %v", metadata)
			continue
		}

		// Chroma返回的是距离（如L2距离），值越小越相似。
		// 将其转换为一个0到1之间的相似度分数，值越大越相似。
		similarity := float32(1.0 / (1.0 + distance))

		results = append(results, SearchResult{
			Answer:     answer,
			Similarity: similarity,
			SourceID:   int64(sourceID),
		})
	}

	if len(results) == 0 {
		return nil, sql.ErrNoRows
	}

	return results, nil
}
