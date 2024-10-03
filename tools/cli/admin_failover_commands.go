// Copyright (c) 2017-2020 Uber Technologies Inc.
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
	"context"
	"encoding/json"
	"fmt"
	"os/user"
	"time"

	"github.com/pborman/uuid"
	"github.com/urfave/cli/v2"

	"github.com/uber/cadence/client/frontend"
	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/types"
	"github.com/uber/cadence/service/worker/failovermanager"
)

const (
	defaultAbortReason                      = "Failover aborted through admin CLI"
	defaultBatchFailoverSize                = 20
	defaultBatchFailoverWaitTimeInSeconds   = 30
	defaultFailoverWorkflowTimeoutInSeconds = 1200
)

type startParams struct {
	targetCluster                  string
	sourceCluster                  string
	batchFailoverSize              int
	batchFailoverWaitTimeInSeconds int
	failoverWorkflowTimeout        int
	failoverTimeout                int
	domains                        []string
	drillWaitTime                  int
	cron                           string
}

// AdminFailoverStart start failover workflow
func AdminFailoverStart(c *cli.Context) error {
	targetcluster, err := getRequiredOption(c, FlagTargetCluster)
	if err != nil {
		return PrintableError("Flag not found: ", err)
	}
	sourcecluster, err := getRequiredOption(c, FlagSourceCluster)
	if err != nil {
		return PrintableError("Flag not found: ", err)
	}
	params := &startParams{
		targetCluster:                  targetcluster,
		sourceCluster:                  sourcecluster,
		batchFailoverSize:              c.Int(FlagFailoverBatchSize),
		batchFailoverWaitTimeInSeconds: c.Int(FlagFailoverWaitTime),
		failoverTimeout:                c.Int(FlagFailoverTimeout),
		failoverWorkflowTimeout:        c.Int(FlagExecutionTimeout),
		domains:                        c.StringSlice(FlagFailoverDomains),
		drillWaitTime:                  c.Int(FlagFailoverDrillWaitTime),
		cron:                           c.String(FlagCronSchedule),
	}
	return failoverStart(c, params)
}

// AdminFailoverPause pause failover workflow
func AdminFailoverPause(c *cli.Context) error {
	err := executePauseOrResume(c, getFailoverWorkflowID(c), true)
	if err != nil {
		return PrintableError("Failed to pause failover workflow", err)
	}
	fmt.Println("Failover paused on " + getFailoverWorkflowID(c))
	return nil
}

// AdminFailoverResume resume a paused failover workflow
func AdminFailoverResume(c *cli.Context) error {
	err := executePauseOrResume(c, getFailoverWorkflowID(c), false)
	if err != nil {
		return PrintableError("Failed to resume failover workflow", err)
	}
	fmt.Println("Failover resumed on " + getFailoverWorkflowID(c))
	return nil
}

// AdminFailoverQuery query a failover workflow
func AdminFailoverQuery(c *cli.Context) error {
	client := getCadenceClient(c)
	tcCtx, cancel, err := newContext(c)
	if err != nil {
		return PrintableError("Error creating new context: ", err)
	}
	defer cancel()
	workflowID := getFailoverWorkflowID(c)
	runID := getRunID(c)
	result, err := query(tcCtx, client, workflowID, runID)
	if err != nil {
		return err
	}
	request := &types.DescribeWorkflowExecutionRequest{
		Domain: common.SystemLocalDomainName,
		Execution: &types.WorkflowExecution{
			WorkflowID: workflowID,
			RunID:      runID,
		},
	}

	descResp, err := client.DescribeWorkflowExecution(tcCtx, request)
	if err != nil {
		return PrintableError("Failed to describe workflow", err)
	}
	if isWorkflowTerminated(descResp) {
		result.State = failovermanager.WorkflowAborted
	}
	prettyPrintJSONObject(result)
	return nil
}

// AdminFailoverAbort abort a failover workflow
func AdminFailoverAbort(c *cli.Context) error {
	client := getCadenceClient(c)
	tcCtx, cancel, err := newContext(c)
	if err != nil {
		return PrintableError("Error creating new context: ", err)
	}
	defer cancel()

	reason := c.String(FlagReason)
	if len(reason) == 0 {
		reason = defaultAbortReason
	}
	workflowID := getFailoverWorkflowID(c)
	runID := getRunID(c)
	request := &types.TerminateWorkflowExecutionRequest{
		Domain: common.SystemLocalDomainName,
		WorkflowExecution: &types.WorkflowExecution{
			WorkflowID: workflowID,
			RunID:      runID,
		},
		Reason: reason,
	}

	err = client.TerminateWorkflowExecution(tcCtx, request)
	if err != nil {
		return PrintableError("Failed to abort failover workflow", err)
	}

	fmt.Println("Failover aborted")
	return nil
}

