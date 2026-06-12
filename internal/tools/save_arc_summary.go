package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/voocel/agentcore/schema"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/errs"
	"github.com/voocel/ainovel-cli/internal/store"
)

// SaveArcSummaryTool 保存弧级摘要和角色快照，Editor 在弧结束时调用。
type SaveArcSummaryTool struct {
	store *store.Store
}

func NewSaveArcSummaryTool(store *store.Store) *SaveArcSummaryTool {
	return &SaveArcSummaryTool{store: store}
}

func (t *SaveArcSummaryTool) Name() string { return "save_arc_summary" }
func (t *SaveArcSummaryTool) Description() string {
	return "保存弧级摘要和角色状态快照（长篇模式，弧结束时调用）"
}
func (t *SaveArcSummaryTool) Label() string { return "保存弧摘要" }

// 写工具，禁止并发。
func (t *SaveArcSummaryTool) ReadOnly(_ json.RawMessage) bool        { return false }
func (t *SaveArcSummaryTool) ConcurrencySafe(_ json.RawMessage) bool { return false }

func (t *SaveArcSummaryTool) Schema() map[string]any {
	snapshotSchema := schema.Object(
		schema.Property("name", schema.String("角色名")).Required(),
		schema.Property("status", schema.String("当前状态（存活/受伤/失踪等）")).Required(),
		schema.Property("power", schema.String("能力变化")),
		schema.Property("motivation", schema.String("当前动机")).Required(),
		schema.Property("relations", schema.String("关键关系变化")),
	)
	voiceSchema := schema.Object(
		schema.Property("name", schema.String("角色名")).Required(),
		schema.Property("rules", schema.Array("2-3 条语言特征规则（每条 ≤30 字）", schema.String(""))).Required(),
	)
	styleRulesSchema := schema.Object(
		schema.Property("prose", schema.Array("3-5 条叙述风格规则（每条 ≤50 字，要具体可执行）", schema.String(""))).Required(),
		schema.Property("dialogue", schema.Array("核心角色的对话特征规则", voiceSchema)).Required(),
		schema.Property("taboos", schema.Array("本小说需避免的写法", schema.String(""))),
	)
	styleCardSchema := schema.Object(
		schema.Property("sentence_std_floor", schema.Int("句长标准差下限，建议 3-5 之间；只作目标，不作硬裁决")),
		schema.Property("dialogue_ratio_target", schema.String("对白比例目标范围，如 0.15-0.35，需按章型灵活判断")),
		schema.Property("paragraph_variance_target", schema.String("段落长度变化目标，如 短中长交错 / 转场段落更短")),
		schema.Property("sensory_preferences", schema.Array("本书优先使用的感官描写偏好，如 触觉 / 嗅觉 / 声音", schema.String(""))),
		schema.Property("banned_patterns", schema.Array("本项目特别要避开的 AI 腔模式或套话", schema.String(""))),
		schema.Property("dialogue_dna", schema.Array("角色或角色类型的对白 DNA", schema.Object(
			schema.Property("name", schema.String("角色名或角色类型")).Required(),
			schema.Property("traits", schema.Array("稳定对白特征", schema.String(""))),
		))),
		schema.Property("chapter_ending_policy", schema.String("章末处理策略，如落到动作/物件/选择，避免总结升华")),
		schema.Property("chapter_type_profiles", schema.Array("按章型区分的风格指标范围", schema.Object(
			schema.Property("type", schema.String("章型，如 打斗 / 对话 / 描写 / 过渡")).Required(),
			schema.Property("sentence_std_range", schema.String("句长标准差正常范围，如 4-8")),
			schema.Property("dialogue_ratio_target", schema.String("该章型对白比例目标")),
			schema.Property("paragraph_variance_target", schema.String("该章型段落变化目标")),
			schema.Property("notes", schema.String("章型例外说明")),
		))),
	)
	return schema.Object(
		schema.Property("volume", schema.Int("卷号")).Required(),
		schema.Property("arc", schema.Int("弧号")).Required(),
		schema.Property("title", schema.String("弧标题")).Required(),
		schema.Property("summary", schema.String("弧摘要（500字以内）")).Required(),
		schema.Property("key_events", schema.Array("弧内关键事件", schema.String(""))).Required(),
		schema.Property("character_snapshots", schema.Array("角色状态快照", snapshotSchema)).Required(),
		schema.Property("style_rules", styleRulesSchema),
		schema.Property("style_card", styleCardSchema),
	)
}

