package btcjson

func strPtr(v string) *string {
	return &v
}

func boolPtr(v bool) *bool {
	return &v
}

func llmqTypePtr(v LLMQType) *LLMQType {
	return &v
}
