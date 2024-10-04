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

	"github.com/urfave/cli/v2"

	"github.com/uber/cadence/common/client"
	"github.com/uber/cadence/common/metrics"
)

// SetFactory is used to set the ClientFactory global
func SetFactory(factory ClientFactory) {
	cFactory = factory
}

// NewCliApp instantiates a new instance of the CLI application.
func NewCliApp() *cli.App {
	version := fmt.Sprintf("CLI feature version: %v \n"+
		"   Release version: %v\n"+
		"   Build commit: %v\n"+
		"   Note: CLI feature version is for compatibility checking between server and CLI if enabled feature checking. Server is always backward compatible to older CLI versions, but not accepting newer than it can support.",
		client.SupportedCLIVersion, metrics.ReleaseVersion, metrics.Revision)

	app := cli.NewApp()
	app.Name = "cadence"
	app.Usage = "A command-line tool for cadence users"
	app.Version = version
	app.Flags = []cli.Flag{
		&cli.StringFlag{
			Name:    FlagAddress,
			Aliases: []string{"ad"},
			Value:   "",
			Usage:   "host:port for cadence frontend service",
			EnvVars: []string{"CADENCE_CLI_ADDRESS"},
		},
		&cli.StringFlag{
			Name:    FlagDomain,
			Aliases: []string{"do"},
			Usage:   "cadence workflow domain",
			EnvVars: []string{"CADENCE_CLI_DOMAIN"},
		},
		&cli.IntFlag{
			Name:    FlagContextTimeout,
			Aliases: []string{"ct"},
			Value:   defaultContextTimeoutInSeconds,
			Usage:   "optional timeout for context of RPC call in seconds",
			EnvVars: []string{"CADENCE_CONTEXT_TIMEOUT"},
		},
		&cli.StringFlag{
			Name:    FlagJWT,
			Usage:   "optional JWT for authorization. Either this or --jwt-private-key is needed for jwt authorization",
			EnvVars: []string{"CADENCE_CLI_JWT"},
		},
		&cli.StringFlag{
			Name:    FlagJWTPrivateKey,
			Aliases: []string{"jwt-pk"},
			Usage:   "optional private key path to create JWT. Either this or --jwt is needed for jwt authorization. --jwt flag has priority over this one if both provided",
			EnvVars: []string{"CADENCE_CLI_JWT_PRIVATE_KEY"},
		},
		&cli.StringFlag{
			Name:    FlagTransport,
			Aliases: []string{"t"},
			Usage:   "optional argument for transport protocol format, either 'grpc' or 'tchannel'. Defaults to tchannel if not provided",
			EnvVars: []string{"CADENCE_CLI_TRANSPORT_PROTOCOL"},
		},
		&cli.StringFlag{
			Name:    FlagTLSCertPath,
			Aliases: []string{"tcp"},
			Usage:   "optional argument for path to TLS certificate. Defaults to an empty string if not provided",
			EnvVars: []string{"CADENCE_CLI_TLS_CERT_PATH"},
		},
	}
	app.Commands = []*cli.Command{
		{
			Name:        "domain",
			Aliases:     []string{"d"},
			Usage:       "Operate cadence domain",
			Subcommands: newDomainCommands(),
		},
		{
			Name:        "workflow",
			Aliases:     []string{"wf"},
			Usage:       "Operate cadence workflow",
			Subcommands: newWorkflowCommands(),
		},
		{
			Name:        "tasklist",
			Aliases:     []string{"tl"},
			Usage:       "Operate cadence tasklist",
			Subcommands: newTaskListCommands(),
		},
		{
			Name:    "admin",
			Aliases: []string{"adm"},
			Usage:   "Run admin operation",
			Subcommands: []*cli.Command{
				{
					Name:        "workflow",
					Aliases:     []string{"wf"},
					Usage:       "Run admin operation on workflow",
					Subcommands: newAdminWorkflowCommands(),
				},
				{
					Name:        "shard",
					Aliases:     []string{"shar"},
					Usage:       "Run admin operation on specific shard",
					Subcommands: newAdminShardManagementCommands(),
				},
				{
					Name:        "history_host",
					Aliases:     []string{"hist"},
					Usage:       "Run admin operation on history host",
					Subcommands: newAdminHistoryHostCommands(),
				},
				{
					Name:        "kafka",
					Aliases:     []string{"ka"},
					Usage:       "Run admin operation on kafka messages",
					Subcommands: newAdminKafkaCommands(),
				},
				{
					Name:        "domain",
					Aliases:     []string{"d"},
					Usage:       "Run admin operation on domain",
					Subcommands: newAdminDomainCommands(),
				},
				{
					Name:        "elasticsearch",
					Aliases:     []string{"es"},
					Usage:       "Run admin operation on ElasticSearch",
					Subcommands: newAdminElasticSearchCommands(),
				},
				{
					Name:        "tasklist",
					Aliases:     []string{"tl"},
					Usage:       "Run admin operation on taskList",
					Subcommands: newAdminTaskListCommands(),
				},
				{
					Name:        "cluster",
					Aliases:     []string{"cl"},
					Usage:       "Run admin operation on cluster",
					Subcommands: newAdminClusterCommands(),
				},
				{
					Name:        "isolation-groups",
					Aliases:     []string{"ig"},
					Usage:       "Run admin operation on isolation-groups",
					Subcommands: newAdminIsolationGroupCommands(),
				},
				{
					Name:        "dlq",
					Usage:       "Run admin operation on DLQ",
					Subcommands: newAdminDLQCommands(),
				},
				{
					Name:        "database",
					Aliases:     []string{"db"},
					Usage:       "Run admin operations on database",
					Subcommands: newDBCommands(),
				},
				{
					Name:        "queue",
					Aliases:     []string{"q"},
					Usage:       "Run admin operations on queue",
					Subcommands: newAdminQueueCommands(),
				},
				{
					Name:        "async-wf-queue",
					Aliases:     []string{"aq"},
					Usage:       "Run admin operations on async workflow queues",
					Subcommands: newAdminAsyncQueueCommands(),
				},
				{
					Name:        "config",
					Aliases:     []string{"c"},
					Usage:       "Run admin operation on config store",
					Subcommands: newAdminConfigStoreCommands(),
				},
			},
		},
		{
			Name:        "cluster",
			Aliases:     []string{"cl"},
			Usage:       "Operate cadence cluster",
			Subcommands: newClusterCommands(),
		},
	}
	app.CommandNotFound = func(context *cli.Context, command string) {
		printMessage("command not found: " + command)
	}

	// set builder if not customized
	if cFactory == nil {
		SetFactory(NewClientFactory())
	}
	return app
}