func (t *SaveArcSummaryTool) Execute(_ context.Context, args json.RawMessage) (json.RawMessage, error) {
	var a struct {
		Volume             int                        `json:"volume"`
		Arc                int                        `json:"arc"`
		Title              string                     `json:"title"`
		Summary            string                     `json:"summary"`
		KeyEvents          []string                   `json:"key_events"`
		CharacterSnapshots []domain.CharacterSnapshot `json:"character_snapshots"`
		StyleRules         *struct {
			Prose    []string                `json:"prose"`
			Dialogue []domain.CharacterVoice `json:"dialogue"`
			Taboos   []string                `json:"taboos"`
		} `json:"style_rules"`
		StyleCard *domain.StyleCard `json:"style_card"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return nil, fmt.Errorf("invalid args: %w: %w", errs.ErrToolArgs, err)
	}
	if a.Volume <= 0 || a.Arc <= 0 {
		return nil, fmt.Errorf("volume and arc must be > 0: %w", errs.ErrToolArgs)
	}

	arcSummary := domain.ArcSummary{
		Volume:    a.Volume,
		Arc:       a.Arc,
		Title:     a.Title,
		Summary:   a.Summary,
		KeyEvents: a.KeyEvents,
	}
	if err := t.store.Summaries.SaveArcSummary(arcSummary); err != nil {
		return nil, fmt.Errorf("save arc summary: %w: %w", errs.ErrStoreWrite, err)
	}

	if len(a.CharacterSnapshots) > 0 {
		for i := range a.CharacterSnapshots {
			a.CharacterSnapshots[i].Volume = a.Volume
			a.CharacterSnapshots[i].Arc = a.Arc
		}
		if err := t.store.Characters.SaveSnapshots(a.Volume, a.Arc, a.CharacterSnapshots); err != nil {
			return nil, fmt.Errorf("save character snapshots: %w: %w", errs.ErrStoreWrite, err)
		}
	}

	styleRulesSaved := false
	styleCardSaved := false
	if (a.StyleRules != nil && len(a.StyleRules.Prose) > 0) || a.StyleCard != nil {
		existing, err := t.store.World.LoadStyleRules()
		if err != nil {
			return nil, fmt.Errorf("load style rules: %w: %w", errs.ErrStoreRead, err)
		}
		rules := domain.WritingStyleRules{
			Volume:    a.Volume,
			Arc:       a.Arc,
			StyleCard: a.StyleCard,
			UpdatedAt: time.Now().Format(time.RFC3339),
		}
		if existing != nil {
			rules.Prose = existing.Prose
			rules.Dialogue = existing.Dialogue
			rules.Taboos = existing.Taboos
			rules.StyleCard = existing.StyleCard
		}
		if a.StyleRules != nil {
			rules.Prose = a.StyleRules.Prose
			rules.Dialogue = a.StyleRules.Dialogue
			rules.Taboos = a.StyleRules.Taboos
		}
		if a.StyleCard != nil {
			rules.StyleCard = a.StyleCard
		}
		if err := t.store.World.SaveStyleRules(rules); err != nil {
			return nil, fmt.Errorf("save style rules: %w: %w", errs.ErrStoreWrite, err)
		}
		styleRulesSaved = len(rules.Prose) > 0
		styleCardSaved = a.StyleCard != nil
	}

	if _, err := t.store.Checkpoints.AppendArtifact(
		domain.ArcScope(a.Volume, a.Arc), "arc_summary",
		fmt.Sprintf("summaries/arc-v%02da%02d.json", a.Volume, a.Arc),
	); err != nil {
		return nil, fmt.Errorf("checkpoint arc summary: %w: %w", errs.ErrStoreWrite, err)
	}

	return json.Marshal(map[string]any{
		"saved": true, "type": "arc_summary",
		"volume": a.Volume, "arc": a.Arc,
		"snapshots":         len(a.CharacterSnapshots),
		"style_rules_saved": styleRulesSaved,
		"style_card_saved":  styleCardSaved,
	})
}
