package style

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/voocel/ainovel-cli/internal/domain"
)

type sentenceInfo struct {
	Text      string
	Start     int
	End       int
	Paragraph int
}

// AnalyzeChineseProse computes deterministic, local style signals from prose text.
func AnalyzeChineseProse(text string) *domain.StyleStats {
	paragraphs := splitParagraphs(text)
	sentences := splitSentences(text)
	sentenceLengths := make([]float64, 0, len(sentences))
	for _, sentence := range sentences {
		if l := proseLen(sentence.Text); l > 0 {
			sentenceLengths = append(sentenceLengths, float64(l))
		}
	}
	paragraphLengths := make([]float64, 0, len(paragraphs))
	for _, paragraph := range paragraphs {
		paragraphLengths = append(paragraphLengths, float64(proseLen(paragraph)))
	}

	sentenceMean, sentenceStd := meanStd(sentenceLengths)
	paragraphMean, paragraphStd := meanStd(paragraphLengths)
	dialogueRatio := computeDialogueRatio(text)
	startUnique := sentenceStartUniqueRate(sentences)
	startCategories := sentenceStartCategoryStats(sentences)
	emotionLabelDensity, emotionLabelHits := emotionLabelSignal(text)
	repeatedNGramRate, repeatedNGrams := repeatedNGramSignal(text, 3)
	patternMatches := matchPatterns(text)
	patternDensity := 0.0
	if proseLen(text) > 0 {
		patternDensity = float64(len(patternMatches)) * 1000 / float64(proseLen(text))
	}
	uniformRatio := paragraphUniformRatio(paragraphLengths)
	homogeneousRatio := homogeneousSentenceRatio(sentenceLengths)

	metrics := map[string]domain.StyleMetric{
		"sentence_length_mean":                                {Value: round2(sentenceMean), Unit: "runes", Message: "句长均值"},
		domain.StyleMetricSentenceLengthStddev:                {Value: round2(sentenceStd), Unit: "runes", Message: "句长标准差"},
		"sentence_length_cv":                                  {Value: round2(coefficient(sentenceMean, sentenceStd)), Message: "句长变异系数"},
		domain.StyleMetricHomogeneousSentenceRatio:            {Value: round2(homogeneousRatio), Message: "连续同构句比例"},
		"paragraph_length_mean":                               {Value: round2(paragraphMean), Unit: "runes", Message: "段落长度均值"},
		domain.StyleMetricParagraphLengthStddev:               {Value: round2(paragraphStd), Unit: "runes", Message: "段落长度标准差"},
		domain.StyleMetricParagraphUniformRatio:               {Value: round2(uniformRatio), Message: "段落长度均匀度"},
		"dialogue_ratio":                                      {Value: round2(dialogueRatio), Message: "对话比例"},
		domain.StyleMetricSentenceStartUniqueRate:             {Value: round2(startUnique), Message: "句首独特率"},
		domain.StyleMetricSentenceStartDominantCategoryRatio:  {Value: round2(startCategories.DominantRatio), Message: "句首主导类别占比"},
		domain.StyleMetricSentenceStartAbstractConnectorRatio: {Value: round2(startCategories.AbstractConnectorRatio), Message: "抽象/连接词句首占比"},
		domain.StyleMetricEmotionLabelDensityPer1000:          {Value: round2(emotionLabelDensity), Unit: "hits/1000_runes", Message: "情绪标签密度"},
		domain.StyleMetricRepeatedNGramRate:                   {Value: round2(repeatedNGramRate), Message: "重复三字片段密度"},
		domain.StyleMetricPatternDensityPer1000:               {Value: round2(patternDensity), Unit: "hits/1000_runes", Message: "套话命中密度"},
	}
	for category, ratio := range startCategories.Ratios {
		metrics["sentence_start_category_"+category+"_ratio"] = domain.StyleMetric{Value: round2(ratio), Message: "句首类别占比:" + category}
	}

	hotspots := buildHotspots(sentences, sentenceStd, startUnique, startCategories, dialogueRatio, uniformRatio, emotionLabelDensity, emotionLabelHits, repeatedNGrams, patternMatches)
	return &domain.StyleStats{
		SchemaVersion:  domain.StyleStatsSchemaVersion,
		ComputedAt:     time.Now().Format(time.RFC3339),
		RulesetVersion: rulesetVersion,
		Metrics:        metrics,
		Hotspots:       hotspots,
		Summary:        summarize(metrics, hotspots),
	}
}

