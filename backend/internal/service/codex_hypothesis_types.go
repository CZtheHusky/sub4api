package service

const CodexHypothesisPromptPlaceholder = "{{MODELTRACE_PROMPT}}"

type CodexHypothesisReplayMessage struct {
	Role  string
	Parts []string
}

type CodexHypothesisReplay struct {
	Instructions   string
	Messages       []CodexHypothesisReplayMessage
	InstallationID string
	RecordedModel  string
}
