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

package diagnostics

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/uber/cadence/client"
	"github.com/uber/cadence/client/frontend"
	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/messaging"
	"github.com/uber/cadence/common/types"
	"github.com/uber/cadence/service/worker/diagnostics/analytics"
	"github.com/uber/cadence/service/worker/diagnostics/invariants"
)

const (
	workflowTimeoutSecond = int32(110)
	testTimeStamp         = int64(2547596872371000000)
	timeUnit              = time.Second
)

func Test__retrieveExecutionHistory(t *testing.T) {
	dwtest := testDiagnosticWorkflow(t)
	result, err := dwtest.retrieveExecutionHistory(context.Background(), retrieveExecutionHistoryInputParams{
		Domain: "test",
		Execution: &types.WorkflowExecution{
			WorkflowID: "123",
			RunID:      "abc",
		},
	})
	require.NoError(t, err)
	require.Equal(t, testWorkflowExecutionHistoryResponse(), result)
}

func Test__identifyTimeouts(t *testing.T) {
	dwtest := testDiagnosticWorkflow(t)
	workflowTimeoutData := invariants.ExecutionTimeoutMetadata{
		ExecutionTime:     110 * time.Second,
		ConfiguredTimeout: 110 * time.Second,
		LastOngoingEvent: &types.HistoryEvent{
			ID:        1,
			Timestamp: common.Int64Ptr(testTimeStamp),
			WorkflowExecutionStartedEventAttributes: &types.WorkflowExecutionStartedEventAttributes{
				ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(workflowTimeoutSecond),
			},
		},
	}
	workflowTimeoutDataInBytes, err := json.Marshal(workflowTimeoutData)
	require.NoError(t, err)
	expectedResult := []invariants.InvariantCheckResult{
		{
			InvariantType: invariants.TimeoutTypeExecution.String(),
			Reason:        "START_TO_CLOSE",
			Metadata:      workflowTimeoutDataInBytes,
		},
	}
	result, err := dwtest.identifyTimeouts(context.Background(), identifyTimeoutsInputParams{History: testWorkflowExecutionHistoryResponse()})
	require.NoError(t, err)
	require.Equal(t, expectedResult, result)
}

func Test__rootCauseTimeouts(t *testing.T) {
	dwtest := testDiagnosticWorkflow(t)
	workflowTimeoutData := invariants.ExecutionTimeoutMetadata{
		ExecutionTime:     110 * time.Second,
		ConfiguredTimeout: 110 * time.Second,
		LastOngoingEvent: &types.HistoryEvent{
			ID:        1,
			Timestamp: common.Int64Ptr(testTimeStamp),
			WorkflowExecutionStartedEventAttributes: &types.WorkflowExecutionStartedEventAttributes{
				ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(workflowTimeoutSecond),
			},
		},
		Tasklist: &types.TaskList{
			Name: "testasklist",
			Kind: nil,
		},
	}
	workflowTimeoutDataInBytes, err := json.Marshal(workflowTimeoutData)
	require.NoError(t, err)
	issues := []invariants.InvariantCheckResult{
		{
			InvariantType: invariants.TimeoutTypeExecution.String(),
			Reason:        "START_TO_CLOSE",
			Metadata:      workflowTimeoutDataInBytes,
		},
	}
	taskListBacklog := int64(10)
	taskListBacklogInBytes, err := json.Marshal(invariants.PollersMetadata{TaskListBacklog: taskListBacklog})
	require.NoError(t, err)
	expectedRootCause := []invariants.InvariantRootCauseResult{
		{
			RootCause: invariants.RootCauseTypePollersStatus,
			Metadata:  taskListBacklogInBytes,
		},
	}
	result, err := dwtest.rootCauseTimeouts(context.Background(), rootCauseTimeoutsParams{History: testWorkflowExecutionHistoryResponse(), Domain: "test-domain", Issues: issues})
	require.NoError(t, err)
	require.Equal(t, expectedRootCause, result)
}

func Test__emit(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := messaging.NewMockClient(ctrl)
	mockProducer := messaging.NewMockProducer(ctrl)
	mockProducer.EXPECT().Publish(gomock.Any(), gomock.Any()).Return(nil)
	mockClient.EXPECT().NewProducer(WfDiagnosticsAppName).Return(mockProducer, nil)
	err := emit(context.Background(), analytics.WfDiagnosticsUsageData{}, mockClient)
	require.NoError(t, err)
}

func testDiagnosticWorkflow(t *testing.T) *dw {
	ctrl := gomock.NewController(t)
	mockClientBean := client.NewMockBean(ctrl)
	mockFrontendClient := frontend.NewMockClient(ctrl)
	mockClientBean.EXPECT().GetFrontendClient().Return(mockFrontendClient).AnyTimes()
	mockFrontendClient.EXPECT().GetWorkflowExecutionHistory(gomock.Any(), gomock.Any()).Return(testWorkflowExecutionHistoryResponse(), nil).AnyTimes()
	mockFrontendClient.EXPECT().DescribeTaskList(gomock.Any(), gomock.Any()).Return(&types.DescribeTaskListResponse{
		Pollers: []*types.PollerInfo{
			{
				Identity: "dca24-xy",
			},
		},
		TaskListStatus: &types.TaskListStatus{
			BacklogCountHint: int64(10),
		},
	}, nil).AnyTimes()
	return &dw{
		clientBean: mockClientBean,
	}
}

func testWorkflowExecutionHistoryResponse() *types.GetWorkflowExecutionHistoryResponse {
	return &types.GetWorkflowExecutionHistoryResponse{
		History: &types.History{
			Events: []*types.HistoryEvent{
				{
					ID:        1,
					Timestamp: common.Int64Ptr(testTimeStamp),
					WorkflowExecutionStartedEventAttributes: &types.WorkflowExecutionStartedEventAttributes{
						ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(workflowTimeoutSecond),
					},
				},
				{
					ID:                                       2,
					Timestamp:                                common.Int64Ptr(testTimeStamp + int64(workflowTimeoutSecond)*timeUnit.Nanoseconds()),
					WorkflowExecutionTimedOutEventAttributes: &types.WorkflowExecutionTimedOutEventAttributes{TimeoutType: types.TimeoutTypeStartToClose.Ptr()},
				},
			},
		},
	}
}
