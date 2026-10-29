// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package redaction provides PII-aware log scrubbing. The ScrubbingWriter
// wraps an io.Writer and rewrites every JSON log line in-flight, replacing
// known PII patterns (email, phone, SSN, credit-card, JWT) with [REDACTED]
// BEFORE the line hits disk.
//
// Compliance: GDPR Art. 32, SOC2 CC6.1, HIPAA 164.312(b).
//
// Wiring (done once in logger.Init):
//
//      log.Logger = log.Output(redaction.NewWriter(os.Stdout))
//
// GAP CLOSED: PII redaction in logs (GDPR / privacy compliance) — auto-wired
package redaction

import (
        "bytes"
        "io"
        "regexp"
        "sync"
)

// Patterns is the default PII regex set. Add domain-specific patterns here.
var Patterns = []*regexp.Regexp{
        // Email
        regexp.MustCompile(`(?i)"[a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,}"`),
        // Raw email in any string field value (after the colon)
        regexp.MustCompile(`(?i)(\\?")([a-z0-9._%+\-]+@[a-z0-9.\-]+\.[a-z]{2,})(\\?")`),
        // E.164 phone
        regexp.MustCompile(`\+?[1-9]\d{6,14}`),
        // US SSN (XXX-XX-XXXX)
        regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
        // Visa/MC/Amex credit card (13-19 digit runs, possibly space/dash separated)
        regexp.MustCompile(`\b(?:\d[ -]*?){13,19}\b`),
        // JWT (eyJ... three base64url segments)
        regexp.MustCompile(`eyJ[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+\.[a-zA-Z0-9_-]+`),
}

// replacement is the redaction token.
const replacement = "[REDACTED]"

// Scrub applies the Patterns to a string, replacing matches with [REDACTED].
// Use this for ad-hoc scrubbing of values before logging them.
func Scrub(s string) string {
        for _, p := range Patterns {
                s = p.ReplaceAllString(s, replacement)
        }
        return s
}

// ScrubbingWriter is an io.Writer that scrubs each line before passing it
// to the underlying writer. Lines are assumed to be newline-delimited JSON
// (the zerolog default).
type ScrubbingWriter struct {
        dst io.Writer
        mu  sync.Mutex
        buf bytes.Buffer
}

// NewWriter wraps dst so every line written through it is scrubbed.
func NewWriter(dst io.Writer) *ScrubbingWriter {
        return &ScrubbingWriter{dst: dst}
}

// Write scrubs the buffer line-by-line and forwards to the underlying writer.
// We split on newlines because zerolog writes one JSON object per line.
func (s *ScrubbingWriter) Write(p []byte) (int, error) {
        s.mu.Lock()
        defer s.mu.Unlock()

        s.buf.Write(p)
        total := len(p)

        for {
                // Find the next newline in the buffer.
                idx := bytes.IndexByte(s.buf.Bytes(), '\n')
                if idx < 0 {
                        // No complete line yet; wait for more data.
                        break
                }
                // Extract the complete line INCLUDING the newline.
                line := make([]byte, idx+1)
                copy(line, s.buf.Next(idx+1))

                // Scrub + forward.
                scrubbed := scrubBytes(line)
                if _, err := s.dst.Write(scrubbed); err != nil {
                        return total, err
                }
        }
        return total, nil
}

// scrubBytes applies every Pattern to a byte slice.
func scrubBytes(line []byte) []byte {
        for _, p := range Patterns {
                line = p.ReplaceAll(line, []byte(replacement))
        }
        return line
}
