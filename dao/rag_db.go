package dao

import (
	"context"
	"fmt"

	"gitee.com/taoJie_1/chat/model/db"
)

const CannedResponseVectorIDPrefix = "cw_canned_"

type RagDocument struct {
	ID       string                 // 文档的唯一ID，我们可以用 "kw-" + source_id
	Content  string                 // 文档内容，这里是关键词本身
	Rag      []float32              // 文本向量
	Metadata map[string]interface{} // 元数据，用于存储额外信息，如回答内容
}

type RagDb struct {
	CollectionName string
}

// UpsertKeywords 插入或更新关键词到向量数据库
// 这是同步的核心方法
func (d *RagDb) UpsertKeywords(ctx context.Context, docs []db.KeywordRule) (int, error) {
	fmt.Println(docs[0].Embedding)
	if len(docs) == 0 {
		return 0, nil
	}

	// --- 这是调用向量数据库SDK的核心逻辑 ---
	// 伪代码，您需要替换为实际的向量数据库客户端调用
	// 大部分向量数据库的SDK都直接提供了Upsert或类似的方法

	// 1. 准备要Upsert的数据
	// ragDocs := make([]RagDocument, len(docs))
	// for i, doc := range docs {
	//     ragDocs[i] = RagDocument{
	//         ID:     fmt.Sprintf("cw_canned_%d", doc.SourceID), // 构造一个带前缀的唯一ID
	//         Content:  doc.ShortCode,
	//         Rag:   doc.Rag,
	//         Metadata: map[string]interface{}{
	//             "answer":    doc.Content,
	//             "source_id": doc.SourceID,
	//         },
	//     }
	// }
	//
	// err := collection.Upsert(ragDocs);
	// if err != nil {
	//     return 0, err
	// }

	fmt.Printf("INFO: Upserting %d documents into rag collection '%s'\n", len(docs), d.CollectionName)
	for _, doc := range docs {
		// 使用Chatwoot的ID作为唯一标识
		uniqueID := fmt.Sprintf("cw_canned_%d", doc.Id)
		fmt.Printf("  -> Upserting rag for doc ID: %s, keyword: '%s'\n", uniqueID, doc.CannedResponse.ShortCode)
	}

	return len(docs), nil
}

// CleanUpStaleEntries (重要补充) 清理不再存在的条目
// 在Upsert后，还需要一个步骤来删除那些在Chatwoot中已被删除的条目
func (d *RagDb) CleanUpStaleEntries(ctx context.Context, activeIDs []string) (int, error) {
	// 1. 从向量数据库中获取所有文档的ID (通常SDK会提供只获取ID的方法)
	// allIDsInDB, _ := collection.GetAllIDs()
	allIDsInDB := []string{"cw_canned_1", "cw_canned_2", "cw_canned_999"} // 模拟
	fmt.Printf("INFO: Found %d documents in rag DB.\n", len(allIDsInDB))

	// 2. 找出需要删除的ID
	activeIDSet := make(map[string]struct{}, len(activeIDs))
	for _, id := range activeIDs {
		activeIDSet[id] = struct{}{}
	}

	var idsToDelete []string
	for _, idInDB := range allIDsInDB {
		if _, ok := activeIDSet[idInDB]; !ok {
			idsToDelete = append(idsToDelete, idInDB)
		}
	}

	if len(idsToDelete) == 0 {
		return 0, nil
	}

	// 3. 批量删除
	// err := collection.Delete(idsToDelete)
	// if err != nil {
	//     return 0, err
	// }
	fmt.Printf("INFO: Deleting %d stale documents from rag DB: %v\n", len(idsToDelete), idsToDelete)

	return len(idsToDelete), nil
}

func (d *RagDb) GetSimilarContent(query string) (string, float32, error) {
	return "", float32(1), nil
}
