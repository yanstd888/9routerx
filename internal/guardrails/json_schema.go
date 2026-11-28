// 9RouterX — this file is part of a port maintained by https://github.com/yanstd888/ — https://t.me/aiclub_id
package guardrails

import (
	"encoding/json"
	"fmt"
)

// JSON-schema validation as a guardrails detector. A custom pattern carries
// its schema in the Regex field (the JSON Schema document, marshalled as a
// string) and the detector name "json_schema" opts the engine in.
//
// Coverage (audit, G4.1): the outbound tap runs at two buffered sites — the
// chat handler's non-streaming response (forward.go) and the OpenAI executor's
// buffered answer (openai.go) — plus the streaming tap frame-by-frame. The
// schema detector intentionally only enforces on the buffered paths: a stream
// has already put bytes on the wire before the full answer exists to validate,
// so blocking there would truncate a response the client half-received. The
// streaming tap skips schema validation for that reason (commented at the
// call site); PII masking still runs frame-by-frame.

// validateJSONSchema validates one decoded JSON value against the node,
// returning the mismatch paths. Delegates to schemaValidate.
func validateJSONSchema(node *jsonSchemaNode, value any) []string {
	return schemaValidate(node, value, "")
}

// jsonSchemaNode models the schema subset.
type jsonSchemaNode struct {
	Type       string                     `json:"type"`
	Properties map[string]*jsonSchemaNode `json:"properties,omitempty"`
	Required   []string                   `json:"required,omitempty"`
	Items      *jsonSchemaNode            `json:"items,omitempty"`
}

func (n *jsonSchemaNode) compile() (*jsonSchemaNode, error) { return n, nil }

// schemaValidate validates one decoded JSON value against the node, returning
// the mismatch paths.
func schemaValidate(node *jsonSchemaNode, value any, path string) []string {
	if node == nil {
		return nil
	}
	var mismatches []string
	switch node.Type {
	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: expected object", pathLabel(path))}
		}
		for _, req := range node.Required {
			if _, present := obj[req]; !present {
				mismatches = append(mismatches, fmt.Sprintf("%s: missing required property %q", pathLabel(path), req))
			}
		}
		for name, child := range node.Properties {
			childValue, present := obj[name]
			if !present {
				continue
			}
			mismatches = append(mismatches, schemaValidate(child, childValue, pathLabel(path)+"."+name)...)
		}
	case "array":
		arr, ok := value.([]any)
		if !ok {
			return []string{fmt.Sprintf("%s: expected array", pathLabel(path))}
		}
		if node.Items != nil {
			for i, item := range arr {
				mismatches = append(mismatches, schemaValidate(node.Items, item, fmt.Sprintf("%s[%d]", pathLabel(path), i))...)
			}
		}
	case "string":
		if _, ok := value.(string); !ok {
			mismatches = append(mismatches, fmt.Sprintf("%s: expected string", pathLabel(path)))
		}
	case "number":
		if _, ok := value.(float64); !ok {
			mismatches = append(mismatches, fmt.Sprintf("%s: expected number", pathLabel(path)))
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			mismatches = append(mismatches, fmt.Sprintf("%s: expected boolean", pathLabel(path)))
		}
	case "null":
		if value != nil {
			mismatches = append(mismatches, fmt.Sprintf("%s: expected null", pathLabel(path)))
		}
	}
	return mismatches
}

func pathLabel(path string) string {
	if path == "" {
		return "$"
	}
	return path
}

// ValidateJSONSchema checks a schema carried by a json_schema custom pattern.
// The dashboard uses it before saving a policy so an invalid document cannot
// become a silently skipped detector at runtime.
func ValidateJSONSchema(schemaJSON string) error {
	_, err := parseSchemaNode(schemaJSON)
	return err
}

// parseSchemaNode decodes the schema JSON a custom pattern carries.
func parseSchemaNode(schemaJSON string) (*jsonSchemaNode, error) {
	var node jsonSchemaNode
	if err := json.Unmarshal([]byte(schemaJSON), &node); err != nil {
		return nil, err
	}
	return &node, nil
}
