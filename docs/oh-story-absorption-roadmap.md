# oh-story Agent Markdown 增强路线

本文记录从 `worldwonderer/oh-story-claudecode` 吸收到 `ainovel-cli` 的现实路线。当前阶段不处理任何拆文、benchmark、导入、扫榜、封面或发布链路，只优先增强本项目已有 agent markdown：`assets/prompts/*.md`、`assets/references/*.md`、`assets/rules/*.md`。

## 当前边界

- 只改 agent markdown 资产，不新增 Go 功能模块作为本路线目标。
- 不处理 `internal/host/bench`、`internal/domain/benchmark`、`/importbench` 或任何拆文目录兼容。
- 不接入 oh-story 的 Claude Code skills、hooks、agents、browser-cdp、story-setup、story-import、story-cover。
- 不把平台榜单、市场采集、拆文报告、原文片段或对标作品内容放入 prompt。
- 只吸收通用写作方法、审稿标准、去 AI 味规则、角色/对话/钩子/反转/节奏方法。

## 已有 Markdown 基础

- `assets/prompts/writer.md` 已规定章节写作流程、最简记忆包、商业门禁、AI Gate 和提交前自检。
- `assets/prompts/editor.md` 已规定七维审阅、原文举证、章节契约、S1-S4 到 severity 的映射和 AI Gate 审美检查。
- `assets/prompts/architect-long.md` 已规定长篇 premise、characters、world_rules、layered_outline、compass、卷弧滚动规划。
- `assets/prompts/architect-short.md` 已规定短篇 premise、outline、characters、world_rules 和短篇收束约束。
- `assets/references/hook-techniques.md`、`reversal-toolkit.md`、`dialogue-writing.md`、`longform-planning.md`、`quality-checklist.md`、`anti-ai-tone.md` 已是最适合吸收 oh-story 方法论的落点。
- `assets/rules/default.md` 已承载低争议机械规则，例如疲劳词、禁用短语和字数约束。

## 优先级 1：Writer Markdown 增强

目标：让 Writer 更稳定地写出有推进、有情绪、有钩子的章节，同时减少 AI 味和模板化自检。

吸收方向：

- 从 oh-story 的写作方法中提炼“最小剧情循环”：目标 -> 阻碍 -> 行动 -> 反馈 -> 新期待。
- 强化章首进入方式：冲突、异常、欲望、危险、关系张力，不用抽象回顾开场。
- 强化章尾钩子类型：危机、选择、未完成目标、关系变动、信息差、情绪余波。
- 强化“只写本章该完成的事”：不提前解释世界观、不透支后续反转、不把大纲总结塞进正文。
- 强化去 AI 味执行：具体动作替代心理标签，短句/停顿打破整齐段落，避免章末升华总结。

建议文件：

- `assets/prompts/writer.md`
- `assets/references/hook-techniques.md`
- `assets/references/anti-ai-tone.md`
- `assets/references/quality-checklist.md`

验收标准：

- Writer prompt 不变长成写作教材，只增加可执行约束和引用现有 reference。
- 新增内容能直接对应 `plan_chapter`、`draft_chapter`、`commit_chapter` 流程。
- 不新增对拆文、benchmark、扫榜、外部平台数据的依赖。

## 优先级 2：Editor Markdown 增强

目标：让 Editor 更像严格审稿人，而不是泛泛评价文本；所有问题都必须能落到证据、严重度和修改方向。

吸收方向：

- 引入 oh-story 审稿的“黄金三问”：读者为什么翻下一页；本章改变了什么；哪个原文证据支持判断。
- 强化 S1-S4 判定：结构性问题优先于局部文笔，局部措辞不能压过角色动机、因果和追读期待。
- 强化商业门禁：核心卖点、冲突推进、情绪曲线、钩子期待、最小剧情循环。
- 强化 AI 味审查：不只列禁用词，还判断段落节奏、心理标签、对话同腔、总结式收尾。
- 强化建议格式：每条 issue 必须说明“改哪里、为什么、改成什么方向”，不能只说“加强”。

建议文件：

- `assets/prompts/editor.md`
- `assets/references/quality-checklist.md`
- `assets/references/anti-ai-tone.md`
- `assets/references/dialogue-writing.md`

验收标准：

- Editor 仍保持七维评审结构，不新增第八维。
- S1-S4 与现有 `critical/error/warning` 映射保持一致。
- 审美问题仍必须引用原文 evidence。
- 不要求 Editor 读取或分析拆文材料。

## 优先级 3：Architect Long Markdown 增强

目标：让长篇规划更重视连载引擎、卷弧功能差异、长期冲突和中期转向，避免“换地图升级打怪”的空洞长篇。

吸收方向：

