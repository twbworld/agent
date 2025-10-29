package enum

import (
	"strings"
	"testing"
)

// TestTriagePromptConsistency 单元测试，用于确保分诊台(Triage)的系统提示词中
// 使用的分类标签与代码中定义的常量保持严格一致。
// 这可以防止因修改常量而忘记更新提示词导致的潜在BUG。
func TestTriagePromptConsistency(t *testing.T) {
	prompt := string(SystemPromptTriage)

	// 1. 定义所有需要被检查的常量值
	intents := []TriageIntent{
		TriageIntentProductInquiry,
		TriageIntentOrderInquiry,
		TriageIntentAfterSales,
		TriageIntentRequestHuman,
		TriageIntentOffTopic,
		TriageIntentOtherInquiry,
	}

	emotions := []TriageEmotion{
		TriageEmotionAngry,
		TriageEmotionFrustrated,
		TriageEmotionAnxious,
		TriageEmotionConfused,
		TriageEmotionNeutral,
		TriageEmotionPositive,
	}

	urgencies := []TriageUrgency{
		TriageUrgencyCritical,
		TriageUrgencyHigh,
		TriageUrgencyMedium,
		TriageUrgencyLow,
	}

	// 2. 遍历并断言每个常量的值都存在于Prompt中
	// 为了精确匹配，我们检查带引号的字符串，例如 "product_inquiry"
	for _, intent := range intents {
		expectedSubstring := `"` + string(intent) + `"`
		if !strings.Contains(prompt, expectedSubstring) {
			t.Errorf("SystemPromptTriage应包含意图常量: %s", expectedSubstring)
		}
	}

	for _, emotion := range emotions {
		expectedSubstring := `"` + string(emotion) + `"`
		if !strings.Contains(prompt, expectedSubstring) {
			t.Errorf("SystemPromptTriage应包含情绪常量: %s", expectedSubstring)
		}
	}

	for _, urgency := range urgencies {
		expectedSubstring := `"` + string(urgency) + `"`
		if !strings.Contains(prompt, expectedSubstring) {
			t.Errorf("SystemPromptTriage应包含紧急度常量: %s", expectedSubstring)
		}
	}
}