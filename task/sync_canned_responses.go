package task

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"gitee.com/taoJie_1/chat/dao"
	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/internal/chatwoot"
	"gitee.com/taoJie_1/chat/internal/vector"
	"gitee.com/taoJie_1/chat/model/common"
	"gitee.com/taoJie_1/chat/model/enum"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"golang.org/x/sync/errgroup"
)

// 同步Chatwoot关键字到数据库和向量数据库
func (m *Manager) KeywordReloader() error {
	ctx := context.Background()
	global.Log.Println("开始同步Chatwoot关键词...")

	if global.ChatwootService == nil {
		return fmt.Errorf("Chatwoot客户端未初始化")
	}

	responses, err := global.ChatwootService.GetCannedResponses()
	if err != nil {
		return fmt.Errorf("从Chatwoot获取预设回复失败: %w", err)
	}

	var (
		exactMatchRules        []chatwoot.CannedResponse
		semanticRulesToProcess []chatwoot.CannedResponse
	)

	semanticPrefix := global.Config.Ai.SemanticPrefix

	// 1. 根据规则进行分类
	for _, resp := range responses {
		if resp.Content == "" || resp.ShortCode == "" {
			continue
		}

		if strings.HasPrefix(resp.ShortCode, semanticPrefix) {
			semanticRulesToProcess = append(semanticRulesToProcess, resp)
		} else {
			resp.ShortCode = strings.ToLower(resp.ShortCode)
			exactMatchRules = append(exactMatchRules, resp)
		}
	}

	global.Log.Infof("分类完成: %d 条精确匹配规则, %d 条语义匹配规则待处理", len(exactMatchRules), len(semanticRulesToProcess))

	// --- 语义规则处理 ---
	var documentsForVectorDB []vector.Document
	var allSemanticIDs []string

	if len(semanticRulesToProcess) > 0 {
		// 2. 并发处理所有语义匹配规则（仅LLM生成问题）
		if global.LlmService == nil {
			return fmt.Errorf("LLM客户端未初始化")
		}
		type semanticJob struct {
			resp             chatwoot.CannedResponse
			standardQuestion string
		}

		var (
			llmGroup      errgroup.Group
			mu            sync.Mutex
			completedJobs []semanticJob
		)
		llmGroup.SetLimit(5)

		for _, resp := range semanticRulesToProcess {
			resp := resp // 避免闭包陷阱
			llmGroup.Go(func() error {
				var llmInputText string
				var promptToUse enum.SystemPrompt

				seedQuestion := strings.TrimSpace(strings.TrimPrefix(resp.ShortCode, semanticPrefix))
				if seedQuestion != "" {
					llmInputText = seedQuestion
					promptToUse = enum.SystemPromptGenQuestionFromKeyword
				} else {
					llmInputText = resp.Content
					promptToUse = enum.SystemPromptGenQuestionFromContent
				}

				standardQuestion, err := global.LlmService.GenerateStandardQuestion(ctx, promptToUse, llmInputText)
				if err != nil {
					global.Log.Warnf("为ID %d 的内容生成标准问题失败: %v", resp.Id, err)
					return nil
				}
				standardQuestion = strings.Trim(standardQuestion, `\"'。， `)
				if standardQuestion == "" {
					global.Log.Warnf("LLM为ID %d 的内容生成了空问题", resp.Id)
					return nil
				}

				if gin.Mode() == gin.DebugMode {
					fmt.Printf("==========LLM生成标准问题, Seed: '%s', StandardQuestion: '%s'", llmInputText, standardQuestion)
				}

				mu.Lock()
				completedJobs = append(completedJobs, semanticJob{resp: resp, standardQuestion: standardQuestion})
				mu.Unlock()
				return nil
			})
		}

		if err := llmGroup.Wait(); err != nil {
			global.Log.Errorf("处理LLM生成标准问题任务时发生意外错误: %v", err)
		}

		// 3. 批量为所有生成的标准问题创建向量
		if len(completedJobs) > 0 {
			keywordToEmbed := make([]string, 0, len(completedJobs))
			for _, job := range completedJobs {
				// keywordToEmbed = append(keywordToEmbed, fmt.Sprintf("问题: %s; 答案: %s", job.standardQuestion, job.resp.Content))
				keywordToEmbed = append(keywordToEmbed, job.standardQuestion)
			}

			embedCtx, cancel := context.WithTimeout(ctx, time.Duration(global.Config.LlmEmbedding.BatchTimeout)*time.Second)
			defer cancel()

			embeddings, err := m.embeddingService.CreateEmbeddings(embedCtx, keywordToEmbed)
			if err != nil {
				global.Log.Errorf("批量创建向量失败: %v", err)
			} else if len(embeddings) == len(completedJobs) {
				// 4. 构建最终的向量文档
				for i, job := range completedJobs {
					docID := fmt.Sprintf("%s%d", dao.CannedResponseVectorIDPrefix, job.resp.Id)
					doc := vector.Document{
						ID: docID,
						Metadata: map[string]interface{}{
							dao.VectorMetadataKeyQuestion: job.standardQuestion,
							dao.VectorMetadataKeyAnswer:   job.resp.Content,
							dao.VectorMetadataKeySourceID: int64(job.resp.Id),
						},
						Embedding: embeddings[i],
					}
					documentsForVectorDB = append(documentsForVectorDB, doc)
					allSemanticIDs = append(allSemanticIDs, docID)
				}
			}
		}
	}

	// 5. 并发执行数据库同步
	var finalGroup errgroup.Group
	finalGroup.Go(func() error {
		return syncDb(exactMatchRules)
	})
	finalGroup.Go(func() error {
		if err := syncVectorDb(ctx, documentsForVectorDB, allSemanticIDs); err != nil {
			global.Log.Errorf("同步关键词到向量数据库时发生非阻塞错误: %v", err)
		}
		return nil
	})
	if err := finalGroup.Wait(); err != nil {
		return err
	}

	// 6. 重新加载关键词到内存
	return m.LoadKeywords()
}

// 数据库处理
func syncDb(matchRules []chatwoot.CannedResponse) error {
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
func syncVectorDb(ctx context.Context, documents []vector.Document, activeIDs []string) error {
	if len(documents) > 0 {
		upsertedCount, err := dao.App.VectorDb.BatchUpsert(ctx, documents)
		if err != nil {
			global.Log.Errorln("[gsgf4g]同步关键词到向量数据库失败:", err)
			return fmt.Errorf("同步关键词到向量数据库失败: %w", err)
		}
		global.Log.Printf("成功同步 %d 条语义匹配规则到向量数据库", upsertedCount)
	} else {
		global.Log.Println("没有需要同步到向量数据库的语义匹配规则")
	}

	// 清理向量数据库中不再存在的条目
	prunedCount, err := dao.App.VectorDb.PruneStale(ctx, activeIDs)
	if err != nil {
		global.Log.Warnf("[gjsf8g]清理向量数据库中过期条目失败: %v", err)
	}
	if prunedCount > 0 {
		global.Log.Printf("成功从向量数据库中清理 %d 条过期条目", prunedCount)
	}

	return nil
}

// 从sqlite加载关键词到内存
func (m *Manager) LoadKeywords() error {
	var (
		err          error
		keywordslist []common.KeywordsList = make([]common.KeywordsList, 0)
	)

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

	global.Log.Printf("成功加载 %d 条关键词到内存", len(tempMap))

	return nil
}
