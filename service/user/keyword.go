package user

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gitee.com/taoJie_1/mall-agent/internal/chatwoot"

	"gitee.com/taoJie_1/mall-agent/dao"
	"gitee.com/taoJie_1/mall-agent/global"
	"gitee.com/taoJie_1/mall-agent/model/dto"
	"gitee.com/taoJie_1/mall-agent/model/enum"
	"gitee.com/taoJie_1/mall-agent/task"
	"gitee.com/taoJie_1/mall-agent/utils"
	"golang.org/x/sync/errgroup"
)

// KeywordService 定义知识库条目管理接口。
type KeywordService interface {
	// ListItems 从 Chatwoot 获取所有预设回复，并归类为知识条目。
	ListItems(ctx context.Context) ([]*dto.KnowledgeItem, error)
	// UpsertItem 创建或更新知识条目。
	UpsertItem(ctx context.Context, req *dto.UpsertKnowledgeItemRequest) error
	// DeleteItem 删除与知识条目相关的所有预设回复。
	DeleteItem(ctx context.Context, itemID string) error
	// GenerateQuestions 调用 LLM 根据上下文生成问题。
	GenerateQuestions(ctx context.Context, req *dto.GenerateQuestionRequest) (*dto.GenerateQuestionResponse, error)
	// ForceSync 手动触发知识库同步任务。
	ForceSync(ctx context.Context) error
}

type keywordService struct {
	taskManager *task.Manager
}

// NewKeywordService 创建 KeywordService 实例。
func NewKeywordService(tm *task.Manager) KeywordService {
	return &keywordService{taskManager: tm}
}

func (s *keywordService) ListItems(ctx context.Context) ([]*dto.KnowledgeItem, error) {
	if global.ChatwootService == nil {
		return nil, errors.New("chatwoot 服务未初始化")
	}

	responses, err := global.ChatwootService.GetCannedResponses()
	if err != nil {
		return nil, fmt.Errorf("从 Chatwoot 获取预设回复失败: %w", err)
	}

	// 按答案内容对问题进行分组，并记录最新的更新时间
	groupedItems := make(map[string]*dto.KnowledgeItem)
	for _, resp := range responses {
		if resp.Content == "" {
			continue
		}

		qType, qText := s.parseShortCode(resp.ShortCode)
		if qText == "" {
			continue
		}

		// 解析更新时间
		updatedAt, err := time.Parse(time.RFC3339, resp.UpdatedAt)
		if err != nil {
			// 如果解析失败，使用一个很早的时间，确保其排序在后
			updatedAt = time.Time{}
			global.Log.Warnf("解析预设回复 #%d 的更新时间 '%s' 失败: %v", resp.Id, resp.UpdatedAt, err)
		}
		updatedAtUnix := updatedAt.Unix()

		item, exists := groupedItems[resp.Content]
		if !exists {
			item = &dto.KnowledgeItem{
				ID:        utils.Hash(resp.Content),
				Answer:    resp.Content,
				Questions: []*dto.Question{},
				UpdatedAt: updatedAtUnix,
			}
			groupedItems[resp.Content] = item
		}

		// 追加问题
		item.Questions = append(item.Questions, &dto.Question{
			ID:       resp.Id,
			Question: qText,
			Type:     string(qType),
		})

		// 更新该知识条目的最新时间戳
		if updatedAtUnix > item.UpdatedAt {
			item.UpdatedAt = updatedAtUnix
		}
	}

	// 将 map 转换为切片以便排序
	knowledgeItems := make([]*dto.KnowledgeItem, 0, len(groupedItems))
	for _, item := range groupedItems {
		knowledgeItems = append(knowledgeItems, item)
	}

	// 按更新时间降序排序
	sort.Slice(knowledgeItems, func(i, j int) bool {
		return knowledgeItems[i].UpdatedAt > knowledgeItems[j].UpdatedAt
	})

	return knowledgeItems, nil
}

