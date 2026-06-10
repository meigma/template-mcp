package reloader

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// fingerprintErrorMarker prefixes the placeholder fingerprintTools records
// for an unfingerprintable definition. It can never equal a real hex
// fingerprint, and the router's drain gate treats any marker-prefixed
// fingerprint as never matching.
const fingerprintErrorMarker = "!error: "

// Fingerprint returns a deterministic fingerprint of a tool's full wire
// definition: name, title, description, input and output schemas,
// annotations, icons, and _meta. No fields are ignored — even an
// annotations-only change (for example a readOnlyHint flip) produces a
// different fingerprint, because annotation hints affect client behavior.
//
// Two definitions fingerprint equal exactly when their wire forms are
// equivalent, regardless of Go representation: a schema held as a
// map[string]any and the same schema held as a [json.RawMessage] with a
// different key order fingerprint identically. Fields absent from the wire
// compare equal however they are expressed in Go (nil versus empty icons or
// _meta), while an empty annotations object differs from no annotations
// object, exactly as the wire bytes do.
//
// Fingerprint is exported for the downstream adapter: its per-tool
// generation tracking and stale-view gating need the same change detector
// the core uses.
func Fingerprint(tool *mcp.Tool) (string, error) {
	if tool == nil {
		return "", errors.New("tool is nil")
	}

	wire, err := json.Marshal(tool)
	if err != nil {
		return "", fmt.Errorf("marshal tool %q wire definition: %w", tool.Name, err)
	}
	canonical, err := canonicalJSON(wire)
	if err != nil {
		return "", fmt.Errorf("canonicalize tool %q wire definition: %w", tool.Name, err)
	}

	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// canonicalJSON re-encodes one JSON document in a canonical form: object
// keys sorted, whitespace and string escaping normalized. Number literals
// are preserved textually rather than round-tripped through float64, so
// distinct literals for the same value (100 versus 1e2) compare as changed —
// the safe direction for swap gating — and large integers cannot collide by
// losing precision.
func canonicalJSON(doc []byte) ([]byte, error) {
	decoder := json.NewDecoder(bytes.NewReader(doc))
	decoder.UseNumber()

	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	return json.Marshal(value)
}

// fingerprintTools fingerprints every tool in tools, keyed by tool name.
//
// A definition that cannot be fingerprinted should be unreachable — the
// upstream health gate only admits marshalable definitions — so it is logged
// loudly and recorded under a deterministic marker that can never equal a
// real fingerprint: the tool stays visible to change detection and buffered
// calls against it gate as stale rather than silently matching.
func fingerprintTools(logger *slog.Logger, tools []*mcp.Tool) map[string]string {
	fingerprints := make(map[string]string, len(tools))
	for _, tool := range tools {
		if tool == nil {
			logger.Error("skipping nil tool in snapshot while fingerprinting")
			continue
		}
		fingerprint, err := Fingerprint(tool)
		if err != nil {
			logger.Error("fingerprinting tool definition failed", "tool", tool.Name, "error", err)
			fingerprint = fingerprintErrorMarker + err.Error()
		}
		fingerprints[tool.Name] = fingerprint
	}
	return fingerprints
}
