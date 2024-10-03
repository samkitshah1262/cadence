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
	"os"
	"time"

	"github.com/urfave/cli/v2"

	"github.com/uber/cadence/common/types"
)

type (
	TaskListPollerRow struct {
		ActivityIdentity string    `header:"Activity Poller Identity"`
		DecisionIdentity string    `header:"Decision Poller Identity"`
		LastAccessTime   time.Time `header:"Last Access Time"`
	}
	TaskListPartitionRow struct {
		ActivityPartition string `header:"Activity Task List Partition"`
		DecisionPartition string `header:"Decision Task List Partition"`
		Host              string `header:"Host"`
	}
)

// DescribeTaskList show pollers info of a given tasklist
func DescribeTaskList(c *cli.Context) error {
	wfClient := getWorkflowClient(c)
	domain, err := getRequiredOption(c, FlagDomain)
	if err != nil {
		return PrintableError("Domain flag not found: ", err)
	}
	taskList, err := getRequiredOption(c, FlagTaskList)
	if err != nil {
		return PrintableError("Tasklist flag not found: ", err)
	}
	taskListType := strToTaskListType(c.String(FlagTaskListType)) // default type is decision

	ctx, cancel, err := newContext(c)
	if err != nil {
		return PrintableError("Error creating new context: ", err)
	}
	defer cancel()

	request := &types.DescribeTaskListRequest{
		Domain: domain,
		TaskList: &types.TaskList{
			Name: taskList,
		},
		TaskListType: &taskListType,
	}
	response, err := wfClient.DescribeTaskList(ctx, request)
	if err != nil {
		return PrintableError("Operation DescribeTaskList failed.", err)
	}

	pollers := response.Pollers
	if len(pollers) == 0 {
		return PrintableError(colorMagenta("No poller for tasklist: "+taskList), nil)
	}

	return printTaskListPollers(pollers, taskListType)
}

// ListTaskListPartitions gets all the tasklist partition and host information.
func ListTaskListPartitions(c *cli.Context) error {
	frontendClient := cFactory.ServerFrontendClient(c)
	domain, err := getRequiredOption(c, FlagDomain)
	if err != nil {
		return PrintableError("Domain flag not found: ", err)
	}
	taskList, err := getRequiredOption(c, FlagTaskList)
	if err != nil {
		return PrintableError("Tasklist flag not found: ", err)
	}

	ctx, cancel, err := newContext(c)
	if err != nil {
		return PrintableError("Error creating new context: ", err)
	}
	defer cancel()
	request := &types.ListTaskListPartitionsRequest{
		Domain:   domain,
		TaskList: &types.TaskList{Name: taskList},
	}

	response, err := frontendClient.ListTaskListPartitions(ctx, request)
	if err != nil {
		return PrintableError("Operation ListTaskListPartitions failed.", err)
	}
	if len(response.DecisionTaskListPartitions) > 0 {
		return printTaskListPartitions("Decision", response.DecisionTaskListPartitions)
	}
	if len(response.ActivityTaskListPartitions) > 0 {
		return printTaskListPartitions("Activity", response.ActivityTaskListPartitions)
	}
	return nil
}

func printTaskListPollers(pollers []*types.PollerInfo, taskListType types.TaskListType) error {
	table := []TaskListPollerRow{}
	for _, poller := range pollers {
		table = append(table, TaskListPollerRow{
			ActivityIdentity: poller.GetIdentity(),
			DecisionIdentity: poller.GetIdentity(),
			LastAccessTime:   time.Unix(0, poller.GetLastAccessTime())})
	}
	return RenderTable(os.Stdout, table, RenderOptions{Color: true, PrintDateTime: true, OptionalColumns: map[string]bool{
		"Activity Poller Identity": taskListType == types.TaskListTypeActivity,
		"Decision Poller Identity": taskListType == types.TaskListTypeDecision,
	}})
}

func printTaskListPartitions(taskListType string, partitions []*types.TaskListPartitionMetadata) error {
	table := []TaskListPartitionRow{}
	for _, partition := range partitions {
		table = append(table, TaskListPartitionRow{
			ActivityPartition: partition.GetKey(),
			DecisionPartition: partition.GetKey(),
			Host:              partition.GetOwnerHostName(),
		})
	}
	return RenderTable(os.Stdout, table, RenderOptions{Color: true, OptionalColumns: map[string]bool{
		"Activity Task List Partition": taskListType == "Activity",
		"Decision Task List Partition": taskListType == "Decision",
	}})
}
