package adapter

import (
	"encoding/json"
	"strings"

	"teleagent2api/internal/config"
)

// allowedRequestFields lists the fields we forward to the upstream API.
// Any field not in this list is stripped before forwarding.
var allowedRequestFields = map[string]bool{
	"model":       true,
	"messages":    true,
	"stream":      true,
	"temperature": true,
	"top_p":       true,
	"max_tokens":  true,
	"tools":       true,
	"tool_choice": true,
}

// SanitizeRequest strips fields that the upstream does not support,
// preventing "API 调用参数有误" errors from Claude Code requests.
// It also caps max_tokens to the model's maximum output limit.
func SanitizeRequest(body []byte, modelMeta map[string]config.ModelMeta) []byte {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		return body // not valid JSON, forward as-is
	}

	cleaned := make(map[string]json.RawMessage, len(raw))
	for k, v := range raw {
		if allowedRequestFields[k] {
			cleaned[k] = v
		}
	}

	// Resolve model name and cap max_tokens
	if modelRaw, ok := cleaned["model"]; ok {
		var modelName string
		_ = json.Unmarshal(modelRaw, &modelName)
		if meta, ok := modelMeta[modelName]; ok {
			if maxTokensRaw, ok := cleaned["max_tokens"]; ok {
				var maxTokens int
				_ = json.Unmarshal(maxTokensRaw, &maxTokens)
				if maxTokens > meta.MaxOutput {
					capped, _ := json.Marshal(meta.MaxOutput)
					cleaned["max_tokens"] = capped
				}
			}
		}
	}

	out, err := json.Marshal(cleaned)
	if err != nil {
		return body
	}
	return out
}

// TransformNonStreamingResponse rewrites an upstream non-streaming response
// to be fully OpenAI-compatible.
func TransformNonStreamingResponse(body []byte) []byte {
	var resp map[string]json.RawMessage
	if err := json.Unmarshal(body, &resp); err != nil {
		return body
	}

	// Transform choices
	if choicesRaw, ok := resp["choices"]; ok {
		var choices []map[string]json.RawMessage
		if err := json.Unmarshal(choicesRaw, &choices); err != nil {
			return body
		}
		for i, choice := range choices {
			choices[i] = transformChoice(choice)
		}
		choicesOut, _ := json.Marshal(choices)
		resp["choices"] = choicesOut
	}

	// Clean usage: remove non-standard sub-fields
	if usageRaw, ok := resp["usage"]; ok {
		var usage map[string]json.RawMessage
		if err := json.Unmarshal(usageRaw, &usage); err == nil {
			delete(usage, "completion_tokens_details")
			delete(usage, "prompt_tokens_details")
			usageOut, _ := json.Marshal(usage)
			resp["usage"] = usageOut
		}
	}

	// Remove non-standard top-level fields
	delete(resp, "request_id")

	out, err := json.Marshal(resp)
	if err != nil {
		return body
	}
	return out
}

// transformChoice rewrites a single choice object.
func transformChoice(choice map[string]json.RawMessage) map[string]json.RawMessage {
	msgRaw, ok := choice["message"]
	if !ok {
		return choice
	}

	var msg map[string]json.RawMessage
	if err := json.Unmarshal(msgRaw, &msg); err != nil {
		return choice
	}

	// Extract reasoning_content
	var reasoningContent string
	if rcRaw, ok := msg["reasoning_content"]; ok {
		_ = json.Unmarshal(rcRaw, &reasoningContent)
		delete(msg, "reasoning_content")
	}

	// If content is empty but reasoning_content exists, move it to content
	var content string
	if cRaw, ok := msg["content"]; ok {
		_ = json.Unmarshal(cRaw, &content)
	}
	if strings.TrimSpace(content) == "" && reasoningContent != "" {
		contentOut, _ := json.Marshal(reasoningContent)
		msg["content"] = contentOut
	}

	msgOut, _ := json.Marshal(msg)
	choice["message"] = msgOut
	return choice
}

// StreamChunkState tracks state across SSE chunks for transformation.
type StreamChunkState struct {
	roleSent    bool // whether we've emitted the role in a delta
	seenContent bool // whether we've seen a content-bearing delta yet
}

// NewStreamChunkState creates a new state tracker for streaming transformations.
func NewStreamChunkState() *StreamChunkState {
	return &StreamChunkState{}
}

// TransformChunk rewrites a single SSE data payload to be OpenAI-compatible.
// Returns (transformedJSON, skip). If skip is true, this chunk should be
// dropped entirely (used for reasoning-only chunks).
func (s *StreamChunkState) TransformChunk(data []byte) ([]byte, bool) {
	var chunk map[string]json.RawMessage
	if err := json.Unmarshal(data, &chunk); err != nil {
		return data, false
	}

	choicesRaw, ok := chunk["choices"]
	if !ok {
		return data, false
	}
	var choices []map[string]json.RawMessage
	if err := json.Unmarshal(choicesRaw, &choices); err != nil {
		return data, false
	}

	allReasoning := true

	for i, choice := range choices {
		deltaRaw, ok := choice["delta"]
		if !ok {
			allReasoning = false
			continue
		}
		var delta map[string]json.RawMessage
		if err := json.Unmarshal(deltaRaw, &delta); err != nil {
			allReasoning = false
			continue
		}

		// Strip reasoning_content — it's not in OpenAI spec
		hasReasoningContent := false
		if _, ok := delta["reasoning_content"]; ok {
			hasReasoningContent = true
			delete(delta, "reasoning_content")
		}

		_, hasContent := delta["content"]

		// Reasoning-only chunk (no real content) — skip it
		if hasReasoningContent && !hasContent {
			// But check finish_reason: if this is also the final chunk with
			// no content ever sent, convert it so the client gets a response
			if fr, ok := choice["finish_reason"]; ok {
				var frStr string
				_ = json.Unmarshal(fr, &frStr)
				if frStr != "" && frStr != "null" && !s.seenContent {
					// Entirely reasoning-only response — keep the chunk but
					// with empty content so the client sees finish_reason
					delta["content"] = json.RawMessage(`""`)
					if !s.roleSent {
						delta["role"] = json.RawMessage(`"assistant"`)
						s.roleSent = true
					}
					deltaOut, _ := json.Marshal(delta)
					choice["delta"] = deltaOut
					choices[i] = choice
					allReasoning = false
					s.seenContent = true
					continue
				}
			}
			continue
		}

		allReasoning = false
		s.seenContent = true

		if hasContent {
			// Role only in first content-bearing delta
			if s.roleSent {
				delete(delta, "role")
			} else {
				s.roleSent = true
			}
		}

		deltaOut, _ := json.Marshal(delta)
		choice["delta"] = deltaOut
		choices[i] = choice
	}

	if allReasoning {
		return nil, true
	}

	choicesOut, _ := json.Marshal(choices)
	chunk["choices"] = choicesOut

	if usageRaw, ok := chunk["usage"]; ok {
		var usage map[string]json.RawMessage
		if err := json.Unmarshal(usageRaw, &usage); err == nil {
			delete(usage, "completion_tokens_details")
			delete(usage, "prompt_tokens_details")
			usageOut, _ := json.Marshal(usage)
			chunk["usage"] = usageOut
		}
	}

	out, err := json.Marshal(chunk)
	if err != nil {
		return data, false
	}
	return out, false
}
