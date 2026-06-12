package style

type prosePattern struct {
	RuleID         string
	Phrase         string
	Severity       string
	Message        string
	SuggestionType string
}

const rulesetVersion = "anti_ai_style_rules.v1"

var antiAIPatterns = []prosePattern{
	{RuleID: "cliche_summary_in_the_end", Phrase: "这一刻", Severity: "warning", Message: "常见章末升华或总结腔", SuggestionType: "remove_summary"},
	{RuleID: "cliche_summary_in_the_end", Phrase: "仿佛一切", Severity: "warning", Message: "常见章末升华或总结腔", SuggestionType: "remove_summary"},
	{RuleID: "cliche_explaining_emotion", Phrase: "说不清为什么", Severity: "info", Message: "解释情绪多于展示动作", SuggestionType: "show_in_scene"},
	{RuleID: "cliche_explaining_emotion", Phrase: "一种莫名", Severity: "warning", Message: "抽象情绪解释偏多", SuggestionType: "show_in_scene"},
	{RuleID: "template_transition", Phrase: "然而", Severity: "info", Message: "模板化转折词命中", SuggestionType: "vary_transition"},
	{RuleID: "template_transition", Phrase: "与此同时", Severity: "info", Message: "模板化转场词命中", SuggestionType: "vary_transition"},
	{RuleID: "abstract_big_word", Phrase: "命运", Severity: "info", Message: "抽象大词命中", SuggestionType: "ground_abstract_word"},
	{RuleID: "abstract_big_word", Phrase: "宿命", Severity: "info", Message: "抽象大词命中", SuggestionType: "ground_abstract_word"},
	{RuleID: "universal_verb", Phrase: "意识到", Severity: "info", Message: "万能认知动词命中", SuggestionType: "replace_with_action"},
	{RuleID: "universal_verb", Phrase: "感受到", Severity: "info", Message: "万能感知动词命中", SuggestionType: "replace_with_action"},
}