// AdminFailoverRollback rollback a failover run
func AdminFailoverRollback(c *cli.Context) error {
	client := getCadenceClient(c)
	tcCtx, cancel, err := newContext(c)
	if err != nil {
		return PrintableError("Error creating new context: ", err)
	}
	defer cancel()

	runID := getRunID(c)

	queryResult, err := query(tcCtx, client, failovermanager.FailoverWorkflowID, runID)
	if err != nil {
		return err
	}
	if isWorkflowRunning(queryResult) {
		request := &types.TerminateWorkflowExecutionRequest{
			Domain: common.SystemLocalDomainName,
			WorkflowExecution: &types.WorkflowExecution{
				WorkflowID: failovermanager.FailoverWorkflowID,
				RunID:      runID,
			},
			Reason:   "Rollback",
			Identity: getCliIdentity(),
		}

		err := client.TerminateWorkflowExecution(tcCtx, request)
		if err != nil {
			return PrintableError("Failed to terminate failover workflow", err)
		}
	}
	// query again
	queryResult, err = query(tcCtx, client, failovermanager.FailoverWorkflowID, runID)
	if err != nil {
		return err
	}
	var rollbackDomains []string
	// rollback includes both success and failed domains to make sure no leftover domains
	rollbackDomains = append(rollbackDomains, queryResult.SuccessDomains...)
	rollbackDomains = append(rollbackDomains, queryResult.FailedDomains...)

	params := &startParams{
		targetCluster:                  queryResult.SourceCluster,
		sourceCluster:                  queryResult.TargetCluster,
		domains:                        rollbackDomains,
		batchFailoverSize:              c.Int(FlagFailoverBatchSize),
		batchFailoverWaitTimeInSeconds: c.Int(FlagFailoverWaitTime),
		failoverTimeout:                c.Int(FlagFailoverTimeout),
		failoverWorkflowTimeout:        c.Int(FlagExecutionTimeout),
	}
	return failoverStart(c, params)
}

// AdminFailoverList list failover runs
func AdminFailoverList(c *cli.Context) error {
	if err := c.Set(FlagWorkflowID, getFailoverWorkflowID(c)); err != nil {
		return err
	}
	if err := c.Set(FlagDomain, common.SystemLocalDomainName); err != nil {
		return err
	}
	return ListWorkflow(c)
}

func query(
	tcCtx context.Context,
	client frontend.Client,
	workflowID string,
	runID string) (*failovermanager.QueryResult, error) {

	request := &types.QueryWorkflowRequest{
		Domain: common.SystemLocalDomainName,
		Execution: &types.WorkflowExecution{
			WorkflowID: workflowID,
			RunID:      runID,
		},
		Query: &types.WorkflowQuery{
			QueryType: failovermanager.QueryType,
		},
	}
	queryResp, err := client.QueryWorkflow(tcCtx, request)
	if err != nil {
		return nil, PrintableError("Failed to query failover workflow", err)
	}

	if queryResp.GetQueryResult() == nil {
		return nil, PrintableError("QueryResult has no value", nil)
	}
	var queryResult failovermanager.QueryResult
	err = json.Unmarshal(queryResp.GetQueryResult(), &queryResult)
	if err != nil {
		return nil, PrintableError("Unable to deserialize QueryResult", nil)
	}
	return &queryResult, nil
}

func isWorkflowRunning(queryResult *failovermanager.QueryResult) bool {
	return queryResult.State == failovermanager.WorkflowRunning ||
		queryResult.State == failovermanager.WorkflowPaused
}

func getCadenceClient(c *cli.Context) frontend.Client {
	svcClient := cFactory.ServerFrontendClient(c)
	return svcClient
}

func getRunID(c *cli.Context) string {
	if c.IsSet(FlagRunID) {
		return c.String(FlagRunID)
	}
	return ""
}