func splitParagraphs(text string) []string {
	chunks := strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' })
	out := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		chunk = strings.TrimSpace(chunk)
		if chunk != "" {
			out = append(out, chunk)
		}
	}
	return out
}

func splitSentences(text string) []sentenceInfo {
	runes := []rune(text)
	var out []sentenceInfo
	start := 0
	paragraph := 1
	for i := 0; i < len(runes); i++ {
		if runes[i] == '\n' {
			paragraph++
		}
		if !isSentenceEnd(runes, i) {
			continue
		}
		end := i + 1
		for end < len(runes) && isClosingQuote(runes[end]) {
			end++
		}
		addSentence(&out, runes, start, end, paragraph)
		start = end
		i = end - 1
	}
	addSentence(&out, runes, start, len(runes), paragraph)
	return out
}

func addSentence(out *[]sentenceInfo, runes []rune, start, end, paragraph int) {
	for start < end && unicode.IsSpace(runes[start]) {
		start++
	}
	for end > start && unicode.IsSpace(runes[end-1]) {
		end--
	}
	if start >= end {
		return
	}
	text := strings.TrimSpace(string(runes[start:end]))
	if proseLen(text) == 0 {
		return
	}
	*out = append(*out, sentenceInfo{Text: text, Start: start, End: end, Paragraph: paragraph})
}

func isSentenceEnd(runes []rune, i int) bool {
	switch runes[i] {
	case '。', '！', '？', '!', '?':
		return true
	case '…':
		return i+1 < len(runes) && runes[i+1] == '…'
	default:
		return false
	}
}

func isClosingQuote(r rune) bool {
	return r == '”' || r == '’' || r == '」' || r == '』' || r == '》' || r == '"' || r == '\''
}

func proseLen(text string) int {
	count := 0
	for _, r := range text {
		if unicode.IsSpace(r) || strings.ContainsRune("，、；：,.!?！？。…“”‘’「」『』（）()《》", r) {
			continue
		}
		count++
	}
	return count
}

func meanStd(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	mean := sum / float64(len(values))
	variance := 0.0
	for _, value := range values {
		delta := value - mean
		variance += delta * delta
	}
	return mean, math.Sqrt(variance / float64(len(values)))
}

func coefficient(mean, std float64) float64 {
	if mean == 0 {
		return 0
	}
	return std / mean
}

func computeDialogueRatio(text string) float64 {
	total := proseLen(text)
	if total == 0 {
		return 0
	}
	var quoteStack []rune
	dialogue := 0
	for _, r := range text {
		switch r {
		case '“', '「', '『', '"':
			if r == '"' && len(quoteStack) > 0 && quoteStack[len(quoteStack)-1] == '"' {
				quoteStack = quoteStack[:len(quoteStack)-1]
				continue
			}
			quoteStack = append(quoteStack, r)
			continue
		case '”', '」', '』':
			if len(quoteStack) > 0 {
				quoteStack = quoteStack[:len(quoteStack)-1]
			}
			continue
		}
		if len(quoteStack) > 0 && !unicode.IsSpace(r) && !strings.ContainsRune("，、；：,.!?！？。…", r) {
			dialogue++
		}
	}
	return float64(dialogue) / float64(total)
}

func sentenceStartUniqueRate(sentences []sentenceInfo) float64 {
	starts := map[string]bool{}
	count := 0
	for _, sentence := range sentences {
		start := firstMeaningfulRunes(sentence.Text, 2)
		if start == "" {
			continue
		}
		starts[start] = true
		count++
	}
	if count == 0 {
		return 1
	}
	return float64(len(starts)) / float64(count)
}

type sentenceStartStats struct {
	Ratios                 map[string]float64
	DominantCategory       string
	DominantRatio          float64
	AbstractConnectorRatio float64
}

func sentenceStartCategoryStats(sentences []sentenceInfo) sentenceStartStats {
	counts := map[string]int{}
	total := 0
	for _, sentence := range sentences {
		category := domain.CategorizeSentenceStart(sentence.Text)
		if category == "" {
			continue
		}
		counts[category]++
		total++
	}
	stats := sentenceStartStats{Ratios: map[string]float64{}}
	if total == 0 {
		return stats
	}
	for category, count := range counts {
		ratio := float64(count) / float64(total)
		stats.Ratios[category] = ratio
		if ratio > stats.DominantRatio || (ratio == stats.DominantRatio && category < stats.DominantCategory) {
			stats.DominantCategory = category
			stats.DominantRatio = ratio
		}
	}
	stats.AbstractConnectorRatio = stats.Ratios["abstract_connector"]
	return stats
}

