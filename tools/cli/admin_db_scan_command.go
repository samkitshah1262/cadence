// The MIT License (MIT)
//
// Copyright (c) 2020 Uber Technologies, Inc.
//
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

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/urfave/cli/v2"
	"go.uber.org/zap"

	"github.com/uber/cadence/common"
	"github.com/uber/cadence/common/cache"
	"github.com/uber/cadence/common/collection"
	"github.com/uber/cadence/common/persistence"
	"github.com/uber/cadence/common/reconciliation/fetcher"
	"github.com/uber/cadence/common/reconciliation/invariant"
	"github.com/uber/cadence/common/reconciliation/store"
	"github.com/uber/cadence/service/worker/scanner/executions"
)

const (
	listContextTimeout = time.Minute
)

// AdminDBScan is used to scan over executions in database and detect corruptions.
func AdminDBScan(c *cli.Context) error {
	scanType, err := executions.ScanTypeString(c.String(FlagScanType))

	if err != nil {
		return PrintableError("unknown scan type", err)
	}

	numberOfShards, err := getRequiredIntOption(c, FlagNumberOfShards)
	if err != nil {
		return PrintableError("Error getting number of shards ", err)
	}
	collectionSlice := c.StringSlice(FlagInvariantCollection)

	var collections []invariant.Collection
	for _, v := range collectionSlice {
		collection, err := invariant.CollectionString(v)
		if err != nil {
			return PrintableError("unknown invariant collection", err)
		}
		collections = append(collections, collection)
	}

	logger := zap.NewNop()
	if c.Bool(FlagVerbose) {
		logger, err = zap.NewDevelopment()
		if err != nil {
			// probably impossible with default config
			return PrintableError("could not construct logger", err)
		}
	}

	invariants := scanType.ToInvariants(collections, logger)
	if len(invariants) < 1 {
		return PrintableError(
			fmt.Sprintf("no invariants for scan type %q and collections %q",
				scanType.String(),
				collectionSlice),
			nil,
		)
	}
	ef := scanType.ToExecutionFetcher()

	input, err := getInputFile(c.String(FlagInputFile))
	if err != nil {
		return PrintableError("Error getting input file ", err)
	}

	dec := json.NewDecoder(input)
	if err != nil {
		return PrintableError("", err)
	}
	var data []fetcher.ExecutionRequest

	for {
		var exec fetcher.ExecutionRequest
		if err := dec.Decode(&exec); err != nil {
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break
			}
		} else {
			data = append(data, exec)
		}
	}

	for _, e := range data {
		execution, result, err := checkExecution(c, numberOfShards, e, invariants, ef)
		if err != nil {
			return PrintableError("Error checking execution: ", err)
		}
		out := store.ScanOutputEntity{
			Execution: execution,
			Result:    result,
		}
		data, err := json.Marshal(out)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			continue
		}

		fmt.Println(string(data))
	}
	return nil
}

func checkExecution(
	c *cli.Context,
	numberOfShards int,
	req fetcher.ExecutionRequest,
	invariants []executions.InvariantFactory,
	fetcher executions.ExecutionFetcher,
) (interface{}, invariant.ManagerCheckResult, error) {
	execManager, err := initializeExecutionStore(c, common.WorkflowIDToHistoryShard(req.WorkflowID, numberOfShards))
	if err != nil {
		return nil, invariant.ManagerCheckResult{}, PrintableError("Error initializing exec manager ", err)
	}
	defer execManager.Close()

	historyV2Mgr, err := initializeHistoryManager(c)
	defer historyV2Mgr.Close()

	pr := persistence.NewPersistenceRetryer(
		execManager,
		historyV2Mgr,
		common.CreatePersistenceRetryPolicy(),
	)

	ctx, cancel, err := newContext(c)
	if err != nil {
		return nil, invariant.ManagerCheckResult{}, PrintableError("Error creating new context ", err)
	}
	defer cancel()

	execution, err := fetcher(ctx, pr, req)

	if err != nil {
		return nil, invariant.ManagerCheckResult{}, PrintableError("Error in fetching context: ", err)
	}

	var ivs []invariant.Invariant

	for _, fn := range invariants {
		ivs = append(ivs, fn(pr, cache.NewNoOpDomainCache()))
	}

	return execution, invariant.NewInvariantManager(ivs).RunChecks(ctx, execution), nil
}

// AdminDBScanUnsupportedWorkflow is to scan DB for unsupported workflow for a new release
func AdminDBScanUnsupportedWorkflow(c *cli.Context) error {
	outputFile, err := getOutputFile(c.String(FlagOutputFilename))
	if err != nil {
		return PrintableError("Error getting output file: ", err)
	}
	startShardID := c.Int(FlagLowerShardBound)
	endShardID := c.Int(FlagUpperShardBound)

	defer outputFile.Close()
	for i := startShardID; i <= endShardID; i++ {
		if err := listExecutionsByShardID(c, i, outputFile); err != nil {
			return err
		}
		fmt.Printf("Shard %v scan operation is completed.\n", i)
	}
	return nil
}

func listExecutionsByShardID(
	c *cli.Context,
	shardID int,
	outputFile *os.File,
) error {

	client, err := initializeExecutionStore(c, shardID)
	if err != nil {
		return PrintableError("Error initializinf execution store ", err)
	}
	defer client.Close()

	paginationFunc := func(paginationToken []byte) ([]interface{}, []byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), listContextTimeout)
		defer cancel()

		resp, err := client.ListConcreteExecutions(
			ctx,
			&persistence.ListConcreteExecutionsRequest{
				PageSize:  1000,
				PageToken: paginationToken,
			},
		)
		if err != nil {
			return nil, nil, err
		}
		var paginateItems []interface{}
		for _, history := range resp.Executions {
			paginateItems = append(paginateItems, history)
		}
		return paginateItems, resp.PageToken, nil
	}

	executionIterator := collection.NewPagingIterator(paginationFunc)
	for executionIterator.HasNext() {
		result, err := executionIterator.Next()
		if err != nil {
			return PrintableError(fmt.Sprintf("Failed to scan shard ID: %v for unsupported workflow. Please retry.", shardID), err)
		}
		execution := result.(*persistence.ListConcreteExecutionsEntity)
		executionInfo := execution.ExecutionInfo
		if executionInfo != nil && executionInfo.CloseStatus == 0 && execution.VersionHistories == nil {

			outStr := fmt.Sprintf("cadence --address <host>:<port> --domain <%v> workflow reset --wid %v --rid %v --reset_type LastDecisionCompleted --reason 'release 0.16 upgrade'\n",
				executionInfo.DomainID,
				executionInfo.WorkflowID,
				executionInfo.RunID,
			)
			if _, err = outputFile.WriteString(outStr); err != nil {
				return PrintableError("Failed to write data to file", err)
			}
			if err = outputFile.Sync(); err != nil {
				return PrintableError("Failed to sync data to file", err)
			}
		}
	}
	return nil
}
