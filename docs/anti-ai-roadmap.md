# 去 AI 味升级 Roadmap

本文记录 `ainovel-cli` 去 AI 味能力的后续演进方案。目标是把当前的"规则库 + prompt 审稿 + 返工队列"升级为"可解释指标 + 热点证据 + 语义审稿 + 项目文风卡 + 分级返工"的闭环。

## 背景

当前实现已经具备三层基础能力：

- `assets/references/anti-ai-tone.md`：语义层 AI 味判据，覆盖固定套句、套路句式、告诉代替展示、节奏过整、对话同腔、章末升华。
- `assets/rules/default.md`：机械规则，覆盖固定短语、疲劳词和章节字数。
- `assets/prompts/writer.md` / `assets/prompts/editor.md`：Writer 写前规避，Editor 写后审稿并通过 `save_review` 触发 `polish` / `rewrite`。
- `internal/domain/writing.go`：已有 `WritingStyleRules` 和 `CommitResult`，后续新增字段时应先确认 domain 归属。

主要缺口：

- AI 味缺少独立可观测指标和局部证据定位，很多判断依赖 Editor 的主观审美。
- `commit_chapter` 当前会返回 `CommitResult` 事实，并额外挂出 `rule_violations`；但还没有句长、段落、对话比例、重复度等自然度统计。
- 项目级文风目前依赖 `style_rules` / `style_anchors`，缺少结构化的 `StyleCard`。
- 返工粒度不够明确，容易在局部 AI 腔问题上触发整章重写。
- `commit_chapter` 的返回事实不会自动进入 Editor 视野；新增 `style_stats` 后必须通过持久工件和 `novel_context` 注入审稿上下文。

## Phase 1：机械自然度统计与热点证据

目标：把"AI 味"从纯审美判断变成可观测事实，并为局部修补提供可定位证据。

设计原则：不要把 `naturalness_score` 或任何单一分数作为硬裁决依据。指标只负责解释"哪里可疑、为什么可疑"，最终是否 `polish` / `rewrite` 仍由 Editor 结合叙事功能判断。

### 文件级变更

- 新增 `internal/domain/style.go`
  - 定义 `StyleStats` / `Metric` / `Hotspot` 等类型，作为 `style_stats.v1` 的 Go 侧承载，遵循 repo 惯例（domain 类型在 `internal/domain` 包）。
  - `StyleStats` 字段：`Chapter`、`ComputedAt`、`SchemaVersion`、`Metrics`、`Hotspots`、`Summary`。
- 新增 `internal/style/stats.go`
  - 实现 `AnalyzeChineseProse(text string) *domain.StyleStats`。
  - 输出句长均值、句长标准差、句长变异系数、连续同构句比例、段落长度均值、段落长度标准差、对话比例、句首独特率、重复 n-gram、套话命中密度。
  - 输出 `hotspots` 数组，每条包含 `rule_id`、`severity`、`span` 或段落/句子序号、`evidence`、`message`、`suggestion_type`。
  - 输出 `summary` 文本摘要，供 `novel_context` 以最小 token 成本注入上下文；完整指标仍保留在 `Metrics` 字段中。
  - 可输出 `style_health` / `naturalness_score` 作为跨章排序和趋势摘要，但必须保留单项指标和热点证据，避免黑盒评分。
  - 中文断句需单独实现并测试：处理 `。！？……`、引号内对话、短对白、半句动作标签，不能只按句号切分。
  - 所有指标只需文本本身，不依赖任何外部 API（无 logprobs/困惑度）或训练模型。纯文本统计，零外部依赖。
- 新增 `internal/style/patterns.go`
  - 放置中文 AI 腔固定模式。
  - 初始覆盖总结腔、解释腔、模板转折、章末升华、万能动词、抽象大词。
  - 每个模式必须有稳定 `rule_id` 和修复建议类型，便于 `diag` / TUI / Editor 引用同一套 taxonomy。
- 新增 `internal/style/stats_test.go`
  - 覆盖低句长方差、低对话比例、句首重复、段落过均匀、套话密度高等样例。
