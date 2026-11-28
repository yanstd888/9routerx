package executor

import (
	"context"
	"errors"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

// DevinDesktopModel is an enabled native GetCliModelConfigs entry.
// Fields follow OmniRoute 632d84f5, open-sse/services/devinDesktopModels.ts.
type DevinDesktopModel struct {
	ID              string
	Name            string
	ContextLength   int
	MaxOutputTokens int
	SupportsVision  bool
}

// FetchDevinDesktopModels uses the supplied transport and operator base URL.
// Discovery is unary and authenticates directly with Metadata.api_key, not JWT.
func FetchDevinDesktopModels(ctx context.Context, client *http.Client, apiKey, base string) ([]DevinDesktopModel, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if apiKey == "" {
		return nil, errors.New("devin Desktop API key is required")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ctx, endpoint, err := devinDesktopURL(ctx, base, devinDesktopModelsPath)
	if err != nil {
		return nil, err
	}
	metadata := devinDesktopMetadata{apiKey: apiKey, sessionID: uuid.NewString()}
	resp, err := devinDesktopPost(ctx, devinDesktopClient(client), endpoint, wsMsg(1, metadata.encode()), false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	payload, err := devinDesktopBounded(ctx, resp, devinDesktopFrameLimit)
	if err != nil {
		return nil, devinDesktopFailure(ctx, "model discovery", err)
	}
	if len(payload) >= 2 && payload[0] == 0x1f && payload[1] == 0x8b {
		payload, err = devinDesktopGunzip(payload, devinDesktopFrameLimit)
		if err != nil {
			return nil, devinDesktopFailure(ctx, "model discovery", err)
		}
	}
	models, err := devinDesktopDecodeModels(payload)
	if err != nil {
		return nil, devinDesktopFailure(ctx, "model discovery", err)
	}
	return models, nil
}

func devinDesktopDecodeModels(payload []byte) ([]DevinDesktopModel, error) {
	models := make([]DevinDesktopModel, 0)
	indexes := make(map[string]int)
	err := devinDesktopFields(payload, func(f devinDesktopField) error {
		if f.number != 1 {
			return nil
		}
		if err := f.require(2); err != nil {
			return err
		}
		model, disabled, err := devinDesktopDecodeModel(f.bytes)
		if err != nil {
			return err
		}
		if model.ID == "" || disabled {
			return nil
		}
		if index, exists := indexes[model.ID]; exists {
			models[index] = model
			return nil
		}
		if len(models) >= 4096 {
			return errors.New("model count exceeds safety limit")
		}
		indexes[model.ID] = len(models)
		models = append(models, model)
		return nil
	})
	return models, err
}

func devinDesktopDecodeModel(payload []byte) (DevinDesktopModel, bool, error) {
	var model DevinDesktopModel
	var disabled bool
	err := devinDesktopFields(payload, func(f devinDesktopField) error {
		switch f.number {
		case 1, 22:
			if err := f.require(2); err != nil {
				return err
			}
			text := strings.TrimSpace(string(f.bytes))
			if f.number == 1 {
				model.Name = text
			} else {
				model.ID = text
			}
		case 4, 5, 18:
			if err := f.require(0); err != nil {
				return err
			}
			switch f.number {
			case 4:
				disabled = f.value != 0
			case 5:
				model.SupportsVision = f.value != 0
			case 18:
				if f.value > uint64(math.MaxInt) {
					return errors.New("model context overflows integer")
				}
				model.ContextLength = int(f.value)
			}
		}
		return nil
	})
	if model.Name == "" {
		model.Name = model.ID
	}
	if model.ContextLength == 0 {
		model.ContextLength = 200000
	}
	model.MaxOutputTokens = min(model.ContextLength, 64000)
	return model, disabled, err
}
