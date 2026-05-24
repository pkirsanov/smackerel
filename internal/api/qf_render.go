package api

import (
	"context"
	"encoding/json"
	"time"

	"github.com/smackerel/smackerel/internal/connector"
	"github.com/smackerel/smackerel/internal/connector/qfdecisions"
)

func renderQFCard(artifact connector.RawArtifact, metadata any, surface string) *qfdecisions.PacketCard {
	decoded := qfMetadataMap(metadata)
	if len(decoded) == 0 || decoded["packet_id"] == nil {
		return nil
	}
	artifact.Metadata = decoded
	card, err := qfdecisions.RenderPacketCard(context.Background(), artifact, qfdecisions.RenderOptions{Surface: surface, DeepLinkSigningSupported: true, PreferredSurfaceHintSupported: true, Now: time.Now().UTC()})
	if err != nil {
		return nil
	}
	return &card
}

func qfMetadataMap(metadata any) map[string]any {
	switch value := metadata.(type) {
	case map[string]any:
		return value
	case json.RawMessage:
		return decodeQFMetadata(value)
	case []byte:
		return decodeQFMetadata(value)
	default:
		return nil
	}
}

func decodeQFMetadata(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	return decoded
}