- 修改 `internal/tools/commit_chapter.go`
  - 在主提交路径和 rewrite/polish 提交路径中调用 `style.AnalyzeChineseProse`。
  - 在 `rule_violations` 旁新增 `style_stats` 字段。
  - 保持"只返事实，不阻断提交"的现有工具契约。
- 修改 `internal/domain/writing.go`
  - `CommitResult` 定义在此文件；不建议直接塞完整 `style_stats`。
  - 如需恢复 `PendingCommit.Result` 的提交事实，只保留 `style_stats_ref` 或摘要字段，完整指标由持久工件读取。
  - 若 `style_stats` 只作为 `commit_chapter` 即时返回事实，可优先扩展 `internal/tools/commit_chapter.go` 的 `commitOutput`，避免污染 domain 通用结构。
- 修改 `internal/store/world.go` 或新增轻量存储方法
  - 如果 Phase 2 要让 Editor 通过 `novel_context` 读取 `style_stats`，需要把每章统计落盘到 `meta/style_stats.json` 或章节侧车文件。
  - 持久化 payload 必须包含 `schema_version`、`chapter`、`computed_at`、`metrics`、`hotspots`、`ruleset_version` 或等价字段。
  - rewrite/polish 提交成功后应覆盖对应章节的统计，保证诊断读取的是最新终稿。
  - 不建议依赖 `meta/last_commit.json` 一次性信号传递 `style_stats`；当前该信号接口没有稳定消费方，`style_stats` 应落成长期事实后再由 `novel_context` / `diag` 读取。

### 验收标准

- 普通提交和 rewrite/polish 提交都返回 `style_stats`。
- rewrite/polish 后对应章节的 `style_stats` 会覆盖旧统计，而不是保留首次提交数据。
- `style_stats.hotspots` 至少能定位到段落或句子级证据，Editor 可以引用具体 `rule_id` 和 `evidence`。
- `style_stats` 不改变现有 flow、checkpoint、review_required 行为。
- `go test ./internal/style ./internal/tools` 通过。
- commit 时除了返回 `style_stats`，同步落盘对应章节的 `meta/stats/chapter_{N}.json`（每章独立文件，避免单文件膨胀和并发写冲突）。schema 包含 `model` 字段记录当前使用的 LLM，便于后续按 provider/model 分组分析指标差异。

### 评测闭环：draft vs commit 同章对比

本项目全程 AI 生成，无人类参照文本，"去 AI 味是否有效"的验证方式是对比同一章的**初稿（draft）和终稿（commit）**：

```
draft_chapter 产出 → AnalyzeChineseProse → style_stats_draft
    ↓ polish / rewrite
commit_chapter 产出 → AnalyzeChineseProse → style_stats_commit
    ↓ 对比
指标变化方向是否与 Editor 裁决一致？
```

- 如果 Editor 判 polish/rewrite，返工后 `style_stats` 的句长方差、句首独特率、套话密度等指标应**改善**。
- 如果指标无变化但 Editor 判 accept，说明当前指标未覆盖 Editor 关注的问题维度，需补充指标或修正阈值。
- 如果指标改善但 Editor 仍不满，说明指标和审美脱节，需调整热点到问题类型的映射规则。
- 对比结果进入 Phase 5 的"改写有效性"诊断，不阻断 flow。

## Phase 2：Editor 使用统计事实

目标：让审稿能引用指标，而不是只说"感觉 AI 味重"。

Editor 看到的应是压缩后的诊断事实，不是完整原始指标表。`novel_context` 可以注入当前章 `style_stats.summary`、高优先级 `hotspots` 和与项目 `StyleCard` 的偏差说明；完整历史统计只供 `diag` 和聚合生成使用。

### 文件级变更

- 修改 `assets/prompts/editor.md`
  - 要求 aesthetic 维度优先读取 `novel_context` 注入的 `working_memory.style_stats` 或等价路径。
  - 不直接假设 Editor 能看到 `commit_chapter` 上一轮原始返回；数据必须由持久工件或 `novel_context` 转交。
  - 要求 issue 优先引用 `style_stats.hotspots[*].rule_id` / `evidence`，而不是只引用综合分。
  - 明确指标到问题类型的映射：
    - 句长标准差低：归为节奏过整。
    - 段落长度过均匀：归为结构模板化。
    - 对话比例异常：归为对话/叙述失衡。
    - 句首独特率低：归为句式重复。
    - 套话密度高：归为固定套句或总结腔。
