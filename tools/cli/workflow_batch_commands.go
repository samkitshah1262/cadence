// Copyright (c) 2017 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/pborman/uuid"
	"github.com/urfave/cli/v2"

	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/types"
	"github.com/uber/cadence/service/worker/batcher"
)

// TerminateBatchJob stops abatch job
func TerminateBatchJob(c *cli.Context) error {
	jobID, err := getRequiredOption(c, FlagJobID)
	if err != nil {
		return PrintableError("Flag not found: ", err)
	}
	reason, err := getRequiredOption(c, FlagReason)
	if err != nil {
		return PrintableError("Flag not found: ", err)
	}
	svcClient := cFactory.ServerFrontendClient(c)
	tcCtx, cancel, err := newContext(c)
	if err != nil {
		return PrintableError("Error creating new context: ", err)
	}
	defer cancel()

	err = svcClient.TerminateWorkflowExecution(
		tcCtx,
		&types.TerminateWorkflowExecutionRequest{
			Domain: common.BatcherLocalDomainName,
			WorkflowExecution: &types.WorkflowExecution{
				WorkflowID: jobID,
				RunID:      "",
			},
			Reason:   reason,
			Identity: getCliIdentity(),
		},
	)
	if err != nil {
		return PrintableError("Failed to terminate batch job", err)
	}
	output := map[string]interface{}{
		"msg": "batch job is terminated",
	}
	prettyPrintJSONObject(output)
	return nil
}

// DescribeBatchJob describe the status of the batch job
func DescribeBatchJob(c *cli.Context) error {
	jobID, err := getRequiredOption(c, FlagJobID)
	if err != nil {
		return PrintableError("Flag not found: ", err)
	}
	svcClient := cFactory.ServerFrontendClient(c)
	tcCtx, cancel, err := newContext(c)
	if err != nil {
		return PrintableError("Error creating new context: ", err)
	}
	defer cancel()

	wf, err := svcClient.DescribeWorkflowExecution(
		tcCtx,
		&types.DescribeWorkflowExecutionRequest{
			Domain: common.BatcherLocalDomainName,
			Execution: &types.WorkflowExecution{
				WorkflowID: jobID,
				RunID:      "",
			},
		},
	)
	if err != nil {
		return PrintableError("Failed to describe batch job", err)
	}

	output := map[string]interface{}{}
	if wf.WorkflowExecutionInfo.CloseStatus != nil {
		if wf.WorkflowExecutionInfo.GetCloseStatus() != types.WorkflowExecutionCloseStatusCompleted {
			output["msg"] = "batch job stopped status: " + wf.WorkflowExecutionInfo.GetCloseStatus().String()
		} else {
			output["msg"] = "batch job is finished successfully"
		}
	} else {
		output["msg"] = "batch job is running"
		if len(wf.PendingActivities) > 0 {
			hbdBinary := wf.PendingActivities[0].HeartbeatDetails
			hbd := batcher.HeartBeatDetails{}
			err := json.Unmarshal(hbdBinary, &hbd)
			if err != nil {
				return PrintableError("Failed to describe batch job", err)
			}
			output["progress"] = hbd
		}
	}
	prettyPrintJSONObject(output)
	return nil
}

// ListBatchJobs list the started batch jobs
func ListBatchJobs(c *cli.Context) error {
	domain, err := getRequiredOption(c, FlagDomain)
	if err != nil {
		return PrintableError("Flag not found: ", err)
	}
	pageSize := c.Int(FlagPageSize)
	svcClient := cFactory.ServerFrontendClient(c)

	tcCtx, cancel, err := newContext(c)
	if err != nil {
		return PrintableError("Error creating new context: ", err)
	}
	defer cancel()

	resp, err := svcClient.ListWorkflowExecutions(
		tcCtx,
		&types.ListWorkflowExecutionsRequest{
			Domain:   common.BatcherLocalDomainName,
			PageSize: int32(pageSize),
			Query:    fmt.Sprintf("CustomDomain = '%v'", domain),
		},
	)
	if err != nil {
		return PrintableError("Failed to list batch jobs", err)
	}
	output := make([]interface{}, 0, len(resp.Executions))
	for _, wf := range resp.Executions {
		job := map[string]string{
			"jobID":     wf.Execution.GetWorkflowID(),
			"startTime": convertTime(wf.GetStartTime(), false),
			"reason":    string(wf.Memo.Fields["Reason"]),
			"operator":  string(wf.SearchAttributes.IndexedFields["Operator"]),
		}

		if wf.CloseStatus != nil {
			job["status"] = wf.CloseStatus.String()
			job["closeTime"] = convertTime(wf.GetCloseTime(), false)
		} else {
			job["status"] = "RUNNING"
		}

		output = append(output, job)
	}
	prettyPrintJSONObject(output)
	return nil
}

