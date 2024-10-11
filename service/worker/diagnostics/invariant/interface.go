// The MIT License (MIT)

// Copyright (c) 2017-2020 Uber Technologies Inc.

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package invariant

import (
	"context"
	"encoding/json"
)

// InvariantCheckResult is the result from the invariant check
type InvariantCheckResult struct {
	InvariantType string
	Reason        string
	Metadata      []byte
}

// InvariantRootCauseResult is the root cause for the issues identified in the invariant check
type InvariantRootCauseResult struct {
	RootCause RootCause
	Metadata  []byte
}

type RootCause string

const (
	RootCauseTypeMissingPollers                      RootCause = "There are no pollers for the tasklist"
	RootCauseTypePollersStatus                       RootCause = "There are pollers for the tasklist. Check backlog status"
	RootCauseTypeHeartBeatingNotEnabled              RootCause = "HeartBeating not enabled for activity"
	RootCauseTypeHeartBeatingEnabledMissingHeartbeat RootCause = "HeartBeating enabled for activity but timed out due to missing heartbeat"
	RootCauseTypeServiceSideIssue                    RootCause = "There is an issue in the worker service code that is causing this failure. Check identity for service logs"
)

func (r RootCause) String() string {
	return string(r)
}

// Invariant represents a condition of a workflow execution.
type Invariant interface {
	Check(context.Context) ([]InvariantCheckResult, error)
	RootCause(context.Context, []InvariantCheckResult) ([]InvariantRootCauseResult, error)
}

func MarshalData(rc any) []byte {
	data, _ := json.Marshal(rc)
	return data
}