- 修改 `assets/prompts/writer.md`
  - 提交前自检新增量化目标。
  - 要求避免连续同构句、对白比例按章型合理波动、段落长度有变化。
- 修改 `internal/tools/novel_context.go` / `internal/tools/novel_context_builders.go`
  - 将当前待审章节的 `style_stats` 摘要和高优先级 `hotspots` 注入 `working_memory`。
  - 对 rewrite/polish 队列中的章节，注入原终稿和最新草稿的统计对比摘要（如可得），避免把大表直接塞进 prompt。
- 修改 `internal/tools/commit_chapter_test.go`
  - 验证正常提交会附带 `style_stats`。
  - 验证 rewrite/polish 提交也会附带 `style_stats`。
- 修改 `internal/tools/novel_context_test.go`
  - 验证 Editor/Writer 可通过 `novel_context` 读取目标章节的 `style_stats`。
- 修改 `internal/tools/save_review_test.go`
  - 增加一例 aesthetic warning/error 映射不破坏现有 verdict 升级逻辑。

### 验收标准

- Editor prompt 能明确消费 `style_stats`，并要求 issue 引用具体指标。
- `style_stats` 数据流清晰：`commit_chapter` 计算并落盘，`novel_context` 注入，Editor 引用。
- Editor 引用的是热点证据与偏差摘要，不把完整指标表当成写作目标。
- 现有 `accept` / `polish` / `rewrite` 规则不新增第四种 verdict。
- 机械指标只辅助审稿，不绕过 Editor 的叙事判断。

### 先决条件

在进入 Phase 5 之前，必须先完成以下数据链路：

1. `commit_chapter` 产出 `style_stats`。
2. `style_stats` 作为长期事实落盘，而不是只停留在工具即时返回。
3. `novel_context` 能把目标章节的 `style_stats` 注入到 Editor/Writer 可见的上下文。
4. `diag` 的 `Snapshot` / `Stats` 结构能读取并聚合这些统计。

没有这四步，后续 `AIFlavorHotspots`、TUI 展示和审稿引用都不可实现。

其中第 4 步应在 Phase 1 完成时同步打通最小只读路径：即使 TUI 展示放到 Phase 5，`diag.Snapshot` 也应尽早能读到 `style_stats`，避免后续指标 schema 返工。

## Phase 3：项目文风卡 StyleCard

目标：把"每本书应该怎样不像 AI"项目化，而不是全局一套规则。

三层职责必须分开：

- `StyleStats`：章节观测事实，来自确定性分析，可回放、可聚合。
- `WritingStyleRules`：弧级总结出的 LLM 可读文风规则，偏定性。
- `StyleCard`：给 Writer/Editor 消费的项目级量化目标和偏差摘要，由 `StyleStats` 聚合并受用户规则约束。

实现状态：`StyleCard` 已采用完整字段集，并作为 `WritingStyleRules.StyleCard` 存在于 `meta/style_rules.json`；不拆分 `meta/style_card.json`。

### 文件级变更

- 新增 `internal/domain/style.go`
  - 定义 `StyleStats` / `StyleCard`，明确观测事实和消费卡片的边界。
  - `StyleStats` 不嵌入 `WritingStyleRules`；`StyleCard` 可以作为 `WritingStyleRules` 的量化扩展或同 schema 子对象。
  - 字段建议：`DialogueRatioTarget`、`SentenceStdFloor`、`ParagraphVarianceTarget`、`SensoryPreferences`、`BannedPatterns`、`DialogueDNA`、`ChapterEndingPolicy`、`ChapterTypeProfiles`。
  - `ChapterTypeProfiles`：按章型（打斗/对话/描写/过渡四类）区分各指标的正常范围。纯打斗章的句长和对话比例基线完全不同于日常过渡章，不能用全局阈值一刀切。各章型的指标范围从已写章节按类型聚合统计生成，不依赖训练。
