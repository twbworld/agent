package task

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitee.com/taoJie_1/chat/dao"
	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/model/common"
	"gitee.com/taoJie_1/chat/model/db"
	"gitee.com/taoJie_1/chat/pkg/chatwoot"
	"github.com/jmoiron/sqlx"
	"golang.org/x/sync/errgroup"
)

// 同步Chatwoot关键字到数据库和向量数据库
func (m *Manager) KeywordReloader() error {
	global.Log.Println("开始同步Chatwoot关键词...")
	var err error

	// 从Chatwoot API获取数据
	responses, err := global.ChatwootService.GetCannedResponses()
	if err != nil {
		return fmt.Errorf("从Chatwoot获取预设回复失败: %w", err)
	}

	// responses := []chatwoot.CannedResponse{
	// 	{ AccountId: 1, ShortCode: "test", Content:   "这是一个测试回复1", },
	// 	{ AccountId: 1, ShortCode: "hello", Content:   "你好，这是一个问候回复1",},
	// }

	var (
		exactMatchRules        []chatwoot.CannedResponse //精确匹配
		semanticRulesToProcess []db.KeywordRule          //语义匹配, 用于向量数据库
		keywordsToEmbed        []string                  //存储向量化的文本
		allSemanticIDs         []string
	)

	for _, resp := range responses {
		if resp.ShortCode == "" || resp.Content == "" {
			continue
		}

		// 匹配前缀
		if strings.HasPrefix(resp.ShortCode, global.Config.Ai.SemanticPrefix) {
			//加入"精确匹配"和"向量化"
			keyword := strings.TrimSpace(strings.TrimPrefix(resp.ShortCode, global.Config.Ai.SemanticPrefix))
			if keyword == "" {
				continue
			}
			resp.ShortCode = strings.ToLower(keyword) //更新为不带前缀的小写关键词

			// 需加入精确匹配列表
			exactMatchRules = append(exactMatchRules, resp)

			//为文本向量化做准备
			semanticRulesToProcess = append(semanticRulesToProcess, db.KeywordRule{
				CannedResponse: resp,
			})
			keywordsToEmbed = append(keywordsToEmbed, keyword)

			//为存储向量数据准备
			allSemanticIDs = append(allSemanticIDs, fmt.Sprintf("%s%d", dao.CannedResponseVectorIDPrefix, resp.Id))

		} else if global.Config.Ai.ExactPrefix == "" || strings.HasPrefix(resp.ShortCode, global.Config.Ai.SemanticPrefix) {
			// 加入"精确匹配"
			keyword := strings.TrimSpace(strings.TrimPrefix(resp.ShortCode, global.Config.Ai.SemanticPrefix))
			if keyword == "" {
				continue
			}
			resp.ShortCode = strings.ToLower(keyword)
			exactMatchRules = append(exactMatchRules, resp)
		}
	}

	// 获取向量化结果
	var vectors [][]float32
	if len(keywordsToEmbed) > 0 {
		vectors, err = m.embeddingService.CreateEmbeddings(context.Background(), keywordsToEmbed)
		if err != nil {
			return fmt.Errorf("批量创建向量失败: %w", err)
		}
	}
	var semanticMatchRules []db.KeywordRule
	if len(vectors) == len(semanticRulesToProcess) {
		for i, rule := range semanticRulesToProcess {
			rule.Embedding = vectors[i]
			semanticMatchRules = append(semanticMatchRules, rule)
		}
	}

	var g errgroup.Group
	g.Go(func() error {
		return handleDb(exactMatchRules)
	})
	g.Go(func() error {
		return handleRag(semanticMatchRules, allSemanticIDs)
	})
	if err := g.Wait(); err != nil {
		return err
	}

	return m.LoadKeywords()
}

// 数据库处理
func handleDb(matchRules []chatwoot.CannedResponse) error {
	var count int64
	err := dao.Tx(func(tx *sqlx.Tx) (e error) {
		// 清空旧数据
		e = dao.App.KeywordsDb.CleanTable(tx)
		if e != nil {
			return e
		}

		// 插入新数据
		count, e = dao.App.KeywordsDb.BatchInsert(matchRules, tx)
		if e != nil {
			return e
		}
		return
	})
	if err != nil {
		global.Log.Errorln("[isfsifi]同步关键词到SQLite失败:", err)
		return fmt.Errorf("同步关键词到SQLite失败: %w", err)
	}

	global.Log.Printf("成功从Chatwoot同步 %d 条关键词到SQLite", count)
	return nil
}

// 向量数据库处理
func handleRag(Rules []db.KeywordRule, activeIDs []string) error {
	upsertedCount, err := dao.App.RagDb.UpsertKeywords(context.Background(), Rules)
	if err != nil {
		global.Log.Errorln("[gsgf4g]同步关键词到向量数据库失败:", err)
		return fmt.Errorf("同步关键词到向量数据库失败: %w", err)
	}
	global.Log.Printf("成功同步 %d 条语义匹配关键词到向量数据库", upsertedCount)

	// 清理已被删除的旧关键字
	_, err = dao.App.RagDb.CleanUpStaleEntries(context.Background(), activeIDs)
	if err != nil {
		global.Log.Warnf("[gjsf8g]清理向量数据库中过期条目失败: %v", err)
	}

	return nil
}

// 从sqlite加载关键词到内存
func (m *Manager) LoadKeywords() error {
	var (
		err          error
		keywordslist []common.KeywordsList = make([]common.KeywordsList, 0)
	)
	startTime := time.Now()

	if err = dao.App.KeywordsDb.GetKeywordsAllList(&keywordslist); err != nil {
		return fmt.Errorf("加载Keywords失败: %w", err)
	}
	if len(keywordslist) < 1 {
		return nil
	}

	tempMap := make(map[string]string)

	if len(keywordslist) > 0 {
		for _, v := range keywordslist {
			tempMap[strings.ToLower(v.ShortCode)] = v.Content
		}
	}

	global.CannedResponses.Lock()
	global.CannedResponses.Data = tempMap
	global.CannedResponses.Unlock()

	duration := time.Since(startTime)
	global.Log.Printf("成功加载 %d 条关键词, 耗时: %v", len(tempMap), duration)

	return nil
}
