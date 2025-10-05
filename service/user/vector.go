package user

import (
	"gitee.com/taoJie_1/chat/dao"
	"gitee.com/taoJie_1/chat/global"
)

type IVectorService interface {
	Search(query string) (string, float32, error)
}

type VectorService struct{}

func NewVectorService() *VectorService {
	return &VectorService{}
}

// Search 在向量数据库中搜索相似的文档
// 返回: (answer string, similarity float32, err error)
// answer: 匹配到的最相似的答案
// similarity: 相似度分数
// err: 错误信息
func (s *VectorService) Search(query string) (string, float32, error) {
	// 1. 将用户输入转换为向量
	// 这里的实现取决于您选择的 embedding 模型
	// queryVector, err := global.EmbeddingService.CreateEmbedding(query)
	// if err != nil {
	// 	return "", 0, err
	// }

	// 2. 在 ChromaDB/向量数据库 中搜索
	// searchResults, err := dao.App.RagDb.Search(queryVector)
	// if err != nil {
	// 	return "", 0, err
	// }

	// --- 以下是占位逻辑，因为您尚未确定如何对接 ---
	// 假设 dao.App.RagDb.Search 已经被调用，并返回了模拟结果
	answer, similarity, err := dao.App.RagDb.GetSimilarContent(query)
	if err != nil {
		return "", 0, err
	}
	// --- 占位逻辑结束 ---

	// 3. 检查相似度是否满足阈值
	if similarity < global.Config.Ai.VectorSimilarityThreshold {
		// 相似度太低，认为没有找到有效答案
		return "", similarity, nil
	}

	// 4. 如果满足阈值，返回最相似的答案和分数
	return answer, similarity, nil
}