- 修改 `internal/domain/writing.go`
  - 评估是否把 `StyleCard` 嵌入 `WritingStyleRules`，或把 `WritingStyleRules` 重命名/扩展为统一文风结构。
  - 推荐方向：`StyleCard` 取代 `style_anchors` 的量化用途，`WritingStyleRules` 保留为 LLM 可读规则；两者通过同一 `meta/style_rules.json` 或统一 schema 管理，`StyleStats` 独立存放。
- 修改 `internal/store/world.go`
  - 优先方案：扩展现有 `SaveStyleRules` / `LoadStyleRules`，把 `StyleCard` 并入 `meta/style_rules.json`，维持单一事实源。
  - 只有在明确需要物理拆分、且加载优先级/迁移脚本都设计完毕时，才考虑新增 `meta/style_card.json`。
  - 无论是否拆分，章节级 `StyleStats` 都不与 `style_rules.json` 混写。
- 修改 `internal/tools/save_arc_summary.go`
  - 弧级总结时允许保存 `style_card`。
  - LLM 只负责提炼可读文风规则；句长、对话比例、段落变化等量化字段应优先由已落盘 `style_stats` 聚合生成，避免让 LLM 猜数值。
- 修改 `internal/tools/novel_context_builders.go`
  - 将 `style_card` 注入 `reference_pack`。
  - 优先级高于通用 `anti_ai_tone`，低于用户显式 `user_rules` / `user_directives`。
- 新增 `internal/tools/style_card_test.go`
  - 验证 `style_card` 能落盘。
  - 验证 `novel_context` 能读取并注入 `reference_pack`。

当前测试覆盖落在 `internal/store/world_test.go`、`internal/tools/save_arc_summary_test.go` 和 `internal/tools/novel_context_test.go`，没有单独拆出 `style_card_test.go`。

### 验收标准

- 弧级总结后可以产生或更新项目级文风约束；若采用统一 schema，则更新 `meta/style_rules.json`，只有在明确拆分时才落 `meta/style_card.json`。
- Writer 下一章能在 `reference_pack.style_card` 或统一后的 `style_rules` 扩展字段里读取项目文风约束。
- 用户规则仍然优先于自动提炼的文风卡。
- 明确 `StyleStats` / `StyleCard` / `WritingStyleRules` 的单一事实来源和消费边界，避免两个文风文件给出冲突约束。

## Phase 4：返工策略分级

目标：减少整章重写，优先局部修，防止过度漂白和风格漂移。

### 文件级变更

- 修改 `assets/references/anti-ai-tone.md`
  - 增加 spot-fix 优先级：删空泛总结、替换模板句、调整句长、修对话同腔，最后才整段重写。
- 修改 `assets/prompts/writer.md`
  - 对 `polish` 明确要求先用 `edit_chapter` 修命中片段。
  - 只有结构性 AI 腔、情节功能缺失或多 Gate 同时命中时才使用 `draft_chapter(mode="write")` 整章覆盖。
- 修改 `internal/domain/review.go`
  - 给 issue 增加可选 `targets` 或等价片段定位字段。
  - 保存具体片段、问题类型和建议处理方式。
- 修改 `internal/tools/save_review.go`
  - 优先不新增新的根级路由字段；继续以现有 `scope + affected_chapters` 作为唯一路由输入。
  - 若未来确实需要 finer-grained 修补模式，优先把修补目标挂在 issue 层，而不是再引入第二套根级路由语义。
- 修改 `internal/tools/edit_chapter_test.go`
  - 覆盖 polish 队列中局部替换优先路径。

### 句子级返工路径

当前 spot-fix 靠 `edit_chapter` 的 `old_string` 精确匹配，适合修改固定文本但无法处理"模式问题"（如"把本章所有排比三连拆掉"）。

改进方向：Phase 1 的 `hotspots` 已经能定位到段落或句子级。返工时 Writer 应能接收 `hotspots` 数组，对每个标记位置单独重写目标段落/句子，然后拼接回去——而不是靠 `edit_chapter` 逐条精确匹配 `old_string`。实现路径：

