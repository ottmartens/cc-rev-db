package main

import (
	"os"
	"time"

	"github.com/ottmartens/cc-rev-db/command"
	"github.com/ottmartens/cc-rev-db/logger"
)

type NodeReporter int

func (r NodeReporter) Register(pid *int, reply *int) error {

	node := node{
		id:  len(registeredNodes),
		pid: *pid,
	}

	registeredNodes[node.id] = &node

	logger.Verbose("added process %d (pid: %d) to process list", node.id, node.pid)

	*reply = node.id
	return nil
}

func (r NodeReporter) ReportCommandResult(cmd *command.Command, reply *int) error {
	nodeId := cmd.NodeId

	logger.Info("Node %v successfully executed command %v", nodeId, cmd)

	if cmd.Result.Exited {
		logger.Info("Node %v exited", nodeId)

		delete(registeredNodes, nodeId)

		if len(registeredNodes) == 0 {
			go func() {
				time.Sleep(time.Millisecond * 500)
				logger.Info("All nodes exited. Exiting")
				os.Exit(0)
			}()
		}
	}

	return nil
}