func (s *keywordService) UpsertItem(ctx context.Context, req *dto.UpsertKnowledgeItemRequest) error {
	if global.ChatwootService == nil {
		return errors.New("chatwoot 服务未初始化")
	}

	const maxQuestionsPerItem = 20
	if len(req.Questions) > maxQuestionsPerItem {
		return fmt.Errorf("每个知识条目最多只能关联 %d 个问题", maxQuestionsPerItem)
	}

	// 1. 对于更新操作，先删除 Chatwoot 上的旧条目
	if req.ID != "" {
		deletedResponses, err := s.findAndDeleteByGroupID(ctx, req.ID)
		if err != nil {
			return fmt.Errorf("更新时删除旧条目失败: %w", err)
		}
		// 异步执行旧条目的缓存清理
		if len(deletedResponses) > 0 {
			go s.purgeLocalCaches(context.Background(), deletedResponses)
		}
	}

	// 2. 并发地在 Chatwoot 中创建所有新条目，并收集创建成功的结果
	var createdResponses []chatwoot.CannedResponse
	var mu sync.Mutex
	g, _ := errgroup.WithContext(ctx)
	g.SetLimit(10)

	for _, q := range req.Questions {
		q := q
		g.Go(func() error {
			shortCode := s.buildShortCode(q.Type, q.Question)
			newResp, err := global.ChatwootService.CreateCannedResponse(shortCode, req.Answer)
			if err != nil {
				return fmt.Errorf("为问题 '%s' 创建预设回复失败: %w", q.Question, err)
			}
			mu.Lock()
			createdResponses = append(createdResponses, *newResp)
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// 3. 对刚刚创建的条目执行“实时精准更新”
	if len(createdResponses) > 0 {
		go func() {
			if err := s.taskManager.ProcessAndCacheResponses(context.Background(), createdResponses); err != nil {
				global.Log.Errorf("UpsertItem后执行实时精准缓存失败: %v", err)
			}
		}()
	}

	// 4. 调度一个延迟后的“延迟重同步校准”任务
	debounceDelay := time.Duration(global.Config.Ai.KeywordReloadDebounce) * time.Second
	s.taskManager.DebounceKeywordReload(debounceDelay)

	return nil
}

func (s *keywordService) DeleteItem(ctx context.Context, itemID string) error {
	// 1. 从 Chatwoot 查找并删除条目，同时获取被删除条目的详细信息
	deletedResponses, err := s.findAndDeleteByGroupID(ctx, itemID)
	if err != nil {
		return err
	}
	if len(deletedResponses) == 0 {
		global.Log.Infof("DeleteItem: 未找到与 groupID %s 匹配的条目，无需操作", itemID)
		return nil
	}

	// 2. 对本地所有缓存进行精准清理
	if err := s.purgeLocalCaches(ctx, deletedResponses); err != nil {
		// 即便缓存清理失败，也不应阻塞主流程，因为有每日同步任务作为兜底
		global.Log.Errorf("精准清理缓存失败，等待每日同步任务修复: %v", err)
	}

	return nil
}

func (s *keywordService) GenerateQuestions(ctx context.Context, req *dto.GenerateQuestionRequest) (*dto.GenerateQuestionResponse, error) {
	if global.LlmService == nil {
		return nil, errors.New("LLM 服务未初始化")
	}

	var prompt enum.SystemPrompt
	if req.Type == "keyword" {
		prompt = enum.SystemPromptGenQuestionFromKeyword
	} else {
		prompt = enum.SystemPromptGenQuestionFromContent
	}

	// 要求 LLM 返回换行分隔的列表，便于解析。
	const instruction = "请生成3个相关的、不同表述方式的用户问题。每个问题占一行，不要带序号或任何多余符号。"
	fullPrompt := fmt.Sprintf("%s\n\n%s", req.Context, instruction)

	rawResult, err := global.LlmService.GetCompletion(ctx, enum.ModelSmall, prompt, fullPrompt, 0.5)
	if err != nil {
		return nil, fmt.Errorf("调用LLM生成问题失败: %w", err)
	}

	questions := strings.Split(rawResult, "\n")
	var cleanedQuestions []string
	for _, q := range questions {
		trimmed := strings.TrimSpace(q)
		if trimmed != "" {
			cleanedQuestions = append(cleanedQuestions, trimmed)
		}
	}

	return &dto.GenerateQuestionResponse{Questions: cleanedQuestions}, nil
}

func (s *keywordService) ForceSync(ctx context.Context) error {
	return s.taskManager.KeywordReloader()
}

// --- 辅助方法 ---

// findAndDeleteByGroupID 根据分组 ID 查找并删除所有关联的预设回复，并返回被删除的条目列表。
func (s *keywordService) findAndDeleteByGroupID(ctx context.Context, groupID string) ([]chatwoot.CannedResponse, error) {
	if global.ChatwootService == nil {
		return nil, errors.New("chatwoot 服务未初始化")
	}
	allResponses, err := global.ChatwootService.GetCannedResponses()
	if err != nil {
		return nil, fmt.Errorf("删除时获取全量预设回复失败: %w", err)
	}

	var responsesToDelete []chatwoot.CannedResponse
	for _, resp := range allResponses {
		if utils.Hash(resp.Content) == groupID {
			responsesToDelete = append(responsesToDelete, resp)
		}
	}

	if len(responsesToDelete) == 0 {
		return nil, nil
	}

	g, _ := errgroup.WithContext(ctx)
	g.SetLimit(10)
	for _, r := range responsesToDelete {
		resp := r
		g.Go(func() error {
			return global.ChatwootService.DeleteCannedResponse(resp.Id)
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return responsesToDelete, nil
}

// purgeLocalCaches 对内存、Redis 和 ChromaDB 进行精准的缓存清理。
func (s *keywordService) purgeLocalCaches(ctx context.Context, deletedResponses []chatwoot.CannedResponse) error {
	var shortCodesToDel []string
	var vectorIDsToDel []string

	for _, resp := range deletedResponses {
		qType, qText := s.parseShortCode(resp.ShortCode)
		if qText == "" {
			continue
		}

		// 收集用于精确匹配的 short code
		if qType == enum.KeywordTypeExact || qType == enum.KeywordTypeHybrid {
			shortCodesToDel = append(shortCodesToDel, strings.ToLower(qText))
		}

		// 收集用于语义匹配的 vector id
		if qType == enum.KeywordTypeSemantic || qType == enum.KeywordTypeHybrid {
			vectorIDsToDel = append(vectorIDsToDel, fmt.Sprintf("%s%d", dao.CannedResponseVectorIDPrefix, resp.Id))
		}
	}

	g, gCtx := errgroup.WithContext(ctx)

	// 清理 Redis 和内存
	if len(shortCodesToDel) > 0 {
		g.Go(func() error {
			// 清理内存 Map
			global.CannedResponses.Lock()
			for _, code := range shortCodesToDel {
				delete(global.CannedResponses.Data, code)
			}
			global.CannedResponses.Unlock()
			global.Log.Debugf("精准清理内存缓存: %v", shortCodesToDel)

			// 清理 Redis
			if _, err := dao.App.KeywordsDb.DeleteKeywordsFromRedis(gCtx, shortCodesToDel); err != nil {
				return fmt.Errorf("精准清理 Redis 失败: %w", err)
			}
			global.Log.Debugf("精准清理 Redis 缓存: %v", shortCodesToDel)
			return nil
		})
	}

	// 清理 ChromaDB
	if len(vectorIDsToDel) > 0 && global.VectorDb != nil {
		g.Go(func() error {
			if _, err := dao.App.VectorDb.DeleteByIDs(gCtx, vectorIDsToDel); err != nil {
				return fmt.Errorf("精准清理 ChromaDB 失败: %w", err)
			}
			global.Log.Debugf("精准清理 ChromaDB 缓存: %v", vectorIDsToDel)
			return nil
		})
	}

	return g.Wait()
}

// parseShortCode 从 short_code 字符串中解析类型和文本。
func (s *keywordService) parseShortCode(shortCode string) (qType enum.KeywordType, qText string) {
	hybridPrefix := global.Config.Ai.HybridPrefix
	semanticPrefix := global.Config.Ai.SemanticPrefix

	if hybridPrefix != "" && strings.HasPrefix(shortCode, hybridPrefix) {
		return enum.KeywordTypeHybrid, strings.TrimPrefix(shortCode, hybridPrefix)
	}
	if strings.HasPrefix(shortCode, semanticPrefix) {
		return enum.KeywordTypeSemantic, strings.TrimPrefix(shortCode, semanticPrefix)
	}
	return enum.KeywordTypeExact, shortCode
}

// buildShortCode 从类型和文本构建 short_code 字符串。
func (s *keywordService) buildShortCode(qType string, qText string) string {
	switch enum.KeywordType(qType) {
	case enum.KeywordTypeHybrid:
		return global.Config.Ai.HybridPrefix + qText
	case enum.KeywordTypeSemantic:
		return global.Config.Ai.SemanticPrefix + qText
	default: // EXACT
		return qText
	}
}