- Writer 的 polish 流程中，如果 `rewrite_brief.issues` 包含 `hotspot_id`，优先对标记位置做局部重写。
- `draft_chapter(mode="write")` 整章覆盖仅用于结构性 AI 腔、情节功能缺失或多 Gate 同时命中。

### 验收标准

- 局部 AI 腔问题优先走 `edit_chapter` 或句子级热点重写。
- 整章重写只用于结构性问题或多 Gate 重度命中。
- 返工后 `commit_chapter` 仍校验草稿与终稿不同，防止空提交。
- Editor 在 spot-fix 后仍可基于新审稿结果升级为整章 rewrite，避免局部修补改坏结构后无出口。

## Phase 5：诊断与报告

目标：让用户能看到"AI 味为什么重"，并能追踪跨章节趋势。

### 文件级变更

- 修改 `internal/diag/rules_quality.go`
  - 新增 `AIFlavorHotspots` 诊断。
  - 汇总章节级 `style_stats`，识别各单项指标异常和跨章趋势。
  - 继续保留可读热点解释：句长单一、套话密度高、对话比例异常等。
  - 新增 `RewriteEffectiveness` 诊断：对比同一章 draft 和 commit 的 `style_stats`，判断返工是否真的改善了指标。如果 polish/rewrite 后指标无变化甚至恶化，标记为"无效返工"，提示 Editor 可能未对准问题或 Writer 未按热点修改。
  - 新增 `EditorConsistency` 检查：当连续 N 章 Editor 都判 accept 但 `style_stats` 指标持续恶化时，触发告警——说明 Editor 的审美判断可能与可观测事实脱节。
- 修改 `internal/store/world.go` / `internal/tools/commit_chapter.go`
  - 返工提交在覆盖当前章节 `style_stats` 前保存前后指标对比到 `meta/stats/rewrite_comparisons.jsonl`，供 `RewriteEffectiveness` 读取。
- 修改 `internal/diag/snapshot.go` / `internal/diag/types.go`
  - 为 `diag` 增加 `style_stats` 数据源和承载字段；否则 `AIFlavorHotspots` 无从实现。
  - 该只读字段建议在 Phase 1 完成时先接入，Phase 5 再补完整报告和 TUI 展示。
- 修改 `internal/entry/tui/report.go`
  - 在报告中显示自然度、AI 味热点、返工原因。
- 修改 `internal/entry/tui/panels.go`
  - 可选显示最近章节自然度趋势。
- 修改 `README.md`
  - 更新"去 AI 味与自定义规则"。
  - 说明机械规则、自然度统计、StyleCard 三层模型。
- 维护 `docs/anti-ai-roadmap.md`
  - 记录指标定义、阈值、误判边界和后续调参计划。

### 验收标准

- `/diag` 能输出 AI 味相关诊断。
- `/diag` 能输出 `AIFlavorHotspots`、`RewriteEffectiveness`、`EditorConsistency` 三类诊断。
- TUI 报告能解释主要问题来源和热点证据，而不是只显示 rewrite/polish 次数或单一分数。
- README 能说明用户如何通过 `rules.md` 和未来 `StyleCard` 调整项目文风。
- TUI 展示应等字段稳定后再做；建议连续 20 章以上统计字段无 schema 调整后再接入面板。

## 推荐实施顺序

优先做 Phase 1 + Phase 2，同时提前打通 `diag.Snapshot` 的只读字段：

1. 改动集中，主要新增 `internal/style` 并扩展 `commit_chapter` 输出。
2. 不改变 flow，不引入新 verdict，风险低。
3. 能立刻让 Editor 获得可引用证据。
4. 先把 `style_stats` 写入 `diag.Snapshot`，后续 StyleCard 和诊断都能复用这套指标。

然后做 Phase 3，再做 Phase 4：

1. `StyleCard` 提供项目级目标，Phase 4 的 spot-fix / full rewrite 分级才有依据。
2. Phase 3 可先做最小版本，只包含 `SentenceStdFloor`、`DialogueRatioTarget`、`BannedPatterns`，不必一次性覆盖完整文风。
3. 分级返工放在 StyleCard 之后，避免先写一套全局硬编码策略，后续再被项目文风推翻。