// StartBatchJob starts a batch job
func StartBatchJob(c *cli.Context) error {
	domain, err := getRequiredOption(c, FlagDomain)
	if err != nil {
		return PrintableError("Flag not found: ", err)
	}
	query, err := getRequiredOption(c, FlagListQuery)
	if err != nil {
		return PrintableError("Flag not found: ", err)
	}
	reason, err := getRequiredOption(c, FlagReason)
	if err != nil {
		return PrintableError("Flag not found: ", err)
	}
	batchType, err := getRequiredOption(c, FlagBatchType)
	if err != nil {
		return PrintableError("Flag not found: ", err)
	}

	if !validateBatchType(batchType) {
		return PrintableError("batchType is not valid, supported:"+strings.Join(batcher.AllBatchTypes, ","), nil)
	}
	operator := getCurrentUserFromEnv()
	var sigName, sigVal string
	if batchType == batcher.BatchTypeSignal {
		sigName, err = getRequiredOption(c, FlagSignalName)
		if err != nil {
			return PrintableError("Flag not found: ", err)
		}
		sigVal, err = getRequiredOption(c, FlagInput)
		if err != nil {
			return PrintableError("Flag not found: ", err)
		}
	}
	var sourceCluster, targetCluster string
	if batchType == batcher.BatchTypeReplicate {
		sourceCluster, err = getRequiredOption(c, FlagSourceCluster)
		if err != nil {
			return PrintableError("Flag not found: ", err)
		}
		targetCluster, err = getRequiredOption(c, FlagTargetCluster)
		if err != nil {
			return PrintableError("Flag not found: ", err)
		}
	}
	rps := c.Int(FlagRPS)
	pageSize := c.Int(FlagPageSize)
	concurrency := c.Int(FlagConcurrency)
	retryAttempt := c.Int(FlagRetryAttempts)
	heartBeatTimeout := time.Duration(c.Int(FlagActivityHeartBeatTimeout)) * time.Second

	svcClient := cFactory.ServerFrontendClient(c)
	tcCtx, cancel, err := newContext(c)
	if err != nil {
		return PrintableError("Add new context failed.", err)
	}
	defer cancel()

	resp, err := svcClient.CountWorkflowExecutions(
		tcCtx,
		&types.CountWorkflowExecutionsRequest{
			Domain: domain,
			Query:  query,
		},
	)
	if err != nil {
		return PrintableError("Failed to count impacting workflows for starting a batch job", err)
	}
	fmt.Printf("This batch job will be operating on %v workflows.\n", resp.GetCount())
	if !c.Bool(FlagYes) {
		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Print("Please confirm[Yes/No]:")
			text, err := reader.ReadString('\n')
			if err != nil {
				return PrintableError("Failed to  get confirmation for starting a batch job", err)
			}
			if strings.EqualFold(strings.TrimSpace(text), "yes") {
				break
			} else {
				fmt.Println("Batch job is not started")
				return nil
			}
		}

	}
	tcCtx, cancel, err = newContext(c)
	if err != nil {
		return PrintableError("Add new context failed.", err)
	}
	defer cancel()

	params := batcher.BatchParams{
		DomainName: domain,
		Query:      query,
		Reason:     reason,
		BatchType:  batchType,
		SignalParams: batcher.SignalParams{
			SignalName: sigName,
			Input:      sigVal,
		},
		ReplicateParams: batcher.ReplicateParams{
			SourceCluster: sourceCluster,
			TargetCluster: targetCluster,
		},
		RPS:                      rps,
		Concurrency:              concurrency,
		PageSize:                 pageSize,
		AttemptsOnRetryableError: retryAttempt,
		ActivityHeartBeatTimeout: heartBeatTimeout,
	}
	input, err := json.Marshal(params)
	if err != nil {
		return PrintableError("Failed to encode batch job parameters", err)
	}
	memo, err := getWorkflowMemo(map[string]interface{}{
		"Reason": reason,
	})
	if err != nil {
		return PrintableError("Failed to encode batch job memo", err)
	}
	searchAttributes, err := serializeSearchAttributes(map[string]interface{}{
		"CustomDomain": domain,
		"Operator":     operator,
	})
	if err != nil {
		return PrintableError("Failed to encode batch job search attributes", err)
	}
	workflowID := uuid.NewRandom().String()
	request := &types.StartWorkflowExecutionRequest{
		Domain:                              common.BatcherLocalDomainName,
		RequestID:                           uuid.New(),
		WorkflowID:                          workflowID,
		ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(int32(batcher.InfiniteDuration.Seconds())),
		TaskStartToCloseTimeoutSeconds:      common.Int32Ptr(int32(defaultDecisionTimeoutInSeconds)),
		TaskList:                            &types.TaskList{Name: batcher.BatcherTaskListName},
		Memo:                                memo,
		SearchAttributes:                    searchAttributes,
		WorkflowType:                        &types.WorkflowType{Name: batcher.BatchWFTypeName},
		Input:                               input,
	}
	_, err = svcClient.StartWorkflowExecution(tcCtx, request)
	if err != nil {
		return PrintableError("Failed to start batch job", err)
	}
	output := map[string]interface{}{
		"msg":   "batch job is started",
		"jobID": workflowID,
	}
	prettyPrintJSONObject(output)
	return nil
}

func validateBatchType(bt string) bool {
	for _, b := range batcher.AllBatchTypes {
		if b == bt {
			return true
		}
	}
	return false
}
