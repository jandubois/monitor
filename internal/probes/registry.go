// Package probes provides the built-in probe registry.
package probes

import (
	"github.com/jankremlacek/monitor/internal/probe"
	"github.com/jankremlacek/monitor/internal/probes/command"
	"github.com/jankremlacek/monitor/internal/probes/debug"
	"github.com/jankremlacek/monitor/internal/probes/diskspace"
	"github.com/jankremlacek/monitor/internal/probes/github"
	"github.com/jankremlacek/monitor/internal/probes/gitstatus"
)

// GetAllDescriptions returns descriptions of all built-in probes.
func GetAllDescriptions() []probe.Description {
	return []probe.Description{
		command.GetDescription(),
		debug.GetDescription(),
		diskspace.GetDescription(),
		github.GetDescription(),
		gitstatus.GetDescription(),
	}
}