func failoverStart(c *cli.Context, params *startParams) error {
	err := validateStartParams(params)
	if err != nil {
		return PrintableError("Failed validation of start params", err)
	}

	workflowID := failovermanager.FailoverWorkflowID
	targetCluster := params.targetCluster
	sourceCluster := params.sourceCluster
	batchFailoverSize := params.batchFailoverSize
	batchFailoverWaitTimeInSeconds := params.batchFailoverWaitTimeInSeconds
	workflowTimeout := int32(params.failoverWorkflowTimeout)
	domains := params.domains
	drillWaitTime := time.Duration(params.drillWaitTime) * time.Second
	var gracefulFailoverTimeoutInSeconds *int32
	if params.failoverTimeout > 0 {
		gracefulFailoverTimeoutInSeconds = common.Int32Ptr(int32(params.failoverTimeout))
	}

	client := getCadenceClient(c)
	tcCtx, cancel, err := newContext(c)
	if err != nil {
		return PrintableError("Error creating new context: ", err)
	}
	defer cancel()
	operator, err := getOperator()
	if err != nil {
		return PrintableError("Failed to get operator", err)
	}
	memo, err := getWorkflowMemo(map[string]interface{}{
		common.MemoKeyForOperator: operator,
	})
	if err != nil {
		return PrintableError("Failed to serialize memo", err)
	}
	request := &types.StartWorkflowExecutionRequest{
		Domain:                              common.SystemLocalDomainName,
		RequestID:                           uuid.New(),
		WorkflowID:                          workflowID,
		WorkflowIDReusePolicy:               types.WorkflowIDReusePolicyAllowDuplicate.Ptr(),
		TaskList:                            &types.TaskList{Name: failovermanager.TaskListName},
		ExecutionStartToCloseTimeoutSeconds: common.Int32Ptr(workflowTimeout),
		TaskStartToCloseTimeoutSeconds:      common.Int32Ptr(defaultDecisionTimeoutInSeconds),
		Memo:                                memo,
		WorkflowType:                        &types.WorkflowType{Name: failovermanager.FailoverWorkflowTypeName},
	}
	if params.drillWaitTime > 0 {
		request.WorkflowID = failovermanager.DrillWorkflowID
		request.CronSchedule = params.cron
	} else {
		if len(params.cron) > 0 {
			return PrintableError("The drill wait time is required when cron is specified.", nil)
		}

		// block if there is an on-going failover drill
		if err := executePauseOrResume(c, failovermanager.DrillWorkflowID, true); err != nil {
			switch err.(type) {
			case *types.EntityNotExistsError:
				break
			case *types.WorkflowExecutionAlreadyCompletedError:
				break
			default:
				return PrintableError("Failed to send pase signal to drill workflow", err)
			}
		}
		fmt.Println("The failover drill workflow is paused. Please run 'cadence admin cluster failover resume --fd'" +
			" to resume the drill workflow.")
	}

	foParams := failovermanager.FailoverParams{
		TargetCluster:                    targetCluster,
		SourceCluster:                    sourceCluster,
		BatchFailoverSize:                batchFailoverSize,
		BatchFailoverWaitTimeInSeconds:   batchFailoverWaitTimeInSeconds,
		Domains:                          domains,
		DrillWaitTime:                    drillWaitTime,
		GracefulFailoverTimeoutInSeconds: gracefulFailoverTimeoutInSeconds,
	}
	input, err := json.Marshal(foParams)
	if err != nil {
		return PrintableError("Failed to serialize Failover Params", err)
	}
	request.Input = input
	wf, err := client.StartWorkflowExecution(tcCtx, request)
	if err != nil {
		return PrintableError("Failed to start failover workflow", err)
	}
	fmt.Println("Failover workflow started")
	fmt.Println("wid: " + workflowID)
	fmt.Println("rid: " + wf.GetRunID())
	return nil
}

func getFailoverWorkflowID(c *cli.Context) string {
	if c.Bool(FlagFailoverDrill) {
		return failovermanager.DrillWorkflowID
	}
	return failovermanager.FailoverWorkflowID
}

func getOperator() (string, error) {
	user, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("Unable to get operator info", err)
	}

	return fmt.Sprintf("%s (username: %s)", user.Name, user.Username), nil
}

func isWorkflowTerminated(descResp *types.DescribeWorkflowExecutionResponse) bool {
	return types.WorkflowExecutionCloseStatusTerminated.String() == descResp.GetWorkflowExecutionInfo().GetCloseStatus().String()
}

func executePauseOrResume(c *cli.Context, workflowID string, isPause bool) error {
	client := getCadenceClient(c)
	tcCtx, cancel, err := newContext(c)
	if err != nil {
		return PrintableError("Error creating new context: ", err)
	}
	defer cancel()

	runID := getRunID(c)
	var signalName string
	if isPause {
		signalName = failovermanager.PauseSignal
	} else {
		signalName = failovermanager.ResumeSignal
	}

	request := &types.SignalWorkflowExecutionRequest{
		Domain: common.SystemLocalDomainName,
		WorkflowExecution: &types.WorkflowExecution{
			WorkflowID: workflowID,
			RunID:      runID,
		},
		SignalName: signalName,
		Identity:   getCliIdentity(),
	}

	return client.SignalWorkflowExecution(tcCtx, request)
}

func validateStartParams(params *startParams) error {
	if len(params.targetCluster) == 0 {
		return fmt.Errorf("targetCluster is not provided", nil)
	}
	if len(params.sourceCluster) == 0 {
		return fmt.Errorf("sourceCluster is not provided", nil)
	}
	if params.targetCluster == params.sourceCluster {
		return fmt.Errorf("targetCluster is same as sourceCluster", nil)
	}
	if params.batchFailoverSize <= 0 {
		params.batchFailoverSize = defaultBatchFailoverSize
	}
	if params.batchFailoverWaitTimeInSeconds <= 0 {
		params.batchFailoverWaitTimeInSeconds = defaultBatchFailoverWaitTimeInSeconds
	}
	if params.failoverWorkflowTimeout <= 0 {
		params.failoverWorkflowTimeout = defaultFailoverWorkflowTimeoutInSeconds
	}
	return nil
}
