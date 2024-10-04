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
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v2"

	"github.com/uber/cadence/common/types"
	"github.com/uber/cadence/tools/common/commoncli"
)

type (
	TaskListRow struct {
		Name        string `header:"Task List Name"`
		Type        string `header:"Type"`
		PollerCount int    `header:"Poller Count"`
	}
	TaskListStatusRow struct {
		ReadLevel int64   `header:"Read Level"`
		AckLevel  int64   `header:"Ack Level"`
		Backlog   int64   `header:"Backlog"`
		RPS       float64 `header:"RPS"`
		StartID   int64   `header:"Lease Start TaskID"`
		EndID     int64   `header:"Lease End TaskID"`
	}
)

// AdminDescribeTaskList displays poller and status information of task list.
func AdminDescribeTaskList(c *cli.Context) error {
	frontendClient := cFactory.ServerFrontendClient(c)
	domain, err := getRequiredOption(c, FlagDomain)
	if err != nil {
		return PrintableError("Domain flag not found: ", err)
	}
	taskList, err := getRequiredOption(c, FlagTaskList)
	if err != nil {
		return PrintableError("Tasklist flag not found: ", err)
	}
	taskListType := types.TaskListTypeDecision
	if strings.ToLower(c.String(FlagTaskListType)) == "activity" {
		taskListType = types.TaskListTypeActivity
	}

	ctx, cancel, err := newContext(c)
	if err != nil {
		return PrintableError("Error creating new context: ", err)
	}
	defer cancel()
	request := &types.DescribeTaskListRequest{
		Domain:                domain,
		TaskList:              &types.TaskList{Name: taskList},
		TaskListType:          &taskListType,
		IncludeTaskListStatus: true,
	}

	response, err := frontendClient.DescribeTaskList(ctx, request)
	if err != nil {
		return commoncli.Problem("Operation DescribeTaskList failed.", err)
	}

	taskListStatus := response.GetTaskListStatus()
	if taskListStatus == nil {
		return commoncli.Problem(colorMagenta("No tasklist status information."), nil)
	}
	if err := printTaskListStatus(taskListStatus); err != nil {
		return fmt.Errorf("failed to print task list status: %w", err)
	}
	fmt.Printf("\n")

	pollers := response.Pollers
	if len(pollers) == 0 {
		return commoncli.Problem(colorMagenta("No poller for tasklist: "+taskList), nil)
	}
	return printTaskListPollers(pollers, taskListType)
}

// AdminListTaskList displays all task lists under a domain.
func AdminListTaskList(c *cli.Context) error {
	frontendClient := cFactory.ServerFrontendClient(c)
	domain, err := getRequiredOption(c, FlagDomain)
	if err != nil {
		return PrintableError("Domain flag not found: ", err)
	}
	ctx, cancel, err := newContext(c)
	if err != nil {
		return PrintableError("Error creating new context: ", err)
	}
	defer cancel()
	request := &types.GetTaskListsByDomainRequest{
		Domain: domain,
	}

	response, err := frontendClient.GetTaskListsByDomain(ctx, request)
	if err != nil {
		return commoncli.Problem("Operation GetTaskListByDomain failed.", err)
	}

	fmt.Println("Task Lists for domain " + domain + ":")
	table := []TaskListRow{}
	for name, taskList := range response.GetDecisionTaskListMap() {
		table = append(table, TaskListRow{name, "Decision", len(taskList.GetPollers())})
	}
	for name, taskList := range response.GetActivityTaskListMap() {
		table = append(table, TaskListRow{name, "Activity", len(taskList.GetPollers())})
	}
	return RenderTable(os.Stdout, table, RenderOptions{Color: true, Border: true})
}

func printTaskListStatus(taskListStatus *types.TaskListStatus) error {
	table := []TaskListStatusRow{{
		ReadLevel: taskListStatus.GetReadLevel(),
		AckLevel:  taskListStatus.GetAckLevel(),
		Backlog:   taskListStatus.GetBacklogCountHint(),
		RPS:       taskListStatus.GetRatePerSecond(),
		StartID:   taskListStatus.GetTaskIDBlock().GetStartID(),
		EndID:     taskListStatus.GetTaskIDBlock().GetEndID(),
	}}
	return RenderTable(os.Stdout, table, RenderOptions{Color: true})
}
