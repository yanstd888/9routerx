package executor

func moonshotToolNames(body map[string]any) {
	messages, _ := body["messages"].([]any)
	names := map[string]string{}
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok || message["role"] != "assistant" {
			continue
		}
		calls, _ := message["tool_calls"].([]any)
		for _, rawCall := range calls {
			call, ok := rawCall.(map[string]any)
			if !ok {
				continue
			}
			function, _ := call["function"].(map[string]any)
			id, _ := call["id"].(string)
			name, validName := function["name"].(string)
			if id != "" && validName {
				names[id] = name
			}
		}
	}
	for _, raw := range messages {
		message, ok := raw.(map[string]any)
		if !ok || message["role"] != "tool" {
			continue
		}
		if _, supplied := message["name"].(string); supplied {
			continue
		}
		id, _ := message["tool_call_id"].(string)
		if name, exists := names[id]; exists {
			message["name"] = name
		}
	}
}

// Root anyOf is a union, so only requirements common to every object branch
// survive flattening. Nested unions and referenced/non-object branches stay intact.
func moonshotRootToolUnions(body map[string]any) {
	tools, _ := body["tools"].([]any)
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		function, ok := tool["function"].(map[string]any)
		if !ok {
			function = tool
		}
		parameters, ok := function["parameters"].(map[string]any)
		if !ok {
			continue
		}
		branches, ok := parameters["anyOf"].([]any)
		if !ok || len(branches) == 0 || !moonshotObjectBranches(branches) {
			continue
		}
		properties, _ := parameters["properties"].(map[string]any)
		var common []string
		for index, rawBranch := range branches {
			branch := rawBranch.(map[string]any)
			branchProps, hasProperties := branch["properties"].(map[string]any)
			if hasProperties && properties == nil {
				properties = map[string]any{}
			}
			for key, value := range branchProps {
				if _, exists := properties[key]; !exists {
					properties[key] = value
				}
			}
			if _, exists := parameters["additionalProperties"]; !exists {
				if value, supplied := branch["additionalProperties"]; supplied {
					parameters["additionalProperties"] = value
				}
			}
			required := moonshotStringList(branch["required"])
			if index == 0 {
				common = required
			} else {
				intersection := common[:0]
				for _, key := range common {
					if moonshotContains(required, key) {
						intersection = append(intersection, key)
					}
				}
				common = intersection
			}
		}
		required := moonshotStringList(parameters["required"])
		for _, key := range common {
			if !moonshotContains(required, key) {
				required = append(required, key)
			}
		}
		filtered := []string{}
		for _, key := range required {
			if _, exists := properties[key]; properties == nil || exists {
				filtered = append(filtered, key)
			}
		}
		delete(parameters, "anyOf")
		if properties != nil {
			parameters["properties"] = properties
		}
		if _, exists := parameters["type"]; !exists {
			parameters["type"] = "object"
		}
		if len(filtered) > 0 {
			parameters["required"] = filtered
		} else {
			delete(parameters, "required")
		}
	}
}

func moonshotObjectBranches(branches []any) bool {
	for _, raw := range branches {
		branch, ok := raw.(map[string]any)
		if !ok {
			return false
		}
		if _, referenced := branch["$ref"]; referenced {
			return false
		}
		if typ, exists := branch["type"]; exists && typ != "object" && !moonshotContains(moonshotStringList(typ), "object") {
			return false
		}
	}
	return true
}

func moonshotStringList(value any) []string {
	list, _ := value.([]any)
	result := []string{}
	for _, raw := range list {
		if item, ok := raw.(string); ok && !moonshotContains(result, item) {
			result = append(result, item)
		}
	}
	return result
}

func moonshotContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