最后做 Phase 5：

1. 诊断和 TUI 展示依赖前面已有数据。
2. 适合在指标稳定后再做，避免报告字段频繁变动。

## 指标初始建议

以下阈值只作为初始值，后续应通过真实章节调参：

- 句长标准差：低于 3 视为强风险，3-5 视为弱风险；同时统计连续同构句比例，避免全局 std 被短对白和长描写稀释。
- 对话比例：普通网文章节低于 10% 或高于 55% 触发提示；该指标必须标记为"需章型上下文判断"，纯打斗、独白、谈判等章型由 Editor 结合大纲豁免。
- 句首独特率：低于 50% 强风险，50%-70% 弱风险。
- 段落长度均匀度：最短段 / 最长段大于 0.8 且段落数充足时提示节奏过整。
- 套话密度：每千字命中 3 个以上固定套话时提示。
- 所有阈值必须以真实章节回放调参；中文断句、短对白和章型差异会显著影响这些数字，不能把初始阈值当稳定规则。
- 调参方式：取已完成章节的 draft 和 commit 版本，分别跑 `AnalyzeChineseProse`，对比 Editor 判 accept 的章和判 polish/rewrite 的章的指标分布，找到区分度最高的阈值区间。不需要人类标注数据。

## 外部实现参考

以下项目和研究只作为实现思路参考，不作为"检测器规避"目标。本项目约束：全 AI 生成（无人类参照文本）、调用 API 无 logprobs/困惑度、免训练（不引入需训练的模型）。

**诊断工具参考**：

- `Vale` / `proselint` / `write-good`：可借鉴稳定规则 ID、位置证据、JSON 诊断和可配置规则包。
- `slopless` / `prose-linter`：可借鉴 pattern density、句长变异、重复 n-gram、段落均匀度等透明指标，但不要照搬英文词表。

**改写管线参考**：

- `lynote-ai/humanize-text`：句子级检测 → 句子级重写 → 拼接的管线模式值得借鉴。其 detector-in-the-loop 版本用多种方法打分后只重写被标记句子。但对小说的参考价值主要在管线结构，翻译链策略会破坏叙事声线和专名一致性，不适用。
- `ai-detector-skill`：强调 AI-like signals、短文本护栏、可解释信号输出——与 Phase 1 的"热点证据"设计方向一致。

**风格迁移参考**：

- `CAT-LLM`：中文长文风格迁移，用 TSD 显式建模"风格定义"。适合从"作者风格/体裁风格"角度做改写，但需要训练，仅作为 Phase 3 StyleCard 长期演进参考。
- `TINYSTYLER` / `AuthorMix`：作者嵌入 + LoRA 的少样本风格迁移思路，在项目积累足够章节后可作为 StyleCard 自动提炼的远期方向。

**论文结论**：

- 中文 AI 文本常见信号：句长变化小、标点密集、词汇重复——与 Phase 1 指标方向一致。
- 单指标不可靠，genre / domain 对结果影响很大——验证了 StyleCard 按项目/章型区分的必要性。
- 推理型模型比传统模型更难检出——因此 `style_stats` 持久化应记录 `model` 字段，便于按 provider/model 分组分析。
- 对本项目最有价值的是"可解释诊断 + 局部修复建议 + 趋势回放 + 改写有效性验证"，不是追求某个第三方 AI 检测分数。

## 设计边界

- 不做检测器规避承诺，只做写作质量和自然度改进。
- 不让机械指标直接决定 `rewrite`，最终裁定仍由 Editor 结合叙事功能判断。
- 不因单个指标异常触发大改，必须结合原文证据和阅读影响。
- 不牺牲剧情功能：伏笔、钩子、角色特征、关键信息优先保留。
- 不引入风格克隆风险：StyleCard 只总结本项目已写内容或用户授权样本。
- 不为提高分数而强行增加同义词、碎句或"人类瑕疵"；角色口癖、题材惯用表达和叙事节奏优先于机械多样性。
- 所有指标计算不依赖外部 API（无 logprobs/困惑度/embedding），不引入需训练的模型。纯文本统计，零外部依赖，可离线运行。
