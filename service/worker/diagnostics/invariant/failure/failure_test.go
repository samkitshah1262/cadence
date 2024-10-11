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

package failure

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/types"
	"github.com/uber/cadence/service/worker/diagnostics/invariant"
)

const (
	testDomain = "test-domain"
)

func Test__Check(t *testing.T) {
	metadata := failureMetadata{
		Identity: "localhost",
	}
	metadataInBytes, err := json.Marshal(metadata)
	require.NoError(t, err)
	testCases := []struct {
		name           string
		testData       *types.GetWorkflowExecutionHistoryResponse
		expectedResult []invariant.InvariantCheckResult
		err            error
	}{
		{
			name:     "workflow execution timeout",
			testData: failedWfHistory(),
			expectedResult: []invariant.InvariantCheckResult{
				{
					InvariantType: ActivityFailed.String(),
					Reason:        GenericError.String(),
					Metadata:      metadataInBytes,
				},
				{
					InvariantType: ActivityFailed.String(),
					Reason:        PanicError.String(),
					Metadata:      metadataInBytes,
				},
				{
					InvariantType: ActivityFailed.String(),
					Reason:        CustomError.String(),
					Metadata:      metadataInBytes,
				},
				{
					InvariantType: WorkflowFailed.String(),
					Reason:        TimeoutError.String(),
					Metadata:      metadataInBytes,
				},
			},
			err: nil,
		},
	}
	for _, tc := range testCases {
		inv := NewInvariant(Params{
			WorkflowExecutionHistory: tc.testData,
			Domain:                   testDomain,
		})
		result, err := inv.Check(context.Background())
		require.Equal(t, tc.err, err)
		require.Equal(t, len(tc.expectedResult), len(result))
		require.ElementsMatch(t, tc.expectedResult, result)
	}
}

func failedWfHistory() *types.GetWorkflowExecutionHistoryResponse {
	return &types.GetWorkflowExecutionHistoryResponse{
		History: &types.History{
			Events: []*types.HistoryEvent{
				{
					ActivityTaskFailedEventAttributes: &types.ActivityTaskFailedEventAttributes{
						Reason:   common.StringPtr("cadenceInternal:Generic"),
						Details:  []byte("test-activity-failure"),
						Identity: "localhost",
					},
				},
				{
					ActivityTaskFailedEventAttributes: &types.ActivityTaskFailedEventAttributes{
						Reason:   common.StringPtr("cadenceInternal:Panic"),
						Details:  []byte("test-activity-failure"),
						Identity: "localhost",
					},
				},
				{
					ActivityTaskFailedEventAttributes: &types.ActivityTaskFailedEventAttributes{
						Reason:   common.StringPtr("custom error"),
						Details:  []byte("test-activity-failure"),
						Identity: "localhost",
					},
				},
				{
					ID: 10,
					DecisionTaskCompletedEventAttributes: &types.DecisionTaskCompletedEventAttributes{
						Identity: "localhost",
					},
				},
				{
					WorkflowExecutionFailedEventAttributes: &types.WorkflowExecutionFailedEventAttributes{
						Reason:                       common.StringPtr("cadenceInternal:Timeout START_TO_CLOSE"),
						Details:                      []byte("test-activity-failure"),
						DecisionTaskCompletedEventID: 10,
					},
				},
			},
		},
	}
}

func Test__RootCause(t *testing.T) {
	metadata := failureMetadata{
		Identity: "localhost",
	}
	metadataInBytes, err := json.Marshal(metadata)
	require.NoError(t, err)
	testCases := []struct {
		name           string
		input          []invariant.InvariantCheckResult
		expectedResult []invariant.InvariantRootCauseResult
		err            error
	}{
		{
			name: "customer side failure",
			input: []invariant.InvariantCheckResult{
				{
					InvariantType: ActivityFailed.String(),
					Reason:        CustomError.String(),
					Metadata:      metadataInBytes,
				}},
			expectedResult: []invariant.InvariantRootCauseResult{{
				RootCause: invariant.RootCauseTypeServiceSideIssue,
				Metadata:  metadataInBytes,
			}},
			err: nil,
		},
	}
	inv := NewInvariant(Params{})
	for _, tc := range testCases {
		result, err := inv.RootCause(context.Background(), tc.input)
		require.Equal(t, tc.err, err)
		require.Equal(t, len(tc.expectedResult), len(result))
		require.ElementsMatch(t, tc.expectedResult, result)
	}
}
