package executor

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/binary"
	"encoding/json/jsontext"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/encoding/protowire"
)

type devinDesktopField struct {
	number protowire.Number
	wire   protowire.Type
	bytes  []byte
	value  uint64
}

func (f devinDesktopField) require(wire protowire.Type) error {
	if f.wire != wire {
		return fmt.Errorf("invalid wire type for field %d", f.number)
	}
	return nil
}

func devinDesktopFields(payload []byte, visit func(devinDesktopField) error) error {
	for len(payload) > 0 {
		number, wire, n := protowire.ConsumeTag(payload)
		if n < 0 || number < 1 || number > protowire.MaxValidNumber {
			return errors.New("invalid protobuf tag")
		}
		payload = payload[n:]
		f := devinDesktopField{number: number, wire: wire}
		switch wire {
		case protowire.VarintType:
			f.value, n = protowire.ConsumeVarint(payload)
		case protowire.BytesType:
			f.bytes, n = protowire.ConsumeBytes(payload)
		case protowire.Fixed32Type:
			_, n = protowire.ConsumeFixed32(payload)
		case protowire.Fixed64Type:
			_, n = protowire.ConsumeFixed64(payload)
		default:
			return errors.New("unsupported protobuf wire type")
		}
		if n < 0 {
			return fmt.Errorf("decode protobuf field: %w", protowire.ParseError(n))
		}
		if err := visit(f); err != nil {
			return err
		}
		payload = payload[n:]
	}
	return nil
}

func devinDesktopEnvelope(payload []byte, flags byte) []byte {
	frame := make([]byte, 5+len(payload))
	frame[0] = flags
	binary.BigEndian.PutUint32(frame[1:5], uint32(len(payload)))
	copy(frame[5:], payload)
	return frame
}

func devinDesktopGunzip(payload []byte, limit int) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("gzip header: %w", err)
	}
	defer reader.Close()
	decoded, err := io.ReadAll(io.LimitReader(reader, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("gzip payload: %w", err)
	}
	if len(decoded) > limit {
		return nil, errors.New("gzip payload exceeds safety limit")
	}
	return decoded, nil
}

func devinDesktopFrames(ctx context.Context, upstream io.Reader, encoding string, visit func(devinDesktopResponse) error) error {
	reader := bufio.NewReader(upstream)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var header [5]byte
		if _, err := io.ReadFull(reader, header[:]); err != nil {
			return fmt.Errorf("Connect stream ended without valid trailers: %w", err)
		}
		flags := header[0]
		if flags & ^byte(3) != 0 {
			return errors.New("invalid Connect frame flags")
		}
		size := binary.BigEndian.Uint32(header[1:])
		if size > devinDesktopFrameLimit {
			return errors.New("Connect frame exceeds safety limit")
		}
		payload := make([]byte, int(size))
		if _, err := io.ReadFull(reader, payload); err != nil {
			return fmt.Errorf("truncated Connect frame: %w", err)
		}
		if flags&1 != 0 {
			if encoding != "" && encoding != "gzip" {
				return errors.New("unsupported Connect compression")
			}
			var err error
			payload, err = devinDesktopGunzip(payload, devinDesktopFrameLimit)
			if err != nil {
				return err
			}
		}
		if flags&2 != 0 {
			if reader.Buffered() != 0 {
				return errors.New("data follows Connect terminal frame")
			}
			return devinDesktopTrailer(payload)
		}
		response, err := devinDesktopDecodeResponse(payload)
		if err != nil {
			return err
		}
		if err := visit(response); err != nil {
			return err
		}
	}
}

func devinDesktopTrailer(payload []byte) error {
	var trailer struct {
		Error    jsontext.Value      `json:"error"`
		Metadata map[string][]string `json:"metadata"`
	}
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return errors.New("invalid Connect terminal trailer")
	}
	if err := json.Unmarshal(trimmed, &trailer); err != nil {
		return fmt.Errorf("invalid Connect terminal trailer: %w", err)
	}
	if len(trailer.Error) != 0 {
		return errors.New("Connect terminal trailer reports upstream failure")
	}
	return nil
}
