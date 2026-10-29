// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// logger.go - tiny logger for the generator. The generator runs out-of-band
// from the API so it doesn't need the full zerolog 12-layer matrix; we just
// emit structured-ish lines to stderr.
package main

import (
	"fmt"
	"os"
	"strings"
	"time"
)

type genLogger struct{ start time.Time }

func newLogger() *genLogger { return &genLogger{start: time.Now()} }

func (l *genLogger) info(event string, kv ...string) {
	l.emit("INFO", event, kv)
}
func (l *genLogger) warn(event string, kv ...string) {
	l.emit("WARN", event, kv)
}
func (l *genLogger) error(event string, err error, kv ...string) {
	kv = append(kv, "err", err.Error())
	l.emit("ERROR", event, kv)
}
func (l *genLogger) fatal(event string, err error) {
	l.emit("FATAL", event, []string{"err", err.Error()})
	os.Exit(1)
}

func (l *genLogger) emit(level, event string, kv []string) {
	fields := []string{
		"level=" + level,
		"event=" + event,
		"elapsedMs=" + time.Since(l.start).String(),
	}
	for i := 0; i+1 < len(kv); i += 2 {
		fields = append(fields, kv[i]+"="+kv[i+1])
	}
	fmt.Fprintln(os.Stderr, strings.Join(fields, " "))
}