func firstMeaningfulRunes(text string, limit int) string {
	var b strings.Builder
	for _, r := range text {
		if unicode.IsSpace(r) || strings.ContainsRune("，、；：,.!?！？。…“”‘’「」『』", r) {
			continue
		}
		b.WriteRune(r)
		if proseLen(b.String()) >= limit {
			break
		}
	}
	return b.String()
}

func repeatedNGramSignal(text string, n int) (float64, []string) {
	var runes []rune
	for _, r := range text {
		if unicode.IsSpace(r) || strings.ContainsRune("，、；：,.!?！？。…“”‘’「」『』（）()《》", r) {
			continue
		}
		runes = append(runes, r)
	}
	if len(runes) < n {
		return 0, nil
	}
	counts := map[string]int{}
	for i := 0; i <= len(runes)-n; i++ {
		counts[string(runes[i:i+n])]++
	}
	var repeated []string
	for gram, count := range counts {
		if count >= 3 {
			repeated = append(repeated, gram)
		}
	}
	sort.Strings(repeated)
	return float64(len(repeated)) / float64(len(runes)), repeated
}

func emotionLabelSignal(text string) (float64, []string) {
	var hits []string
	for _, label := range domain.EmotionLabels {
		count := strings.Count(text, label)
		for range count {
			hits = append(hits, label)
		}
	}
	length := proseLen(text)
	if length == 0 {
		return 0, hits
	}
	return float64(len(hits)) * 1000 / float64(length), hits
}

func paragraphUniformRatio(lengths []float64) float64 {
	if len(lengths) < 3 {
		return 0
	}
	minValue, maxValue := lengths[0], lengths[0]
	for _, value := range lengths[1:] {
		if value < minValue {
			minValue = value
		}
		if value > maxValue {
			maxValue = value
		}
	}
	if maxValue == 0 {
		return 0
	}
	return minValue / maxValue
}

func homogeneousSentenceRatio(lengths []float64) float64 {
	if len(lengths) < 3 {
		return 0
	}
	matched := 0
	for i := 1; i < len(lengths); i++ {
		if math.Abs(lengths[i]-lengths[i-1]) <= 2 {
			matched++
		}
	}
	return float64(matched) / float64(len(lengths)-1)
}

type patternMatch struct {
	prosePattern
	Start int
	End   int
	Text  string
}

func matchPatterns(text string) []patternMatch {
	runes := []rune(text)
	var matches []patternMatch
	for _, pattern := range antiAIPatterns {
		phrase := []rune(pattern.Phrase)
		for i := 0; i <= len(runes)-len(phrase); i++ {
			if string(runes[i:i+len(phrase)]) == pattern.Phrase {
				matches = append(matches, patternMatch{prosePattern: pattern, Start: i, End: i + len(phrase), Text: pattern.Phrase})
			}
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Start < matches[j].Start })
	return matches
}

