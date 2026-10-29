// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// helpers.go - tiny indirections so the rest of httpx stays self-contained.
package httpx

import (
	"errors"
	"time"
)

// errorsAs is errors.As re-exported so the envelope code doesn't need its own
// import block.
func errorsAs(err error, target any) bool { return errors.As(err, target) }

// timeNowUTC is wrapped so tests can stub the clock.
var timeNowUTC = func() time.Time { return time.Now().UTC() }
