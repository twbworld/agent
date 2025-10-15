package task

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"gitee.com/taoJie_1/chat/dao"
	"gitee.com/taoJie_1/chat/global"
	"gitee.com/taoJie_1/chat/internal/chatwoot"
	"gitee.com/taoJie_1/chat/model/common"
	"gitee.com/taoJie_1/chat/model/enum"
	"github.com/jmoiron/sqlx"
	"golang.org/x/sync/errgroup"
)

// 同步Chatwoot关键字到数据库和向量数据库
func (m *Manager) KeywordReloader() error {
	ctx := context.Background()
	global.Log.Println("开始同步Chatwoot关键词...")
	var err error

	// 从Chatwoot API获取数据
	responses, err := global.ChatwootService.GetCannedResponses()
	if err != nil {
		return fmt.Errorf("从Chatwoot获取预设回复失败: %w", err)
	}

	var (
		exactMatchRules        []chatwoot.CannedResponse //精确匹配
		semanticRulesToProcess []common.KeywordRule      //待处理的语义匹配规则
		rulesForLLMGen         []chatwoot.CannedResponse //待由LLM生成ShortCode的规则
		allSemanticIDs         []string                  //所有将存入向量数据库的文档ID
	)

	semanticPrefix := global.Config.Ai.SemanticPrefix

	for _, resp := range responses {
		if resp.Content == "" {
			continue
		}

		// 根据ShortCode规则对响应进行分类
		if resp.ShortCode == semanticPrefix {
			// 规则3: ai@ (仅使用Content，由LLM生成ShortCode)
			rulesForLLMGen = append(rulesForLLMGen, resp)
		} else if strings.HasPrefix(resp.ShortCode, semanticPrefix) {
			// 规则2: ai@keyword (精确+向量)
			keyword := strings.TrimSpace(strings.TrimPrefix(resp.ShortCode, semanticPrefix))
			if keyword == "" {
				continue
			}
			resp.ShortCode = strings.ToLower(keyword)
			exactMatchRules = append(exactMatchRules, resp)
			semanticRulesToProcess = append(semanticRulesToProcess, common.KeywordRule{CannedResponse: resp})
		} else if resp.ShortCode != "" {
			// 规则1: keyword (仅精确)
			resp.ShortCode = strings.ToLower(resp.ShortCode)
			exactMatchRules = append(exactMatchRules, resp)
		}
	}

	// 并发调用LLM为`ai@`规则生成ShortCode
	if len(rulesForLLMGen) > 0 {
		var (
			llmGroup errgroup.Group
			mu       sync.Mutex
		)
		llmGroup.SetLimit(5)

		for _, resp := range rulesForLLMGen {
			resp := resp // 避免闭包陷阱
			llmGroup.Go(func() error {
				// 为Content生成一个最合适的“问题”作为ShortCode
				generatedShortCode, err := global.LlmService.GetCompletion(ctx, enum.ModelSmall, enum.SystemPromptForCannedResponse, resp.Content, 0.2)
				if err != nil {
					global.Log.Warnf("为ID %d 的内容生成ShortCode失败: %v", resp.Id, err)
					return nil // 即使单个失败，也不中断整个同步任务
				}

				// 清理LLM可能返回的多余字符
				generatedShortCode = strings.Trim(generatedShortCode, `"'。， `)
				if generatedShortCode == "" {
					return nil
				}

				mu.Lock()
				defer mu.Unlock()

				// 根据微调方案，将AI生成的规则同时用于精确匹配和语义匹配
				resp.ShortCode = strings.ToLower(generatedShortCode)
				exactMatchRules = append(exactMatchRules, resp)
				semanticRulesToProcess = append(semanticRulesToProcess, common.KeywordRule{CannedResponse: resp})
				return nil
			})
		}
		if err := llmGroup.Wait(); err != nil {
			return fmt.Errorf("处理LLM生成任务时发生错误: %w", err)
		}
	}

	// --- 数据准备阶段 ---
	var keywordsToEmbed []string
	for i := range semanticRulesToProcess {
		rule := &semanticRulesToProcess[i]
		// 采用仅关键词向量化
		// keywordsToEmbed = append(keywordsToEmbed, keyword)
		// 采用混合索引策略，将问题和回答合并以生成更丰富的语义向量
		combinedText := fmt.Sprintf("问题：%s\n回答：%s", rule.CannedResponse.ShortCode, rule.CannedResponse.Content)
		keywordsToEmbed = append(keywordsToEmbed, combinedText)
		allSemanticIDs = append(allSemanticIDs, fmt.Sprintf("%s%d", dao.CannedResponseVectorIDPrefix, rule.CannedResponse.Id))
	}

	// 获取向量化结果
	// 其实可以放到协程内, 但为了保持“准备-执行-收尾”三段式结构
	var vectors [][]float32
	if len(keywordsToEmbed) > 0 {
		var err error
		vectors, err = m.embeddingService.CreateEmbeddings(context.Background(), keywordsToEmbed)
		if err != nil {
			return fmt.Errorf("批量创建向量失败: %w", err)
		}
	}

	// 将向量结果填充回规则列表
	var semanticMatchRules []common.KeywordRule
	if len(vectors) == len(semanticRulesToProcess) {
		for i, rule := range semanticRulesToProcess {
			rule.Embedding = vectors[i]
			semanticMatchRules = append(semanticMatchRules, rule)
		}
	}

	var g errgroup.Group
	g.Go(func() error {
		return syncDb(exactMatchRules)
	})
	// syncVectorDb 失败仅记录日志，不应阻塞关键词加载
	g.Go(func() error {
		if err := syncVectorDb(ctx, semanticMatchRules, allSemanticIDs); err != nil {
			global.Log.Errorf("同步关键词到向量数据库时发生非阻塞错误: %v", err)
		}
		return nil
	})
	if err := g.Wait(); err != nil {
		return err
	}

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
func syncVectorDb(ctx context.Context, Rules []common.KeywordRule, activeIDs []string) error {
	upsertedCount, err := dao.App.VectorDb.BatchUpsert(ctx, Rules)
	if err != nil {
		global.Log.Errorln("[gsgf4g]同步关键词到向量数据库失败:", err)
		return fmt.Errorf("同步关键词到向量数据库失败: %w", err)
	}
	global.Log.Printf("成功同步 %d 条语义匹配关键词到向量数据库", upsertedCount)

	_, err = dao.App.VectorDb.PruneStale(ctx, activeIDs)
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