func buildHotspots(sentences []sentenceInfo, sentenceStd, startUnique float64, startCategories sentenceStartStats, dialogueRatio, uniformRatio, emotionLabelDensity float64, emotionLabelHits []string, repeated []string, patterns []patternMatch) []domain.StyleHotspot {
	var hotspots []domain.StyleHotspot
	add := func(ruleID, severity string, span *domain.TextSpan, paragraph, sentence int, evidence, message, suggestion string) {
		hotspots = append(hotspots, domain.StyleHotspot{ID: fmt.Sprintf("hs_%03d", len(hotspots)+1), RuleID: ruleID, Severity: severity, Span: span, ParagraphIndex: paragraph, SentenceIndex: sentence, Evidence: evidence, Message: message, SuggestionType: suggestion})
	}
	if sentenceStd <= domain.StyleThresholdLowSentenceStddev && len(sentences) >= 4 {
		add("low_sentence_variance", "warning", &domain.TextSpan{Start: sentences[0].Start, End: sentences[min(3, len(sentences))-1].End}, sentences[0].Paragraph, 1, joinSentenceEvidence(sentences, 3), "句长变化偏低，节奏可能过整", "vary_sentence_length")
	}
	if startUnique < 0.7 && len(sentences) >= 4 {
		add("repeated_sentence_start", "warning", &domain.TextSpan{Start: sentences[0].Start, End: sentences[min(4, len(sentences))-1].End}, sentences[0].Paragraph, 1, joinSentenceEvidence(sentences, 4), "句首重复偏多，句式可能同构", "vary_sentence_opening")
	}
	if startCategories.DominantRatio >= domain.StyleThresholdSentenceStartDominantRatio && len(sentences) >= 5 {
		add("dominant_sentence_start_category", "info", nil, 0, 0, fmt.Sprintf("%s=%.2f", startCategories.DominantCategory, startCategories.DominantRatio), "句首类别过于集中，开场方式可能固化", "vary_sentence_opening_category")
	}
	if startCategories.AbstractConnectorRatio >= domain.StyleThresholdAbstractConnectorRatio && len(sentences) >= 5 {
		add("abstract_connector_sentence_start", "warning", nil, 0, 0, fmt.Sprintf("abstract_connector=%.2f", startCategories.AbstractConnectorRatio), "抽象/连接词起句偏多，转场可能模板化", "remove_template_transition")
	}
	if dialogueRatio < 0.1 && len(sentences) >= 4 {
		add("low_dialogue_ratio", "info", nil, 0, 0, fmt.Sprintf("dialogue_ratio=%.2f", dialogueRatio), "对话比例偏低，需结合章型判断", "check_dialogue_balance")
	}
	if dialogueRatio > 0.55 {
		add("high_dialogue_ratio", "info", nil, 0, 0, fmt.Sprintf("dialogue_ratio=%.2f", dialogueRatio), "对话比例偏高，需结合章型判断", "check_dialogue_balance")
	}
	if uniformRatio > domain.StyleThresholdParagraphUniformRatio {
		add("uniform_paragraph_length", "warning", nil, 0, 0, fmt.Sprintf("min/max=%.2f", uniformRatio), "段落长度过于均匀，结构可能模板化", "vary_paragraph_length")
	}
	if emotionLabelDensity >= domain.StyleThresholdEmotionLabelDensity {
		add("emotion_label_density", "warning", nil, 0, 0, strings.Join(uniqueLimited(emotionLabelHits, 5), "、"), "情绪标签密度偏高，可能以概述代替展示", "show_emotion_with_body_action")
	}
	for _, gram := range repeated {
		add("repeated_ngram", "info", nil, 0, 0, gram, "重复片段出现较多", "reduce_repetition")
		if len(hotspots) >= 8 {
			return hotspots
		}
	}
	for _, match := range patterns {
		add(match.RuleID, match.Severity, &domain.TextSpan{Start: match.Start, End: match.End}, 0, 0, match.Text, match.Message, match.SuggestionType)
		if len(hotspots) >= 12 {
			break
		}
	}
	return hotspots
}

func joinSentenceEvidence(sentences []sentenceInfo, limit int) string {
	parts := make([]string, 0, min(limit, len(sentences)))
	for i := 0; i < len(sentences) && i < limit; i++ {
		parts = append(parts, sentences[i].Text)
	}
	return strings.Join(parts, " / ")
}

func summarize(metrics map[string]domain.StyleMetric, hotspots []domain.StyleHotspot) string {
	parts := []string{
		fmt.Sprintf("句长std %.2f", metrics[domain.StyleMetricSentenceLengthStddev].Value),
		fmt.Sprintf("对话比 %.2f", metrics["dialogue_ratio"].Value),
		fmt.Sprintf("句首独特率 %.2f", metrics[domain.StyleMetricSentenceStartUniqueRate].Value),
		fmt.Sprintf("情绪标签 %.2f/千字", metrics[domain.StyleMetricEmotionLabelDensityPer1000].Value),
		fmt.Sprintf("套话密度 %.2f/千字", metrics[domain.StyleMetricPatternDensityPer1000].Value),
	}
	if len(hotspots) > 0 {
		parts = append(parts, fmt.Sprintf("热点 %d 个，首要 %s", len(hotspots), hotspots[0].RuleID))
	} else {
		parts = append(parts, "未发现高优先级机械热点")
	}
	return strings.Join(parts, "；")
}

func uniqueLimited(items []string, limit int) []string {
	seen := map[string]bool{}
	out := make([]string, 0, min(limit, len(items)))
	for _, item := range items {
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func round2(value float64) float64 {
	return math.Round(value*100) / 100
}
