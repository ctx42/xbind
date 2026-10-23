// SPDX-FileCopyrightText: (c) 2026 Rafal Zajac
// SPDX-License-Identifier: MIT

package xbind

import (
	"errors"
	"time"

	"github.com/ctx42/testing/pkg/must"
)

// errTest is a general error used in tests.
var errTest = errors.New("test error")

// waw represents the Warsaw timezone.
var waw = must.Value(time.LoadLocation("Europe/Warsaw"))
