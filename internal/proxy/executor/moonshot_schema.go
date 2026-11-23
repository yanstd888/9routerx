package executor

import (
	json "encoding/json/v2"
	"fmt"
)

// SanitizeMoonshotTools follows the pinned union-preserving schema hygiene.
func SanitizeMoonshotTools(body []byte) ([]byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode moonshot tools: %w", err)
	}
	if !moonshotSanitizeTools(payload) {
		return body, nil
	}
	out, err := json.Marshal(payload, json.Deterministic(true))
	if err != nil {
		return nil, fmt.Errorf("encode moonshot tools: %w", err)
	}
	return out, nil
}

func moonshotSanitizeTools(body map[string]any) bool {
	tools, ok := body["tools"].([]any)
	if !ok || len(tools) == 0 {
		return false
	}
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		function, wrapped := tool["function"].(map[string]any)
		if !wrapped {
			if tool["type"] != "function" {
				continue
			}
			function = tool
		}
		schema := moonshotSchema(function["parameters"], 0)
		if _, typed := schema["type"]; !typed {
			_, anyOf := schema["anyOf"]
			_, oneOf := schema["oneOf"]
			_, allOf := schema["allOf"]
			if !anyOf && !oneOf && !allOf {
				schema["type"] = "object"
				if _, ok := schema["properties"].(map[string]any); !ok {
					schema["properties"] = map[string]any{}
					if _, exists := schema["additionalProperties"]; !exists {
						schema["additionalProperties"] = true
					}
				}
			}
		}
		function["parameters"] = schema
	}
	return true
}

func moonshotSchema(raw any, depth int) map[string]any {
	source, ok := raw.(map[string]any)
	result := map[string]any{}
	if !ok || depth > 32 {
		return result
	}
	for key, value := range source {
		if value == nil {
			continue
		}
		switch key {
		case "properties":
			if properties, ok := value.(map[string]any); ok {
				cleaned := map[string]any{}
				for name, property := range properties {
					if boolean, ok := property.(bool); ok {
						cleaned[name] = boolean
					} else {
						cleaned[name] = moonshotSchema(property, depth+1)
					}
				}
				result[key] = cleaned
			} else {
				result[key] = value
			}
		case "items":
			if tuple, ok := value.([]any); ok {
				result[key] = map[string]any{}
				for _, item := range tuple {
					if _, ok := item.(map[string]any); ok {
						result[key] = moonshotSchema(item, depth+1)
						break
					}
				}
			} else if _, ok := value.(map[string]any); ok {
				result[key] = moonshotSchema(value, depth+1)
			}
		case "anyOf", "oneOf", "allOf":
			if branches, ok := value.([]any); ok {
				cleaned := make([]any, len(branches))
				for index, branch := range branches {
					cleaned[index] = moonshotSchema(branch, depth+1)
				}
				result[key] = cleaned
			}
		case "additionalProperties":
			if boolean, ok := value.(bool); ok {
				result[key] = boolean
			} else if _, ok := value.(map[string]any); ok {
				result[key] = moonshotSchema(value, depth+1)
			}
		case "enum":
			if values, ok := value.([]any); ok {
				cleaned := []any{}
				for _, item := range values {
					if item != nil {
						cleaned = append(cleaned, item)
					}
				}
				result[key] = cleaned
			} else {
				result[key] = value
			}
		case "required":
			if values, ok := value.([]any); ok {
				cleaned := []any{}
				for _, item := range values {
					if _, ok := item.(string); ok {
						cleaned = append(cleaned, item)
					}
				}
				result[key] = cleaned
			} else {
				result[key] = value
			}
		default:
			result[key] = value
		}
	}
	if properties, ok := result["properties"].(map[string]any); ok {
		if required, ok := result["required"].([]any); ok {
			filtered := []any{}
			for _, raw := range required {
				if _, exists := properties[raw.(string)]; exists {
					filtered = append(filtered, raw)
				}
			}
			result["required"] = filtered
		}
	}
	_, explicit := result["additionalProperties"]
	properties, hasProperties := result["properties"].(map[string]any)
	if !explicit && (result["type"] == "object" || hasProperties) {
		if _, supplied := result["properties"]; !supplied {
			result["properties"] = map[string]any{}
			result["additionalProperties"] = true
		} else if hasProperties && len(properties) == 0 {
			result["additionalProperties"] = true
		}
	}
	return result
}