- 强化题材定位：目标读者、核心消费点、核心兑现承诺、写作禁区必须互相一致。
- 强化故事引擎：外部推进与内部推进都要能跨卷持续运转。
- 强化中期转向：前期方法何时失效，故事如何换挡，主角付出什么新代价。
- 强化卷功能：每卷必须回答“新增什么、失去什么、关系如何变化、为何进入下一卷”。
- 强化弧节奏：铺垫、积累、爆发、收获的循环要服务弧目标，不是均匀排章节。
- 强化反转设计：误导、假胜、揭示后新问题必须改变局势，而不是解释设定。

建议文件：

- `assets/prompts/architect-long.md`
- `assets/references/longform-planning.md`
- `assets/references/reversal-toolkit.md`
- `assets/references/quality-checklist.md`

验收标准：

- 不改变 `save_foundation` 的字段契约。
- 不新增新的规划 artifact 类型。
- 所有增强都服务 premise、characters、world_rules、layered_outline、compass 的生成质量。

## 优先级 4：Architect Short Markdown 增强

目标：让短篇规划更集中、更高密度、更强收束，避免短篇被规划成长篇开头。

吸收方向：

- 强化短篇核心：单冲突、单目标、单段关键关系、一个主要情绪体验。
- 强化开头三段：尽快给出身份差、冲突、危险、误会、秘密或情绪钩子。
- 强化反转与回收：短篇反转必须有伏笔和代价，结尾必须回应核心承诺。
- 强化角色数量限制：角色必须服务主冲突，不能为世界观完整而堆人。
- 强化结尾类型：HE/BE/开放式都要有情绪余韵，不能只做剧情说明。

建议文件：

- `assets/prompts/architect-short.md`
- `assets/references/reversal-toolkit.md`
- `assets/references/hook-techniques.md`
- `assets/references/quality-checklist.md`

验收标准：

- 不改变短篇 `premise -> outline -> characters -> world_rules` 的工具顺序。
- 不引入短篇拆文、榜单或平台采集流程。
- 规划输出仍能被现有 parser 和 store 接受。

## 优先级 5：References 精炼合并

目标：把 oh-story 方法论放进现有 references，避免 prompt 膨胀，也避免新增一堆没人引用的 Markdown。

建议合并：

- `assets/references/hook-techniques.md`：章首钩子、段落级钩子、章尾期待、弱钩子修正。
- `assets/references/reversal-toolkit.md`：假胜、误导、揭示后新问题、代价兑现、短篇反转收束。
- `assets/references/dialogue-writing.md`：潜台词、信息控制、角色声线区分、对白推动行动。
- `assets/references/longform-planning.md`：卷弧节奏、中期转向、长期冲突引擎、高潮构建。
- `assets/references/quality-checklist.md`：黄金三问、最小剧情循环、S1/S2 举证模板、商业门禁。
- `assets/references/anti-ai-tone.md`：低争议禁用句式、节奏量化思路、删除/改写时保留剧情功能。

暂不新增：

- 暂不新增 `market-selection.md`、`genre-modules.md`、`cover-styles.md`。
- 暂不修改 `assets/load.go`，除非已有 reference 明显无法承载。
- 暂不把平台运营指标硬编码进 Writer 或 Editor；平台偏好交给 user rules。

验收标准：

- 每个 reference 增强都至少被一个 agent prompt 明确引用。
- 资料是可执行写作规则，不是长篇理论摘抄。
- 不引入对外部样本、拆文报告或榜单数据的依赖。

## 优先级 6：Rules Markdown 增强

目标：只把可机械检查、低争议、跨题材通用的规则放入 `assets/rules/default.md`。

可吸收：

- 明显 AI 套句和高频疲劳词。
- 过度总结式章末句。
- 过于工整的万能句式提示。
- 用户可覆盖的字数、禁用词、疲劳词示例。

不吸收：

- 平台偏好、题材偏好、审美偏好。
- 需要语境判断的复杂写法。
- 会误伤角色口头禅、专有名词、古风/玄幻特定表达的规则。

建议文件：

- `assets/rules/default.md`
- `rules.md.example`

验收标准：

- 规则数量克制，默认只拦明显问题。
- 用户能通过项目规则覆盖或调整。
- 不让机械规则替代 Editor 的语境判断。

## 推荐实施顺序

1. 增强 `writer.md` 与 `quality-checklist.md`，先解决正文推进、钩子、最小剧情循环。
2. 增强 `editor.md` 与 `anti-ai-tone.md`，让审稿问题更可证据化、可执行。
3. 增强 `architect-long.md` 与 `longform-planning.md`，改善长篇卷弧规划质量。
4. 增强 `architect-short.md` 与 `reversal-toolkit.md`，改善短篇集中度和反转收束。
5. 精炼 `hook-techniques.md`、`dialogue-writing.md`，补足高频写作技法。
6. 最后少量更新 `assets/rules/default.md` 和 `rules.md.example`。
